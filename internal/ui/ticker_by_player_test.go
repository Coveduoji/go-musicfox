package ui

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-musicfox/go-musicfox/internal/configs"
	"github.com/go-musicfox/go-musicfox/internal/player"
	"github.com/go-musicfox/go-musicfox/internal/types"
)

type spectrumTickerEngine struct {
	player.Player
	state atomic.Int32
}

func (e *spectrumTickerEngine) State() types.State { return types.State(e.state.Load()) }
func (e *spectrumTickerEngine) Spectrum() player.SpectrumFrame {
	return player.SpectrumFrame{Supported: true}
}

func TestSpectrumTickerRunsOnlyWhilePlaying(t *testing.T) {
	previous := configs.AppConfig
	configs.AppConfig = &configs.Config{}
	defer func() { configs.AppConfig = previous }()
	configs.AppConfig.Main.Lyric.Layout = "adaptive"
	configs.AppConfig.Main.Visualizer.Enable = true
	engine := &spectrumTickerEngine{}
	engine.state.Store(int32(types.Paused))
	ticker := newTickerByPlayer(&Player{Player: engine})
	defer ticker.Close()
	if !ticker.reactive {
		t.Fatal("PCM engine did not enable spectrum refresh")
	}
	if err := ticker.Start(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ticker.c:
		t.Fatal("paused player ticked")
	case <-time.After(150 * time.Millisecond):
	}
	engine.state.Store(int32(types.Playing))
	select {
	case <-ticker.c:
	case <-time.After(time.Second):
		t.Fatal("playing player did not tick")
	}
	ticker.SetEnabled(false)
	select {
	case <-ticker.c:
	default:
	}
	select {
	case <-ticker.c:
		t.Fatal("disabled spectrum kept refreshing")
	case <-time.After(150 * time.Millisecond):
	}
	ticker.SetEnabled(true)
	select {
	case <-ticker.c:
	case <-time.After(time.Second):
		t.Fatal("runtime enable did not restart refresh")
	}
	engine.state.Store(int32(types.Paused))
	// Drain a tick that may have been queued before the pause.
	select {
	case <-ticker.c:
	default:
	}
	select {
	case <-ticker.c:
		t.Fatal("paused player kept ticking")
	case <-time.After(150 * time.Millisecond):
	}
	if err := ticker.Close(); err != nil {
		t.Fatal(err)
	}
	if err := ticker.Close(); err != nil {
		t.Fatal(err)
	}
	configs.AppConfig.Main.Visualizer.Enable = false
	disabled := newTickerByPlayer(&Player{Player: engine})
	defer disabled.Close()
	if disabled.IsReactive() {
		t.Fatal("disabled spectrum enabled extra timer")
	}
	configs.AppConfig.Main.Visualizer.Enable = true
	configs.AppConfig.Main.Lyric.Layout = "classic"
	classic := newTickerByPlayer(&Player{Player: engine})
	defer classic.Close()
	if classic.reactive {
		t.Fatal("classic layout enabled additional timer")
	}
}
