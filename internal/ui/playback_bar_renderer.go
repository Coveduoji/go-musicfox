package ui

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/anhoder/foxful-cli/model"
	"github.com/anhoder/foxful-cli/util"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/go-musicfox/go-musicfox/internal/configs"
	"github.com/go-musicfox/go-musicfox/internal/player"
	"github.com/go-musicfox/go-musicfox/internal/types"
	"github.com/go-musicfox/go-musicfox/utils/likelist"
)

const playbackRed = "#EA403F"
const playbackWhite = "#FFFFFF"

type playbackHit struct {
	x, y, width int
	action      string
}
type playbackSegment struct {
	x            int
	text, action string
	active       bool
	iconCells    int
}

func playbackSpectrumVisible(width, height int, enabled bool) bool {
	return enabled && width >= 32 && height >= 20
}
func playbackFooterRows(width, height int) int {
	rows := 3
	if width < 76 {
		rows++
	}
	if playbackSpectrumVisible(width, height, true) {
		rows += 2
	}
	return rows
}
func playbackFooterHeight(width, height int) int {
	return min(max(0, height), playbackFooterRows(width, height))
}

// PlaybackBarRenderer groups controls and displays PCM-driven spectrum levels.
type PlaybackBarRenderer struct {
	netease         *Netease
	state           playerRendererState
	progress        *ProgressRenderer
	hits            []playbackHit
	spectrumWidth   int
	spectrumLevels  [player.SpectrumBands]uint8
	spectrumFrame   [3]string
	icons           *playbackIcons
	spectrumImage   *spectrumImage
	spectrumEnabled bool
}

// SetSpectrumEnabled changes only the current session, preserving startup defaults.
func (r *PlaybackBarRenderer) SetSpectrumEnabled(enabled bool) {
	r.spectrumEnabled = enabled
	if r.netease.player != nil {
		r.netease.player.SetSpectrumEnabled(enabled)
	}
}
func NewPlaybackBarRenderer(n *Netease, state playerRendererState) *PlaybackBarRenderer {
	r := &PlaybackBarRenderer{netease: n, state: state, progress: NewProgressRenderer(n, state), spectrumEnabled: configs.AppConfig.SpectrumEnabled()}
	if n.nowPlayingRenderer != nil && n.nowPlayingRenderer.nativeSupported {
		r.icons = newPlaybackIcons(n.nowPlayingRenderer.write)
		r.spectrumImage = newSpectrumImage(n.nowPlayingRenderer.write)
	}
	return r
}
func (r *PlaybackBarRenderer) Update(msg tea.Msg, _ *model.App) {
	r.hits = nil
	if _, ok := msg.(tea.WindowSizeMsg); ok {
		r.Hide()
	}
}
func (r *PlaybackBarRenderer) Hide() {
	if r.spectrumImage != nil {
		r.spectrumImage.Hide()
	}
	if r.icons != nil {
		r.icons.Hide()
	}
}
func (r *PlaybackBarRenderer) Invalidate() {
	if r.spectrumImage != nil {
		r.spectrumImage.Invalidate()
	}
	if r.icons != nil {
		r.icons.Invalidate()
	}
}
func (r *PlaybackBarRenderer) View(*model.App, *model.Main) (string, int) { return "", 0 }
func (r *PlaybackBarRenderer) hit(x, y int) *playbackHit {
	for i := range r.hits {
		h := &r.hits[i]
		if y == h.y && x >= h.x && x < h.x+h.width {
			return h
		}
	}
	return nil
}

func modeSymbol(mode types.Mode) string {
	switch mode {
	case types.PmOrdered:
		return "→"
	case types.PmSingleLoop:
		return "↻1"
	case types.PmListRandom, types.PmInfRandom:
		return "⇄"
	case types.PmIntelligent:
		return "♥"
	default:
		return "↻"
	}
}
func playbackClock(seconds int) string {
	seconds = max(0, seconds)
	return fmt.Sprintf("%02d:%02d", seconds/60, seconds%60)
}

