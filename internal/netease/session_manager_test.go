package netease

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

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
