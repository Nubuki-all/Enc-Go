// Package telegram Initializes the telegram Client
package telegram

import (
	"errors"
	"time"

	"github.com/celestix/gotgproto"
	"github.com/celestix/gotgproto/ext"
	"github.com/celestix/gotgproto/sessionMaker"
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

var (
	Client   *gotgproto.Client
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

func InitClients() {
	if config.Conf.TgDispatcher.UsesGotg() || config.Conf.TgTransfer.UsesGotg() {
		InitClient()
	}
	if config.Conf.TgDispatcher.UsesBotAPI() || config.Conf.TgTransfer.UsesBotAPI() {
		InitBotAPIClient()
	}
}

func InitClient() {
	var err error
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
		},
	)
	if err != nil {
		logger.CriticalW("failed to start client:", zap.Error(err))
	}

	logger.Infof("client (@%s) has been started...\n", Client.Self.Username)
	Context = Client.CreateContext()
}

func Restart() error {
	if Client == nil && BotAPI == nil {
		return errors.New("no telegram client available to restart")
	}
	if Client != nil {
		Client.Stop()
		Client = nil
	}
	StopBotAPI()
	BotAPI = nil
	InitClients()
	if ReAddHandlers != nil {
		ReAddHandlers()
	}
	return nil
}
