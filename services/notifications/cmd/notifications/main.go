package main

import (
	notification "chatflow/services/notifications/internal"
	conf "chatflow/services/notifications/internal/config"
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
	defer pool.Close()

	repository := notification.NewRepository(pool)
	service := notification.NewService(repository)

	kafkaClient, err := kgo.NewClient(
		kgo.SeedBrokers(conf.KafkaBrokers()...),
		kgo.ConsumeTopics(conf.KafkaTopic()))
	if err != nil {
		log.Fatal(err)
	}
	defer kafkaClient.Close()

	mail, err := notification.NewMail(fmt.Sprintf("%s:%s", conf.SmtpHost(), conf.SmtpPort()))
	if err != nil {
		log.Fatal(err)
	}

	server := notification.NewServer(service, kafkaClient, mail)
	server.Run()
}
