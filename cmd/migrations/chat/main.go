package main

import (
	"chatflow/config"
	"database/sql"
	"log"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func main() {

	config.InitConfig()

	db, err := connect()
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		_ = db.Close()
	}()

	if err = goose.SetDialect("postgres"); err != nil {
		log.Fatal(err)
	}
	if err = goose.Up(db, "./migrations"); err != nil {
		log.Fatal(err)
	}

}

func connect() (*sql.DB, error) {

	connStr := config.DBPath()
	db, err := sql.Open("pgx", connStr)
	if err != nil {
		return nil, err
	}

	if err = db.Ping(); err != nil {
		return nil, err
	}

	return db, nil
}
