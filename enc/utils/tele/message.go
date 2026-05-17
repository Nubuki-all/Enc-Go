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
	"enc/config"
	"enc/internal/clients/telegram"
	"enc/utils/tele/parser"
	"enc/utils/tele/parser/htmlparser"
)

type Message struct {
	ChatID    int64
	ID        int
	SenderID  int64
	ReplyToID int
	TopicID   int
	Text      string
	Ctx       *ext.Context
	Raw       *gp.Message
	BotMsg    *tgbotapi.Message
	ReplyToMessage *Message
	SenderName     string
	IsPrivate      bool
}

type EditOptions struct {
	HTMLParseMode bool
	ReplyMarkup   tg.ReplyMarkupClass
	Media         tg.InputMediaClass
	Entities      []tg.MessageEntityClass
}

type ReplyOptions struct {
	HTMLParseMode     bool
	SleepDelete       int
	ReplyMarkup       tg.ReplyMarkupClass
	DisableWebPreview bool
}

type SendOptions struct {
	HTMLParseMode bool
	ReplyMarkup   tg.ReplyMarkupClass
	Caption       string
	ReplyTo       int
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
}

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
	senderName := ""
	if c != nil && c.Entities != nil {
		if user, ok := c.Entities.Users[senderID]; ok {
			senderName = user.FirstName
		}
	}
	return &Message{
		Raw: m, Ctx: c, ChatID: chatID, SenderID: senderID, ID: m.ID,
		ReplyToID: replyTo, TopicID: topicID, Text: m.Text, SenderName: senderName, IsPrivate: chatID == senderID,
	}
}

func NewMsgFromBotAPI(m *tgbotapi.Message) *Message {
	replyToID := 0
	if m.ReplyToMessage != nil { replyToID = m.ReplyToMessage.MessageID }
	return &Message{
		BotMsg: m, ChatID: m.Chat.ID, SenderID: m.From.ID, ID: m.MessageID,
		ReplyToID: replyToID, Text: m.Text, SenderName: m.From.FirstName, IsPrivate: m.Chat.Type == "private",
	}
}

func (m *Message) SetRepliedToMessage() error {
	if m.ReplyToMessage != nil { return nil }
	rep, err := GetRepliedToMessage(m)
	if err != nil { return err }
	m.ReplyToMessage = rep
	return nil
}

func (m *Message) ReplyText(text string) (*Message, error) { return m.Reply(text, &ReplyOptions{}) }

func (m *Message) Reply(text string, opts *ReplyOptions) (*Message, error) {
	if config.Conf.UseBotAPI && telegram.BotAPI != nil {
		msg := tgbotapi.NewMessage(m.ChatID, text)
		msg.ReplyToMessageID = m.ID
		if opts.HTMLParseMode { msg.ParseMode = "HTML" }
		if opts.ReplyMarkup != nil { msg.ReplyMarkup = ToBotMarkup(opts.ReplyMarkup) }
		msg.DisableWebPagePreview = opts.DisableWebPreview
		sent, err := telegram.BotAPI.Send(msg)
		if err != nil { return nil, err }
		res := NewMsgFromBotAPI(&sent)
		if opts.SleepDelete != 0 {
			go func() { time.Sleep(time.Duration(opts.SleepDelete) * time.Second); _ = res.Delete() }()
		}
		return res, nil
	}
	var pText string
	var entities []tg.MessageEntityClass
	if opts.HTMLParseMode { pText, entities, _ = htmlparser.Parse(text) } else { pText, entities, _ = parser.Parse(text, true) }
	inputRep := &tg.InputReplyToMessage{ReplyToMsgID: m.ID, TopMsgID: m.TopicID}
	msg, err := telegram.Context.SendMessage(m.ChatID, &tg.MessagesSendMessageRequest{
		Message: pText, Entities: entities, ReplyTo: inputRep, ReplyMarkup: opts.ReplyMarkup, NoWebpage: opts.DisableWebPreview,
	})
	if err != nil { return nil, err }
	res := NewMsgFromGotg(msg, telegram.Context, m.ChatID, telegram.Context.Self.ID)
	if opts.SleepDelete != 0 {
		go func() { time.Sleep(time.Duration(opts.SleepDelete) * time.Second); _ = res.Delete() }()
	}
	return res, nil
}

func (m *Message) RespondText(text string) (*Message, error) { return m.Respond(text, &SendOptions{}) }

