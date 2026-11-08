package app

import "time"

// Returning to the app refreshes account data without discarding the page or
// the playing queue. Attempts are throttled even when the network fails.
func (a *app) freshen() {
	a.freshenHome()
	a.freshenAccount(time.Now())
	if a.route.page == pagePlaylist {
		a.loadPlaylist(a.route.id, false)
	}
}

func (a *app) freshenAccount(now time.Time) {
	if a.profile == nil || now.Sub(a.accountAt) < time.Minute {
		return
	}
	a.accountAt = now
	a.loadLiked(a.profile.UserID)
	a.loadMine(a.profile.UserID)
}
