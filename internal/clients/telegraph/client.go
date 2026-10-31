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

// Init must be called once

func Init() {
	Client = telegraph.GetTelegraphClient(&telegraph.ClientOpt{
		ApiUrl: config.Conf.TelegraphAPI,
		HttpClient: &http.Client{
			Timeout: 6 * time.Second,
		},
	})

	var err error
	Account, err = Client.CreateAccount("Mediainfo", &telegraph.CreateAccountOpts{
		AuthorName: config.Rconf.TgpAuthor,
	})
	if err != nil {
		logger.ErrorW("Could not create Telegraph account", zap.Error(err))
	}
}
