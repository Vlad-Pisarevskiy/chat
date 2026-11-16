-- +goose Up
CREATE TABLE notifications(
    id SERIAL,
    chat_id INT,
    sender_id INT,
    recipients_ids INT[],
    title TEXT,
    preview TEXT,
    date TIMESTAMP,
    status TEXT DEFAULT 'pending'
);

CREATE TABLE outbox(
    task_id INT,
    type TEXT,
    status TEXT CHECK ( status IN ('pending', 'done', 'failed', 'in progress')),
    chat_id INT,
    sender_id INT,
    recipients_ids INT[],
    title TEXT,
    preview TEXT
);

-- +goose Down
DROP TABLE notifications;
