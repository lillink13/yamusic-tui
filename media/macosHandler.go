//go:build darwin && !nomedia

package media

import (
	"github.com/lillink13/yamusic-tui/media/handler"
	"github.com/lillink13/yamusic-tui/media/handler/macos"
)

func NewHandler(name, description string) handler.MediaHandler {
	return macos.NewHandler(name, description)
}
