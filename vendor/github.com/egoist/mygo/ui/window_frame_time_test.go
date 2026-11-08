package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/internal/platform"
	"github.com/egoist/mygo/internal/surface"
)

type frameTimeSurface struct {
	platform.Surface
	draw func()
}

func (s *frameTimeSurface) FrameNow() { s.draw() }

type immediateFrameTimeSurface struct{ frameTimeSurface }

func (*immediateFrameTimeSurface) FramesAtOnce() {}

// A Windows frame deferred for GPU room must sample animation at drawing time,
// while a refresh-driven surface keeps the time supplied by its refresh.
func TestDeferredFrameTime(t *testing.T) {
	for _, immediate := range []bool{false, true} {
		name := "refresh"
		if immediate {
			name = "immediate"
		}
		t.Run(name, func(t *testing.T) {
			requested := time.Unix(100, 0)
			now := requested
			rt := &engine{clock: func() time.Time { return now }}
			var drawn time.Time
			s := frameTimeSurface{draw: func() { drawn = rt.frameTime() }}
			var target platform.Surface = &s
			if immediate {
				target = &immediateFrameTimeSurface{s}
			}
			waiter := &refreshWaiter{waiting: true}
			h := &windowHost{rt: rt, conn: &surface.Conn{Surface: target, Post: func(fn func()) { fn() }}, refresh: waiter}
			if !h.waitDisplay(requested) {
				t.Fatal("frame did not wait")
			}
			now = requested.Add(12 * time.Millisecond)
			waiter.wake()
			want := requested
			if immediate {
				want = now
			}
			if !drawn.Equal(want) {
				t.Fatalf("animation sampled %v; want %v", drawn, want)
			}
		})
	}
}
