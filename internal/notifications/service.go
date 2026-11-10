package notification

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/smtp"
)

type Service struct {
	db *Repository
}

func NewService(db *Repository) *Service {
	return &Service{db: db}
}

func (s *Service) ProcessNotify(notify []byte) error {

	var data Data
	if err := json.Unmarshal(notify, &data); err != nil {
		return err
	}

	switch data.Type {
	case "chat_notify":
		var chatNotify ChatNotify
		if err := json.Unmarshal(data.Payload, &chatNotify); err != nil {
			return err
		}

		s.db.SaveNotify()

	case "channel_notify":
		var channelNotify ChannelNotify

		if err := json.Unmarshal(data.Payload, &channelNotify); err != nil {
			return err
		}

		s.db.SaveNotify()
	}

	s.EmailSend(data)

	// TODO: grpc запрос к чатам

}

func (s *Service) EmailSend(data Data, smtpHost, smtpPort string) error {

	address := smtpHost + ":" + smtpPort

	from := "sender@example.com"
	to := []string{"recipient@example.com"}

	var auth smtp.Auth = nil
	var message []byte

	switch data.Type {

	case "chat_notify":

		var chatNotify ChatNotify
		if err := json.Unmarshal(data.Payload, &chatNotify); err != nil {
			return err
		}

		message = []byte("To: recipient@example.com\r\n" +
			"Subject: Тестовое письмо из Go!\r\n" +
			"\r\n" +
			"Привет! Уведомляем, что у нас есть непрочитанные личные чаты.\r\n")

	case "channel_notify":
		var channelNotify ChannelNotify

		if err := json.Unmarshal(data.Payload, &channelNotify); err != nil {
			return err
		}

		message = []byte("To: recipient@example.com\r\n" +
			"Subject: Уведомление о посте в канале\r\n" +
			"\r\n" +
			"Привет! Уведомляем, что в канале на который вы подписаны вышел пост.\r\n")

	default:
		return errors.New("unknown data type")
	}

	err := smtp.SendMail(address, auth, from, to, message)
	if err != nil {
		fmt.Printf("Ошибка отправки: %s\n", err)
		return err
	}

	fmt.Println("Письмо успешно отправлено в Mailpit!")

	return nil
}
