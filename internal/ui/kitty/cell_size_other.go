//go:build !linux && !darwin && !freebsd && !openbsd && !netbsd && !dragonfly

package kitty

// CellSize is optional; Kitty can preserve the source aspect ratio without it.
func CellSize() (width, height float64) { return 0, 0 }
