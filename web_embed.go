package personalserver

import (
	"embed"
	"io/fs"
)

//go:embed all:web/dist
var embeddedWeb embed.FS

func WebDistFS() (fs.FS, error) {
	return fs.Sub(embeddedWeb, "web/dist")
}
