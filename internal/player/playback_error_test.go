package player

import (
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"netease-music-mygo/internal/netease"
)

// A FLAC cut inside its last frame must not look like a successfully finished song.
func TestTruncatedFLACReportsFailure(t *testing.T) {
	b, err := os.ReadFile("testdata/seek.flac")
	if err != nil {
		t.Fatal(err)
	}
	s, err := openFLACStream(downloaded(b[:len(b)-2000]))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	// Metadata is intact but no audio frame arrived at all.
	empty, err := openFLACStream(downloaded(b[:s.start]))
	if err != nil {
		t.Fatal(err)
	}
	defer empty.Close()
	if _, err := empty.Read(make([]float32, 4096)); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("missing frames reported as %v", err)
	}
	buf := make([]float32, 4096)
	for {
		_, err := s.Read(buf)
		if err == nil {
			continue
		}
		if errors.Is(err, io.EOF) {
			t.Fatal("truncated FLAC reported normal EOF")
		}
		return
	}
}

type failedStream struct {
	stream
	err error
}

func (s failedStream) Read([]float32) (int, error) { return 0, s.err }
func (s failedStream) SetPosition(int64) error     { return s.err }

// A decode failure must stop on its track, keep its reason and reject obsolete callbacks.
func TestPlaybackFailureStopsQueue(t *testing.T) {
	posted := make(chan func(), 4)
	p := &Player{post: func(fn func()) { posted <- fn }, gen: 7, playing: true, at: 0,
		queue: []netease.Track{{ID: 1}, {ID: 2}}, order: []int{0, 1}}
	p.track = &p.queue[0]
	errRead := errors.New("connection lost")
	s := p.watchStream(failedStream{err: errRead}, p.gen)
	s.Read(nil)
	s.Read(nil)
	(<-posted)()
	if p.Playing() || p.Ready() || p.Current().ID != 1 || p.Err() == "" || p.gen != 8 {
		t.Fatal("failure skipped track or lost its reason")
	}
	select {
	case <-posted:
		t.Fatal("failure posted more than once")
	default:
	}
	before := p.Err()
	p.playbackFailed(7, errors.New("obsolete"))
	if p.Err() != before {
		t.Fatal("obsolete failure replaced current state")
	}
}

// A normal EOF must remain normal, while a failed seek must be reported.
func TestPlaybackEOFAndSeek(t *testing.T) {
	posted := make(chan func(), 2)
	p := &Player{post: func(fn func()) { posted <- fn }}
	p.watchStream(failedStream{err: io.EOF}, 0).Read(nil)
	select {
	case <-posted:
		t.Fatal("normal EOF reported as failure")
	default:
	}
	p.watchStream(failedStream{err: errors.New("seek failed")}, 0).SetPosition(123)
	(<-posted)()
	if p.Err() == "" {
		t.Fatal("seek failure disappeared")
	}
}

// Automatic continuation skips unavailable tracks only within the queue's repeat policy.
func TestLoadFailurePolicy(t *testing.T) {
	for _, tc := range []struct {
		name      string
		automatic bool
		at        int
		repeat    RepeatMode
		err       error
		want      int
	}{
		{"manual or resume", false, 0, RepeatOff, netease.ErrUnavailable, -1},
		{"automatic unavailable", true, 0, RepeatOff, netease.ErrUnavailable, 1},
		{"end without repeat", true, 1, RepeatOff, netease.ErrUnavailable, -1},
		{"repeat queue", true, 1, RepeatAll, netease.ErrUnavailable, 0},
		{"request failure", true, 0, RepeatAll, errors.New("offline"), -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := Player{order: []int{0, 1}, repeat: tc.repeat}
			if got := p.nextAfterLoadFailure(tc.at, 0, tc.automatic, tc.err); got != tc.want {
				t.Fatalf("next = %d, want %d", got, tc.want)
			}
		})
	}
}

type playbackTransport func(*http.Request) (*http.Response, error)

func (f playbackTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// A rejected manual skip keeps the selected song and waits for Play before retrying.
func TestNextRetriesRejectedTrackOnlyOnPlay(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
	var requests atomic.Int32
	http.DefaultTransport = playbackTransport(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/eapi/song/enhance/player/url/v1":
			requests.Add(1)
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"code":-460,"message":"network risk"}`)),
				Request:    req,
			}, nil
		case "/api/song/lyric":
			return nil, errors.New("lyrics unavailable in playback test")
		case "/cover":
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("")),
				Request:    req,
			}, nil
		default:
			t.Errorf("unexpected request: %s", req.URL.Path)
			return nil, errors.New("unexpected request")
		}
	})
	posted := make(chan func(), 4)
	p := Player{
		post:  func(fn func()) { posted <- fn },
		queue: []netease.Track{{ID: 1}, {ID: 2}, {ID: 3}},
		order: []int{0, 1, 2},
	}
	p.track = &p.queue[0]
	p.queue[1].Album.PicURL = "https://audio.example/cover"
	defer p.stop()

	for i, action := range []func(){p.Next, p.TogglePlay} {
		action()
		// Drain the cover, lyric and playback callbacks before the next action.
		timeout := time.After(3 * time.Second)
		for range 3 {
			select {
			case fn := <-posted:
				fn()
			case <-timeout:
				t.Fatal("track load did not complete")
			}
		}
		if p.Current().ID != 2 || p.Playing() || p.loading || !strings.Contains(p.Err(), "-460") {
			t.Fatalf("attempt %d: track=%d playing=%v loading=%v error=%q",
				i+1, p.Current().ID, p.Playing(), p.loading, p.Err())
		}
		if got := requests.Load(); got != int32(i+1) {
			t.Fatalf("attempt %d: got %d total song URL requests", i+1, got)
		}
	}
}

// An automatic continuation also stops when the service rejects the request.
func TestSongURLRejection(t *testing.T) {
	err := &netease.SongURLError{Code: -460, Message: "network risk"}
	p := Player{order: []int{0, 1}}
	if next := p.nextAfterLoadFailure(0, 0, true, err); next != -1 {
		t.Fatalf("rejection advanced to index %d", next)
	}
}
