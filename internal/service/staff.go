package service

import (
	errors1 "chatflow/internal/app-errors"
	"chatflow/internal/model"
	"context"
	"net/http"
	"regexp"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

var loginRegexp = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

var handleRegexp = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{4,31}$`)

// validateRegister сначала проверяет формат и только потом ходит в базу.
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

	return s.db.LoginExists(ctx, user.Login)
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

// verifyName проверяет название группы или канала, уже после trim.
func verifyName(name string) error {

	length := utf8.RuneCountInString(name)
	if length < minNameLength || length > maxNameLength {
		return errors1.ErrIncorrectNameLength
	}

	return nil
}

// verifyBodyLength проверяет длину текста сообщения или подписи, уже после trim.
func verifyBodyLength(body string) error {

	if utf8.RuneCountInString(body) > maxBodyLength {
		return errors1.ErrLongBody
	}

	return nil
}

// checkImage проверяет файл и определяет его формат по содержимому: имя и
// Content-Type части присылает клиент, и оба можно подделать.
func checkImage(data []byte) (model.Image, error) {

	if len(data) == nullLength {
		return model.Image{}, errors1.ErrNoImage
	}

	if len(data) > MaxImageSize {
		return model.Image{}, errors1.ErrImageTooLarge
	}

	contentType := http.DetectContentType(data[:min(len(data), sniffLength)])
	if contentType != contentTypeJPEG && contentType != contentTypePNG {
		return model.Image{}, errors1.ErrBadImageFormat
	}

	return model.Image{ContentType: contentType, Data: data}, nil
}

func verifyChannelDescription(desc string) error {

	if utf8.RuneCountInString(desc) > maxDescriptionLength {
		return errors1.ErrTooLongDescription
	}

	return nil
}

func verifyChannelHandle(handle string) error {

	if !handleRegexp.MatchString(handle) {
		return errors1.ErrInvalidHandle
	}

	return nil
}
