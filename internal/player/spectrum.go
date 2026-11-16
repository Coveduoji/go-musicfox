package player

import (
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gopxl/beep"
	"gonum.org/v1/gonum/dsp/fourier"
)

const (
	SpectrumBands      = 24
	spectrumWindow     = 2048
	spectrumAnalysisHz = 20
)

type SpectrumFrame struct {
	Levels    [SpectrumBands]float64
	UpdatedAt time.Time
	Supported bool
}

// SpectrumProvider is optional; external playback engines need not expose PCM.
type SpectrumProvider interface{ Spectrum() SpectrumFrame }

// SpectrumController is optional for engines with local PCM access.
type SpectrumController interface{ SetSpectrumEnabled(bool) }

type spectrumWindowFrame struct {
	samples [2][spectrumWindow]float64
	epoch   uint64
}

// spectrumAnalyzer never performs FFT or waits for the worker on the audio path.
type spectrumAnalyzer struct {
	active    atomic.Bool
	epoch     atomic.Uint64
	queue     chan spectrumWindowFrame
	done      chan struct{}
	closeOnce sync.Once
	startOnce sync.Once
	mu        sync.RWMutex
	frame     SpectrumFrame
	rate      int
	// The following fields belong exclusively to the audio callback.
	ring                    [2][spectrumWindow]float64
	position, filled, since int
	captureEpoch            uint64
}

func newSpectrumAnalyzer(rate int) *spectrumAnalyzer {
	a := &spectrumAnalyzer{rate: rate, queue: make(chan spectrumWindowFrame, 1), done: make(chan struct{}), frame: SpectrumFrame{Supported: true}}
	return a
}
func (a *spectrumAnalyzer) SetActive(active bool) {
	if a == nil {
		return
	}
	a.active.Store(false)
	a.epoch.Add(1)
	a.mu.Lock()
	a.frame = SpectrumFrame{Supported: true}
	a.mu.Unlock()
	a.active.Store(active)
	if active {
		a.startOnce.Do(func() { go a.run() })
	}
}
func (a *spectrumAnalyzer) Close() {
	if a != nil {
		a.closeOnce.Do(func() { a.SetActive(false); close(a.done) })
	}
}
func (a *spectrumAnalyzer) Snapshot() SpectrumFrame {
	if a == nil {
		return SpectrumFrame{}
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.frame
}

func (a *spectrumAnalyzer) Push(samples [][2]float64) {
	if !a.active.Load() {
		return
	}
	epoch := a.epoch.Load()
	if epoch != a.captureEpoch {
		a.position = 0
		a.filled = 0
		a.since = 0
		a.captureEpoch = epoch
	}
	for _, sample := range samples {
		a.ring[0][a.position] = sample[0]
		a.ring[1][a.position] = sample[1]
		a.position = (a.position + 1) % spectrumWindow
		a.filled = min(spectrumWindow, a.filled+1)
		a.since++
		if a.filled < spectrumWindow || a.since < a.rate/spectrumAnalysisHz {
			continue
		}
		a.since = 0
		window := spectrumWindowFrame{epoch: epoch}
		for c := range 2 {
			n := copy(window.samples[c][:], a.ring[c][a.position:])
			copy(window.samples[c][n:], a.ring[c][:a.position])
		}
		select {
		case a.queue <- window:
		default:
		}
	}
}

type spectrumTransform struct {
	fft          *fourier.FFT
	window       [spectrumWindow]float64
	sequence     [spectrumWindow]float64
	coefficients [spectrumWindow/2 + 1]complex128
	power        [spectrumWindow/2 + 1]float64
	starts, ends [SpectrumBands]int
}

func newSpectrumTransform(rate int) *spectrumTransform {
	t := &spectrumTransform{fft: fourier.NewFFT(spectrumWindow)}
	for i := range t.window {
		t.window[i] = 0.5 * (1 - math.Cos(2*math.Pi*float64(i)/float64(spectrumWindow-1)))
	}
	high := math.Min(16000, float64(rate)/2)
	for band := range SpectrumBands {
		lo := 40 * math.Pow(high/40, float64(band)/SpectrumBands)
		hi := 40 * math.Pow(high/40, float64(band+1)/SpectrumBands)
		t.starts[band] = min(spectrumWindow/2, max(1, int(math.Ceil(lo*spectrumWindow/float64(rate)))))
		t.ends[band] = min(spectrumWindow/2+1, max(t.starts[band]+1, int(math.Ceil(hi*spectrumWindow/float64(rate)))))
	}
	return t
}
func (t *spectrumTransform) Levels(samples *[2][spectrumWindow]float64) [SpectrumBands]float64 {
	clear(t.power[:])
	for channel := range 2 {
		for i, sample := range samples[channel] {
			t.sequence[i] = sample * t.window[i]
		}
		t.fft.Coefficients(t.coefficients[:], t.sequence[:])
		for i, c := range t.coefficients {
			t.power[i] += (real(c)*real(c) + imag(c)*imag(c)) / 2
		}
	}
	var levels [SpectrumBands]float64
	for band := range levels {
		peak := 0.0
		for _, power := range t.power[t.starts[band]:t.ends[band]] {
			peak = math.Max(peak, power)
		}
		amplitude := math.Sqrt(peak) * 4 / spectrumWindow
		levels[band] = max(0, min(1, (20*math.Log10(amplitude+1e-12)+60)/60))
	}
	return levels
}
func (a *spectrumAnalyzer) run() {
	transform := newSpectrumTransform(a.rate)
	var previous [SpectrumBands]float64
	var epoch uint64
	for {
		select {
		case <-a.done:
			return
		case window := <-a.queue:
			if !a.active.Load() || window.epoch != a.epoch.Load() {
				continue
			}
			if window.epoch != epoch {
				clear(previous[:])
				epoch = window.epoch
			}
			levels := transform.Levels(&window.samples)
			for i, level := range levels {
				weight := 0.25
				if level > previous[i] {
					weight = 0.75
				}
				levels[i] = previous[i] + weight*(level-previous[i])
			}
			a.mu.Lock()
			if a.active.Load() && window.epoch == a.epoch.Load() {
				a.frame = SpectrumFrame{Levels: levels, UpdatedAt: time.Now(), Supported: true}
				previous = levels
			}
			a.mu.Unlock()
		}
	}
}

type spectrumTap struct {
	source   beep.Streamer
	analyzer *spectrumAnalyzer
}

func (t *spectrumTap) Stream(samples [][2]float64) (int, bool) {
	n, ok := t.source.Stream(samples)
	t.analyzer.Push(samples[:n])
	return n, ok
}
func (t *spectrumTap) Err() error { return t.source.Err() }
