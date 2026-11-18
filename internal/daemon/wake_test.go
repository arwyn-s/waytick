package daemon

import (
	"testing"
	"time"

	"github.com/arwyn-s/waytick/internal/timer"
)

func TestNextWake(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	ms := time.Millisecond
	cases := []struct {
		name string
		t    timer.Timer
		want time.Duration
		ok   bool
	}{
		{"idle", timer.Timer{Status: timer.Idle}, 0, false},
		{"zero value", timer.Timer{}, 0, false},
		{"paused", timer.Timer{Status: timer.Paused, Total: time.Minute, Remaining: 30 * time.Second}, 0, false},
		{"running, mid-second", timer.Timer{Status: timer.Running, Total: time.Hour, EndAt: now.Add(10*time.Minute + 300*ms)}, 300 * ms, true},
		{"running, on a second", timer.Timer{Status: timer.Running, Total: time.Hour, EndAt: now.Add(10 * time.Minute)}, time.Second, true},
		{"running, warning first", timer.Timer{Status: timer.Running, Total: 100 * time.Second, EndAt: now.Add(10*time.Second + 900*ms)}, 900 * ms, true},
		{"running, warning before boundary", timer.Timer{Status: timer.Running, Total: 105 * time.Second, EndAt: now.Add(10*time.Second + 900*ms)}, 400 * ms, true},
		{"running, already warned", timer.Timer{Status: timer.Running, Total: 105 * time.Second, Warned: true, EndAt: now.Add(10*time.Second + 900*ms)}, 900 * ms, true},
		{"running, overdue", timer.Timer{Status: timer.Running, Total: time.Minute, EndAt: now.Add(-time.Second)}, 0, true},
		{"scheduled", timer.Timer{Status: timer.Scheduled, Total: time.Minute, StartAt: now.Add(5*time.Minute + 250*ms)}, 250 * ms, true},
		{"done, lingering", timer.Timer{Status: timer.Done, DoneAt: now.Add(-9*time.Second - 600*ms)}, 400 * ms, true},
		{"done, long linger capped", timer.Timer{Status: timer.Done, DoneAt: now}, maxSleep, true},
	}
	for _, c := range cases {
		got, ok := nextWake(&c.t, now)
		if got != c.want || ok != c.ok {
			t.Errorf("%s: got %v %v, want %v %v", c.name, got, ok, c.want, c.ok)
		}
	}
}
