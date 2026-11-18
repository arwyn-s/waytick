// Package waybar renders a timer snapshot as a waybar custom-module JSON line.
package waybar

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/arwyn-s/waytick/internal/timer"
)

type Output struct {
	Text       string   `json:"text"`
	Alt        string   `json:"alt"`
	Class      []string `json:"class"`
	Tooltip    string   `json:"tooltip"`
	Percentage int      `json:"percentage"`
}

const (
	iconRunning   = "󰔛"
	iconScheduled = "󰥔"
	iconPaused    = "󰏤"
	iconDone      = "󰀠"
)

// Clock formats d as m:ss, or h:mm:ss from an hour up. Rounds up so a timer
// never shows 0:00 while it still has time left.
func Clock(d time.Duration) string {
	s := max(int((d+time.Second-1)/time.Second), 0)
	h, m, sec := s/3600, s/60%60, s%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, sec)
	}
	return fmt.Sprintf("%d:%02d", m, sec)
}

// Render builds the module output for t at now.
func Render(t *timer.Timer, now time.Time) Output {
	o := Output{Alt: string(t.Status), Class: []string{string(t.Status)}}
	if t.Status == timer.Idle || t.Status == "" {
		o.Alt, o.Class = "idle", []string{"idle"}
		return o
	}

	// Whole seconds, rounded up like Clock, so output changes at most 1×/s.
	left := (t.Left(now) + time.Second - 1).Truncate(time.Second)
	if t.Total > 0 {
		o.Percentage = int(100 * (t.Total - left) / t.Total)
	}
	bucket := min(o.Percentage/10, 9)
	total := Clock(t.Total)

	switch t.Status {
	case timer.Scheduled:
		o.Text = fmt.Sprintf("%s in %s", iconScheduled, Clock(t.StartAt.Sub(now)))
		o.Tooltip = fmt.Sprintf("%s timer\nstarts at %s", total, t.StartAt.Format("15:04"))
	case timer.Running:
		o.Text = fmt.Sprintf("%s %s", iconRunning, Clock(left))
		o.Tooltip = fmt.Sprintf("%s timer\nends at %s", total, t.EndAt.Format("15:04"))
		if t.Warned {
			o.Alt, o.Class[0] = "warning", "warning"
		}
	case timer.Paused:
		o.Text = fmt.Sprintf("%s %s", iconPaused, Clock(left))
		o.Tooltip = fmt.Sprintf("%s timer\npaused", total)
	case timer.Done:
		o.Text = iconDone + " done"
		o.Tooltip = total + " timer finished"
		o.Percentage, bucket = 100, 9
	}
	o.Class = append(o.Class, fmt.Sprintf("p%d", bucket))
	return o
}

func (o Output) JSON() []byte {
	b, _ := json.Marshal(o)
	return b
}
