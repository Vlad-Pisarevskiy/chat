package server

import "time"

const (
	readBuffer    = 1024
	writeBuffer   = 1024
	msgBufferSize = 256
	readLimit     = 1024
	cookieMaxAge  = int((time.Hour * 24 * 30) / time.Second)

	readDeadline  = time.Second * 30
	writeDeadline = time.Second * 30
	tickerTiming  = time.Second * 25

	messageIdKey   = "messageID"
	presenceType   = "presence"
	messageType    = "message"
	peerID         = "peer_id"
	userIdKey      = "userID"
	handleKey      = "handle"
	chatIdKey      = "chatID"
	tokenKey       = "token"
	sendType       = "send"
	ackType        = "ack"
	emptyMessageID = ""
	emptyHandle
	emptyPeer = ""
	emptyChat = ""
	nullID    = 0
)
