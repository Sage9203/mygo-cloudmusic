package netease

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

// A restored account must reach both account lookup and the first audio URL request.
func TestRestoredSessionReachesSongURL(t *testing.T) {
	client := isolatedSessionTest(t)
	defaultSession.credentials.read = func() ([]byte, error) {
		return []byte("MUSIC_U=test-session; __csrf=test-csrf"), nil
	}
	client.Transport = songURLTransport(func(req *http.Request) (*http.Response, error) {
		for _, name := range []string{"MUSIC_U", "__csrf"} {
			want := "test-session"
			if name == "__csrf" {
				want = "test-csrf"
			}
			if cookie, err := req.Cookie(name); err != nil || cookie.Value != want {
				t.Errorf("%s did not receive restored %s", req.URL.Path, name)
			}
		}
		var body string
		switch req.URL.Path {
		case "/api/nuser/account/get":
			body = `{"code":200,"profile":{"userId":1}}`
		case "/eapi/song/enhance/player/url/v1":
			body = `{"code":-460,"message":"network risk"}`
		default:
			return nil, errors.New("unexpected request path")
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    req,
		}, nil
	})
	if !RestoreSession() {
		t.Fatal("could not restore the test session")
	}
	if profile, err := Account(); err != nil || profile == nil || profile.UserID != 1 {
		t.Fatalf("account lookup failed: %v", err)
	}
	stream, err := SongURL(context.Background(), 2, "exhigh")
	var rejection *SongURLError
	if stream != nil || !errors.As(err, &rejection) || rejection.Code != -460 {
		t.Fatalf("expected a playback rejection, got %v", err)
	}
}

// A credential read begun before logout cannot restore the account after memory was cleared.
func TestRestoreCannotReviveLoggedOutSession(t *testing.T) {
	isolatedSessionTest(t)
	started, release := make(chan struct{}), make(chan struct{})
	defaultSession.credentials.read = func() ([]byte, error) {
		close(started)
		<-release
		return []byte("MUSIC_U=old"), nil
	}
	restored := make(chan bool, 1)
	go func() { restored <- RestoreSession() }()
	<-started
	cleanup := ForgetSession()
	close(release)
	if <-restored {
		t.Fatal("late credential read restored logged-out account")
	}
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
	if cookieValue(defaultSession.client, "MUSIC_U") != "" {
		t.Fatal("logout undone")
	}
}

// Image responses must not transplant a CDN's cookies into the music account's domain.
func TestFetchKeepsCookieDomainsSeparate(t *testing.T) {
	client := isolatedSessionTest(t)
	client.Transport = songURLTransport(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Set-Cookie": []string{"MUSIC_U=cdn; Path=/"}}, Body: io.NopCloser(strings.NewReader("cover")), Request: req}, nil
	})
	if _, err := Fetch("https://cover.example/cover"); err != nil {
		t.Fatal(err)
	}
	if cookieValue(client, "MUSIC_U") != "" {
		t.Fatal("CDN cookie was installed as a music session")
	}
}

// Cookies set after a cover redirect belong to the response host, not the original music URL.
func TestFetchRedirectKeepsResponseCookieDomain(t *testing.T) {
	client := isolatedSessionTest(t)
	client.Transport = songURLTransport(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "music.163.com" {
			return &http.Response{StatusCode: 302, Header: http.Header{"Location": []string{"https://cover.example/cover"}}, Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Set-Cookie": []string{"MUSIC_U=cdn; Path=/"}}, Body: io.NopCloser(strings.NewReader("cover")), Request: req}, nil
	})
	if _, err := Fetch("https://music.163.com/cover"); err != nil {
		t.Fatal(err)
	}
	if cookieValue(client, "MUSIC_U") != "" {
		t.Fatal("redirected CDN cookie became a music session")
	}
}