func (m *Message) Respond(text string, opts *SendOptions) (*Message, error) {
	if config.Conf.UseBotAPI && telegram.BotAPI != nil {
		msg := tgbotapi.NewMessage(m.ChatID, text)
		if opts.HTMLParseMode { msg.ParseMode = "HTML" }
		if opts.ReplyMarkup != nil { msg.ReplyMarkup = ToBotMarkup(opts.ReplyMarkup) }
		sent, err := telegram.BotAPI.Send(msg)
		if err != nil { return nil, err }
		return NewMsgFromBotAPI(&sent), nil
	}
	var pText string
	var entities []tg.MessageEntityClass
	if opts.HTMLParseMode { pText, entities, _ = htmlparser.Parse(text) } else { pText, entities, _ = parser.Parse(text, true) }
	inputRep := &tg.InputReplyToMessage{ReplyToMsgID: m.TopicID}
	msg, err := telegram.Context.SendMessage(m.ChatID, &tg.MessagesSendMessageRequest{
		Message: pText, Entities: entities, ReplyTo: inputRep, ReplyMarkup: opts.ReplyMarkup,
	})
	if err != nil { return nil, err }
	return NewMsgFromGotg(msg, telegram.Context, m.ChatID, telegram.Context.Self.ID), nil
}

func (m *Message) EditText(text string) (*Message, error) { return m.Edit(text, &EditOptions{}) }

func (m *Message) Edit(text string, opts *EditOptions) (*Message, error) {
	if config.Conf.UseBotAPI && telegram.BotAPI != nil && opts.Media == nil {
		msg := tgbotapi.NewEditMessageText(m.ChatID, m.ID, text)
		if opts.HTMLParseMode { msg.ParseMode = "HTML" }
		if opts.ReplyMarkup != nil {
			if im, ok := ToBotMarkup(opts.ReplyMarkup).(tgbotapi.InlineKeyboardMarkup); ok { msg.ReplyMarkup = &im }
		}
		sent, err := telegram.BotAPI.Send(msg)
		if err != nil { return m, nil }
		return NewMsgFromBotAPI(&sent), nil
	}
	var pText string
	var entities []tg.MessageEntityClass
	if len(opts.Entities) != 0 { pText, entities = text, opts.Entities } else if opts.HTMLParseMode { pText, entities, _ = htmlparser.Parse(text) } else { pText, entities, _ = parser.Parse(text, true) }
	markup := opts.ReplyMarkup
	if markup == nil && m.Raw != nil { markup = m.Raw.ReplyMarkup }
	msg, err := telegram.Context.EditMessage(m.ChatID, &tg.MessagesEditMessageRequest{
		ID: m.ID, Message: pText, Entities: entities, ReplyMarkup: markup, Media: opts.Media,
	})
	if err != nil { return nil, err }
	return NewMsgFromGotg(msg, telegram.Context, m.ChatID, m.SenderID), nil
}

func (m *Message) Delete() error {
	if config.Conf.UseBotAPI { _, err := telegram.BotAPI.Request(tgbotapi.NewDeleteMessage(m.ChatID, m.ID)); return err }
	return telegram.Context.DeleteMessages(m.ChatID, []int{m.ID})
}

func (m *Message) Forward(chatID int64) (*Message, error) {
	if config.Conf.UseBotAPI && telegram.BotAPI != nil {
		sent, err := telegram.BotAPI.Send(tgbotapi.NewForward(chatID, m.ChatID, m.ID))
		if err != nil { return nil, err }
		return NewMsgFromBotAPI(&sent), nil
	}
	resp, err := telegram.Context.ForwardMessages(m.ChatID, chatID, &tg.MessagesForwardMessagesRequest{DropAuthor: true, ID: []int{m.ID}})
	if err != nil { return nil, err }
	var updates []tg.UpdateClass
	if u, ok := resp.(*tg.Updates); ok { updates = u.GetUpdates() }
	if len(updates) == 0 { return nil, fmt.Errorf("no updates") }
	var rawMsg tg.MessageClass
	switch u := updates[0].(type) {
	case *tg.UpdateNewMessage: rawMsg = u.GetMessage()
	case *tg.UpdateNewChannelMessage: rawMsg = u.GetMessage()
	}
	if rawMsg == nil { return nil, fmt.Errorf("failed to extract message") }
	return NewMsgFromGotg(gp.ConstructMessage(rawMsg), telegram.Context, chatID, telegram.Context.Self.ID), nil
}

func (m *Message) Pin(silent bool) error {
	if config.Conf.UseBotAPI && telegram.BotAPI != nil {
		_, err := telegram.BotAPI.Send(tgbotapi.PinChatMessageConfig{ChatID: m.ChatID, MessageID: m.ID, DisableNotification: silent})
		return err
	}
	ctx := telegram.Context
	peerID, _ := ctx.ResolveInputPeerById(m.ChatID)
	_, err := ctx.Raw.MessagesUpdatePinnedMessage(ctx, &tg.MessagesUpdatePinnedMessageRequest{Peer: peerID, ID: m.ID, Silent: silent})
	return err
}

