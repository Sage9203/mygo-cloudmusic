package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"netease-music-mygo/internal/netease"
)

// The spinning record must request its own repaints: a moving backdrop does
// not repaint its sibling, so otherwise the label jumps only on view rebuilds.
func TestDailyRecordRequestsFrames(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.dailySpin.speed = 1
	songs := []netease.Track{{ID: 1}, {ID: 2}, {ID: 3}}
	tt := ui.NewTester(func(c *ui.Context) {
		a.dailyStack(c, songs, false, 1)
	}, 1000, 400)
	tt.Frame()
	tt.Advance(2 * time.Second)
	if !tt.FramePending() {
		t.Fatal("spinning record stopped repainting after its layout settled")
	}
	a.dailySpin.speed = 0
	tt.Frame()
	tt.Advance(2 * time.Second)
	if tt.FramePending() {
		t.Fatal("stopped record keeps requesting animation frames")
	}
}
