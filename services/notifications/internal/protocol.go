package internal

import "encoding/json"

type Data struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type ChatNotify struct {
	ChatID     int `json:"chat_id"`
	SenderID   int `json:"sender_id"`
	ReceiverID int `json:"receiver_id"`
}

type ChannelNotify struct {
	ChannelID  int `json:"channel_id"`
	ReceiverID int `json:"receiver_id"`
}
