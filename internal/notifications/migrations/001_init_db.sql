-- +goose up
CREATE TABLE notifications(
    type TEXT,
    sender INT
)