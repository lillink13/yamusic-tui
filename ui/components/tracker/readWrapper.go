package tracker

import (
	"io"
	"sync"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	mp3 "github.com/dece2183/go-stream-mp3"
	"github.com/lillink13/yamusic-tui/log"
	"github.com/lillink13/yamusic-tui/stream"
)

const (
	_PROGRESS_UPDATE_PERIOD = 33 * time.Millisecond
)

type readWrapper struct {
	program        *tea.Program
	decoder        *mp3.Decoder
	trackBuffer    *stream.BufferedStream
	trackBuffered  bool
	trackDone      bool
	lastUpdateTime time.Time

	// Latest decoded mono PCM window, tapped on the audio goroutine in Read and
	// read by the visualizer on the Update goroutine. Guarded by vizMu — the
	// audio thread never blocks for long since the window is tiny. vizEnabled
	// (atomic, toggled from the Update goroutine) lets the audio thread skip the
	// tap entirely when the visualizer is off.
	vizEnabled atomic.Bool
	vizMu      sync.Mutex
	vizSamples []float64
}

func (w *readWrapper) NewReader(reader *stream.BufferedStream) {
	var err error

	w.trackBuffered = false
	w.trackDone = false
	w.trackBuffer = reader
	w.decoder, err = mp3.NewDecoder(w.trackBuffer)
	if err != nil {
		log.Print(log.LVL_ERROR, "failed to create mp3 decoder: %s", err)
		return
	}

	w.lastUpdateTime = time.Now()
}

func (w *readWrapper) Close() {
	if w.decoder != nil {
		w.decoder.Seek(0, io.SeekStart)
	}

	if w.trackBuffer != nil {
		w.trackBuffer.Close()
	}
}

func (w *readWrapper) Read(dest []byte) (n int, err error) {
	if w.trackBuffer == nil {
		err = io.EOF
		return
	}

	n, err = w.decoder.Read(dest)
	if err != nil && err != io.EOF {
		if w.trackBuffer.Error() != nil {
			err = w.trackBuffer.Error()
			log.Print(log.LVL_ERROR, "buffering error: %s", err)
			go w.program.Send(STOP)
			return
		}
		// bypass mp3 decoding error after rewinding
		log.Print(log.LVL_WARNIGN, "mp3 decoding error: %s", err)
		err = nil
	}

	if w.vizEnabled.Load() {
		w.tapPCM(dest[:n])
	}

	if w.trackBuffer.IsBuffered() && !w.trackBuffered {
		w.trackBuffered = true
		go w.program.Send(BUFFERING_COMPLETE)
	}

	if w.trackBuffer.IsDone() && !w.trackDone {
		w.trackDone = true
		w.decoder.Seek(0, io.SeekStart)
		w.trackBuffer.Close()
		go w.program.Send(NEXT)
	} else if !w.trackDone && time.Since(w.lastUpdateTime) > _PROGRESS_UPDATE_PERIOD {
		w.lastUpdateTime = time.Now()
		fraction := ProgressControl(w.trackBuffer.Progress())
		go w.program.Send(fraction)
	}

	return
}

func (w *readWrapper) Seek(offset int64, whence int) (int64, error) {
	w.lastUpdateTime = time.Now()
	return w.decoder.Seek(offset, whence)
}

func (w *readWrapper) Length() int64 {
	return w.trackBuffer.Length()
}

func (w *readWrapper) Progress() float64 {
	return w.trackBuffer.Progress()
}

// tapPCM stores the tail of the freshly decoded buffer (interleaved s16le
// stereo, matching the oto context) as the latest mono PCM window for the
// visualizer. Called on the audio goroutine; keeps the critical section to a
// single small copy.
func (w *readWrapper) tapPCM(pcm []byte) {
	frames := len(pcm) / 4
	if frames == 0 {
		return
	}
	take := frames
	if take > _VIZ_FFT_SIZE {
		take = _VIZ_FFT_SIZE
	}
	off := (frames - take) * 4

	w.vizMu.Lock()
	if cap(w.vizSamples) < take {
		w.vizSamples = make([]float64, take)
	} else {
		w.vizSamples = w.vizSamples[:take]
	}
	for i := 0; i < take; i++ {
		l := int16(uint16(pcm[off+i*4]) | uint16(pcm[off+i*4+1])<<8)
		r := int16(uint16(pcm[off+i*4+2]) | uint16(pcm[off+i*4+3])<<8)
		w.vizSamples[i] = (float64(l) + float64(r)) / (2 * 32768)
	}
	w.vizMu.Unlock()
}

// latestPCM copies the most recent mono PCM window into dst (reusing its
// capacity) and returns it. Called on the Update goroutine.
func (w *readWrapper) latestPCM(dst []float64) []float64 {
	w.vizMu.Lock()
	dst = append(dst[:0], w.vizSamples...)
	w.vizMu.Unlock()
	return dst
}
