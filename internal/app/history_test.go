package app

import (
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// The liked-songs shortcut opens the existing playlist and Back restores its visible rows.
func TestLikedPlaylistReturnsToVisibleRows(t *testing.T) {
	a := newTestApp()
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if tt.HasText("我喜欢的音乐") {
		t.Fatal("liked songs shown before sign-in")
	}
	a.profile = &netease.Profile{UserID: 1}
	a.accountAt = time.Now()
	a.mine = []netease.Playlist{{ID: 42, Name: "收藏曲目", SpecialType: 5}}
	tracks := make([]netease.Track, 100)
	for i := range tracks {
		tracks[i] = netease.Track{ID: int64(i + 1), Name: fmt.Sprintf("曲目%d", i+1),
			Album: netease.Album{ID: 43, Name: fmt.Sprintf("专辑%d", i+1)}}
	}
	a.playlists[42] = &netease.Playlist{ID: 42, Name: "收藏曲目", Tracks: tracks}
	a.playlistAt = map[int64]time.Time{42: time.Now()}
	a.albums[43] = &albumData{album: &netease.AlbumProfile{}}
	tt.Frame()
	if err := tt.Click("我喜欢的音乐"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.route != (route{page: pagePlaylist, id: 42}) {
		t.Fatalf("shortcut opened %+v", a.route)
	}
	tt.Wheel(700, 400, 0, 2400)
	tt.Advance(time.Second)
	tt.Frame()
	before, _ := a.trackList.Visible()
	if before < 2 {
		t.Fatal("playlist did not scroll")
	}
	if err := tt.Click(fmt.Sprintf("专辑%d", before+3)); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.route != (route{page: pageAlbum, id: 43}) {
		t.Fatalf("album link opened %+v", a.route)
	}
	tt.Key(0, ui.KeyBack)
	tt.Frame()
	first, last := a.trackList.Visible()
	if first == 0 || before+2 < first || before+2 > last {
		t.Fatalf("returned to rows %d–%d, want the original album row %d in view", first, last, before+2)
	}
}

// The side buttons of a mouse go back and forward between pages.
func TestBackAndForward(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.playlists[42] = &netease.Playlist{ID: 42, Name: "飙升榜", Tracks: []netease.Track{{ID: 1, Name: "一张相片"}}}
	tt := ui.NewTester(a.view, 1200, 800)
	charts, list := route{page: pageCharts}, route{page: pagePlaylist, id: 42}
	a.open(charts)
	a.open(list)
	tt.Frame()

	tt.Key(0, ui.KeyBack)
	tt.Frame()
	if a.route != charts {
		t.Fatalf("back went to %+v", a.route)
	}
	tt.Key(0, ui.KeyForward)
	tt.Frame()
	if a.route != list {
		t.Fatalf("forward went to %+v", a.route)
	}

	// Back closes Now Playing before it leaves the page under it.
	a.pl.Cue([]netease.Track{{ID: 1, Name: "一张相片"}})
	a.nowPlaying = true
	tt.Frame()
	tt.Key(0, ui.KeyBack)
	tt.Frame()
	if a.nowPlaying || a.route != list {
		t.Fatalf("back with Now Playing open: open %v, page %+v", a.nowPlaying, a.route)
	}

	// A new page leaves nothing to go forward to.
	tt.Key(0, ui.KeyBack)
	tt.Frame()
	a.open(route{page: pageHome})
	tt.Key(0, ui.KeyForward)
	tt.Frame()
	if a.route != (route{page: pageHome}) {
		t.Fatalf("forward after a new page went to %+v", a.route)
	}
}
