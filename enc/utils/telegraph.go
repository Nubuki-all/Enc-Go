// utils/telegraph.go
package utils

import (
	"fmt"
	"time"

	"github.com/celestix/telegraph-go/v2"

	"enc/config"
	"enc/internal/clients/telegram"
	tgph "enc/internal/clients/telegraph"
)

// PostToTelegraph publishes a acct, retrying on network errors.
func PostToTelegraph(title, text string) (*telegraph.Page, error) {
	if tgph.Client == nil {
		return nil, fmt.Errorf("PostToTelegraph failed because client wasn't successfully initialized")
	}
	// Derive author and URL (once)
	author := config.Rconf.TgpAuthor
	authorURL := config.Rconf.TgpAuthorURL

	// placeholder
	if author == "" {
		self := telegram.Client.Self
		author = self.FirstName
		un := self.Username
		if authorURL == "" && un != "" {
			authorURL = "https://t.me/" + un
		}
	}

	var lastErr error
	// Use the Account object directly to create pages
	for i := 0; i < 5; i++ {
		if tgph.Account == nil {
			tgph.Account, lastErr = tgph.Client.CreateAccount("Mediainfo", &telegraph.CreateAccountOpts{
				AuthorName: config.Rconf.TgpAuthor,
			})
			if lastErr != nil {
				time.Sleep(time.Second)
				continue
			}
		}
		page, err := tgph.Account.CreatePage(
			tgph.Client,
			title,
			text,
			&telegraph.PageOpts{
				AuthorName: author,
				AuthorUrl:  authorURL,
			},
		)
		if err == nil {
			return page, nil
		}
		lastErr = err
		time.Sleep(time.Second)
	}
	return nil, fmt.Errorf("PostToTelegraph failed after retries: %w", lastErr)
}
