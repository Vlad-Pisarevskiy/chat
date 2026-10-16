package service

import (
	errors1 "chatflow/internal/app-errors"
	"context"
	"regexp"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

func (s *Service) validateRegister(ctx context.Context, user RegisterInput) error {

	if err := correctLogin(user.Login); err != nil {
		return err
	}

	if err := s.db.LoginExists(ctx, user.Login); err != nil {
		return err
	}

	if err := correctName(user.Name); err != nil {
		return err
	}

	if err := correctPassword(user.Password); err != nil {
		return err
	}

	return nil
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

	if len(name) > maxNameLength {
		return errors1.ErrLongName
	}

	return nil
}

func correctLogin(login string) error {

	if login == emptyLogin {
		return errors1.ErrEmptyLogin
	}

	if len(login) < minLoginLength {
		return errors1.ErrShortLogin
	}

	if len(login) > maxLoginLength {
		return errors1.ErrLongLogin
	}

	return nil
}

func correctPassword(password string) error {

	if password == emptyPassword {
		return errors1.ErrEmptyPassword
	}

	if len(password) < minPasswordLength {
		return errors1.ErrShortPassword
	}

	if len(password) > maxPasswordLength {
		return errors1.ErrLongPassword
	}

	return nil
}

func verifyChannelName(name string) error {

	if utf8.RuneCountInString(name) < minNameLength || utf8.RuneCountInString(name) > maxNameLength {
		return errors1.ErrIncorrectNameLength
	}

	return nil
}

func verifyChannelDescription(desc string) error {

	if utf8.RuneCountInString(desc) > maxDescriptionLength {
		return errors1.ErrIncorrectData
	}

	return nil
}

var handleRegexp = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{4,31}$`)

func verifyChannelHandle(handle string) error {

	if !handleRegexp.MatchString(handle) {
		return errors1.ErrInvalidHandle
	}

	return nil
}

func verifyGroup(group GroupCreate) error {

	if len(group.Name) < minNameLength || len(group.Name) > maxNameLength {
		return errors1.ErrIncorrectNameLength
	}

	if len(group.Members) < minMembers {
		return errors1.ErrIncorrectMembersCount
	}

	return nil
}
