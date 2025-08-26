package tele

import (
	"fmt"
	"path/filepath"

	"github.com/celestix/gotgproto/ext"
	gp "github.com/celestix/gotgproto/types"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"

	"enc/internal/bot"
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
}

type EditOptions struct {
	HtmlParseMode bool
	ReplyMarkup   tg.ReplyMarkupClass
	// will add more fields as required
}

type ReplyOptions struct {
	HtmlParseMode bool
	ReplyMarkup   tg.ReplyMarkupClass
	// will add more fields as required
}

type UploadOptions struct {
	Name          string
	Caption       string
	Path          string
	Bytes         []byte
	Quote         bool
	HtmlParseMode bool
	ReplyMarkup   tg.ReplyMarkupClass
	// will add more fields as required
}

// NewFromGotg constructs a wrapper from a concrete *gp.Message.
func NewMsgFromGotg(m *gp.Message, c *ext.Context, chatID int64, senderID int64) *Message {
	replyTo := 0
	if replyMessage, ok := m.ReplyTo.(*tg.MessageReplyHeader); ok {
		replyTo = replyMessage.ReplyToMsgID
	}
	return &Message{
		Raw:       m,
		Ctx:       c,
		ChatID:    chatID,
		SenderID:  senderID,
		ID:        m.ID,
		ReplyToID: replyTo,
	}
}

// RawMessage returns the underlying *gp.Message.
func (m *Message) RawMessage() *gp.Message {
	return m.Raw
}

func (m *Message) ReplyText(text string) (*Message, error) {
	pText, entities, _ := parser.Parse(text, true)
	inputRep := &tg.InputReplyToMessage{
		ReplyToMsgID: m.ID,
	}
	msg, err := m.Ctx.SendMessage(m.ChatID, &tg.MessagesSendMessageRequest{
		Message:  pText,
		Entities: entities,
		ReplyTo:  inputRep,
	},
	)
	if err != nil {
		logger.ErrorW(err.Error())
		return nil, err
	}
	return NewMsgFromGotg(msg, m.Ctx, m.ChatID, m.Ctx.Self.ID), nil
}

func (m *Message) Reply(text string, replyOptions *ReplyOptions) (*Message, error) {
	var pText string
	var entities []tg.MessageEntityClass
	if replyOptions.HtmlParseMode {
		pText, entities, _ = htmlparser.Parse(text)
	} else {
		pText, entities, _ = parser.Parse(text, true)
	}
	inputRep := &tg.InputReplyToMessage{
		ReplyToMsgID: m.ID,
	}
	msg, err := m.Ctx.SendMessage(m.ChatID, &tg.MessagesSendMessageRequest{
		Message:     pText,
		Entities:    entities,
		ReplyTo:     inputRep,
		ReplyMarkup: replyOptions.ReplyMarkup,
	},
	)
	if err != nil {
		logger.ErrorW(err.Error())
		return nil, err
	}
	return NewMsgFromGotg(msg, m.Ctx, m.ChatID, m.Ctx.Self.ID), nil
}

func (m *Message) EditText(text string) (*Message, error) {
	pText, entities, _ := parser.Parse(text, true)
	msg, err := m.Ctx.EditMessage(m.ChatID, &tg.MessagesEditMessageRequest{
		ID:       m.ID,
		Message:  pText,
		Entities: entities,
	},
	)
	if err != nil {
		logger.ErrorW(err.Error())
		return nil, err
	}
	if m.Raw == nil {
		return NewMsgFromGotg(msg, m.Ctx, m.ChatID, m.Ctx.Self.ID), nil
	}
	return NewMsgFromGotg(msg, m.Ctx, m.ChatID, m.SenderID), nil
}

