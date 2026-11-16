package ui

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-musicfox/go-musicfox/internal/configs"
	"github.com/go-musicfox/go-musicfox/internal/player"
	"github.com/go-musicfox/go-musicfox/internal/types"
)

type tickerByPlayer struct {
	c                    chan time.Time
	p                    *Player
	reactive             bool
	enabled              atomic.Bool
	changed              chan struct{}
	stop                 chan struct{}
	startOnce, closeOnce sync.Once
}

func newTickerByPlayer(p *Player) *tickerByPlayer {
	_, supported := p.Player.(player.SpectrumProvider)
	t := &tickerByPlayer{c: make(chan time.Time, 1), p: p, reactive: supported && configs.AppConfig.Main.Lyric.Layout == "adaptive", changed: make(chan struct{}, 1), stop: make(chan struct{})}
	t.enabled.Store(configs.AppConfig.SpectrumEnabled())
	return t
}
func (t *tickerByPlayer) IsReactive() bool { return t.reactive && t.enabled.Load() }
func (t *tickerByPlayer) SetEnabled(enabled bool) {
	t.enabled.Store(enabled)
	select {
	case t.changed <- struct{}{}:
	default:
	}
}
func (t *tickerByPlayer) Start() error {
	if t.reactive {
		t.startOnce.Do(func() {
			go func() {
				var ticker *time.Ticker
				var ticks <-chan time.Time
				defer func() {
					if ticker != nil {
						ticker.Stop()
					}
				}()
				update := func() {
					if t.enabled.Load() && ticker == nil {
						ticker = time.NewTicker(100 * time.Millisecond)
						ticks = ticker.C
					}
					if !t.enabled.Load() && ticker != nil {
						ticker.Stop()
						ticker = nil
						ticks = nil
					}
				}
				update()
				for {
					select {
					case <-t.stop:
						return
					case <-t.changed:
						update()
					case now := <-ticks:
						if t.IsReactive() && t.p.State() == types.Playing {
							select {
							case t.c <- now:
							default:
							}
						}
					}
				}
			}()
		})
	}
	return nil
}
func (t *tickerByPlayer) Ticker() <-chan time.Time  { return t.c }
func (t *tickerByPlayer) PassedTime() time.Duration { return t.p.PassedTime() }
func (t *tickerByPlayer) Close() error              { t.closeOnce.Do(func() { close(t.stop) }); return nil }
