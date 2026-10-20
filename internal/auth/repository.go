package auth

import (
	errors1 "chatflow/internal/app-errors"
	"chatflow/internal/model"
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

func NewRepository(pool *pgxpool.Pool) *Repository {
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

func (r *Repository) AddToken(ctx context.Context, userID int, token []byte) error {

	_, err := r.pool.Exec(ctx, `INSERT INTO tokens(user_id, token_hash, created_at, expires_at)
									VALUES ($1, $2, $3, $4)`, userID, token, time.Now(), time.Now().Add(tokenTTL))
	if err != nil {
		return err
	}

	return nil
}

func (r *Repository) RemoveToken(ctx context.Context, token []byte) error {

	_, err := r.pool.Exec(ctx, `DELETE FROM tokens WHERE token_hash = $1`, token)
	if err != nil {
		return err
	}

	return nil
}

// TODO: доступ к функции нужен будет монолиту
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

func isUniqueViolation(err error) bool {

	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation
}
