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

// A URL issued for a logged-out account must not reach playback or preload.
func TestSongURLRejectsResponseAfterLogout(t *testing.T) {
	client := isolatedSessionTest(t)
	defaultSession.credentials.read = func() ([]byte, error) { return []byte("MUSIC_U=old"), nil }
	if !RestoreSession() {
		t.Fatal("could not restore test session")
	}
	client.Transport = songURLTransport(func(req *http.Request) (*http.Response, error) {
		if cookie, err := req.Cookie("MUSIC_U"); err != nil || cookie.Value != "old" {
			t.Fatal("request did not use the old session")
		}
		// Logout clears the active account synchronously while this request is in flight.
		_ = BeginLogout()
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"code":200,"data":[{"url":"https://audio.example/old","type":"mp3","level":"exhigh"}]}`)),
			Request:    req,
		}, nil
	})
	stream, err := SongURL(context.Background(), 2, "exhigh")
	if stream != nil || !errors.Is(err, ErrSessionChanged) {
		t.Fatalf("got stream=%v, err=%v; want no stream and ErrSessionChanged", stream, err)
	}
}

// A signed-in listener can play a full stream from the web endpoint after the desktop endpoint refuses it.
func TestSongURLWebFallback(t *testing.T) {
	for _, tc := range []struct {
		name, first, web string
		wantCalls        int
		wantErr          error
	}{
		{"full stream", `{"code":-460}`, `{"code":200,"data":[{"url":"https://audio.example/a","type":"mp3","level":"standard"}]}`, 2, nil},
		{"trial", `{"code":-460}`, `{"code":200,"data":[{"url":"https://audio.example/preview","freeTrialInfo":{}}]}`, 2, ErrTrialOnly},
		{"authentication required", `{"code":301}`, "", 1, &SongURLError{}},
		{"rate limited", `{"code":429}`, "", 1, &SongURLError{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := isolatedSessionTest(t)
			defaultSession.credentials.read = func() ([]byte, error) { return []byte("MUSIC_U=test-session"), nil }
			if !RestoreSession() {
				t.Fatal("could not restore test session")
			}
			calls := 0
			client.Transport = songURLTransport(func(req *http.Request) (*http.Response, error) {
				calls++
				body := tc.first
				if calls == 2 {
					body = tc.web
					if req.URL.Host != "music.163.com" || req.URL.Path != "/weapi/song/enhance/player/url/v1" {
						t.Fatalf("unexpected fallback endpoint: %s", req.URL)
					}
					if cookie, err := req.Cookie("MUSIC_U"); err != nil || cookie.Value != "test-session" {
						t.Fatal("fallback did not use the active session")
					}
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
			})
			stream, err := SongURL(context.Background(), 1931658532, "exhigh")
			if calls != tc.wantCalls {
				t.Fatalf("got %d calls, want %d, error = %v", calls, tc.wantCalls, err)
			}
			var apiErr *SongURLError
			switch tc.wantErr.(type) {
			case *SongURLError:
				if stream != nil || !errors.As(err, &apiErr) {
					t.Fatalf("got stream=%v, error=%v; want API error", stream, err)
				}
			default:
				if !errors.Is(err, tc.wantErr) || (tc.wantErr == nil && (stream == nil || stream.Level != "standard")) {
					t.Fatalf("got stream=%v, error=%v", stream, err)
				}
			}
		})
	}
}
