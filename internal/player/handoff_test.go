package player

import (
	"bytes"
	"context"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// A cache file that cannot be reopened must report its error, not successful EOF.
func TestCacheHandoffReadFailure(t *testing.T) {
	body := []byte("downloaded audio")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(body) }))
	defer srv.Close()
	part, err := os.CreateTemp(t.TempDir(), "*.part")
	if err != nil {
		t.Fatal(err)
	}
	d := startDownload(context.Background(), srv.URL, int64(len(body)), part, func(path string, n int64) (string, bool) {
		return filepath.Join(filepath.Dir(path), "missing.flac"), false
	})
	defer d.Close()
	d.waitFor(math.MaxInt64)
	_, err = io.ReadAll(&downloadReader{d: d})
	if !os.IsNotExist(err) {
		t.Fatalf("cache reopen failure became EOF: %v", err)
	}
	if d.Progress() == 1 {
		t.Fatal("failed cache handoff reported complete")
	}
}

// A successful cache move must preserve the whole download for current readers.
func TestCacheHandoffPreservesAudio(t *testing.T) {
	body := bytes.Repeat([]byte("audio"), 4096)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(body) }))
	defer srv.Close()
	dir := t.TempDir()
	for _, keep := range []bool{false, true} {
		part, err := os.CreateTemp(dir, "*.part")
		if err != nil {
			t.Fatal(err)
		}
		original := part.Name()
		kept := filepath.Join(dir, "kept.flac")
		d := startDownload(context.Background(), srv.URL, int64(len(body)), part, func(path string, n int64) (string, bool) {
			if !keep {
				return path, false
			}
			if err := os.Rename(path, kept); err != nil {
				t.Error(err)
				return path, false
			}
			return kept, true
		})
		defer d.Close()
		d.waitFor(math.MaxInt64)
		// Completion wakes readers before the writer drops its reference.
		for {
			d.mu.Lock()
			users := d.users
			d.mu.Unlock()
			if users == 1 {
				break
			}
			runtime.Gosched()
		}
		got, err := io.ReadAll(&downloadReader{d: d})
		if err != nil || !bytes.Equal(got, body) {
			t.Fatalf("handoff corrupted audio: %v", err)
		}
		if d.Progress() != 1 {
			t.Fatal("successful download incomplete")
		}
		d.Close()
		if _, err := os.Stat(original); !os.IsNotExist(err) {
			t.Fatal("old temporary file retained")
		}
		if keep {
			if _, err := os.Stat(kept); err != nil {
				t.Fatal("kept file removed", err)
			}
		}
	}
}
