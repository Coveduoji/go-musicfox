package model

import (
	tea "github.com/charmbracelet/bubbletea"
)

type Component interface {
	Update(msg tea.Msg, a *App)
	View(a *App, main *Main) (view string, lines int)
}

// BottomComponent is anchored below the body using its actual rendered height.
type BottomComponent interface {
	Component
	ViewBottom(*App, *Main) string
}

// FlexibleComponent receives the viewport remaining above fixed bottom components.
type FlexibleComponent interface {
	Component
	ViewWithSize(*App, *Main, int, int) string
}

// HidingComponent releases visible resources when Main yields to another page.
type HidingComponent interface {
	Component
	Hide()
}

// InvalidatingComponent restores external resources after a full-screen repaint.
type InvalidatingComponent interface {
	Component
	Invalidate()
}
