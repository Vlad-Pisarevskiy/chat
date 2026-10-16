package postgres

import (
	errors1 "chatflow/internal/app-errors"
	"chatflow/internal/model"
	"chatflow/internal/protocol"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

// chatAccess — что известно о пользователе в чате: есть ли он там и с какой ролью.
type chatAccess struct {
	chatType string
	member   bool
	role     string
}

func New(pool *pgxpool.Pool) *Repository {

	return &Repository{pool: pool}
}

func (r *Repository) RegisterUser(ctx context.Context, client model.User) error {

	_, err := r.pool.Exec(ctx, "INSERT INTO users(name, login, password) VALUES ($1, $2, $3)",
		client.Name, client.Login, client.Password)
	if err != nil {
		// Два одновременных запроса с одним логином оба проходят LoginExists,
		// второго останавливает только UNIQUE.
		if isUniqueViolation(err) {
			return errors1.ErrExistsLogin
		}
		return fmt.Errorf("регистрация пользователя: %w", err)
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
			return nil, errors1.ErrUserDoesntExist
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
		slog.Warn("срок токена не продлён", "err", err)
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

func (r *Repository) GetGroups(ctx context.Context, userID int) ([]model.GroupFromDB, error) {

	rows, err := r.pool.Query(ctx, `SELECT c.id, c.name FROM chats c
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

	rows, err := r.pool.Query(ctx, `SELECT c.id, c.name, COALESCE(c.description, '') AS description,
										COALESCE(uc.role, 'member') AS role
										FROM chats c
										JOIN users_chats uc ON uc.chat_id = c.id
										WHERE c.type = 'channel' AND uc.user_id = $1`, userID)
	if err != nil {
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

	row = tx.QueryRow(ctx, `INSERT INTO chats(name, type, owner_id, description, handle) VALUES($1, $2, $3, $4, $5) RETURNING id`,
		name, "channel", ownerID, description, handle)
	if err = row.Scan(&id); err != nil {
		// Проверка выше не спасает от двух одновременных запросов с одним handle.
		if isUniqueViolation(err) {
			return nullID, errors1.ErrNameInUse
		}
		return nullID, fmt.Errorf("создание канала: %w", err)
	}

	_, err = tx.Exec(ctx, `INSERT INTO users_chats(chat_id, user_id, role) VALUES($1, $2, $3)`, id, ownerID, ownerRole)
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

	row := tx.QueryRow(ctx, `INSERT INTO chats(name, type, owner_id, description) VALUES($1, $2, $3, $4) RETURNING id`, name, "channel", ownerID, description)
	if err = row.Scan(&id); err != nil {
		return nullID, err
	}

	_, err = tx.Exec(ctx, `INSERT INTO users_chats(chat_id, user_id, role) VALUES($1, $2, $3)`, id, ownerID, ownerRole)
	if err != nil {
		return nullID, err
	}

	return id, tx.Commit(ctx)
}

func (r *Repository) GetChannel(ctx context.Context, handle string) (*model.ChannelFromDB, error) {

	var ch model.ChannelFromDB
	row := r.pool.QueryRow(ctx, `SELECT id, name, COALESCE(description, '') FROM chats
									 WHERE handle = $1 AND type = 'channel'`, handle)

	if err := row.Scan(&ch.ID, &ch.Name, &ch.Description); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors1.ErrChatNotFound
		}
		return nil, fmt.Errorf("поиск канала: %w", err)
	}

	return &ch, nil
}

// FindDirect ищет личный чат пары, ничего не создавая.
func (r *Repository) FindDirect(ctx context.Context, userID, peerID int) (int, bool, error) {

	low, high := orderedPair(userID, peerID)

	var chatID int
	err := r.pool.QueryRow(ctx, `SELECT chat_id FROM direct_chats WHERE user_low = $1 AND user_high = $2`,
		low, high).Scan(&chatID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nullID, false, nil
		}
		return nullID, false, fmt.Errorf("поиск личного чата: %w", err)
	}

	return chatID, true, nil
}

// GetOrCreateDirect возвращает личный чат пары и сообщает, создан ли он сейчас.
// Одновременные вызовы для одной пары сходятся в один чат: второй упирается в
// первичный ключ direct_chats и берёт чат первого.
func (r *Repository) GetOrCreateDirect(ctx context.Context, userID, peerID int) (int, bool, error) {

	chatID, ok, err := r.FindDirect(ctx, userID, peerID)
	if err != nil || ok {
		return chatID, false, err
	}

	chatID, created, err := r.createDirect(ctx, userID, peerID)
	if err != nil || created {
		return chatID, created, err
	}

	// Пару успели создать параллельно: наша транзакция откатилась, берём чат,
	// который создал другой запрос.
	chatID, ok, err = r.FindDirect(ctx, userID, peerID)
	if err != nil {
		return nullID, false, err
	}
	if !ok {
		return nullID, false, fmt.Errorf("личный чат %d-%d пропал сразу после создания", userID, peerID)
	}

	return chatID, false, nil
}

// createDirect заводит личный чат. created = false значит, что пару в это же
// время создал другой запрос, и транзакция откатилась.
func (r *Repository) createDirect(ctx context.Context, userID, peerID int) (int, bool, error) {

	low, high := orderedPair(userID, peerID)

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nullID, false, fmt.Errorf("создание личного чата: %w", err)
	}
	defer func(tx pgx.Tx, ctx context.Context) {
		_ = tx.Rollback(ctx)
	}(tx, ctx)

	var chatID int
	if err = tx.QueryRow(ctx, `INSERT INTO chats DEFAULT VALUES RETURNING id`).Scan(&chatID); err != nil {
		return nullID, false, fmt.Errorf("создание личного чата: %w", err)
	}

	_, err = tx.Exec(ctx, `INSERT INTO users_chats (chat_id, user_id) VALUES ($1, $2), ($1, $3)`, chatID, low, high)
	if err != nil {
		if isForeignKeyViolation(err) {
			return nullID, false, errors1.ErrUserDoesntExist
		}
		return nullID, false, fmt.Errorf("участники личного чата: %w", err)
	}

	err = tx.QueryRow(ctx, `INSERT INTO direct_chats (user_low, user_high, chat_id) VALUES ($1, $2, $3)
								ON CONFLICT (user_low, user_high) DO NOTHING RETURNING chat_id`,
		low, high, chatID).Scan(&chatID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nullID, false, nil
		}
		return nullID, false, fmt.Errorf("пара личного чата: %w", err)
	}

	if err = tx.Commit(ctx); err != nil {
		return nullID, false, fmt.Errorf("создание личного чата: %w", err)
	}

	return chatID, true, nil
}

// SendMessage сохраняет сообщение и возвращает получателей. Повтор с той же
// парой (отправитель, client_msg_id) ничего не вставляет: возвращается
// сохранённое раньше сообщение и duplicate = true.
func (r *Repository) SendMessage(ctx context.Context, send protocol.Send, from int) (*protocol.Message, []int, bool, error) {

	msg := protocol.Message{ChatID: send.ChatID, SenderID: from, Body: send.Body}

	err := r.pool.QueryRow(ctx, `INSERT INTO messages (chat_id, sender_id, client_msg_id, body, created_at)
		SELECT $1, $2, $3, $4, now()
		WHERE EXISTS (SELECT 1 FROM users_chats uc
			JOIN chats c ON c.id = uc.chat_id
			WHERE uc.chat_id = $1 AND uc.user_id = $2
			AND (c.type <> 'channel' OR uc.role IN ('owner', 'admin')))
		ON CONFLICT (sender_id, client_msg_id) DO NOTHING
		RETURNING id, created_at`,
		send.ChatID, from, send.ClientMsgID, send.Body).Scan(&msg.Id, &msg.Time)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, false, fmt.Errorf("сохранение сообщения: %w", err)
		}
		return r.notInserted(ctx, send, from)
	}
	msg.Time = msg.Time.UTC()

	_, err = r.pool.Exec(ctx, `UPDATE users_chats SET last_read = $1
									WHERE chat_id = $2 AND user_id = $3`, msg.Id, msg.ChatID, msg.SenderID)
	if err != nil {
		slog.Warn("last_read отправителя не обновлён", "chat", msg.ChatID, "err", err)
	}

	rows, err := r.pool.Query(ctx, `SELECT user_id FROM users_chats WHERE chat_id = $1`, send.ChatID)
	if err != nil {
		return nil, nil, false, fmt.Errorf("получатели сообщения: %w", err)
	}

	ids, err := pgx.CollectRows(rows, pgx.RowTo[int])
	if err != nil {
		return nil, nil, false, fmt.Errorf("получатели сообщения: %w", err)
	}

	return &msg, ids, false, nil
}

// notInserted выясняет, почему INSERT в SendMessage ничего не вставил: это
// повтор уже сохранённого сообщения или у отправителя нет права писать в чат.
func (r *Repository) notInserted(ctx context.Context, send protocol.Send, from int) (*protocol.Message, []int, bool, error) {

	msg := protocol.Message{SenderID: from}
	err := r.pool.QueryRow(ctx, `SELECT id, chat_id, body, created_at FROM messages
									 WHERE sender_id = $1 AND client_msg_id = $2`,
		from, send.ClientMsgID).Scan(&msg.Id, &msg.ChatID, &msg.Body, &msg.Time)
	if err == nil {
		msg.Time = msg.Time.UTC()
		return &msg, nil, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, false, fmt.Errorf("поиск повтора сообщения: %w", err)
	}

	if _, err = r.requireMember(ctx, send.ChatID, from); err != nil {
		return nil, nil, false, err
	}

	return nil, nil, false, errors1.ErrNoWriteAccess
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
		return nullID, errors1.ErrUnknownMembers
	}

	var chatID int
	row = tx.QueryRow(ctx, `INSERT INTO chats(name, type) VALUES($1, 'group') RETURNING id`, name)
	if err := row.Scan(&chatID); err != nil {
		return nullID, err
	}

	_, err = tx.Exec(ctx, `INSERT INTO users_chats(chat_id, user_id) SELECT $1, unnest($2::bigint[])`, chatID, members)
	if err != nil {
		return nullID, err
	}

	if err = tx.Commit(ctx); err != nil {
		return nullID, fmt.Errorf("создание группы: %w", err)
	}

	return chatID, nil
}

func (r *Repository) LoadMessages(ctx context.Context, chatID, from int) ([]protocol.Message, error) {

	if _, err := r.requireMember(ctx, chatID, from); err != nil {
		return nil, err
	}

	rows, err := r.pool.Query(ctx,
		`SELECT m.id, m.chat_id, m.sender_id, m.body, m.created_at
			 FROM messages m
			 JOIN users_chats uc ON uc.chat_id = m.chat_id AND uc.user_id = $2
			 WHERE m.chat_id = $1 AND m.id > COALESCE(uc.border, 0)
			 ORDER BY m.id`, chatID, from)
	if err != nil {
		return nil, fmt.Errorf("история чата: %w", err)
	}
	defer rows.Close()

	// Не nil: пустой слайс кодируется в [], а nil — в null, на котором падает фронт.
	messages := make([]protocol.Message, nullLength)
	var lastID int

	for rows.Next() {
		var message protocol.Message
		if err = rows.Scan(
			&message.Id,
			&message.ChatID,
			&message.SenderID,
			&message.Body,
			&message.Time); err != nil {
			return nil, fmt.Errorf("история чата: %w", err)
		}

		message.Time = message.Time.UTC()
		messages = append(messages, message)
		lastID = message.Id
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("история чата: %w", err)
	}

	if len(messages) == nullLength {
		return messages, nil
	}

	_, err = r.pool.Exec(ctx, `UPDATE users_chats
								   SET last_read = $1
								   WHERE chat_id=$2 AND user_id = $3`, lastID, chatID, from)
	if err != nil {
		slog.Warn("last_read не обновлён", "chat", chatID, "err", err)
	}

	return messages, nil
}

// ClearChat стирает личную переписку у одного участника: запоминает границу,
// не новее которой история ему больше не отдаётся. Возвращает границу.
func (r *Repository) ClearChat(ctx context.Context, chatID, userID int) (int, error) {

	access, err := r.requireMember(ctx, chatID, userID)
	if err != nil {
		return nullID, err
	}

	if access.chatType != directType {
		return nullID, errors1.ErrNotDirectChat
	}

	// Граница — последнее сообщение чата на момент запроса. GREATEST: повторная
	// очистка двигает границу только вперёд.
	var border int
	err = r.pool.QueryRow(ctx, `UPDATE users_chats
		SET border = GREATEST(COALESCE(border, 0), (SELECT COALESCE(MAX(id), 0) FROM messages WHERE chat_id = $1))
		WHERE chat_id = $1 AND user_id = $2
		RETURNING border`, chatID, userID).Scan(&border)
	if err != nil {
		return nullID, fmt.Errorf("очистка чата: %w", err)
	}

	return border, nil
}

func (r *Repository) DeleteMessage(ctx context.Context, userID, messageID, chatID int) error {

	_, err := r.pool.Exec(ctx, `DELETE FROM messages WHERE chat_id = $1 AND sender_id = $2 AND id = $3`,
		chatID, userID, messageID)

	if err != nil {
		return err
	}

	return nil
}

// DeleteChat удаляет личный чат у обоих участников вместе с сообщениями и
// возвращает id участников: им сервер разошлёт chat_removed.
func (r *Repository) DeleteChat(ctx context.Context, chatID, userID int) ([]int, error) {

	access, err := r.requireMember(ctx, chatID, userID)
	if err != nil {
		return nil, err
	}

	if access.chatType != directType {
		return nil, errors1.ErrNotDirectChat
	}

	// Основной запрос видит снимок до DELETE, поэтому участники находятся, хотя
	// их строки удаляются каскадом в том же запросе.
	rows, err := r.pool.Query(ctx, `WITH deleted AS (DELETE FROM chats WHERE id = $1 RETURNING id)
		SELECT uc.user_id FROM users_chats uc JOIN deleted d ON d.id = uc.chat_id`, chatID)
	if err != nil {
		return nil, fmt.Errorf("удаление чата: %w", err)
	}

	members, err := pgx.CollectRows(rows, pgx.RowTo[int])
	if err != nil {
		return nil, fmt.Errorf("удаление чата: %w", err)
	}

	// Пусто — чат удалил второй участник, пока шёл этот запрос.
	if len(members) == nullLength {
		return nil, errors1.ErrChatNotFound
	}

	return members, nil
}

func (r *Repository) JoinChannel(ctx context.Context, userID, channelID int) (*model.ChannelFromDB, error) {

	channel := model.ChannelFromDB{ID: channelID}
	var public, member bool

	row := r.pool.QueryRow(ctx, `SELECT c.name, c.handle IS NOT NULL, uc.user_id IS NOT NULL, COALESCE(uc.role, 'member')
									 FROM chats c
									 LEFT JOIN users_chats uc ON uc.chat_id = c.id AND uc.user_id = $2
									 WHERE c.id = $1 AND c.type = 'channel'`, channelID, userID)
	if err := row.Scan(&channel.Name, &public, &member, &channel.Role); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors1.ErrChatNotFound
		}
		return nil, fmt.Errorf("вступление в канал: %w", err)
	}

	// Уже в канале: отвечаем текущей ролью, так повторный join не падает
	if member {
		return &channel, nil
	}

	// У приватного канала нет handle, вступить в него без приглашения нельзя
	if !public {
		return nil, errors1.ErrChatNotFound
	}

	_, err := r.pool.Exec(ctx, `INSERT INTO users_chats(chat_id, user_id, role) VALUES ($1, $2, $3)
									ON CONFLICT (user_id, chat_id) DO NOTHING`, channelID, userID, memberRole)
	if err != nil {
		return nil, fmt.Errorf("вступление в канал: %w", err)
	}

	channel.Role = memberRole

	return &channel, nil
}

func (r *Repository) DeleteExpiredTokens(ctx context.Context) (int64, error) {

	tag, err := r.pool.Exec(ctx, `DELETE FROM tokens WHERE expires_at < $1`, time.Now())
	if err != nil {
		return nullID, err
	}

	return tag.RowsAffected(), nil
}

// chatAccess узнаёт тип чата и место пользователя в нём. Чата нет — ErrChatNotFound.
func (r *Repository) chatAccess(ctx context.Context, chatID, userID int) (chatAccess, error) {

	var access chatAccess

	err := r.pool.QueryRow(ctx, `SELECT c.type, uc.user_id IS NOT NULL, COALESCE(uc.role, '')
									 FROM chats c
									 LEFT JOIN users_chats uc ON uc.chat_id = c.id AND uc.user_id = $2
									 WHERE c.id = $1`, chatID, userID).Scan(&access.chatType, &access.member, &access.role)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return access, errors1.ErrChatNotFound
		}
		return access, fmt.Errorf("доступ к чату: %w", err)
	}

	return access, nil
}

// requireMember пропускает дальше, только если чат есть и пользователь в нём состоит.
func (r *Repository) requireMember(ctx context.Context, chatID, userID int) (chatAccess, error) {

	access, err := r.chatAccess(ctx, chatID, userID)
	if err != nil {
		return access, err
	}

	if !access.member {
		return access, errors1.ErrNotChatMember
	}

	return access, nil
}

func scanRows(rows pgx.Rows, users []*model.UserFromDB) ([]*model.UserFromDB, error) {

	for rows.Next() {

		var user model.UserFromDB

		if err := rows.Scan(
			&user.ID,
			&user.Name,
			&user.Login); err != nil {
			return nil, err
		}

		users = append(users, &user)
	}

	if rows.Err() != nil {
		return users, rows.Err()
	}

	return users, nil
}

// orderedPair упорядочивает пару, чтобы (A, B) и (B, A) были одной строкой direct_chats.
func orderedPair(a, b int) (int, int) {

	if a < b {
		return a, b
	}

	return b, a
}

func isUniqueViolation(err error) bool {

	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation
}

func isForeignKeyViolation(err error) bool {

	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolation
}
