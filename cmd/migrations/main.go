package main

import (
	"chatflow/config"
	"database/sql"
	"log"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"golang.org/x/crypto/bcrypt"
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

	hash, err := bcrypt.GenerateFromPassword([]byte("123"), bcrypt.DefaultCost)
	if err != nil {
		log.Println(err)
	}

	_, err = db.Exec(`INSERT INTO users(name, login, password)
				   VALUES ('Пользователь1', 'user1', $1),
				          ('Пользователь2', 'user2', $1),
				          ('Пользователь3', 'user3', $1)`, hash)

	if err != nil {
		log.Println(err)
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
