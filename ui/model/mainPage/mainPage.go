package mainpage

import (
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/lillink13/yamusic-tui/api"
	"github.com/lillink13/yamusic-tui/cache"
	"github.com/lillink13/yamusic-tui/config"
	"github.com/lillink13/yamusic-tui/log"
	"github.com/lillink13/yamusic-tui/media/handler"
	"github.com/lillink13/yamusic-tui/ui/components/input"
	"github.com/lillink13/yamusic-tui/ui/components/playlist"
	"github.com/lillink13/yamusic-tui/ui/components/search"
	"github.com/lillink13/yamusic-tui/ui/components/tracker"
	"github.com/lillink13/yamusic-tui/ui/components/tracklist"
	"github.com/lillink13/yamusic-tui/ui/model"
	"github.com/lillink13/yamusic-tui/ui/style"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dece2183/go-clipboard"
)

// dataLoadedMsg carries everything fetched by the background load command so it
// can be applied to the model inside Update (on the Bubble Tea goroutine)
// instead of being mutated directly from a background goroutine.
type dataLoadedMsg struct {
	client        *api.YaMusicClient
	wave          *api.StationTracks
	likedTracks   []api.Track
	likedIds      map[string]bool
	cachedTracks  []api.Track
	cachedIds     map[string]bool
	userPlaylists []*playlist.Item
	errLabel      string
}

// mediaSnapshot is a small, mutex-guarded view of the player state that the
// system media handler (MPRIS / macOS Now Playing / Windows SMTC) queries from
// its own goroutine. The Bubble Tea Update goroutine is the sole writer (it owns
// the tracker); mediaHandle only ever reads this snapshot, so the media handler
// never touches live tracker state across goroutines.
type mediaSnapshot struct {
	state    handler.PlaybackState
	volume   float64
	position time.Duration
	metadata handler.TrackMetadata
}

// Messages emitted by mediaHandle so that media-key / MPRIS commands are applied
// to the tracker on the Bubble Tea goroutine (inside Update) instead of from the
// mediaHandle goroutine, which used to race the Update loop.
type (
	mediaPlayPauseMsg struct{}
	mediaSeekMsg      time.Duration // relative rewind
	mediaSetPosMsg    time.Duration // absolute position
	mediaSetVolumeMsg float64
	mediaShuffleMsg   struct{}
)

type Model struct {
	program       *tea.Program
	client        *api.YaMusicClient
	clipboard     *clipboard.Clipboard
	mediaHandler  handler.MediaHandler
	width, height int

	spinner   spinner.Model
	playlists *playlist.Model
	tracklist *tracklist.Model
	tracker   *tracker.Model

	searchDialog           *search.Model
	inputDialog            *input.Model
	isLoading              bool
	isSearchActive         bool
	isAddPlaylistActive    bool
	isRenamePlaylistActive bool
	isPlaylistHideOverride bool

	currentPlaylistIndex int
	likedTracksMap       map[string]bool
	cachedTracksMap      map[string]bool

	mediaMu   sync.RWMutex
	mediaSnap mediaSnapshot
}

// mainpage.Model constructor.
func New(mediaHandler handler.MediaHandler) *Model {
	m := &Model{}

	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	m.program = p
	m.clipboard = clipboard.New()
	m.mediaHandler = mediaHandler
	m.likedTracksMap = make(map[string]bool)
	m.cachedTracksMap = make(map[string]bool)
	m.spinner = spinner.New(spinner.WithSpinner(spinner.Points))
	m.playlists = playlist.New(m.program, "YaMusic")
	m.tracklist = tracklist.New(m.program, &m.likedTracksMap, &m.cachedTracksMap)
	m.tracker = tracker.New(m.program, &m.likedTracksMap)
	m.searchDialog = search.New()
	m.inputDialog = input.New()

	// Seed the media snapshot so a query arriving before the first Update gets a
	// sane answer (the player starts stopped at the configured volume).
	m.mediaSnap.state = handler.STATE_STOPED
	m.mediaSnap.volume = config.Current.Volume

	return m
}

//
// model.Model interface implementation
//

func (m *Model) Run() error {
	go m.mediaHandle()
	_, err := m.program.Run()
	m.tracker.Stop()
	return err
}

