package player

import (
	"context"
	"errors"
	"io"
	"sync"
)

// watchedStream reports decoder failures before audio treats them as its end.
// The owning load generation rejects errors from a track already replaced.
type watchedStream struct {
	stream
	once   sync.Once
	failed func(error)
}

func (p *Player) watchStream(s stream, gen int) stream {
	return &watchedStream{stream: s, failed: func(err error) {
		p.post(func() { p.playbackFailed(gen, err) })
	}}
}

func (s *watchedStream) report(err error) {
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, context.Canceled) {
		s.once.Do(func() { s.failed(err) })
	}
}

func (s *watchedStream) Read(dst []float32) (int, error) {
	n, err := s.stream.Read(dst)
	s.report(err)
	return n, err
}

func (s *watchedStream) SetPosition(frame int64) error {
	err := s.stream.SetPosition(frame)
	s.report(err)
	return err
}

func (p *Player) playbackFailed(gen int, err error) {
	if gen != p.gen {
		return
	}
	p.stop()
	p.loading, p.playing, p.awaiting = false, false, false
	p.err = "播放失败：" + err.Error() + "；点击播放重试"
	p.sync()
}
