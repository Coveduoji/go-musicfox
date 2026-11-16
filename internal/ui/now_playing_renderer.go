package ui

import (
	"context"
	"fmt"
	"image"
	"math"
	"os"
	"strings"
	"time"

	"github.com/anhoder/foxful-cli/model"
	"github.com/anhoder/foxful-cli/util"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/truncate"

	"github.com/go-musicfox/go-musicfox/internal/ui/kitty"
)

type playingPanelLayout struct {
	lyricGap                                                   bool
	horizontal                                                 bool
	coverCols, coverRows, lyricLines, detailWidth, headerLines int
	columnGap                                                  int
}

func playingPanelSize(width, height int, lyrics, cover bool, cellRatio float64, left int) playingPanelLayout {
	availableWidth := max(0, width-left)
	p := playingPanelLayout{horizontal: width >= 76, detailWidth: min(48, max(0, availableWidth-4)), headerLines: min(2, height)}
	available := height - p.headerLines - 1
	if lyrics {
		switch {
		case available >= 5:
			p.lyricLines = 5
		case available >= 3:
			p.lyricLines = 3
		case available >= 1:
			p.lyricLines = 1
		}
	}
	p.lyricGap = p.horizontal && height >= 14 && p.lyricLines == 5
	p.columnGap = min(6, 4+max(0, width-76)/12)
	if !cover || width < 16 {
		return p
	}
	if cellRatio <= 0 {
		cellRatio = 2
	}
	detailHeight := p.headerLines
	if p.lyricLines > 0 {
		detailHeight += p.lyricLines + 1
		if p.lyricGap {
			detailHeight += p.lyricLines - 1
		}
	}
	if p.horizontal {
		// Match the right-hand title/lyric block, constrained by available columns.
		p.coverRows = min(detailHeight, int(float64(max(0, availableWidth-p.columnGap-28))/cellRatio))
	} else {
		p.coverRows = min(10, int(float64(max(0, availableWidth-4))/cellRatio), height-detailHeight-1)
	}
	if p.coverRows < 3 {
		p.coverRows = 0
		return p
	}
	p.coverCols = int(math.Ceil(float64(p.coverRows) * cellRatio))
	if p.horizontal {
		p.detailWidth = min(48, max(0, availableWidth-p.coverCols-p.columnGap))
	}
	return p
}

type coverLoadResult struct{ img image.Image }
type coverPlacement struct{ x, y, cols, rows int }

// NowPlayingRenderer reuses the v4.8.5 image cache, Kitty protocol and lyric
// service. Native images are sent once per song, then only their placement moves.
type NowPlayingRenderer struct {
	netease          *Netease
	state            playerRendererState
	images           *kitty.ImageCache
	nativeSupported  bool
	write            func(string) (int, error)
	result           <-chan coverLoadResult
	cancel           context.CancelFunc
	url              string
	img              image.Image
	retryAt          time.Time
	imageID          uint32
	encoded          string
	uploaded, placed bool
	placement        coverPlacement
	writeRetryAt     time.Time
}

func NewNowPlayingRenderer(n *Netease, state playerRendererState) *NowPlayingRenderer {
	return &NowPlayingRenderer{netease: n, state: state, images: n.coverRenderer.imageCache, nativeSupported: n.coverRenderer.kittySupport, write: os.Stdout.WriteString, imageID: kitty.NewImageID()}
}

func (r *NowPlayingRenderer) cancelLoad() {
	if r.cancel != nil {
		r.cancel()
		r.cancel = nil
	}
	r.result = nil
}
func (r *NowPlayingRenderer) clearPlacement() {
	if r.placed {
		_, _ = r.write(kitty.DeleteImage(r.imageID))
	}
	r.placed = false
}

// Hide is also called by foxful-cli when leaving Main for a standalone page.
func (r *NowPlayingRenderer) Hide()       { r.cancelLoad(); r.clearPlacement(); r.uploaded = false }
func (r *NowPlayingRenderer) Invalidate() { r.placed = false; r.uploaded = false }

func (r *NowPlayingRenderer) Close() { r.Hide() }
func (r *NowPlayingRenderer) Update(msg tea.Msg, a *model.App) {
	r.netease.lyricRenderer.Update(msg, a)
	if _, ok := msg.(tea.WindowSizeMsg); ok {
		r.clearPlacement()
	}
}
func (r *NowPlayingRenderer) View(*model.App, *model.Main) (string, int) { return "", 0 }

func (r *NowPlayingRenderer) loadCover(url string) {
	if r.url != url {
		r.Hide()
		r.url = url
		r.img = nil
		r.encoded = ""
		r.retryAt = time.Time{}
		r.writeRetryAt = time.Time{}
	}
	select {
	case result := <-r.result:
		r.cancelLoad()
		r.img = result.img
		r.encoded = ""
		r.uploaded = false
		r.retryAt = time.Now().Add(30 * time.Second)
	default:
	}
	if r.img != nil || r.result != nil || url == "" || time.Now().Before(r.retryAt) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), kitty.DefaultHTTPTimeout)
	result := make(chan coverLoadResult, 1)
	r.cancel = cancel
	r.result = result
	go func() {
		defer cancel()
		img, _ := r.images.GetImage(ctx, url, 24, 12)
		if img != nil {
			img = kitty.ResizeImageSquare(img, 320)
		}
		result <- coverLoadResult{img: img}
		if ctx.Err() == nil {
			r.netease.App.Rerender(false)
		}
	}()
}

