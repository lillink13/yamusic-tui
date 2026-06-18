package loginpage

import (
	"bytes"
	"strings"

	"github.com/lillink13/yamusic-tui/api"
	"github.com/lillink13/yamusic-tui/config"
	"github.com/lillink13/yamusic-tui/ui/style"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mdp/qrterminal/v3"
	"github.com/pkg/browser"
)

type loginState int

const (
	stateInput loginState = iota
	stateValidating
)

// tokenValidatedMsg carries the result of checking a token against account/status.
type tokenValidatedMsg struct {
	token string
	err   error
}

type Model struct {
	err           error
	program       *tea.Program
	width, height int

	input   textinput.Model
	spinner spinner.Model
	help    help.Model
	helpMap *helpKeyMap

	state       loginState
	authURL     string
	qr          string
	status      string
	statusError bool
}

// loginpage.Model constructor.
func New() *Model {
	authURL := api.OAuthTokenURL()

	var qrBuf bytes.Buffer
	qrterminal.GenerateWithConfig(authURL, qrterminal.Config{
		Level:          qrterminal.L,
		Writer:         &qrBuf,
		HalfBlocks:     true,
		BlackChar:      qrterminal.BLACK_BLACK,
		WhiteChar:      qrterminal.WHITE_WHITE,
		BlackWhiteChar: qrterminal.BLACK_WHITE,
		WhiteBlackChar: qrterminal.WHITE_BLACK,
		QuietZone:      1,
	})

	in := textinput.New()
	in.Placeholder = "paste token or redirect URL…"
	in.Width = 48
	in.CharLimit = 0 // tokens and especially pasted URLs exceed any small limit
	in.Focus()

	m := &Model{
		input:   in,
		spinner: spinner.New(spinner.WithSpinner(spinner.Points)),
		help:    help.New(),
		helpMap: newHelpMap(),
		authURL: authURL,
		qr:      strings.TrimRight(qrBuf.String(), "\n"),
		state:   stateInput,
	}

	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	m.program = p

	return m
}

//
// model.Model interface implementation
//

func (m *Model) Run() error {
	_, err := m.program.Run()
	if err != nil {
		return err
	}
	return m.err
}

func (m *Model) Send(msg tea.Msg) {
	go m.program.Send(msg)
}

//
// tea.Model interface implementation
//

func (m *Model) Init() tea.Cmd {
	// Open the browser straight away; the screen explains what happened and how
	// to fall back (Ctrl+O / the printed URL / the QR code).
	return tea.Batch(textinput.Blink, m.openBrowserCmd())
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
		return m, tea.ClearScreen

	case tokenValidatedMsg:
		if msg.err != nil {
			m.state = stateInput
			m.status = loginErrorText(msg.err)
			m.statusError = true
			m.input.Focus()
			return m, textinput.Blink
		}
		config.Current.Token = msg.token
		m.err = config.Save()
		return m, tea.Quit

	case spinner.TickMsg:
		if m.state == stateValidating {
			m.spinner, cmd = m.spinner.Update(msg)
			cmds = append(cmds, cmd)
		}

	case tea.KeyMsg:
		controls := config.Current.Controls
		keypress := msg.String()

		switch {
		case controls.Quit.Contains(keypress):
			return m, tea.Quit
		case keypress == "ctrl+o":
			m.status = "Opening browser…"
			m.statusError = false
			return m, m.openBrowserCmd()
		case m.state == stateValidating:
			// ignore edits while a check is in flight
			return m, nil
		case controls.Apply.Contains(keypress):
			token := api.ParseOAuthToken(m.input.Value())
			if token == "" {
				m.status = "Paste your token or the redirect URL first (Ctrl+O opens the browser)."
				m.statusError = true
				break
			}
			m.state = stateValidating
			m.status = "Checking token…"
			m.statusError = false
			return m, tea.Batch(m.spinner.Tick, validateCmd(token))
		default:
			m.input, cmd = m.input.Update(msg)
			cmds = append(cmds, cmd)
		}

	default:
		m.input, cmd = m.input.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m *Model) View() string {
	title := style.DialogTitleStyle.Render("Sign in to Yandex Music")

	muted := lipgloss.NewStyle().Foreground(style.InactiveTextColor)
	steps := lipgloss.JoinVertical(lipgloss.Left,
		"1. A browser opened for Yandex login "+muted.Render("(Ctrl+O to reopen)")+".",
		"2. After you allow access you land on "+style.AccentTextStyle.Render("music.yandex.ru")+".",
		"3. Copy the whole address bar, paste it below, and press Enter.",
	)

	var statusLine string
	switch {
	case m.state == stateValidating:
		statusLine = m.spinner.View() + " " + m.status
	case m.status != "" && m.statusError:
		statusLine = style.ErrorTextStyle.Render("✗ " + m.status)
	case m.status != "":
		statusLine = muted.Render(m.status)
	default:
		statusLine = muted.Render("token or redirect URL — both work")
	}

	left := lipgloss.JoinVertical(lipgloss.Left,
		steps,
		"",
		muted.Render(m.authURL),
		"",
		m.input.View(),
		"",
		statusLine,
	)

	right := lipgloss.JoinVertical(lipgloss.Center,
		m.qr,
		muted.Render("scan to open"),
	)

	body := left
	// Only place the QR beside the text when there is room, otherwise it would
	// wrap and break the layout on narrow terminals.
	if m.width == 0 || m.width >= lipgloss.Width(left)+lipgloss.Width(right)+8 {
		body = lipgloss.JoinHorizontal(lipgloss.Top, left, "   ", right)
	}

	content := lipgloss.JoinVertical(lipgloss.Left, title, "", body)
	content = style.DialogBoxStyle.Render(content)
	content = lipgloss.JoinVertical(lipgloss.Left, content, style.DialogHelpStyle.Render(m.help.View(m.helpMap)))

	return lipgloss.Place(
		m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		content,
	)
}

//
// private methods
//

func (m *Model) resize(width, height int) {
	m.width, m.height = width, height
}

func (m *Model) openBrowserCmd() tea.Cmd {
	authURL := m.authURL
	return func() tea.Msg {
		// Best-effort: on headless/SSH this fails and the user uses the QR/URL.
		_ = browser.OpenURL(authURL)
		return nil
	}
}

func validateCmd(token string) tea.Cmd {
	return func() tea.Msg {
		_, err := api.NewClient(config.DirName, token)
		return tokenValidatedMsg{token: token, err: err}
	}
}

func loginErrorText(err error) string {
	s := strings.ToLower(err.Error())
	switch {
	case strings.Contains(s, "unauthorized") || strings.Contains(s, "401") || strings.Contains(s, "forbidden") || strings.Contains(s, "403"):
		return "Token rejected — copy a fresh one and try again."
	case strings.Contains(s, "connect") || strings.Contains(s, "dial") || strings.Contains(s, "timeout") || strings.Contains(s, "no such host"):
		return "Can't reach Yandex — check your connection."
	default:
		return "Login failed: " + err.Error()
	}
}
