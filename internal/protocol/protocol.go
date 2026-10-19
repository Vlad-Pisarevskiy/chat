package protocol

import (
	"encoding/json"
	"time"
)

// Коды кадра error. По коду фронт решает, предлагать ли повторить отправку.
const (
	CodeBadRequest = "bad_request"
	CodeForbidden  = "forbidden"
	CodeNotFound   = "not_found"
	CodeInternal   = "internal"
)

// Виды чата в кадре chat_added.
const (
	KindDirect  = "direct"
	KindGroup   = "group"
	KindChannel = "channel"
)

type Data struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type Send struct {
	PeerID      int    `json:"peer_id,omitempty"`
	ChatID      int    `json:"chat_id,omitempty"`
	Body        string `json:"body"`
	ClientMsgID string `json:"client_msg_id"`
}

type Message struct {
	Id       int       `json:"message_id"`
	ChatID   int       `json:"chat_id"`
	SenderID int       `json:"sender_id"`
	Body     string    `json:"body"`
	ImageID  int       `json:"image_id,omitempty"`
	Time     time.Time `json:"time"`
}

type Ack struct {
	ClientMsgID string    `json:"client_msg_id"`
	MessageID   int       `json:"message_id"`
	ChatID      int       `json:"chat_id"`
	Time        time.Time `json:"time"`
}

type Removed struct {
	ChatID int `json:"chat_id"`
}

type Cleared struct {
	ChatID        int `json:"chat_id"`
	UpToMessageID int `json:"up_to_message_id"`
}

type Deleted struct {
	ChatID    int `json:"chat_id"`
	MessageID int `json:"message_id"`
}

type Presence struct {
	UserID int  `json:"user_id"`
	Online bool `json:"online"`
}

type ChatAdded struct {
	ChatID int    `json:"chat_id"`
	Kind   string `json:"kind"`
	Name   string `json:"name,omitempty"`
	Role   string `json:"role,omitempty"`
	PeerID int    `json:"peer_id,omitempty"`
}

type Error struct {
	Code        string `json:"code"`
	Description string `json:"description"`
	ClientMsgID string `json:"client_msg_id,omitempty"`
}
