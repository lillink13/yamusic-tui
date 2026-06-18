//go:build windows && !nomedia

package media

import (
	"github.com/lillink13/yamusic-tui/media/handler"
	"github.com/lillink13/yamusic-tui/media/handler/win"
)

func NewHandler(name, description string) handler.MediaHandler {
	return win.NewHandler(name, description)
}
