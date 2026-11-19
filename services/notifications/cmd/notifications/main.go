package main

import (
	conf "chatflow/services/notifications/internal/config"
	repository2 "chatflow/services/notifications/internal/repository"
	server2 "chatflow/services/notifications/internal/server"
	notification "chatflow/services/notifications/internal/service"
	"context"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/twmb/franz-go/pkg/kgo"
)

func main() {

	conf.InitConfig()

	pool, err := pgxpool.New(context.Background(), conf.DBPath())
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	uow := repository2.NewUnitOfWork(pool)
	service := notification.NewService(uow)

	kafkaClient, err := kgo.NewClient(
		kgo.SeedBrokers(conf.KafkaBrokers()...),
		kgo.ConsumeTopics(conf.KafkaTopic()))
	if err != nil {
		log.Fatal(err)
	}
	defer kafkaClient.Close()

	server := server2.NewServer(service, kafkaClient)
	server.Run(context.Background())
}