func (m *Message) Unpin(silent bool) error {
	if config.Conf.UseBotAPI && telegram.BotAPI != nil {
		_, err := telegram.BotAPI.Send(tgbotapi.UnpinChatMessageConfig{ChatID: m.ChatID, MessageID: m.ID})
		return err
	}
	ctx := telegram.Context
	peerID, _ := ctx.ResolveInputPeerById(m.ChatID)
	_, err := ctx.Raw.MessagesUpdatePinnedMessage(ctx, &tg.MessagesUpdatePinnedMessageRequest{Peer: peerID, ID: m.ID, Silent: silent, Unpin: true})
	return err
}

func (m *Message) Action(action string, prog int) error {
	if config.Conf.UseBotAPI && telegram.BotAPI != nil {
		act := ""
		switch action {
		case "typing": act = tgbotapi.ChatTyping
		case "upload": act = tgbotapi.ChatUploadDocument
		case "upload-photo": act = tgbotapi.ChatUploadPhoto
		case "upload-vid": act = tgbotapi.ChatUploadVideo
		}
		if act != "" { _, err := telegram.BotAPI.Send(tgbotapi.NewChatAction(m.ChatID, act)); return err }
		return nil
	}
	ctx := telegram.Context
	peer, _ := ctx.ResolveInputPeerById(m.ChatID)
	msgAction := ctx.Sender.To(peer).TypingAction()
	if m.TopicID != 0 { msgAction.ThreadID(m.TopicID) }
	switch action {
	case "game": return msgAction.GamePlay(ctx)
	case "typing": return msgAction.Typing(ctx)
	case "upload": return msgAction.UploadDocument(ctx, prog)
	case "upload-photo": return msgAction.UploadPhoto(ctx, prog)
	case "upload-vid": return msgAction.UploadVideo(ctx, prog)
	default: return msgAction.Cancel(ctx)
	}
}

func (m *Message) UploadDoc(opts *UploadOptions) (*Message, error) {
	var file tg.InputFileClass
	var err error
	ctx := telegram.Context
	uClient := uploader.NewUploader(ctx.Raw)
	if opts.Path != "" { file, err = uClient.FromPath(ctx, opts.Path) } else if opts.Bytes != nil {
		name := opts.Name; if name == "" { name = "Unknown" }; file, err = uClient.FromBytes(ctx, name, opts.Bytes)
	} else { return nil, fmt.Errorf("no content") }
	if err != nil { return nil, err }
	media := &tg.InputMediaUploadedDocument{File: file, Attributes: []tg.DocumentAttributeClass{&tg.DocumentAttributeFilename{FileName: filepath.Base(opts.Path)}}}
	if opts.Edit { return m.Edit(opts.Caption, &EditOptions{Media: media, ReplyMarkup: opts.ReplyMarkup, HTMLParseMode: opts.HTMLParseMode}) }
	inputRep := &tg.InputReplyToMessage{}
	if opts.Quote { inputRep.ReplyToMsgID = m.ID }
	var pCap string; var entities []tg.MessageEntityClass
	if opts.Caption != "" { if opts.HTMLParseMode { pCap, entities, _ = htmlparser.Parse(opts.Caption) } else { pCap, entities, _ = parser.Parse(opts.Caption, true) } }
	msg, err := ctx.SendMedia(m.ChatID, &tg.MessagesSendMediaRequest{Media: media, Message: pCap, Entities: entities, ReplyTo: inputRep, ReplyMarkup: opts.ReplyMarkup})
	if err != nil { return nil, err }
	return NewMsgFromGotg(msg, telegram.Context, m.ChatID, telegram.Context.Self.ID), nil
}

func (m *Message) ReplyMediaDoc(media tg.InputMediaClass, opts *UploadOptions) (*Message, error) {
	ctx := telegram.Context
	inputRep := &tg.InputReplyToMessage{}; if opts.Quote { inputRep.ReplyToMsgID = m.ID }
	var pCap string; var entities []tg.MessageEntityClass
	if opts.Caption != "" { if opts.HTMLParseMode { pCap, entities, _ = htmlparser.Parse(opts.Caption) } else { pCap, entities, _ = parser.Parse(opts.Caption, true) } }
	msg, err := ctx.SendMedia(m.ChatID, &tg.MessagesSendMediaRequest{Media: media, Message: pCap, Entities: entities, ReplyTo: inputRep, ReplyMarkup: opts.ReplyMarkup})
	if err != nil { return nil, err }
	return NewMsgFromGotg(msg, telegram.Context, m.ChatID, telegram.Context.Self.ID), nil
}

