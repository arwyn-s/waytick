package timer

import (
	"slices"
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 10, 14, 0, 0, 0, time.Local)

func at(d time.Duration) time.Time { return t0.Add(d) }

func mustEvents(t *testing.T, got []Event, want ...Event) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
}

func TestStartImmediate(t *testing.T) {
	var tm Timer
	evs, err := tm.Start(t0, 100*time.Second, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	mustEvents(t, evs, EvStarted)
	if tm.Status != Running || !tm.EndAt.Equal(at(100*time.Second)) {
		t.Fatalf("got %+v", tm)
	}
}

func TestStartInvalid(t *testing.T) {
	var tm Timer
	for _, d := range []time.Duration{0, -5 * time.Minute} {
		if _, err := tm.Start(t0, d, time.Time{}); err == nil {
			t.Errorf("Start(%v) accepted", d)
		}
	}
}

func TestFullLifecycleEventsOnce(t *testing.T) {
	var tm Timer
	evs, _ := tm.Start(t0, 100*time.Second, at(5*time.Second))
	mustEvents(t, evs, EvScheduled)

	var all []Event
	for s := 0; s <= 120; s++ {
		all = append(all, tm.Tick(at(time.Duration(s)*time.Second))...)
	}
	mustEvents(t, all, EvStarted, EvWarning, EvDone)
	if tm.Status != Idle {
		t.Fatalf("status = %v, want idle after linger", tm.Status)
	}
}

func TestWarningThreshold(t *testing.T) {
	var tm Timer
	tm.Start(t0, 100*time.Second, time.Time{})
	mustEvents(t, tm.Tick(at(89*time.Second)))            // 11s left
	mustEvents(t, tm.Tick(at(90*time.Second)), EvWarning) // 10s left = 10%
	mustEvents(t, tm.Tick(at(91*time.Second)))
}

func TestJumpFiresInOrder(t *testing.T) {
	var tm Timer
	tm.Start(t0, 100*time.Second, at(5*time.Second))
	// Suspend through the whole thing: started then done; the warning is
	// marked but not emitted because the timer already expired.
	mustEvents(t, tm.Tick(at(200*time.Second)), EvStarted, EvDone)
	if tm.Status != Done {
		t.Fatalf("status = %v", tm.Status)
	}
	if !tm.DoneAt.Equal(at(200 * time.Second)) {
		t.Fatalf("DoneAt = %v", tm.DoneAt)
	}
	mustEvents(t, tm.Tick(at(210*time.Second)))
	if tm.Status != Idle {
		t.Fatalf("linger should have expired, status = %v", tm.Status)
	}
}

func TestJumpIntoWarning(t *testing.T) {
	var tm Timer
	tm.Start(t0, 100*time.Second, at(5*time.Second))
	mustEvents(t, tm.Tick(at(100*time.Second)), EvStarted, EvWarning)
}

func TestScheduledStartsAtScheduledInstant(t *testing.T) {
	var tm Timer
	tm.Start(t0, time.Minute, at(10*time.Second))
	tm.Tick(at(13 * time.Second)) // late tick
	if !tm.EndAt.Equal(at(70 * time.Second)) {
		t.Fatalf("EndAt = %v, want start+total from scheduled instant", tm.EndAt)
	}
}

func TestPauseKeepsRemaining(t *testing.T) {
	var tm Timer
	tm.Start(t0, time.Minute, time.Time{})
	if err := tm.Pause(at(20 * time.Second)); err != nil {
		t.Fatal(err)
	}
	mustEvents(t, tm.Tick(at(10*time.Minute)))
	if got := tm.Left(at(10 * time.Minute)); got != 40*time.Second {
		t.Fatalf("Left while paused = %v", got)
	}
	tm.Resume(at(10 * time.Minute))
	if got := tm.Left(at(10*time.Minute + 5*time.Second)); got != 35*time.Second {
		t.Fatalf("Left after resume = %v", got)
	}
}

func TestToggleAndStateErrors(t *testing.T) {
	var tm Timer
	if err := tm.Pause(t0); err == nil {
		t.Fatal("pause on idle should fail")
	}
	tm.Start(t0, time.Minute, time.Time{})
	tm.Toggle(at(time.Second))
	if tm.Status != Paused {
		t.Fatal("toggle should pause")
	}
	tm.Toggle(at(2 * time.Second))
	if tm.Status != Running {
		t.Fatal("toggle should resume")
	}
}

func TestAddRearmsWarning(t *testing.T) {
	var tm Timer
	tm.Start(t0, 100*time.Second, time.Time{})
	mustEvents(t, tm.Tick(at(95*time.Second)), EvWarning)
	tm.Add(at(95*time.Second), time.Minute) // 65s left of 160s total
	if tm.Warned {
		t.Fatal("warning should re-arm")
	}
	mustEvents(t, tm.Tick(at(143*time.Second)))            // 17s left, just above the 16s threshold
	mustEvents(t, tm.Tick(at(144*time.Second)), EvWarning) // 16s = 10% of 160s
}

func TestAddWhenDoneStartsNew(t *testing.T) {
	var tm Timer
	tm.Start(t0, time.Second, time.Time{})
	tm.Tick(at(time.Second))
	evs, err := tm.Add(at(2*time.Second), 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	mustEvents(t, evs, EvStarted)
	if tm.Total != 5*time.Minute || tm.Status != Running {
		t.Fatalf("got %+v", tm)
	}
}

func TestAddScheduled(t *testing.T) {
	var tm Timer
	tm.Start(t0, time.Minute, at(time.Minute))
	tm.Add(t0, time.Minute)
	if tm.Total != 2*time.Minute || tm.Status != Scheduled {
		t.Fatalf("got %+v", tm)
	}
}

func TestCancel(t *testing.T) {
	for _, setup := range []struct {
		name  string
		start func(*Timer)
	}{
		{"running", func(tm *Timer) { tm.Start(t0, time.Minute, time.Time{}) }},
		{"scheduled", func(tm *Timer) { tm.Start(t0, time.Minute, at(time.Minute)) }},
		{"paused", func(tm *Timer) { tm.Start(t0, time.Minute, time.Time{}); tm.Pause(at(time.Second)) }},
	} {
		t.Run(setup.name, func(t *testing.T) {
			var tm Timer
			setup.start(&tm)
			tm.Cancel()
			if tm != (Timer{Status: Idle}) {
				t.Fatalf("after cancel got %+v, want a clean idle timer", tm)
			}
			// Nothing scheduled may still fire.
			mustEvents(t, tm.Tick(at(10*time.Minute)))
		})
	}
}

func TestResumeWhenNotPaused(t *testing.T) {
	var tm Timer
	if err := tm.Resume(t0); err == nil {
		t.Fatal("resume on idle should fail")
	}
	tm.Start(t0, time.Minute, time.Time{})
	if err := tm.Resume(at(time.Second)); err == nil {
		t.Fatal("resume on running should fail")
	}
	if !tm.EndAt.Equal(at(time.Minute)) {
		t.Fatalf("failed resume must not change EndAt, got %v", tm.EndAt)
	}
}

func TestPauseScheduledRejected(t *testing.T) {
	var tm Timer
	tm.Start(t0, time.Minute, at(time.Minute))
	if err := tm.Pause(at(time.Second)); err == nil {
		t.Fatal("pause on scheduled should fail")
	}
	if tm.Status != Scheduled {
		t.Fatalf("status = %v, want scheduled", tm.Status)
	}
}

func TestAddInvalid(t *testing.T) {
	var tm Timer
	if _, err := tm.Start(t0, time.Minute, time.Time{}); err != nil {
		t.Fatal(err)
	}
	for _, d := range []time.Duration{0, -time.Minute} {
		if _, err := tm.Add(at(time.Second), d); err == nil {
			t.Errorf("Add(%v) accepted", d)
		}
	}
	if tm.Total != time.Minute || !tm.EndAt.Equal(at(time.Minute)) {
		t.Fatalf("rejected Add must not change the timer, got %+v", tm)
	}
}

func TestLeft(t *testing.T) {
	var tm Timer
	if got := tm.Left(t0); got != 0 {
		t.Errorf("idle Left = %v, want 0", got)
	}
	tm.Start(t0, time.Minute, at(30*time.Second))
	if got := tm.Left(at(10 * time.Second)); got != time.Minute {
		t.Errorf("scheduled Left = %v, want full total", got)
	}
	tm.Tick(at(30 * time.Second))
	if got := tm.Left(at(50 * time.Second)); got != 40*time.Second {
		t.Errorf("running Left = %v, want 40s", got)
	}
	// Past EndAt without a Tick: clamps to zero, never negative.
	if got := tm.Left(at(5 * time.Minute)); got != 0 {
		t.Errorf("overdue Left = %v, want 0", got)
	}
}
