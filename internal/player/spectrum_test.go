package player

import (
	"github.com/go-musicfox/go-musicfox/internal/types"
	"math"
	"testing"
	"time"

	"github.com/gopxl/beep"
)

func tone(hz float64) [2][spectrumWindow]float64 {
	var samples [2][spectrumWindow]float64
	for i := range samples[0] {
		samples[0][i] = math.Sin(2 * math.Pi * hz * float64(i) / 44100)
		samples[1][i] = -samples[0][i]
	}
	return samples
}
func peakBand(levels [SpectrumBands]float64) int {
	best := 0
	for i := range levels {
		if levels[i] > levels[best] {
			best = i
		}
	}
	return best
}
func TestSpectrumRespondsToFrequencyAndSilence(t *testing.T) {
	transform := newSpectrumTransform(44100)
	var silent [2][spectrumWindow]float64
	if levels := transform.Levels(&silent); levels != [SpectrumBands]float64{} {
		t.Fatal("silence generated energy")
	}
	low, high := tone(440), tone(4000)
	lo, hi := transform.Levels(&low), transform.Levels(&high)
	if peakBand(lo) >= peakBand(hi) {
		t.Fatal("frequency increase did not move peak")
	}
	if lo[peakBand(lo)] < 0.9 || hi[peakBand(hi)] < 0.9 {
		t.Fatal("antiphase stereo lost energy")
	}
	for _, tc := range []struct {
		hz     float64
		levels [SpectrumBands]float64
	}{{440, lo}, {4000, hi}} {
		band := peakBand(tc.levels)
		bin := int(math.Round(tc.hz * spectrumWindow / 44100))
		if bin < transform.starts[band] || bin >= transform.ends[band] {
			t.Fatalf("%v Hz peaked in wrong band %d", tc.hz, band)
		}
	}
}
func TestSpectrumTapPreservesPCMAndPauseClearsFrame(t *testing.T) {
	a := newSpectrumAnalyzer(44100)
	defer a.Close()
	a.SetActive(true)
	offset := 0
	tap := spectrumTap{analyzer: a, source: beep.StreamerFunc(func(samples [][2]float64) (int, bool) {
		for i := range samples {
			v := math.Sin(2 * math.Pi * 1000 * float64(offset) / 44100)
			samples[i] = [2]float64{v, -v}
			offset++
		}
		return len(samples), true
	})}
	samples := make([][2]float64, 4410)
	n, ok := tap.Stream(samples)
	if n != len(samples) || !ok || tap.Err() != nil {
		t.Fatal("tap changed streamer result")
	}
	for i, s := range samples {
		v := math.Sin(2 * math.Pi * 1000 * float64(i) / 44100)
		if s != [2]float64{v, -v} {
			t.Fatal("tap changed audio")
		}
	}
	deadline := time.After(time.Second)
	for a.Snapshot().UpdatedAt.IsZero() {
		select {
		case <-deadline:
			t.Fatal("no PCM analysis result")
		case <-time.After(time.Millisecond):
		}
	}
	a.SetActive(false)
	for range 20 {
		if f := a.Snapshot(); !f.UpdatedAt.IsZero() || f.Levels != [SpectrumBands]float64{} {
			t.Fatal("old audio republished after pause")
		}
		time.Sleep(time.Millisecond)
	}
	a.Close()
}
func BenchmarkSpectrumTransform(b *testing.B) {
	transform := newSpectrumTransform(44100)
	samples := tone(1000)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		transform.Levels(&samples)
	}
}
func BenchmarkSpectrumCapture(b *testing.B) {
	a := newSpectrumAnalyzer(44100)
	defer a.Close()
	a.SetActive(true)
	samples := make([][2]float64, 441)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		a.Push(samples)
	}
}

func TestDisabledSpectrumDoesNotNeedAnalyzer(t *testing.T) {
	var analyzer *spectrumAnalyzer
	analyzer.SetActive(true)
	if frame := analyzer.Snapshot(); frame.Supported || !frame.UpdatedAt.IsZero() {
		t.Fatal("nil analyzer advertises spectrum")
	}
	analyzer.Close()
}

func TestBeepSpectrumToggleDoesNotRestartPlayback(t *testing.T) {
	p := &beepPlayer{state: types.Playing, spectrum: newSpectrumAnalyzer(44100)}
	defer p.spectrum.Close()
	for _, enabled := range []bool{true, false, true, false} {
		p.SetSpectrumEnabled(enabled)
		if p.state != types.Playing || p.spectrum.active.Load() != enabled {
			t.Fatal("toggle changed playback or failed to control sampling")
		}
		if !enabled && !p.Spectrum().UpdatedAt.IsZero() {
			t.Fatal("disabled spectrum retained old frame")
		}
	}
}
