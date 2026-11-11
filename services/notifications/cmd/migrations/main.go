package main

import (
	"chatflow/services/notifications"
	"chatflow/services/notifications/internal/config"
	"database/sql"
	"log"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func main() {
	config.InitConfig()

	db, err := sql.Open("pgx", config.DBPath())
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err = goose.SetDialect("postgres"); err != nil {
		log.Fatal(err)
	}
	goose.SetBaseFS(notifications.Migrations)
	if err = goose.Up(db, "migrations"); err != nil {
		log.Fatal(err)
	}
}
