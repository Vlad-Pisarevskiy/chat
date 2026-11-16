package service

import (
	notifyserv "chatflow/gen/notify_service/v1"
	"chatflow/services/notifications/internal/model"
	"chatflow/services/notifications/internal/repository"
	"context"
)

type Service struct {
	db            *repository.NotifyRepository
	mail          *Mail
	notifications notifyserv.NotifyServiceClient
}

func NewService(db *repository.NotifyRepository, mail *Mail) *Service {
	return &Service{
		db:   db,
		mail: mail}
}

func (s *Service) ProcessNotify(ctx context.Context, notify model.Notification) error {

	//TODO outbox
	dbNotify, err := s.db.SaveNotify(ctx, notify)
	if err != nil {
		return err
	}

	for _, recipient := range notify.RecipientsIDs {

		if err = s.mail.send(notify.Title, notify.Preview); err != nil {
			return err
		}

		resp, err := s.notifications.SendNotify(ctx, &notifyserv.NotifyRequest{
			UserID:  int64(recipient),
			Title:   notify.Title,
			Preview: notify.Preview,
		})

		if err != nil {
			return err
		}

		if resp.Send {
			if err = s.db.ChangeStatus(ctx, dbNotify.ID, "delivered"); err != nil {
				return err
			}
		} else {
			if err = s.db.ChangeStatus(ctx, dbNotify.ID, "failed"); err != nil {
				return err
			}
		}

	}

	return nil
}
