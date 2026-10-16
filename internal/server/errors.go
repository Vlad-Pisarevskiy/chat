package server

import (
	errors1 "chatflow/internal/app-errors"
	"chatflow/internal/protocol"
	"errors"
	"net/http"
)

// failure — как ошибка выглядит снаружи: HTTP-статус, код кадра error и текст
// для человека.
type failure struct {
	status int
	code   string
	text   string
}

// knownErrors — ошибки, которые клиент различает. Всё остальное считается
// сбоем сервера и наружу уходит только общим текстом.
var knownErrors = []struct {
	err    error
	status int
	code   string
}{
	{errors1.ErrChatNotFound, http.StatusNotFound, protocol.CodeNotFound},
	{errors1.ErrUserDoesntExist, http.StatusNotFound, protocol.CodeNotFound},
	{errors1.ErrNotChatMember, http.StatusForbidden, protocol.CodeForbidden},
	{errors1.ErrNoWriteAccess, http.StatusForbidden, protocol.CodeForbidden},
	{errors1.ErrIncorrectLoginData, http.StatusUnauthorized, protocol.CodeForbidden},
	{errors1.ErrExistsLogin, http.StatusConflict, protocol.CodeBadRequest},
	{errors1.ErrNameInUse, http.StatusConflict, protocol.CodeBadRequest},
}

// classify сопоставляет ошибке ответ. known = false — это сбой сервера: его
// надо записать в лог, клиенту достаётся только «внутренняя ошибка сервера».
func classify(err error) (failure, bool) {

	var validation *errors1.Validation
	if errors.As(err, &validation) {
		return failure{http.StatusBadRequest, protocol.CodeBadRequest, validation.Error()}, true
	}

	for _, known := range knownErrors {
		if errors.Is(err, known.err) {
			return failure{known.status, known.code, known.err.Error()}, true
		}
	}

	return failure{http.StatusInternalServerError, protocol.CodeInternal, internalErrorText}, false
}
