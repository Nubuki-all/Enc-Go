// Package startup wires up background workers, clients, and hook handlers.
package startup

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/siku2/arigo"
	"go.uber.org/zap"

	"enc/config"
	"enc/internal/bot"
	"enc/internal/bot/entimer"
	"enc/internal/clients/telegram"
	"enc/internal/db"
	"enc/internal/logger"

	"enc/utils"
	"enc/utils/random"
	"enc/utils/tele"

	//"enc/internal/rss"
	//"enc/internal/workers"
	"enc/workers/auto/channelstatus"
	"enc/workers/auto/rssmonitor"
	"enc/workers/auto/transcode"
	"enc/workers/downloaders"
)

func Finalize(ctx context.Context) {
	// hack to avoid circular import
	telegram.ReAddHandlers = AddAllHandlers

	bot.ID, _ = strconv.ParseInt(config.Rconf.BotID, 10, 64)
	if !config.Conf.NoExternalDL {
		go startRPC()
		go startQbit()
	}
	rssmonitor.ScheduleRSS(config.Conf.RSSDelay)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGABRT)
	go func() {
		<-sigCh
		onTermination(ctx)
	}()

	// handle restart
	var startDetached bool
	for _, arg := range os.Args {
		if arg == "--detached" {
			config.Rconf.Detached = true
		}
		if arg == "--start-detached" {
			startDetached = true
		}
	}
	if startDetached && !config.Rconf.Detached {
		_ = utils.Restart(0, 0, true)
	}

	if len(os.Args) >= 3 && os.Args[1] == "restart" {
		if err := onRestart(); err != nil {
			logger.WarnW("onRestart failed", zap.Error(err))
		}
	} else {
		time.Sleep(1 * time.Second)
		onStart()
	}
	downloaders.CleanAllQbit(ctx) // prevent stale qbit downloads from blocking newer ones

	entimer.Timer.Start()
	AddAllCommands()
	AddAllHandlers()
	telegram.StartUpdates()

	// start autos
	channelstatus.Start()
	transcode.Start()
	go startAutoRestart()
	go startTelegramPing()
	bot.Started = true
}

func startAutoRestart() {
	interval := config.Rconf.AutoRestart
	if interval <= 0 {
		return
	}
	time.Sleep(interval)
	logger.Info("Auto Restarting...")

	var chatID int64
	if config.Conf.LogChannel != 0 {
		chatID = config.Conf.LogChannel
	} else if len(config.Rconf.Owners) > 0 {
		chatID = config.Rconf.Owners[0]
	}

	var msgID int
	if chatID != 0 {
		msg, err := tele.SendMsg(chatID, "`Auto Restarting Please Wait…`", nil)
		if err == nil {
			msgID = msg.ID
		} else {
			chatID = 0
		}
	}

	_ = utils.Restart(chatID, msgID, config.Rconf.Detached)
}

func startTelegramPing() {
	for {
		time.Sleep(30 * time.Minute)
		if err := safePing(); err != nil {
			logger.WarnW("Telegram ping failed, retrying in 1 minute", zap.Error(err))
			time.Sleep(1 * time.Minute)
			if err := safePing(); err != nil {
				logger.ErrorW("Telegram connection lost, restarting...", zap.Error(err))
				_ = utils.Restart(0, 0, config.Rconf.Detached)
			}
		}
	}
}

func safePing() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("recovered from panic in Ping: %v", r)
		}
	}()
	if telegram.Client == nil {
		return fmt.Errorf("telegram.Client is nil")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return telegram.Client.Ping(ctx)
}

// startRPC launches aria2c in RPC mode, then connects the Go API client
func startRPC() {
	bot.DownloaderMutex.RLock()
	defer bot.DownloaderMutex.RUnlock()
	cmd := exec.Command(
		"aria2c",
		"--enable-rpc=true",
		"--rpc-listen-all",
		"--rpc-max-request-size=1024M",
		"--rpc-listen-port="+strconv.Itoa(config.Conf.Aria2Port),
		"--seed-time=0",
		"--follow-torrent=mem",
		"--summary-interval=0",
		"--daemon=true",
		"--allow-overwrite=true",
		"--user-agent=Wget/1.12",
		"--bt-remove-unselected-file=true",
		"--max-connection-per-server=10",
		"--http-accept-gzip=true",
		"--split=10",
		"--disk-cache=40M",
		"--min-split-size=10M",
		"--optimize-concurrent-downloads=true",
	)
	if err := cmd.Start(); err != nil {
		logger.ErrorW("failed to start aria2c", zap.Error(err))
		return
	}
	// wait a moment then create API client
	time.Sleep(1 * time.Second)
	client, err := arigo.Dial(fmt.Sprint("ws://localhost:", config.Conf.Aria2Port, "/jsonrpc"), "")
	if err != nil {
		logger.ErrorW("aria2 API init failed", zap.Error(err))
		return
	}
	_, err = client.AddURI(arigo.URIs("https://nyaa.si/download/1752639.torrent"), nil)
	if err != nil {
		logger.ErrorW("aria2 API test download failed", zap.Error(err))
		return
	}
	time.Sleep(1 * time.Second)
	statuses := client.GetDownloads()
	client.DeleteDownloads(statuses, true, true, true)
	bot.Aria2Client = client
	bot.SAS = true
}