func (m *Model) Send(msg tea.Msg) {
	go m.program.Send(msg)
}

//
// tea.Model interface implementation
//

func (m *Model) Init() tea.Cmd {
	m.isLoading = true
	m.tracker.HideError()
	return tea.Batch(m.spinner.Tick, m.loadData)
}

func (m *Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := message.(type) {
	case dataLoadedMsg:
		m.applyLoadedData(msg)
		m.isLoading = false
		return m, model.Cmd(playlist.CURSOR_UP)

	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
		return m, tea.ClearScreen

	case tea.KeyMsg:
		controls := config.Current.Controls
		keypress := msg.String()

		switch {
		case controls.Quit.Contains(keypress):
			return m, tea.Quit
		case m.isSearchActive || m.isAddPlaylistActive:
			m.searchDialog, cmd = m.searchDialog.Update(message)
			cmds = append(cmds, cmd)
		case m.isRenamePlaylistActive:
			m.inputDialog, cmd = m.inputDialog.Update(message)
			cmds = append(cmds, cmd)
		case controls.Reload.Contains(keypress):
			m.isLoading = true
			m.tracker.HideError()
			cmd = m.playlists.Reset()
			cmds = append(cmds, cmd)
			cmds = append(cmds, m.spinner.Tick)
			cmds = append(cmds, m.loadData)
		default:
			if m.isLoading {
				m.spinner, cmd = m.spinner.Update(message)
				cmds = append(cmds, cmd)
			} else {
				m.playlists, cmd = m.playlists.Update(message)
				cmds = append(cmds, cmd)
				m.tracklist, cmd = m.tracklist.Update(message)
				cmds = append(cmds, cmd)
				m.tracker, cmd = m.tracker.Update(message)
				cmds = append(cmds, cmd)
			}
		}

	// playlist control update
	case playlist.Control:
		switch msg {
		case playlist.CURSOR_UP, playlist.CURSOR_DOWN:
			selectedPlaylist := m.playlists.SelectedItem()

			if m.currentPlaylistIndex >= 0 {
				currentPlaylist := m.playlists.Items()[m.currentPlaylistIndex]
				if selectedPlaylist.IsSame(currentPlaylist) && len(selectedPlaylist.Tracks) > 0 {
					selectedPlaylist.SelectedTrack = selectedPlaylist.CurrentTrack
					m.playlists.SetItem(m.playlists.Index(), selectedPlaylist)
				}
			}

			m.displayPlaylist(selectedPlaylist)
			m.indicateCurrentTrackPlaying(m.tracker.IsPlaying())

			m.tracklist.Shufflable = (selectedPlaylist.Kind != playlist.NONE && selectedPlaylist.Kind != playlist.MYWAVE && len(selectedPlaylist.Tracks) > 0)
		case playlist.RENAME:
			selectedPlaylist := m.playlists.SelectedItem()
			if selectedPlaylist.Kind < playlist.USER {
				break
			}
			m.inputDialog.Title = "Rename playlist " + selectedPlaylist.Name
			m.inputDialog.SetValue(selectedPlaylist.Name)
			m.isRenamePlaylistActive = true
		case playlist.TOGGLE_VIEW:
			m.isPlaylistHideOverride = !m.isPlaylistHideOverride
		}

	// tracklist control update
	case tracklist.Control:
		switch msg {
		case tracklist.PLAY:
			playlistItem := m.playlists.SelectedItem()
			if !playlistItem.Active {
				break
			}
			m.playSelectedPlaylist(m.tracklist.Index())
		case tracklist.CURSOR_UP, tracklist.CURSOR_DOWN:
			currentPlaylist := m.playlists.SelectedItem()
			cursorIndex := m.tracklist.Index()
			currentPlaylist.SelectedTrack = cursorIndex
			cmd = m.playlists.SetItem(m.playlists.Index(), currentPlaylist)
			cmds = append(cmds, cmd)
		case tracklist.LIKE:
			cmd = m.likeSelectedTrack()
			cmds = append(cmds, cmd)
		case tracklist.ADD_TO_PLAYLIST:
			selectedTrack := m.tracklist.SelectedItem()
			m.searchDialog.Title = "Add " + selectedTrack.Track.Title + " to"
			m.searchDialog.Action = "add"
			m.isAddPlaylistActive = true
			m.Send(search.UPDATE_SUGGESTIONS)
		case tracklist.REMOVE_FROM_PLAYLIST:
			selectedPlaylist := m.playlists.SelectedItem()
			cmd = m.removeFromPlaylist(selectedPlaylist, m.tracklist.Index())
			cmds = append(cmds, cmd)
		case tracklist.SEARCH:
			m.searchDialog.Title = "Search"
			m.searchDialog.Action = "search"
			m.isSearchActive = true
			m.Send(search.UPDATE_SUGGESTIONS)
		case tracklist.SHUFFLE:
			cmd = m.shufflePlaylist(m.playlists.SelectedItem())
			cmds = append(cmds, cmd)
		case tracklist.SHARE:
			link := api.ShareTrackLink(m.tracklist.SelectedItem().Track)
			if link != "" {
				m.clipboard.CopyText(link)
			}
		}

	// player control update
	case tracker.Control:
		switch msg {
		case tracker.NEXT:
			m.nextTrack()
		case tracker.PREV:
			m.prevTrack()
		case tracker.LIKE:
			cmd = m.likePlayingTrack()
			cmds = append(cmds, cmd)
		case tracker.PLAY, tracker.PAUSE:
			m.mediaHandler.OnPlayPause()
		case tracker.STOP:
			m.mediaHandler.OnEnded()
		case tracker.REWIND:
			m.mediaHandler.OnSeek(m.tracker.Position())
		case tracker.VOLUME:
			m.mediaHandler.OnVolume()
		case tracker.CACHE_TRACK:
			cmd = m.cacheCurrentTrack()
			cmds = append(cmds, cmd)
		case tracker.BUFFERING_COMPLETE:
			cacheMode := config.Current.CacheTracks
			if cacheMode == config.CACHE_ALL || (cacheMode == config.CACHE_LIKED_ONLY && m.likedTracksMap[m.tracker.CurrentTrack().Id]) {
				cmd = m.cacheCurrentTrack()
				cmds = append(cmds, cmd)
			}
		}

		m.tracker, cmd = m.tracker.Update(message)
		cmds = append(cmds, cmd)

	// search control update
	case search.Control:
		if m.isSearchActive {
			cmd = m.searchControl(msg)
			cmds = append(cmds, cmd)
		} else if m.isAddPlaylistActive {
			cmd = m.addPlaylistControl(msg)
			cmds = append(cmds, cmd)
		}

	// input dialog control update
	case input.Control:
		m.isRenamePlaylistActive = false
		cmd = m.renamePlaylistControl(msg)
		cmds = append(cmds, cmd)

	// system media handler commands, applied here on the Bubble Tea goroutine
	// (mediaHandle forwards them as messages instead of touching the tracker).
	case mediaPlayPauseMsg:
		if m.tracker.IsPlaying() {
			cmds = append(cmds, model.Cmd(tracker.PAUSE))
		} else {
			cmds = append(cmds, model.Cmd(tracker.PLAY))
		}
	case mediaSeekMsg:
		cmd = m.tracker.Rewind(time.Duration(msg))
		cmds = append(cmds, cmd)
	case mediaSetPosMsg:
		m.tracker.SetPos(time.Duration(msg))
	case mediaSetVolumeMsg:
		m.tracker.SetVolume(float64(msg))
	case mediaShuffleMsg:
		if m.currentPlaylistIndex >= 0 && m.currentPlaylistIndex < len(m.playlists.Items()) {
			currentPlaylist := m.playlists.Items()[m.currentPlaylistIndex]
			if len(currentPlaylist.Tracks) > 0 && currentPlaylist.Kind >= playlist.LIKES {
				cmd = m.shufflePlaylist(currentPlaylist)
				cmds = append(cmds, cmd)
			}
		}

	default:
		if m.isLoading {
			m.spinner, cmd = m.spinner.Update(message)
			cmds = append(cmds, cmd)
		} else if m.isSearchActive || m.isAddPlaylistActive {
			m.searchDialog, cmd = m.searchDialog.Update(message)
			cmds = append(cmds, cmd)
		} else if m.isRenamePlaylistActive {
			m.inputDialog, cmd = m.inputDialog.Update(message)
			cmds = append(cmds, cmd)
		} else {
			m.playlists, cmd = m.playlists.Update(message)
			cmds = append(cmds, cmd)
			m.tracklist, cmd = m.tracklist.Update(message)
			cmds = append(cmds, cmd)
			m.tracker, cmd = m.tracker.Update(message)
			cmds = append(cmds, cmd)
		}
	}

	m.refreshMediaSnapshot()
	return m, tea.Batch(cmds...)
}

