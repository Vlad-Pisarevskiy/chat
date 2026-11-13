-- +goose Up
CREATE TABLE notifications(
    id SERIAL,
    chat_id INT,
    sender_id INT,
    recipients_ids INT[],
    title TEXT,
    preview TEXT,
    date TIMESTAMP
);

-- +goose Down
DROP TABLE notifications;
