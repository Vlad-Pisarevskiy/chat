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

func Port() string {
	return os.Getenv("PORT")
}

func SmtpHost() string {
	return os.Getenv("SMTP_HOST")
}

func KafkaTopic() string {
	return os.Getenv("TOPIC")
}

func DBPath() string {
	return os.Getenv("DATABASE_URL")
}
