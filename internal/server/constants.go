package server

import "time"

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

	peerID    = "peer_id"
	userIdKey = "userID"
	handleKey = "handle"
	chatIdKey = "chatID"
	tokenKey  = "token"

	jsonMediaType     = "application/json"
	internalErrorText = "внутренняя ошибка сервера"
	unauthorizedText  = "нужно войти"
	notJSONText       = "тело запроса должно быть в формате JSON"
	badChatText       = "чат не найден"
	badPeerText       = "peer_id должен быть числом"
	noDirectChatText  = "переписки ещё не было"

	emptyHandle      = ""
	emptyDescription = ""
	emptyToken       = ""
	emptyOrigin      = ""
	emptyClientMsgID = ""
	nullID           = 0

	// Отрицательный Max-Age велит браузеру удалить куку сразу.
	removeCookieAge = -1
)
