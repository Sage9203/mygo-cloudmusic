package player

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"netease-music-mygo/internal/netease"
)

// Consuming a FLAC frame must not make its reusable output allocation disappear.
func TestFLACOutputAllocationBudget(t *testing.T) {
	b, err := os.ReadFile("testdata/seek.flac")
	if err != nil {
		t.Fatal(err)
	}
	s, err := openFLACStream(downloaded(b))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	f, err := s.parse(s.r)
	if err != nil {
		t.Fatal(err)
	}
	allocs := testing.AllocsPerRun(50, func() { s.decode(f); s.pend = s.pend[len(s.pend):] })
	t.Logf("warmed output allocations per FLAC frame: %g", allocs)
	if allocs != 0 {
		t.Fatalf("frame output keeps allocating: %g", allocs)
	}
}

// Read-only backing storage must fall back without losing its already-downloaded prefix.
func TestAudioStorageFallbackPreservesBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audio.part")
	if err := os.WriteFile(path, []byte("prefix"), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	d := opened(f, 6)
	d.part = path
	defer d.Close()
	if err := d.write([]byte("-tail")); err != nil {
		t.Fatal(err)
	}
	if got := d.peek(0, 11); !bytes.Equal(got, []byte("prefix-tail")) {
		t.Fatalf("fallback corrupted bytes: %q", got)
	}
	if d.file != nil {
		t.Fatal("failed file still used")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("failed temporary backing not removed")
	}
}

// Unwritable storage must not allow its fallback buffer to grow without a bound.
func TestAudioMemoryFallbackLimit(t *testing.T) {
	d := downloaded(nil)
	defer d.Close()
	chunk := make([]byte, 512<<10)
	for i := 0; i < maxMemoryDownload/len(chunk); i++ {
		if err := d.write(chunk); err != nil {
			t.Fatal(err)
		}
	}
	if err := d.write([]byte{1}); !errors.Is(err, errDownloadMemory) {
		t.Fatalf("memory limit: %v", err)
	}
	if len(d.data) != maxMemoryDownload || cap(d.data) > maxMemoryDownload || d.n != maxMemoryDownload {
		t.Fatal("rejected write grew memory")
	}
}

// Reusing frame storage must preserve samples across partial reads and a seek.
func TestFLACReusedOutputSamples(t *testing.T) {
	b, err := os.ReadFile("testdata/seek.flac")
	if err != nil {
		t.Fatal(err)
	}
	want := decodeAll(t, b)
	s, err := openFLACStream(downloaded(b))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	buf := make([]float32, 258)
	var got []float32
	for {
		n, err := s.Read(buf)
		got = append(got, buf[:n]...)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if !slices.Equal(got, want) {
		t.Fatal("partial reads changed decoded samples")
	}
	if err := s.SetPosition(12345); err != nil {
		t.Fatal(err)
	}
	n, err := s.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(buf[:n], want[12345*2:12345*2+n]) {
		t.Fatal("seek changed decoded samples")
	}
}

// Uncached downloads must use removable storage instead of retaining the whole song.
func TestUncachedAudioStorage(t *testing.T) {
	body := bytes.Repeat([]byte("test-audio"), 1024)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(body) }))
	defer srv.Close()
	for _, cache := range []*Cache{nil, NewCache(t.TempDir(), 0), NewCache(t.TempDir(), 1)} {
		d := cache.download(context.Background(), 1, &netease.Stream{URL: srv.URL, Size: int64(len(body)), Level: "exhigh", Type: "mp3"})
		defer d.Close()
		got, err := io.ReadAll(&downloadReader{d: d})
		if err != nil || !bytes.Equal(got, body) {
			t.Fatalf("audio bytes: %v", err)
		}
		for {
			d.mu.Lock()
			users := d.users
			d.mu.Unlock()
			if users == 1 {
				break
			}
			runtime.Gosched()
		}
		d.mu.Lock()
		file, path, memory := d.file, d.part, len(d.data)
		d.mu.Unlock()
		if file == nil || path == "" || memory != 0 {
			t.Fatalf("uncached song retained in RAM: file=%v, bytes=%d", file != nil, memory)
		}
		if !d.share() {
			t.Fatal("cannot open second reader")
		}
		d.Close()
		if _, err := os.Stat(path); err != nil {
			t.Fatal("temporary file removed before final reader closed")
		}
		d.release()
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("temporary file retained after final reader closed")
		}
	}
}
