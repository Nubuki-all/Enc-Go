// Package tele contains telegram helpers
package tele

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/celestix/gotgproto/ext"
	gp "github.com/celestix/gotgproto/types"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"

	// "enc/config"
	tgclient "enc/internal/clients/telegram"
	"enc/internal/logger"
	"enc/utils/tele/parser"
	"enc/utils/tele/parser/htmlparser"
)

type Message struct {
	ChatID    int64
	Ctx       *ext.Context
	ID        int
	Raw       *gp.Message
	SenderID  int64
	Chat      *tg.Chat
	User      *tg.User
	ReplyToID int
	TopicID   int
}

type EditOptions struct {
	HTMLParseMode bool
	ReplyMarkup   tg.ReplyMarkupClass
	Media         tg.InputMediaClass
	Entities      []tg.MessageEntityClass
	// will add more fields as required
}

type ReplyOptions struct {
	HTMLParseMode     bool
	SleepDelete       int
	ReplyMarkup       tg.ReplyMarkupClass
	DisableWebPreview bool
	// will add more fields as required
}
type SendOptions struct {
	HTMLParseMode bool
	ReplyMarkup   tg.ReplyMarkupClass
	Caption       string
	ReplyTo       int
	// will add more fields as required
}

type UploadOptions struct {
	Name          string
	Caption       string
	Path          string
	Bytes         []byte
	Edit          bool
	Quote         bool
	HTMLParseMode bool
	ReplyMarkup   tg.ReplyMarkupClass
	// will add more fields as required
}

// NewMsgFromGotg constructs a wrapper from a concrete *gp.Message.
func NewMsgFromGotg(m *gp.Message, c *ext.Context, chatID int64, senderID int64) *Message {
	var replyTo, topicID int
	if msgHeader, ok := m.ReplyTo.(*tg.MessageReplyHeader); ok {
		replyTo = msgHeader.ReplyToMsgID
		if msgHeader.ForumTopic {
			if topicID = msgHeader.ReplyToTopID; topicID == 0 {
				topicID = replyTo
				replyTo = 0
			}
		}

	}
	return &Message{
		Raw:       m,
		Ctx:       c,
		ChatID:    chatID,
		SenderID:  senderID,
		ID:        m.ID,
		ReplyToID: replyTo,
		TopicID:   topicID,
	}
}

// RawMessage returns the underlying *gp.Message.
func (m *Message) RawMessage() *gp.Message {
	return m.Raw
}

func (m *Message) ReplyText(text string) (*Message, error) {
	pText, _, _ := parser.Parse(text, false)
	msg := tgbotapi.NewMessage(m.ChatID, pText)
	msg.ReplyToMessageID = m.ID
	msg.ParseMode = "HTML"
	res, err := tgclient.BotAPI.Send(msg)
	if err != nil {
		logger.ErrorW(err.Error())
		return nil, err
	}
	return &Message{
		ChatID:   res.Chat.ID,
		ID:       res.MessageID,
		SenderID: res.From.ID,
		Ctx:      tgclient.Context,
		Raw: &gp.Message{
			Message: &tg.Message{
				ID:      res.MessageID,
				Message: res.Text,
				PeerID:  &tg.PeerUser{UserID: res.From.ID},
			},
			Text: res.Text,
		},
	}, nil
}

