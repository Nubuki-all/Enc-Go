package botapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"enc/internal/clients/telegram/botapi/models"
)

type (
	HandlerFunc func(context.Context, *Bot, *models.Update)
	HandlerType int
	MatchType   int
	Option      func(*Bot)
	HttpClient  interface {
		Do(*http.Request) (*http.Response, error)
	}
)

const (
	HandlerTypeMessageText HandlerType = iota
	HandlerTypeCallbackQueryData
)

const (
	MatchTypeExact MatchType = iota
	MatchTypePrefix
	MatchTypeContains
)

type Bot struct {
	token      string
	serverURL  string
	httpClient HttpClient
	pollTime   time.Duration
	cap        int
	handlers   []handler
}

type handler struct {
	typeOf    HandlerType
	pattern   string
	matchType MatchType
	re        *regexp.Regexp
	fn        HandlerFunc
}

func New(token string, opts ...Option) (*Bot, error) {
	b := &Bot{token: token, serverURL: "https://api.telegram.org", httpClient: http.DefaultClient, pollTime: 50 * time.Second, cap: 1024}
	for _, opt := range opts {
		opt(b)
	}
	return b, nil
}

func WithHTTPClient(pollTimeout time.Duration, client HttpClient) Option {
	return func(b *Bot) {
		b.pollTime = pollTimeout
		if client != nil {
			b.httpClient = client
		}
	}
}

func WithUpdatesChannelCap(cap int) Option { return func(b *Bot) { b.cap = cap } }
func WithServerURL(serverURL string) Option {
	return func(b *Bot) { b.serverURL = strings.TrimRight(serverURL, "/") }
}

func (b *Bot) RegisterHandler(handlerType HandlerType, pattern string, matchType MatchType, fn HandlerFunc) string {
	b.handlers = append(b.handlers, handler{typeOf: handlerType, pattern: pattern, matchType: matchType, fn: fn})
	return fmt.Sprintf("%d", len(b.handlers)-1)
}

func (b *Bot) RegisterHandlerRegexp(handlerType HandlerType, re *regexp.Regexp, fn HandlerFunc) string {
	b.handlers = append(b.handlers, handler{typeOf: handlerType, re: re, fn: fn})
	return fmt.Sprintf("%d", len(b.handlers)-1)
}

func (b *Bot) Start(ctx context.Context) {
	offset := 0
	for ctx.Err() == nil {
		updates, err := b.getUpdates(ctx, offset)
		if err != nil {
			time.Sleep(time.Second)
			continue
		}
		for i := range updates {
			upd := updates[i]
			if upd.ID >= offset {
				offset = upd.ID + 1
			}
			b.dispatch(ctx, &upd)
		}
	}
}

func (b *Bot) dispatch(ctx context.Context, upd *models.Update) {
	for _, h := range b.handlers {
		if h.matches(upd) {
			go h.fn(ctx, b, upd)
		}
	}
}

func (h handler) matches(upd *models.Update) bool {
	var val string
	switch h.typeOf {
	case HandlerTypeMessageText:
		if upd.Message == nil {
			return false
		}
		val = upd.Message.Text
	case HandlerTypeCallbackQueryData:
		if upd.CallbackQuery == nil {
			return false
		}
		val = upd.CallbackQuery.Data
	}
	if h.re != nil {
		return h.re.MatchString(val)
	}
	switch h.matchType {
	case MatchTypeExact:
		return val == h.pattern
	case MatchTypePrefix:
		return strings.HasPrefix(val, h.pattern)
	case MatchTypeContains:
		return strings.Contains(val, h.pattern)
	default:
		return false
	}
}

type SendMessageParams struct {
	ChatID any    `json:"chat_id"`
	Text   string `json:"text"`
}
type EditMessageTextParams struct {
	ChatID    any    `json:"chat_id"`
	MessageID int    `json:"message_id"`
	Text      string `json:"text"`
}
type DeleteMessageParams struct {
	ChatID    any `json:"chat_id"`
	MessageID int `json:"message_id"`
}
type AnswerCallbackQueryParams struct {
	CallbackQueryID string `json:"callback_query_id"`
	Text            string `json:"text,omitempty"`
	ShowAlert       bool   `json:"show_alert,omitempty"`
}
type SetMyCommandsParams struct {
	Commands []models.BotCommand `json:"commands"`
}

func (b *Bot) SendMessage(ctx context.Context, params *SendMessageParams) (*models.Message, error) {
	var out models.Message
	return &out, b.call(ctx, "sendMessage", params, &out)
}

func (b *Bot) EditMessageText(ctx context.Context, params *EditMessageTextParams) (*models.Message, error) {
	var out models.Message
	return &out, b.call(ctx, "editMessageText", params, &out)
}

func (b *Bot) DeleteMessage(ctx context.Context, params *DeleteMessageParams) (bool, error) {
	var out bool
	return out, b.call(ctx, "deleteMessage", params, &out)
}

func (b *Bot) AnswerCallbackQuery(ctx context.Context, params *AnswerCallbackQueryParams) (bool, error) {
	var out bool
	return out, b.call(ctx, "answerCallbackQuery", params, &out)
}

func (b *Bot) SetMyCommands(ctx context.Context, params *SetMyCommandsParams) (bool, error) {
	var out bool
	return out, b.call(ctx, "setMyCommands", params, &out)
}

func (b *Bot) getUpdates(ctx context.Context, offset int) ([]models.Update, error) {
	var out []models.Update
	err := b.call(ctx, "getUpdates", map[string]any{"offset": offset, "timeout": int(b.pollTime.Seconds())}, &out)
	return out, err
}

func (b *Bot) call(ctx context.Context, method string, params any, out any) error {
	body, err := json.Marshal(params)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.serverURL+"/bot"+b.token+"/"+method, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var envelope struct {
		OK          bool            `json:"ok"`
		Result      json.RawMessage `json:"result"`
		Description string          `json:"description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return err
	}
	if !envelope.OK {
		return fmt.Errorf("bot api %s failed: %s", method, envelope.Description)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(envelope.Result, out)
}
