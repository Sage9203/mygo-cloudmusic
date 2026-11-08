package app

import (
	"context"
	"testing"
)

// A disabled caption feature must reject download actions before touching its unavailable source.
func TestDisabledCaptionsDoNotDownload(t *testing.T) {
	previous, previousDownload := captionsAvailable, downloadModel
	t.Cleanup(func() { captionsAvailable = previous; downloadModel = previousDownload })
	captionsAvailable = func() bool { return false }
	calls := 0
	downloadModel = func(context.Context, string, func(int64, int64)) error { calls++; return nil }
	a := newTestApp()
	a.startModelDownload()
	if calls != 0 || a.captions.model.downloading {
		t.Fatal("disabled feature started a download")
	}
}
