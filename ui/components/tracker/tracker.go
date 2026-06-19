package tracker

import (
	"fmt"
	"io"
	"math"
	"strings"
	"time"

	"github.com/lillink13/yamusic-tui/api"
	"github.com/lillink13/yamusic-tui/config"
	"github.com/lillink13/yamusic-tui/log"
	"github.com/lillink13/yamusic-tui/stream"
	"github.com/lillink13/yamusic-tui/ui/helpers"
	"github.com/lillink13/yamusic-tui/ui/model"
	"github.com/lillink13/yamusic-tui/ui/style"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ebitengine/oto/v3"
)

type Control uint

const (
	PLAY Control = iota
	PAUSE
	STOP
	NEXT
	PREV
	LIKE
	REWIND
	VOLUME
	CACHE_TRACK
	BUFFERING_COMPLETE
	TOGGLE_LYRICS
	TOGGLE_VISUALIZER
	TOGGLE_VIEW
)

type ProgressControl float64

func (p ProgressControl) Value() float64 {
	return float64(p)
}

const (
	_VOLUME_FADE_STEPS     = 2
	_VOLUME_SNAP_THRESHOLD = 0.005 // snap to 0/1 when close enough

	_MARQUEE_GAP             = 4 // spaces between the end and the wrapped-around start
	_MARQUEE_FRAMES_PER_STEP = 4 // advance one column every N playback frames (~7 cols/s at 30fps)

	_VIZ_MARGIN     = 4    // sidebar/border padding subtracted from width for the visualizer
	_VIZ_PEAK_DECAY = 0.97 // per-frame decay of the auto-gain peak so it tracks quieter passages
	_VIZ_MIN_PEAK   = 0.5  // floor for the peak, so silence/near-silence doesn't blow up to full bars
	_VIZ_ATTACK     = 0.55 // how fast a bar rises toward a louder target
	_VIZ_DECAY      = 0.22 // how fast a bar falls toward a quieter target
)

var rewindAmount = time.Duration(config.Current.RewindDuration) * time.Second

type Model struct {
	width          int
	track          api.Track
	lyrics         []api.LyricPair
	textLyrics     []string
	progress       progress.Model
	volumeBar      progress.Model
	help           help.Model
	helpMap        *helpKeyMap
	Hidden         bool
	showLyrics     bool
	showVisualizer bool
	showError      bool
	errorText      string

	paused         bool
	playtime       time.Duration
	playStarted    time.Time
	volume         float64
	volumeIncremet float64
	volumeBarShown float64 // eased value the volume bar is drawn at (lerps toward volume)
	lastVolumeKey  time.Time
	animTick       uint64 // monotonic frame counter advanced each ProgressControl, drives the now-playing marquee

	vizBars       []float64 // smoothed spectrum bar levels [0,1] drawn by the visualizer
	vizPCM        []float64 // reusable scratch for the latest PCM window
	vizRe, vizIm  []float64 // reusable FFT scratch (allocated once, sized _VIZ_FFT_SIZE)
	vizPeak       float64   // decaying magnitude peak, for auto-gain normalization
	playerContext *oto.Context
	player        *oto.Player
	trackWrapper  *readWrapper

	program  *tea.Program
	likesMap *map[string]bool
}

