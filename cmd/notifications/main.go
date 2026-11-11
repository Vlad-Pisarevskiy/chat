package main

import (
	notification "chatflow/internal/notifications"
	conf "chatflow/internal/notifications/config"
	"context"
	"fmt"
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

	repository := notification.NewRepository(pool)
	service := notification.NewService(repository)

	kafkaClient, err := kgo.NewClient(
		kgo.SeedBrokers("localhost:9092"),
		kgo.ConsumeTopics(conf.KafkaTopic()))
	if err != nil {
		log.Fatal(err)
	}

	mail, err := notification.NewMail(fmt.Sprintf("%s:%s", conf.SmtpHost(), conf.SmtpPort()))
	if err != nil {
		log.Fatal(err)
	}

	server := notification.NewServer(service, kafkaClient, mail)
	server.Run()
}
