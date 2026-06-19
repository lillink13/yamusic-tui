package tracker

import (
	"math"
	"strings"
)

const (
	_VIZ_FFT_SIZE = 1024 // PCM window fed to the FFT (must be a power of two)
	_VIZ_HEIGHT   = 5    // rows of the spectrum panel
)

// _vizBlocks indexes the eight partial-block heights (index 0 is a space).
var _vizBlocks = []rune(" ▁▂▃▄▅▆▇█")

// fft computes the in-place radix-2 Cooley-Tukey FFT of re/im. len(re) must be a
// power of two and len(im) == len(re).
func fft(re, im []float64) {
	n := len(re)
	if n <= 1 {
		return
	}

	// bit-reversal permutation
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			re[i], re[j] = re[j], re[i]
			im[i], im[j] = im[j], im[i]
		}
	}

	for length := 2; length <= n; length <<= 1 {
		ang := -2 * math.Pi / float64(length)
		wre, wim := math.Cos(ang), math.Sin(ang)
		half := length >> 1
		for i := 0; i < n; i += length {
			cwre, cwim := 1.0, 0.0
			for k := 0; k < half; k++ {
				j := i + k + half
				vre := re[j]*cwre - im[j]*cwim
				vim := re[j]*cwim + im[j]*cwre
				re[j], im[j] = re[i+k]-vre, im[i+k]-vim
				re[i+k], im[i+k] = re[i+k]+vre, im[i+k]+vim
				cwre, cwim = cwre*wre-cwim*wim, cwre*wim+cwim*wre
			}
		}
	}
}

// spectrumMagnitudes turns a mono PCM window (samples in [-1,1]) into `cols`
// log-frequency-bucketed magnitudes. A Hann window is applied before the FFT and
// the per-bucket magnitude is log-compressed; the values are NOT normalized to a
// fixed range (the caller applies its own gain/AGC), but silence yields zeros.
// It allocates fresh FFT scratch; the hot path uses spectrumInto instead.
func spectrumMagnitudes(samples []float64, cols int) []float64 {
	return spectrumInto(samples, cols, make([]float64, _VIZ_FFT_SIZE), make([]float64, _VIZ_FFT_SIZE))
}

// spectrumInto is spectrumMagnitudes with caller-provided FFT scratch (re and im
// must each have capacity for _VIZ_FFT_SIZE) so the per-frame hot path allocates
// nothing but the small result slice. re is fully overwritten and im is zeroed.
func spectrumInto(samples []float64, cols int, re, im []float64) []float64 {
	out := make([]float64, cols)
	if cols <= 0 || len(samples) == 0 {
		return out
	}

	n := _VIZ_FFT_SIZE
	re = re[:n]
	im = im[:n]
	start := len(samples) - n
	for i := 0; i < n; i++ {
		im[i] = 0 // clear stale scratch before the transform
		var s float64
		if idx := start + i; idx >= 0 && idx < len(samples) {
			s = samples[idx]
		}
		// Hann window to reduce spectral leakage.
		w := 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(n-1))
		re[i] = s * w
	}

	fft(re, im)

	half := n / 2
	for c := 0; c < cols; c++ {
		// Log-spaced bin range so low frequencies (where music lives) get more
		// columns than the sparse highs.
		lo := int(math.Pow(float64(half), float64(c)/float64(cols)))
		hi := int(math.Pow(float64(half), float64(c+1)/float64(cols)))
		if lo < 1 {
			lo = 1 // skip the DC bin
		}
		if hi <= lo {
			hi = lo + 1
		}
		if hi > half {
			hi = half
		}

		var sum float64
		cnt := 0
		for b := lo; b < hi; b++ {
			sum += math.Hypot(re[b], im[b])
			cnt++
		}
		if cnt > 0 {
			out[c] = math.Log10(1 + sum/float64(cnt))
		}
	}

	return out
}

// renderSpectrum draws bar levels (each in [0,1]) as a height-row block panel.
// Bars grow from the bottom; every row is exactly len(bars) runes wide so the
// panel is a stable rectangle.
func renderSpectrum(bars []float64, height int) string {
	if height <= 0 || len(bars) == 0 {
		return ""
	}

	maxEighths := height * 8
	rows := make([]string, height)
	var sb strings.Builder
	for row := 0; row < height; row++ {
		sb.Reset()
		baseFromBottom := (height - 1 - row) * 8 // eighths below this row
		for _, b := range bars {
			if b < 0 {
				b = 0
			} else if b > 1 {
				b = 1
			}
			fill := int(b*float64(maxEighths)) - baseFromBottom
			switch {
			case fill <= 0:
				sb.WriteRune(_vizBlocks[0])
			case fill >= 8:
				sb.WriteRune(_vizBlocks[8])
			default:
				sb.WriteRune(_vizBlocks[fill])
			}
		}
		rows[row] = sb.String()
	}

	return strings.Join(rows, "\n")
}