func (r *NowPlayingRenderer) placeCover(p coverPlacement) {
	if r.img == nil || time.Now().Before(r.writeRetryAt) || (r.placed && r.placement == p) {
		return
	}
	if r.encoded == "" {
		seq, err := kitty.TransmitImage(r.img, 0, 0, r.imageID)
		if err != nil {
			r.writeRetryAt = time.Now().Add(time.Second)
			return
		}
		r.encoded = seq
	}
	seq := ""
	if !r.uploaded {
		seq = r.encoded
	}
	if r.placed {
		seq += kitty.DeleteImage(r.imageID)
	}
	// Height matches the lyric block; Kitty computes the square width exactly.
	seq += fmt.Sprintf("\x1b[s\x1b[%d;%dH", p.y+1, p.x+1) + kitty.PlaceImage(r.imageID, 0, p.rows) + "\x1b[u"
	n, err := r.write(seq)
	if err != nil || n != len(seq) {
		r.placed = false
		r.uploaded = false
		r.writeRetryAt = time.Now().Add(time.Second)
		return
	}
	r.uploaded = true
	r.placed = true
	r.placement = p
}

// Raise the group within spare space without changing the menu or footer.
func playingPanelTop(height, contentHeight int) int {
	spare := max(0, height-contentHeight)
	return max(0, spare/2-min(4, spare/4))
}

func (r *NowPlayingRenderer) ViewWithSize(a *model.App, main *model.Main, width, height int) string {
	if width < 1 || height < 1 {
		r.Hide()
		return ""
	}
	song := r.state.CurSong()
	url := getCoverUrl(song)
	lr := r.netease.lyricRenderer
	ratio := 2.0
	if cw, ch := kitty.CellSize(); cw > 0 && ch > 0 {
		ratio = ch / cw
	}
	left := min(max(0, main.MenuStartColumn()), width-1)
	p := playingPanelSize(width, height, lr.isVisible, r.nativeSupported && song.Id != 0 && url != "", ratio, left)
	var cover string
	if p.coverCols > 0 {
		r.loadCover(url)
		cover = strings.Repeat(strings.Repeat(" ", p.coverCols)+"\n", p.coverRows-1) + strings.Repeat(" ", p.coverCols)
	} else {
		r.Hide()
	}
	title := cleanPlayingText(song.Name)
	if title == "" {
		title = "尚未播放"
	}
	details := []string{lipgloss.NewStyle().Bold(true).Render(util.SetFgStyle(truncatePlayingText(title, p.detailWidth, "…"), util.GetPrimaryColor()))}
	if p.headerLines > 1 {
		var artists []string
		for _, artist := range song.Artists {
			artists = append(artists, cleanPlayingText(artist.Name))
		}
		details = append(details, lipgloss.NewStyle().Foreground(LyricInactiveColor).Render(truncatePlayingText(strings.Join(artists, "/"), p.detailWidth, "…")))
	}
	if p.lyricLines > 0 {
		lr.SetCurrentTime(r.state.PassedTime().Milliseconds())
		old := lr.lyricLines
		lr.lyricLines = p.lyricLines
		lr.prepareLyricLines()
		lr.lyricLines = old
		details = append(details, "")
		empty := true
		for _, line := range lr.lyrics[:p.lyricLines] {
			if strings.TrimSpace(stripAnsiCodes(line)) != "" {
				empty = false
			}
		}
		for i, line := range lr.lyrics[:p.lyricLines] {
			if p.lyricGap && i > 0 {
				details = append(details, "")
			}
			if empty && i == p.lyricLines/2 {
				line = lipgloss.NewStyle().Foreground(LyricInactiveColor).Render("暂无歌词")
			}
			details = append(details, truncatePlayingText(cleanPlayingText(line), p.detailWidth, "…"))
		}
	}
	align := lipgloss.Left
	info := lipgloss.NewStyle().Width(p.detailWidth).Align(align).Render(strings.Join(details, "\n"))
	content := info
	if cover != "" {
		if p.horizontal {
			content = lipgloss.JoinHorizontal(lipgloss.Top, cover, strings.Repeat(" ", p.columnGap), info)
		} else {
			content = lipgloss.JoinVertical(lipgloss.Left, cover, "", info)
		}
	}
	groupHeight := lipgloss.Height(content)
	x := left
	top := playingPanelTop(height, groupHeight)
	if cover != "" {
		coverX, coverY := x, a.WindowHeight()-playbackFooterHeight(width, a.WindowHeight())-height+top
		r.placeCover(coverPlacement{coverX, coverY, p.coverCols, p.coverRows})
	}
	content = lipgloss.NewStyle().MarginLeft(x).Render(content)
	return lipgloss.Place(width, height, lipgloss.Left, lipgloss.Top, strings.Repeat("\n", top)+content)
}

func cleanPlayingText(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\n", " "), "\r", " ")
}
func truncatePlayingText(s string, width int, tail string) string {
	if width <= 0 {
		return ""
	}
	return truncate.StringWithTail(s, uint(width), tail)
}
