package ui

import (
	"context"
	"fmt"
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

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/anhoder/foxful-cli/model"
	"github.com/charmbracelet/x/ansi"

	"github.com/go-musicfox/go-musicfox/internal/configs"
	"github.com/go-musicfox/go-musicfox/internal/lyric"
	"github.com/go-musicfox/go-musicfox/internal/structs"
	"github.com/go-musicfox/go-musicfox/internal/ui/kitty"
)

func newPixelPanelTestApp(t *testing.T, status string) (*model.App, *Netease, *PixelCoverRenderer) {
	t.Helper()
	previous := configs.AppConfig
	configs.AppConfig = &configs.Config{}
	t.Cleanup(func() { configs.AppConfig = previous })
	n := &Netease{}
	state := songInfoTestState{song: structs.Song{Id: 1, Name: "山丘 (Live)", Artists: []structs.Artist{{Name: "李宗盛"}}, Duration: time.Minute}}
	n.lyricRenderer = NewLyricRenderer(n, lyric.NewService(nil, true, 0, false), true)
	n.songInfoRenderer = NewSongInfoRenderer(n, state)
	n.progressRenderer = NewProgressRenderer(n, state)
	r := &PixelCoverRenderer{netease: n, state: state, img: image.NewUniform(color.NRGBA{R: 210, G: 140, B: 80, A: 255})}
	opts := model.DefaultOptions()
	opts.EnableStartup = false
	opts.DualColumn = true
	opts.MainMenu = &pageLayoutTestMenu{itemCount: 10}
	opts.MainMenuTitle = &model.MenuItem{Title: "网易云音乐"}
	opts.Components = []model.Component{r, n.songInfoRenderer, n.progressRenderer}
	opts.StatusBar = nil
	if status != "none" {
		opts.StatusBar = &model.DefaultStatusBar{}
		if status == "top" {
			opts.StatusBarPosition = model.StatusBarTop
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	opts.TeaOptions = []tea.ProgramOption{tea.WithContext(ctx), tea.WithInput(nil), tea.WithOutput(io.Discard)}
	app := model.NewApp(opts)
	n.App = app
	_ = app.Run()
	t.Cleanup(r.Close)
	return app, n, r
}

func TestPixelPanelReflowsAndKeepsFooterFixed(t *testing.T) {
	for _, status := range []string{"none", "bottom", "top"} {
		t.Run(status, func(t *testing.T) {
			app, _, r := newPixelPanelTestApp(t, status)
			for _, size := range [][2]int{{120, 44}, {60, 44}, {40, 32}, {80, 16}, {24, 10}, {10, 5}, {24, 2}, {24, 1}, {120, 44}} {
				w, h := size[0], size[1]
				_, _ = app.Update(tea.WindowSizeMsg{Width: w, Height: h})
				view := app.Main().View(app)
				if got := lipgloss.Height(view); got != h {
					t.Fatalf("%dx%d: frame height %d", w, h, got)
				}
				for row, line := range strings.Split(view, "\n") {
					if got := ansi.StringWidth(line); got > w {
						t.Fatalf("%dx%d row %d: width=%d", w, h, row, got)
					}
				}
				progressRow := h - 1
				if status == "bottom" {
					progressRow--
				}
				marker := "00:00/01:00"
				if w < ProgressTimeDisplayWidth {
					marker = "00:00"
				}
				if progressRow >= 0 {
					if got := visibleRowContaining(t, view, marker, h); got != progressRow {
						t.Fatalf("%dx%d: progress row=%d want=%d", w, h, got, progressRow)
					}
				}
				if h >= 32 {
					rows := strings.Split(ansi.Strip(view), "\n")
					first, last, lyricRow := -1, -1, -1
					for i, row := range rows {
						if strings.Contains(row, "▀") {
							if first < 0 {
								first = i
							}
							last = i
						}
						if strings.Contains(row, "暂无歌词") {
							lyricRow = i
						}
					}
					if first < 0 || lyricRow < 0 {
						t.Fatalf("%dx%d: missing cover/lyrics", w, h)
					}
					if w >= 76 && (lyricRow < first || lyricRow > last) {
						t.Fatalf("wide layout does not place lyrics next to cover")
					}
					if w < 76 && lyricRow <= last {
						t.Fatalf("narrow layout does not place lyrics below cover")
					}
				}
				// The test image remains cached across shrink/expand cycles.
				if r.img == nil {
					t.Fatal("resize discarded the source image")
				}
			}
		})
	}
}

func TestPixelPanelFitsRemainingViewport(t *testing.T) {
	app, _, r := newPixelPanelTestApp(t, "none")
	for _, w := range []int{1, 7, 12, 24, 40, 60, 75, 76, 120} {
		for _, h := range []int{0, 1, 2, 3, 5, 7, 10, 16} {
			t.Run(fmt.Sprintf("%dx%d", w, h), func(t *testing.T) {
				view := r.ViewWithSize(app, app.Main(), w, h)
				if h == 0 {
					if view != "" {
						t.Fatal("zero-height viewport produced content")
					}
					return
				}
				if got := lipgloss.Height(view); got > h {
					t.Fatalf("height=%d exceeds %d", got, h)
				}
				for _, row := range strings.Split(view, "\n") {
					if got := ansi.StringWidth(row); got > w {
						t.Fatalf("width=%d exceeds %d", got, w)
					}
				}
			})
		}
	}
}

func TestPixelCoverUsesBothCellColors(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	for x := range 2 {
		img.SetNRGBA(x, 0, color.NRGBA{R: 255, A: 255})
		img.SetNRGBA(x, 1, color.NRGBA{B: 255, A: 255})
	}
	rows := renderPixelCover(img, 2)
	screen := formStyledScreen(rows[0])
	cell := screen.CellAt(0, 0)
	if cell.Content != "▀" || !colorsEqual(cell.Style.Fg, color.NRGBA{R: 255, A: 255}) || !colorsEqual(cell.Style.Bg, color.NRGBA{B: 255, A: 255}) {
		t.Fatalf("pixel colors lost: %+v", cell)
	}
}

func TestPixelCoverDiscardsCancelledSongResult(t *testing.T) {
	_, _, r := newPixelPanelTestApp(t, "none")
	previous := make(chan pixelCoverResult, 1)
	r.url = "old"
	r.result = previous
	r.cover("", 8)
	previous <- pixelCoverResult{img: image.NewUniform(color.White)}
	r.cover("", 8)
	if r.img != nil {
		t.Fatal("stale song cover replaced the current placeholder")
	}
}

func TestPlaybarIconHitBoundsFollowClippedText(t *testing.T) {
	app, n, _ := newPixelPanelTestApp(t, "none")
	handler := &EventHandler{netease: n}
	for _, centered := range []bool{false, true} {
		app.With(func(o *model.Options) { o.CenterEverything = centered })
		for _, width := range []int{120, 80, 40, 24, 12} {
			_, _ = app.Update(tea.WindowSizeMsg{Width: width, Height: 36})
			view, _ := n.songInfoRenderer.View(app, app.Main())
			plain := strings.Split(ansi.Strip(view), "\n")[0]
			for _, bound := range n.playbarBounds {
				if bound.end > width {
					t.Fatalf("hit target extends past %d: %+v", width, bound)
				}
				if handler.playbarElementAt(bound.start) != bound.element || handler.playbarElementAt(bound.end-1) != bound.element {
					t.Fatalf("hit target mismatch: %+v", bound)
				}
				expected := ""
				switch bound.element {
				case PlaybarElementState:
					expected = "[▶]"
				case PlaybarElementHeart:
					expected = "♡"
				}
				if expected != "" {
					pos := strings.Index(plain, expected)
					if pos < 0 || ansi.StringWidth(plain[:pos]) != bound.start {
						t.Fatalf("icon %q does not match hit target %+v: %q", expected, bound, plain)
					}
				}
			}
			// Empty padding has no accidental song/artist action.
			if width >= 80 && handler.playbarElementAt(width-1) != PlaybarElementNone {
				t.Fatalf("trailing blank is clickable: %q", plain)
			}
		}
	}
}

func TestPixelCoverFetchesOnceAcrossResize(t *testing.T) {
	_, _, r := newPixelPanelTestApp(t, "none")
	r.img = nil
	r.images = kitty.NewImageCache(2)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
		if err := png.Encode(w, img); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	r.cover(server.URL, 12)
	// Await the worker's buffered result without a polling loop or sleeps.
	var result pixelCoverResult
	select {
	case result = <-r.result:
	case <-time.After(3 * time.Second):
		t.Fatal("cover worker did not finish")
	}
	ready := make(chan pixelCoverResult, 1)
	ready <- result
	r.result = ready
	for _, cols := range []int{12, 24, 8, 32} {
		rows := r.cover(server.URL, cols)
		if len(rows) != cols/2 {
			t.Fatalf("wrong cover height for %d columns", cols)
		}
	}
	if requests.Load() != 1 {
		t.Fatalf("resizing downloaded the cover %d times", requests.Load())
	}
	if r.img == nil {
		t.Fatal("downloaded cover was not adopted")
	}
}
