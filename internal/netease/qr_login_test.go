package netease

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func isolatedSessionTest(t *testing.T) *http.Client {
	t.Helper()
	previous := defaultSession
	defaultSession = newSessionManager(credentialStore{
		read:   func() ([]byte, error) { return nil, nil },
		write:  func([]byte) error { return nil },
		remove: func() error { return nil },
	})
	deviceOnce.Do(func() { deviceID = "0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123" })
	t.Cleanup(func() { defaultSession = previous })
	return defaultSession.client
}
func cookieValue(client *http.Client, name string) string {
	for _, c := range client.Jar.Cookies(siteURL) {
		if c.Name == name {
			return c.Value
		}
	}
	return ""
}
func pendingLogin(token string) *QRLogin {
	l := NewQRLogin()
	l.client.Jar.SetCookies(siteURL, []*http.Cookie{{Name: "MUSIC_U", Value: token, Path: "/"}})
	return l
}

// A response arriving after cancellation must never replace the active account's cookies.
func TestCanceledQRResponseStaysPrivate(t *testing.T) {
	client := isolatedSessionTest(t)
	client.Jar.SetCookies(siteURL, []*http.Cookie{{Name: "MUSIC_U", Value: "existing", Path: "/"}})
	started, release := make(chan struct{}), make(chan struct{})
	client.Transport = songURLTransport(func(req *http.Request) (*http.Response, error) {
		close(started)
		<-release
		return &http.Response{StatusCode: 200, Header: http.Header{"Set-Cookie": []string{"MUSIC_U=late; Path=/"}}, Body: io.NopCloser(strings.NewReader(`{"code":803}`)), Request: req}, nil
	})
	l := NewQRLogin()
	done := make(chan struct{})
	go func() { defer close(done); _, _, _, _, _ = l.Check("fixture") }()
	<-started
	l.Cancel()
	close(release)
	<-done
	if got := cookieValue(client, "MUSIC_U"); got != "existing" {
		t.Fatalf("late login changed active session: %q", got)
	}
	if _, err := l.Commit(); err == nil {
		t.Fatal("cancelled attempt was accepted")
	}
}

// Login cookies are private until accepted, then credential failures can be retried without rescanning.
func TestQRCommitAndPersistenceRetry(t *testing.T) {
	client := isolatedSessionTest(t)
	l := pendingLogin("accepted")
	defer l.Cancel()
	calls := 0
	defaultSession.credentials.write = func(data []byte) error {
		calls++
		if !strings.Contains(string(data), "MUSIC_U=accepted") {
			t.Fatal("wrong session saved")
		}
		if calls == 1 {
			return errors.New("store unavailable")
		}
		return nil
	}
	if cookieValue(client, "MUSIC_U") != "" {
		t.Fatal("pending login polluted active jar")
	}
	save, err := l.Commit()
	if err != nil {
		t.Fatal(err)
	}
	l.Cancel()
	if calls != 0 || cookieValue(client, "MUSIC_U") != "accepted" {
		t.Fatal("commit should be memory-only")
	}
	if err := save(); err == nil {
		t.Fatal("lost credential failure")
	}
	if err := save(); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("save attempts %d", calls)
	}
}

// A stale queued save must not overwrite another account or undo local logout.
func TestObsoleteSessionWritesAreDiscarded(t *testing.T) {
	isolatedSessionTest(t)
	var stored string
	defaultSession.credentials.write = func(b []byte) error { stored = string(b); return nil }
	defaultSession.credentials.remove = func() error { stored = ""; return nil }
	first, second := pendingLogin("first"), pendingLogin("second")
	defer first.Cancel()
	defer second.Cancel()
	saveFirst, err := first.Commit()
	if err != nil {
		t.Fatal(err)
	}
	saveSecond, err := second.Commit()
	if err != nil {
		t.Fatal(err)
	}
	if err := saveSecond(); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(saveFirst(), ErrSessionChanged) {
		t.Fatal("stale save accepted")
	}
	if !strings.Contains(stored, "second") {
		t.Fatalf("new account overwritten: %q", stored)
	}
	clear := defaultSession.clear()
	if err := clear(); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(saveSecond(), ErrSessionChanged) {
		t.Fatal("logout undone by late save")
	}
	if stored != "" {
		t.Fatal("logout did not clear stored session")
	}
}