// startQbit launches two qbittorrent‐nox daemons on different ports
func startQbit() {
	bot.DownloaderMutex.RLock()
	defer bot.DownloaderMutex.RUnlock()
	// Determine working directory at runtime
	wd, err := os.Getwd()
	if err != nil {
		logger.ErrorW("could not get working directory", zap.Error(err))
		return
	}
	anyStarted := false

	ports := []int{config.Conf.QBitPort, config.Conf.QBitPort2}
	for i, p := range ports {
		// Base arguments common to both instances
		args := []string{
			"-d",
			"--webui-port=" + strconv.Itoa(p),
			"--profile=" + filepath.Join(wd, ".qbit"),
		}

		// For the second port, add --configuration=1
		if i == 1 {
			args = append(args, "--configuration=1")
		}

		cmd := exec.Command("qbittorrent-nox", args...)
		if err := cmd.Start(); err != nil {
			logger.WarnW(
				"failed to start qbittorrent-nox",
				zap.Int("port", p),
				zap.Strings("args", args),
				zap.Error(err),
			)
		} else {
			anyStarted = true
		}
	}

	bot.SQS = anyStarted
}

// onStart sends a startup notification to owners and log channel
func onStart() {
	for _, owner := range config.Rconf.Owners {
		cerr := tele.SendMessage(
			owner,
			fmt.Sprintf(
				"**I'm %s %s**",
				random.Enquip(),
				random.Enmoji(),
			),
		)
		if cerr != nil {
			logger.ErrorW(cerr.Error())
		}
	}
	if config.Conf.LogChannel != 0 {
		err := tele.SendMessage(
			config.Conf.LogChannel,
			fmt.Sprintf(
				"**Bot is %s %s**",
				random.Enquip(),
				random.Enmoji(),
			),
		)
		if err != nil {
			logger.ErrorW(err.Error())
		}
	}
	// report aria2/qbit status
	statusMessages := make(map[bool]string)
	statusMessages[true] = "Online"
	statusMessages[false] = "Offline/Not_ready"
	var ariStat, qbitStat string
	if message, ok := statusMessages[bot.SAS]; ok {
		ariStat = message
	}
	if message, ok := statusMessages[bot.SQS]; ok {
		qbitStat = message
	}
	status := fmt.Sprintf("**Aria2:** `%s`\n**Qbit:** `%s`",
		ariStat, qbitStat)
	var id int64
	if config.Conf.LogChannel != 0 {
		id = config.Conf.LogChannel
	} else {
		id, _ = strconv.ParseInt(
			strings.Split(config.Conf.Owner, " ")[0],
			10,
			64,
		)
	}
	err := tele.SendMessage(id, status)
	if err != nil {
		logger.ErrorW(err.Error())
	}
}

// onRestart handles “restart” or “update” command‐line flags
func onRestart() error {
	cmd := os.Args[1]
	arg := os.Args[2] // ex: "12345:678"
	parts := strings.Split(arg, ":")
	if len(parts) != 2 {
		return fmt.Errorf("invalid restart arg: %s", arg)
	}
	chatID, _ := strconv.ParseInt(parts[0], 10, 64)
	msgID, _ := strconv.Atoi(parts[1])

	if chatID == 0 && msgID == 0 {
		onStart()
		return nil
	}

	var msg string
	if cmd == "restart" {
		msg = "**Restarted!**"
	} else if strings.HasPrefix(cmd, "update") {
		fields := strings.Split(cmd, " ")
		if len(fields) > 1 && fields[1] == "True" {
			msg = "**Updated to:** `" + bot.Version + "`"
		} else {
			msg = "**No major update found, but restarted!**"
		}
	} else {
		return nil
	}
	err := tele.EditMessage(
		chatID,
		msgID,
		msg,
	)
	return err
}

// onTermination sends a farewell and cleans up
func onTermination(ctx context.Context) {
	defer func() {
		// persist state
		downloaders.CleanAllQbit(ctx)
		db.SaveAll(ctx)
		if db.UseMongo() {
			err := db.Client.Disconnect(ctx)
			if err != nil {
				logger.Warn(err)
			}
		} // else {
		//	localdb.SaveQueueLocal()
		//	localdb.SaveSettingsLocal()
		//	localdb.SaveTempUsersLocal()
		//	localdb.SaveRSSLocal()
		//}
		os.Exit(0)
	}()

	farewell := fmt.Sprintf("**I'm %s %s**", random.Enquip2(), random.Enmoji2())
	if config.Conf.LogChannel != 0 {
		err := tele.SendMessage(config.Conf.LogChannel, farewell)
		if err != nil {
			logger.ErrorW(err.Error())
		}
	} else {
		for _, owner := range config.Rconf.Owners {
			serr := tele.SendMessage(owner, farewell)
			if serr != nil {
				logger.ErrorW(serr.Error())
			}
		}
	}
}
