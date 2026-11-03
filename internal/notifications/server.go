package notification

import (
	"context"
	"log"

	"github.com/twmb/franz-go/pkg/kgo"
)

const notificationsTopic = "notifications"

type Server struct {
	service *Service
	client  *kgo.Client
}

func NewServer(service *Service) *Server {

	client, err := kgo.NewClient(
		kgo.SeedBrokers("localhost:9092"),
		kgo.ConsumeTopics(notificationsTopic),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))

	if err != nil {
		log.Fatal(err)
		return nil
	}

	return &Server{
		service: service,
		client:  client,
	}
}

func (s *Server) Run() error {

	for {
		fetches := s.client.PollFetches(context.Background())

		fetches.EachRecord(func(r *kgo.Record) {

			r.Value
		})
	}
}