func New(p *tea.Program, likesMap *map[string]bool) *Model {
	m := &Model{
		program:        p,
		likesMap:       likesMap,
		progress:       progress.New(),
		volumeBar:      progress.New(),
		help:           help.New(),
		helpMap:        newHelpMap(),
		paused:         true,
		volume:         config.Current.Volume,
		showLyrics:     config.Current.ShowLyrics,
		showVisualizer: config.Current.ShowVisualizer,
	}

	m.volumeIncremet = m.volume / _VOLUME_FADE_STEPS

	m.progress.ShowPercentage = false
	m.progress.Empty = m.progress.Full
	m.progress.FullColor = string(style.AccentColor)
	m.progress.EmptyColor = string(style.BackgroundColor)
	m.progress.SetSpringOptions(60, 1)

	m.volumeBar.FullColor = string(style.AccentColor)
	m.volumeBar.EmptyColor = string(style.BackgroundColor)
	m.volumeBar.Width = style.VolumeIndicatorWidth
	m.volumeBarShown = m.volume

	m.help.Ellipsis = "…"
	m.trackWrapper = &readWrapper{program: m.program}
	m.trackWrapper.vizEnabled.Store(config.Current.ShowVisualizer)

	op := &oto.NewContextOptions{
		SampleRate:   44100,
		ChannelCount: 2,
		BufferSize:   time.Millisecond * time.Duration(config.Current.BufferSize),
		Format:       oto.FormatSignedInt16LE,
	}

	var err error
	var readyChan chan struct{}
	m.playerContext, readyChan, err = oto.NewContext(op)
	if err != nil {
		log.Print(log.LVL_PANIC, "failed to create player context: %s", err)
		model.PrettyExit(err, 12)
	}
	<-readyChan

	return m
}

func (m *Model) Init() tea.Cmd {
	return nil
}

func (m *Model) View() string {
	if m.Hidden {
		return ""
	}

	var playButton string
	if m.IsPlaying() {
		playButton = style.ActiveButtonStyle.Padding(0, 1).Margin(0).Render(style.IconPlay)
	} else {
		playButton = style.ActiveButtonStyle.Padding(0, 1).Margin(0).Render(style.IconStop)
	}

	var volumeIndicator string
	var volumeIndicatorWidth int
	if style.VolumeIndicatorWidth > 0 && m.width > style.VolumeIndicatorAutohide {
		var volumeIcon string
		if m.volume <= 0 {
			volumeIcon = style.IconVolumeOff
		} else if m.volume < 0.1 {
			volumeIcon = style.IconVolumeLow
		} else if m.volume < 0.5 {
			volumeIcon = style.IconVolumeMid
		} else {
			volumeIcon = style.IconVolumeHigh
		}
		volumeIndicator = " " + volumeIcon + " " + m.volumeBar.ViewAs(m.volumeBarShown)
		volumeIndicatorWidth = lipgloss.Width(volumeIndicator)
	}

	m.progress.Width = m.width - volumeIndicatorWidth - 9
	tracker := style.TrackProgressStyle.Render(m.progress.View())
	tracker = lipgloss.JoinHorizontal(lipgloss.Top, playButton, tracker, volumeIndicator)

	if m.showLyrics {
		tracker = lipgloss.JoinVertical(lipgloss.Left, m.renderLyrics(), "", tracker)
	}

	if m.showVisualizer {
		tracker = lipgloss.JoinVertical(lipgloss.Left, m.renderVisualizer(), "", tracker)
	}

	if m.showError && !config.Current.SuppressErrors {
		errText := "Error: " + m.errorText + "; -> " + log.Location()
		maxLen := m.Width() - 4
		if lipgloss.Width(errText) > maxLen {
			errText = lipgloss.NewStyle().MaxWidth(maxLen-1).Render(errText) + "…"
		}
		tracker = lipgloss.JoinVertical(lipgloss.Left, style.ErrorTextStyle.Render(errText), "", tracker)
	}

	var trackTitle string
	if !m.help.ShowAll {
		if m.track.Available {
			trackTitle = style.TrackTitleStyle.Render(m.track.Title)
		} else {
			trackTitle = style.TrackTitleStyle.Strikethrough(true).Render(m.track.Title)
		}

		trackVersion := style.TrackVersionStyle.Render(" " + m.track.Version)
		trackTitle = lipgloss.JoinHorizontal(lipgloss.Top, trackTitle, trackVersion)

		durTotal := time.Millisecond * time.Duration(m.track.DurationMs)
		durEllapsed := time.Millisecond * time.Duration(float64(m.track.DurationMs)*m.progress.Percent())
		trackTime := style.TrackVersionStyle.Render(fmt.Sprintf("%02d:%02d/%02d:%02d",
			int(durEllapsed.Minutes()),
			int(durEllapsed.Seconds())%60,
			int(durTotal.Minutes()),
			int(durTotal.Seconds())%60,
		))

		var trackLike string
		if (*m.likesMap)[m.track.Id] {
			trackLike = style.IconLiked + " "
		} else {
			trackLike = style.IconNotLiked + " "
		}

		trackAddInfo := style.TrackAddInfoStyle.Render(trackLike + trackTime)
		addInfoLen := lipgloss.Width(trackAddInfo)
		maxLen := m.Width() - addInfoLen - 4

		trackTitleLen := lipgloss.Width(trackTitle)
		if trackTitleLen > maxLen {
			// Too long to fit — scroll it instead of truncating. The marquee renders
			// from the raw text in the title style (dropping the separate version
			// styling while scrolling).
			rawTitle := m.track.Title
			if m.track.Version != "" {
				rawTitle += " " + m.track.Version
			}
			titleStyle := style.TrackTitleStyle
			if !m.track.Available {
				titleStyle = titleStyle.Strikethrough(true)
			}
			// MaxWidth clamps to maxLen display cells; marquee slices by rune, so a
			// window of wide (CJK/emoji) glyphs would otherwise overflow and wrap the
			// fixed-height now-playing line.
			trackTitle = titleStyle.MaxWidth(maxLen).Render(marquee(rawTitle, maxLen, m.animTick))
		} else if trackTitleLen < maxLen {
			trackTitle += strings.Repeat(" ", maxLen-trackTitleLen)
		}

		trackArtist := style.TrackArtistStyle.Render(helpers.ArtistList(m.track.Artists))
		trackArtistLen := lipgloss.Width(trackArtist)
		if trackArtistLen > maxLen {
			trackArtist = style.TrackArtistStyle.MaxWidth(maxLen).Render(marquee(helpers.ArtistList(m.track.Artists), maxLen, m.animTick))
		} else if trackArtistLen < maxLen {
			trackArtist += strings.Repeat(" ", maxLen-trackArtistLen)
		}

		trackTitle = lipgloss.NewStyle().Width(m.width - lipgloss.Width(trackAddInfo) - 4).Render(trackTitle)
		trackTitle = lipgloss.JoinHorizontal(lipgloss.Top, trackTitle, trackAddInfo)
		trackTitle = lipgloss.JoinVertical(lipgloss.Left, trackTitle, trackArtist, "")

		tracker = lipgloss.JoinVertical(lipgloss.Left, tracker, trackTitle)
	}

	tracker = lipgloss.JoinVertical(lipgloss.Left, tracker, m.help.View(m.helpMap))
	return style.TrackBoxStyle.Width(m.width).Render(tracker)
}

