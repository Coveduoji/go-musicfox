//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package kitty

import (
	"os"

	"golang.org/x/sys/unix"
)

// CellSize reads terminal geometry without consuming Bubble Tea's input stream.
func CellSize() (width, height float64) {
	ws, err := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ)
	if err != nil || ws.Col == 0 || ws.Row == 0 || ws.Xpixel == 0 || ws.Ypixel == 0 {
		return 0, 0
	}
	return float64(ws.Xpixel) / float64(ws.Col), float64(ws.Ypixel) / float64(ws.Row)
}
