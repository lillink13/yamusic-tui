//go:build linux && !nomedia

package media

import (
	"github.com/lillink13/yamusic-tui/media/handler"
	"github.com/lillink13/yamusic-tui/media/handler/mpris"
)

func NewHandler(name, description string) handler.MediaHandler {
	return mpris.NewHandler(name, description)
}
