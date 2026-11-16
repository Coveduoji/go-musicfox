package ui

import (
	"bytes"
	"embed"
	"fmt"
	"image/png"
	"strings"
	"time"

	"github.com/go-musicfox/go-musicfox/internal/types"
	"github.com/go-musicfox/go-musicfox/internal/ui/kitty"
	"github.com/go-musicfox/go-musicfox/utils/likelist"
)

// Assets are rasterized once during development; playback adds no SVG dependency.
//
//go:embed assets/playback/*.png
var playbackIconAssets embed.FS

type playbackIconAsset struct {
	id       uint32
	encoded  string
	uploaded bool
}
type playbackIconPlacement struct {
	key         string
	x, y, cells int
	seen        bool
}
type playbackIcons struct {
	write      func(string) (int, error)
	assets     map[string]*playbackIconAsset
	placements map[string]playbackIconPlacement
	retryAt    time.Time
}

func newPlaybackIcons(write func(string) (int, error)) *playbackIcons {
	return &playbackIcons{write: write, assets: make(map[string]*playbackIconAsset), placements: make(map[string]playbackIconPlacement)}
}
func (i *playbackIcons) begin() {
	for action, p := range i.placements {
		p.seen = false
		i.placements[action] = p
	}
}
func (i *playbackIcons) end() {
	for action, p := range i.placements {
		if !p.seen {
			i.remove(action)
		}
	}
}
func (i *playbackIcons) remove(action string) {
	p, ok := i.placements[action]
	if !ok {
		return
	}
	_, _ = i.write(kitty.DeleteImage(i.assets[p.key].id))
	delete(i.placements, action)
}
func (i *playbackIcons) Hide() {
	for action := range i.placements {
		i.remove(action)
	}
	for _, a := range i.assets {
		a.uploaded = false
	}
}
func (i *playbackIcons) Invalidate() {
	clear(i.placements)
	for _, a := range i.assets {
		a.uploaded = false
	}
}
func (i *playbackIcons) draw(action, name string, x, y, cells int) bool {
	if name == "" || cells < 2 || y < 0 || time.Now().Before(i.retryAt) {
		return false
	}
	key := action + ":" + name
	if p, ok := i.placements[action]; ok && p.key == key && p.x == x && p.y == y && p.cells == cells {
		p.seen = true
		i.placements[action] = p
		return true
	}
	a := i.assets[key]
	if a == nil {
		data, err := playbackIconAssets.ReadFile("assets/playback/" + name + ".png")
		if err != nil {
			return false
		}
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			return false
		}
		a = &playbackIconAsset{id: kitty.NewImageID()}
		a.encoded, err = kitty.TransmitImage(img, 0, 0, a.id)
		if err != nil {
			return false
		}
		i.assets[key] = a
	}
	seq := ""
	if !a.uploaded {
		seq = a.encoded
	}
	if p, ok := i.placements[action]; ok {
		seq += kitty.DeleteImage(i.assets[p.key].id)
	}
	// A one-row square icon remains round for different terminal font metrics.
	seq += fmt.Sprintf("\x1b[s\x1b[%d;%dH", y+1, x+1+max(0, (cells-2)/2)) + kitty.PlaceImage(a.id, 0, 1) + "\x1b[u"
	n, err := i.write(seq)
	if err != nil || n != len(seq) {
		i.retryAt = time.Now().Add(time.Second)
		i.Hide()
		return false
	}
	a.uploaded = true
	i.placements[action] = playbackIconPlacement{key, x, y, cells, true}
	return true
}
func (r *PlaybackBarRenderer) iconName(action string) string {
	switch action {
	case "state":
		if r.state.State() == types.Playing {
			return "pause"
		}
		return "play"
	case "heart":
		if likelist.IsLikeSong(r.state.CurSong().Id) {
			return "heart-liked"
		}
		return "heart"
	case "previous":
		return "skip-back"
	case "next":
		return "skip-forward"
	case "queue":
		return "list-music"
	case "spectrum":
		if r.spectrumEnabled {
			return "music-2-active"
		}
		return "music-2"
	case "volumeDown":
		return "minus"
	case "volumeUp":
		return "plus"
	case "mode":
		switch r.state.Mode() {
		case types.PmOrdered:
			return "arrow-right"
		case types.PmSingleLoop:
			return "repeat-1-active"
		case types.PmListRandom, types.PmInfRandom:
			return "shuffle-active"
		case types.PmIntelligent:
			return "heart-liked"
		default:
			return "repeat-active"
		}
	}
	return ""
}
func playbackIconFallback(symbol string, cells int) string {
	if cells < 3 {
		return symbol
	}
	return " " + symbol + strings.Repeat(" ", max(0, cells-1-len([]rune(symbol))))
}