func (m *Message) Edit(text string, editOptions *EditOptions) (*Message, error) {
	var pText string
	var entities []tg.MessageEntityClass
	if editOptions.HtmlParseMode {
		pText, entities, _ = htmlparser.Parse(text)
	} else {
		pText, entities, _ = parser.Parse(text, true)
	}
	msg, err := m.Ctx.EditMessage(m.ChatID, &tg.MessagesEditMessageRequest{
		ID:          m.ID,
		Message:     pText,
		Entities:    entities,
		ReplyMarkup: editOptions.ReplyMarkup,
	},
	)
	if err != nil {
		logger.ErrorW(err.Error())
		return nil, err
	}
	if m.Raw == nil {
		return NewMsgFromGotg(msg, m.Ctx, m.ChatID, m.Ctx.Self.ID), nil
	}
	return NewMsgFromGotg(msg, m.Ctx, m.ChatID, m.SenderID), nil
}

func (m *Message) RawText() (string, error) {
	if m.Raw == nil {
		return "", nil
	}
	return parser.Unparse(m.Raw.Text, m.Raw.Entities)
}

func (m *Message) Delete() error {
	return m.Ctx.DeleteMessages(m.ChatID, []int{m.ID})
}

func (m *Message) UploadDoc(uploadOptions *UploadOptions) (*Message, error) {
	var file tg.InputFileClass
	var inputRep *tg.InputReplyToMessage
	var err error
	ctx := m.Ctx
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
		err = fmt.Errorf("No content was specified to upload.")
	}
	if err != nil {
		logger.Error(err)
		return nil, err
	}

	if uploadOptions.Quote {
		inputRep = &tg.InputReplyToMessage{
			ReplyToMsgID: m.ID,
		}
	}
	var pCaption string
	var entities []tg.MessageEntityClass
	if caption := uploadOptions.Caption; caption != "" {
		if uploadOptions.HtmlParseMode {
			pCaption, entities, _ = htmlparser.Parse(caption)
		} else {
			pCaption, entities, _ = parser.Parse(caption, true)
		}
	}
	media := &tg.InputMediaUploadedDocument{
		File: file,
		// set attributes so file has a name
		Attributes: []tg.DocumentAttributeClass{
			&tg.DocumentAttributeFilename{FileName: name},
		},
	}
	msg, err := ctx.SendMedia(m.ChatID,
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
		return NewMsgFromGotg(msg, m.Ctx, m.ChatID, m.Ctx.Self.ID), nil
	}
	return NewMsgFromGotg(msg, m.Ctx, m.ChatID, m.SenderID), nil
}

func SendMessage(id int64, text string) error {
	// peer, _ := GetPeer(id)
	pText, entities, _ := parser.Parse(text, true)
	// ParseMarkdown(text, true)
	_, err := telegram.Context.SendMessage(id, &tg.MessagesSendMessageRequest{
		Message: pText,
		// Peer:     peer,
		Entities: entities,
	},
	)
	return err
}

func EditMessage(id int64, msgID int, text string) error {
	// peer, _ := GetPeer(id)
	pText, entities, _ := parser.Parse(text, true)
	// ParseMarkdown(text, true)
	_, err := telegram.Context.EditMessage(id, &tg.MessagesEditMessageRequest{
		ID:      msgID,
		Message: pText,
		// Peer:     peer,
		Entities: entities,
	},
	)
	return err
}

//Broadcast a message to every registered user in bot and returns a list of the messages sent.
func BcMsg(text string, exceptUser int64, mlist []*Message) []*Message {
	AddMessage := func (id int64) {
		ctx := telegram.Context
		pText, entities, _ := parser.Parse(text, true)
		resp, err := ctx.SendMessage(id, &tg.MessagesSendMessageRequest{
			Message: pText,
			Entities: entities,
		},
		)
		if err != nil {
			logger.ErrorW(err.Error())
			return nil, err
		}
		msg := NewMsgFromGotg(resp, ctx, id, ctx.Self.ID)
		mlist = append(mlist, msg)
	}
	for _, owner := range config.Rconf.Owners {
		if owner == exceptUser {
			continue
		}
		AddMessage(owner)
	}
	for _, user := range strings.Fields(bot.TempUsers) {
		if user == exceptUser {
			continue
		}
		AddMessage(user)
	}
	if logchat := config.Conf.LogChannel; logchat != 0 {
		AddMessage(logchat)
	}
	return mlist
}