// Package daemon owns the single waytick timer: it serves commands on a unix
// socket, ticks the state machine, persists state, sends notifications and
// streams waybar output to `waytick watch` subscribers.
package daemon

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/arwyn-s/waytick/internal/notify"
	"github.com/arwyn-s/waytick/internal/timer"
	"github.com/arwyn-s/waytick/internal/waybar"
)

// Dir is $XDG_RUNTIME_DIR/waytick. or tmp/waytick-xxxxx/
func Dir() string {
	base := os.Getenv("XDG_RUNTIME_DIR")
	if base == "" {
		base = filepath.Join(os.TempDir(), fmt.Sprintf("waytick-%d", os.Getuid()))
	}
	return filepath.Join(base, "waytick")
}

func SocketPath() string { return filepath.Join(Dir(), "sock") }
func statePath() string  { return filepath.Join(Dir(), "state.json") }

// Request is one JSON line sent by the client.
type Request struct {
	Cmd     string        `json:"cmd"`
	Total   time.Duration `json:"total,omitempty"`   // start
	StartAt time.Time     `json:"start_at,omitzero"` // start (scheduled)
	Dur     time.Duration `json:"dur,omitempty"`     // add
}

// Response is the single JSON line answered for every command except watch.
type Response struct {
	OK     bool          `json:"ok"`
	Error  string        `json:"error,omitempty"`
	Timer  timer.Timer   `json:"timer"`
	Output waybar.Output `json:"output"`
}

type Daemon struct {
	mu    sync.Mutex
	t     timer.Timer
	saved []byte
	last  []byte
	subs  map[chan []byte]struct{}
	notes chan *notify.Notification // nil = close the current one
	notif *notify.Notifier
	wake  *time.Timer // fires tick at the next moment the state can change
}

func Run() error {
	if err := os.MkdirAll(Dir(), 0o700); err != nil {
		return err
	}
	ln, err := listen(SocketPath())
	if err != nil {
		return err
	}
	defer ln.Close()

	d := &Daemon{
		t:     timer.Timer{Status: timer.Idle},
		subs:  map[chan []byte]struct{}{},
		notes: make(chan *notify.Notification, 16),
	}
	d.notif = &notify.Notifier{AppName: "waytick", OnAction: d.onAction}
	d.wake = time.AfterFunc(time.Hour, d.tick)
	d.wake.Stop()
	d.load()
	go d.notifyWorker()

	d.tick() // fire anything that came due while we were down, and arm d.wake

	log.Printf("waytick: listening on %s", SocketPath())
	for {
		c, err := ln.Accept()
		if err != nil {
			return err
		}
		go d.serve(c)
	}
}

// listen binds the socket, replacing a stale one but refusing to steal a live one.
func listen(p string) (net.Listener, error) {
	if c, err := net.Dial("unix", p); err == nil {
		c.Close()
		return nil, errors.New("waytick daemon already running")
	}
	os.Remove(p)
	return net.Listen("unix", p)
}

func (d *Daemon) load() {
	b, err := os.ReadFile(statePath())
	if err != nil {
		return
	}
	var t timer.Timer
	if err := json.Unmarshal(b, &t); err != nil {
		log.Printf("waytick: ignoring bad state file: %v", err)
		return
	}
	d.t, d.saved = t, b
}

// persist writes state.json if it changed. Caller holds d.mu.
func (d *Daemon) persist() {
	b, _ := json.Marshal(d.t)
	if bytes.Equal(b, d.saved) {
		return
	}
	tmp := statePath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err == nil {
		err = os.Rename(tmp, statePath())
		if err != nil {
			log.Printf("waytick: save state: %v", err)
		}
	}
	d.saved = b
}

// tick is run by d.wake.
func (d *Daemon) tick() {
	d.mu.Lock()
	d.step(time.Now(), nil)
	d.mu.Unlock()
}

// step runs a tick, handles events, persists, broadcasts and schedules the
// next wake. Caller holds d.mu. extra are events produced by a command just
// applied.
func (d *Daemon) step(now time.Time, extra []timer.Event) {
	evs := append(extra, d.t.Tick(now)...)
	for _, ev := range evs {
		d.announce(ev, now)
	}
	d.persist()
	d.broadcast(now)
	if wait, ok := nextWake(&d.t, now); ok {
		d.wake.Reset(wait)
	} else {
		d.wake.Stop() // idle or paused: only a command can change anything
	}
}

func (d *Daemon) broadcast(now time.Time) {
	line := waybar.Render(&d.t, now).JSON()
	if bytes.Equal(line, d.last) {
		return
	}
	d.last = line
	for ch := range d.subs {
		select {
		case ch <- line:
		default: // slow subscriber: drop, it gets the next line
		}
	}
}

