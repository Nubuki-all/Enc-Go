package models

type Update struct {
	ID            int            `json:"update_id"`
	Message       *Message       `json:"message,omitempty"`
	CallbackQuery *CallbackQuery `json:"callback_query,omitempty"`
}

type User struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot,omitempty"`
	FirstName string `json:"first_name,omitempty"`
	Username  string `json:"username,omitempty"`
}

type Chat struct {
	ID       int64  `json:"id"`
	Type     string `json:"type,omitempty"`
	Title    string `json:"title,omitempty"`
	Username string `json:"username,omitempty"`
}

type Message struct {
	ID              int      `json:"message_id"`
	MessageThreadID int      `json:"message_thread_id,omitempty"`
	From            *User    `json:"from,omitempty"`
	Chat            Chat     `json:"chat"`
	Text            string   `json:"text,omitempty"`
	Caption         string   `json:"caption,omitempty"`
	ReplyToMessage  *Message `json:"reply_to_message,omitempty"`
}

type CallbackQuery struct {
	ID      string                   `json:"id"`
	From    User                     `json:"from,omitempty"`
	Message MaybeInaccessibleMessage `json:"message,omitempty"`
	Data    string                   `json:"data,omitempty"`
}

type MaybeInaccessibleMessage struct {
	Message             *Message
	InaccessibleMessage *InaccessibleMessage
}

type InaccessibleMessage struct {
	Chat      Chat `json:"chat"`
	MessageID int  `json:"message_id"`
	Date      int  `json:"date"`
}

type BotCommand struct {
	Command     string `json:"command"`
	Description string `json:"description"`
}
