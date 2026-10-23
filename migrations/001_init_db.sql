-- +goose Up
CREATE TABLE chats
(
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name TEXT DEFAULT 'default',
    type TEXT NOT NULL DEFAULT 'direct' CHECK (type IN ('direct','group', 'channel')),
    owner_id BIGINT ,
    handle TEXT UNIQUE,
    description TEXT
);

CREATE TABLE users_chats
(
    chat_id   BIGINT REFERENCES chats (id) ON DELETE CASCADE,
    user_id   BIGINT,
    last_read BIGINT,
    border BIGINT DEFAULT 0,
    role TEXT DEFAULT 'member' CHECK(role IN ('member', 'owner', 'admin')),
    PRIMARY KEY (user_id, chat_id)
);

-- Картинки лежат в базе. Сообщение ссылается на свою картинку через
-- messages.image_id; байты живут отдельно, чтобы история их не тянула.
CREATE TABLE images
(
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    chat_id      BIGINT NOT NULL REFERENCES chats (id) ON DELETE CASCADE,
    content_type TEXT NOT NULL CHECK (content_type IN ('image/jpeg', 'image/png')),
    data         BYTEA NOT NULL
);

-- Внешний ключ индекса сам не создаёт. Без него каскадное удаление чата и
-- проверка доступа к картинке по чату читали бы всю таблицу с байтами.
CREATE INDEX images_chat_id_idx ON images (chat_id);

CREATE TABLE messages
(
    id        BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    chat_id   BIGINT REFERENCES chats (id) ON DELETE CASCADE NOT NULL ,
    sender_id BIGINT NOT NULL,
    client_msg_id uuid,
    body      TEXT,
    created_at timestamptz,
    image_id  BIGINT UNIQUE REFERENCES images (id) ON DELETE SET NULL,
    -- Повтор send с тем же client_msg_id не должен порождать второе сообщение.
    UNIQUE (sender_id, client_msg_id)
);

-- История читается пачками по курсору: «последние N сообщений чата с id меньше
-- заданного». По этому индексу база сразу встаёт на курсор и идёт назад, не
-- перебирая весь чат.
CREATE INDEX messages_chat_id_id_idx ON messages (chat_id, id);

-- Один личный чат на пару. Пара хранится упорядоченной, чтобы (A, B) и (B, A)
-- были одной строкой и упирались в один первичный ключ.
CREATE TABLE direct_chats
(
    user_low  BIGINT NOT NULL,
    user_high BIGINT NOT NULL,
    chat_id   BIGINT NOT NULL ,
    PRIMARY KEY (user_low, user_high),
    CHECK (user_low < user_high)
);

-- +goose Down
DROP TABLE direct_chats;
DROP TABLE users_chats;
DROP TABLE messages;
DROP TABLE images;
DROP TABLE chats;

