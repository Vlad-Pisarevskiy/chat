package internal

type Service struct {
	db *Repository
}

type Notify struct {
	Type       string
	ChatID     string
	ReceiverID string
}

func NewService(db *Repository) *Service {
	return &Service{db: db}
}

func (s *Service) ProcessNotify(data Data) error {
	return nil
}
