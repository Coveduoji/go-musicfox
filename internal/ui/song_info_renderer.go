package ui

import (
	"fmt"
	"strings"

	"github.com/anhoder/foxful-cli/model"
	"github.com/anhoder/foxful-cli/util"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/go-musicfox/go-musicfox/internal/types"
	"github.com/go-musicfox/go-musicfox/utils/likelist"
)

type playbarBound struct {
	start, end int
	action     string
}

// SongInfoRenderer preserves the existing playback actions in a fixed bottom row.
type SongInfoRenderer struct {
	netease *Netease
	state   playerRendererState
}

func NewSongInfoRenderer(n *Netease, state playerRendererState) *SongInfoRenderer {
	return &SongInfoRenderer{n, state}
}
func (r *SongInfoRenderer) Update(tea.Msg, *model.App) {}
func (r *SongInfoRenderer) ViewBottom(a *model.App, m *model.Main) string {
	v, _ := r.View(a, m)
	return strings.TrimRight(v, "\n") + "\n"
}

func (r *SongInfoRenderer) View(a *model.App, m *model.Main) (string, int) {
	type segment struct {
		text   string
		color  termenv.Color
		action string
	}
	var parts []segment
	add := func(text string, c termenv.Color, action string) { parts = append(parts, segment{text, c, action}) }
	width := max(0, a.WindowWidth())
	song := r.state.CurSong()
	if width >= 40 {
		if !m.CenterEverything() {
			add(strings.Repeat(" ", max(0, min(m.MenuStartColumn()-4, (width-60)/2))), termenv.ANSIBrightBlack, "")
		}
		add("["+r.state.Mode().Name()+"] ", termenv.ANSIBrightMagenta, "mode")
		add(fmt.Sprintf("%d%% ", r.state.Volume()), termenv.ANSIBrightBlue, "")
	}
	if r.state.State() == types.Playing {
		add("[Ⅱ] ", termenv.ANSIBrightYellow, "state")
	} else {
		add("[▶] ", termenv.ANSIYellow, "state")
	}
	if song.Id > 0 {
		if likelist.IsLikeSong(song.Id) {
			add("♥ ", termenv.ANSIRed, "heart")
		} else {
			add("♡ ", termenv.ANSIWhite, "heart")
		}
	}
	add(song.Name, util.GetPrimaryColor(), "song")
	add(" ", termenv.ANSIBrightBlack, "")
	var artists []string
	for _, artist := range song.Artists {
		artists = append(artists, artist.Name)
	}
	add(strings.Join(artists, "/"), termenv.ANSIBrightBlack, "artist")
	total := 0
	for i := range parts {
		parts[i].text = truncatePlayingText(strings.ReplaceAll(strings.ReplaceAll(parts[i].text, "\n", " "), "\r", " "), max(0, width-total), "")
		total += lipgloss.Width(parts[i].text)
	}
	left := 0
	if m.CenterEverything() {
		left = max(0, (width-total)/2)
	}
	var b strings.Builder
	b.WriteString(strings.Repeat(" ", left))
	x := left
	r.netease.playbarBounds = nil
	for _, part := range parts {
		w := lipgloss.Width(part.text)
		if w > 0 && part.action != "" {
			r.netease.playbarBounds = append(r.netease.playbarBounds, playbarBound{x, x + w, part.action})
		}
		b.WriteString(util.SetFgStyle(part.text, part.color))
		x += w
	}
	b.WriteString(strings.Repeat(" ", max(0, width-x)))
	return b.String() + "\n\n", 3
}
