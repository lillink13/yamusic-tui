package stream

import (
	"bytes"
	"io"
	"sync"
	"testing"
)

// nopCloser adapts a bytes.Reader to io.ReadCloser without invalidating it on
// Close, so the test can keep exercising Read/Seek after the stream "closes".
type nopCloser struct{ *bytes.Reader }

func (nopCloser) Close() error { return nil }

func newTestStream(size int) *BufferedStream {
	data := make([]byte, size)
	return NewBufferedStream(nopCloser{bytes.NewReader(data)}, int64(size))
}

// Progress must reflect the read position, served from the lock-free shadow
// index rather than the mutex-guarded readIndex.
func TestProgressTracksReadPosition(t *testing.T) {
	const size = 4096
	bs := newTestStream(size)
	defer bs.Close()

	if p := bs.Progress(); p != 0 {
		t.Fatalf("initial progress = %v, want 0", p)
	}

	buf := make([]byte, size/4)
	if _, err := io.ReadFull(bs, buf); err != nil {
		t.Fatalf("ReadFull: %v", err)
	}
	if p := bs.Progress(); p < 0.2 || p > 0.3 {
		t.Fatalf("progress after reading 1/4 = %v, want ~0.25", p)
	}

	if _, err := bs.Seek(size/2, io.SeekStart); err != nil {
		t.Fatalf("Seek: %v", err)
	}
	if p := bs.Progress(); p < 0.45 || p > 0.55 {
		t.Fatalf("progress after seek to 1/2 = %v, want ~0.5", p)
	}
}

// Reading Progress/IsDone/IsBuffered (the UI goroutine) concurrently with
// Read/Seek (the audio goroutine) must not race. Run with -race to catch a
// regression of the unlocked readIndex read this shadow index replaced.
func TestProgressConcurrentWithReadSeek(t *testing.T) {
	bs := newTestStream(1 << 16)
	defer bs.Close()

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				_ = bs.Progress()
				_ = bs.IsDone()
				_ = bs.IsBuffered()
			}
		}
	}()

	buf := make([]byte, 512)
	for i := 0; i < 200; i++ {
		_, _ = bs.Read(buf)
		_, _ = bs.Seek(int64((i*256)%(1<<16)), io.SeekStart)
	}

	close(stop)
	wg.Wait()
}
