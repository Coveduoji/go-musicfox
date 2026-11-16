package ui

import (
	"github.com/go-musicfox/go-musicfox/internal/types"
	"image"
	"math"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/go-musicfox/go-musicfox/internal/player"
)

func TestSpectrumPixelsStayAboveAndBelowFixedBaseline(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 960, 60))
	var levels [player.SpectrumBands]uint8
	for _, active := range []bool{false, true, false} {
		for i := range levels {
			if active {
				levels[i] = 16
			} else {
				levels[i] = 0
			}
		}
		paintSpectrum(img, levels, 8)
		lineY := img.Bounds().Dy() * 5 / 6
		for x := 0; x < img.Bounds().Dx(); x++ {
			line := img.NRGBAAt(x, lineY)
			if line.R != 107 || line.G != 107 || line.B != 107 || line.A != 255 {
				t.Fatal("toggle moved or interrupted baseline")
			}
			for y := lineY + 1; y < img.Bounds().Dy(); y++ {
				pixel := img.NRGBAAt(x, y)
				if pixel.A > 0 && (pixel.R > 54 || pixel.G > 54 || pixel.B > 54) {
					t.Fatal("white bar extends below divider")
				}
			}
		}
		if active && img.NRGBAAt(0, lineY-1).R != 255 {
			t.Fatal("bar is not attached above divider")
		}
		if !active && img.NRGBAAt(0, lineY-1).A != 0 {
			t.Fatal("disabled spectrum retains bar")
		}
	}
}
func TestSpectrumImageCachesAndReplacesInPlace(t *testing.T) {
	var output string
	r := newSpectrumImage(func(s string) (int, error) { output += s; return len(s), nil })
	var levels [player.SpectrumBands]uint8
	if !r.show(120, 30, levels) {
		t.Fatal("initial chart missing")
	}
	output = ""
	if !r.show(120, 30, levels) || output != "" {
		t.Fatal("unchanged chart retransmitted")
	}
	levels[0] = 16
	if !r.show(120, 30, levels) || !strings.Contains(output, "a=t") || !strings.Contains(output, "a=p") {
		t.Fatal("changed chart not transmitted and placed")
	}
	output = ""
	r.Hide()
	if !strings.Contains(output, "a=d") {
		t.Fatal("chart not cleared on page change")
	}
	output = ""
	r.show(120, 30, levels)
	if !strings.Contains(output, "a=t") {
		t.Fatal("chart not restored")
	}
}
func TestSpectrumToggleKeepsLineControlsAndCoverFixed(t *testing.T) {
	for _, native := range []bool{false, true} {
		for _, width := range []int{120, 90, 60, 40} {
			app, n, r := newPlayingTestApp(t, native)
			_, _ = app.Update(tea.WindowSizeMsg{Width: width, Height: 52})
			bar := n.playbackBarRenderer
			state := &spectrumPlayingState{movingPlayingState: movingPlayingState{playingTestState: r.state.(playingTestState), playbackState: types.Playing}, frame: player.SpectrumFrame{Supported: true, UpdatedAt: time.Now()}}
			for i := range state.frame.Levels {
				state.frame.Levels[i] = 0.5
			}
			bar.state = state
			bar.SetSpectrumEnabled(false)
			off := strings.Split(stripAnsiCodes(app.Main().View(app)), "\n")
			beforeCover := r.placement
			hits := append([]playbackHit(nil), bar.hits...)
			imageY := 0
			if native {
				imageY = bar.spectrumImage.y
			}
			bar.SetSpectrumEnabled(true)
			on := strings.Split(stripAnsiCodes(app.Main().View(app)), "\n")
			if native {
				if imageY != bar.spectrumImage.y {
					t.Fatal("toggle moved native spectrum")
				}
				pixelHeight := bar.spectrumImage.image.Bounds().Dy()
				lineY := float64(imageY) + float64(pixelHeight*5/6)*3/float64(pixelHeight)
				baselineRow := 52 - 3
				if width < 76 {
					baselineRow--
				}
				if math.Abs(lineY-float64(baselineRow)-0.5) > 0.1 {
					t.Fatalf("line moved from disabled baseline: %v", lineY)
				}
			} else {
				beforeRow := -1
				for y, line := range off {
					if strings.Contains(line, strings.Repeat("─", width)) {
						beforeRow = y
					}
				}
				if beforeRow < 0 || !strings.ContainsRune(on[beforeRow], '─') {
					t.Fatal("toggle moved text divider")
				}
			}
			if r.placement != beforeCover {
				t.Fatal("toggle moved main panel")
			}
			for _, hit := range hits {
				if next := bar.hit(hit.x, hit.y); next == nil || next.action != hit.action {
					t.Fatal("toggle moved control")
				}
			}
		}
	}
}
func BenchmarkNativeSpectrumUpdate(b *testing.B) {
	r := newSpectrumImage(func(s string) (int, error) { return len(s), nil })
	var levels [player.SpectrumBands]uint8
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		for j := range levels {
			levels[j] = uint8((i + j) % 17)
		}
		r.show(120, 30, levels)
	}
}
