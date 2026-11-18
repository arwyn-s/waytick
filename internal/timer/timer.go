// Package timer is the pure waytick state machine. It does no I/O and never
// reads the clock itself: every method takes `now`, so it is deterministic
// under test and safe across suspend (all instants are absolute).
package timer

import (
	"errors"
	"time"
)

type Status string

const (
	Idle      Status = "idle"
	Scheduled Status = "scheduled"
	Running   Status = "running"
	Paused    Status = "paused"
	Done      Status = "done"
)

type Event string // notification events

const (
	EvScheduled Event = "scheduled"
	EvStarted   Event = "started"
	EvWarning   Event = "warning"
	EvDone      Event = "done"
)

// DoneLinger is how long the Done state stays visible before returning to Idle.
const DoneLinger = 10 * time.Second

var (
	ErrInvalid = errors.New("duration must be positive")
	ErrState   = errors.New("no timer in a state that allows this")
)

// Timer is serialised as-is to state.json.
type Timer struct {
	Status    Status        `json:"status"`
	Total     time.Duration `json:"total"`
	StartAt   time.Time     `json:"start_at,omitzero"` // Scheduled: when it begins running
	EndAt     time.Time     `json:"end_at,omitzero"`   // Running: when it reaches zero
	Remaining time.Duration `json:"paused_remaining"`  // Paused: time left
	DoneAt    time.Time     `json:"done_at,omitzero"`  // Done: when it finished
	Started   bool          `json:"started_notified"`
	Warned    bool          `json:"warned"`
}

// Start replaces any timer. startAt <= now starts immediately.
func (t *Timer) Start(now time.Time, total time.Duration, startAt time.Time) ([]Event, error) {
	if total <= 0 {
		return nil, ErrInvalid
	}
	*t = Timer{Total: total}
	if startAt.After(now) {
		t.Status = Scheduled
		t.StartAt = startAt
		return []Event{EvScheduled}, nil
	}
	t.begin(now)
	return []Event{EvStarted}, nil
}

func (t *Timer) begin(at time.Time) {
	t.Status = Running
	t.StartAt = time.Time{}
	t.EndAt = at.Add(t.Total)
	t.Started = true
}

// Threshold is the remaining time at or below which the warning fires.
func (t *Timer) Threshold() time.Duration { return t.Total / 10 }

// Left returns the time remaining on the countdown (Total while Scheduled).
func (t *Timer) Left(now time.Time) time.Duration {
	switch t.Status {
	case Scheduled:
		return t.Total
	case Running:
		if d := t.EndAt.Sub(now); d > 0 {
			return d
		}
		return 0
	case Paused:
		return t.Remaining
	}
	return 0
}

// Tick advances the machine to `now`, returning the events crossed, in order.
// A large jump (suspend) can return several events at once.
func (t *Timer) Tick(now time.Time) []Event {
	var evs []Event // notification events
	if t.Status == Scheduled && !now.Before(t.StartAt) {
		// Start from the scheduled instant, not from now, so a late tick
		// does not stretch the timer.
		t.begin(t.StartAt)
		evs = append(evs, EvStarted)
	}
	if t.Status == Running {
		left := t.EndAt.Sub(now)
		if !t.Warned && left <= t.Threshold() {
			t.Warned = true
			if left > 0 {
				evs = append(evs, EvWarning)
			}
		}
		if left <= 0 {
			t.Status = Done
			t.DoneAt = now // linger from when it was observed, so it shows after a resume from suspend
			t.EndAt = time.Time{}
			evs = append(evs, EvDone)
		}
	}
	if t.Status == Done && now.Sub(t.DoneAt) >= DoneLinger {
		*t = Timer{Status: Idle}
	}
	return evs
}

func (t *Timer) Pause(now time.Time) error {
	if t.Status != Running {
		return ErrState
	}
	t.Remaining = t.Left(now)
	t.EndAt = time.Time{}
	t.Status = Paused
	return nil
}

func (t *Timer) Resume(now time.Time) error {
	if t.Status != Paused {
		return ErrState
	}
	t.EndAt = now.Add(t.Remaining)
	t.Remaining = 0
	t.Status = Running
	return nil
}

func (t *Timer) Toggle(now time.Time) error {
	if t.Status == Paused {
		return t.Resume(now)
	}
	return t.Pause(now)
}

func (t *Timer) Cancel() { *t = Timer{Status: Idle} }

// Add extends an active timer by d. With no active timer (Idle or Done) it
// starts a fresh d-long timer, which is what the "+5m" action relies on.
func (t *Timer) Add(now time.Time, d time.Duration) ([]Event, error) {
	if d <= 0 {
		return nil, ErrInvalid
	}
	switch t.Status {
	case Idle, Done, "":
		return t.Start(now, d, time.Time{})
	case Scheduled:
		t.Total += d
	case Running:
		t.Total += d
		t.EndAt = t.EndAt.Add(d)
	case Paused:
		t.Total += d
		t.Remaining += d
	}
	// Re-arm the warning if the extension lifted us back above the threshold.
	if t.Warned && t.Left(now) > t.Threshold() {
		t.Warned = false
	}
	return nil, nil
}
