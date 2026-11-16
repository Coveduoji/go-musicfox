package ui

import (
	"context"
	"image"
	"image/color"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/anhoder/foxful-cli/model"
	"github.com/anhoder/foxful-cli/style"
	"github.com/charmbracelet/x/ansi"

	"github.com/go-musicfox/go-musicfox/internal/ui/kitty"
)

type pixelPanelLayout struct {
	horizontal                        bool
	coverCols, lyricLines, lyricWidth int
}

func pixelPanelSize(width, height int, lyrics bool) pixelPanelLayout {
	p := pixelPanelLayout{horizontal: width >= 76, lyricWidth: max(0, width-4)}
	if width < 8 || height < 1 {
		return p
	}
	if lyrics {
		p.lyricLines = 5
		if height < 12 {
			p.lyricLines = 3
		}
		if height < 5 {
			p.lyricLines = 1
		}
	}
	if p.horizontal {
		p.coverCols = min(32, (width-10)/3, height*2)
		p.coverCols -= p.coverCols % 2
		if p.coverCols < 6 {
			p.coverCols = 0
		}
		p.lyricWidth = min(48, max(0, width-p.coverCols-8))
	} else {
		rows := min(12, (width-4)/2, height-p.lyricLines-2)
		if rows >= 3 {
			p.coverCols = rows * 2
		}
	}
	return p
}

type pixelCoverResult struct{ img image.Image }

// PixelCoverRenderer shares the existing image cache and lyric service. Workers
// only return immutable images; all UI state stays on the Bubble Tea thread.
type PixelCoverRenderer struct {
	netease  *Netease
	state    playerRendererState
	images   *kitty.ImageCache
	result   <-chan pixelCoverResult
	cancel   context.CancelFunc
	url      string
	img      image.Image
	retryAt  time.Time
	rows     []string
	cols     int
	styleGen uint64
}

func NewPixelCoverRenderer(n *Netease, state playerRendererState) *PixelCoverRenderer {
	return &PixelCoverRenderer{netease: n, state: state, images: n.coverRenderer.imageCache}
}

func (r *PixelCoverRenderer) Close() {
	if r.cancel != nil {
		r.cancel()
		r.cancel = nil
	}
	r.result = nil
}

func (r *PixelCoverRenderer) Update(msg tea.Msg, a *model.App) {
	r.netease.lyricRenderer.Update(msg, a)
}

// Main supplies the actual remaining viewport through ViewWithSize.
func (r *PixelCoverRenderer) View(*model.App, *model.Main) (string, int) { return "", 0 }

func (r *PixelCoverRenderer) cover(url string, cols int) []string {
	if r.url != url {
		r.Close()
		r.url = url
		r.img = nil
		r.rows = nil
		r.retryAt = time.Time{}
	}
	select {
	case result := <-r.result:
		r.img = result.img
		r.rows = nil
		r.Close()
		r.retryAt = time.Now().Add(30 * time.Second)
	default:
	}
	if url != "" && r.img == nil && r.result == nil && !time.Now().Before(r.retryAt) {
		ctx, cancel := context.WithTimeout(context.Background(), kitty.DefaultHTTPTimeout)
		result := make(chan pixelCoverResult, 1)
		r.cancel = cancel
		r.result = result
		go func() {
			defer cancel()
			// A stable cache size prevents another download on each window resize.
			img, _ := r.images.GetImage(ctx, url, 32, 16)
			result <- pixelCoverResult{img: img}
			if ctx.Err() == nil {
				r.netease.App.Rerender(false)
			}
		}()
	}
	generation := style.StyleGeneration()
	if r.rows == nil || r.cols != cols || r.styleGen != generation {
		r.rows = renderPixelCover(r.img, cols)
		r.cols = cols
		r.styleGen = generation
	}
	return r.rows
}

// Each half-block cell contains two square pixels, matching the typical 1:2
// terminal cell aspect ratio. The existing image cache handles fetch and decode.
func renderPixelCover(img image.Image, cols int) []string {
	rows := make([]string, cols/2)
	base := style.CurrentStyleSet().AppBackground
	if img == nil {
		for y := range rows {
			rows[y] = style.CurrentStyleSet().Muted.Inherit(base).Width(cols).Align(lipgloss.Center).Render("░ ░ ░")
		}
		return rows
	}
	bounds := img.Bounds()
	size := min(bounds.Dx(), bounds.Dy())
	ox, oy := bounds.Min.X+(bounds.Dx()-size)/2, bounds.Min.Y+(bounds.Dy()-size)/2
	sample := func(x, y int) color.NRGBA {
		return color.NRGBAModel.Convert(img.At(ox+min(size-1, x*size/cols), oy+min(size-1, y*size/cols))).(color.NRGBA)
	}
	for y := range rows {
		var b strings.Builder
		for x := range cols {
			top, bottom := sample(x, y*2), sample(x, y*2+1)
			switch {
			case top.A < 128 && bottom.A < 128:
				b.WriteString(base.Render(" "))
			case top.A < 128:
				b.WriteString(base.Foreground(bottom).Render("▄"))
			case bottom.A < 128:
				b.WriteString(base.Foreground(top).Render("▀"))
			default:
				b.WriteString(base.Foreground(top).Background(bottom).Render("▀"))
			}
		}
		rows[y] = b.String()
	}
	return rows
}

func (r *PixelCoverRenderer) ViewWithSize(a *model.App, main *model.Main, width, height int) string {
	if width < 1 || height < 1 {
		r.Close()
		return ""
	}
	lr := r.netease.lyricRenderer
	p := pixelPanelSize(width, height, lr.isVisible)
	var cover, lyrics string
	if p.coverCols > 0 && r.state.CurSong().Id != 0 {
		cover = strings.Join(r.cover(getCoverUrl(r.state.CurSong()), p.coverCols), "\n")
	} else {
		r.Close()
	}
	if p.lyricLines > 0 {
		lr.SetCurrentTime(r.state.PassedTime().Milliseconds())
		previous := lr.lyricLines
		lr.lyricLines = p.lyricLines
		lr.prepareLyricLines()
		lr.lyricLines = previous
		rows := make([]string, len(lr.lyrics))
		empty := true
		for i, row := range lr.lyrics {
			row = strings.ReplaceAll(strings.ReplaceAll(row, "\n", " "), "\r", " ")
			rows[i] = ansi.Truncate(row, p.lyricWidth, "…")
			if strings.TrimSpace(ansi.Strip(row)) != "" {
				empty = false
			}
		}
		if empty {
			rows = []string{ansi.Truncate(style.CurrentStyleSet().Muted.Render("暂无歌词"), p.lyricWidth, "")}
		}
		lyrics = lipgloss.NewStyle().Inherit(style.CurrentStyleSet().AppBackground).Width(p.lyricWidth).Align(lipgloss.Center).Render(strings.Join(rows, "\n"))
	}
	content := lyrics
	if cover != "" {
		if lyrics == "" {
			content = cover
		} else if p.horizontal {
			content = lipgloss.JoinHorizontal(lipgloss.Center, cover, "    ", lyrics)
		} else {
			content = lipgloss.JoinVertical(lipgloss.Center, cover, "", lyrics)
		}
	}
	// Place never extends beyond the actual space left above the fixed footer.
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content, lipgloss.WithWhitespaceStyle(style.CurrentStyleSet().AppBackground))
}
