package service

import "chatflow/internal/protocol"

type RegisterInput struct {
	Name     string
	Login    string
	Password string
}

type ChatDelete struct {
	UserID int `json:"user_id"`
	ChatID int `json:"chat_id"`
}

type GroupCreate struct {
	OwnerID int    `json:"-"`
	Name    string `json:"name"`
	Members []int  `json:"member_ids"`
}

type ChannelCreates struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
	Handle      *string `json:"handle"`
}

type MessageDelete struct {
	UserID    int `json:"-"`
	ChatID    int `json:"-"`
	MessageID int `json:"message_id"`
}

type JoinChannel struct {
	UserID    int `json:"user_id"`
	ChannelID int `json:"channel_id"`
}

// CreatedGroup — созданная группа: название уже после trim, участники вместе
// с создателем. Им всем сервер разошлёт chat_added.
type CreatedGroup struct {
	ChatID  int
	Name    string
	Members []int
}

// SentMessage — результат send. Duplicate означает повтор уже сохранённого
// сообщения: Recipients тогда пуст, рассылать нечего, нужен только ack.
type SentMessage struct {
	Message    *protocol.Message
	Recipients []int
	Duplicate  bool
}
