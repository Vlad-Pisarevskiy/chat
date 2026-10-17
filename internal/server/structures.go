package server

import "time"

type RegisterRequest struct {
	Name     string `json:"name"`
	Login    string `json:"login"`
	Password string `json:"password"`
}

// ImageResponse — ответ на отправку картинки, он же ack. ImageID не приходит
// только у повтора, который оказался уже сохранённым текстовым сообщением.
type ImageResponse struct {
	MessageID int       `json:"message_id"`
	ChatID    int       `json:"chat_id"`
	ImageID   int       `json:"image_id,omitempty"`
	Time      time.Time `json:"time"`
}

type LoginRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}
