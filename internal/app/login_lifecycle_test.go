package app

import (
	"errors"
	"github.com/egoist/mygo/ui"
	"image/png"
	"netease-music-mygo/internal/netease"
	"os"
	"path/filepath"
	"testing"
)

type fakeLogin struct {
	commits  int
	canceled bool
	done     chan struct{}
}

func (f *fakeLogin) Key() (string, error) { return "test", nil }
func (f *fakeLogin) Check(string) (int, string, string, string, error) {
	return netease.QRWaiting, "", "", "", nil
}
func (f *fakeLogin) Account() (*netease.Profile, error) { return &netease.Profile{UserID: 1}, nil }
func (f *fakeLogin) Commit() (func() error, error) {
	f.commits++
	return func() error { return nil }, nil
}
func (f *fakeLogin) Cancel() {
	if !f.canceled {
		f.canceled = true
		close(f.done)
	}
}
func (f *fakeLogin) Done() <-chan struct{} { return f.done }

// Closing or replacing the login dialog must discard its pending successful response.
func TestLoginRejectsStaleConfirmation(t *testing.T) {
	for _, closed := range []bool{true, false} {
		a := newTestApp()
		f := &fakeLogin{done: make(chan struct{})}
		a.login = loginState{open: !closed, gen: 3, attempt: f}
		gen := 3
		if !closed {
			gen = 2
		}
		a.acceptLogin(gen, f, &netease.Profile{UserID: 1}, nil)
		if f.commits != 0 || !f.canceled || a.profile != nil {
			t.Fatal("obsolete login was committed")
		}
	}
}

// Failure to fetch the authorized account must leave the existing session uncommitted.
func TestLoginAccountFailureDoesNotCommit(t *testing.T) {
	a := newTestApp()
	f := &fakeLogin{done: make(chan struct{})}
	defer f.Cancel()
	a.login = loginState{open: true, gen: 1, attempt: f}
	a.acceptLogin(1, f, nil, errors.New("offline"))
	if f.commits != 0 || a.login.finished || a.login.err == "" {
		t.Fatal("account failure was hidden")
	}
}

// A persistence failure keeps this login usable, and the retry button requests another save.
func TestLoginSaveRetryButton(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	saved := make(chan struct{}, 1)
	a.login = loginState{open: true, gen: 1, finished: true, persist: func() error { saved <- struct{}{}; return nil }}
	a.loginSaveComplete(1, errors.New("credential store unavailable"))
	if a.login.saveErr == "" || !a.login.finished || a.login.saving {
		t.Fatal("persistence failure did not leave a retryable login")
	}
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if dir := os.Getenv("NM_LOGIN_FRAME"); dir != "" {
		f, err := os.Create(filepath.Join(dir, "login-save-error.png"))
		if err != nil {
			t.Fatal(err)
		}
		if err = png.Encode(f, tt.Image()); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	if err := tt.Click("重试保存"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	wait(t, saved, "retry saving")
	if !a.login.saving || a.login.saveErr != "" {
		t.Fatalf("retry did not start: saving=%v", a.login.saving)
	}
}

// A save finishing for an earlier generation must not change a newly opened login dialog.
func TestLoginIgnoresObsoleteSaveResult(t *testing.T) {
	a := newTestApp()
	a.login = loginState{open: true, gen: 2}
	a.loginSaveComplete(1, errors.New("old failure"))
	if a.login.saveErr != "" || a.login.saving || !a.login.open {
		t.Fatal("obsolete save changed current dialog")
	}
}

// Escape closes the QR dialog and cancels its private request context before a response is accepted.
func TestLoginEscapeCancelsAttempt(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	f := &fakeLogin{done: make(chan struct{})}
	a.login = loginState{open: true, gen: 1, attempt: f}
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.login.open || !f.canceled || a.login.attempt != nil {
		t.Fatal("closing the dialog left its attempt alive")
	}
}
