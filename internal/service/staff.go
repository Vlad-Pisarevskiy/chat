package service

import (
	errors1 "chatflow/internal/app-errors"
	"chatflow/internal/model"
	"net/http"
	"regexp"
	"unicode/utf8"
)

var handleRegexp = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{4,31}$`)

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

// verifyHistoryLimit проверяет размер пачки истории. Лишнее не урезается молча:
// фронт по короткой пачке решает, что история кончилась.
func verifyHistoryLimit(limit int) error {

	if limit < 1 || limit > maxHistoryLimit {
		return errors1.ErrBadLimit
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
