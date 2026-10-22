package auth

import (
	errors1 "chatflow/internal/app-errors"
	"context"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

func (s *Service) validateRegister(ctx context.Context, user RegisterInput) error {

	if err := correctName(user.Name); err != nil {
		return err
	}

	if err := correctLogin(user.Login); err != nil {
		return err
	}

	if err := correctPassword(user.Password); err != nil {
		return err
	}

	return s.db.(ctx, user.Login)
}

func hashPassword(password string) (string, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return emptyHash, err
	}
	return string(hashedPassword), nil
}

func correctName(name string) error {

	if name == emptyName {
		return errors1.ErrEmptyName
	}

	if utf8.RuneCountInString(name) > maxNameLength {
		return errors1.ErrLongName
	}

	return nil
}

func correctPassword(password string) error {

	if password == emptyPassword {
		return errors1.ErrEmptyPassword
	}

	length := utf8.RuneCountInString(password)
	if length < minPasswordLength {
		return errors1.ErrShortPassword
	}

	// bcrypt смотрит только на первые 72 байта и на более длинный пароль
	// возвращает ошибку. Кириллица — два байта на букву, поэтому лимит по байтам
	// проверяется отдельно от лимита по символам.
	if length > maxPasswordLength || len(password) > maxPasswordBytes {
		return errors1.ErrLongPassword
	}

	return nil
}
