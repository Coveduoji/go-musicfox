package ui

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anhoder/foxful-cli/model"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/go-musicfox/go-musicfox/internal/configs"
	"github.com/go-musicfox/go-musicfox/internal/lyric"
	"github.com/go-musicfox/go-musicfox/internal/player"
	"github.com/go-musicfox/go-musicfox/internal/structs"
	"github.com/go-musicfox/go-musicfox/internal/types"
	"github.com/go-musicfox/go-musicfox/internal/ui/kitty"
)

type playingTestMenu struct{ model.DefaultMenu }

func (playingTestMenu) GetMenuKey() string { return "playing-test" }
func (playingTestMenu) MenuViews() []model.MenuItem {
	items := make([]model.MenuItem, 10)
	for i := range items {
		items[i] = model.MenuItem{Title: "测试菜单"}
	}
	return items
}

type playingTestState struct{ song structs.Song }

func (s playingTestState) CurSong() structs.Song    { return s.song }
func (playingTestState) CurSongIndex() int          { return 0 }
func (playingTestState) PassedTime() time.Duration  { return 2 * time.Second }
func (playingTestState) State() types.State         { return types.Playing }
func (playingTestState) Volume() int                { return 40 }
func (playingTestState) Mode() types.Mode           { return types.PmListLoop }
func (s playingTestState) Playlist() []structs.Song { return []structs.Song{s.song} }

