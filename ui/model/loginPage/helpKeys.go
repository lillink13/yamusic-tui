package loginpage

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/lillink13/yamusic-tui/config"
)

type helpKeyMap struct {
	apply key.Binding
	open  key.Binding
	quit  key.Binding
}

func newHelpMap() *helpKeyMap {
	controls := config.Current.Controls
	return &helpKeyMap{
		apply: key.NewBinding(
			controls.Apply.Binding(),
			controls.Apply.Help("login"),
		),
		open: key.NewBinding(
			key.WithKeys("ctrl+o"),
			key.WithHelp("ctrl+o", "open browser"),
		),
		quit: key.NewBinding(
			controls.Quit.Binding(),
			controls.Quit.Help("quit"),
		),
	}
}

func (k helpKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.apply, k.open, k.quit}
}

func (k helpKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		k.ShortHelp(),
	}
}
