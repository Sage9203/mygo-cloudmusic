package netease

import (
	"errors"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"
)

var ErrSessionChanged = errors.New("session changed before it could be saved")

// credentialStore is concrete: the OS implementation and test stores share these operations.
type credentialStore struct {
	read   func() ([]byte, error)
	write  func([]byte) error
	remove func() error
}

// sessionManager owns the active cookies, their generation and serialized credential I/O.
// Production keeps one instance; callbacks retain that instance rather than re-reading globals.
type sessionManager struct {
	mu          sync.Mutex
	revision    uint64
	storeMu     sync.Mutex
	client      *http.Client
	credentials credentialStore
}

func newSessionManager(credentials credentialStore) *sessionManager {
	jar, _ := cookiejar.New(nil)
	return &sessionManager{client: &http.Client{Timeout: 20 * time.Second, Jar: jar}, credentials: credentials}
}

var defaultSession = newSessionManager(credentialStore{readSession, writeSession, deleteSession})
var siteURL, _ = url.Parse(apiBase)

// RestoreSession reads credentials on a worker, rejecting a result superseded by login/logout.
func RestoreSession() bool { return defaultSession.restore() }

func (s *sessionManager) restore() bool {
	s.mu.Lock()
	revision := s.revision
	s.mu.Unlock()
	b, err := s.credentials.read()
	if err != nil || len(b) == 0 {
		return false
	}
	var cookies []*http.Cookie
	for _, part := range strings.Split(string(b), ";") {
		name, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok {
			cookies = append(cookies, &http.Cookie{Name: name, Value: value, Path: "/"})
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.revision != revision {
		return false
	}
	s.replaceCookies(cookies)
	s.revision++
	return true
}

// These helpers run under s.mu; the cookie jar is itself safe for HTTP workers.
func (s *sessionManager) replaceCookies(cookies []*http.Cookie) {
	s.client.Jar.SetCookies(siteURL, []*http.Cookie{{Name: "MUSIC_U", MaxAge: -1, Path: "/"}, {Name: "__csrf", MaxAge: -1, Path: "/"}, {Name: "NMTID", MaxAge: -1, Path: "/"}})
	s.client.Jar.SetCookies(siteURL, cookies)
}
func (s *sessionManager) encodedSession() []byte {
	var parts []string
	for _, c := range s.client.Jar.Cookies(siteURL) {
		if c.Name == "MUSIC_U" || c.Name == "__csrf" {
			parts = append(parts, c.Name+"="+c.Value)
		}
	}
	return []byte(strings.Join(parts, "; "))
}

// Writes are serialized, and an obsolete queued save cannot overwrite a later login/logout.
func (s *sessionManager) persist(revision uint64, data []byte) error {
	s.storeMu.Lock()
	defer s.storeMu.Unlock()
	s.mu.Lock()
	current := s.revision
	s.mu.Unlock()
	if revision != current {
		return ErrSessionChanged
	}
	var err error
	if len(data) == 0 {
		err = s.credentials.remove()
	} else {
		err = s.credentials.write(data)
	}
	if err != nil {
		return err
	}
	s.mu.Lock()
	current = s.revision
	s.mu.Unlock()
	if revision != current {
		return ErrSessionChanged
	}
	return nil
}
func (s *sessionManager) clear() func() error {
	s.mu.Lock()
	s.replaceCookies(nil)
	s.revision++
	revision := s.revision
	s.mu.Unlock()
	return func() error { return s.persist(revision, nil) }
}

// BeginLogout clears memory immediately; slow credential and remote work belongs on a worker.
func (s *sessionManager) beginLogout() func() error {
	jar, _ := cookiejar.New(nil)
	jar.SetCookies(siteURL, s.client.Jar.Cookies(siteURL))
	previous := &http.Client{Timeout: s.client.Timeout, Transport: s.client.Transport, Jar: jar}
	cleanup := s.clear()
	return func() error {
		err := cleanup()
		if errors.Is(err, ErrSessionChanged) {
			return nil
		}
		if err != nil {
			return err
		}
		// This request is isolated: its response cannot clear a newly accepted session.
		req, err := http.NewRequest("POST", apiBase+"/api/logout", strings.NewReader(url.Values{}.Encode()))
		if err == nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			_ = doWithClient(previous, req, &struct{}{})
		}
		return nil
	}
}

// Responses started under a previous account cannot reinstall its cookies after logout.
func (s *sessionManager) applyResponse(revision uint64, u *url.URL, cookies []*http.Cookie) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if revision != s.revision {
		return
	}
	s.client.Jar.SetCookies(u, cookies)
}

// ForgetSession clears local memory now and returns the queued credential deletion.
func ForgetSession() func() error { return defaultSession.clear() }

// BeginLogout clears active memory now; the returned worker serializes credential cleanup
// before sending logout with an isolated copy of the old cookies.
func BeginLogout() func() error { return defaultSession.beginLogout() }

// snapshot pairs request cookies and revision atomically with login/logout.
func (s *sessionManager) snapshot(u *url.URL) (uint64, []*http.Cookie) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.revision, s.client.Jar.Cookies(u)
}

// request prevents an in-flight response from repopulating cookies after account changes.
func (s *sessionManager) request(req *http.Request) (*http.Response, error) {
	revision, cookies := s.snapshot(req.URL)
	client := *s.client
	client.Jar = nil
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	res, err := client.Do(req)
	if err == nil {
		responseURL := req.URL
		if res.Request != nil {
			responseURL = res.Request.URL
		}
		s.applyResponse(revision, responseURL, res.Cookies())
	}
	return res, err
}
