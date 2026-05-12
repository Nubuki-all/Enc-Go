// Package telegram Initializes the telegram Client
package telegram

import (
	"errors"
	"time"

	"github.com/celestix/gotgproto"
	"github.com/celestix/gotgproto/ext"
	"github.com/celestix/gotgproto/sessionMaker"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/gotd/contrib/middleware/ratelimit"
	"github.com/gotd/td/telegram"
	_ "github.com/mattn/go-sqlite3"

	"go.uber.org/zap"

	"golang.org/x/time/rate"

	"gorm.io/driver/sqlite"

	"enc/config"
	"enc/internal/logger"
	"enc/utils/floodwait"
)

// HandleBotAPIUpdate quick hack to avoid circular import
var HandleBotAPIUpdate func(update tgbotapi.Update)

var (
	Client   *gotgproto.Client
	BotAPI   *tgbotapi.BotAPI
	Context  *ext.Context
	stopChan = make(chan struct{})
	Idle     = func() {
		<-stopChan
	}
	Stop = func() {
		close(stopChan)
	}
)

// ReAddHandlers quick hack to avoid circular import
var ReAddHandlers func()

func InitClient() {
	var err error
	if config.Conf.UseBotAPI {
		BotAPI, err = tgbotapi.NewBotAPI(config.Conf.BotToken)
		if err != nil {
			logger.CriticalW("failed to start bot api client:", zap.Error(err))
		}
	}

	waiter := floodwait.NewSimpleWaiter()
	ratelimiter := ratelimit.New(rate.Every(time.Millisecond*100), 30)
	panicHandler := func(_ *ext.Context, _ *ext.Update, err string) {
		logger.ErrorW("Panic!", zap.String("Error", err))
	}
	errHandler := func(_ *ext.Context, _ *ext.Update, _ string) error {
		return nil
	}

	Client, err = gotgproto.NewClient(
		config.Conf.AppID,
		config.Conf.APIHash,
		gotgproto.ClientTypeBot(config.Conf.BotToken),
		&gotgproto.ClientOpts{
			Logger:       logger.Logger,
			Session:      sessionMaker.SqlSession(sqlite.Open(".encbot.session")),
			PanicHandler: panicHandler,
			ErrorHandler: errHandler,
			Middlewares:  []telegram.Middleware{waiter, ratelimiter},
			NoUpdates:    config.Conf.UseBotAPI,
		},
	)
	if err != nil {
		logger.CriticalW("failed to start client:", zap.Error(err))
	}

	logger.Infof("client (@%s) has been started...\n", Client.Self.Username)
	Context = Client.CreateContext()
}

func StartUpdates() {
	if config.Conf.UseBotAPI && BotAPI != nil {
		go startBotAPIUpdateLoop()
	}
}

func startBotAPIUpdateLoop() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := BotAPI.GetUpdatesChan(u)

	for update := range updates {
		if HandleBotAPIUpdate != nil {
			HandleBotAPIUpdate(update)
		}
	}
}

func Restart() error {
	if Client == nil {
		return errors.New("no telegram client available to restart")
	}
	Client.Stop()
	InitClient()
	ReAddHandlers()
	return nil
}