func (m *Message) Reply(text string, replyOptions *ReplyOptions) (*Message, error) {
	var pText string
	var parseMode string
	if replyOptions.HTMLParseMode {
		pText, _, _ = htmlparser.Parse(text)
		parseMode = "HTML"
	} else {
		pText, _, _ = parser.Parse(text, false)
		parseMode = "HTML"
	}

	msg := tgbotapi.NewMessage(m.ChatID, pText)
	msg.ReplyToMessageID = m.ID
	msg.ParseMode = parseMode
	msg.DisableWebPagePreview = replyOptions.DisableWebPreview
	msg.ReplyMarkup = ConvertReplyMarkup(replyOptions.ReplyMarkup)

	res, err := tgclient.BotAPI.Send(msg)
	if err != nil {
		logger.ErrorW(err.Error())
		return nil, err
	}

	retMsg := &Message{
		ChatID:   res.Chat.ID,
		ID:       res.MessageID,
		SenderID: res.From.ID,
		Ctx:      tgclient.Context,
		Raw: &gp.Message{
			Message: &tg.Message{
				ID:      res.MessageID,
				Message: res.Text,
				PeerID:  &tg.PeerUser{UserID: res.From.ID},
			},
			Text: res.Text,
		},
	}
	if replyOptions.SleepDelete != 0 {
		go func(msg *Message, seconds time.Duration) {
			time.Sleep(seconds)
			_ = msg.Delete()
		}(
			retMsg,
			time.Duration(replyOptions.SleepDelete)*time.Second,
		)
	}
	return retMsg, nil
}

func (m *Message) RespondText(text string) (*Message, error) {
	pText, _, _ := parser.Parse(text, false)
	msg := tgbotapi.NewMessage(m.ChatID, pText)
	msg.ParseMode = "HTML"
	res, err := tgclient.BotAPI.Send(msg)
	if err != nil {
		logger.ErrorW(err.Error())
		return nil, err
	}
	return &Message{
		ChatID:   res.Chat.ID,
		ID:       res.MessageID,
		SenderID: res.From.ID,
		Ctx:      tgclient.Context,
		Raw: &gp.Message{
			Message: &tg.Message{
				ID:      res.MessageID,
				Message: res.Text,
				PeerID:  &tg.PeerUser{UserID: res.From.ID},
			},
			Text: res.Text,
		},
	}, nil
}

func (m *Message) Respond(text string, opt *SendOptions) (*Message, error) {
	var pText string
	var parseMode string
	if opt == nil {
		opt = &SendOptions{}
	}
	if opt.HTMLParseMode {
		pText, _, _ = htmlparser.Parse(text)
		parseMode = "HTML"
	} else {
		pText, _, _ = parser.Parse(text, false)
		parseMode = "HTML"
	}

	msg := tgbotapi.NewMessage(m.ChatID, pText)
	msg.ParseMode = parseMode
	if opt.ReplyTo != 0 {
		msg.ReplyToMessageID = opt.ReplyTo
	}
	msg.ReplyMarkup = ConvertReplyMarkup(opt.ReplyMarkup)

	res, err := tgclient.BotAPI.Send(msg)
	if err != nil {
		logger.ErrorW(err.Error())
		return nil, err
	}
	return &Message{
		ChatID:   res.Chat.ID,
		ID:       res.MessageID,
		SenderID: res.From.ID,
		Ctx:      tgclient.Context,
		Raw: &gp.Message{
			Message: &tg.Message{
				ID:      res.MessageID,
				Message: res.Text,
				PeerID:  &tg.PeerUser{UserID: res.From.ID},
			},
			Text: res.Text,
		},
	}, nil
}

func (m *Message) EditText(text string) (*Message, error) {
	pText, _, _ := parser.Parse(text, false)
	msg := tgbotapi.NewEditMessageText(m.ChatID, m.ID, pText)
	msg.ParseMode = "HTML"
	res, err := tgclient.BotAPI.Send(msg)
	if err != nil {
		logger.ErrorW(err.Error())
		return nil, err
	}
	r := res
	return &Message{
		ChatID:   r.Chat.ID,
		ID:       r.MessageID,
		SenderID: r.From.ID,
		Ctx:      tgclient.Context,
		Raw: &gp.Message{
			Message: &tg.Message{
				ID:      r.MessageID,
				Message: r.Text,
				PeerID:  &tg.PeerUser{UserID: r.From.ID},
			},
			Text: r.Text,
		},
	}, nil
}

