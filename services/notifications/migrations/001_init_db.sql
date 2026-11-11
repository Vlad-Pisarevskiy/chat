-- +goose Up
CREATE TABLE notifications(
    type TEXT,
    sender INT
);

-- +goose Down
DROP TABLE notifications;
