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
	return "8001"
}

func AuthGRPCAddress() string {
	if address := os.Getenv("AUTH_GRPC_ADDR"); address != "" {
		return address
	}
	return "localhost:50051"
}

func Host() string {
	return os.Getenv("HOST")
}

func DBPath() string {
	return os.Getenv("DATABASE_URL")
}

func KafkaPort() string {
	return os.Getenv("KAFKA_PORT")
}

func KafkaHost() string {
	return os.Getenv("KAFKA_HOST")
}

func KafkaTopic() string {
	return os.Getenv("KAFKA_TOPIC")
}
