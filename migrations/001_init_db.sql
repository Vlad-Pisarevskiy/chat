-- +goose Up
CREATE TABLE users
(
    id       BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name     TEXT NOT NULL ,
    login    TEXT UNIQUE NOT NULL,
    password TEXT NOT NULL
);

CREATE TABLE chats
(
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    label TEXT DEFAULT 'default',
    type TEXT NOT NULL DEFAULT 'direct' CHECK (type IN ('direct','group', 'channel')),
    owner_id BIGINT DEFAULT 0 REFERENCES users (id)
);

CREATE TABLE users_chats
(
    chat_id   BIGINT REFERENCES chats (id) ON DELETE CASCADE,
    user_id   BIGINT REFERENCES users (id) ON DELETE CASCADE,
    last_read BIGINT,
    PRIMARY KEY (user_id, chat_id)
);

CREATE TABLE messages
(
    id        BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    chat_id   BIGINT REFERENCES chats (id) ON DELETE CASCADE NOT NULL ,
    sender_id BIGINT REFERENCES users(id) NOT NULL,
    client_msg_id uuid,
    body      TEXT,
    created_at timestamptz
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
DROP TABLE users_chats;
DROP TABLE messages;
DROP TABLE chats;
DROP TABLE users;

