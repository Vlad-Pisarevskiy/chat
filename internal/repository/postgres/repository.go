package postgres

import (
	errors1 "chatflow/internal/app-errors"
	"chatflow/internal/model"
	"chatflow/internal/protocol"
	"context"
	"errors"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {

	return &Repository{pool: pool}
}

func (r *Repository) RegisterUser(ctx context.Context, client model.User) error {

	_, err := r.pool.Exec(ctx, "INSERT INTO users(name, login, password) VALUES ($1, $2, $3)",
		client.Name, client.Login, client.Password)
	if err != nil {
		return err
	}

	return nil
}

func (r *Repository) LoginExists(ctx context.Context, login string) error {

	row := r.pool.QueryRow(ctx, "SELECT login FROM users WHERE login = $1", login)

	var dbLogin string

	err := row.Scan(&dbLogin)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}

	return errors1.ErrExistsLogin
}

func (r *Repository) FindUserByLogin(ctx context.Context, login string) (*model.User, error) {

	var user model.User

	row := r.pool.QueryRow(ctx, "SELECT id, name, login, password FROM users WHERE login=($1)", login)
	if err := row.Scan(
		&user.ID,
		&user.Name,
		&user.Login,
		&user.Password); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors1.ErrIncorrectLoginData
		}
		return nil, err
	}

	return &user, nil
}

func (r *Repository) FindUserByID(ctx context.Context, id int) (*model.UserFromDB, error) {

	var user model.UserFromDB

	row := r.pool.QueryRow(ctx, "SELECT id, name, login FROM users WHERE id=$1", id)
	if err := row.Scan(
		&user.ID,
		&user.Name,
		&user.Login); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors1.ErrIncorrectLoginData
		}
		return nil, err
	}

	return &user, nil
}

func (r *Repository) GetUsersExcept(ctx context.Context, id int) ([]*model.UserFromDB, error) {

	var users []*model.UserFromDB

	rows, err := r.pool.Query(ctx, "SELECT id, name, login FROM users WHERE id <> $1", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users, err = scanRows(rows, users)
	if err != nil {
		if users != nil {
			return users, nil
		}
		return nil, err
	}

	return users, nil
}

func (r *Repository) AddToken(ctx context.Context, userID int, token []byte) error {

	_, err := r.pool.Exec(ctx, `INSERT INTO tokens(user_id, token_hash, created_at, expires_at) 
									VALUES ($1, $2, $3, $4)`, userID, token, time.Now(), time.Now().Add(tokenTTL))
	if err != nil {
		return err
	}

	return nil
}

func (r *Repository) CheckToken(ctx context.Context, token []byte) (userID int, err error) {

	row := r.pool.QueryRow(ctx, "SELECT user_id FROM tokens WHERE token_hash=($1) AND expires_at>$2", token, time.Now())

	err = row.Scan(&userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nullID, errors1.ErrInvalidToken
		}
		return userID, err
	}

	_, err = r.pool.Exec(ctx, `UPDATE tokens SET expires_at = $1 WHERE token_hash = $2 AND expires_at < $3`,
		time.Now().Add(tokenTTL), token, time.Now().Add(tokenThreshold))
	if err != nil {
		log.Println(err)
	}

	return userID, nil
}

func (r *Repository) RemoveToken(ctx context.Context, token []byte) error {

	_, err := r.pool.Exec(ctx, `DELETE FROM tokens WHERE token_hash = $1`, token)
	if err != nil {
		return err
	}

	return nil
}

func (r *Repository) GetUsers(ctx context.Context) ([]*model.UserFromDB, error) {

	var users []*model.UserFromDB

	rows, err := r.pool.Query(ctx, "SELECT id, name, login FROM users")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users, err = scanRows(rows, users)
	if err != nil {
		if users != nil {
			return users, nil
		}
		return nil, err
	}

	return users, nil
}

