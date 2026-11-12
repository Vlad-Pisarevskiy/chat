package internal

type Service struct {
	db *Repository
}

type Notification struct {
	Type       string
	ChatID     string
	ReceiverID string
}

func NewService(db *Repository) *Service {
	return &Service{db: db}
}

func (s *Service) ProcessNotify(notify Notification) error {
	return nil
}
