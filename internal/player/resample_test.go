package player

import (
	"fmt"
	"io"
	"math"
	"os"
	"testing"
)

type signalStream struct {
	rate     int
	samples  []float32
	position int
}

func (s *signalStream) SampleRate() int                { return s.rate }
func (s *signalStream) Channels() int                  { return 2 }
func (s *signalStream) Len() int64                     { return int64(len(s.samples) / 2) }
func (s *signalStream) Close() error                   { return nil }
func (s *signalStream) source() (*download, *spectrum) { return nil, nil }
func (s *signalStream) SetPosition(frame int64) error  { s.position = int(frame) * 2; return nil }
func (s *signalStream) Read(dst []float32) (int, error) {
	n := copy(dst, s.samples[s.position:])
	s.position += n
	if s.position == len(s.samples) {
		return n, io.EOF
	}
	return n, nil
}
func tones(rate int, frequencies ...float64) *signalStream {
	s := &signalStream{rate: rate, samples: make([]float32, rate*2)}
	for i := 0; i < rate; i++ {
		for _, freq := range frequencies {
			s.samples[2*i] += float32(0.25 * math.Sin(2*math.Pi*freq*float64(i)/float64(rate)))
		}
		s.samples[2*i+1] = -s.samples[2*i]
	}
	return s
}
func readSignal(t *testing.T, s stream, block int) []float32 {
	t.Helper()
	var out []float32
	buf := make([]float32, block)
	for {
		n, err := s.Read(buf)
		out = append(out, buf[:n]...)
		if err != nil {
			if err != io.EOF {
				t.Fatal(err)
			}
			return out
		}
	}
}
func toneAmplitude(samples []float32, frequency float64) float64 {
	// Measure a central integer-duration interval, excluding filter boundary padding.
	begin, end := 4800, len(samples)/2-4800
	re, im := 0.0, 0.0
	for i := begin; i < end; i++ {
		angle := 2 * math.Pi * frequency * float64(i) / playbackRate
		re += float64(samples[2*i]) * math.Cos(angle)
		im += float64(samples[2*i]) * math.Sin(angle)
	}
	return 2 * math.Hypot(re, im) / float64(end-begin)
}

// Reducing Hi-Res audio must suppress frequencies that would fold into the audible band.
func TestPlaybackResamplerRejectsAliasing(t *testing.T) {
	for _, rate := range []int{96000, 192000} {
		s := forPlayback(tones(rate, 1000, 30000))
		got := readSignal(t, s, 1014)
		if s.SampleRate() != playbackRate || len(got) != 2*playbackRate {
			t.Fatalf("rate %d: wrong output duration/rate", rate)
		}
		pass, alias := toneAmplitude(got, 1000), toneAmplitude(got, 18000)
		t.Logf("%d Hz: 1kHz amplitude %.8f, folded 18kHz amplitude %.8f", rate, pass, alias)
		if math.Abs(pass-0.25) > 0.0005 || alias > 0.0001 {
			t.Fatalf("rate %d: pass=%g alias=%g", rate, pass, alias)
		}
	}
}

// Upsampling preserves audible high-frequency amplitude and does not mix the stereo channels.
func TestPlaybackResamplerPassband(t *testing.T) {
	for _, rate := range []int{44100, 88200, 96000, 192000} {
		got := readSignal(t, forPlayback(tones(rate, 1000, 19000, 20000)), 254)
		for _, frequency := range []float64{1000, 19000, 20000} {
			amplitude := toneAmplitude(got, frequency)
			t.Logf("%d Hz: %.0f Hz amplitude %.8f", rate, frequency, amplitude)
			if math.Abs(amplitude-0.25) > 0.0025 {
				t.Errorf("rate %d: %.0fHz amplitude %g", rate, frequency, amplitude)
			}
		}
		for i := 0; i < len(got); i += 2 {
			if got[i] != -got[i+1] {
				t.Fatal("stereo channels were changed")
			}
		}
	}
}

// Seeking uses source history so that playback resumes at the same filtered samples.
func TestPlaybackResamplerSeek(t *testing.T) {
	for _, rate := range []int{44100, 96000} {
		s := forPlayback(tones(rate, 1703))
		reference := readSignal(t, s, 512)
		if s.Len() != playbackRate {
			t.Fatal("incorrect reported duration")
		}
		for _, frame := range []int64{0, 1, 4567, 47980} {
			if err := s.SetPosition(frame); err != nil {
				t.Fatal(err)
			}
			buf := make([]float32, 40)
			n, err := s.Read(buf)
			if err != nil && err != io.EOF {
				t.Fatal(err)
			}
			if d := maxDiff(buf[:n], reference[frame*2:]); d > 1e-6 {
				t.Fatalf("rate %d seek %d: error %g", rate, frame, d)
			}
		}
	}
}

// Native-rate playback keeps samples exact, including values below 16-bit precision.
func TestPlaybackNativeRateBypassesResampling(t *testing.T) {
	s := tones(playbackRate, 1234)
	s.samples[0], s.samples[1] = 1.0/(1<<23), -1.0/(1<<23)
	got := readSignal(t, forPlayback(s), 322)
	if maxDiff(got, s.samples) != 0 {
		t.Fatal("native samples changed")
	}
}

// This reports the cost of filtering one second of music, including filter setup.
func BenchmarkPlaybackResampler(b *testing.B) {
	for _, rate := range []int{44100, 96000, 192000} {
		b.Run(fmt.Sprint(rate), func(b *testing.B) {
			signal := tones(rate, 1000, 30000)
			buf := make([]float32, 4096)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				signal.SetPosition(0)
				s := forPlayback(signal)
				for {
					_, err := s.Read(buf)
					if err == io.EOF {
						break
					}
					if err != nil {
						b.Fatal(err)
					}
				}
			}
		})
	}
}

// Filtering must preserve exact FLAC seeks and reproduce MP3 playback after returning to its start.
func TestPlaybackResamplerWithCodecs(t *testing.T) {
	for _, format := range []string{"flac", "mp3"} {
		t.Run(format, func(t *testing.T) {
			data, err := os.ReadFile("testdata/seek." + format)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := openStream(downloaded(data), 5000)
			if err != nil {
				t.Fatal(err)
			}
			defer raw.Close()
			s := forPlayback(raw)
			reference := readSignal(t, s, 1024)
			positions := []int64{0}
			if format == "flac" {
				positions = append(positions, 1, 12345, 200123)
			}
			for _, position := range positions {
				if err := s.SetPosition(position); err != nil {
					t.Fatal(err)
				}
				buf := make([]float32, 1024)
				n, err := s.Read(buf)
				if err != nil && err != io.EOF {
					t.Fatal(err)
				}
				if d := maxDiff(buf[:n], reference[position*2:]); d > 1e-6 {
					t.Fatalf("position %d differs by %g", position, d)
				}
			}
		})
	}
}
