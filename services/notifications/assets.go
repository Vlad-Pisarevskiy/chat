package notifications

import "embed"

// Migrations contains only the schema owned by the notifications service.
//
//go:embed migrations/*.sql
var Migrations embed.FS
