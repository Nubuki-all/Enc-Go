package telegraph

import (
	"net/http"
	"time"

	"github.com/celestix/telegraph-go/v2"
	"go.uber.org/zap"

	"enc/config"
	"enc/internal/logger"
)

var (
	Account *telegraph.Account
	Client  *telegraph.TelegraphClient
)

// InitTelegraph must be called once (e.g. from main.init()) after config is loaded.

func Init() {
	Client = telegraph.GetTelegraphClient(&telegraph.ClientOpt{
		ApiUrl: config.Conf.TelegraphAPI,
		HttpClient: &http.Client{
			Timeout: 6 * time.Second,
		},
	})

	var err error
	// Create a single account (Mediainfo) and store it
	Account, err = Client.CreateAccount("Mediainfo", &telegraph.CreateAccountOpts{
		AuthorName: config.Rconf.TgpAuthor,
	})
	if err != nil {
		logger.ErrorW("Could not create Telegraph account", zap.Error(err))
	}
}
