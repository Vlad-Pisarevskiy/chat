package server

import (
	"chatflow/services/notifications/internal/model"
	"chatflow/services/notifications/internal/service"
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

const notificationsTopic = "notifications"

type Server struct {
	service *service.Service
	client  *kgo.Client
}

func NewServer(service *service.Service, client *kgo.Client) *Server {

	return &Server{
		service: service,
		client:  client,
	}
}

func (s *Server) Run(ctx context.Context) {

	for {

		fetches := s.client.PollFetches(ctx)

		fetches.EachRecord(func(r *kgo.Record) {

			processCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()

			var notify model.Notification
			if err := json.Unmarshal(r.Value, &notify); err != nil {
				log.Println(err)
				return
			}

			if err := s.service.ProcessNotify(processCtx, notify); err != nil {
				log.Println(err)
				return
			}
		})
	}
}