func (d *Daemon) announce(ev timer.Event, now time.Time) {
	total := waybar.Clock(d.t.Total)
	var n notify.Notification
	switch ev {
	case timer.EvScheduled:
		n = notify.Notification{
			Urgency: notify.Low, Timeout: -1,
			Summary: fmt.Sprintf("⏳ %s timer in %s", total, waybar.Clock(d.t.StartAt.Sub(now))),
			Body:    "starts at " + d.t.StartAt.Format("15:04"),
		}
	case timer.EvStarted:
		n = notify.Notification{
			Urgency: notify.Normal, Timeout: -1,
			Summary: fmt.Sprintf("▶ %s timer started", total),
			Body:    "ends at " + d.t.EndAt.Format("15:04"),
		}
	case timer.EvWarning:
		n = notify.Notification{
			Urgency: notify.Normal, Timeout: -1,
			Summary: fmt.Sprintf("⚠ %s left", waybar.Clock(d.t.Left(now))),
			Body:    fmt.Sprintf("%s timer ends at %s", total, d.t.EndAt.Format("15:04")),
		}
	case timer.EvDone:
		n = notify.Notification{
			Urgency: notify.Critical, Timeout: 0,
			Summary: fmt.Sprintf("⏰ %s timer done", total),
			Body:    "finished at " + d.t.DoneAt.Format("15:04"),
			Actions: []string{"add5", "+5m", "dismiss", "dismiss"},
		}
		if snd := os.Getenv("WAYTICK_SOUND"); snd != "" {
			go func() {
				if err := exec.Command("paplay", snd).Run(); err != nil {
					log.Printf("waytick: sound: %v", err)
				}
			}()
		}
	default:
		return
	}
	d.queue(&n)
}

// queue hands a notification (nil = close) to the worker, in order.
func (d *Daemon) queue(n *notify.Notification) {
	select {
	case d.notes <- n:
	default:
		log.Printf("waytick: notification queue full, dropped one")
	}
}

// notifyWorker sends notifications off the timer lock so a slow or absent
// notification server never stalls ticking.
func (d *Daemon) notifyWorker() {
	for n := range d.notes {
		if n == nil {
			d.notif.Close()
			continue
		}
		if err := d.notif.Send(*n); err != nil {
			log.Printf("waytick: notify: %v", err)
		}
	}
}

func (d *Daemon) onAction(key string) {
	if key != "add5" {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	now := time.Now()
	evs, err := d.t.Add(now, 5*time.Minute)
	if err != nil {
		log.Printf("waytick: +5m action: %v", err)
		return
	}
	d.step(now, evs)
}

func (d *Daemon) apply(req Request) (err error) {
	now := time.Now()
	var evs []timer.Event
	switch req.Cmd {
	case "start":
		evs, err = d.t.Start(now, req.Total, req.StartAt)
	case "pause":
		err = d.t.Pause(now)
	case "resume":
		err = d.t.Resume(now)
	case "toggle":
		err = d.t.Toggle(now)
	case "cancel":
		d.t.Cancel()
		d.queue(nil)
	case "add":
		evs, err = d.t.Add(now, req.Dur)
	case "status":
	default:
		return fmt.Errorf("unknown command %q", req.Cmd)
	}
	if err != nil {
		return err
	}
	d.step(now, evs)
	return nil
}

func (d *Daemon) serve(c net.Conn) {
	defer c.Close()
	var req Request
	if err := json.NewDecoder(bufio.NewReader(c)).Decode(&req); err != nil {
		return
	}
	if req.Cmd == "watch" {
		d.watch(c)
		return
	}
	d.mu.Lock()
	err := d.apply(req)
	resp := Response{OK: err == nil, Timer: d.t, Output: waybar.Render(&d.t, time.Now())}
	d.mu.Unlock()
	if err != nil {
		resp.Error = err.Error()
	}
	json.NewEncoder(c).Encode(resp)
}

func (d *Daemon) watch(c net.Conn) {
	ch := make(chan []byte, 4)
	d.mu.Lock()
	d.subs[ch] = struct{}{}
	first := waybar.Render(&d.t, time.Now()).JSON()
	d.mu.Unlock()
	defer func() {
		d.mu.Lock()
		delete(d.subs, ch)
		d.mu.Unlock()
	}()

	// Detect the client going away even while the timer is idle.
	gone := make(chan struct{})
	go func() {
		buf := make([]byte, 1)
		for {
			if _, err := c.Read(buf); err != nil {
				close(gone)
				return
			}
		}
	}()

	line := first
	for {
		if _, err := c.Write(append(line, '\n')); err != nil {
			return
		}
		select {
		case line = <-ch:
		case <-gone:
			return
		}
	}
}
