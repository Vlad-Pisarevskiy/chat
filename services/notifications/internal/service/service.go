package service

import (
	notifyserv "chatflow/gen/notify_service/v1"
	"chatflow/services/notifications/internal/model"
	"chatflow/services/notifications/internal/repository"
	"context"
)

type Service struct {
	db            *repository.Repository
	mail          *Mail
	notifications notifyserv.NotifyServiceClient
}

func NewService(db *repository.Repository, mail *Mail) *Service {
	return &Service{
		db:   db,
		mail: mail}
}

func (s *Service) ProcessNotify(ctx context.Context, notify model.Notification) error {

	//TODO outbox
	if err := s.db.SaveNotify(ctx, notify); err != nil {
		return err
	}

	for _, recipient := range notify.RecipientsIDs {

		if err := s.mail.send(notify.Title, notify.Preview); err != nil {
			return err
		}

		send, err := s.notifications.SendNotify(ctx, &notifyserv.NotifyRequest{
			UserID:  int64(recipient),
			Title:   notify.Title,
			Preview: notify.Preview,
		})

		if !send {

		}

		if err != nil {
			return err
		}
	}

	return nil
}