func (m *Model) Update(message tea.Msg) (*Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := message.(type) {
	case tea.KeyMsg:
		controls := config.Current.Controls
		keypress := msg.String()

		switch {
		case controls.ShowAllKeys.Contains(keypress):
			m.help.ShowAll = !m.help.ShowAll

		case controls.PlayerPause.Contains(keypress):
			if m.player == nil {
				break
			}
			if m.player.IsPlaying() {
				m.Pause()
				cmds = append(cmds, model.Cmd(PAUSE))
			} else {
				m.Play()
				cmds = append(cmds, model.Cmd(PLAY))
			}

		case controls.PlayerRewindForward.Contains(keypress):
			cmd = m.Rewind(rewindAmount)
			cmds = append(cmds, cmd, model.Cmd(REWIND))

		case controls.PlayerRewindBackward.Contains(keypress):
			cmd = m.Rewind(-rewindAmount)
			cmds = append(cmds, cmd, model.Cmd(REWIND))

		case controls.PlayerNext.Contains(keypress):
			cmds = append(cmds, model.Cmd(NEXT))

		case controls.PlayerPrevious.Contains(keypress):
			cmds = append(cmds, model.Cmd(PREV))

		case controls.PlayerLike.Contains(keypress):
			cmds = append(cmds, model.Cmd(LIKE))

		case controls.PlayerCache.Contains(keypress):
			if !m.IsStoped() {
				// Finish buffering off the UI goroutine so the interface stays
				// responsive on a slow/large track, then trigger the cache write
				// once the whole track is available.
				buf := m.trackWrapper.trackBuffer
				go func() {
					buf.BufferAll()
					m.program.Send(CACHE_TRACK)
				}()
			}

		case controls.PlayerVolUp.Contains(keypress):
			m.SetVolume(m.volume + m.dynamicVolumeStep())
			cmds = append(cmds, model.Cmd(VOLUME))

		case controls.PlayerVolDown.Contains(keypress):
			m.SetVolume(m.volume - m.dynamicVolumeStep())
			cmds = append(cmds, model.Cmd(VOLUME))

		case controls.PlayerToggleLyrics.Contains(keypress):
			m.SetLirycs(!m.showLyrics)
			cmds = append(cmds, model.Cmd(TOGGLE_LYRICS))
		case controls.PlayerToggleVisualizer.Contains(keypress):
			m.SetVisualizer(!m.showVisualizer)
			cmds = append(cmds, model.Cmd(TOGGLE_VISUALIZER))
		case controls.PlayerHide.Contains(keypress):
			m.Hidden = !m.Hidden
			cmds = append(cmds, model.Cmd(TOGGLE_VIEW))
		}

	// player control update
	case Control:
		switch msg {
		case PLAY:
			m.Play()
		case PAUSE:
			m.Pause()
		case STOP:
			m.Stop()
		}

	// track progress update
	case ProgressControl:
		m.volumeFadeTick()
		m.volumeBarTick()
		m.animTick++
		if m.showVisualizer {
			m.updateVisualizer()
		}
		cmd = m.progress.SetPercent(msg.Value())
		cmds = append(cmds, cmd)

	case progress.FrameMsg:
		var progressModel tea.Model
		progressModel, cmd = m.progress.Update(msg)
		m.progress = progressModel.(progress.Model)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m *Model) SetWidth(width int) {
	m.width = width
	m.help.Width = width - 4
}

func (m *Model) Width() int {
	return m.width
}

func (m *Model) Height() int {
	baseHeight := 4
	if m.showLyrics {
		baseHeight += 4
	}
	if m.showVisualizer {
		baseHeight += _VIZ_HEIGHT + 1 // panel rows + spacer
	}
	if m.showError && !config.Current.SuppressErrors {
		baseHeight += 2
	}
	return baseHeight
}

func (m *Model) Progress() float64 {
	return m.progress.Percent()
}

func (m *Model) Position() time.Duration {
	return time.Duration(float64(m.track.DurationMs)*m.trackWrapper.Progress()) * time.Millisecond
}

func (m *Model) SetVolume(v float64) {
	if v < _VOLUME_SNAP_THRESHOLD {
		v = 0
	} else if v > 1-_VOLUME_SNAP_THRESHOLD {
		v = 1
	}
	m.volume = v
	m.volumeIncremet = m.volume / _VOLUME_FADE_STEPS
	// The volume bar eases toward m.volume on the playback tick; when nothing is
	// playing there is no tick to animate it, so snap it for immediate feedback.
	if m.player == nil || m.paused {
		m.volumeBarShown = m.volume
	}
	config.Current.Volume = m.volume
	config.Save()
}

// volumeBarTick eases the drawn volume level toward the target volume. Called on
// the playback tick so a volume change glides instead of snapping.
func (m *Model) volumeBarTick() {
	const ease = 0.3
	diff := m.volume - m.volumeBarShown
	if diff < 0.002 && diff > -0.002 {
		m.volumeBarShown = m.volume
		return
	}
	m.volumeBarShown += diff * ease
}

func (m *Model) SetLirycs(show bool) {
	m.showLyrics = show
	config.Current.ShowLyrics = m.showLyrics
	config.Save()
}

func (m *Model) SetVisualizer(show bool) {
	m.showVisualizer = show
	m.trackWrapper.vizEnabled.Store(show) // let the audio thread skip the PCM tap when off
	config.Current.ShowVisualizer = m.showVisualizer
	config.Save()
}

func (m *Model) Volume() float64 {
	return m.volume
}

func (m *Model) StartTrack(track *api.Track, reader *stream.BufferedStream, lyrics []api.LyricPair, textLyrics []string) {
	m.showError = false
	m.volume = config.Current.Volume
	m.volumeIncremet = m.volume / _VOLUME_FADE_STEPS

	if m.player != nil {
		m.Stop()
	}

	m.track = *track
	m.trackWrapper.NewReader(reader)
	m.player = m.playerContext.NewPlayer(m.trackWrapper)
	m.player.SetVolume(0)
	m.player.Play()
	m.lyrics = lyrics
	m.textLyrics = textLyrics
	m.paused = false
	m.playtime = 0
	m.playStarted = time.Now()
}

func (m *Model) Stop() {
	if m.player == nil {
		return
	}

	if m.player.IsPlaying() {
		m.player.SetVolume(0)
		m.player.Pause()
	}

	if m.trackWrapper.trackBuffer.Error() != nil {
		m.ShowError("track buffering")
	}

	m.trackWrapper.Close()
	m.player.Close()
	m.player = nil
	m.playtime += time.Since(m.playStarted)
	m.paused = true

	// No more PCM will arrive — settle the visualizer to silence instead of
	// freezing on the last frame.
	m.resetVisualizer()
}

func (m *Model) IsPlaying() bool {
	return m.player != nil && m.trackWrapper.trackBuffer != nil && m.player.IsPlaying()
}

func (m *Model) IsStoped() bool {
	return m.player == nil || m.trackWrapper.trackBuffer == nil
}

func (m *Model) CurrentTrack() *api.Track {
	return &m.track
}

func (m *Model) Play() {
	if m.player == nil || m.trackWrapper.trackBuffer == nil {
		return
	}
	if m.player.IsPlaying() {
		return
	}
	m.volume = config.Current.Volume
	m.volumeIncremet = m.volume / _VOLUME_FADE_STEPS
	m.player.SetVolume(0)
	m.player.Play()
	m.paused = false
	m.playStarted = time.Now()
}

func (m *Model) Pause() {
	if m.player == nil || m.trackWrapper.trackBuffer == nil {
		return
	}
	if !m.player.IsPlaying() {
		return
	}
	m.playtime += time.Since(m.playStarted)
	m.paused = true
	// Playback ticks (which ease the volume bar and drive the visualizer) are
	// about to stop — land the volume bar on its target so it can't freeze
	// mid-ease, and settle the spectrum to silence instead of freezing.
	m.volumeBarShown = m.volume
	m.resetVisualizer()
}

func (m *Model) Rewind(amount time.Duration) tea.Cmd {
	if m.player == nil || m.trackWrapper == nil {
		go m.program.Send(STOP)
		return nil
	}

	m.player.SetVolume(0)

	amountMs := float64(amount.Milliseconds())
	currentPos := int64(float64(m.trackWrapper.Length()) * m.trackWrapper.Progress())
	byteOffset := int64(math.Round((float64(m.trackWrapper.Length()) / float64(m.track.DurationMs)) * amountMs))

	// align position by 4 bytes
	currentPos += byteOffset
	currentPos += currentPos % 4

	if currentPos <= 0 {
		m.player.Seek(0, io.SeekStart)
	} else if currentPos >= m.trackWrapper.Length() {
		m.player.Seek(0, io.SeekEnd)
	} else {
		m.player.Seek(currentPos, io.SeekStart)
	}

	return m.progress.SetPercent(m.trackWrapper.Progress())
}

func (m *Model) SetPos(pos time.Duration) {
	if m.player == nil || m.trackWrapper == nil {
		go m.program.Send(STOP)
		return
	}

	posMs := pos.Milliseconds()
	byteOffset := int64(math.Round((float64(m.trackWrapper.Length()) / float64(m.track.DurationMs)) * float64(posMs)))

	// align position by 4 bytes
	byteOffset += byteOffset % 4
	m.player.Seek(byteOffset, io.SeekStart)
}

func (m *Model) TrackBuffer() *stream.BufferedStream {
	return m.trackWrapper.trackBuffer
}

func (m *Model) Playtime() time.Duration {
	if m.paused {
		return m.playtime
	}
	return m.playtime + time.Since(m.playStarted)
}

func (m *Model) ShowError(text string) {
	m.showError = true
	m.errorText = text
}

func (m *Model) HideError() {
	m.showError = false
}

func (m *Model) dynamicVolumeStep() float64 {
	now := time.Now()
	delta := now.Sub(m.lastVolumeKey)
	m.lastVolumeKey = now

	step := config.Current.VolumeStep
	switch {
	case delta < 80*time.Millisecond:
		return step
	case delta < 200*time.Millisecond:
		return step * 0.4
	default:
		return step * 0.2
	}
}

func (m *Model) volumeFadeTick() {
	if !m.IsPlaying() {
		return
	}

	if m.volumeIncremet == 0 {
		m.player.SetVolume(0)
		return
	}

	var targetVolume float64
	if m.paused {
		targetVolume = 0
	} else {
		targetVolume = m.volume
	}

	currVol := m.player.Volume()
	if currVol >= targetVolume+m.volumeIncremet {
		m.player.SetVolume(currVol - m.volumeIncremet/2)
	} else if currVol <= targetVolume-m.volumeIncremet {
		m.player.SetVolume(currVol + m.volumeIncremet/2)
	} else if currVol != targetVolume {
		m.player.SetVolume(targetVolume)
		if m.paused {
			m.player.Pause()
		}
	}
}

func (m *Model) renderLyrics() string {
	currentLine := " "
	nextLine := " "
	previousLine := " "

	if m.player != nil && m.showLyrics {
		switch {
		case m.track.LyricsInfo.HasAvailableSyncLyrics:
			for idx, line := range m.lyrics {
				if line.Timestamp > int(m.Position().Milliseconds()-1000) {
					previousLine = m.tryGetLyricsLine(idx - 2)
					currentLine = m.lyricsBreak(m.tryGetLyricsLine(idx - 1))
					nextLine = m.tryGetLyricsLine(idx)
					break
				}
			}
		case len(m.textLyrics) > 0:
			// Plain-text fallback: there are no per-line timestamps, so approximate
			// a follow by scrolling through the lyrics in proportion to playback
			// progress, reusing the same prev/current/next window.
			idx := int(m.trackWrapper.Progress() * float64(len(m.textLyrics)))
			if idx >= len(m.textLyrics) {
				idx = len(m.textLyrics) - 1
			}
			previousLine = m.tryGetTextLine(idx - 1)
			currentLine = m.lyricsBreak(m.tryGetTextLine(idx))
			nextLine = m.tryGetTextLine(idx + 1)
		default:
			currentLine = "This song doesn't have lyrics!"
		}
	}

	previousLine = lipgloss.NewStyle().Foreground(style.LyricsPreviosTextColor).Render(previousLine)
	nextLine = lipgloss.NewStyle().Foreground(style.LyricsNextTextColor).Render(nextLine)
	currentLine = lipgloss.NewStyle().Foreground(style.LyricsCurrentTextColor).Render(currentLine)

	lyrics := lipgloss.JoinVertical(lipgloss.Center, previousLine, currentLine, nextLine)
	lyrics = lipgloss.NewStyle().Width(m.width - 4).AlignHorizontal(lipgloss.Center).Render(lyrics)

	return lyrics
}

func (m *Model) tryGetLyricsLine(idx int) (line string) {
	if idx < 0 || idx >= len(m.lyrics) {
		return
	}
	return m.lyrics[idx].Line
}

func (m *Model) tryGetTextLine(idx int) string {
	if idx < 0 || idx >= len(m.textLyrics) {
		return " "
	}
	return m.textLyrics[idx]
}

// marquee returns a width-wide slice of text. When text fits it is right-padded
// with spaces; when it is longer than width it scrolls horizontally, advancing
// with tick and wrapping around through a small gap. Slicing is rune-based, so
// it assumes single-width glyphs (true for latin/cyrillic titles).
func marquee(text string, width int, tick uint64) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= width {
		return text + strings.Repeat(" ", width-len(runes))
	}
	period := len(runes) + _MARQUEE_GAP
	offset := int((tick / _MARQUEE_FRAMES_PER_STEP) % uint64(period))
	buf := make([]rune, 0, len(runes)+_MARQUEE_GAP+width)
	buf = append(buf, runes...)
	for i := 0; i < _MARQUEE_GAP; i++ {
		buf = append(buf, ' ')
	}
	buf = append(buf, runes...) // wrap-around source for the tail window
	return string(buf[offset : offset+width])
}