func (r *Repository) GetGroups(ctx context.Context, userID int) ([]model.GroupFromDB, error) {

	rows, err := r.pool.Query(ctx, `SELECT c.id, c.label FROM chats c 
										JOIN users_chats uc ON uc.chat_id = c.id 
										WHERE c.type = 'group' AND uc.user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	groups, err := pgx.CollectRows(rows, pgx.RowToStructByName[model.GroupFromDB])
	if err != nil {
		return nil, err
	}

	return groups, nil
}

func (r *Repository) GetChannels(ctx context.Context, userID int) ([]model.ChannelFromDB, error) {

	rows, err := r.pool.Query(ctx, `SELECT c.id, c.name FROM chats c
										JOIN users_chats uc ON uc.chat_id = c.id 
										WHERE c.type = 'channel' AND uc.user_id = $1`, userID)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	channels, err := pgx.CollectRows(rows, pgx.RowToStructByName[model.ChannelFromDB])
	if err != nil {
		return nil, err
	}

	return channels, err
}

func (r *Repository) CreatePublicChannel(ctx context.Context, ownerID int, name string, description *string, handle string) (int, error) {

	var id int
	var exists bool

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nullID, err
	}

	defer func(tx pgx.Tx, ctx context.Context) {
		_ = tx.Rollback(ctx)
	}(tx, ctx)

	row := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM chats WHERE handle = $1)`, handle)
	if err = row.Scan(&exists); err != nil {
		return nullID, err
	}

	if exists {
		return nullID, errors1.ErrNameInUse
	}

	row = tx.QueryRow(ctx, `INSERT INTO chats(label, type, owner_id, description, handle) VALUES($1, $2, $3, $4, $5) RETURNING id`,
		name, "channel", ownerID, description, handle)
	if err = row.Scan(&id); err != nil {
		return nullID, err
	}

	_, err = tx.Exec(ctx, `INSERT INTO users_chats(chat_id, user_id, role) VALUES($1, $2, $3)`, id, ownerID, chatOwner)
	if err != nil {
		return nullID, err
	}

	return id, tx.Commit(ctx)
}

func (r *Repository) CreatePrivateChannel(ctx context.Context, ownerID int, name string, description *string) (int, error) {

	var id int

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nullID, err
	}
	defer func(tx pgx.Tx, ctx context.Context) {
		_ = tx.Rollback(ctx)
	}(tx, ctx)

	row := tx.QueryRow(ctx, `INSERT INTO chats(label, type, owner_id, description) VALUES($1, $2, $3, $4) RETURNING id`, name, "channel", ownerID, description)
	if err = row.Scan(&id); err != nil {
		return nullID, err
	}

	_, err = tx.Exec(ctx, `INSERT INTO users_chats(chat_id, user_id, role) VALUES($1, $2, $3)`, id, ownerID, chatOwner)
	if err != nil {
		return nullID, err
	}

	return id, tx.Commit(ctx)
}

func (r *Repository) ChatExists(ctx context.Context, from int, to int) (int, bool, error) {

	row := r.pool.QueryRow(ctx,
		`SELECT uc.chat_id
			 FROM users_chats uc
			 JOIN chats c ON c.id = uc.chat_id AND c.type = 'direct'
			 WHERE uc.user_id IN ($1, $2)
			 GROUP BY uc.chat_id
			 HAVING count(*) = 2
			 LIMIT 1`, from, to)

	var chatID int
	if err := row.Scan(&chatID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nullID, false, nil
		}
		return chatID, true, err
	}

	return chatID, true, nil
}

func (r *Repository) StartChat(ctx context.Context, from, to int) (int, error) {

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nullID, err
	}
	defer func(tx pgx.Tx, ctx context.Context) {
		_ = tx.Rollback(ctx)
	}(tx, ctx)

	var chatID int
	if err = tx.QueryRow(ctx, `INSERT INTO chats DEFAULT VALUES RETURNING id`).Scan(&chatID); err != nil {
		return nullID, err
	}

	_, err = tx.Exec(ctx, `INSERT INTO users_chats (chat_id, user_id) VALUES ($1, $2), ($1, $3)`, chatID, from, to)
	if err != nil {
		return nullID, err
	}

	return chatID, tx.Commit(ctx)
}

