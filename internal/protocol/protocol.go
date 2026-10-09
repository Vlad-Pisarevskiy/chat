package protocol

import (
	"encoding/json"
	"time"
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

type Presence struct {
	UserID int  `json:"user_id"`
	Online bool `json:"online"`
}

type Error struct {
	Code        int    `json:"code"`
	Description string `json:"description"`
}