func (m *Model) vizCols() int {
	cols := m.width - _VIZ_MARGIN
	if cols < 0 {
		return 0
	}
	return cols
}

// updateVisualizer pulls the latest decoded PCM window, computes a log-frequency
// spectrum, normalizes it with a decaying-peak auto-gain (so the bars fill the
// panel regardless of loudness), and eases the result into m.vizBars. Driven by
// ProgressControl, so it only runs while audio is actually playing.
func (m *Model) updateVisualizer() {
	cols := m.vizCols()
	if cols <= 0 {
		return
	}
	m.vizPCM = m.trackWrapper.latestPCM(m.vizPCM)
	if m.vizRe == nil {
		m.vizRe = make([]float64, _VIZ_FFT_SIZE)
		m.vizIm = make([]float64, _VIZ_FFT_SIZE)
	}
	mags := spectrumInto(m.vizPCM, cols, m.vizRe, m.vizIm)

	maxMag := 0.0
	for _, v := range mags {
		if v > maxMag {
			maxMag = v
		}
	}
	m.vizPeak *= _VIZ_PEAK_DECAY
	if maxMag > m.vizPeak {
		m.vizPeak = maxMag
	}
	if m.vizPeak < _VIZ_MIN_PEAK {
		m.vizPeak = _VIZ_MIN_PEAK
	}

	if len(m.vizBars) != cols {
		m.vizBars = make([]float64, cols)
	}
	for i, v := range mags {
		target := v / m.vizPeak
		if target > 1 {
			target = 1
		}
		// Rise quickly to a louder target, fall back more gently.
		if target > m.vizBars[i] {
			m.vizBars[i] += (target - m.vizBars[i]) * _VIZ_ATTACK
		} else {
			m.vizBars[i] += (target - m.vizBars[i]) * _VIZ_DECAY
		}
	}
}

