package main

import (
	notifyserv "chatflow/gen/notify_service/v1"
	"chatflow/services/notifications/internal/repository"
	"context"
	"fmt"
	"log"
)

type Worker struct {
	mail          *Mail
	notifications notifyserv.NotifyServiceClient
	db            *repository.OutboxRepository
}

func NewWorker(db *repository.OutboxRepository, mail *Mail) *Worker {

	return &Worker{
		db:   db,
		mail: mail,
	}
}

func (w *Worker) Run(ctx context.Context) error {

	for {

		tasks, err := w.db.GetTasks(ctx)
		if err != nil {
			return err
		}

		go func() {
			err = w.Process(ctx, tasks)
		}()
	}
}

func (w *Worker) Process(ctx context.Context, tasks []repository.TaskFromDB) error {

	for _, task := range tasks {
		switch task.Type {
		case "mail":

			for _, _ = range task.RecipientsIDs {
				if err := w.mail.send(task.Title, task.Preview); err != nil {
					log.Println(err)

					if err = w.db.ClaimFailed(ctx, task.ID); err != nil {
						log.Println(err)
					}
				}
			}

		case "websocket":

			if err := w.Send(ctx, task); err != nil {
				log.Println(err)

				if err = w.db.ClaimFailed(ctx, task.ID); err != nil {
					log.Println(err)
				}
			}

		default:
			fmt.Printf("Unknown type of task %d", task.ID)
			if err := w.db.ClaimFailed(ctx, task.ID); err != nil {
				log.Println(err)
			}
		}
	}

	return nil
}

func (w *Worker) Send(ctx context.Context, task repository.TaskFromDB) error {
	for _, recipient := range task.RecipientsIDs {

		resp, err := w.notifications.SendNotify(ctx, &notifyserv.NotifyRequest{
			UserID:  int64(recipient),
			Title:   task.Title,
			Preview: task.Preview,
		})

		if err != nil {
			return err
		}

		if resp.Send {
			if err = w.db.ChangeStatus(ctx, task.ID, "delivered"); err != nil {
				return err
			}
		} else {
			if err = w.db.ChangeStatus(ctx, task.ID, "failed"); err != nil {
				return err
			}
		}
	}

	return nil
}