func (m *Message) Edit(text string, editOptions *EditOptions) (*Message, error) {
	var pText string
	var parseMode string
	if editOptions == nil {
		editOptions = &EditOptions{}
	}
	if editOptions.HTMLParseMode {
		pText, _, _ = htmlparser.Parse(text)
		parseMode = "HTML"
	} else {
		pText, _, _ = parser.Parse(text, false)
		parseMode = "HTML"
	}
	msg := tgbotapi.NewEditMessageText(m.ChatID, m.ID, pText)
	msg.ParseMode = parseMode
	markup := ConvertReplyMarkup(editOptions.ReplyMarkup)
	if markup != nil {
		if im, ok := markup.(tgbotapi.InlineKeyboardMarkup); ok {
			msg.ReplyMarkup = &im
		}
	}

	res, err := tgclient.BotAPI.Send(msg)
	if err != nil {
		logger.ErrorW(err.Error())
		return nil, err
	}
	r := res
	return &Message{
		ChatID:   r.Chat.ID,
		ID:       r.MessageID,
		SenderID: r.From.ID,
		Ctx:      tgclient.Context,
		Raw: &gp.Message{
			Message: &tg.Message{
				ID:      r.MessageID,
				Message: r.Text,
				PeerID:  &tg.PeerUser{UserID: r.From.ID},
			},
			Text: r.Text,
		},
	}, nil
}

func (m *Message) RawText() (string, error) {
	if m.Raw == nil {
		return "", nil
	}
	return parser.Unparse(m.Raw.Text, m.Raw.Entities)
}

func (m *Message) Delete() error {
	msg := tgbotapi.NewDeleteMessage(m.ChatID, m.ID)
	_, err := tgclient.BotAPI.Request(msg)
	return err
}

func (m *Message) Forward(chatID int64) (*Message, error) {
	resp, err := tgclient.Context.ForwardMessages(
		m.ChatID, chatID,
		&tg.MessagesForwardMessagesRequest{
			DropAuthor: true,
			ID:         []int{m.ID},
		},
	)
	if err != nil {
		logger.ErrorW(err.Error())
		return nil, err
	}
	var msg *gp.Message
	var updates []tg.UpdateClass
	switch r := resp.(type) {
	case *tg.Updates:
		updates = r.GetUpdates()
	}
	update := updates[0]
	var rawMsg tg.MessageClass
	switch update := update.(type) {
	case *tg.UpdateNewMessage:
		rawMsg = update.GetMessage()
	case *tg.UpdateNewChannelMessage:
		rawMsg = update.GetMessage()
	}
	if rawMsg != nil {
		msg = gp.ConstructMessage(rawMsg)
	} else {
		return nil, fmt.Errorf("failed to extract message from updates")
	}
	return NewMsgFromGotg(msg, tgclient.Context, chatID, tgclient.Context.Self.ID), nil
}

func (m *Message) Pin(silent bool) error {
	ctx := tgclient.Context
	peerID, err := ctx.ResolveInputPeerById(m.ChatID)
	if err != nil {
		return err
	}
	req := &tg.MessagesUpdatePinnedMessageRequest{
		Peer:   peerID,
		ID:     m.ID,
		Silent: silent,
	}
	_, err = ctx.Raw.MessagesUpdatePinnedMessage(ctx, req)
	return err
}

func (m *Message) Unpin(silent bool) error {
	ctx := tgclient.Context
	peerID, err := ctx.ResolveInputPeerById(m.ChatID)
	if err != nil {
		return err
	}
	req := &tg.MessagesUpdatePinnedMessageRequest{
		Peer:   peerID,
		ID:     m.ID,
		Silent: silent,
		Unpin:  true,
	}
	_, err = ctx.Raw.MessagesUpdatePinnedMessage(ctx, req)
	return err
}

