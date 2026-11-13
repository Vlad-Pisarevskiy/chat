package main

import (
	conf "chatflow/services/notifications/internal/config"
	repository2 "chatflow/services/notifications/internal/repository"
	server2 "chatflow/services/notifications/internal/server"
	notification "chatflow/services/notifications/internal/service"
	"context"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
)

func main() {

	conf.InitConfig()

	pool, err := pgxpool.New(context.Background(), conf.DBPath())
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	repository := repository2.NewRepository(pool)
	service := notification.NewService(repository)

	kafkaClient, err := kgo.NewClient(
		kgo.SeedBrokers(conf.KafkaBrokers()...),
		kgo.ConsumeTopics(conf.KafkaTopic()))
	if err != nil {
		log.Fatal(err)
	}
	defer kafkaClient.Close()

	admin := kadm.NewClient(kafkaClient)

	mail, err := notification.NewMail(fmt.Sprintf("%s:%s", conf.SmtpHost(), conf.SmtpPort()))
	if err != nil {
		log.Fatal(err)
	}

	server := server2.NewServer(service, kafkaClient, mail)
	server.Run()
}
