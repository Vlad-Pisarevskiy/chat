package chat

import "embed"

// Templates contains the chat pages, independent of the working directory.
//
//go:embed web/*.html
var Templates embed.FS

// Migrations contains only the schema owned by the chat service.
//
//go:embed migrations/*.sql
var Migrations embed.FS
