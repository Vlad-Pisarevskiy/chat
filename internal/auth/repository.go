package auth

import (
	auth "chatflow/gen/auth_service/v1"
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

func (r *Repository) CheckToken(ctx context.Context, token []byte) (int, error) {

	var userID int
	row := r.pool.QueryRow(ctx, "SELECT user_id FROM tokens WHERE token_hash=($1) AND expires_at>$2", token, time.Now())

	err := row.Scan(&userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nullID, ErrInvalidToken
		}
		return nullID, err
	}

	_, err = r.pool.Exec(ctx, `UPDATE tokens SET expires_at = $1 WHERE token_hash = $2 AND expires_at < $3`,
		time.Now().Add(tokenTTL), token, time.Now().Add(tokenThreshold))
	if err != nil {
		slog.Warn("срок токена не продлён", "err", err)
	}

	return userID, nil
}

func (r *Repository) DeleteExpiredTokens(ctx context.Context) (int64, error) {

	tag, err := r.pool.Exec(ctx, `DELETE FROM tokens WHERE expires_at < $1`, time.Now())
	if err != nil {
		return nullID, err
	}

	return tag.RowsAffected(), nil
}

func (r *Repository) FindUserByLogin(ctx context.Context, login string) (*User, error) {

	var user User

	row := r.pool.QueryRow(ctx, "SELECT id, name, login, password FROM users WHERE login=($1)", login)
	if err := row.Scan(
		&user.ID,
		&user.Name,
		&user.Login,
		&user.Password); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrIncorrectLoginData
		}
		return nil, err
	}

	return &user, nil
}

func (r *Repository) GetUsersExcept(ctx context.Context, id int) ([]*auth.UserFromDB, error) {

	var users []*auth.UserFromDB
	// TODO вызов таблицы
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

func scanRows(rows pgx.Rows, users []*auth.UserFromDB) ([]*auth.UserFromDB, error) {

	for rows.Next() {

		var user auth.UserFromDB

		if err := rows.Scan(
			&user.Id,
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

func isUniqueViolation(err error) bool {

	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation
}
