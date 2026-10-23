package config

import (
	"chatflow/internal/auth"
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
		log.Fatal(auth.ErrEmptyDatabasePath)
	}
}

func Port() string {
	return os.Getenv("PORT")
}

func Host() string {
	return os.Getenv("HOST")
}

func DBPath() string {
	return os.Getenv("DATABASE_URL")
}
