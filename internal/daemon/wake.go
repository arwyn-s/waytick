package daemon

import (
	"time"

	"github.com/arwyn-s/waytick/internal/timer"
)

// maxSleep caps every wait. Go timers run on the monotonic clock, which stops
// during suspend, so a long sleep would fire late after a resume.
const maxSleep = time.Second

// nextWake returns how long until t's state or its waybar output can next
// change, or false if only a command can change it (idle, paused).
//
// The countdown is shown in whole seconds rounded up (waybar.Clock), so the
// text changes exactly when the remaining time crosses a whole second.
func nextWake(t *timer.Timer, now time.Time) (time.Duration, bool) {
	var until time.Duration
	switch t.Status {
	case timer.Scheduled:
		until = toSecond(t.StartAt.Sub(now))
	case timer.Running:
		left := t.EndAt.Sub(now)
		until = toSecond(left)
		if w := left - t.Threshold(); !t.Warned && w > 0 {
			until = min(until, w)
		}
	case timer.Done:
		until = t.DoneAt.Add(timer.DoneLinger).Sub(now)
	default:
		return 0, false
	}
	return min(max(until, 0), maxSleep), true
}

// toSecond returns the time until d next reaches a whole second.
func toSecond(d time.Duration) time.Duration {
	if d <= 0 {
		return 0
	}
	if r := d % time.Second; r > 0 {
		return r
	}
	return time.Second
}
