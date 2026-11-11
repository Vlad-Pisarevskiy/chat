package config

import (
	"log"
	"os"
	"strings"

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
	if topic := os.Getenv("TOPIC"); topic != "" {
		return topic
	}
	return "notifications"
}

func KafkaBrokers() []string {
	if brokers := os.Getenv("KAFKA_BROKERS"); brokers != "" {
		return strings.Split(brokers, ",")
	}
	return []string{"localhost:9092"}
}

func DBPath() string {
	return os.Getenv("DATABASE_URL")
}
