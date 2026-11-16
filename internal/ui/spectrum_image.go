package ui

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"time"

	"github.com/go-musicfox/go-musicfox/internal/player"
	"github.com/go-musicfox/go-musicfox/internal/ui/kitty"
)

// The UI renders serially, so one encoder buffer can be reused without locking.
type spectrumPNGPool struct{ buffer *png.EncoderBuffer }

func (p *spectrumPNGPool) Get() *png.EncoderBuffer       { return p.buffer }
func (p *spectrumPNGPool) Put(buffer *png.EncoderBuffer) { p.buffer = buffer }

// spectrumImage shares the cover/icon transport. All chart geometry is in one
// image so terminal font baselines cannot move the line through the bars.
type spectrumImage struct {
	write            func(string) (int, error)
	encoder          png.Encoder
	id               uint32
	image            *image.NRGBA
	levels           [player.SpectrumBands]uint8
	encoded          string
	columns, y       int
	uploaded, placed bool
	retryAt          time.Time
}

func newSpectrumImage(write func(string) (int, error)) *spectrumImage {
	return &spectrumImage{write: write, id: kitty.NewImageID(), encoder: png.Encoder{CompressionLevel: png.BestSpeed, BufferPool: &spectrumPNGPool{}}}
}
func (s *spectrumImage) Hide() {
	if s.placed {
		_, _ = s.write(kitty.DeleteImage(s.id))
	}
	s.placed = false
	s.uploaded = false
}
func (s *spectrumImage) Invalidate() { s.placed = false; s.uploaded = false }
func (s *spectrumImage) show(columns, y int, levels [player.SpectrumBands]uint8) bool {
	if columns < 1 || y < 0 || time.Now().Before(s.retryAt) {
		return false
	}
	cw, ch := kitty.CellSize()
	if cw <= 0 || ch <= 0 {
		cw = 8
		ch = 16
	}
	width, height := max(1, int(math.Round(float64(columns)*cw))), max(3, int(math.Round(3*ch)))
	changed := s.image == nil || s.image.Bounds().Dx() != width || s.image.Bounds().Dy() != height || s.levels != levels
	if !changed && s.placed && s.columns == columns && s.y == y {
		return true
	}
	if changed {
		if s.image == nil || s.image.Bounds().Dx() != width || s.image.Bounds().Dy() != height {
			s.image = image.NewNRGBA(image.Rect(0, 0, width, height))
		}
		paintSpectrum(s.image, levels, max(4, int(cw)))
		encoded, err := kitty.TransmitImageWithEncoder(s.image, s.id, &s.encoder)
		if err != nil {
			return false
		}
		s.encoded = encoded
		s.levels = levels
		s.uploaded = false
	}
	seq := ""
	if !s.uploaded {
		seq = s.encoded
	}
	if !s.uploaded || !s.placed || s.columns != columns || s.y != y {
		seq += fmt.Sprintf("\x1b[s\x1b[%d;1H", y+1) + kitty.PlaceImage(s.id, columns, 3) + "\x1b[u"
	}
	n, err := s.write(seq)
	if err != nil || n != len(seq) {
		s.Hide()
		s.retryAt = time.Now().Add(time.Second)
		return false
	}
	s.columns = columns
	s.y = y
	s.uploaded = true
	s.placed = true
	return true
}

func paintSpectrum(img *image.NRGBA, levels [player.SpectrumBands]uint8, step int) {
	clear(img.Pix)
	width, height := img.Bounds().Dx(), img.Bounds().Dy()
	baseline := height * 5 / 6
	white := image.NewUniform(color.NRGBA{255, 255, 255, 255})
	// One physical pixel, continuous across the full width.
	draw.Draw(img, image.Rect(0, baseline, width, baseline+1), image.NewUniform(color.NRGBA{107, 107, 107, 255}), image.Point{}, draw.Src)
	count := (width + step - 1) / step
	for i := 0; i < count; i++ {
		pos := float64(i) * float64(player.SpectrumBands-1) / float64(max(1, count-1))
		lo := int(pos)
		hi := min(player.SpectrumBands-1, lo+1)
		value := float64(levels[lo]) + (float64(levels[hi])-float64(levels[lo]))*(pos-float64(lo))
		barHeight := int(math.Round(value * float64(max(0, baseline-1)) / 16))
		if barHeight <= 0 {
			continue
		}
		left, right := i*step, min(width, i*step+max(1, step/3))
		draw.Draw(img, image.Rect(left, baseline-barHeight, right, baseline), white, image.Point{}, draw.Src)
		reflection := min(height-baseline-1, max(1, barHeight/4))
		for row := 0; row < reflection; row++ {
			alpha := uint8(150 * (1 - float64(row)/float64(reflection)))
			shade := color.NRGBA{54, 54, 54, alpha}
			for x := left; x < right; x++ {
				img.SetNRGBA(x, baseline+1+row, shade)
			}
		}
	}
}