func (m *Message) RawText() (string, error) {
	if m.BotMsg != nil { return m.BotMsg.Text, nil }
	if m.Raw == nil { return m.Text, nil }
	return parser.Unparse(m.Raw.Text, m.Raw.Entities)
}

func SendMessage(id int64, text string) error {
	if config.Conf.UseBotAPI && telegram.BotAPI != nil { _, err := telegram.BotAPI.Send(tgbotapi.NewMessage(id, text)); return err }
	pT, ent, _ := parser.Parse(text, true)
	_, err := telegram.Context.SendMessage(id, &tg.MessagesSendMessageRequest{Message: pT, Entities: ent})
	return err
}

func SendMsg(id int64, text string, opts *SendOptions) (*Message, error) {
	if config.Conf.UseBotAPI && telegram.BotAPI != nil {
		msg := tgbotapi.NewMessage(id, text); msg.ParseMode = "HTML"
		if opts != nil { if opts.ReplyTo != 0 { msg.ReplyToMessageID = opts.ReplyTo }; msg.ReplyMarkup = ToBotMarkup(opts.ReplyMarkup) }
		sent, err := telegram.BotAPI.Send(msg)
		if err != nil { return nil, err }
		return NewMsgFromBotAPI(&sent), nil
	}
	var pT string; var ent []tg.MessageEntityClass
	if opts != nil && opts.HTMLParseMode { pT, ent, _ = htmlparser.Parse(text) } else { pT, ent, _ = parser.Parse(text, true) }
	inputRep := &tg.InputReplyToMessage{}; if opts != nil && opts.ReplyTo != 0 { inputRep.ReplyToMsgID = opts.ReplyTo }
	markup := tg.ReplyMarkupClass(nil); if opts != nil { markup = opts.ReplyMarkup }
	msg, err := telegram.Context.SendMessage(id, &tg.MessagesSendMessageRequest{Message: pT, Entities: ent, ReplyMarkup: markup, ReplyTo: inputRep})
	if err != nil { return nil, err }
	return NewMsgFromGotg(msg, telegram.Context, id, telegram.Client.Self.ID), nil
}

func EditMessage(id int64, msgID int, text string) error {
	if config.Conf.UseBotAPI && telegram.BotAPI != nil { _, err := telegram.BotAPI.Send(tgbotapi.NewEditMessageText(id, msgID, text)); return err }
	pT, ent, _ := parser.Parse(text, true)
	_, err := telegram.Context.EditMessage(id, &tg.MessagesEditMessageRequest{ID: msgID, Message: pT, Entities: ent})
	return err
}

func ForwardMessage(fID, tID int64, mID int) (*Message, error) {
	if config.Conf.UseBotAPI && telegram.BotAPI != nil { sent, err := telegram.BotAPI.Send(tgbotapi.NewForward(tID, fID, mID)); if err != nil { return nil, err }; return NewMsgFromBotAPI(&sent), nil }
	resp, err := telegram.Context.ForwardMessages(fID, tID, &tg.MessagesForwardMessagesRequest{DropAuthor: true, ID: []int{mID}})
	if err != nil { return nil, err }
	var updates []tg.UpdateClass; if u, ok := resp.(*tg.Updates); ok { updates = u.GetUpdates() }
	if len(updates) == 0 { return nil, fmt.Errorf("no updates") }
	var rawMsg tg.MessageClass; switch u := updates[0].(type) { case *tg.UpdateNewMessage: rawMsg = u.GetMessage(); case *tg.UpdateNewChannelMessage: rawMsg = u.GetMessage() }
	return NewMsgFromGotg(gp.ConstructMessage(rawMsg), telegram.Context, tID, telegram.Context.Self.ID), nil
}

func SendMedia(id int64, media tg.InputMediaClass, opts *SendOptions) (*Message, error) {
	var pT string; var ent []tg.MessageEntityClass
	if opts != nil && opts.HTMLParseMode { pT, ent, _ = htmlparser.Parse(opts.Caption) } else if opts != nil { pT, ent, _ = parser.Parse(opts.Caption, true) }
	inputRep := &tg.InputReplyToMessage{}; if opts != nil && opts.ReplyTo != 0 { inputRep.ReplyToMsgID = opts.ReplyTo }
	markup := tg.ReplyMarkupClass(nil); if opts != nil { markup = opts.ReplyMarkup }
	msg, err := telegram.Context.SendMedia(id, &tg.MessagesSendMediaRequest{Message: pT, Entities: ent, ReplyMarkup: markup, ReplyTo: inputRep, Media: media})
	if err != nil { return nil, err }
	return NewMsgFromGotg(msg, telegram.Context, id, telegram.Client.Self.ID), nil
}
