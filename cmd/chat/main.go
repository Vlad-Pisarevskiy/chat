package main

import (
	"chatflow/config"
	hub2 "chatflow/internal/hub"
	"chatflow/internal/repository/postgres"
	server2 "chatflow/internal/server"
	service2 "chatflow/internal/service"
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
)

const (
	timeout = time.Second * 10
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
	service := service2.New(repository)

	server := server2.New(service, hub)
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