func (m *Model) View() string {
	if m.isLoading {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, m.spinner.View())
	}

	if m.isSearchActive || m.isAddPlaylistActive {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, m.searchDialog.View())
	} else if m.isRenamePlaylistActive {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, m.inputDialog.View())
	}

	playlistView := m.playlists.View()
	playlistWidth := lipgloss.Width(playlistView)

	m.tracker.SetWidth(m.width - playlistWidth - 2)
	m.tracklist.SetWidth(m.width - playlistWidth - 2)

	trackerView := m.tracker.View()
	trackerHeight := lipgloss.Height(trackerView)
	m.tracklist.SetHeight(m.height - trackerHeight - 2)

	tracklistView := m.tracklist.View()

	var midPanel string
	if m.tracklist.Hidden {
		midPanel = trackerView
	} else if m.tracker.Hidden {
		midPanel = tracklistView
	} else {
		midPanel = lipgloss.JoinVertical(lipgloss.Left, tracklistView, trackerView)
	}

	return lipgloss.JoinHorizontal(lipgloss.Bottom, playlistView, midPanel)
}

//
// private methods
//

func (m *Model) resize(width, height int) {
	m.width, m.height = width, height

	m.playlists.SetSize(style.SidePanelWidth, height-4)
	if !m.isPlaylistHideOverride {
		m.playlists.Hidden = m.width < style.SidePanelAutohide
	}

	searchWidth := style.SearchModalWidth
	if searchWidth > m.width {
		searchWidth = m.width - 2
	}

	m.searchDialog.SetSize(searchWidth, m.height-4)
	m.inputDialog.SetWidth(searchWidth)
}

