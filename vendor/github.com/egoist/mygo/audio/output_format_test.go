//go:build windows && (amd64 || arm64)

package audio

import (
	"encoding/binary"
	"math"
	"os"
	"runtime"
	"testing"
	"time"
	"unsafe"
)

// Native-rate playback must advance one source second per output second.
func TestNativeRateClock(t *testing.T) {
	mu.Lock()
	defer mu.Unlock()
	saved := mixRate
	defer func() { mixRate = saved }()
	for _, rate := range []int{44100, 48000, 96000, 192000} {
		mixRate = rate
		samples := make([]float32, rate*4)
		for i := range samples {
			samples[i] = float32(i%101-50) / (1 << 23)
		}
		v := &voice{src: &memSource{samples: samples, channels: 2}, rate: float64(rate), length: int64(rate * 2), volume: 1, speed: 1, playing: true, snap: true}
		v.reset(0)
		out := make([]float32, rate*2)
		if n, _ := v.play(out, 1); n != rate || math.Abs(v.at()-float64(rate)) > 0.001 {
			t.Fatalf("rate %d: %d frames, position %g", rate, n, v.at())
		}
		for i, got := range out {
			if got != samples[i] {
				t.Fatalf("rate %d sample %d: %g != %g", rate, i, got, samples[i])
			}
		}
	}
}

// Reconfiguration must reject an old device buffer before consuming the new track.
func TestOutputFormatChange(t *testing.T) {
	before := outputSettings()
	mu.Lock()
	testMode = true // configure without starting a real device in a unit test
	mu.Unlock()
	defer func() { mu.Lock(); requestedFormat = before; testMode = false; mu.Unlock() }()
	ConfigureOutput(96000, true)
	mu.Lock()
	testMode = false
	mu.Unlock()
	if _, ok := mixFor(make([]float32, 960), outputLatency(), before); ok {
		t.Fatal("mixed new source into old output format")
	}
}

// Integer output must preserve 24-bit samples, clip endpoints, and pad valid bits correctly.
func TestOutputPCM(t *testing.T) {
	samples := []float32{-1, -0.5, -1.0 / (1 << 23), 0, 1.0 / (1 << 23), 0.5, 1, 2}
	for _, f := range exclusiveFormats(96000) {
		b := make([]byte, len(samples)*int(f.bitsPerSample/8))
		encodeOutput(b, samples, f)
		for i, s := range samples {
			width := int(f.bitsPerSample / 8)
			var got float64
			if f.subFormat == ksDataFormatSubtypeFloat {
				got = float64(math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:])))
			} else {
				var raw uint32
				for k := 0; k < width; k++ {
					raw |= uint32(b[i*width+k]) << (8 * k)
				}
				value := int32(raw<<(32-f.bitsPerSample)) >> (32 - f.bitsPerSample)
				value >>= f.bitsPerSample - f.validBits
				scale := math.Ldexp(1, int(f.validBits)-1)
				got = float64(value) / scale
				want := min(max(math.Round(float64(s)*scale), -scale), scale-1) / scale
				if got != want {
					t.Fatalf("%d/%d sample %g: %g != %g", f.bitsPerSample, f.validBits, s, got, want)
				}
				continue
			}
			if got != float64(s) {
				t.Fatal("float sample changed")
			}
		}
	}
}

// Optional local probe opens and writes silence, without changing Windows device settings.
func TestDeviceFormats(t *testing.T) {
	if os.Getenv("MYGO_OUTPUT_PROBE") == "" {
		t.Skip("explicit local hardware probe only")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, rate := range []int{44100, 48000, 96000, 192000, 12345} {
		for _, exclusive := range []bool{false, true} {
			d, n, err := openDevice(100*time.Millisecond, outputFormat{rate, exclusive})
			if err != nil {
				t.Errorf("%d exclusive=%v: %v", rate, exclusive, err)
				continue
			}
			for i := 0; i < 60; i++ {
				if err := d.write(make([]float32, n*2)); err != nil {
					t.Errorf("write %d exclusive=%v: %v", rate, exclusive, err)
					break
				}
			}
			t.Logf("requested %d exclusive=%v: %s", rate, exclusive, OutputStatus())
			d.close()
		}
	}
}

// Optional hardware check verifies that a normal close can finish queued samples.
func TestDeviceDrain(t *testing.T) {
	if os.Getenv("MYGO_OUTPUT_PROBE") == "" {
		t.Skip("explicit local hardware probe only")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, exclusive := range []bool{false, true} {
		dev, n, err := openDevice(100*time.Millisecond, outputFormat{rate: 48000, exclusive: exclusive})
		if err != nil {
			t.Skipf("current audio device unavailable for probe: %v", err)
		}
		d := dev.(*wasapiDevice)
		func() {
			defer d.close()
			for i := 0; i < 10; i++ {
				if err := d.write(make([]float32, n*2)); err != nil {
					t.Fatal(err)
				}
			}
			var before, after uint32
			if hr := comCall(d.client, 6, uintptr(unsafe.Pointer(&before))); failed(hr) {
				t.Fatal(hresult("padding before drain", hr))
			}
			d.drain()
			if hr := comCall(d.client, 6, uintptr(unsafe.Pointer(&after))); failed(hr) {
				t.Fatal(hresult("padding after drain", hr))
			}
			t.Logf("exclusive=%v: queued %d frames, after drain %d", d.exclusive, before, after)
			if after != 0 {
				t.Fatal("queued samples did not finish before close")
			}
		}()
	}
}
