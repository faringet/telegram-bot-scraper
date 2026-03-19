package postgresmigrations

import "embed"

//go:embed *.up.sql
var FS embed.FS

const Dir = "."
