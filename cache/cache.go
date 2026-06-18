package cache

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/lillink13/yamusic-tui/config"
)

func getCacheDir() (string, error) {
	var (
		cacheDir string
		err      error
	)

	if len(config.Current.CacheDir) == 0 {
		userDir, err := os.UserCacheDir()
		if err != nil {
			return "", err
		}
		cacheDir = filepath.Join(userDir, config.DirName)
	} else {
		cacheDir, err = filepath.Abs(config.Current.CacheDir)
		if err != nil {
			return "", err
		}
	}

	err = os.MkdirAll(cacheDir, 0755)
	if err != nil {
		return "", err
	}

	return cacheDir, nil
}

func Read(trackId string) (*os.File, int64, error) {
	dir, err := getCacheDir()
	if err != nil {
		return nil, 0, err
	}

	file, err := os.Open(filepath.Join(dir, trackId+".mp3"))
	if err != nil {
		return nil, 0, err
	}

	stat, _ := file.Stat()
	return file, stat.Size(), nil
}

// Write opens a temporary file for a cache entry. The track is written here and
// only becomes a usable cache entry after Commit renames it into place, so an
// interrupted or partial write never leaves a playable but truncated file.
func Write(trackId string) (*os.File, error) {
	dir, err := getCacheDir()
	if err != nil {
		return nil, err
	}

	file, err := os.OpenFile(filepath.Join(dir, trackId+".mp3.part"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}

	return file, nil
}

// Commit atomically promotes a fully-written temporary file (from Write) to its
// final cache entry. Call it only after the complete track has been written and
// the file has been closed.
func Commit(trackId string) error {
	dir, err := getCacheDir()
	if err != nil {
		return err
	}
	return os.Rename(filepath.Join(dir, trackId+".mp3.part"), filepath.Join(dir, trackId+".mp3"))
}

// Discard removes the temporary file left by Write, e.g. after a write error.
func Discard(trackId string) error {
	dir, err := getCacheDir()
	if err != nil {
		return err
	}
	return os.Remove(filepath.Join(dir, trackId+".mp3.part"))
}

// CleanPartial removes leftover *.mp3.part temporary files from interrupted
// writes (e.g. after a crash). Intended to run once at startup, before any
// caching begins, so it never races a write in progress.
func CleanPartial() {
	dir, err := getCacheDir()
	if err != nil {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".mp3.part") {
			os.Remove(filepath.Join(dir, entry.Name()))
		}
	}
}

func Remove(trackId string) error {
	dir, err := getCacheDir()
	if err != nil {
		return err
	}

	return os.Remove(filepath.Join(dir, trackId+".mp3"))
}
