package kafka

import (
	"github.com/twmb/franz-go/pkg/kgo"
)

type Client struct {
	client *kgo.Client
}

func NewClient(client *kgo.Client) *Client {

	return &Client{client: client}
}
