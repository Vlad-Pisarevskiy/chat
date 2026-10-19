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

// queryer — то, что общо у пула и транзакции: один и тот же запрос можно
// выполнить и вне транзакции, и внутри неё.
type queryer interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
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

	msg, inserted, err := r.insertMessage(ctx, r.pool, send, from, nil)
	if err != nil {
		return nil, nil, false, err
	}
	if !inserted {
		return r.notInserted(ctx, send, from)
	}

	return r.delivered(ctx, msg)
}

// SendImage сохраняет сообщение с картинкой. Картинка и сообщение пишутся в
// одной транзакции: картинки без сообщения не остаётся ни при повторе, ни при
// отказе в правах, ни при гонке двух одинаковых запросов. Остальное, как у
// SendMessage: получатели и duplicate.
func (r *Repository) SendImage(ctx context.Context, send protocol.Send, image model.Image, from int) (*protocol.Message, []int, bool, error) {

	// Повтор и отказ выясняем до записи байтов: ради них пять мегабайт в базу
	// писать незачем.
	msg, duplicate, err := r.findSent(ctx, r.pool, send.ClientMsgID, from)
	if err != nil {
		return nil, nil, false, err
	}
	if duplicate {
		return msg, nil, true, nil
	}

	allowed, err := r.canWrite(ctx, send.ChatID, from)
	if err != nil {
		return nil, nil, false, err
	}
	if !allowed {
		return nil, nil, false, r.denied(ctx, send.ChatID, from)
	}

	msg, inserted, err := r.insertImageMessage(ctx, send, image, from)
	if err != nil {
		return nil, nil, false, err
	}
	if !inserted {
		// Между проверкой и записью успел такой же запрос или у отправителя
		// отняли право: транзакция откатилась вместе с картинкой.
		return r.notInserted(ctx, send, from)
	}

	return r.delivered(ctx, msg)
}

// insertImageMessage пишет картинку и сообщение в одной транзакции. inserted =
// false значит, что сообщение не вставилось, и откатилась в том числе картинка.
func (r *Repository) insertImageMessage(ctx context.Context, send protocol.Send, image model.Image, from int) (*protocol.Message, bool, error) {

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("сохранение картинки: %w", err)
	}
	defer func(tx pgx.Tx, ctx context.Context) {
		_ = tx.Rollback(ctx)
	}(tx, ctx)

	var imageID int
	err = tx.QueryRow(ctx, `INSERT INTO images (chat_id, content_type, data) VALUES ($1, $2, $3) RETURNING id`,
		send.ChatID, image.ContentType, image.Data).Scan(&imageID)
	if err != nil {
		// Чат удалили между проверкой права и записью.
		if isForeignKeyViolation(err) {
			return nil, false, errors1.ErrChatNotFound
		}
		return nil, false, fmt.Errorf("сохранение картинки: %w", err)
	}

	msg, inserted, err := r.insertMessage(ctx, tx, send, from, &imageID)
	if err != nil || !inserted {
		return nil, false, err
	}

	if err = tx.Commit(ctx); err != nil {
		return nil, false, fmt.Errorf("сохранение картинки: %w", err)
	}

	return msg, true, nil
}

// insertMessage вставляет сообщение, если у отправителя есть право писать в
// чат и такого client_msg_id ещё не было. imageID — картинка сообщения или nil.
func (r *Repository) insertMessage(ctx context.Context, q queryer, send protocol.Send, from int, imageID *int) (*protocol.Message, bool, error) {

	msg := protocol.Message{ChatID: send.ChatID, SenderID: from, Body: send.Body}
	if imageID != nil {
		msg.ImageID = *imageID
	}

	err := q.QueryRow(ctx, `INSERT INTO messages (chat_id, sender_id, client_msg_id, body, created_at, image_id)
		SELECT $1, $2, $3, $4, now(), $5::bigint
		WHERE `+writeAllowedSQL+`
		ON CONFLICT (sender_id, client_msg_id) DO NOTHING
		RETURNING id, created_at`,
		send.ChatID, from, send.ClientMsgID, send.Body, imageID).Scan(&msg.Id, &msg.Time)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("сохранение сообщения: %w", err)
	}
	msg.Time = msg.Time.UTC()

	return &msg, true, nil
}

