package app

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/png"
	"testing"
)

// A giant PNG header must be refused before either full image decode allocates pixels.
func TestCoverDimensionsBeforeDecode(t *testing.T) {
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	data := b.Bytes()
	binary.BigEndian.PutUint32(data[16:20], 100000)
	binary.BigEndian.PutUint32(data[20:24], 100000)
	binary.BigEndian.PutUint32(data[29:33], crc32.ChecksumIEEE(data[12:29]))
	if _, _, err := decodeArt(data); err == nil {
		t.Fatal("oversized cover decoded")
	}
}

// A backdrop reuses the existing cover bitmap and thumbnail without a second fetch.
func TestBackdropSharesCover(t *testing.T) {
	q := make(chan func(), 1)
	c := newArtCache(func(fn func()) { q <- fn })
	preview := image.NewRGBA(image.Rect(0, 0, 48, 48))
	cover := &artwork{preview: preview, size: len(preview.Pix)}
	c.add("https://invalid.example/cover", cover)
	c.get("https://invalid.example/cover", true)
	wait(t, q, "backdrop")()
	back := c.get("https://invalid.example/cover", true)
	if back == nil || back.bmp != cover.bmp || back.backdrop == nil {
		t.Fatal("backdrop did not share the loaded cover")
	}
}
