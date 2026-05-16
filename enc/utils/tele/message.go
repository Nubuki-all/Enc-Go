// Package tele contains telegram helpers
package tele

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	botmodels "enc/internal/clients/telegram/botapi/models"
	"github.com/celestix/gotgproto/ext"
	gp "github.com/celestix/gotgproto/types"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"

	"enc/config"
	"enc/internal/clients/telegram"
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
	BotAPI    *botmodels.Message
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

// NewGotgMessageFromBotAPI converts a Bot API message into the gotg-like
// message shape handlers still use during the transport migration.
func NewGotgMessageFromBotAPI(m *botmodels.Message) *gp.Message {
	if m == nil {
		return nil
	}
	msg := &tg.Message{
		ID:      m.ID,
		Message: m.Text,
	}
	if m.ReplyToMessage != nil {
		msg.ReplyTo = &tg.MessageReplyHeader{ReplyToMsgID: m.ReplyToMessage.ID}
	}
	if m.MessageThreadID != 0 {
		msg.ReplyTo = &tg.MessageReplyHeader{ReplyToTopID: m.MessageThreadID, ForumTopic: true}
	}
	return &gp.Message{Message: msg, Text: m.Text}
}

// NewMsgFromBotAPI constructs a handler message from a Bot API message.
func NewMsgFromBotAPI(m *botmodels.Message) *Message {
	if m == nil {
		return nil
	}
	senderID := int64(0)
	if m.From != nil {
		senderID = m.From.ID
	}
	replyTo := 0
	if m.ReplyToMessage != nil {
		replyTo = m.ReplyToMessage.ID
	}
	return &Message{
		Raw:       NewGotgMessageFromBotAPI(m),
		ChatID:    m.Chat.ID,
		SenderID:  senderID,
		ID:        m.ID,
		ReplyToID: replyTo,
		TopicID:   m.MessageThreadID,
		BotAPI:    m,
	}
}

// NewMsgFromGotg constructs a wrapper from a concrete *gp.Message.
func NewMsgFromGotg(m *gp.Message, c any, chatID int64, senderID int64) *Message {
	ctx := unwrapGotgContext(c)
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
		Ctx:       ctx,
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
	if m.BotAPI != nil || telegram.Context == nil {
		msg, err := telegram.SendBotAPIMessage(context.Background(), m.ChatID, text)
		if err != nil {
			logger.ErrorW(err.Error())
			return nil, err
		}
		return NewMsgFromBotAPI(msg), nil
	}
	pText, entities, _ := parser.Parse(text, true)
	topicID := m.TopicID
	if topicID == m.ID || topicID == 1 {
		topicID = 0
	}
	inputRep := &tg.InputReplyToMessage{
		ReplyToMsgID: m.ID,
		TopMsgID:     topicID,
	}
	msg, err := telegram.Context.SendMessage(
		m.ChatID, &tg.MessagesSendMessageRequest{
			Message:  pText,
			Entities: entities,
			ReplyTo:  inputRep,
		},
	)
	if err != nil {
		logger.ErrorW(err.Error())
		return nil, err
	}
	return NewMsgFromGotg(msg, telegram.Context, m.ChatID, telegram.Context.Self.ID), nil
}

func (m *Message) Reply(text string, replyOptions *ReplyOptions) (*Message, error) {
	if m.BotAPI != nil || telegram.Context == nil {
		msg, err := telegram.SendBotAPIMessage(context.Background(), m.ChatID, text)
		if err != nil {
			logger.ErrorW(err.Error())
			return nil, err
		}
		return NewMsgFromBotAPI(msg), nil
	}
	var pText string
	var entities []tg.MessageEntityClass
	if replyOptions.HTMLParseMode {
		pText, entities, _ = htmlparser.Parse(text)
	} else {
		pText, entities, _ = parser.Parse(text, true)
	}
	topicID := m.TopicID
	if topicID == m.ID || topicID == 1 {
		topicID = 0
	}
	inputRep := &tg.InputReplyToMessage{
		ReplyToMsgID: m.ID,
		TopMsgID:     topicID,
	}
	msg, err := telegram.Context.SendMessage(
		m.ChatID, &tg.MessagesSendMessageRequest{
			Message:     pText,
			Entities:    entities,
			ReplyTo:     inputRep,
			ReplyMarkup: replyOptions.ReplyMarkup,
			NoWebpage:   replyOptions.DisableWebPreview,
		},
	)
	if err != nil {
		logger.ErrorW(err.Error())
		return nil, err
	}
	if replyOptions.SleepDelete != 0 {
		go func(msg *Message, seconds time.Duration) {
			time.Sleep(seconds)
			_ = msg.Delete()
		}(
			NewMsgFromGotg(msg, telegram.Context, m.ChatID, telegram.Context.Self.ID),
			time.Duration(replyOptions.SleepDelete)*time.Second,
		)
	}
	return NewMsgFromGotg(msg, telegram.Context, m.ChatID, telegram.Context.Self.ID), nil
}

