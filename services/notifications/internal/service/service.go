package service

import (
	"chatflow/services/notifications/internal/model"
	"chatflow/services/notifications/internal/repository"
	"context"
)

type Service struct {
	uow *repository.UnitOfWork
}

func NewService(db *repository.UnitOfWork) *Service {
	return &Service{
		uow: db}
}

func (s *Service) SaveNotify(ctx context.Context, notify model.Notification) error {

	return s.uow.Do(ctx, func(r repository.Repos) error {

		_, err := r.Notify.SaveNotify(ctx, notify)
		if err != nil {
			return err
		}

		mailTask := repository.OutboxTask{
			Type:          "mail",
			Status:        "pending",
			ChatID:        notify.ChatID,
			SenderID:      notify.SenderID,
			RecipientsIDs: notify.RecipientsIDs,
			Title:         notify.Title,
			Preview:       notify.Preview,
		}

		websocketTask := repository.OutboxTask{
			Type:          "mail",
			Status:        "pending",
			RecipientsIDs: notify.RecipientsIDs,
			Title:         notify.Title,
			Preview:       notify.Preview,
		}

		if err = r.Outbox.AddTask(ctx, mailTask); err != nil {
			return err
		}

		if err = r.Outbox.AddTask(ctx, websocketTask); err != nil {
			return err
		}

		return nil
	})
}