// loadData performs all start-up network and disk I/O on a background goroutine
// (it is used as a tea.Cmd) and returns the result as a message. It must not
// touch the model directly — applyLoadedData applies the result inside Update,
// on the Bubble Tea goroutine.
func (m *Model) loadData() tea.Msg {
	var result dataLoadedMsg

	// The local cache is available regardless of authentication.
	if cached, err := cache.ListTracks(); err != nil {
		log.Print(log.LVL_ERROR, "failed to list cached tracks: %s", err)
		result.errLabel = "cache list"
	} else {
		result.cachedTracks = cached
		result.cachedIds = make(map[string]bool, len(cached))
		for i := range cached {
			result.cachedIds[cached[i].Id] = true
		}
	}

	if len(config.Current.Token) == 0 {
		log.Print(log.LVL_ERROR, "missing client token, check the config file at '%s'", config.Path())
		result.errLabel = "missing token"
		return result
	}

	client, err := api.NewClient(config.DirName, config.Current.Token)
	if err != nil {
		if _, ok := err.(*url.Error); ok {
			log.Print(log.LVL_ERROR, "failed to connect to the Yandex server: %s", err)
			result.errLabel = "unable to connect to the Yandex server"
		} else {
			log.Print(log.LVL_ERROR, "client init error: %s", err)
			result.errLabel = "unable to login: " + err.Error()
		}
		return result
	}
	result.client = client

	// My Wave rotor session.
	if session, err := client.RotorNewSession(api.MyWaveId); err != nil {
		log.Print(log.LVL_ERROR, "unable to init rotor session: %s", err)
		result.errLabel = "unable to init rotor session"
	} else {
		result.wave = &session
	}

	// Liked tracks (ids + full info).
	if likes, err := client.LikedTracks(); err != nil {
		log.Print(log.LVL_ERROR, "failed to obtain liked tracks for the first time: %s", err)
		result.errLabel = "liked tracks"
	} else {
		likedTracksId := make([]string, len(likes))
		result.likedIds = make(map[string]bool, len(likes))
		for l := range likes {
			result.likedIds[likes[l].Id] = true
			likedTracksId[l] = likes[l].Id
		}
		if full, err := client.Tracks(likedTracksId); err != nil {
			log.Print(log.LVL_ERROR, "failed to obtain liked tracks full info: %s", err)
			result.errLabel = "liked tracks info"
		} else {
			result.likedTracks = full
		}
	}

	// User playlists.
	if playlists, err := client.ListPlaylists(); err != nil {
		log.Print(log.LVL_ERROR, "failed to obtain user playlists: %s", err)
		result.errLabel = "playlists"
	} else {
		for _, pl := range playlists {
			playlistTracks, err := client.PlaylistTracks(pl.Kind, pl.Owner.Uid, false)
			if err != nil {
				log.Print(log.LVL_ERROR, "failed to obtain playlist [%s] tracks: %s", pl.Title, err)
				result.errLabel = "playlist tracks"
				continue
			}
			result.userPlaylists = append(result.userPlaylists, &playlist.Item{
				Name:     pl.Title,
				Kind:     pl.Kind,
				Revision: pl.Revision,
				Active:   true,
				Subitem:  true,
				Tracks:   playlistTracks,
			})
		}
	}

	return result
}

