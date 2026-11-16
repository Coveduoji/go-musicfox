package ui

import (
	"context"
	"errors"
	"github.com/go-musicfox/go-musicfox/internal/lyric"
	"github.com/go-musicfox/go-musicfox/internal/structs"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/go-musicfox/go-musicfox/internal/player"
	"github.com/go-musicfox/go-musicfox/internal/types"
)

func TestNativePlaybackIconsCacheAndLifecycle(t *testing.T) {
	app, n, r := newPlayingTestApp(t, true)
	icons := n.playbackBarRenderer.icons
	var output string
	icons.write = func(s string) (int, error) { output += s; return len(s), nil }
	_, _ = app.Update(tea.WindowSizeMsg{Width: 120, Height: 48})
	_ = app.Main().View(app)
	if len(icons.placements) < 8 || !strings.Contains(output, "a=t") {
		t.Fatal("native control images missing")
	}
	output = ""
	_ = app.Main().View(app)
	if output != "" {
		t.Fatal("unchanged frame retransmitted controls")
	}
	state := &movingPlayingState{playingTestState: r.state.(playingTestState), playbackState: types.Paused}
	n.playbackBarRenderer.state = state
	_ = app.Main().View(app)
	if !strings.Contains(output, "a=t") || !strings.Contains(output, "a=p") {
		t.Fatal("state change did not update icon")
	}
	output = ""
	icons.Hide()
	if len(icons.placements) != 0 || !strings.Contains(output, "a=d") {
		t.Fatal("page change left controls displayed")
	}
	output = ""
	_ = app.Main().View(app)
	if !strings.Contains(output, "a=t") {
		t.Fatal("return did not restore controls")
	}
	icons.Invalidate()
	output = ""
	_ = app.Main().View(app)
	if !strings.Contains(output, "a=t") {
		t.Fatal("clear screen did not restore controls")
	}
}

func TestNativeIconFailureKeepsTextControls(t *testing.T) {
	app, n, _ := newPlayingTestApp(t, true)
	n.playbackBarRenderer.icons.write = func(string) (int, error) { return 0, errors.New("write failure") }
	_, _ = app.Update(tea.WindowSizeMsg{Width: 120, Height: 48})
	view := stripAnsiCodes(n.playbackBarRenderer.ViewBottom(app, app.Main()))
	if !strings.ContainsAny(view, "♡♥") || !strings.Contains(view, "⏮") {
		t.Fatal("failed image hid text fallback")
	}
}

func TestSpectrumSpansWholeDivider(t *testing.T) {
	_, n, r := newPlayingTestApp(t, false)
	state := &spectrumPlayingState{movingPlayingState: movingPlayingState{playingTestState: r.state.(playingTestState), playbackState: types.Playing}, frame: player.SpectrumFrame{Supported: true, UpdatedAt: time.Now()}}
	for i := range state.frame.Levels {
		state.frame.Levels[i] = 1
	}
	n.playbackBarRenderer.state = state
	for _, width := range []int{32, 60, 120, 159, 240} {
		rows := n.playbackBarRenderer.spectrumRows(width)
		top := []rune(stripAnsiCodes(rows[0]))
		if len(top) != width || top[0] != '█' || top[(width-1)/2*2] != '█' {
			t.Fatalf("%d columns: spectrum leaves empty side margins", width)
		}
	}
}

func TestCoverMatchesTitleAndLyricsVerticalBounds(t *testing.T) {
	app, n, r := newPlayingTestApp(t, true)
	service := lyric.NewService(playingLyricFetcher{structs.LRCData{Original: "[00:00.00]第一句\n[00:01.00]第二句\n[00:02.00]第三句\n[00:03.00]第四句\n[00:04.00]第五句"}}, false, 0, false)
	if err := service.SetSong(context.Background(), r.state.CurSong()); err != nil {
		t.Fatal(err)
	}
	service.UpdatePosition(2 * time.Second)
	n.lyricRenderer = NewLyricRenderer(n, service, true)
	_, _ = app.Update(tea.WindowSizeMsg{Width: 120, Height: 52})
	rows := strings.Split(stripAnsiCodes(app.Main().View(app)), "\n")
	title, last := -1, -1
	for y, line := range rows {
		if strings.Contains(line, "山丘 (Live)") {
			title = y
		}
		if strings.Contains(line, "第五句") {
			last = y
		}
	}
	if title != r.placement.y || last != r.placement.y+r.placement.rows-1 {
		t.Fatalf("title/lyrics bounds %d..%d differ from cover %+v", title, last, r.placement)
	}
}

func TestSpectrumSpacedBarsContinuousDividerAndReflection(t *testing.T) {
	_, n, r := newPlayingTestApp(t, false)
	state := &spectrumPlayingState{movingPlayingState: movingPlayingState{playingTestState: r.state.(playingTestState), playbackState: types.Playing}, frame: player.SpectrumFrame{Supported: true, UpdatedAt: time.Now()}}
	for i := range state.frame.Levels {
		state.frame.Levels[i] = 1
	}
	n.playbackBarRenderer.state = state
	rows := n.playbackBarRenderer.spectrumRows(120)
	for y, row := range rows {
		chars := []rune(stripAnsiCodes(row))
		for x, ch := range chars {
			if x%2 == 1 {
				expected := ' '
				if y == 2 {
					expected = '─'
				}
				if ch != expected {
					t.Fatal("bar gap must retain only the continuous divider")
				}
				continue
			}
			if y < 2 && ch != '█' {
				t.Fatal("full-height audio bar missing")
			}
			if y == 2 && ch != '▀' {
				t.Fatal("reflection is not short and top-aligned")
			}
		}
	}
	state.playbackState = types.Paused
	rows = n.playbackBarRenderer.spectrumRows(120)
	if stripAnsiCodes(rows[2]) != strings.Repeat("─", 120) {
		t.Fatal("pause must retain only the fixed divider")
	}
}

func TestPlaybackPanelTracksMenuLeftEdge(t *testing.T) {
	for _, native := range []bool{false, true} {
		app, _, r := newPlayingTestApp(t, native)
		for _, width := range []int{180, 120, 90, 76, 60, 40} {
			_, _ = app.Update(tea.WindowSizeMsg{Width: width, Height: 52})
			view := stripAnsiCodes(app.Main().View(app))
			left := app.Main().MenuStartColumn()
			if r.placed && r.placement.x != left {
				t.Fatalf("width=%d cover x=%d menu=%d", width, r.placement.x, left)
			}
			if !native || width < 76 {
				for _, row := range strings.Split(view, "\n") {
					if at := strings.Index(row, "山丘 (Live)"); at >= 0 && lipgloss.Width(row[:at]) != left {
						t.Fatalf("width=%d title does not align to menu", width)
					}
				}
			}
		}
	}
}

func TestSpectrumTextDividerUsesFixedRow(t *testing.T) {
	_, n, _ := newPlayingTestApp(t, false)
	rows := n.playbackBarRenderer.spectrumRows(120)
	if stripAnsiCodes(rows[2]) != strings.Repeat("─", 120) {
		t.Fatal("quiet divider does not occupy fixed baseline row")
	}
}
