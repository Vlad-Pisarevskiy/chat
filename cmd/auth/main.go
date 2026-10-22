package main

import (
	"chatflow/config"
	"chatflow/internal/auth"
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	interval = time.Hour * 2
)

func main() {

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(context.Background(), config.DBPath())
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	repository := auth.NewRepository(pool)
	service := auth.NewService(repository)
	handler := auth.NewHandler(service)

	go cleanupTokens(ctx, service)

	router := handler.RegisterRouter()

	log.Fatal(router.Run(fmt.Sprintf("%s:%s", config.Host(), config.Port())))
}

func cleanupTokens(ctx context.Context, service *auth.Service) {

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			deleted, err := service.CleanupTokens(ctx)
			if err != nil {
				log.Println(err)
				continue
			}
			log.Printf("Deleted %d expired tokens", deleted)
		}
	}
}
