package tracker

import (
	"math"
	"strings"
	"testing"
)

func TestFFTImpulse(t *testing.T) {
	// FFT of a unit impulse is a flat spectrum of all ones.
	n := 8
	re := make([]float64, n)
	im := make([]float64, n)
	re[0] = 1
	fft(re, im)
	for i := 0; i < n; i++ {
		if math.Abs(re[i]-1) > 1e-9 || math.Abs(im[i]) > 1e-9 {
			t.Fatalf("impulse bin %d = (%v,%v), want (1,0)", i, re[i], im[i])
		}
	}
}

func TestFFTConstant(t *testing.T) {
	// FFT of a constant puts all energy in the DC bin.
	n := 8
	re := make([]float64, n)
	im := make([]float64, n)
	for i := range re {
		re[i] = 1
	}
	fft(re, im)
	if math.Abs(re[0]-float64(n)) > 1e-9 || math.Abs(im[0]) > 1e-9 {
		t.Fatalf("DC bin = (%v,%v), want (%d,0)", re[0], im[0], n)
	}
	for i := 1; i < n; i++ {
		if mag := math.Hypot(re[i], im[i]); mag > 1e-9 {
			t.Fatalf("bin %d magnitude = %v, want 0", i, mag)
		}
	}
}

func TestFFTSinusoidPeak(t *testing.T) {
	// A real cosine of integer frequency k concentrates all energy in bins k and
	// n-k (n/2 each), with the rest ~0.
	n := 64
	k := 5
	re := make([]float64, n)
	im := make([]float64, n)
	for i := 0; i < n; i++ {
		re[i] = math.Cos(2 * math.Pi * float64(k) * float64(i) / float64(n))
	}
	fft(re, im)
	for i := 0; i < n; i++ {
		mag := math.Hypot(re[i], im[i])
		if i == k || i == n-k {
			if mag < float64(n)/2-1e-3 {
				t.Fatalf("bin %d magnitude = %v, want ~%v", i, mag, n/2)
			}
		} else if mag > 1e-6 {
			t.Fatalf("bin %d magnitude = %v, want ~0", i, mag)
		}
	}
}

func TestSpectrumMagnitudesSilence(t *testing.T) {
	out := spectrumMagnitudes(make([]float64, _VIZ_FFT_SIZE), 16)
	if len(out) != 16 {
		t.Fatalf("len = %d, want 16", len(out))
	}
	for i, v := range out {
		if v != 0 {
			t.Fatalf("silence bucket %d = %v, want 0", i, v)
		}
	}
}

func TestSpectrumMagnitudesToneIsNonZero(t *testing.T) {
	n := _VIZ_FFT_SIZE
	samples := make([]float64, n)
	for i := 0; i < n; i++ {
		samples[i] = math.Sin(2 * math.Pi * 40 * float64(i) / float64(n))
	}
	out := spectrumMagnitudes(samples, 24)
	var total float64
	for _, v := range out {
		total += v
	}
	if total <= 0 {
		t.Fatalf("tone produced no spectral energy: %v", out)
	}
}

func TestSpectrumMagnitudesEdge(t *testing.T) {
	if got := spectrumMagnitudes(nil, 8); len(got) != 8 {
		t.Fatalf("nil samples: len = %d, want 8 zeros", len(got))
	}
	if got := spectrumMagnitudes(make([]float64, 10), 0); len(got) != 0 {
		t.Fatalf("cols<=0: len = %d, want 0", len(got))
	}
}

func TestSpectrumIntoReusesScratchSafely(t *testing.T) {
	n := _VIZ_FFT_SIZE
	re := make([]float64, n)
	im := make([]float64, n)
	samples := make([]float64, n)
	for i := range samples {
		samples[i] = math.Sin(2 * math.Pi * 40 * float64(i) / float64(n))
	}
	// Dirty the scratch: spectrumInto must clear im (and fully overwrite re) so the
	// reused-buffer result matches a fresh-allocation run.
	for i := range im {
		re[i] = -987.6
		im[i] = 1234.5
	}
	got := spectrumInto(samples, 24, re, im)
	want := spectrumMagnitudes(samples, 24)
	for i := range want {
		if math.Abs(got[i]-want[i]) > 1e-9 {
			t.Fatalf("bucket %d: reused-scratch %v != fresh %v", i, got[i], want[i])
		}
	}
}

func TestRenderSpectrumDimensions(t *testing.T) {
	out := renderSpectrum([]float64{0, 0.5, 1}, 4)
	rows := strings.Split(out, "\n")
	if len(rows) != 4 {
		t.Fatalf("rows = %d, want 4", len(rows))
	}
	for i, r := range rows {
		if n := len([]rune(r)); n != 3 {
			t.Fatalf("row %d width = %d, want 3 (%q)", i, n, r)
		}
	}
}

func TestRenderSpectrumZerosAndOnes(t *testing.T) {
	for _, r := range strings.ReplaceAll(renderSpectrum([]float64{0, 0}, 2), "\n", "") {
		if r != ' ' {
			t.Fatalf("zeros produced non-space %q", r)
		}
	}
	for _, r := range strings.ReplaceAll(renderSpectrum([]float64{1, 1}, 2), "\n", "") {
		if r != '█' {
			t.Fatalf("ones produced non-full-block %q", r)
		}
	}
}

func TestRenderSpectrumEdge(t *testing.T) {
	if renderSpectrum(nil, 3) != "" {
		t.Fatal("empty bars should render empty")
	}
	if renderSpectrum([]float64{0.5}, 0) != "" {
		t.Fatal("zero height should render empty")
	}
	// out-of-range bar values are clamped rather than panicking
	_ = renderSpectrum([]float64{-1, 2, 0.5}, 3)
}
