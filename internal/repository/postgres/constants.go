package postgres

import "time"

const (
	chatAdmin  = "admin"
	chatOwner  = "owner"
	chatMember = "member"

	nullID         = 0
	tokenTTL       = time.Hour * 24
	tokenThreshold = time.Hour * 12
)
