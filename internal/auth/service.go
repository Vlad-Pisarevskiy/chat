package auth

import (
	errors1 "chatflow/internal/app-errors"
	"chatflow/internal/model"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"regexp"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

type User struct {
	ID       int
	Name     string
	Login    string
	Password string
}

type RegisterInput struct {
	Name     string
	Login    string
	Password string
}

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

	user := model.User{
		Name:     input.Name,
		Login:    input.Login,
		Password: hash,
	}

	return s.db.RegisterUser(ctx, user)
}

func (s *Service) LoginUser(ctx context.Context, login, password string) (*model.User, string, error) {

	if err := correctLogin(login); err != nil {
		return nil, emptyToken, errors1.ErrIncorrectLoginData
	}

	user, err := s.db.FindUserByLogin(ctx, login)
	if err != nil {
		return nil, emptyToken, err
	}

	if err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return nil, emptyToken, errors1.ErrIncorrectLoginData
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
		return errors1.ErrEmptyLogin
	}

	if !loginRegexp.MatchString(login) {
		return errors1.ErrLoginFormat
	}

	if len(login) < minLoginLength {
		return errors1.ErrShortLogin
	}

	if len(login) > maxLoginLength {
		return errors1.ErrLongLogin
	}

	return nil
}