// delivered отмечает сообщение прочитанным у отправителя и находит получателей.
func (r *Repository) delivered(ctx context.Context, msg *protocol.Message) (*protocol.Message, []int, bool, error) {

	_, err := r.pool.Exec(ctx, `UPDATE users_chats SET last_read = $1
									WHERE chat_id = $2 AND user_id = $3`, msg.Id, msg.ChatID, msg.SenderID)
	if err != nil {
		slog.Warn("last_read отправителя не обновлён", "chat", msg.ChatID, "err", err)
	}

	rows, err := r.pool.Query(ctx, `SELECT user_id FROM users_chats WHERE chat_id = $1`, msg.ChatID)
	if err != nil {
		return nil, nil, false, fmt.Errorf("получатели сообщения: %w", err)
	}

	ids, err := pgx.CollectRows(rows, pgx.RowTo[int])
	if err != nil {
		return nil, nil, false, fmt.Errorf("получатели сообщения: %w", err)
	}

	return msg, ids, false, nil
}

// canWrite сообщает, может ли пользователь писать в чат.
func (r *Repository) canWrite(ctx context.Context, chatID, userID int) (bool, error) {

	var allowed bool
	if err := r.pool.QueryRow(ctx, `SELECT `+writeAllowedSQL, chatID, userID).Scan(&allowed); err != nil {
		return false, fmt.Errorf("право писать в чат: %w", err)
	}

	return allowed, nil
}

// findSent ищет сохранённое сообщение по паре (отправитель, client_msg_id).
func (r *Repository) findSent(ctx context.Context, q queryer, clientMsgID string, from int) (*protocol.Message, bool, error) {

	msg := protocol.Message{SenderID: from}
	err := q.QueryRow(ctx, `SELECT id, chat_id, body, COALESCE(image_id, 0), created_at FROM messages
								WHERE sender_id = $1 AND client_msg_id = $2`,
		from, clientMsgID).Scan(&msg.Id, &msg.ChatID, &msg.Body, &msg.ImageID, &msg.Time)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("поиск повтора сообщения: %w", err)
	}
	msg.Time = msg.Time.UTC()

	return &msg, true, nil
}

// denied объясняет, почему пользователю нельзя писать в чат.
func (r *Repository) denied(ctx context.Context, chatID, userID int) error {

	if _, err := r.requireMember(ctx, chatID, userID); err != nil {
		return err
	}

	return errors1.ErrNoWriteAccess
}

// notInserted выясняет, почему INSERT сообщения ничего не вставил: это повтор
// уже сохранённого сообщения или у отправителя нет права писать в чат.
func (r *Repository) notInserted(ctx context.Context, send protocol.Send, from int) (*protocol.Message, []int, bool, error) {

	msg, duplicate, err := r.findSent(ctx, r.pool, send.ClientMsgID, from)
	if err != nil {
		return nil, nil, false, err
	}
	if duplicate {
		return msg, nil, true, nil
	}

	return nil, nil, false, r.denied(ctx, send.ChatID, from)
}

// GetImage отдаёт картинку, если пользователь вправе её смотреть. Не вправе и
// не существует — одна и та же ErrImageNotFound: иначе по id можно было бы
// проверить, есть ли чужая картинка.
func (r *Repository) GetImage(ctx context.Context, imageID, userID int) (*model.Image, error) {

	var chatID int
	err := r.pool.QueryRow(ctx, `SELECT chat_id FROM images WHERE id = $1`, imageID).Scan(&chatID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors1.ErrImageNotFound
		}
		return nil, fmt.Errorf("поиск картинки: %w", err)
	}

	// Тот же доступ, что у истории чата.
	if _, err = r.requireRead(ctx, chatID, userID); err != nil {
		if errors.Is(err, errors1.ErrNotChatMember) || errors.Is(err, errors1.ErrChatNotFound) {
			return nil, errors1.ErrImageNotFound
		}
		return nil, err
	}

	var image model.Image
	err = r.pool.QueryRow(ctx, `SELECT content_type, data FROM images WHERE id = $1`, imageID).
		Scan(&image.ContentType, &image.Data)
	if err != nil {
		// Чат удалили между запросами.
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors1.ErrImageNotFound
		}
		return nil, fmt.Errorf("чтение картинки: %w", err)
	}

	return &image, nil
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

