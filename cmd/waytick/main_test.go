package main

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/arwyn-s/waytick/internal/daemon"
)

// parseStart runs "waytick start <args>" through the real CLI and returns
// the request it would have sent to the daemon.
func parseStart(args []string, now time.Time) (daemon.Request, error) {
	var got daemon.Request
	app := newApp(func() time.Time { return now }, func(r daemon.Request) (daemon.Response, error) {
		got = r
		return daemon.Response{OK: true}, nil
	})
	app.Writer, app.ErrWriter = io.Discard, io.Discard
	err := app.Run(context.Background(), append([]string{"waytick", "start"}, args...))
	return got, err
}

var now = time.Date(2026, 11, 10, 14, 0, 0, 0, time.Local)

func TestParseStart(t *testing.T) {
	cases := []struct {
		args    []string
		total   time.Duration
		startAt time.Time
		wantErr bool
	}{
		{args: []string{"25m"}, total: 25 * time.Minute},
		{args: []string{"1h30m"}, total: 90 * time.Minute},
		{args: []string{"45s"}, total: 45 * time.Second},
		{args: []string{"25m", "--in", "1m"}, total: 25 * time.Minute, startAt: now.Add(time.Minute)},
		{args: []string{"--in", "1m", "25m"}, total: 25 * time.Minute, startAt: now.Add(time.Minute)},
		{args: []string{"10m", "--at", "14:30"}, total: 10 * time.Minute, startAt: now.Add(30 * time.Minute)},
		{args: []string{"10m", "--at", "13:00"}, total: 10 * time.Minute, startAt: now.Add(23 * time.Hour)}, // past → tomorrow
		{args: []string{"10m", "--at", "14:00"}, total: 10 * time.Minute, startAt: now.Add(24 * time.Hour)}, // now → tomorrow
		{args: []string{"0s"}, wantErr: true},
		{args: []string{"-5m"}, wantErr: true},
		{args: []string{"abc"}, wantErr: true},
		{args: []string{"25"}, wantErr: true}, // Go needs a unit
		{args: []string{}, wantErr: true},
		{args: []string{"5m", "6m"}, wantErr: true},
		{args: []string{"5m", "--in", "0s"}, wantErr: true},
		{args: []string{"5m", "--at", "25:00"}, wantErr: true},
		{args: []string{"5m", "--in", "1m", "--at", "15:00"}, wantErr: true},
	}
	for _, c := range cases {
		req, err := parseStart(c.args, now)
		if c.wantErr {
			if err == nil {
				t.Errorf("%v: want error, got %+v", c.args, req)
			}
			continue
		}
		if err != nil {
			t.Errorf("%v: %v", c.args, err)
			continue
		}
		if req.Total != c.total || !req.StartAt.Equal(c.startAt) {
			t.Errorf("%v: got total=%v start=%v, want %v %v", c.args, req.Total, req.StartAt, c.total, c.startAt)
		}
	}
}
