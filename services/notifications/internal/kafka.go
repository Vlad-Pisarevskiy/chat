package internal

import (
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
)

type KafkaClient struct {
	client *kgo.Client
}

func NewKafkaClient(client * kgo.Client) *KafkaClient{

	return &KafkaClient{client: client}
}

type KafkaAdmin struct {

	admin *kadm.Client
}