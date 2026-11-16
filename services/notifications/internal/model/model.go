package model

import "time"

type Notification struct {
	ChatID        int    `json:"chat_id"`
	SenderID      int    `json:"sender_id"`
	RecipientsIDs []int  `json:"recipients_ids"`
	Title         string `json:"title"`
	Preview       string `json:"preview"`
}

type NotifyFromDB struct {
	ID            int
	ChatID        int       `json:"chat_id"`
	SenderID      int       `json:"sender_id"`
	RecipientsIDs []int     `json:"recipients_ids"`
	Title         string    `json:"title"`
	Preview       string    `json:"preview"`
	Date          time.Time `json:"date"`
	Status        string    `json:"status"`
}
