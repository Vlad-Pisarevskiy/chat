package notification

import (
	"context"

	"github.com/twmb/franz-go/pkg/kgo"
)

const notificationsTopic = "notifications"

type Server struct {
	service *Service
	client  *kgo.Client
}

func NewServer(service *Service, client *kgo.Client) *Server {

	return &Server{
		service: service,
		client:  client,
	}
}

func (s *Server) Run() {

	for {

		fetches := s.client.PollFetches(context.Background())

		fetches.EachRecord(func(r *kgo.Record) {

			if err := s.service.ProcessNotify(r.Value); err != nil {
				// TODO: в зависимости от типа ошибки решать будут ли реатраи
			}
		})
	}
}
