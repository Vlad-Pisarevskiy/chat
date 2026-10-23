package auth

import (
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
	emptyOrigin      = ""
	emptyClientMsgID = ""
	nullID           = 0

	// Отрицательный Max-Age велит браузеру удалить куку сразу.
	removeCookieAge = -1

	minLoginLength = 3
	maxLoginLength = 32

	minPasswordLength = 8
	maxPasswordLength = 64
	maxPasswordBytes  = 72

	nullLength           = 0
	minNameLength        = 1
	maxNameLength        = 50
	maxDescriptionLength = 255
	maxBodyLength        = 4096

	// История отдаётся пачками: без limit в запросе берётся DefaultHistoryLimit.
	DefaultHistoryLimit = 50
	maxHistoryLimit     = 100

	// MaxImageSize — наибольший размер картинки: 5 МБ, то есть 5 МиБ.
	MaxImageSize = 5 << 20

	// Сколько байт хватает http.DetectContentType, чтобы узнать формат.
	sniffLength = 512

	contentTypeJPEG = "image/jpeg"
	contentTypePNG  = "image/png"

	emptyID   = 0
	emptyName = ""
	emptyHash
	emptyLogin
	emptyPassword
	emptyToken
	emptyBody

	adminRole  = "admin"
	memberRole = "member"

	directType = "direct"

	// Коды ошибок Postgres.
	uniqueViolation     = "23505"
	foreignKeyViolation = "23503"

	// Условие «пользователь может писать в чат»: $1 — чат, $2 — пользователь.
	// Участник пишет везде, в канале — только владелец и админы. Единое для
	// текста и картинок.
	writeAllowedSQL = `EXISTS (SELECT 1 FROM users_chats uc
		JOIN chats c ON c.id = uc.chat_id
		WHERE uc.chat_id = $1 AND uc.user_id = $2
		AND (c.type <> 'channel' OR uc.role IN ('owner', 'admin')))`

	tokenTTL       = time.Hour * 24
	tokenThreshold = time.Hour * 12
)
