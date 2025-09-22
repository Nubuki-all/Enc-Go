// Package telegram Initializes the telegram Client
package telegram

import (
	//"context"
	"time"

	"github.com/celestix/gotgproto"
	"github.com/celestix/gotgproto/ext"
	"github.com/celestix/gotgproto/sessionMaker"
	//"github.com/gotd/contrib/middleware/floodwait"
	"github.com/gotd/contrib/middleware/ratelimit"
	"github.com/gotd/td/telegram"
	_ "github.com/mattn/go-sqlite3"
	//"go.uber.org/zap"
	"golang.org/x/time/rate"
	"gorm.io/driver/sqlite"

	//"github.com/celestix/gotgproto/dispatcher/handlers"
	//"github.com/celestix/gotgproto/dispatcher/handlers/filters"

	"enc/config"
	"enc/internal/logger"
	"enc/utils/floodwait"
)

var (
	Client  *gotgproto.Client
	Context *ext.Context
)

func InitClient() {
	var err error
	// Initializing flood waiter, which will wait for stated duration if "FLOOD_WAIT" error occurred
	waiter := floodwait.NewSimpleWaiter()
	// Initializing ratelimiter, which will allow at most 30 requests to Telegram in 100ms
	ratelimiter := ratelimit.New(rate.Every(time.Millisecond*100), 30)
	panicHandler := func (_ *ext.Context, _ *ext.Update, err string) {
		logger.Error("Panic!: " + err)
	}
	errHandler := func (_ *ext.Context, _ *ext.Update, _ string) error {
		return nil
	}

	Client, err = gotgproto.NewClient(
		// Get AppID from https://my.telegram.org/apps
		config.Conf.AppID,
		// Get ApiHash from https://my.telegram.org/apps
		config.Conf.APIHash,
		// ClientType, as we defined above
		gotgproto.ClientTypeBot(config.Conf.BotToken),
		// Optional parameters of client
		&gotgproto.ClientOpts{
			Logger:      logger.Logger,
			Session:     sessionMaker.SqlSession(sqlite.Open(".encbot.session")),
			PanicHandler: panicHandler,
			ErrorHandler: errHandler,
			Middlewares: []telegram.Middleware{waiter, ratelimiter},
		},
	)
	if err != nil {
		logger.Criticalf("failed to start client:", err)
	}

	logger.Infof("client (@%s) has been started...\n", Client.Self.Username)
	Context = Client.CreateContext()
}

