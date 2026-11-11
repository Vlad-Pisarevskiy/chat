package config

import (
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
		log.Fatal("не задан DATABASE_URL")
	}
}

func Port() string {
	if port := os.Getenv("PORT"); port != "" {
		return port
	}
	return "8002"
}

func GRPCPort() string {
	if port := os.Getenv("GRPC_PORT"); port != "" {
		return port
	}
	return "50051"
}

func Host() string {
	return os.Getenv("HOST")
}

func DBPath() string {
	return os.Getenv("DATABASE_URL")
}
