package model

import (
	"fmt"
	"strings"

	"github.com/anhoder/foxful-cli/util"
	"github.com/muesli/reflow/ansi"
	"github.com/muesli/reflow/truncate"
)

type menuGridHit struct{ index, x, y, width int }

// MenuColumnCount exposes the actual responsive column count.
func (m *Main) MenuColumnCount() int { return m.getNumColumns() }

func (m *Main) configureMenuGrid(width int) {
	m.gridColumns = 0
	if m.options.MenuColumns <= 0 {
		return
	}
	m.gridColumns = gridMin(3, m.options.MenuColumns)
	if width < 110 {
		m.gridColumns = gridMin(2, m.gridColumns)
	}
	if width < 75 {
		m.gridColumns = 1
	}
	m.isDualColumn = m.gridColumns > 1
}
func (m *Main) finalizeMenuGrid(width int) {
	if m.gridColumns == 0 {
		return
	}
	columns := m.gridColumns
	m.menuPageSize = gridMax(columns, (m.menuPageSize+columns-1)/columns*columns)
	m.menuCurPage = gridMax(1, m.selectedIndex/m.menuPageSize+1)
	m.menuStartColumn = gridMin(gridMax(0, width-1), gridMax(2, width/20))
	m.menuTitleStartColumn = m.menuStartColumn
	m.menuBottomRow = m.menuStartRow + (m.menuPageSize+columns-1)/columns + 1
	m.gridHits = nil
}

func (m *Main) menuGridLine(a *App, line int) string {
	width := a.WindowWidth()
	columns := m.gridColumns
	left := m.menuStartColumn
	right := gridMax(left, width-left)
	gap := gridMin(4, gridMax(1, (right-left)/gridMax(1, columns*12)))
	cellWidth := gridMax(1, (right-left-gap*(columns-1))/columns)
	var b strings.Builder
	cursor := 0
	pageStart := m.getPageStartIndex()
	for column := 0; column < columns; column++ {
		index := pageStart + line*columns + column
		if index >= len(m.menuList) || index >= pageStart+m.menuPageSize {
			break
		}
		start := left + column*(cellWidth+gap)
		end := gridMin(width, start+cellWidth)
		if column == columns-1 {
			end = gridMin(width, right)
		}
		item := m.menuList[index]
		text := fmt.Sprintf("  %*d. %s", m.getMaxIndexWidth(), index, item.OriginString())
		if m.isSelected(index) {
			text = fmt.Sprintf("› %*d. %s", m.getMaxIndexWidth(), index, item.OriginString())
		}
		text = truncate.StringWithTail(text, uint(gridMax(1, end-start)), "…")
		length := ansi.PrintableRuneWidth(text)
		x := start
		x = gridMax(cursor, x)
		b.WriteString(strings.Repeat(" ", gridMax(0, x-cursor)))
		if m.isSelected(index) {
			b.WriteString(util.SetFgStyle(text, util.GetPrimaryColor()))
		} else {
			b.WriteString(util.SetNormalStyle(text))
		}
		m.gridHits = append(m.gridHits, menuGridHit{index, x, m.menuStartRow + line - 1, length})
		cursor = x + length
	}
	b.WriteString(strings.Repeat(" ", gridMax(0, width-cursor)))
	return b.String()
}

// MenuIndexAt uses the same rendered bounds as the responsive menu grid.
func (m *Main) MenuIndexAt(x, y int) (int, bool) {
	for _, hit := range m.gridHits {
		if hit.y == y && x >= hit.x && x < hit.x+hit.width {
			return hit.index, true
		}
	}
	return 0, false
}

func (m *Main) moveGridVertical(delta int) Page {
	target := m.selectedIndex + delta*m.gridColumns
	var next Page
	if target < 0 {
		if hook := m.menu.TopOutHook(); hook != nil {
			loading := NewLoading(m)
			loading.Start()
			ok, page := hook(m)
			loading.Complete()
			if !ok {
				return page
			}
			m.menuList = m.menu.MenuViews()
		}
	} else if target >= len(m.menuList) {
		if hook := m.menu.BottomOutHook(); hook != nil {
			loading := NewLoading(m)
			loading.Start()
			ok, page := hook(m)
			loading.Complete()
			if !ok {
				return page
			}
			m.menuList = m.menu.MenuViews()
		}
	}
	if target < 0 || target >= len(m.menuList) {
		return next
	}
	m.selectedIndex = target
	if target < m.getPageStartIndex() {
		next = m.PrePage()
	} else if target >= m.menuCurPage*m.menuPageSize {
		next = m.NextPage()
	}
	return next
}
func (m *Main) moveGridHorizontal(delta int) Page {
	column := (m.selectedIndex - m.getPageStartIndex()) % m.gridColumns
	if column+delta < 0 || column+delta >= m.gridColumns {
		return nil
	}
	target := m.selectedIndex + delta
	if delta > 0 && target >= len(m.menuList) {
		if hook := m.menu.BottomOutHook(); hook != nil {
			loading := NewLoading(m)
			loading.Start()
			ok, page := hook(m)
			loading.Complete()
			if !ok {
				return page
			}
			m.menuList = m.menu.MenuViews()
		}
	}
	if target < 0 || target >= len(m.menuList) {
		return nil
	}
	m.selectedIndex = target
	return nil
}

func gridMin(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func gridMax(a, b int) int {
	if a > b {
		return a
	}
	return b
}
