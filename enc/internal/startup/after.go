// internal/startup/after.go
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

	"github.com/gotd/td/tg"
	"github.com/siku2/arigo"
	"go.uber.org/zap"

	"enc/config"
	"enc/internal/bot"
	"enc/internal/bot/entimer"
	"enc/internal/clients/telegram"
	"enc/internal/clients/telegraph"
	"enc/internal/db"
	"enc/internal/logger"
	"enc/utils"
	"enc/utils/random"
	"enc/utils/tele"
	//"enc/internal/rss"
	//"enc/internal/workers"
)

// After wires up background workers, RPC & torrent clients, and hook handlers.
func Finalize(ctx context.Context) {
	// 1) Start your RPC (aria2) process and API client
	go startRPC()

	// 2) Start qbittorrent-nox instances
	go startQbit()

	// 3) Kick off your scheduler and auto‐jobs
	// rss.Scheduler.Start()
	// go workers.AutoStatus(ctx)
	// go workers.AutoTranscode(ctx)

	// 4) Setup OS signal handlers for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGABRT)
	go func() {
		<-sigCh
		onTermination(ctx)
	}()

	// 5) Handle a restart/update invocation from os.Args
	if len(os.Args) == 3 {
		if err := onRestart(); err != nil {
			logger.WarnW("onRestart failed", zap.Error(err))
		}
	} else {
		// give processes a moment, then send onStart notifications
		time.Sleep(1 * time.Second)
		onStart()
	}

	// 6) Start the encoding‐pause timer
	entimer.Timer.Start()

	// 7) Reset the encode job state
	// workers.ResetEncodeJob(true)

	// 8) After a delay, trigger your background \"something\" task
	//go func() {
	//	time.Sleep(30 * time.Second)
	//	workers.Something(ctx)
	//}()

	// 9) Mark the bot as started
	bot.Started = true
}

// startRPC launches aria2c in RPC mode, then connects the Go API client
func startRPC() {
	cmd := exec.Command("aria2c",
		"--enable-rpc=true",
		"--rpc-listen-port="+strconv.Itoa(config.Conf.Aria2Port),
		"--daemon=true",
		"--allow-overwrite=true",
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
	gid, err := client.AddURI(arigo.URIs("https://nyaa.si/download/1752639.torrent"), nil)
	if err != nil {
		logger.ErrorW("aria2 API test download failed", zap.Error(err))
		return
	}
	time.Sleep(1 * time.Second)
	err = gid.Delete()
	if err != nil {
		logger.ErrorW("aria2 API test cleanup failed", zap.Error(err))
		return
	}
	bot.Aria2Client = &client
	bot.SAS = true
}

// startQbit launches two qbittorrent‐nox daemons on different ports
func startQbit() {
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
			"--profile=" + filepath.Join(wd, "qbit"),
		}

		// For the second port, add --configuration=1
		if i == 1 {
			args = append(args, "--configuration=1")
		}

		cmd := exec.Command("qbittorrent-nox", args...)
		if err := cmd.Start(); err != nil {
			logger.WarnW("failed to start qbittorrent-nox",
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
	for _, owner := range strings.Fields(config.Conf.Owner) {
		if id, err := strconv.ParseInt(owner, 10, 64); err == nil {
			peer, _ := tele.GetPeer(id)
			// fmt.Println(peer, id, perr)
			text, entities, _ := tele.MarkdownToEntitiesGeneric(fmt.Sprintf("**I'm %s %s**", random.Enquip(), random.Enmoji()))
			_, cerr := telegram.Context.SendMessage(id, &tg.MessagesSendMessageRequest{
				Message:  text,
				Peer:     peer,
				Entities: tele.ToGotdEntities(entities),
			},
			)
			if cerr != nil {
				logger.ErrorW(cerr.Error())
			}
		} else if err != nil {
			logger.ErrorW(err.Error())
		}
	}
	if config.Conf.LogChannel != 0 {
		telegram.Context.SendMessage(
			config.Conf.LogChannel, &tg.MessagesSendMessageRequest{
				Message: fmt.Sprintf("**Bot is %s %s**", random.Enquip(), random.Enmoji()),
			},
		)
	}
	// report aria2/qbit status
	status := fmt.Sprintf("**Aria2:** `%v`\n**Qbit:** `%v`",
		bot.SAS, bot.SQS)
	var id int64
	if config.Conf.LogChannel != 0 {
		id = config.Conf.LogChannel
	} else {
		id, _ = strconv.ParseInt((strings.Split(config.Conf.Owner, " ")[0]), 10, 64)
	}
	telegram.Context.SendMessage(id, &tg.MessagesSendMessageRequest{
		Message: status,
	})
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

	var msg string
	if cmd == "restart" {
		msg = "**Restarted!**"
	} else if strings.HasPrefix(cmd, "update") {
		fields := strings.Split(cmd, " ")
		if len(fields) > 1 && fields[1] == "True" {
			msg = "**Updated successfully!**"
		} else {
			msg = "**No major update found, but restarted!**"
		}
	} else {
		return nil
	}
	_, err := telegram.Context.EditMessage(
		chatID,
		&tg.MessagesEditMessageRequest{
			ID:      msgID,
			Message: msg,
		},
	)
	return err
}

// onTermination sends a farewell and cleans up
func onTermination(ctx context.Context) {
	defer func() {
		// persist state
		db.SaveAll(ctx)
		if db.UseMongo() {
			db.Client.Disconnect(ctx)
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
		telegram.Context.SendMessage(config.Conf.LogChannel, &tg.MessagesSendMessageRequest{
			Message: farewell,
		})
	} else {
		for _, owner := range strings.Fields(config.Conf.Owner) {
			if id, err := strconv.ParseInt(owner, 10, 64); err == nil {
				telegram.Context.SendMessage(id, &tg.MessagesSendMessageRequest{
					Message: farewell,
				})
			}
		}
	}
}
