// Command testaudio regenerates synthetic regression fixtures without recordings or song samples.
package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/mewkiz/flac"
	"github.com/mewkiz/flac/frame"
	"github.com/mewkiz/flac/meta"
)

const rate = 44100
const length = 5 * rate

func main() {
	lame := flag.String("lame", "", "path to a separately installed LAME encoder")
	out := flag.String("out", "internal/player/testdata", "fixture directory")
	flag.Parse()
	if err := generate(*lame, *out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func generate(lame, out string) error {
	if lame == "" {
		return fmt.Errorf("provide -lame; the encoder is a fixture-generation tool only")
	}
	pcm := make([]int16, length*2)
	for i := 0; i < length; i++ {
		t := float64(i) / rate
		envelope := 0.55 + 0.25*math.Sin(2*math.Pi*0.7*t)
		pcm[2*i] = int16(24000 * envelope * (0.65*math.Sin(2*math.Pi*440*t) + 0.25*math.Sin(2*math.Pi*997*t)))
		pcm[2*i+1] = int16(23000 * envelope * (0.6*math.Sin(2*math.Pi*587*t) + 0.3*math.Sin(2*math.Pi*(200*t+30*t*t))))
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(out, "seek.flac"))
	if err != nil {
		return err
	}
	enc, err := flac.NewEncoder(f, &meta.StreamInfo{BlockSizeMin: 4096, BlockSizeMax: 4096, SampleRate: rate, NChannels: 2, BitsPerSample: 16, NSamples: length})
	if err != nil {
		f.Close()
		return err
	}
	for start := 0; start < length; start += 4096 {
		n := min(4096, length-start)
		block := &frame.Frame{Header: frame.Header{HasFixedBlockSize: true, BlockSize: uint16(n), SampleRate: rate, Channels: frame.ChannelsLR, BitsPerSample: 16}}
		for ch := 0; ch < 2; ch++ {
			samples := make([]int32, n)
			for i := range samples {
				samples[i] = int32(pcm[2*(start+i)+ch])
			}
			block.Subframes = append(block.Subframes, &frame.Subframe{SubHeader: frame.SubHeader{Pred: frame.PredVerbatim}, Samples: samples, NSamples: n})
		}
		if err := enc.WriteFrame(block); err != nil {
			f.Close()
			return err
		}
	}
	if err := enc.Close(); err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "mygo-test-audio-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	wav := filepath.Join(dir, "signal.wav")
	header := []byte("RIFF\x00\x00\x00\x00WAVEfmt \x10\x00\x00\x00\x01\x00\x02\x00\x44\xac\x00\x00\x10\xb1\x02\x00\x04\x00\x10\x00data\x00\x00\x00\x00")
	data := make([]byte, len(pcm)*2)
	for i, sample := range pcm {
		binary.LittleEndian.PutUint16(data[2*i:], uint16(sample))
	}
	binary.LittleEndian.PutUint32(header[4:], uint32(36+len(data)))
	binary.LittleEndian.PutUint32(header[40:], uint32(len(data)))
	if err := os.WriteFile(wav, append(header, data...), 0600); err != nil {
		return err
	}
	cmd := exec.Command(lame, "--silent", "--cbr", "-b", "128", "--noreplaygain", "-t", wav, filepath.Join(out, "seek.mp3"))
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("encode MP3: %w: %s", err, output)
	}
	return nil
}