// LoadMessages отдаёт пачку истории по возрастанию id: limit сообщений с id
// меньше before (nil — без верхней границы), самых новых среди них. Граница
// очистки действует в любой пачке.
func (r *Repository) LoadMessages(ctx context.Context, chatID, from, limit int, before *int) ([]protocol.Message, error) {

	if _, err := r.requireRead(ctx, chatID, from); err != nil {
		return nil, err
	}

	// Байты картинок сюда не попадают: в выборке только image_id. LEFT JOIN:
	// строка users_chats нужна только ради границы очистки, доступ уже решил
	// requireRead. Внутри — пачка от новых к старым (по индексу chat_id, id),
	// снаружи — порядок по возрастанию.
	rows, err := r.pool.Query(ctx,
		`SELECT id, chat_id, sender_id, body, image_id, created_at FROM (
				 SELECT m.id, m.chat_id, m.sender_id, m.body, COALESCE(m.image_id, 0) AS image_id, m.created_at
				 FROM messages m
				 LEFT JOIN users_chats uc ON uc.chat_id = m.chat_id AND uc.user_id = $2
				 WHERE m.chat_id = $1 AND m.id > COALESCE(uc.border, 0)
				   AND ($3::bigint IS NULL OR m.id < $3)
				 ORDER BY m.id DESC
				 LIMIT $4
			 ) page
			 ORDER BY id`, chatID, from, before, limit)
	if err != nil {
		return nil, fmt.Errorf("история чата: %w", err)
	}
	defer rows.Close()

	// Не nil: пустой слайс кодируется в [], а nil — в null, на котором падает фронт.
	messages := make([]protocol.Message, nullLength)
	for rows.Next() {
		var message protocol.Message
		if err = rows.Scan(
			&message.Id,
			&message.ChatID,
			&message.SenderID,
			&message.Body,
			&message.ImageID,
			&message.Time); err != nil {
			return nil, fmt.Errorf("история чата: %w", err)
		}

		message.Time = message.Time.UTC()
		messages = append(messages, message)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("история чата: %w", err)
	}

	if len(messages) == nullLength {
		return messages, nil
	}

	// Пачка идёт по возрастанию, так что последнее сообщение — самое новое в ней.
	// GREATEST: старая пачка при прокрутке вверх не откатывает last_read назад.
	lastID := messages[len(messages)-1].Id

	_, err = r.pool.Exec(ctx, `UPDATE users_chats
								   SET last_read = GREATEST(COALESCE(last_read, 0), $1)
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

// DeleteMessage удаляет сообщение вместе с его картинкой и возвращает id
// участников чата: им сервер разошлёт message_deleted. Проверки идут по
// порядку: чат есть, пользователь в нём состоит, сообщение лежит в этом чате,
// права на него есть.
//
// Всё — одна транзакция. Строка чата блокируется первой и на общее чтение
// ключа: удаление чата каскадом берёт строки участников и сообщений в своём
// порядке, и без этой блокировки встречные порядки давали бы взаимную
// блокировку. Теперь DeleteChat ждёт, пока мы закончим. Дальше строка
// сообщения блокируется на время проверки, поэтому два одновременных удаления
// выстраиваются в очередь, и второе не находит сообщения. Роли двух участников
// блокируются на общее чтение: пока удаляем, их не поменяют, и удаляется
// только то, на что права были на самом деле.
func (r *Repository) DeleteMessage(ctx context.Context, userID, messageID, chatID int) ([]int, error) {

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("удаление сообщения: %w", err)
	}
	defer func(tx pgx.Tx, ctx context.Context) {
		_ = tx.Rollback(ctx)
	}(tx, ctx)

	var chatType string
	err = tx.QueryRow(ctx, `SELECT type FROM chats WHERE id = $1 FOR KEY SHARE`, chatID).Scan(&chatType)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors1.ErrChatNotFound
		}
		return nil, fmt.Errorf("поиск чата: %w", err)
	}

	myRole, member, err := lockRole(ctx, tx, chatID, userID)
	if err != nil {
		return nil, err
	}
	if !member {
		return nil, errors1.ErrNotChatMember
	}

	var authorID int
	err = tx.QueryRow(ctx, `SELECT sender_id FROM messages WHERE id = $1 AND chat_id = $2 FOR UPDATE`,
		messageID, chatID).Scan(&authorID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors1.ErrMessageNotFound
		}
		return nil, fmt.Errorf("поиск сообщения: %w", err)
	}

	// Автор, которого нет среди участников, вышел или исключён: он считается member.
	authorRole := myRole
	if authorID != userID {
		var inChat bool
		authorRole, inChat, err = lockRole(ctx, tx, chatID, authorID)
		if err != nil {
			return nil, err
		}
		if !inChat {
			authorRole = memberRole
		}
	}

	if !canDelete(chatType, myRole, authorRole, userID == authorID) {
		return nil, errors1.ErrNoDeleteAccess
	}

	var imageID *int
	err = tx.QueryRow(ctx, `DELETE FROM messages WHERE id = $1 AND chat_id = $2 RETURNING image_id`,
		messageID, chatID).Scan(&imageID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors1.ErrMessageNotFound
		}
		return nil, fmt.Errorf("удаление сообщения: %w", err)
	}

	// Внешний ключ лишь обнуляет messages.image_id, так что картинка сама не
	// исчезнет: без этого байты остались бы в базе без сообщения.
	if imageID != nil {
		if _, err = tx.Exec(ctx, `DELETE FROM images WHERE id = $1`, *imageID); err != nil {
			return nil, fmt.Errorf("удаление картинки сообщения: %w", err)
		}
	}

	rows, err := tx.Query(ctx, `SELECT user_id FROM users_chats WHERE chat_id = $1`, chatID)
	if err != nil {
		return nil, fmt.Errorf("получатели удаления: %w", err)
	}

	members, err := pgx.CollectRows(rows, pgx.RowTo[int])
	if err != nil {
		return nil, fmt.Errorf("получатели удаления: %w", err)
	}

	if err = tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("удаление сообщения: %w", err)
	}

	return members, nil
}

// lockRole читает роль пользователя в чате и блокирует его строку на общее
// чтение до конца транзакции. Нет строки — пользователя в чате нет.
func lockRole(ctx context.Context, tx pgx.Tx, chatID, userID int) (string, bool, error) {

	var role string
	err := tx.QueryRow(ctx, `SELECT COALESCE(role, 'member') FROM users_chats
		WHERE chat_id = $1 AND user_id = $2
		FOR SHARE`, chatID, userID).Scan(&role)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("роль участника: %w", err)
	}

	return role, true, nil
}

// canDelete — единственное место правила «кто какие сообщения удаляет». Роли
// берутся на момент удаления. В личке и у рядового участника группы — только
// свои; админ группы удаляет ещё и сообщения рядовых участников; в канале
// рядовой участник не удаляет ничего, а админ — всё, кроме постов владельца.
// Владелец удаляет любые.
func canDelete(chatType, myRole, authorRole string, mine bool) bool {

	if myRole == ownerRole && chatType != directType {
		return true
	}

	switch chatType {
	case directType:
		return mine
	case groupType:
		return mine || (myRole == adminRole && authorRole == memberRole)
	case channelType:
		return myRole == adminRole && authorRole != ownerRole
	}

	return false
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

// canRead — единственное место правила «кто читает историю чата и смотрит его
// картинки». Сейчас читают только участники.
func (a chatAccess) canRead() bool {

	return a.member
}

// requireRead пропускает дальше, только если чат есть и пользователь вправе его читать.
func (r *Repository) requireRead(ctx context.Context, chatID, userID int) (chatAccess, error) {

	access, err := r.chatAccess(ctx, chatID, userID)
	if err != nil {
		return access, err
	}

	if !access.canRead() {
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
