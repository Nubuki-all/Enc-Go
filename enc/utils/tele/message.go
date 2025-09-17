package tele

import (
	"fmt"
	"path/filepath"

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
}

type EditOptions struct {
	HtmlParseMode bool
	ReplyMarkup   tg.ReplyMarkupClass
	Media         tg.InputMediaClass
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
	var replyMarkup tg.ReplyMarkupClass
	if m.Raw != nil {
		replyMarkup = m.Raw.ReplyMarkup
	}
	msg, err := m.Ctx.EditMessage(m.ChatID, &tg.MessagesEditMessageRequest{
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
		return NewMsgFromGotg(msg, m.Ctx, m.ChatID, m.Ctx.Self.ID), nil
	}
	return NewMsgFromGotg(msg, m.Ctx, m.ChatID, m.SenderID), nil
}

func (m *Message) Edit(text string, editOptions *EditOptions) (*Message, error) {
	var pText string
	var entities []tg.MessageEntityClass
	if editOptions == nil {
		editOptions = &EditOptions{}
	}
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
		Media:       editOptions.Media,
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

func (m *Message) Forward(chatID int64) (*Message, error) {
	resp, err := m.Ctx.ForwardMessages(m.ChatID, chatID,
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
	switch update := update.(type) {
	case *tg.UpdateNewMessage:
		m := update.GetMessage()
		msg = gp.ConstructMessage(m)
	}
	return NewMsgFromGotg(msg, m.Ctx, chatID, m.Ctx.Self.ID), nil
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
		err = fmt.Errorf("no content was specified to upload")
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

func (m *Message) ReplyMediaDoc(media tg.InputMediaClass, uploadOptions *UploadOptions) (*Message, error) {
	ctx := m.Ctx
	var inputRep *tg.InputReplyToMessage
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
	pText, entities, _ := parser.Parse(text, true)
	_, err := telegram.Context.SendMessage(id, &tg.MessagesSendMessageRequest{
		Message:  pText,
		Entities: entities,
	},
	)
	return err
}

func (m *Message) Action(action string, prog int) error {
	ctx := m.Ctx
	inputPeer, err := ctx.ResolveInputPeerById(m.ChatID)
	if err != nil {
		return err
	}
	msgAction := ctx.Sender.To(inputPeer).TypingAction().ThreadID(m.TopicID)
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

func EditMessage(id int64, msgID int, text string) error {
	pText, entities, _ := parser.Parse(text, true)
	_, err := telegram.Context.EditMessage(id, &tg.MessagesEditMessageRequest{
		ID:       msgID,
		Message:  pText,
		Entities: entities,
	},
	)
	return err
}

func ForwardMessage(fromChatID, toChatID int64, msgID int) (*Message, error) {
	ctx := telegram.Context
	resp, err := ctx.ForwardMessages(fromChatID, toChatID,
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