func (m *Message) RespondText(text string) (*Message, error) {
	if m.BotAPI != nil || telegram.Context == nil {
		msg, err := telegram.SendBotAPIMessage(context.Background(), m.ChatID, text)
		if err != nil {
			logger.ErrorW(err.Error())
			return nil, err
		}
		return NewMsgFromBotAPI(msg), nil
	}
	pText, entities, _ := parser.Parse(text, true)
	topicID := m.TopicID
	if topicID == 1 {
		topicID = 0
	}
	inputRep := &tg.InputReplyToMessage{
		ReplyToMsgID: topicID,
	}
	msg, err := telegram.Context.SendMessage(
		m.ChatID, &tg.MessagesSendMessageRequest{
			Message:  pText,
			Entities: entities,
			ReplyTo:  inputRep,
		},
	)
	if err != nil {
		logger.ErrorW(err.Error())
		return nil, err
	}
	return NewMsgFromGotg(msg, telegram.Context, m.ChatID, telegram.Context.Self.ID), nil
}

func (m *Message) Respond(text string, opt *SendOptions) (*Message, error) {
	if m.BotAPI != nil || telegram.Context == nil {
		msg, err := telegram.SendBotAPIMessage(context.Background(), m.ChatID, text)
		if err != nil {
			logger.ErrorW(err.Error())
			return nil, err
		}
		return NewMsgFromBotAPI(msg), nil
	}
	var pText string
	var entities []tg.MessageEntityClass
	if opt == nil {
		opt = &SendOptions{}
	}
	if opt.HTMLParseMode {
		pText, entities, _ = htmlparser.Parse(text)
	} else {
		pText, entities, _ = parser.Parse(text, true)
	}
	topicID := m.TopicID
	if topicID == 1 {
		topicID = 0
	}
	inputRep := &tg.InputReplyToMessage{
		ReplyToMsgID: topicID,
	}
	msg, err := telegram.Context.SendMessage(
		m.ChatID, &tg.MessagesSendMessageRequest{
			Message:     pText,
			Entities:    entities,
			ReplyTo:     inputRep,
			ReplyMarkup: opt.ReplyMarkup,
		},
	)
	if err != nil {
		logger.ErrorW(err.Error())
		return nil, err
	}
	return NewMsgFromGotg(msg, telegram.Context, m.ChatID, telegram.Context.Self.ID), nil
}

func (m *Message) EditText(text string) (*Message, error) {
	if m.BotAPI != nil || telegram.Context == nil {
		msg, err := telegram.EditBotAPIMessage(context.Background(), m.ChatID, m.ID, text)
		if err != nil {
			logger.ErrorW(err.Error())
			return nil, err
		}
		return NewMsgFromBotAPI(msg), nil
	}
	pText, entities, _ := parser.Parse(text, true)
	var replyMarkup tg.ReplyMarkupClass
	if m.Raw != nil {
		replyMarkup = m.Raw.ReplyMarkup
	}
	msg, err := telegram.Context.EditMessage(
		m.ChatID, &tg.MessagesEditMessageRequest{
			ID:          m.ID,
			Message:     pText,
			Entities:    entities,
			ReplyMarkup: replyMarkup,
		},
	)
	if err != nil {
		logger.ErrorW(err.Error())
		return nil, err
	}
	if m.Raw == nil {
		return NewMsgFromGotg(msg, telegram.Context, m.ChatID, telegram.Context.Self.ID), nil
	}
	return NewMsgFromGotg(msg, telegram.Context, m.ChatID, m.SenderID), nil
}

func (m *Message) Edit(text string, editOptions *EditOptions) (*Message, error) {
	if m.BotAPI != nil || telegram.Context == nil {
		msg, err := telegram.EditBotAPIMessage(context.Background(), m.ChatID, m.ID, text)
		if err != nil {
			logger.ErrorW(err.Error())
			return nil, err
		}
		return NewMsgFromBotAPI(msg), nil
	}
	var pText string
	var entities []tg.MessageEntityClass
	if editOptions == nil {
		editOptions = &EditOptions{}
	}
	if len(editOptions.Entities) != 0 {
		pText, entities = text, editOptions.Entities
	} else if editOptions.HTMLParseMode {
		pText, entities, _ = htmlparser.Parse(text)
	} else {
		pText, entities, _ = parser.Parse(text, true)
	}
	msg, err := telegram.Context.EditMessage(
		m.ChatID, &tg.MessagesEditMessageRequest{
			ID:          m.ID,
			Message:     pText,
			Entities:    entities,
			ReplyMarkup: editOptions.ReplyMarkup,
			Media:       editOptions.Media,
		},
	)
	if err != nil {
		logger.ErrorW(err.Error())
		return nil, err
	}
	if m.Raw == nil {
		return NewMsgFromGotg(msg, telegram.Context, m.ChatID, telegram.Context.Self.ID), nil
	}
	return NewMsgFromGotg(msg, telegram.Context, m.ChatID, m.SenderID), nil
}

