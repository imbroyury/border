package migrations

import "embed"

// FS contains all SQL migration files for use with golang-migrate's iofs source.
//go:embed *.sql
var FS embed.FS
