package kafka

import (
	"context"
	"encoding/json"
	"log"
	"strconv"

	"github.com/twmb/franz-go/pkg/kgo"
)

type Client struct {
	client *kgo.Client
}

type Notification struct {
	ChatID        int    `json:"chat_id"`
	SenderID      int    `json:"sender_id"`
	RecipientsIDs []int  `json:"recipients_ids"`
	Title         string `json:"title"`
	Preview       string `json:"preview"`
}

func NewClient(client *kgo.Client) *Client {

	return &Client{client: client}
}

func (c *Client) Send(ctx context.Context, notify Notification) error {

	data, err := json.Marshal(notify)
	if err != nil {
		return err
	}

	rec := &kgo.Record{
		Key:   []byte(strconv.Itoa(notify.ChatID)),
		Value: data,
	}

	c.client.Produce(ctx, rec, func(rec *kgo.Record, err error) {
		if err != nil {
			log.Println(err)
			return
		}
	})

	return nil
}