// A logout response may arrive after a new login, without changing that new account.
func TestLateLogoutResponseIsIsolated(t *testing.T) {
	client := isolatedSessionTest(t)
	old := pendingLogin("old")
	defer old.Cancel()
	if _, err := old.Commit(); err != nil {
		t.Fatal(err)
	}
	started, release := make(chan struct{}), make(chan struct{})
	client.Transport = songURLTransport(func(req *http.Request) (*http.Response, error) {
		close(started)
		<-release
		return &http.Response{StatusCode: 200, Header: http.Header{"Set-Cookie": []string{"MUSIC_U=; Max-Age=0; Path=/"}}, Body: io.NopCloser(strings.NewReader(`{"code":200}`)), Request: req}, nil
	})
	logout := BeginLogout()
	if cookieValue(client, "MUSIC_U") != "" {
		t.Fatal("logout did not clear memory immediately")
	}
	done := make(chan error, 1)
	go func() { done <- logout() }()
	<-started
	current := pendingLogin("current")
	defer current.Cancel()
	save, err := current.Commit()
	if err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if cookieValue(client, "MUSIC_U") != "current" {
		t.Fatal("late logout erased new session")
	}
	if err := save(); err != nil {
		t.Fatal(err)
	}
}

// Missing profile or authentication cookies must not be accepted as a successful login.
func TestQRLoginRejectsIncompleteAuthorization(t *testing.T) {
	client := isolatedSessionTest(t)
	client.Transport = songURLTransport(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"code":200,"profile":null}`)), Request: req}, nil
	})
	l := NewQRLogin()
	defer l.Cancel()
	if _, err := l.Account(); err == nil {
		t.Fatal("nil account accepted")
	}
	if _, err := l.Commit(); err == nil {
		t.Fatal("missing session cookie accepted")
	}
}

// Ordinary requests already in flight must not reinstall old cookies after local logout.
func TestOldAPIResponseCannotReviveSession(t *testing.T) {
	for _, kind := range []string{"eapi", "web"} {
		t.Run(kind, func(t *testing.T) {
			client := isolatedSessionTest(t)
			client.Jar.SetCookies(siteURL, []*http.Cookie{{Name: "MUSIC_U", Value: "old", Path: "/"}})
			started, release := make(chan struct{}), make(chan struct{})
			client.Transport = songURLTransport(func(req *http.Request) (*http.Response, error) {
				close(started)
				<-release
				return &http.Response{StatusCode: 200, Header: http.Header{"Set-Cookie": []string{"MUSIC_U=late; Path=/"}}, Body: io.NopCloser(strings.NewReader(`{"code":200}`)), Request: req}, nil
			})
			done := make(chan error, 1)
			go func() {
				var res map[string]any
				if kind == "eapi" {
					done <- eapiPost("/api/fixture", map[string]any{}, &res)
				} else {
					done <- get("/api/fixture", nil, &res)
				}
			}()
			<-started
			clear := defaultSession.clear()
			if err := clear(); err != nil {
				t.Fatal(err)
			}
			close(release)
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			if cookieValue(client, "MUSIC_U") != "" {
				t.Fatal("stale response revived logged-out session")
			}
		})
	}
}

// A save already writing when logout occurs must finish before the queued credential deletion.
func TestLogoutWaitsForInFlightSave(t *testing.T) {
	isolatedSessionTest(t)
	var stored string
	started, release := make(chan struct{}), make(chan struct{})
	defaultSession.credentials.write = func(b []byte) error { close(started); <-release; stored = string(b); return nil }
	defaultSession.credentials.remove = func() error { stored = ""; return nil }
	l := pendingLogin("accepted")
	defer l.Cancel()
	save, err := l.Commit()
	if err != nil {
		t.Fatal(err)
	}
	saved := make(chan error, 1)
	go func() { saved <- save() }()
	<-started
	cleanup := defaultSession.clear()
	cleared := make(chan error, 1)
	go func() { cleared <- cleanup() }()
	close(release)
	if !errors.Is(<-saved, ErrSessionChanged) {
		t.Fatal("superseded in-flight save reported current")
	}
	if err := <-cleared; err != nil {
		t.Fatal(err)
	}
	if stored != "" {
		t.Fatal("logout left saved account behind")
	}
}
