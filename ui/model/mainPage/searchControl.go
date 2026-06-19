package mainpage

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lillink13/yamusic-tui/api"
	"github.com/lillink13/yamusic-tui/config"
	"github.com/lillink13/yamusic-tui/log"
	"github.com/lillink13/yamusic-tui/ui/components/playlist"
	"github.com/lillink13/yamusic-tui/ui/components/search"
	"github.com/lillink13/yamusic-tui/ui/helpers"
)

func (m *Model) searchControl(msg search.Control) tea.Cmd {
	switch msg {
	case search.SELECT:
		m.isSearchActive = false

		req, ok := m.searchDialog.SuggestionValue()
		if !ok {
			return nil
		}

		// Run the search — and the per-result track fetches it triggers — off the
		// Bubble Tea goroutine so the interface doesn't freeze for the duration of
		// several HTTP round-trips. Show the loading spinner meanwhile; the result
		// arrives as a searchResultMsg.
		m.isLoading = true
		return tea.Batch(m.spinner.Tick, m.runSearch(req))
	case search.CANCEL:
		m.isSearchActive = false
	case search.UPDATE_SUGGESTIONS:
		suggestions, err := m.client.SearchSuggest(m.searchDialog.InputValue())
		if err != nil {
			log.Print(log.LVL_ERROR, "failed to obtain search [%s] suggestions: %s", m.searchDialog.InputValue(), err)
			m.tracker.ShowError("search seggestion")
			return nil
		}
		m.searchDialog.SetSuggestions(suggestions.Suggestions)
	}

	return nil
}

// runSearch performs the catalog search and builds the result items in a
// background command, returning them via searchResultMsg.
func (m *Model) runSearch(req string) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		res, err := client.Search(req, api.SEARCH_ALL)
		if err != nil {
			return searchResultMsg{req: req, err: err}
		}
		return searchResultMsg{req: req, items: buildSearchItems(client, res)}
	}
}

// buildSearchItems turns a search response into sidebar items, fetching each
// matching artist/album/playlist's tracks. It runs on the background goroutine
// (runSearch), so it only touches the API client and never mutates the model.
func buildSearchItems(client *api.YaMusicClient, res api.SearchResult) []*playlist.Item {
	var items []*playlist.Item

	// The single best match, surfaced first — but only when it is a track (the
	// Best.Result field is decoded as a Track, so for artist/album bests it would
	// be mostly empty; those still appear in the per-type results below).
	if res.Best.Type == api.SEARCH_TRACK && res.Best.Result.Title != "" {
		best := res.Best.Result
		if best.RealId != "" {
			best.Id = best.RealId
		}
		items = append(items, &playlist.Item{
			Name:    "best match: " + best.Title,
			Active:  true,
			Subitem: true,
			Tracks:  []api.Track{best},
		})
	}

	if len(res.Tracks.Results) > 0 {
		items = append(items, &playlist.Item{
			Name:    "search \"" + res.Text + "\"",
			Active:  true,
			Subitem: true,
			Tracks:  res.Tracks.Results,
		})
	}

	if config.Current.Search.Artists {
		for _, artist := range res.Artists.Results {
			if !strings.Contains(strings.ToLower(artist.Name), strings.ToLower(res.Text)) {
				continue
			}

			artistTracks, err := client.ArtistPopularTracks(artist.Id)
			if err != nil {
				log.Print(log.LVL_ERROR, "failed to obtain search [%s] artist [%s] tracks: %s", res.Text, artist.Name, err)
				continue
			}

			tracks, err := client.Tracks(artistTracks.Tracks)
			if err != nil {
				log.Print(log.LVL_ERROR, "failed to obtain search [%s] artist [%s] tracks full info: %s", res.Text, artist.Name, err)
				continue
			}

			items = append(items, &playlist.Item{
				Name:    artist.Name,
				Active:  true,
				Subitem: true,
				Tracks:  tracks,
			})
		}
	}

	if config.Current.Search.Albums {
		for _, album := range res.Albums.Results {
			if !strings.Contains(strings.ToLower(album.Title), strings.ToLower(res.Text)) {
				continue
			}

			albumWithTracks, err := client.Album(album.Id, true)
			if err != nil {
				log.Print(log.LVL_ERROR, "failed to obtain search [%s] album [%s] tracks: %s", res.Text, album.Title, err)
				continue
			}

			albumArtists := helpers.ArtistList(albumWithTracks.Artists)
			if len(albumWithTracks.Volumes) > 1 {
				for i := range albumWithTracks.Volumes {
					items = append(items, &playlist.Item{
						Name:    fmt.Sprintf("%s vol.%d (%s)", albumWithTracks.Title, i, albumArtists),
						Active:  true,
						Subitem: true,
						Tracks:  albumWithTracks.Volumes[i],
					})
				}
			} else if len(albumWithTracks.Volumes) == 1 {
				items = append(items, &playlist.Item{
					Name:    fmt.Sprintf("%s (%s)", albumWithTracks.Title, albumArtists),
					Active:  true,
					Subitem: true,
					Tracks:  albumWithTracks.Volumes[0],
				})
			}
		}
	}

	if config.Current.Search.Playlists {
		for _, pl := range res.Playlists.Results {
			if !strings.Contains(strings.ToLower(pl.Title), strings.ToLower(res.Text)) {
				continue
			}

			playlistTracks, err := client.PlaylistTracks(pl.Kind, pl.Owner.Uid, false)
			if err != nil {
				log.Print(log.LVL_ERROR, "failed to obtain search [%s] playlist [%s] tracks: %s", res.Text, pl.Title, err)
				continue
			}

			items = append(items, &playlist.Item{
				Name:    pl.Title + " by " + pl.Owner.Name,
				Active:  true,
				Subitem: true,
				Tracks:  playlistTracks,
			})
		}
	}

	return items
}

// insertSearchResults splices the freshly built search items into the sidebar
// under a "search results:" header, replacing any previous search section, and
// moves the cursor onto the first result.
func (m *Model) insertSearchResults(items []*playlist.Item) tea.Cmd {
	playlists := m.playlists.Items()
	searchResIndex := len(playlists) + 2
	for i, pl := range playlists {
		if !pl.Active && !pl.Subitem && pl.Name == "search results:" {
			playlists = playlists[:i-1]
			searchResIndex = i + 1
			break
		}
	}

	playlists = append(playlists,
		&playlist.Item{Name: "", Kind: playlist.NONE, Active: false, Subitem: false},
		&playlist.Item{Name: "search results:", Kind: playlist.NONE, Active: false, Subitem: false},
	)
	playlists = append(playlists, items...)

	cmd := m.playlists.SetItems(playlists)
	m.playlists.Select(searchResIndex)
	m.Send(playlist.CURSOR_DOWN)

	return cmd
}
