package kafka

import (
	"context"

	"github.com/twmb/franz-go/pkg/kadm"
)

type Admin struct {
	admin *kadm.Client
}

func NewAdmin(client *Client) *Admin {

	admin := kadm.NewClient(client.client)
	return &Admin{admin: admin}
}

func (a *Admin) InitTopic(ctx context.Context, topic string, partitions int32) error {

	_, err := a.admin.CreateTopic(ctx,
		partitions,
		1,
		map[string]*string{
			"retention.ms":   kadm.StringPtr("86400000"), // хранить сутки
			"cleanup.policy": kadm.StringPtr("delete")},
		topic)

	if err != nil {
		return err
	}

	return nil
}
