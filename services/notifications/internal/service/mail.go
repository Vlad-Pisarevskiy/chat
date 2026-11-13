package service

import (
	"errors"
	"fmt"
	"net/mail"
	"net/smtp"
)

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

func (s *Mail) send(userName, preview string) error {

	from := s.sender
	to := []string{fmt.Sprintf("%s", s.receiver)}

	var auth smtp.Auth = nil
	var message []byte

	message = []byte(fmt.Sprintf("To: %s \r\n"+
		"Subject: Уведомление о новых сообщениях\r\n"+
		"\r\n"+
		"Привет! Уведомляем о новом сообщении от %s:%s.\r\n", s.receiver, userName, preview))

	err := smtp.SendMail(s.addr, auth, from, to, message)
	if err != nil {
		fmt.Printf("Ошибка отправки: %s\n", err)
		return err
	}

	fmt.Println("Письмо успешно отправлено в Mailpit!")

	return nil
}