func (r *PlaybackBarRenderer) spectrumRows(width int) [3]string {
	var levels [player.SpectrumBands]uint8
	if provider, ok := r.state.(player.SpectrumProvider); ok && r.state.State() == types.Playing {
		frame := provider.Spectrum()
		if frame.Supported && !frame.UpdatedAt.IsZero() && time.Since(frame.UpdatedAt) < 400*time.Millisecond {
			for i, value := range frame.Levels {
				levels[i] = uint8(math.Round(max(0, min(1, value)) * 16))
			}
		}
	}
	if width == r.spectrumWidth && levels == r.spectrumLevels {
		return r.spectrumFrame
	}
	white := lipgloss.NewStyle().Foreground(lipgloss.Color(playbackWhite))
	glyphs := []rune{' ', '▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}
	var upper, lower, reflection strings.Builder
	// Separate the bars while keeping the lower baseline connected through each gap.
	bars := (width + 1) / 2
	for i := 0; i < bars; i++ {
		pos := float64(i) * float64(player.SpectrumBands-1) / float64(max(1, bars-1))
		lo := int(pos)
		hi := min(player.SpectrumBands-1, lo+1)
		height := int(math.Round(float64(levels[lo]) + (float64(levels[hi])-float64(levels[lo]))*(pos-float64(lo))))
		if height > 8 {
			upper.WriteString(white.Render(string(glyphs[height-8])))
		} else {
			upper.WriteByte(' ')
		}
		lower.WriteRune(glyphs[min(8, height)])
		// The fixed divider is centered in the last row, matching the disabled line.
		if height > 0 {
			reflection.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(playbackWhite)).Background(lipgloss.Color("#363636")).Render("▀"))
		} else {
			reflection.WriteString(lipgloss.NewStyle().Foreground(LyricInactiveColor).Render("─"))
		}
		if i*2+1 < width {
			upper.WriteByte(' ')
			lower.WriteByte(' ')
			reflection.WriteString(lipgloss.NewStyle().Foreground(LyricInactiveColor).Render("─"))
		}
	}
	r.spectrumWidth = width
	r.spectrumLevels = levels
	r.spectrumFrame = [3]string{upper.String(), white.Render(lower.String()), reflection.String()}
	return r.spectrumFrame
}

func (r *PlaybackBarRenderer) renderRow(width, y int, segments []playbackSegment) string {
	sort.SliceStable(segments, func(i, j int) bool { return segments[i].x < segments[j].x })
	var b strings.Builder
	cursor := 0
	for _, s := range segments {
		if s.x < cursor || s.x >= width {
			continue
		}
		b.WriteString(strings.Repeat(" ", s.x-cursor))
		text := truncatePlayingText(s.text, width-s.x, "")
		w := lipgloss.Width(text)
		if r.icons != nil && s.iconCells > 0 && w >= s.iconCells && r.icons.draw(s.action, r.iconName(s.action), s.x, y, s.iconCells) {
			// Icon prefixes are plain text; captions remain normal terminal text.
			plain := stripAnsiCodes(text)
			prefix := truncatePlayingText(plain, s.iconCells, "")
			text = strings.Repeat(" ", s.iconCells) + strings.TrimPrefix(plain, prefix)
		}
		text = lipgloss.NewStyle().Foreground(lipgloss.Color(playbackWhite)).Render(text)
		if s.active {
			text = util.SetFgStyle(text, termenv.RGBColor(playbackRed))
		}
		b.WriteString(text)
		if w > 0 && s.action != "" && y >= 0 {
			r.hits = append(r.hits, playbackHit{s.x, y, w, s.action})
		}
		cursor = s.x + w
	}
	b.WriteString(strings.Repeat(" ", max(0, width-cursor)))
	return b.String()
}

func playbackCapsule(label string) string {
	primary := termenv.RGBColor(playbackRed)
	return util.SetFgStyle("◖", primary) + util.SetFgBgStyle(" "+label+" ", termenv.ANSIWhite, primary) + util.SetFgStyle("◗", primary)
}

func (r *PlaybackBarRenderer) extras(width int) []playbackSegment {
	gap := 2
	if width < 20 {
		gap = 1
	}
	cells := 1
	if width >= 48 {
		cells = 3
	}
	items := []playbackSegment{{text: playbackIconFallback("≡", cells), action: "queue", iconCells: cells}, {text: playbackIconFallback("♪", cells), action: "spectrum", iconCells: cells, active: r.spectrumEnabled}}
	x := 0
	for i := range items {
		items[i].x = x
		x += lipgloss.Width(items[i].text) + gap
	}
	items = append(items, playbackSegment{x: x, text: playbackIconFallback("−", cells), action: "volumeDown", iconCells: cells})
	x += cells
	if width >= 28 {
		volume := max(0, min(100, r.state.Volume()))
		dot := (volume*4 + 50) / 100
		track := strings.Repeat("━", dot) + "●" + strings.Repeat("─", 4-dot)
		items = append(items, playbackSegment{x: x, text: track, action: "volume"})
		x += 5
	}
	items = append(items, playbackSegment{x: x, text: playbackIconFallback("+", cells), action: "volumeUp", iconCells: cells})
	x += cells
	if width >= 20 {
		items = append(items, playbackSegment{x: x + 1, text: fmt.Sprintf("%d%%", r.state.Volume())})
	}
	return items
}
func playbackSegmentsWidth(items []playbackSegment) int {
	width := 0
	for _, s := range items {
		width = max(width, s.x+lipgloss.Width(s.text))
	}
	return width
}

