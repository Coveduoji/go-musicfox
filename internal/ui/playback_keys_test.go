package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/go-musicfox/go-musicfox/internal/keybindings"
	"github.com/go-musicfox/go-musicfox/internal/player"
	"github.com/go-musicfox/go-musicfox/internal/playlist"
	"github.com/go-musicfox/go-musicfox/internal/structs"
	"github.com/go-musicfox/go-musicfox/internal/types"
)

type playbackKeyEngine struct {
	player.Player
	state            types.State
	toggles, resumes int
}

func (e *playbackKeyEngine) State() types.State { return e.state }
func (e *playbackKeyEngine) Toggle() {
	e.toggles++
	if e.state == types.Playing {
		e.state = types.Paused
	} else {
		e.state = types.Playing
	}
}
func (e *playbackKeyEngine) Resume() { e.resumes++; e.state = types.Playing }

func TestSpaceOnlyTogglesCurrentSong(t *testing.T) {
	app, n, _ := newPlayingTestApp(t, false)
	songs := []structs.Song{{Id: 1, Name: "playing"}, {Id: 2, Name: "selected"}}
	pm := playlist.NewPlaylistManager()
	_ = pm.Initialize(0, songs)
	engine := &playbackKeyEngine{state: types.Playing}
	n.player = &Player{Player: engine, netease: n, playlistManager: pm}
	app.Main().EnterMenu(NewCurPlaylist(newBaseMenu(n), songs), nil)
	app.Main().SetSelectedIndex(1)
	handler := &EventHandler{netease: n, keyToOperateMap: map[string]keybindings.OperateType{" ": keybindings.OpToggle}}
	for _, state := range []types.State{types.Playing, types.Paused, types.Stopped} {
		engine.state = state
		before := engine.toggles
		handled, _, _ := handler.KeyMsgHandle(tea.KeyMsg{Type: tea.KeySpace}, app)
		if !handled || pm.GetCurrentIndex() != 0 {
			t.Fatal("space selected another song")
		}
		expected := before
		if state != types.Stopped {
			expected++
		}
		if engine.toggles != expected {
			t.Fatal("space did not preserve stopped/toggle semantics")
		}
	}
}
func TestEnterResumesSelectedSongWithoutPausing(t *testing.T) {
	app, n, _ := newPlayingTestApp(t, false)
	songs := []structs.Song{{Id: 1, Name: "current"}}
	pm := playlist.NewPlaylistManager()
	_ = pm.Initialize(0, songs)
	engine := &playbackKeyEngine{state: types.Playing}
	n.player = &Player{Player: engine, netease: n, playlistManager: pm}
	app.Main().EnterMenu(NewCurPlaylist(newBaseMenu(n), songs), nil)
	handler := &EventHandler{netease: n, keyToOperateMap: map[string]keybindings.OperateType{"enter": keybindings.OpEnter}}
	for _, state := range []types.State{types.Playing, types.Paused} {
		engine.state = state
		handled, _, _ := handler.KeyMsgHandle(tea.KeyMsg{Type: tea.KeyEnter}, app)
		if !handled || engine.state != types.Playing || engine.toggles != 0 {
			t.Fatal("Enter paused the selected song")
		}
	}
	if engine.resumes != 1 {
		t.Fatal("Enter failed to resume the paused selected song")
	}
}
func TestEnterKeepsOrdinaryMenuNavigation(t *testing.T) {
	app, n, _ := newPlayingTestApp(t, false)
	handler := &EventHandler{netease: n, keyToOperateMap: map[string]keybindings.OperateType{"enter": keybindings.OpEnter}}
	handled, _, _ := handler.KeyMsgHandle(tea.KeyMsg{Type: tea.KeyEnter}, app)
	if handled {
		t.Fatal("Enter intercepted ordinary menu navigation")
	}
}
