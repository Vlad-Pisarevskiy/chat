package postgres

import "time"

const (
	adminRole  = "admin"
	ownerRole  = "owner"
	memberRole = "member"

	nullID         = 0
	tokenTTL       = time.Hour * 24
	tokenThreshold = time.Hour * 12
)
