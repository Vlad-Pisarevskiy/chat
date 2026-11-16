package service

import (
	notifyserv "chatflow/gen/notify_service/v1"
	"chatflow/services/notifications/internal/model"
	"chatflow/services/notifications/internal/repository"
	"context"
)

type Service struct {
	uow           *repository.UnitOfWork
	mail          *Mail
	notifications notifyserv.NotifyServiceClient
}

func NewService(db *repository.UnitOfWork, mail *Mail) *Service {
	return &Service{
		uow:  db,
		mail: mail}
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

//for _, recipient := range notify.RecipientsIDs {
//
//if err = s.mail.send(notify.Title, notify.Preview); err != nil {
//	return err
//}
//
//resp, err := s.notifications.SendNotify(ctx, &notifyserv.NotifyRequest{
//	UserID:  int64(recipient),
//	Title:   notify.Title,
//	Preview: notify.Preview,
//})

//if err != nil {
//	return err
//}

//if resp.Send {
//	if err = r.Notify.ChangeStatus(ctx, dbNotify.ID, "delivered"); err != nil {
//		return err
//	}
//} else {
//	if err = r.Notify.ChangeStatus(ctx, dbNotify.ID, "failed"); err != nil {
//		return err
//	}
//}
//}
