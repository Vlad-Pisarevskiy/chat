package main

import (
	authserv "chatflow/gen/auth_service/v1"
	auth "chatflow/services/auth/internal"
	conf "chatflow/services/auth/internal/config"
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
)

const (
	interval = time.Hour * 2
)

func main() {

	conf.InitConfig()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(context.Background(), conf.DBPath())
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	repository := auth.NewRepository(pool)
	service := auth.NewService(repository)
	handler := auth.NewHandler(service)

	lis, err := net.Listen("tcp", net.JoinHostPort(conf.Host(), conf.GRPCPort()))
	if err != nil {
		log.Fatalf("listen: %v", err)
	}

	s := grpc.NewServer()
	authserv.RegisterUserServiceServer(s, handler)
	authserv.RegisterTokenServiceServer(s, handler)
	go func() {
		log.Fatal(s.Serve(lis))
	}()

	go cleanupTokens(ctx, service)

	router := handler.RegisterRouter()

	log.Fatal(router.Run(fmt.Sprintf("%s:%s", conf.Host(), conf.Port())))
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
