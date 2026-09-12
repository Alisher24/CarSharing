// Package migrations embeds the SQL migrations so that the migrate command carries them in its
// own binary and cannot apply a revision it was not built from.
package migrations

import "embed"

//go:embed *.sql
var Files embed.FS
