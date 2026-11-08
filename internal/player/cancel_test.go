package player

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"netease-music-mygo/internal/netease"
)

// A stop while opening a track used to leave its request and decoder waiting
// for the first bytes; cancellation now unblocks both without a total timeout.
func TestStopCancelsPendingAudio(t *testing.T) {
	entered := make(chan struct{})
	interrupted := make(chan error, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		close(entered)
		<-r.Context().Done()
		interrupted <- r.Context().Err()
	}))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d := (*Cache)(nil).download(ctx, 1, &netease.Stream{URL: srv.URL, Type: "mp3", Size: 1024})
	finished := make(chan error, 1)
	go func() {
		s, err := openStream(d, 1000)
		if err == nil {
			s.Close()
		} else {
			d.Close()
		}
		finished <- err
	}()
	<-entered
	p := &Player{cancel: cancel}
	p.stop()
	if !errors.Is(<-interrupted, context.Canceled) {
		t.Error("old request was not cancelled")
	}
	if err := <-finished; err == nil {
		t.Error("opening an interrupted stream succeeded")
	}
	if p.cancel != nil {
		t.Error("stop kept the old cancellation function")
	}
}
