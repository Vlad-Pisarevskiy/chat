-- +goose Up
CREATE TABLE users
(
    id       BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name     TEXT NOT NULL ,
    login    TEXT UNIQUE NOT NULL,
    password TEXT NOT NULL
);

CREATE TABLE tokens
(
    token_hash bytea PRIMARY KEY,
    user_id    BIGINT REFERENCES users(id) ON DELETE CASCADE,
    expires_at timestamptz,
    created_at timestamptz
);

-- +goose Down
DROP TABLE tokens;
DROP TABLE users;