func (m *Message) UploadDoc(uploadOptions *UploadOptions) (*Message, error) {
	var file tg.InputFileClass
	inputRep := &tg.InputReplyToMessage{}
	var err error
	ctx := tgclient.Context
	bytes := uploadOptions.Bytes
	name := uploadOptions.Name
	path := uploadOptions.Path
	uClient := uploader.NewUploader(ctx.Raw)
	if path != "" {
		name = filepath.Base(path)
		file, err = uClient.FromPath(ctx, path)
	} else if bytes != nil {
		if name == "" {
			name = "Unknown"
		}
		file, err = uClient.FromBytes(ctx, name, bytes)
	} else {
		err = fmt.Errorf("no content was specified to upload")
	}
	if err != nil {
		logger.Error(err)
		return nil, err
	}
	media := &tg.InputMediaUploadedDocument{
		File: file,
		// set attributes so file has a name
		Attributes: []tg.DocumentAttributeClass{
			&tg.DocumentAttributeFilename{FileName: name},
		},
	}
	if uploadOptions.Edit {
		return m.Edit(uploadOptions.Caption, &EditOptions{
			Media:         media,
			ReplyMarkup:   uploadOptions.ReplyMarkup,
			HTMLParseMode: uploadOptions.HTMLParseMode,
		})
	}
	if uploadOptions.Quote {
		inputRep.ReplyToMsgID = m.ID
	}
	var pCaption string
	var entities []tg.MessageEntityClass
	if caption := uploadOptions.Caption; caption != "" {
		if uploadOptions.HTMLParseMode {
			pCaption, entities, _ = htmlparser.Parse(caption)
		} else {
			pCaption, entities, _ = parser.Parse(caption, true)
		}
	}

	msg, err := ctx.SendMedia(
		m.ChatID,
		&tg.MessagesSendMediaRequest{
			Media:       media,
			Message:     pCaption,
			Entities:    entities,
			ReplyTo:     inputRep,
			ReplyMarkup: uploadOptions.ReplyMarkup,
		},
	)
	if err != nil {
		logger.ErrorW(err.Error())
		return nil, err
	}
	if m.Raw == nil {
		return NewMsgFromGotg(msg, tgclient.Context, m.ChatID, tgclient.Context.Self.ID), nil
	}
	return NewMsgFromGotg(msg, tgclient.Context, m.ChatID, m.SenderID), nil
}

func (m *Message) ReplyMediaDoc(media tg.InputMediaClass, uploadOptions *UploadOptions) (*Message, error) {
	ctx := tgclient.Context
	inputRep := &tg.InputReplyToMessage{}
	if uploadOptions.Quote {
		inputRep.ReplyToMsgID = m.ID
	}
	var pCaption string
	var entities []tg.MessageEntityClass
	if caption := uploadOptions.Caption; caption != "" {
		if uploadOptions.HTMLParseMode {
			pCaption, entities, _ = htmlparser.Parse(caption)
		} else {
			pCaption, entities, _ = parser.Parse(caption, true)
		}
	}
	msg, err := ctx.SendMedia(
		m.ChatID,
		&tg.MessagesSendMediaRequest{
			Media:       media,
			Message:     pCaption,
			Entities:    entities,
			ReplyTo:     inputRep,
			ReplyMarkup: uploadOptions.ReplyMarkup,
		},
	)
	if err != nil {
		logger.ErrorW(err.Error())
		return nil, err
	}
	if m.Raw == nil {
		return NewMsgFromGotg(msg, tgclient.Context, m.ChatID, tgclient.Context.Self.ID), nil
	}
	return NewMsgFromGotg(msg, tgclient.Context, m.ChatID, m.SenderID), nil
}

func SendMessage(id int64, text string) error {
	pText, _, _ := parser.Parse(text, false)
	msg := tgbotapi.NewMessage(id, pText)
	msg.ParseMode = "HTML"
	_, err := tgclient.BotAPI.Send(msg)
	return err
}

