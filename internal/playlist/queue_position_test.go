package playlist

import (
	"sync"
	"testing"

	"github.com/go-musicfox/go-musicfox/internal/structs"
)

func TestGetQueuePosition(t *testing.T) {
	manager := NewPlaylistManager()
	check := func(wantIndex, wantTotal int) {
		t.Helper()
		if index, total := manager.GetQueuePosition(); index != wantIndex || total != wantTotal {
			t.Fatalf("queue position = (%d, %d), want (%d, %d)", index, total, wantIndex, wantTotal)
		}
	}
	check(-1, 0)
	if err := manager.Initialize(1, []structs.Song{{Id: 1}, {Id: 2}, {Id: 3}}); err != nil {
		t.Fatal(err)
	}
	check(1, 3)
	if _, err := manager.RemoveSong(0); err != nil {
		t.Fatal(err)
	}
	check(0, 2)
	if err := manager.Initialize(0, nil); err != nil {
		t.Fatal(err)
	}
	check(-1, 0)
}

func TestGetQueuePositionConcurrentReplacement(t *testing.T) {
	manager := NewPlaylistManager()
	queues := [][]structs.Song{{{Id: 1}}, {{Id: 1}, {Id: 2}, {Id: 3}}}
	if err := manager.Initialize(0, queues[0]); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Go(func() {
		for i := range 1000 {
			queue := queues[i%len(queues)]
			if err := manager.Initialize(len(queue)-1, queue); err != nil {
				t.Error(err)
				return
			}
		}
	})
	for range 4 {
		wg.Go(func() {
			for range 1000 {
				index, total := manager.GetQueuePosition()
				if (index != 0 || total != 1) && (index != 2 || total != 3) {
					t.Errorf("inconsistent queue position = (%d, %d)", index, total)
					return
				}
			}
		})
	}
	wg.Wait()
}
