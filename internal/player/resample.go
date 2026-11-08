package player

import (
	"io"
	"math"
)

// MyGo's shared mixer runs at 48 kHz. Resample decoded music with a band-limited
// filter before handing it to that mixer; its cubic interpolation alone aliases
// high-frequency content when reducing the rate. Native 48 kHz streams bypass it.
const playbackRate = 48000
const filterPhases = 256

type playbackResampler struct {
	stream
	ratio    float64
	radius   int
	kernel   []float64
	position int64 // output frames
	first    int64 // source frame at buffer[0]
	buffer   []float32
	scratch  [4096]float32
	end      int64 // actual source end, -1 until Read ends
	readErr  error
}

func forPlayback(s stream) stream {
	if s.SampleRate() == playbackRate {
		return s
	}
	r := &playbackResampler{stream: s, ratio: float64(s.SampleRate()) / playbackRate, end: -1}
	r.radius = 64 * max(1, int(math.Ceil(r.ratio)))
	taps := 2 * r.radius
	r.kernel = make([]float64, (filterPhases+1)*taps)
	cutoff := 0.97 * min(1, 1/r.ratio)
	for phase := 0; phase <= filterPhases; phase++ {
		row := r.kernel[phase*taps : (phase+1)*taps]
		sum := 0.0
		for j := range row {
			x := float64(j-r.radius+1) - float64(phase)/filterPhases
			if math.Abs(x) >= float64(r.radius) {
				continue
			}
			sinc := cutoff
			if x != 0 {
				sinc = math.Sin(math.Pi*cutoff*x) / (math.Pi * x)
			}
			window := 0.42 + 0.5*math.Cos(math.Pi*x/float64(r.radius)) + 0.08*math.Cos(2*math.Pi*x/float64(r.radius))
			row[j] = sinc * window
			sum += row[j]
		}
		for j := range row {
			row[j] /= sum
		}
	}
	return r
}

func (r *playbackResampler) SampleRate() int { return playbackRate }
func (r *playbackResampler) Len() int64 {
	n := r.stream.Len()
	if n < 0 {
		return n
	}
	return (n*playbackRate + int64(r.stream.SampleRate()) - 1) / int64(r.stream.SampleRate())
}

func (r *playbackResampler) SetPosition(frame int64) error {
	frame = max(frame, 0)
	first := max(int64(float64(frame)*r.ratio)-int64(r.radius)+1, 0)
	if err := r.stream.SetPosition(first); err != nil {
		return err
	}
	r.position, r.first, r.end = frame, first, -1
	r.buffer, r.readErr = r.buffer[:0], nil
	return nil
}

func (r *playbackResampler) fill(through int64) {
	for r.end < 0 && r.first+int64(len(r.buffer)/2) <= through {
		n, err := r.stream.Read(r.scratch[:])
		r.buffer = append(r.buffer, r.scratch[:n]...)
		if err == nil && n == 0 {
			err = io.ErrNoProgress
		}
		if err != nil {
			r.end = r.first + int64(len(r.buffer)/2)
			r.readErr = err
		}
	}
}

func (r *playbackResampler) Read(dst []float32) (int, error) {
	// Keep the filter's history, discarding earlier source frames once per read.
	keep := max(int64(float64(r.position)*r.ratio)-int64(r.radius)+1, 0)
	drop := min(max(keep-r.first, 0), int64(len(r.buffer)/2))
	r.buffer = r.buffer[:copy(r.buffer, r.buffer[drop*2:])]
	r.first += drop
	taps := 2 * r.radius
	n := 0
	for n+1 < len(dst) {
		at := float64(r.position) * r.ratio
		center := int64(at)
		r.fill(center + int64(r.radius))
		if r.end >= 0 && at >= float64(r.end) {
			return n, r.readErr
		}
		phase := (at - float64(center)) * filterPhases
		index := int(phase)
		fraction := phase - float64(index)
		a, b := r.kernel[index*taps:], r.kernel[(index+1)*taps:]
		left, right := 0.0, 0.0
		for j := 0; j < taps; j++ {
			source := center + int64(j-r.radius+1)
			offset := (source - r.first) * 2
			// Samples outside the recording are zero padding, not an extra output tail.
			if offset < 0 || offset+1 >= int64(len(r.buffer)) {
				continue
			}
			weight := a[j] + fraction*(b[j]-a[j])
			left += float64(r.buffer[offset]) * weight
			right += float64(r.buffer[offset+1]) * weight
		}
		dst[n], dst[n+1] = float32(left), float32(right)
		n += 2
		r.position++
	}
	return n, nil
}
