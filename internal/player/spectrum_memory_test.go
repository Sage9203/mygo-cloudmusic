package player

import "testing"

// Long and unknown-duration tracks must keep measuring without reserving their entire spectrum.
func TestSpectrumLongAndUnknown(t *testing.T) {
	for _, frames := range []int64{0, 192000 * 36000} {
		s := newSpectrum(192000, frames)
		if len(s.chunks) > maxSpectrumChunks {
			t.Fatal("spectrum allocation follows entire duration")
		}
		samples := make([]float32, fftSize*2)
		s.feed(0, samples)
		if _, ok := s.at(0); !ok {
			t.Fatal("first spectrum missing")
		}
		later := int64(maxSpectrumChunks) * fftSize
		s.feed(later, samples)
		if _, ok := s.at(later); !ok {
			t.Fatal("spectrum stopped beyond the initial window")
		}
		if _, ok := s.at(0); ok {
			t.Fatal("old frame returned the spectrum of a reused slot")
		}
	}
}