func (r *Repository) SendMessage(ctx context.Context, send protocol.Send, from int) (*protocol.Message, []int, error) {

	row := r.pool.QueryRow(ctx, `INSERT INTO messages (chat_id, sender_id, client_msg_id, data, created_at)
		SELECT $1, $2, $3, $4, now()
		WHERE EXISTS (SELECT 1 FROM users_chats WHERE chat_id = $1 AND user_id = $2)
		RETURNING id, created_at`,
		send.ChatID, from, send.ClientMsgID, send.Body)

	var msg protocol.Message
	if err := row.Scan(&msg.Id, &msg.Time); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, errors1.ErrNotChatMember
		}
		return nil, nil, err
	}
	msg.ChatID = send.ChatID
	msg.SenderID = from
	msg.Body = send.Body

	rows, err := r.pool.Query(ctx, `SELECT user_id FROM users_chats WHERE chat_id = $1`, send.ChatID)
	if err != nil {
		return nil, nil, err
	}

	ids, err := pgx.CollectRows(rows, pgx.RowTo[int])
	if err != nil {
		return nil, nil, err
	}

	return &msg, ids, nil
}

func (r *Repository) CreateGroup(ctx context.Context, name string, members []int) (int, error) {

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nullID, err
	}
	defer func(tx pgx.Tx, ctx context.Context) {
		_ = tx.Rollback(ctx)
	}(tx, ctx)

	row := tx.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE id = ANY($1)`, members)
	var count int

	if err = row.Scan(&count); err != nil {
		return nullID, err
	}

	if count != len(members) {
		return nullID, errors.New("incorrect list of users")
	}

	var chatID int
	row = tx.QueryRow(ctx, `INSERT INTO chats(label, type) VALUES($1, 'group') RETURNING id`, name)
	if err := row.Scan(&chatID); err != nil {
		return nullID, err
	}

	_, err = tx.Exec(ctx, `INSERT INTO users_chats(chat_id, user_id) SELECT $1, unnest($2::bigint[])`, chatID, members)
	if err != nil {
		return nullID, err
	}

	tx.Commit(ctx)

	return chatID, nil
}

func (r *Repository) LoadMessages(ctx context.Context, chatID, from int) ([]protocol.Message, error) {

	var messages []protocol.Message
	rows, err := r.pool.Query(ctx,
		`SELECT
    			m.id,
    			m.chat_id,
				m.sender_id,
				m.data,
				m.created_at
			 FROM messages m
			 WHERE m.chat_id = $1 AND EXISTS (SELECT 1 FROM users_chats WHERE chat_id = $1 AND user_id = $2)
			 ORDER BY m.created_at, m.id;
			`, chatID, from)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var message protocol.Message
		if err = rows.Scan(
			&message.Id,
			&message.ChatID,
			&message.SenderID,
			&message.Body,
			&message.Time); err != nil {
			return nil, err
		}

		messages = append(messages, message)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return messages, nil
}

func (r *Repository) DeleteExpiredTokens(ctx context.Context) (int64, error) {

	tag, err := r.pool.Exec(ctx, `DELETE FROM tokens WHERE expires_at < $1`, time.Now())
	if err != nil {
		return nullID, err
	}

	return tag.RowsAffected(), nil
}

func scanRows(rows pgx.Rows, users []*model.UserFromDB) ([]*model.UserFromDB, error) {

	for rows.Next() {

		var user model.UserFromDB

		if err := rows.Scan(
			&user.ID,
			&user.Name,
			&user.Login); err != nil {
			log.Println(err)
			return nil, err
		}

		users = append(users, &user)
	}

	if rows.Err() != nil {
		return users, rows.Err()
	}

	return users, nil
}