func (r *PlaybackBarRenderer) ViewBottom(a *model.App, _ *model.Main) string {
	width, height := a.WindowWidth(), a.WindowHeight()
	r.hits = nil
	if width <= 0 || height <= 0 {
		return ""
	}
	if r.icons != nil {
		r.icons.begin()
		defer r.icons.end()
	}
	fullHeight := playbackFooterRows(width, height)
	rows := make([]string, fullHeight)
	for i := range rows {
		rows[i] = strings.Repeat(" ", width)
	}
	separatorRow := fullHeight - 3
	if width < 76 {
		separatorRow--
	}
	progressRow := separatorRow + 1
	rows[separatorRow] = lipgloss.NewStyle().Foreground(LyricInactiveColor).Render(strings.Repeat("─", width))
	if separatorRow >= 2 {
		var levels [player.SpectrumBands]uint8
		if r.spectrumEnabled {
			spectrum := r.spectrumRows(width)
			rows[separatorRow-2], rows[separatorRow-1], rows[separatorRow] = spectrum[0], spectrum[1], spectrum[2]
			levels = r.spectrumLevels
		}
		// The same native image owns the line even when disabled, so its pixel
		// baseline never changes between text and graphics rendering.
		if r.spectrumImage != nil && r.spectrumImage.show(width, height-fullHeight+separatorRow-2, levels) {
			for i := separatorRow - 2; i <= separatorRow; i++ {
				rows[i] = strings.Repeat(" ", width)
			}
		}
	} else if r.spectrumImage != nil {
		r.spectrumImage.Hide()
	}
	total := max(0, int(r.state.CurSong().Duration.Seconds()))
	elapsed := min(total, max(0, int(r.state.PassedTime().Seconds())))
	left, right := playbackClock(elapsed), playbackClock(total)
	progressWidth := min(80, max(0, width-4))
	progressX := (width - progressWidth) / 2
	barWidth := progressWidth - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if barWidth < 1 {
		right = ""
		barWidth = max(0, progressWidth-lipgloss.Width(left)-1)
	}
	progress := left
	progressY := height - fullHeight + progressRow
	if barWidth > 0 {
		progress += " " + r.progress.renderBar(barWidth)
		if total > 0 && progressY >= 0 {
			r.hits = append(r.hits, playbackHit{progressX + lipgloss.Width(left) + 1, progressY, barWidth, "seek"})
		}
	}
	if right != "" {
		progress += " " + right
	}
	rows[progressRow] = r.renderRow(width, progressY, []playbackSegment{{x: progressX, text: progress}})

	// Like and mode form the left group; only transport buttons occupy the center.
	inset := min(2, max(0, width-1))
	if width < 12 {
		inset = 0
	}
	likeAction := ""
	liked := false
	if r.state.CurSong().Id > 0 {
		likeAction = "heart"
		liked = likelist.IsLikeSong(r.state.CurSong().Id)
	}
	cells := 1
	if width >= 48 {
		cells = 3
	}
	heart := "♡"
	if liked {
		heart = "♥"
	}
	heart = playbackIconFallback(heart, cells)
	controls := []playbackSegment{{x: inset, text: heart, action: likeAction, active: liked, iconCells: cells}}
	leftEdge := inset + lipgloss.Width(heart) + 2
	if width >= 22 {
		mode := lipgloss.NewStyle().Width(cells).Align(lipgloss.Center).Render(modeSymbol(r.state.Mode()))
		if width >= 108 {
			mode += " " + r.state.Mode().Name()
		}
		controls = append(controls, playbackSegment{x: leftEdge, text: mode, action: "mode", iconCells: cells, active: true})
		leftEdge += lipgloss.Width(mode) + 3
	}
	extras := r.extras(width)
	extrasWidth := playbackSegmentsWidth(extras)
	rightEdge := width - 2
	if width >= 76 {
		start := max(0, width-extrasWidth-2)
		for i := range extras {
			extras[i].x += start
		}
		controls = append(controls, extras...)
		rightEdge = start - 2
	} else {
		offset := max(0, (width-extrasWidth)/2)
		for i := range extras {
			extras[i].x += offset
		}
		rows[fullHeight-2] = r.renderRow(width, height-2, extras)
	}
	label := "Ⅱ"
	if r.state.State() != types.Playing {
		label = "▶"
	}
	buttons := []playbackSegment{{text: playbackIconFallback("⏮", cells), action: "previous", iconCells: cells}, {text: playbackCapsule(label), action: "state", iconCells: 5}, {text: playbackIconFallback("⏭", cells), action: "next", iconCells: cells}}
	buttonWidth := func() int {
		w := 0
		for _, b := range buttons {
			w += lipgloss.Width(b.text) + 3
		}
		return max(0, w-3)
	}
	if width < 12 {
		rightEdge = width
	}
	if buttonWidth() > rightEdge-leftEdge {
		buttons = buttons[1:2]
	}
	if buttonWidth() > rightEdge-leftEdge {
		buttons[0].text = label
		buttons[0].iconCells = 0
	}
	start := max(leftEdge, (width-buttonWidth())/2)
	if start+buttonWidth() > rightEdge {
		start = max(leftEdge, rightEdge-buttonWidth())
	}
	for _, button := range buttons {
		button.x = start
		controls = append(controls, button)
		start += lipgloss.Width(button.text) + 3
	}
	rows[fullHeight-1] = r.renderRow(width, height-1, controls)
	return strings.Join(rows[fullHeight-min(fullHeight, height):], "\n")
}
