package main

import (
	"chatflow/config"
	"chatflow/internal/auth"
	"context"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {

	pool, err := pgxpool.New(context.Background(), config.DBPath())
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	repository := auth.NewRepository(pool)
	service := auth.NewService(repository)
	handler := auth.NewHandler(service)

	router := handler.RegisterRouter()

	log.Fatal(router.Run(fmt.Sprintf("%s:%s", config.Host(), config.Port())))
}
