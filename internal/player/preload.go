package player

import (
	"context"
	"time"

	"netease-music-mygo/internal/netease"
)

const preloadBytes = 256 << 10

type preloaded struct {
	id     int64
	level  string
	ctx    context.Context
	cancel context.CancelFunc
	info   *netease.Stream
	dl     *download
	at     time.Time
}

// CancelPreload releases the single lookahead when its queue, account or quality changes.
func (p *Player) CancelPreload() {
	if q := p.preload; q != nil {
		p.preload = nil
		q.cancel()
		if q.dl != nil {
			q.dl.Close()
		}
	}
}

func (p *Player) takePreload(id int64, level string) *preloaded {
	q := p.preload
	if q == nil || q.id != id || q.level != level || q.dl == nil || time.Since(q.at) > 5*time.Minute {
		return nil
	}
	p.preload = nil
	return q
}

// Wait for the current download to finish before fetching at most one prefix.
// No decoder or audio output is opened until that song is actually selected.
func (p *Player) planPreload(current context.Context, gen int, s stream, level string) {
	if p.repeat == RepeatOne || len(p.order) < 2 {
		return
	}
	at := p.at + 1
	if at >= len(p.order) {
		if p.repeat != RepeatAll {
			return
		}
		at = 0
	}
	t := p.queue[p.order[at]]
	if !t.Playable() {
		return
	}
	p.CancelPreload()
	ctx, cancel := context.WithCancel(context.Background())
	q := &preloaded{id: t.ID, level: level, ctx: ctx, cancel: cancel, at: time.Now()}
	p.preload = q
	dl, _ := s.source()
	go func() {
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		for {
			dl.mu.Lock()
			done, err := dl.done, dl.err
			dl.mu.Unlock()
			if done {
				if err != nil {
					return
				}
				break
			}
			select {
			case <-ctx.Done():
				return
			case <-current.Done():
				return
			case <-ticker.C:
			}
		}
		next, info, cached := p.Cache.lookup(t.ID, level)
		if !cached {
			var err error
			info, err = netease.SongURL(ctx, t.ID, level)
			if err != nil {
				return
			}
			next = p.Cache.downloadPaused(ctx, t.ID, info, preloadBytes)
		}
		p.post(func() {
			if p.preload != q || p.gen != gen || ctx.Err() != nil {
				next.Close()
				return
			}
			q.info, q.dl = info, next
		})
	}()
}
