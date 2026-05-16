package telegram

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	botapi "enc/internal/clients/telegram/botapi"
	"enc/internal/clients/telegram/botapi/models"
	"go.uber.org/zap"

	"enc/config"
	"enc/internal/logger"
)

var (
	BotAPI       *botapi.Bot
	botAPIOnce   sync.Once
	botAPICtx    context.Context
	botAPICancel context.CancelFunc
)

func InitBotAPIClient() {
	if BotAPI != nil {
		return
	}
	opts := []botapi.Option{
		botapi.WithHTTPClient(50*time.Second, &http.Client{Timeout: 60 * time.Second}),
		botapi.WithUpdatesChannelCap(1024),
	}
	if url := strings.TrimSpace(config.Conf.BotAPIURL); url != "" {
		opts = append(opts, botapi.WithServerURL(strings.TrimRight(url, "/")))
	}
	b, err := botapi.New(config.Conf.BotToken, opts...)
	if err != nil {
		logger.CriticalW("failed to start bot api client:", zap.Error(err))
	}
	BotAPI = b
	logger.InfoW("bot api client has been started", zap.String("server", config.Conf.BotAPIURL))
}

func StartBotAPIDispatcher() {
	if BotAPI == nil {
		logger.WarnW("bot api dispatcher requested without bot api client")
		return
	}
	botAPIOnce.Do(func() {
		botAPICtx, botAPICancel = context.WithCancel(context.Background())
		go BotAPI.Start(botAPICtx)
	})
}

func StopBotAPI() {
	if botAPICancel != nil {
		botAPICancel()
	}
}

func RuntimeContext() context.Context {
	if Context != nil {
		return Context
	}
	if botAPICtx != nil {
		return botAPICtx
	}
	return context.Background()
}

func SendBotAPIMessage(ctx context.Context, chatID int64, text string) (*models.Message, error) {
	if BotAPI == nil {
		return nil, errors.New("bot api client is not initialized")
	}
	return BotAPI.SendMessage(ctx, &botapi.SendMessageParams{ChatID: chatID, Text: text})
}

func EditBotAPIMessage(ctx context.Context, chatID int64, msgID int, text string) (*models.Message, error) {
	if BotAPI == nil {
		return nil, errors.New("bot api client is not initialized")
	}
	return BotAPI.EditMessageText(ctx, &botapi.EditMessageTextParams{ChatID: chatID, MessageID: msgID, Text: text})
}

func DeleteBotAPIMessage(ctx context.Context, chatID int64, msgID int) error {
	if BotAPI == nil {
		return errors.New("bot api client is not initialized")
	}
	_, err := BotAPI.DeleteMessage(ctx, &botapi.DeleteMessageParams{ChatID: chatID, MessageID: msgID})
	return err
}

func AnswerBotAPICallback(ctx context.Context, callbackID, text string, alert bool) error {
	if BotAPI == nil {
		return errors.New("bot api client is not initialized")
	}
	_, err := BotAPI.AnswerCallbackQuery(ctx, &botapi.AnswerCallbackQueryParams{CallbackQueryID: callbackID, Text: text, ShowAlert: alert})
	return err
}
