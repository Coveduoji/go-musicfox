package ui

import (
	"strconv"
	"testing"

	"github.com/go-musicfox/go-musicfox/internal/playlist"
	"github.com/go-musicfox/go-musicfox/internal/structs"
)

func TestCurSongFollowsQueueChanges(t *testing.T) {
	p := &Player{playlistManager: playlist.NewPlaylistManager()}
	if got := p.CurSong(); got.Id != 0 {
		t.Fatalf("empty queue: song ID = %d, want 0", got.Id)
	}
	if err := p.playlistManager.Initialize(1, []structs.Song{{Id: 1}, {Id: 2}, {Id: 3}}); err != nil {
		t.Fatal(err)
	}
	if got := p.CurSong(); got.Id != 2 {
		t.Fatalf("initialized queue: song ID = %d, want 2", got.Id)
	}
	if _, err := p.playlistManager.RemoveSong(1); err != nil {
		t.Fatal(err)
	}
	if got := p.CurSong(); got.Id != 3 {
		t.Fatalf("removed current song: song ID = %d, want 3", got.Id)
	}
	if err := p.playlistManager.Initialize(0, nil); err != nil {
		t.Fatal(err)
	}
	if got := p.CurSong(); got.Id != 0 {
		t.Fatalf("cleared queue: song ID = %d, want 0", got.Id)
	}
}

func TestCurSongDoesNotAllocate(t *testing.T) {
	p := &Player{playlistManager: playlist.NewPlaylistManager()}
	songs := make([]structs.Song, 1000)
	songs[0].Id = 1
	if err := p.playlistManager.Initialize(0, songs); err != nil {
		t.Fatal(err)
	}
	if allocs := testing.AllocsPerRun(100, func() { benchmarkCurrentSong = p.CurSong() }); allocs != 0 {
		t.Fatalf("CurSong allocations = %g, want 0", allocs)
	}
}

var benchmarkCurrentSong structs.Song

func BenchmarkPlayerCurSong(b *testing.B) {
	for _, count := range []int{100, 1000, 10000} {
		b.Run(strconv.Itoa(count), func(b *testing.B) {
			p := &Player{playlistManager: playlist.NewPlaylistManager()}
			if err := p.playlistManager.Initialize(0, make([]structs.Song, count)); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchmarkCurrentSong = p.CurSong()
			}
		})
	}
}