func (m *Message) Action(action string, prog int) error {
	ctx := tgclient.Context
	inputPeer, err := ctx.ResolveInputPeerById(m.ChatID)
	if err != nil {
		return err
	}
	msgAction := ctx.Sender.To(inputPeer).TypingAction()
	if m.TopicID != 0 {
		msgAction.ThreadID(m.TopicID)
	}
	switch action {
	case "game":
		return msgAction.GamePlay(ctx)
	case "typing":
		return msgAction.Typing(ctx)
	case "upload":
		return msgAction.UploadDocument(ctx, prog)
	case "upload-photo":
		return msgAction.UploadPhoto(ctx, prog)
	case "upload-vid":
		return msgAction.UploadVideo(ctx, prog)
	default:
		return msgAction.Cancel(ctx)
	}
}

func SendMedia(id int64, media tg.InputMediaClass, opts *SendOptions) (*Message, error) {
	var pText string
	var entities []tg.MessageEntityClass
	inputRep := &tg.InputReplyToMessage{}
	if opts == nil {
		opts = &SendOptions{}
	}
	if text := opts.Caption; text != "" {
		if opts.HTMLParseMode {
			pText, entities, _ = htmlparser.Parse(text)
		} else {
			pText, entities, _ = parser.Parse(text, true)
		}
	}
	if opts.ReplyTo != 0 {
		inputRep.ReplyToMsgID = opts.ReplyTo
	}
	msg, err := tgclient.Context.SendMedia(
		id, &tg.MessagesSendMediaRequest{
			Message:     pText,
			Entities:    entities,
			ReplyMarkup: opts.ReplyMarkup,
			ReplyTo:     inputRep,
			Media:       media,
		},
	)
	if err != nil {
		return nil, err
	}
	return NewMsgFromGotg(
		msg, tgclient.Context, id, tgclient.Client.Self.ID,
	), nil
}

func SendMsg(id int64, text string, opts *SendOptions) (*Message, error) {
	var pText string
	var parseMode string
	if opts == nil {
		opts = &SendOptions{}
	}
	if opts.HTMLParseMode {
		pText, _, _ = htmlparser.Parse(text)
		parseMode = "HTML"
	} else {
		pText, _, _ = parser.Parse(text, false)
		parseMode = "HTML"
	}
	msg := tgbotapi.NewMessage(id, pText)
	msg.ParseMode = parseMode
	if opts.ReplyTo != 0 {
		msg.ReplyToMessageID = opts.ReplyTo
	}
	msg.ReplyMarkup = ConvertReplyMarkup(opts.ReplyMarkup)
	res, err := tgclient.BotAPI.Send(msg)
	if err != nil {
		return nil, err
	}
	return &Message{
		ChatID:   res.Chat.ID,
		ID:       res.MessageID,
		SenderID: res.From.ID,
		Ctx:      tgclient.Context,
		Raw: &gp.Message{
			Message: &tg.Message{
				ID:      res.MessageID,
				Message: res.Text,
				PeerID:  &tg.PeerUser{UserID: res.From.ID},
			},
			Text: res.Text,
		},
	}, nil
}

func EditMessage(id int64, msgID int, text string) error {
	pText, _, _ := parser.Parse(text, false)
	msg := tgbotapi.NewEditMessageText(id, msgID, pText)
	msg.ParseMode = "HTML"
	_, err := tgclient.BotAPI.Send(msg)
	return err
}

func ForwardMessage(fromChatID, toChatID int64, msgID int) (*Message, error) {
	ctx := tgclient.Context
	resp, err := ctx.ForwardMessages(
		fromChatID, toChatID,
		&tg.MessagesForwardMessagesRequest{
			DropAuthor: true,
			ID:         []int{msgID},
		},
	)
	if err != nil {
		logger.ErrorW(err.Error())
		return nil, err
	}
	var msg *gp.Message
	var updates []tg.UpdateClass
	switch r := resp.(type) {
	case *tg.Updates:
		updates = r.GetUpdates()
	}
	update := updates[0]
	switch update := update.(type) {
	case *tg.UpdateNewMessage:
		m := update.GetMessage()
		msg = gp.ConstructMessage(m)
	}
	return NewMsgFromGotg(msg, ctx, toChatID, ctx.Self.ID), nil
}
