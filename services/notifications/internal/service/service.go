package service

import (
	"chatflow/services/notifications/internal/model"
	"chatflow/services/notifications/internal/repository"
	"context"
)

type Service struct {
	db   *repository.Repository
	mail *Mail
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

	if err := s.mail.send(notify.Title, notify.Preview); err != nil {
		return err
	}

	return nil
}
