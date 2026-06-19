package playlist

import "github.com/lillink13/yamusic-tui/api"

type Item struct {
	Uid uint64 // owner uid for a liked (foreign) playlist; unused otherwise

	Name         string
	Kind         uint64
	Revision     int
	StationId    api.StationId
	SessionBatch string
	SessionId    string
	Active       bool
	Subitem      bool
	Rotor        bool
	Collapsible  bool      // a section header that folds the items below it
	Collapsed    bool      //
	Section      SectionId // which collapsible section this item belongs to
	ResId        uint64    // liked-collection resource id: playlist kind / album id / artist id

	Tracks        []api.Track
	CurrentTrack  int
	SelectedTrack int
}

func (i *Item) FilterValue() string {
	return i.Name
}

func (i *Item) IsSame(other *Item) bool {
	// Stations all share Kind==STATION and may have duplicate display names, so
	// identify them by StationId.
	if i.Kind == STATION || other.Kind == STATION {
		return i.Kind == other.Kind && i.StationId == other.StationId
	}
	// Liked playlists/artists/albums all carry Kind==NONE and may share display
	// names too, so identify them by their section + resource id (owner uid plus
	// the playlist kind / album id / artist id).
	if i.Section != SectionNone || other.Section != SectionNone {
		return i.Section == other.Section && i.Uid == other.Uid && i.ResId == other.ResId
	}
	// Everything else (built-ins, the user's own playlists) has a unique Kind+Name.
	return i.Kind == other.Kind && i.Name == other.Name
}

func (pl *Item) AddTrack(track *api.Track) {
	pl.Tracks = append([]api.Track{*track}, pl.Tracks...)
	pl.SelectedTrack++
	if pl.CurrentTrack < len(pl.Tracks)-1 {
		pl.CurrentTrack++
	}
}

func (pl *Item) AddTrackToEnd(track *api.Track) {
	pl.Tracks = append(pl.Tracks, *track)
}

func (pl *Item) RemoveTrack(trackId string) int {
	for i, ltrack := range pl.Tracks {
		if ltrack.Id == trackId {
			if i+1 < len(pl.Tracks) {
				pl.Tracks = append(pl.Tracks[:i], pl.Tracks[i+1:]...)
			} else {
				pl.Tracks = pl.Tracks[:i]
			}

			if len(pl.Tracks) == 0 {
				pl.SelectedTrack = 0
				pl.CurrentTrack = 0
			} else {
				if pl.SelectedTrack > i {
					pl.SelectedTrack--
				} else if pl.SelectedTrack >= len(pl.Tracks) {
					pl.SelectedTrack = len(pl.Tracks) - 1
				}

				if pl.CurrentTrack == i {
					pl.CurrentTrack = len(pl.Tracks)
				} else if pl.CurrentTrack > i {
					pl.CurrentTrack--
				}
			}

			return i
		}
	}
	return -1
}
