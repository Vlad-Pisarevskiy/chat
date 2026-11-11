package auth

import "embed"

// Templates contains the auth pages, independent of the working directory.
//
//go:embed web/*.html
var Templates embed.FS

// Migrations contains only the schema owned by the auth service.
//
//go:embed migrations/*.sql
var Migrations embed.FS
