package netease

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type songURLTransport func(*http.Request) (*http.Response, error)

func (f songURLTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// SongURL used to hide API errors such as -460 behind a misleading unavailable-song error.
func TestSongURLPreservesAPIErrors(t *testing.T) {
	for _, tc := range []struct {
		name, body, want string
		unavailable      bool
	}{
		{"risk", "{\"code\":-460,\"message\":\"network risk\"}", "code -460 network risk", false},
		{"msg fallback", "{\"code\":301,\"msg\":\"login required\"}", "code 301 login required", false},
		{"unavailable", "{\"code\":200,\"data\":[{\"url\":null}]}", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			previous := http.DefaultTransport
			defer func() { http.DefaultTransport = previous }()
			http.DefaultTransport = songURLTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path != "/eapi/song/enhance/player/url/v1" || r.Method != "POST" {
					t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(tc.body)), Request: r}, nil
			})
			deviceOnce.Do(func() { deviceID = "0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123" })
			stream, err := SongURL(context.Background(), 2652820720, "exhigh")
			if stream != nil || err == nil {
				t.Fatalf("stream=%v err=%v", stream, err)
			}
			if tc.unavailable {
				if !errors.Is(err, ErrUnavailable) {
					t.Fatalf("want unavailable, got %v", err)
				}
			} else if errors.Is(err, ErrUnavailable) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("lost API cause: %v", err)
			}
		})
	}
}

// A preview must be distinguishable from a missing URL and never enter full-track playback.
func TestSongURLTrialOnly(t *testing.T) {
	previous := http.DefaultTransport
	defer func() { http.DefaultTransport = previous }()
	http.DefaultTransport = songURLTransport(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"code":200,"data":[{"url":"https://audio.example/a","freeTrialInfo":{"start":0,"end":30}}]}`)), Request: req}, nil
	})
	stream, err := SongURL(context.Background(), 1, "exhigh")
	if stream != nil || !errors.Is(err, ErrTrialOnly) {
		t.Fatalf("stream = %v, error = %v", stream, err)
	}
}

// A requested lossless level must not turn unknown or downgraded metadata into a lossless cache entry.
func TestSongURLDoesNotInventQuality(t *testing.T) {
	for _, tc := range []struct{ body, want string }{
		{`{"code":200,"data":[{"url":"https://audio.example/a","type":"mp3"}]}`, ""},
		{`{"code":200,"data":[{"url":"https://audio.example/a","type":"mp3","level":"hires"}]}`, ""},
		{`{"code":200,"data":[{"url":"https://audio.example/a","type":"mp3","level":"exhigh"}]}`, "exhigh"},
		{`{"code":200,"data":[{"url":"https://audio.example/a","type":"flac","level":"hires"}]}`, "hires"},
	} {
		previous := http.DefaultTransport
		http.DefaultTransport = songURLTransport(func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(tc.body)), Request: req}, nil
		})
		st, err := SongURL(context.Background(), 1, "hires")
		http.DefaultTransport = previous
		if err != nil {
			t.Fatal(err)
		}
		if st.Level != tc.want {
			t.Fatalf("got quality %q, want %q", st.Level, tc.want)
		}
	}
}