// applyLoadedData writes the result of loadData into the model. It runs inside
// Update, so every mutation here happens on the Bubble Tea goroutine.
func (m *Model) applyLoadedData(data dataLoadedMsg) {
	m.client = data.client

	if data.likedIds != nil {
		m.likedTracksMap = data.likedIds
	}
	if data.cachedIds != nil {
		m.cachedTracksMap = data.cachedIds
	}

	for i, station := range m.playlists.Items() {
		switch station.Kind {
		case playlist.MYWAVE:
			if data.wave == nil {
				continue
			}
			station.StationId = data.wave.Id
			station.SessionId = data.wave.RadioSessionId
			station.SessionBatch = data.wave.BatchId
			if len(data.wave.Sequence) > 0 {
				station.Tracks = []api.Track{data.wave.Sequence[0].Track}
			}
			m.playlists.SetItem(i, station)
		case playlist.LIKES:
			if data.likedTracks == nil {
				continue
			}
			station.Tracks = data.likedTracks
			m.playlists.SetItem(i, station)
		case playlist.LOCAL:
			if data.cachedTracks == nil {
				// cache.ListTracks failed — leave the station as-is rather than
				// blanking it (matches MYWAVE/LIKES and the original behavior).
				continue
			}
			station.Tracks = data.cachedTracks
			m.playlists.SetItem(i, station)
		}
	}

	for _, pl := range data.userPlaylists {
		m.playlists.InsertItem(-1, pl)
	}

	m.currentPlaylistIndex = -1
	m.playlists.Select(0)

	if data.errLabel != "" {
		m.tracker.ShowError(data.errLabel)
	}
}

// mediaHandle services the system media handler from its own goroutine. Inbound
// commands are forwarded as messages so they are applied to the tracker inside
// Update (never from here); synchronous queries are answered from the snapshot
// that Update keeps fresh. This goroutine therefore never touches live tracker
// or playlist state, which removes the data races with the Update loop.
func (m *Model) mediaHandle() {
	for msg := range m.mediaHandler.Message() {
		switch msg.Type {
		case handler.MSG_NEXT:
			m.Send(tracker.NEXT)
		case handler.MSG_PREVIOUS:
			m.Send(tracker.PREV)
		case handler.MSG_PLAY:
			m.Send(tracker.PLAY)
		case handler.MSG_PAUSE:
			m.Send(tracker.PAUSE)
		case handler.MSG_PLAYPAUSE:
			m.Send(mediaPlayPauseMsg{})
		case handler.MSG_STOP:
			m.Send(tracker.STOP)
		case handler.MSG_SEEK:
			if offset, ok := msg.Arg.(time.Duration); ok {
				m.Send(mediaSeekMsg(offset))
			}
		case handler.MSG_SETPOS:
			if pos, ok := msg.Arg.(time.Duration); ok {
				m.Send(mediaSetPosMsg(pos))
			}
		case handler.MSG_SET_SHUFFLE:
			if val, ok := msg.Arg.(bool); ok && val {
				m.Send(mediaShuffleMsg{})
			}
		case handler.MSG_SET_VOLUME:
			if vol, ok := msg.Arg.(float64); ok {
				m.Send(mediaSetVolumeMsg(vol))
			}

		case handler.MSG_GET_PLAYBACKSTATUS:
			m.mediaHandler.SendAnswer(m.mediaPlaybackState())
		case handler.MSG_GET_SHUFFLE:
			m.mediaHandler.SendAnswer(false)
		case handler.MSG_GET_METADATA:
			m.mediaHandler.SendAnswer(m.mediaMetadata())
		case handler.MSG_GET_VOLUME:
			m.mediaHandler.SendAnswer(m.mediaVolume())
		case handler.MSG_GET_POSITION:
			m.mediaHandler.SendAnswer(m.mediaPosition())
		}
	}
}

