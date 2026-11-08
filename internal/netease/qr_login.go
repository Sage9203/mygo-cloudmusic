package netease

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/cookiejar"
)

// QRLogin owns its cookies until the active UI explicitly accepts it.
type QRLogin struct {
	ctx    context.Context
	cancel context.CancelFunc
	client *http.Client
	owner  *sessionManager
}

func NewQRLogin() *QRLogin {
	return defaultSession.newQRLogin()
}

func (s *sessionManager) newQRLogin() *QRLogin {
	ctx, cancel := context.WithCancel(context.Background())
	jar, _ := cookiejar.New(nil)
	return &QRLogin{ctx: ctx, cancel: cancel, owner: s, client: &http.Client{Timeout: s.client.Timeout, Transport: s.client.Transport, Jar: jar}}
}
func (l *QRLogin) Cancel()               { l.cancel() }
func (l *QRLogin) Done() <-chan struct{} { return l.ctx.Done() }
func (l *QRLogin) Key() (string, error) {
	var res struct {
		Code   int    `json:"code"`
		Unikey string `json:"unikey"`
	}
	if err := eapiPostClient(l.ctx, l.client, "/api/login/qrcode/unikey", map[string]any{"type": 3}, &res); err != nil {
		return "", err
	}
	if res.Code != 200 || res.Unikey == "" {
		return "", fmt.Errorf("netease: no QR key (code %d)", res.Code)
	}
	return res.Unikey, nil
}
func (l *QRLogin) Check(key string) (code int, msg, nickname, avatar string, err error) {
	var res struct {
		Code     int    `json:"code"`
		Message  string `json:"message"`
		Nickname string `json:"nickname"`
		Avatar   string `json:"avatarUrl"`
	}
	err = eapiPostClient(l.ctx, l.client, "/api/login/qrcode/client/login", map[string]any{"key": key, "type": 3}, &res)
	return res.Code, res.Message, res.Nickname, res.Avatar, err
}
func (l *QRLogin) Account() (*Profile, error) {
	req, err := http.NewRequestWithContext(l.ctx, "GET", apiBase+"/api/nuser/account/get", nil)
	if err != nil {
		return nil, err
	}
	var res struct {
		Profile *Profile `json:"profile"`
	}
	if err = doWithClient(l.client, req, &res); err != nil {
		return nil, err
	}
	if res.Profile == nil || res.Profile.UserID <= 0 {
		return nil, errors.New("authorized session has no account profile")
	}
	return res.Profile, nil
}

// Commit is a short in-memory operation, called only after the UI's generation/open check.
// The returned save runs off the UI thread and remains safe if a later session supersedes it.
func (l *QRLogin) Commit() (func() error, error) {
	if err := l.ctx.Err(); err != nil {
		return nil, err
	}
	cookies := l.client.Jar.Cookies(siteURL)
	authenticated := false
	for _, c := range cookies {
		if c.Name == "MUSIC_U" && c.Value != "" {
			authenticated = true
		}
	}
	if !authenticated {
		return nil, errors.New("authorized response has no session cookie")
	}
	l.owner.mu.Lock()
	defer l.owner.mu.Unlock()
	if err := l.ctx.Err(); err != nil {
		return nil, err
	}
	l.owner.replaceCookies(cookies)
	l.owner.revision++
	revision, data := l.owner.revision, l.owner.encodedSession()
	return func() error { return l.owner.persist(revision, data) }, nil
}
