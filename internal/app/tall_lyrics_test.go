package app

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"netease-music-mygo/internal/lyrics"
	"netease-music-mygo/internal/netease"
	"testing"
	"time"
)

// A tall window must keep the sung line in the upper part of the available lyric pane.
func TestTallLyricsPosition(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.pl.Cue([]netease.Track{{ID: 7, Duration: 150000}})
	var lines []lyrics.Line
	for i := range 30 {
		at := time.Duration(i) * 5 * time.Second
		lines = append(lines, lyrics.Line{At: at, End: at + 5*time.Second, Text: fmt.Sprintf("测试歌词%02d", i)})
	}
	a.pl.SetLyrics(lines)
	a.pl.Hold(77 * time.Second)
	a.nowPlaying = true
	tt := ui.NewTester(a.view, 750, 1330)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	active, ok := tt.Find("测试歌词15")
	if !ok {
		t.Fatal("active lyric missing")
	}
	if active.Y > 600 {
		t.Fatalf("active lyric sank to %v in a tall window", active.Y)
	}
}
