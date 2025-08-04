package clients

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/celestix/telegraph-go/v2"
	"github.com/gotd/contrib/middleware/ratelimit"
	"github.com/gotd/td/telegram"
	"golang.org/x/time/rate"

	"enc/config"
	"github.com/celestix/gotgproto"
	//"github.com/celestix/gotgproto/dispatcher/handlers"
	//"github.com/celestix/gotgproto/dispatcher/handlers/filters"
	//"github.com/celestix/gotgproto/ext"
	"github.com/celestix/gotgproto/sessionMaker"
	"github.com/gotd/contrib/middleware/floodwait"
	_ "github.com/mattn/go-sqlite3"
	"gorm.io/driver/sqlite"
)

var (
	Client     *gotgproto.Client
	TgpAccount *telegraph.Account
	TgpClient  *telegraph.TelegraphClient
)

// InitTelegraph must be called once (e.g. from main.init()) after config is loaded.

func InitTelegraph() {
	TgpClient = telegraph.GetTelegraphClient(&telegraph.ClientOpt{
		ApiUrl: config.Conf.TelegraphAPI,
		HttpClient: &http.Client{
			Timeout: 6 * time.Second,
		},
	})

	var err error
	// Create a single account (Mediainfo) and store it
	TgpAccount, err = TgpClient.CreateAccount("Mediainfo", &telegraph.CreateAccountOpts{
		AuthorName: config.Rconf.TgpAuthor,
	})
	if err != nil {
		log.Fatalf("could not create Telegraph account: %v", err)
	}
}

func InitTelegramClient() {
	var err error
	// Initializing flood waiter, which will wait for stated duration if "FLOOD_WAIT" error occurred
	waiter := floodwait.NewWaiter().WithCallback(func(ctx context.Context, wait floodwait.FloodWait) {
		fmt.Printf("Waiting for flood, dur: %d\n", wait.Duration)
	})
	// Initializing ratelimiter, which will allow at most 30 requests to Telegram in 100ms
	ratelimiter := ratelimit.New(rate.Every(time.Millisecond*100), 30)

	Client, err = gotgproto.NewClient(
		// Get AppID from https://my.telegram.org/apps
		config.Conf.AppID,
		// Get ApiHash from https://my.telegram.org/apps
		config.Conf.APIHash,
		// ClientType, as we defined above
		gotgproto.ClientTypeBot(config.Conf.BotToken),
		// Optional parameters of client
		&gotgproto.ClientOpts{
			Session:     sessionMaker.SqlSession(sqlite.Open(".encbot.session")),
			Middlewares: []telegram.Middleware{waiter, ratelimiter},
			RunMiddleware: func(origRun func(ctx context.Context, f func(ctx context.Context) error) (err error), ctx context.Context, f func(ctx context.Context) (err error)) (err error) {
				return origRun(ctx, func(ctx context.Context) error {
					return waiter.Run(ctx, f)
				})
			},
		},
	)
	if err != nil {
		log.Fatalln("failed to start client:", err)
	}

	fmt.Printf("client (@%s) has been started...\n", Client.Self.Username)
}
