package postgres

import "time"

const (
	adminRole  = "admin"
	ownerRole  = "owner"
	memberRole = "member"

	directType  = "direct"
	groupType   = "group"
	channelType = "channel"

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

	nullID         = 0
	nullLength     = 0
	tokenTTL       = time.Hour * 24
	tokenThreshold = time.Hour * 12
)
