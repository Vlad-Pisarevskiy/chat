package auth

import (
	auth "chatflow/gen/auth_service/v1"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"regexp"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

var loginRegexp = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

type Service struct {
	db *Repository
}

func NewService(repository *Repository) *Service {
	return &Service{db: repository}
}

func (s *Service) RegisterUser(ctx context.Context, input RegisterInput) error {

	input.Name = strings.TrimSpace(input.Name)

	if err := s.validateRegister(ctx, input); err != nil {
		return err
	}

	hash, err := hashPassword(input.Password)
	if err != nil {
		return err
	}

	user := User{
		Name:     input.Name,
		Login:    input.Login,
		Password: hash,
	}

	return s.db.RegisterUser(ctx, user)
}

func (s *Service) GetUserExcept(ctx context.Context, id int) ([]*auth.UserFromDB, error) {

	return s.db.GetUsersExcept(ctx, id)
}

func (s *Service) CheckToken(ctx context.Context, token string) (int, error) {

	tokenHash := sha256.Sum256([]byte(token))
	id, err := s.db.CheckToken(ctx, tokenHash[:])
	if err != nil {
		return nullID, err
	}

	return id, nil
}

func (s *Service) CleanupTokens(ctx context.Context) (int64, error) {

	return s.db.DeleteExpiredTokens(ctx)

}

func (s *Service) LoginUser(ctx context.Context, login, password string) (*User, string, error) {

	if err := correctLogin(login); err != nil {
		return nil, emptyToken, ErrIncorrectLoginData
	}

	user, err := s.db.FindUserByLogin(ctx, login)
	if err != nil {
		return nil, emptyToken, err
	}

	if err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return nil, emptyToken, ErrIncorrectLoginData
	}

	token := rand.Text()
	tokenHash := sha256.Sum256([]byte(token))

	if err = s.db.AddToken(ctx, user.ID, tokenHash[:]); err != nil {
		return nil, emptyToken, err
	}

	return user, token, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {

	tokenHash := sha256.Sum256([]byte(token))
	if err := s.db.RemoveToken(ctx, tokenHash[:]); err != nil {
		return err
	}

	return nil
}

func correctLogin(login string) error {

	if login == emptyLogin {
		return ErrEmptyLogin
	}

	if !loginRegexp.MatchString(login) {
		return ErrLoginFormat
	}

	if len(login) < minLoginLength {
		return ErrShortLogin
	}

	if len(login) > maxLoginLength {
		return ErrLongLogin
	}

	return nil
}
