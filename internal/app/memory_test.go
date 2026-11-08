package app

import (
	"runtime"
	"testing"
	"time"
	"weak"

	"github.com/egoist/mygo/ui"
	"netease-music-mygo/internal/netease"
)

// Once Now Playing finishes closing, its large background canvas must be collectible.
func TestClosedNowPlayingReleasesBackground(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.pl.Cue([]netease.Track{{ID: 1, Name: "test", Duration: 200000}})
	a.pl.SetLyrics(nil)
	tt := ui.NewTester(a.view, 900, 600)
	a.nowPlaying = true
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	tt.Image()
	if a.backdropCache.canvas == nil {
		t.Fatal("background not created")
	}
	background := weak.Make(a.backdropCache.canvas)
	a.nowPlaying = false
	tt.Frame()
	tt.Advance(2 * time.Second)
	tt.Frame()
	tt.Image()
	if a.backdropCache.canvas != nil || a.backdropCache.before != nil {
		t.Fatal("closed view retains background")
	}
	runtime.GC()
	if background.Value() != nil {
		t.Fatal("closed background is still reachable")
	}
	runtime.KeepAlive(a)
	runtime.KeepAlive(tt)
}