// refreshMediaSnapshot copies the current player state into the mutex-guarded
// snapshot read by mediaHandle. It runs at the end of Update, so every read of
// the tracker here happens on the Bubble Tea goroutine.
func (m *Model) refreshMediaSnapshot() {
	var state handler.PlaybackState
	switch {
	case m.tracker.IsPlaying():
		state = handler.STATE_PLAYING
	case m.tracker.IsStoped():
		state = handler.STATE_STOPED
	default:
		state = handler.STATE_PAUSED
	}
	volume := m.tracker.Volume()
	position := m.tracker.Position()

	m.mediaMu.Lock()
	m.mediaSnap.state = state
	m.mediaSnap.volume = volume
	m.mediaSnap.position = position
	if state == handler.STATE_STOPED {
		m.mediaSnap.metadata = handler.TrackMetadata{}
	}
	m.mediaMu.Unlock()
}

// setMediaMetadata stores the now-playing metadata in the snapshot. Called from
// playTrack (on the Update goroutine) before notifying the media handler.
func (m *Model) setMediaMetadata(track *api.Track) {
	md := m.buildTrackMetadata(track)
	m.mediaMu.Lock()
	m.mediaSnap.metadata = md
	m.mediaMu.Unlock()
}

func (m *Model) buildTrackMetadata(track *api.Track) handler.TrackMetadata {
	artists := make([]string, 0, len(track.Artists))
	for i := range track.Artists {
		artists = append(artists, track.Artists[i].Name)
	}
	albumArtists := make([]string, 0)
	var albumName string
	genre := make([]string, 0)
	if len(track.Albums) != 0 {
		for i := range track.Albums[0].Artists {
			albumArtists = append(albumArtists, track.Albums[0].Artists[i].Name)
		}
		albumName = track.Albums[0].Title
		genre = append(genre, track.Albums[0].Genre)
	}

	return handler.TrackMetadata{
		TrackId:      track.Id,
		Length:       time.Duration(track.DurationMs) * time.Millisecond,
		CoverUrl:     m.coverFilePath(track),
		AlbumName:    albumName,
		AlbumArtists: albumArtists,
		Artists:      artists,
		Genre:        genre,
		Title:        track.Title,
		Url:          api.ShareTrackLink(track),
	}
}

func (m *Model) mediaPlaybackState() handler.PlaybackState {
	m.mediaMu.RLock()
	defer m.mediaMu.RUnlock()
	return m.mediaSnap.state
}

func (m *Model) mediaVolume() float64 {
	m.mediaMu.RLock()
	defer m.mediaMu.RUnlock()
	return m.mediaSnap.volume
}

func (m *Model) mediaPosition() time.Duration {
	m.mediaMu.RLock()
	defer m.mediaMu.RUnlock()
	return m.mediaSnap.position
}

func (m *Model) mediaMetadata() handler.TrackMetadata {
	m.mediaMu.RLock()
	defer m.mediaMu.RUnlock()
	return m.mediaSnap.metadata
}

func (m *Model) coverFilePath(track *api.Track) string {
	tempDir := filepath.Join(os.TempDir(), config.DirName)
	if os.MkdirAll(tempDir, 0755) != nil {
		return ""
	}
	return filepath.Join(tempDir, track.Id+".jpg")
}

func (m *Model) metadataFilePath() string {
	tempDir := filepath.Join(os.TempDir(), config.DirName)
	if os.MkdirAll(tempDir, 0755) != nil {
		return ""
	}
	return filepath.Join(tempDir, "metadata.mp3")
}
