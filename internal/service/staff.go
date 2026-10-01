package service

import (
	errors1 "chatflow/internal/app-errors"
	"context"
	"fmt"
	"regexp"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

type RegisterInput struct {
	Name     string
	Login    string
	Password string
}

type GroupCreate struct {
	OwnerID int    `json:"-"`
	Name    string `json:"name"`
	Members []int  `json:"member_ids"`
}

type ChannelCreates struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
	Handle      *string `json:"handle"`
}

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

func verifyChannelHandle(handle string) error {

	re, err := regexp.Compile(fmt.Sprintf(`^[A-Za-z][A-Za-z0-9_]{%d, %d}$`,
		minHandleLength, maxHandleLength))
	if err != nil {
		return err
	}

	if re.MatchString(handle) {
		return nil
	}

	if len(handle) > maxHandleLength {
		return errors1.ErrLongHandle
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
