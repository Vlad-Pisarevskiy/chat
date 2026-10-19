package auth

import (
	"chatflow/internal/service"
	"time"
)

const (
	readBuffer    = 1024
	writeBuffer   = 1024
	msgBufferSize = 256
	readLimit     = 32 * 1024
	cookieMaxAge  = int((time.Hour * 24 * 30) / time.Second)

	readDeadline  = time.Second * 30
	writeDeadline = time.Second * 30
	tickerTiming  = time.Second * 25

	presenceType    = "presence"
	messageType     = "message"
	errorType       = "error"
	chatAddedType   = "chat_added"
	chatRemovedType = "chat_removed"
	chatClearedType = "chat_cleared"
	sendType        = "send"
	ackType         = "ack"

	ownerRole = "owner"

	peerID     = "peer_id"
	limitKey   = "limit"
	beforeKey  = "before"
	userIdKey  = "userID"
	handleKey  = "handle"
	chatIdKey  = "chatID"
	imageIdKey = "imageID"
	tokenKey   = "token"

	jsonMediaType      = "application/json"
	multipartMediaType = "multipart/form-data"

	imageField       = "file"
	chatIDField      = "chat_id"
	clientMsgIDField = "client_msg_id"
	bodyField        = "body"

	// Тело запроса с картинкой ограничено с запасом на поля multipart. Файл
	// больше MaxImageSize в него ещё помещается: тогда 413 отвечает сервис.
	multipartOverhead = 1 << 20
	maxRequestBytes   = service.MaxImageSize + multipartOverhead

	// Запись картинки не обрывается вместе с клиентом: сообщение, которое
	// сохранилось, должно быть разослано.
	imageWriteTimeout = time.Second * 30

	// Картинка не меняется никогда, поэтому браузер хранит её год. private не
	// даёт сохранить её общему прокси.
	imageCacheControl = "private, max-age=31536000, immutable"

	internalErrorText = "внутренняя ошибка сервера"
	unauthorizedText  = "нужно войти"
	notJSONText       = "тело запроса должно быть в формате JSON"
	notMultipartText  = "тело запроса должно быть в формате multipart/form-data"
	badChatText       = "чат не найден"
	badPeerText       = "peer_id должен быть числом"
	noDirectChatText  = "переписки ещё не было"

	emptyHandle      = ""
	emptyField       = ""
	emptyDescription = ""
	emptyToken       = ""
	emptyOrigin      = ""
	emptyClientMsgID = ""
	nullID           = 0

	// Отрицательный Max-Age велит браузеру удалить куку сразу.
	removeCookieAge = -1
)
