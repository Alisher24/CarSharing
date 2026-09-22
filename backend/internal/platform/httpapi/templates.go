package httpapi

import (
	"embed"
)

// The markup of the inbox pages, embedded in the binary: the stub stays one process with no build
// step and no volume of files, so its pages travel in it the way the migrations travel in the
// migrator.
//
//go:embed templates/*.html templates/*.css
var inboxMarkup embed.FS
