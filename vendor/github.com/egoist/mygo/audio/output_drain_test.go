package audio

import (
	"errors"
	"slices"
	"testing"
)

type queuedDevice struct {
	pending, heard []float32
	closed         bool
	fail           bool
}

func (d *queuedDevice) write(samples []float32) error {
	d.pending = append(d.pending, samples...)
	if d.fail {
		return errors.New("device removed")
	}
	mu.Lock()
	requestedFormat = outputFormat{rate: 96000}
	mu.Unlock()
	return nil
}
func (d *queuedDevice) drain() { d.heard = append(d.heard, d.pending...); d.pending = nil }
func (d *queuedDevice) close() { d.pending = nil; d.closed = true }

// A sample-rate change must play already-queued tail samples before closing the old output.
func TestFormatChangePreservesQueuedTail(t *testing.T) {
	testQueuedTail(t, false)
}

// A removed device must close directly rather than wait for playback it cannot finish.
func TestDeviceFailureSkipsDrain(t *testing.T) {
	testQueuedTail(t, true)
}

func testQueuedTail(t *testing.T, fail bool) {
	t.Helper()
	mu.Lock()
	oldFormat, oldRate, oldVoices, oldVolume := requestedFormat, mixRate, voices, volume
	oldMixed, oldUnheard, oldHeld := mixed, unheard, held
	defer func() {
		mu.Lock()
		requestedFormat, mixRate, voices, volume = oldFormat, oldRate, oldVoices, oldVolume
		mixed, unheard, held = oldMixed, oldUnheard, oldHeld
		mu.Unlock()
	}()
	format := outputFormat{rate: 48000}
	requestedFormat, mixRate, volume = format, format.rate, 1
	mixed, unheard, held = 0, 0, 4800
	samples := []float32{0.25, -0.25}
	p := newPlayer(&memSource{samples: samples, channels: 2}, format.rate, 2, 1)
	p.v.playing, p.v.active, p.v.snap = true, true, true
	voices = []*voice{p.v}
	mu.Unlock()
	d := &queuedDevice{fail: fail}
	playDevice(d, 1, outputLatency(), format)
	if !d.closed {
		t.Fatal("output left open")
	}
	if fail {
		if len(d.heard) != 0 {
			t.Fatal("attempted to drain a failed output")
		}
	} else if !slices.Equal(d.heard, samples) {
		t.Fatalf("queued tail lost during format change: heard %v, want %v", d.heard, samples)
	}
}