func newPlayingTestApp(t *testing.T, native bool) (*model.App, *Netease, *NowPlayingRenderer) {
	t.Helper()
	previous := configs.AppConfig
	configs.AppConfig = &configs.Config{}
	configs.AppConfig.Main.Lyric.Layout = "adaptive"
	configs.AppConfig.Main.Visualizer.Enable = true
	t.Cleanup(func() { configs.AppConfig = previous })
	n := &Netease{}
	state := playingTestState{structs.Song{Id: 1, Name: "山丘 (Live)", Duration: time.Minute, Artists: []structs.Artist{{Name: "李宗盛"}}, Album: structs.Album{PicUrl: "https://example.invalid/cover.jpg"}}}
	n.lyricRenderer = NewLyricRenderer(n, lyric.NewService(nil, false, 0, false), true)
	r := &NowPlayingRenderer{netease: n, state: state, nativeSupported: native, images: kitty.NewImageCache(2), img: image.NewNRGBA(image.Rect(0, 0, 8, 8)), url: getCoverUrl(state.song), imageID: kitty.NewImageID(), write: func(s string) (int, error) { return len(s), nil }}
	n.nowPlayingRenderer = r
	n.playbackBarRenderer = NewPlaybackBarRenderer(n, state)
	opts := model.DefaultOptions()
	opts.EnableStartup = false
	opts.PrimaryColor = "#EA403F"
	opts.AppName = "musicfox"
	opts.DualColumn = true
	opts.MainMenu = &playingTestMenu{}
	opts.MainMenuTitle = &model.MenuItem{Title: "网易云音乐"}
	opts.Components = []model.Component{r, n.playbackBarRenderer}
	opts.KBControllers = []model.KeyboardController{&playingPageController{}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	opts.TeaOptions = []tea.ProgramOption{tea.WithContext(ctx), tea.WithInput(nil), tea.WithOutput(io.Discard)}
	app := model.NewApp(opts)
	n.App = app
	_ = app.Run()
	t.Cleanup(r.Close)
	return app, n, r
}

func TestPlayingPanelAndControlsFitWindow(t *testing.T) {
	for _, native := range []bool{false, true} {
		app, n, r := newPlayingTestApp(t, native)
		for _, size := range [][2]int{{140, 52}, {60, 44}, {40, 32}, {80, 16}, {24, 10}, {10, 5}, {5, 2}, {1, 1}, {140, 52}} {
			w, h := size[0], size[1]
			_, _ = app.Update(tea.WindowSizeMsg{Width: w, Height: h})
			view := app.Main().View(app)
			rows := strings.Split(view, "\n")
			if len(rows) != h {
				t.Fatalf("%dx%d frame height=%d", w, h, len(rows))
			}
			for y, row := range rows {
				if lipgloss.Width(row) > w {
					t.Fatalf("%dx%d row %d width=%d", w, h, y, lipgloss.Width(row))
				}
			}
			footer := strings.Join(rows[h-playbackFooterHeight(w, h):], "\n")
			if strings.Contains(footer, "山丘") || strings.Contains(footer, "李宗盛") {
				t.Fatal("footer contains song metadata")
			}
			if !native && !strings.ContainsAny(footer, "♥♡") {
				t.Fatalf("%dx%d: like button missing", w, h)
			}
			if h >= 3 && w >= 24 && !strings.Contains(footer, "00:02") {
				t.Fatalf("%dx%d: progress missing", w, h)
			}
			if h >= 32 && !strings.Contains(view, "山丘 (Live)") {
				t.Fatal("song title missing above lyrics")
			}
			if native && h >= 40 {
				if !r.placed {
					t.Fatal("native image was not placed")
				}
				if r.placement.cols > 24 || r.placement.rows > 12 {
					t.Fatal("cover exceeds compact cap")
				}
				lyricRow := -1
				for y, row := range rows {
					if strings.Contains(row, "暂无歌词") {
						lyricRow = y
					}
				}
				if w >= 76 && (lyricRow < r.placement.y || lyricRow >= r.placement.y+r.placement.rows) {
					t.Fatal("wide cover and lyrics are not side by side")
				}
				if w < 76 && lyricRow < r.placement.y+r.placement.rows {
					t.Fatal("narrow lyrics are not below cover")
				}
			}
			for _, hit := range n.playbackBarRenderer.hits {
				if hit.x < 0 || hit.x+hit.width > w || hit.y < 0 || hit.y >= h {
					t.Fatalf("invalid hit target %+v at %dx%d", hit, w, h)
				}
				if got := n.playbackBarRenderer.hit(hit.x, hit.y); got == nil || got.action != hit.action {
					t.Fatalf("hit target does not match %+v", hit)
				}
			}
		}
	}
}

func TestNativeCoverTransfersOnlyWhenNeeded(t *testing.T) {
	app, _, r := newPlayingTestApp(t, true)
	var writes []string
	r.write = func(s string) (int, error) { writes = append(writes, s); return len(s), nil }
	_, _ = app.Update(tea.WindowSizeMsg{Width: 120, Height: 48})
	_ = app.Main().View(app)
	if len(writes) != 1 || !strings.Contains(writes[0], "a=t") {
		t.Fatal("first frame did not transmit original image")
	}
	_ = app.Main().View(app)
	if len(writes) != 1 {
		t.Fatal("unchanged frame retransmitted image")
	}
	_, _ = app.Update(tea.WindowSizeMsg{Width: 60, Height: 48})
	_ = app.Main().View(app)
	if strings.Contains(writes[len(writes)-1], "a=t") {
		t.Fatal("resize uploaded image instead of moving placement")
	}
	r.Hide()
	if r.placed || !strings.Contains(writes[len(writes)-1], "a=d") {
		t.Fatal("leaving page did not remove cover")
	}
	_ = app.Main().View(app)
	if !r.placed {
		t.Fatal("returning to main did not restore cover")
	}
	r.Invalidate()
	_ = app.Main().View(app)
	if !strings.Contains(writes[len(writes)-1], "a=t") {
		t.Fatal("full repaint did not restore image data")
	}
}

func TestUnsupportedTerminalDoesNotLoadOrSimulateCover(t *testing.T) {
	app, _, r := newPlayingTestApp(t, false)
	r.img = nil
	r.images = nil
	writes := 0
	r.write = func(s string) (int, error) { writes++; return len(s), nil }
	_, _ = app.Update(tea.WindowSizeMsg{Width: 120, Height: 44})
	view := app.Main().View(app)
	if r.result != nil || writes != 0 {
		t.Fatal("unsupported terminal attempted image transfer")
	}
	if strings.ContainsAny(view, "▀▄░") {
		t.Fatal("unsupported terminal simulated a pixel cover")
	}
	if !strings.Contains(view, "山丘 (Live)") {
		t.Fatal("fallback lost song title")
	}
}

func TestOriginalCoverDownloadsOnceAndIgnoresOldSong(t *testing.T) {
	_, _, r := newPlayingTestApp(t, true)
	r.img = nil
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		if err := png.Encode(w, image.NewNRGBA(image.Rect(0, 0, 4, 4))); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	r.loadCover(server.URL)
	var loaded coverLoadResult
	select {
	case loaded = <-r.result:
	case <-time.After(3 * time.Second):
		t.Fatal("cover load did not finish")
	}
	ready := make(chan coverLoadResult, 1)
	ready <- loaded
	r.result = ready
	r.loadCover(server.URL)
	r.loadCover(server.URL)
	if requests.Load() != 1 || r.img == nil || r.img.Bounds().Dx() != 320 {
		t.Fatal("cover was not cached as compact original")
	}
	old := make(chan coverLoadResult, 1)
	r.result = old
	r.loadCover("")
	old <- coverLoadResult{image.NewUniform(color.White)}
	r.loadCover("")
	if r.img != nil {
		t.Fatal("cancelled song replaced current image")
	}
}

func TestPlaybackActionsAndSpectrumToggle(t *testing.T) {
	app, n, _ := newPlayingTestApp(t, false)
	for _, width := range []int{120, 60, 36} {
		_, _ = app.Update(tea.WindowSizeMsg{Width: width, Height: 44})
		_ = app.Main().View(app)
		actions := map[string]bool{}
		for _, hit := range n.playbackBarRenderer.hits {
			actions[hit.action] = true
		}
		for _, action := range []string{"heart", "previous", "state", "next", "mode", "seek", "queue", "spectrum", "volumeUp", "volumeDown"} {
			if !actions[action] {
				t.Fatalf("%d columns missing %s", width, action)
			}
		}
	}
	handler := &EventHandler{netease: n}
	var hit *playbackHit
	for i := range n.playbackBarRenderer.hits {
		if n.playbackBarRenderer.hits[i].action == "spectrum" {
			hit = &n.playbackBarRenderer.hits[i]
			break
		}
	}
	_, _, _ = handler.handleAdaptivePlayback(tea.MouseMsg{Action: tea.MouseActionPress}, hit, app, app.Main())
	if n.playbackBarRenderer.spectrumEnabled || !n.lyricRenderer.isVisible {
		t.Fatal("spectrum button must disable spectrum without changing lyrics")
	}
	_, _, _ = handler.handleAdaptivePlayback(tea.MouseMsg{Action: tea.MouseActionPress}, hit, app, app.Main())
	if !n.playbackBarRenderer.spectrumEnabled {
		t.Fatal("second click must re-enable spectrum")
	}

}

type playingPageController struct{}

func (*playingPageController) KeyMsgHandle(msg tea.KeyMsg, a *model.App) (bool, model.Page, tea.Cmd) {
	if msg.String() == "x" {
		return true, &playingOtherPage{}, nil
	}
	return false, nil, nil
}

type playingOtherPage struct{}

func (*playingOtherPage) IgnoreQuitKeyMsg(tea.KeyMsg) bool                 { return false }
func (*playingOtherPage) Type() model.PageType                             { return "playing-test-other" }
func (*playingOtherPage) View(*model.App) string                           { return "other page" }
func (*playingOtherPage) Msg() tea.Msg                                     { return nil }
func (*playingOtherPage) Update(tea.Msg, *model.App) (model.Page, tea.Cmd) { return nil, nil }

func TestLeavingMainClearsOriginalCover(t *testing.T) {
	app, _, r := newPlayingTestApp(t, true)
	_, _ = app.Update(tea.WindowSizeMsg{Width: 120, Height: 44})
	_ = app.View()
	if !r.placed {
		t.Fatal("test cover was not placed")
	}
	_, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if app.CurPage() == app.Main() || r.placed {
		t.Fatal("standalone page retained the native cover")
	}
}

type playingLyricFetcher struct{ data structs.LRCData }

func (f playingLyricFetcher) GetLyric(context.Context, int64) (structs.LRCData, error) {
	return f.data, nil
}

func TestMainPanelPreservesLRCAndYRC(t *testing.T) {
	for name, data := range map[string]structs.LRCData{
		"lrc": {Original: "[00:00.00]上一句\n[00:01.00]正在唱这句\n[00:04.00]下一句", Translated: "[00:01.00]translation"},
		"yrc": {Yrc: "[0,5000](0,2500,0)逐字(2500,2500,0)歌词"},
	} {
		t.Run(name, func(t *testing.T) {
			app, n, r := newPlayingTestApp(t, false)
			service := lyric.NewService(playingLyricFetcher{data}, true, 0, false)
			service.EnableYRC(true)
			if err := service.SetSong(context.Background(), r.state.CurSong()); err != nil {
				t.Fatal(err)
			}
			service.UpdatePosition(2 * time.Second)
			n.lyricRenderer = NewLyricRenderer(n, service, true)
			_, _ = app.Update(tea.WindowSizeMsg{Width: 120, Height: 44})
			view := stripAnsiCodes(app.Main().View(app))
			if name == "lrc" && (!strings.Contains(view, "正在唱这句") || !strings.Contains(view, "translation")) {
				t.Fatal("LRC or translation missing")
			}
			if name == "yrc" && !strings.Contains(view, "逐字歌词") {
				t.Fatal("YRC content missing")
			}
		})
	}
}

type movingPlayingState struct {
	playingTestState
	elapsed       time.Duration
	playbackState types.State
}

func (s *movingPlayingState) PassedTime() time.Duration { return s.elapsed }
func (s *movingPlayingState) State() types.State        { return s.playbackState }

func TestPlayingPanelVisualCenterAndSpacing(t *testing.T) {
	app, _, r := newPlayingTestApp(t, true)
	_, _ = app.Update(tea.WindowSizeMsg{Width: 120, Height: 52})
	view := stripAnsiCodes(app.Main().View(app))
	titleX := -1
	artistX := -1
	for _, row := range strings.Split(view, "\n") {
		if at := strings.Index(row, "山丘 (Live)"); at >= 0 {
			titleX = lipgloss.Width(row[:at])
		}
		if at := strings.Index(row, "李宗盛"); at >= 0 {
			artistX = lipgloss.Width(row[:at])
		}
	}
	if titleX < r.placement.x+r.placement.cols+4 {
		t.Fatal("cover/lyric gap is still too tight")
	}
	if titleX != artistX {
		t.Fatal("title and artist are not aligned")
	}
	if r.placement.x != app.Main().MenuStartColumn() {
		t.Fatal("playback panel does not align with menu")
	}
	// With room to spare, lifting the group preserves the allocated viewport.
	if playingPanelTop(24, 12) >= 6 {
		t.Fatal("panel is still vertically centered too low")
	}
	if playingPanelTop(12, 12) != 0 {
		t.Fatal("compact panel should not use a negative offset")
	}
}

func TestPlaybackGroupsKeepTransportCentered(t *testing.T) {
	app, n, _ := newPlayingTestApp(t, false)
	for _, width := range []int{140, 120, 108, 96, 90, 76, 60, 48, 36, 24} {
		_, _ = app.Update(tea.WindowSizeMsg{Width: width, Height: 44})
		_ = app.Main().View(app)
		hits := map[string]playbackHit{}
		for _, hit := range n.playbackBarRenderer.hits {
			hits[hit.action] = hit
		}
		if hits["heart"].x >= hits["mode"].x || hits["mode"].x >= hits["previous"].x {
			t.Fatalf("%d: left controls mixed into transport", width)
		}
		if hits["previous"].x >= hits["state"].x || hits["state"].x >= hits["next"].x {
			t.Fatalf("%d: transport order incorrect", width)
		}
		if width >= 76 {
			delta := 2*hits["state"].x + hits["state"].width - width
			if delta < -2 || delta > 2 {
				t.Fatalf("%d: primary button lost center", width)
			}
			if hits["next"].x+hits["next"].width >= hits["queue"].x {
				t.Fatalf("%d: right controls overlap transport", width)
			}
		} else if hits["queue"].y >= hits["state"].y {
			t.Fatalf("%d: auxiliary controls did not reflow", width)
		}
	}
}

type spectrumPlayingState struct {
	movingPlayingState
	frame player.SpectrumFrame
}

func (s *spectrumPlayingState) Spectrum() player.SpectrumFrame { return s.frame }

func TestSpectrumUsesAudioAndDoesNotRetransmitCover(t *testing.T) {
	app, n, r := newPlayingTestApp(t, true)
	state := &spectrumPlayingState{movingPlayingState: movingPlayingState{playingTestState: r.state.(playingTestState), playbackState: types.Playing}, frame: player.SpectrumFrame{Supported: true, UpdatedAt: time.Now()}}
	n.playbackBarRenderer.state = state
	r.state = state
	writes := 0
	r.write = func(s string) (int, error) { writes++; return len(s), nil }
	_, _ = app.Update(tea.WindowSizeMsg{Width: 120, Height: 48})
	_ = app.Main().View(app)
	baseline := n.playbackBarRenderer.spectrumRows(120)
	state.elapsed = time.Second
	if n.playbackBarRenderer.spectrumRows(120) != baseline {
		t.Fatal("clock alone animated spectrum")
	}
	state.frame.Levels[8] = 1
	raised := n.playbackBarRenderer.spectrumRows(120)
	if raised == baseline || strings.TrimSpace(stripAnsiCodes(raised[0])) == "" {
		t.Fatal("audio energy did not raise bars")
	}
	_ = app.Main().View(app)
	if writes != 1 {
		t.Fatalf("spectrum caused %d cover writes", writes)
	}
	state.playbackState = types.Paused
	if n.playbackBarRenderer.spectrumRows(120) != baseline {
		t.Fatal("pause retained spectrum")
	}
	state.playbackState = types.Playing
	state.frame.UpdatedAt = time.Now().Add(-time.Second)
	if n.playbackBarRenderer.spectrumRows(120) != baseline {
		t.Fatal("stale PCM retained spectrum")
	}
}

func TestPlayingLayoutStableAfterPlaybackAndLyricsLoad(t *testing.T) {
	for _, size := range [][2]int{{120, 52}, {90, 44}, {60, 44}} {
		app, n, r := newPlayingTestApp(t, true)
		state := &movingPlayingState{playingTestState: r.state.(playingTestState), playbackState: types.Paused}
		r.state = state
		_, _ = app.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		_ = app.Main().View(app)
		before := r.placement
		service := lyric.NewService(playingLyricFetcher{structs.LRCData{Original: "[00:01.00]一行比较长的歌词用于确认异步加载之后布局位置始终不变"}}, true, 0, false)
		if err := service.SetSong(context.Background(), state.CurSong()); err != nil {
			t.Fatal(err)
		}
		service.UpdatePosition(2 * time.Second)
		n.lyricRenderer = NewLyricRenderer(n, service, true)
		state.playbackState = types.Playing
		_ = app.Main().View(app)
		if r.placement != before {
			t.Fatalf("%v playback/lyrics moved cover: %+v -> %+v", size, before, r.placement)
		}
	}
}

func TestSpectrumBaselineAndConfigurationSwitch(t *testing.T) {
	app, n, r := newPlayingTestApp(t, false)
	state := &spectrumPlayingState{movingPlayingState: movingPlayingState{playingTestState: r.state.(playingTestState), playbackState: types.Playing}, frame: player.SpectrumFrame{Supported: true, UpdatedAt: time.Now()}}
	n.playbackBarRenderer.state = state
	empty := n.playbackBarRenderer.spectrumRows(120)
	if stripAnsiCodes(empty[2]) != strings.Repeat("─", 120) {
		t.Fatal("divider is not at the block baseline")
	}
	for i := range state.frame.Levels {
		state.frame.Levels[i] = 0.25
	}
	low := n.playbackBarRenderer.spectrumRows(120)
	if strings.Contains(stripAnsiCodes(low[1]), "─") || !strings.Contains(stripAnsiCodes(low[1]), "▄") {
		t.Fatal("bar does not grow from the divider baseline")
	}
	if strings.TrimSpace(stripAnsiCodes(low[0])) != "" {
		t.Fatal("low bar unexpectedly spans upper row")
	}
	_, _ = app.Update(tea.WindowSizeMsg{Width: 120, Height: 48})
	enabledHeight := playbackFooterHeight(120, 48)
	n.playbackBarRenderer.SetSpectrumEnabled(false)
	if playbackSpectrumVisible(120, 48, n.playbackBarRenderer.spectrumEnabled) {
		t.Fatal("disabled spectrum still visible")
	}
	if playbackFooterHeight(120, 48) != enabledHeight {
		t.Fatal("spectrum toggle moved the fixed footer")
	}
	view := stripAnsiCodes(n.playbackBarRenderer.ViewBottom(app, app.Main()))
	if strings.ContainsAny(view, "▁▂▃▄▅▆▇█") {
		t.Fatal("disabled spectrum left bars in footer")
	}
	if !strings.Contains(view, strings.Repeat("─", 120)) {
		t.Fatal("disabled footer lost divider")
	}
}

func TestNativeCoverUsesSquareAspectRatio(t *testing.T) {
	app, _, r := newPlayingTestApp(t, true)
	var output string
	r.write = func(s string) (int, error) { output += s; return len(s), nil }
	_, _ = app.Update(tea.WindowSizeMsg{Width: 120, Height: 48})
	_ = app.Main().View(app)
	start := strings.Index(output, "a=p,")
	if start < 0 {
		t.Fatal("cover was not placed")
	}
	placement := strings.SplitN(output[start:], "\x1b", 2)[0]
	if strings.Contains(placement, ",c=") || !strings.Contains(placement, ",r=") {
		t.Fatalf("placement forces image aspect: %q", placement)
	}
	for _, ratio := range []float64{1.6, 2, 2.6} {
		p := playingPanelSize(120, 30, true, true, ratio, 30)
		detailRows := p.headerLines + 1 + p.lyricLines
		if p.lyricGap {
			detailRows += p.lyricLines - 1
		}
		if p.coverRows != detailRows {
			t.Fatalf("cover and lyric block differ: %d vs %d", p.coverRows, detailRows)
		}
		if float64(p.coverCols) < float64(p.coverRows)*ratio {
			t.Fatal("square exceeds reserved width")
		}
	}
}
