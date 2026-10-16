package postgres

import "time"

const (
	adminRole  = "admin"
	ownerRole  = "owner"
	memberRole = "member"

	directType = "direct"

	// Коды ошибок Postgres.
	uniqueViolation     = "23505"
	foreignKeyViolation = "23503"

	nullID         = 0
	nullLength     = 0
	tokenTTL       = time.Hour * 24
	tokenThreshold = time.Hour * 12
)
