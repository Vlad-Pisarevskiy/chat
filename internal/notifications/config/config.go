package config

import (
	errors1 "chatflow/internal/app-errors"
	"log"
	"os"

	"github.com/joho/godotenv"
)

func InitConfig() {
	var err error
	if err = godotenv.Load(".env"); err != nil {
		if !os.IsNotExist(err) {
			log.Fatal(err)
		}
	}

	if DBPath() == "" {
		log.Fatal(errors1.ErrEmptyDatabasePath)
	}
}

func SmtpPort() string {
	return os.Getenv("SMTP_PORT")
}

func SmtpHost() string {
	return os.Getenv("SMTP_HOST")
}

func DBPath() string {
	return os.Getenv("DATABASE_URL")
}