// renderVisualizer draws the smoothed spectrum as an accent-colored panel of a
// fixed height (_VIZ_HEIGHT rows) so toggling it or pausing never jitters the
// layout. When the terminal is too narrow to draw any bars it still emits a
// _VIZ_HEIGHT-row blank block so the panel height — and thus Height() — stays
// constant.
func (m *Model) renderVisualizer() string {
	cols := m.vizCols()
	if cols <= 0 {
		return strings.Repeat("\n", _VIZ_HEIGHT-1)
	}
	bars := make([]float64, cols)
	copy(bars, m.vizBars) // surplus entries stay 0; a larger m.vizBars is clipped
	panel := renderSpectrum(bars, _VIZ_HEIGHT)
	return lipgloss.NewStyle().Foreground(style.AccentColor).Render(panel)
}

// resetVisualizer settles the spectrum to silence (no PCM is arriving), used on
// pause and stop so the bars don't freeze on their last frame.
func (m *Model) resetVisualizer() {
	for i := range m.vizBars {
		m.vizBars[i] = 0
	}
	m.vizPeak = 0
}

func (m *Model) lyricsBreak(line string) (newLine string) {
	if strings.TrimSpace(strings.TrimSpace(line)) != "" {
		return line
	}

	switch m.Position().Milliseconds() % 900 / 300 {
	default:
		newLine = style.IconDotLight + style.IconDotDark + style.IconDotDark
	case 1:
		newLine = style.IconDotDark + style.IconDotLight + style.IconDotDark
	case 2:
		newLine = style.IconDotDark + style.IconDotDark + style.IconDotLight
	}

	return
}
