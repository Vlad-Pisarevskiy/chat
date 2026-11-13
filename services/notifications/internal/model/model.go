package model

type Notification struct {
	ChatID        int    `json:"chat_id"`
	SenderID      int    `json:"sender_id"`
	RecipientsIDs []int  `json:"recipients_ids"`
	Title         string `json:"title"`
	Preview       string `json:"preview"`
}
