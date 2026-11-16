package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/anhoder/foxful-cli/model"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/go-musicfox/go-musicfox/internal/configs"
)

type gridTestMenu struct {
	model.DefaultMenu
	count int
}

func (gridTestMenu) GetMenuKey() string { return "three-column-test" }
func (m gridTestMenu) MenuViews() []model.MenuItem {
	items := make([]model.MenuItem, m.count)
	for i := range items {
		items[i] = model.MenuItem{Title: fmt.Sprintf("菜单测试项%d", i)}
	}
	return items
}
func setupMenuGrid(t *testing.T, native bool, count int) (*model.App, *Netease, *NowPlayingRenderer) {
	app, n, r := newPlayingTestApp(t, native)
	configs.AppConfig.Theme.MenuColumns = 3
	app.With(func(opts *model.Options) { opts.MenuColumns = 3 })
	app.Main().EnterMenu(&gridTestMenu{count: count}, &model.MenuItem{Title: "网易云音乐"})
	return app, n, r
}
func TestResponsiveMenuColumnsAlignmentAndMouseBounds(t *testing.T) {
	app, n, r := setupMenuGrid(t, true, 40)
	handler := &EventHandler{netease: n}
	for _, tc := range []struct{ width, columns int }{{160, 3}, {120, 3}, {110, 3}, {109, 2}, {75, 2}, {74, 1}, {40, 1}, {10, 1}, {160, 3}} {
		_, _ = app.Update(tea.WindowSizeMsg{Width: tc.width, Height: 52})
		view := stripAnsiCodes(app.Main().View(app))
		if app.Main().MenuColumnCount() != tc.columns {
			t.Fatalf("width %d: wrong columns", tc.width)
		}
		if r.placed && r.placement.x != app.Main().MenuStartColumn() {
			t.Fatal("cover lost alignment to first menu column")
		}
		found := 0
		columnStarts := make(map[int]int)
		for y, row := range strings.Split(view, "\n") {
			if lipgloss.Width(row) > tc.width {
				t.Fatalf("%d: grid overflow", tc.width)
			}
			for index := 0; index < app.Main().PageSize(); index++ {
				label := fmt.Sprintf(" %*d. 菜单测试项%d", len(fmt.Sprint(app.Main().PageSize()-1)), index, index)
				at := strings.Index(row, label)
				if at < 0 {
					continue
				}
				x := lipgloss.Width(row[:at])
				got, ok := handler.getClickedIndexFromPosition(tea.MouseMsg{X: x, Y: y}, app, app.Main())
				if !ok || got != index {
					t.Fatalf("%d: item %d at (%d,%d) hit %d/%v", tc.width, index, x, y, got, ok)
				}
				found++
				column := index % tc.columns
				if previous, ok := columnStarts[column]; ok && previous != x {
					t.Fatal("menu entries do not share the column left edge")
				}
				columnStarts[column] = x

			}
		}
		if tc.width >= 75 && found != app.Main().PageSize() {
			t.Fatalf("%d: only found %d rendered items", tc.width, found)
		}
	}
}
func TestThreeColumnNavigationPagingAndResize(t *testing.T) {
	app, _, _ := setupMenuGrid(t, false, 40)
	_, _ = app.Update(tea.WindowSizeMsg{Width: 140, Height: 52})
	main := app.Main()
	main.MoveRight()
	if main.SelectedIndex() != 1 {
		t.Fatal("right did not move one column")
	}
	main.MoveDown()
	if main.SelectedIndex() != 4 {
		t.Fatal("down did not move one three-column row")
	}
	main.MoveUp()
	if main.SelectedIndex() != 1 {
		t.Fatal("up changed selected column")
	}
	main.MoveLeft()
	main.MoveLeft()
	if main.SelectedIndex() != 0 {
		t.Fatal("left crossed first column")
	}
	main.MoveBottom()
	if main.SelectedIndex() != 39 {
		t.Fatal("bottom lost column")
	}
	main.MoveTop()
	if main.SelectedIndex() != 0 || main.CurPage() != 1 {
		t.Fatal("top failed")
	}
	main.NextPage()
	if main.SelectedIndex() != 12 || main.CurPage() != 2 {
		t.Fatal("page next did not retain selected cell")
	}
	main.PrePage()
	if main.SelectedIndex() != 0 || main.CurPage() != 1 {
		t.Fatal("page previous failed")
	}
	main.MoveBottom()
	for _, width := range []int{90, 60, 140} {
		_, _ = app.Update(tea.WindowSizeMsg{Width: width, Height: 52})
		if main.SelectedIndex() != 39 || main.CurPage() != (39/main.PageSize()+1) {
			t.Fatal("resize changed or hid selection")
		}
		_ = main.View(app)
	}
}

type longGridMenu struct{ model.DefaultMenu }

func (*longGridMenu) GetMenuKey() string { return "long-grid-test" }
func (*longGridMenu) MenuViews() []model.MenuItem {
	return []model.MenuItem{{Title: "\x1b[31m" + strings.Repeat("长标题", 30) + "\x1b[0m"}, {Title: "第二列"}, {Title: strings.Repeat("超长歌单", 30)}}
}
func TestMenuGridClipsStyledTitlesWithoutOverlap(t *testing.T) {
	app, n, _ := setupMenuGrid(t, false, 3)
	app.Main().EnterMenu(&longGridMenu{}, &model.MenuItem{Title: "长标题菜单"})
	for _, width := range []int{110, 75, 32, 5, 1} {
		_, _ = app.Update(tea.WindowSizeMsg{Width: width, Height: 52})
		view := stripAnsiCodes(app.Main().View(app))
		for _, row := range strings.Split(view, "\n") {
			if lipgloss.Width(row) > width {
				t.Fatalf("%d: styled row overflow", width)
			}
		}
		if width >= 75 {
			handler := &EventHandler{netease: n}
			for y, row := range strings.Split(view, "\n") {
				if !strings.Contains(row, "长标题") {
					continue
				}
				// A visible first-column title must remain selectable after ANSI-aware clipping.
				at := strings.Index(row, "长标题")
				_, ok := handler.getClickedIndexFromPosition(tea.MouseMsg{X: lipgloss.Width(row[:at]), Y: y}, app, app.Main())
				if y >= app.Main().MenuStartRow()-1 && !ok {
					t.Fatal("clipped styled title lost its mouse bounds")
				}
			}
		}
	}
}
