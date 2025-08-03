// utils/telegraph.go
package utils

import (
	"fmt"
	"time"

	"enc/config"
	"enc/internal/clients"

	"github.com/celestix/telegraph-go/v2"
)

// PostToTelegraph publishes a acct, retrying on network errors.
func PostToTelegraph(title, text string) (*telegraph.Page, error) {
	// Derive author and URL (once)
	author := config.Rconf.TgpAuthor
	authorURL := config.Rconf.TgpAuthorUrl

	// placeholder
	if author == "" {
		fn, un := "test", "telegram"
		author = fn
		if authorURL == "" && un != "" {
			authorURL = "https://t.me/" + un
		}
	}

	var lastErr error
	// Use the Account object directly to create pages
	for i := 0; i < 5; i++ {
		page, err := clients.TgpAccount.CreatePage(
			clients.TgpClient,
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
