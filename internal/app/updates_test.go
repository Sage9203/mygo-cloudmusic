package app

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

type releaseTransport func(*http.Request) (*http.Response, error)

// Release summaries show readable headings and keep complete items where possible.
func TestReleaseNotesSummary(t *testing.T) {
	short := appRelease{Body: "### 本版更新\r\n\r\n- 修复问题"}
	if got := short.notes(); got != "本版更新\n\n• 修复问题" {
		t.Fatalf("notes = %q", got)
	}
	item := strings.Repeat("字", 350)
	long := appRelease{Body: "- " + item + "\n- " + item}
	if got := long.notes(); got != "• "+item+"…" {
		t.Fatalf("summary did not end at the first item: %q", got)
	}
	shortFirst := appRelease{Body: "### 更新\n- 修复问题\n- " + strings.Repeat("字", 600)}
	if got := shortFirst.notes(); got != "更新\n• 修复问题…" {
		t.Fatalf("summary cut the second item: %q", got)
	}
}

func (f releaseTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// Release selection compares numeric versions and respects the installed channel.
func TestFindAppRelease(t *testing.T) {
	const payload = `[
		{"tag_name":"v0.1.26-preview.10","published_at":"2026-11-08T10:00:00Z","prerelease":true},
		{"tag_name":"v0.1.26-preview.2","published_at":"2026-11-09T10:00:00Z","prerelease":true},
		{"tag_name":"v0.1.25","published_at":"2026-11-07T10:00:00Z"},
		{"tag_name":"v9.0.0","published_at":"2026-11-09T10:00:00Z","draft":true},
		{"tag_name":"v8.0.0","published_at":null},
		{"tag_name":"v7.0.0/elsewhere","published_at":"2026-11-09T10:00:00Z"}
	]`
	for _, test := range []struct {
		current string
		tag     string
		newer   bool
	}{
		{"0.1.26-preview.2", "v0.1.26-preview.10", true},
		{"0.1.26-preview.10+local.1", "v0.1.26-preview.10", false},
		{"0.1.26", "v0.1.25", false},
	} {
		t.Run(test.current, func(t *testing.T) {
			client := &http.Client{Transport: releaseTransport(func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodGet || req.URL.String() != releasesAPI+"?per_page=100&page=1" ||
					req.Header.Get("Authorization") != "" || req.Header.Get("Cookie") != "" {
					t.Fatal("expected an unauthenticated GET of the project's releases")
				}
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(payload))}, nil
			})}
			got, newer, err := findAppRelease(context.Background(), client, test.current)
			if err != nil || got.Tag != test.tag || newer != test.newer {
				t.Fatalf("release = %q, newer = %v, err = %v", got.Tag, newer, err)
			}
		})
	}
}

// HTTP failures and empty responses must leave the result unknown.
func TestFindAppReleaseUnavailable(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusForbidden} {
		client := &http.Client{Transport: releaseTransport(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader("[]"))}, nil
		})}
		if _, newer, err := findAppRelease(context.Background(), client, "0.1.26-preview.2"); newer || err == nil {
			t.Fatalf("status %d: newer = %v, err = %v", status, newer, err)
		}
	}
}

// A failed manual check can be retried, and the release button opens the selected tag.
func TestCheckForUpdatesFromSettings(t *testing.T) {
	const current = "0.1.26-preview.2"
	const tag = "v0.1.26-preview.3"
	original := checkAppRelease
	defer func() { checkAppRelease = original }()
	replies := make(chan releaseCheckState, 2)
	replies <- releaseCheckState{err: context.DeadlineExceeded}
	replies <- releaseCheckState{
		release: appRelease{Tag: tag, Body: "更新说明正文", PublishedAt: time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC)},
		newer:   true,
	}
	versions := make(chan string, 2)
	checkAppRelease = func(version string) (appRelease, bool, error) {
		versions <- version
		reply := <-replies
		return reply.release, reply.newer, reply.err
	}
	pending := make(chan func(), 1)
	a := newTestApp()
	tt := ui.NewTester(func(c *ui.Context) {
		a.settingsContent(c, current, func(fn func()) { pending <- fn })
	}, 560, 2000)
	clickAndComplete := func() {
		t.Helper()
		if err := tt.Click("检查更新"); err != nil {
			t.Fatal(err)
		}
		if !a.releaseCheck.running {
			t.Fatal("the click did not start a check")
		}
		select {
		case complete := <-pending:
			complete()
		case <-time.After(5 * time.Second):
			t.Fatal("the worker did not return to the UI")
		}
		if got := <-versions; got != current {
			t.Fatalf("checked version %q", got)
		}
		tt.Frame()
	}
	tt.Frame()
	clickAndComplete()
	if a.releaseCheck.running || !errors.Is(a.releaseCheck.err, context.DeadlineExceeded) {
		t.Fatal("the timed-out check did not finish with its error")
	}
	clickAndComplete()
	if err := tt.Click("查看发布页"); err != nil {
		t.Fatal(err)
	}
	if urls := tt.OpenedURLs(); len(urls) != 1 || urls[0] != releasesPage+tag {
		t.Fatalf("opened %v", urls)
	}
}

// The data kept under the app's old name moves to the directory of its
// name, which Path made empty, but not over data kept there already.
func TestMoveOldData(t *testing.T) {
	root := t.TempDir()
	old, dir := filepath.Join(root, oldName), filepath.Join(root, "MyGo 云音乐")
	os.MkdirAll(old, 0o700)
	os.WriteFile(filepath.Join(old, "settings.json"), []byte("old"), 0o600)
	os.MkdirAll(dir, 0o700)
	moveDir(old, dir)
	if b, _ := os.ReadFile(filepath.Join(dir, "settings.json")); string(b) != "old" {
		t.Fatalf("settings %q after moving", b)
	}

	os.MkdirAll(old, 0o700)
	os.WriteFile(filepath.Join(old, "settings.json"), []byte("older"), 0o600)
	moveDir(old, dir)
	if b, _ := os.ReadFile(filepath.Join(dir, "settings.json")); string(b) != "old" {
		t.Fatalf("settings %q, moved over what was there", b)
	}
}
