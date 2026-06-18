package mainpage

import (
	"os"

	"github.com/bogem/id3v2/v2"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/lillink13/yamusic-tui/api"
	"github.com/lillink13/yamusic-tui/cache"
	"github.com/lillink13/yamusic-tui/log"
	"github.com/lillink13/yamusic-tui/ui/components/playlist"
)

func (m *Model) cacheCurrentTrack() tea.Cmd {
	currentTrack := m.tracker.CurrentTrack()
	if m.tracker.IsStoped() || m.cachedTracksMap[currentTrack.Id] {
		return nil
	}

	buf := m.tracker.TrackBuffer()
	if buf == nil || !buf.IsBuffered() {
		// Caching a partially-buffered track yields a truncated file that plays
		// for ~1s and then skips; only cache once the whole track is buffered.
		return nil
	}

	metadataFile, err := os.OpenFile(m.metadataFilePath(), os.O_RDONLY, 0755)
	if err != nil {
		log.Print(log.LVL_ERROR, "failed to open metadata file: %s", err)
		m.tracker.ShowError("cache open")
		return nil
	}

	defer metadataFile.Close()

	cacheFile, err := cache.Write(currentTrack.Id)
	if err != nil {
		log.Print(log.LVL_ERROR, "failed to write cache file: %s", err)
		m.tracker.ShowError("cache write")
		return nil
	}

	tag := id3v2.NewEmptyTag()
	tag.Reset(metadataFile, id3v2.Options{Parse: true})
	if _, err = tag.WriteTo(cacheFile); err == nil {
		_, err = buf.WriteTo(cacheFile)
	}
	if cerr := cacheFile.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = cache.Commit(currentTrack.Id)
	}
	if err != nil {
		cache.Discard(currentTrack.Id)
		log.Print(log.LVL_ERROR, "failed to cache track: %s", err)
		m.tracker.ShowError("cache write")
		return nil
	}

	m.cachedTracksMap[currentTrack.Id] = true
	cachePlaylist, index := m.playlists.GetFirst(playlist.LOCAL)
	cachePlaylist.AddTrack(currentTrack)
	cmd := m.playlists.SetItem(index, cachePlaylist)

	if m.playlists.SelectedItem().Kind == playlist.LOCAL {
		m.displayPlaylist(cachePlaylist)
	}

	m.indicateCurrentTrackPlaying(m.tracker.IsPlaying())
	return cmd
}

// dropCorruptCache removes a truncated cache entry so the track is streamed
// fresh. Unlike removeCache it makes no UI fuss and leaves the LOCAL playlist
// entry in place (it self-corrects on the next cache listing once the file is
// gone); clearing cachedTracksMap drops the "cached" indicator.
func (m *Model) dropCorruptCache(track *api.Track) {
	cache.Remove(track.Id)
	cache.Discard(track.Id)
	delete(m.cachedTracksMap, track.Id)
}

func (m *Model) removeCache(track *api.Track) tea.Cmd {
	if m.tracker.CurrentTrack().Id == track.Id && len(m.tracker.CurrentTrack().RealId) == 0 {
		m.tracker.ShowError("can't remove currently playing track")
		return nil
	}

	err := cache.Remove(track.Id)
	if err != nil {
		log.Print(log.LVL_ERROR, "failed to remove cached file: %s", err)
		m.tracker.ShowError("cache remove")
		return nil
	}

	cachePlaylist, index := m.playlists.GetFirst(playlist.LOCAL)
	cachePlaylist.RemoveTrack(track.Id)

	delete(m.cachedTracksMap, track.Id)
	cmd := m.playlists.SetItem(index, cachePlaylist)

	if m.playlists.SelectedItem().Kind == playlist.LOCAL {
		m.displayPlaylist(cachePlaylist)
	}

	m.indicateCurrentTrackPlaying(m.tracker.IsPlaying())
	return cmd
}
