package main

import (
	"chatflow/services/chat/internal/config"
	hub2 "chatflow/services/chat/internal/hub"
	"chatflow/services/chat/internal/kafka"
	"chatflow/services/chat/internal/repository/postgres"
	server2 "chatflow/services/chat/internal/server"
	service2 "chatflow/services/chat/internal/service"
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	timeout           = time.Second * 10
	defaultPartitions = 3
)

func main() {

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)

	config.InitConfig()

	pool, err := pgxpool.New(context.Background(), config.DBPath())
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	hub := hub2.New()
	repository := postgres.New(pool)
	authConn, err := grpc.NewClient(config.AuthGRPCAddress(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatal(err)
	}
	defer authConn.Close()

	client, err := kgo.NewClient(kgo.SeedBrokers(fmt.Sprintf("%s:%s", config.KafkaHost(), config.KafkaPort())))
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	if err = client.Ping(ctx); err != nil {
		log.Fatal(err)
	}

	kafkaClient := kafka.NewClient(client)
	service := service2.New(repository, authConn, kafkaClient)

	admin := kafka.NewAdmin(kafkaClient)
	if err = admin.InitTopic(ctx, config.KafkaTopic(), defaultPartitions); err != nil && errors.Is(err, kerr.TopicAlreadyExists) {
		log.Fatal(err)
	}

	server, err := server2.New(service, hub)
	if err != nil {
		log.Fatal(err)
	}
	r := server.GetRouter()

	srv := &http.Server{
		Addr:    fmt.Sprintf("%s:%s", config.Host(), config.Port()),
		Handler: r,
	}

	go listenAndServe(srv, errCh)

	select {
	case <-ctx.Done():
		stop()
	case err = <-errCh:
		log.Println(err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err = srv.Shutdown(shutdownCtx); err != nil {
		log.Println(err)
	}

	hub.Close()
}

func listenAndServe(server *http.Server, errCh chan error) {
	if err := server.ListenAndServe(); err != nil {
		if errors.Is(err, http.ErrServerClosed) {
			return
		}
		errCh <- err
	}
}
