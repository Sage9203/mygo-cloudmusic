package player

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"netease-music-mygo/internal/netease"
)

// A lookahead stops at its byte budget and resumes without losing or repeating bytes.
func TestPreloadPrefixAndResume(t *testing.T) {
	data := bytes.Repeat([]byte("0123456789abcdef"), 32768)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(data) }))
	defer srv.Close()
	d := startDownloadPaused(context.Background(), srv.URL, int64(len(data)), nil, nil, preloadBytes)
	defer d.Close()
	if n := d.waitFor(preloadBytes); n != preloadBytes {
		t.Fatalf("prefix contains %d bytes", n)
	}
	d.resumeDownload()
	if n := d.waitFor(int64(len(data))); n != int64(len(data)) {
		t.Fatalf("resumed length %d", n)
	}
	if got := d.peek(0, len(data)); !bytes.Equal(got, data) {
		t.Fatal("resuming changed audio bytes")
	}
}

// A cached next track is prepared without querying an online stream and transfers ownership on selection.
func TestPreloadUsesCachedNext(t *testing.T) {
	data, err := os.ReadFile("testdata/seek.mp3")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	meta, _ := json.Marshal(netease.Stream{Type: "mp3", Level: "exhigh", Size: int64(len(data))})
	if err := os.WriteFile(filepath.Join(dir, "7-exhigh.mp3"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "7-exhigh.json"), meta, 0600); err != nil {
		t.Fatal(err)
	}
	s, err := openStream(downloaded(data), 5000)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	q := make(chan func(), 4)
	p := &Player{post: func(fn func()) { q <- fn }, Cache: NewCache(dir, 1<<20), queue: []netease.Track{{ID: 1}, {ID: 7}}, order: []int{0, 1}, at: 0, gen: 4}
	defer p.stop()
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
	p.queue[1].Album.PicURL = "https://cover.example/cached-next"
	http.DefaultTransport = playbackTransport(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/cached-next":
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(""))}, nil
		case "/api/song/lyric":
			return nil, errors.New("lyrics unavailable in cache test")
		default:
			t.Errorf("unexpected request for cached track: %s", req.URL.Path)
			return nil, errors.New("unexpected request")
		}
	})
	p.planPreload(context.Background(), 4, s, "exhigh")
	select {
	case fn := <-q:
		fn()
	case <-time.After(2 * time.Second):
		t.Fatal("cached next track not prepared")
	}
	prepared := p.preload
	if prepared == nil || prepared.dl == nil || prepared.dl.file == nil {
		t.Fatal("prepared track not reusable")
	}
	p.Next()
	// Drain the cover, lyrics and playback callbacks.
	for range 3 {
		next(t, q)
	}
	if !p.Ready() || !p.Playing() || p.Current().ID != 7 {
		t.Fatalf("cached next track did not start: %s", p.Err())
	}
	out := p.out
	defer out.Close()
	if p.preload != nil || prepared.ctx.Err() != nil {
		t.Fatal("switching cancelled the selected lookahead")
	}
	p.stop()
	if prepared.ctx.Err() != context.Canceled {
		t.Fatal("selected lookahead did not become the active load")
	}
}

// Discarding a parked lookahead cancels it and rejects a mismatching or expired candidate.
func TestPreloadCancelAndIdentity(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	d := downloaded([]byte("prefix"))
	q := &preloaded{id: 7, level: "lossless", ctx: ctx, cancel: cancel, dl: d, at: time.Now()}
	p := &Player{preload: q}
	if p.takePreload(8, "lossless") != nil || p.takePreload(7, "exhigh") != nil {
		t.Fatal("reused wrong track or quality")
	}
	q.at = time.Now().Add(-6 * time.Minute)
	if p.takePreload(7, "lossless") != nil {
		t.Fatal("reused expired stream URL")
	}
	p.CancelPreload()
	if ctx.Err() != context.Canceled || p.preload != nil {
		t.Fatal("lookahead not cancelled")
	}
}

// Shuffling a list may start anywhere, but a deliberately selected track stays first.
func TestShuffleFirstSelection(t *testing.T) {
	p := &Player{order: []int{0, 1, 2, 3}}
	seen := map[int]bool{}
	for range 128 {
		p.shuffleOrder(-1)
		seen[p.order[0]] = true
	}
	if len(seen) < 2 {
		t.Fatal("shuffle always starts at the same song")
	}
	p.shuffleOrder(2)
	if p.order[0] != 2 {
		t.Fatal("selected song did not stay first")
	}
}
