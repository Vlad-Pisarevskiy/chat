package notification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/mail"
	"net/smtp"

	"github.com/twmb/franz-go/pkg/kgo"
)

const notificationsTopic = "notifications"

type Server struct {
	service *Service
	client  *kgo.Client
	email   *Mail
}

type Mail struct {
	addr     string
	sender   string
	receiver string
}

type Option func(*Mail) error

func WithSender(s string) Option {
	return func(e *Mail) error {

		if !validEmail(s) {
			return errors.New("incorrect sender email format")
		}

		return nil
	}
}

func WithRecipient(r string) Option {
	return func(e *Mail) error {

		if !validEmail(r) {
			return errors.New("incorrect sender email format")
		}
		return nil
	}
}

func validEmail(value string) bool {
	addr, err := mail.ParseAddress(value)
	return err == nil && addr.Address == value
}

func NewMail(addr string, opts ...Option) (*Mail, error) {

	e := &Mail{
		addr:     addr,
		sender:   "sender@example.com",
		receiver: "recipient@example.com",
	}

	for _, opt := range opts {
		if err := opt(e); err != nil {
			return nil, err
		}
	}

	return e, nil
}

func NewServer(service *Service, client *kgo.Client, email *Mail) *Server {

	return &Server{
		service: service,
		client:  client,
		email:   email,
	}
}

func (s *Server) Run() {

	for {

		fetches := s.client.PollFetches(context.Background())

		fetches.EachRecord(func(r *kgo.Record) {

			var data Data
			if err := json.Unmarshal(r.Value, &data); err != nil {
				log.Println(err)
				return
				//TODO: Ретраи, добавить в другую партицию итд
			}

			switch data.Type {
			case "chat_notify":
				var chatNotify ChatNotify
				if err := json.Unmarshal(data.Payload, &chatNotify); err != nil {
					log.Println(err)
				}

			case "channel_notify":
				var channelNotify ChannelNotify
				if err := json.Unmarshal(data.Payload, &channelNotify); err != nil {
					log.Println(err)
				}
			}

			if err := s.service.ProcessNotify(data); err != nil {
				log.Println(err)
			}
		})
	}
}

func (s *Mail) send(data Data) error {

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

	err := smtp.SendMail(s.addr, auth, from, to, message)
	if err != nil {
		fmt.Printf("Ошибка отправки: %s\n", err)
		return err
	}

	fmt.Println("Письмо успешно отправлено в Mailpit!")

	return nil
}