func (m *Message) RawText() (string, error) {
	if m.Raw == nil {
		return "", nil
	}
	return parser.Unparse(m.Raw.Text, m.Raw.Entities)
}

func (m *Message) Delete() error {
	if m.BotAPI != nil || telegram.Context == nil {
		return telegram.DeleteBotAPIMessage(context.Background(), m.ChatID, m.ID)
	}
	return telegram.Context.DeleteMessages(m.ChatID, []int{m.ID})
}

func (m *Message) Forward(chatID int64) (*Message, error) {
	resp, err := telegram.Context.ForwardMessages(
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
	return NewMsgFromGotg(msg, telegram.Context, chatID, telegram.Context.Self.ID), nil
}

func (m *Message) Pin(silent bool) error {
	ctx := telegram.Context
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
	ctx := telegram.Context
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
	ctx := telegram.Context
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
		return NewMsgFromGotg(msg, telegram.Context, m.ChatID, telegram.Context.Self.ID), nil
	}
	return NewMsgFromGotg(msg, telegram.Context, m.ChatID, m.SenderID), nil
}

func (m *Message) ReplyMediaDoc(media tg.InputMediaClass, uploadOptions *UploadOptions) (*Message, error) {
	ctx := telegram.Context
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
		return NewMsgFromGotg(msg, telegram.Context, m.ChatID, telegram.Context.Self.ID), nil
	}
	return NewMsgFromGotg(msg, telegram.Context, m.ChatID, m.SenderID), nil
}

func SendMessage(id int64, text string) error {
	if config.Conf.TgDispatcher.UsesBotAPI() && telegram.BotAPI != nil {
		_, err := telegram.SendBotAPIMessage(context.Background(), id, text)
		return err
	}
	if telegram.Context == nil {
		return fmt.Errorf("no telegram messaging client is initialized")
	}
	pText, entities, _ := parser.Parse(text, true)
	_, err := telegram.Context.SendMessage(
		id, &tg.MessagesSendMessageRequest{
			Message:  pText,
			Entities: entities,
		},
	)
	return err
}

func (m *Message) Action(action string, prog int) error {
	ctx := telegram.Context
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
	msg, err := telegram.Context.SendMedia(
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
		msg, telegram.Context, id, telegram.Client.Self.ID,
	), nil
}

func SendMsg(id int64, text string, opts *SendOptions) (*Message, error) {
	if config.Conf.TgDispatcher.UsesBotAPI() && telegram.BotAPI != nil {
		msg, err := telegram.SendBotAPIMessage(context.Background(), id, text)
		if err != nil {
			return nil, err
		}
		return NewMsgFromBotAPI(msg), nil
	}
	if telegram.Context == nil {
		return nil, fmt.Errorf("no telegram messaging client is initialized")
	}
	var pText string
	var entities []tg.MessageEntityClass
	inputRep := &tg.InputReplyToMessage{}
	if opts == nil {
		opts = &SendOptions{}
	}
	if opts.HTMLParseMode {
		pText, entities, _ = htmlparser.Parse(text)
	} else {
		pText, entities, _ = parser.Parse(text, true)
	}
	if opts.ReplyTo != 0 {
		inputRep.ReplyToMsgID = opts.ReplyTo
	}
	msg, err := telegram.Context.SendMessage(
		id, &tg.MessagesSendMessageRequest{
			Message:     pText,
			Entities:    entities,
			ReplyMarkup: opts.ReplyMarkup,
			ReplyTo:     inputRep,
		},
	)
	if err != nil {
		return nil, err
	}
	return NewMsgFromGotg(
		msg, telegram.Context, id, telegram.Client.Self.ID,
	), nil
}

func EditMessage(id int64, msgID int, text string) error {
	if config.Conf.TgDispatcher.UsesBotAPI() && telegram.BotAPI != nil {
		_, err := telegram.EditBotAPIMessage(context.Background(), id, msgID, text)
		return err
	}
	if telegram.Context == nil {
		return fmt.Errorf("no telegram messaging client is initialized")
	}
	pText, entities, _ := parser.Parse(text, true)
	_, err := telegram.Context.EditMessage(
		id, &tg.MessagesEditMessageRequest{
			ID:       msgID,
			Message:  pText,
			Entities: entities,
		},
	)
	return err
}

func ForwardMessage(fromChatID, toChatID int64, msgID int) (*Message, error) {
	ctx := telegram.Context
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
