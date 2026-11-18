// Package notify sends desktop notifications over the
// org.freedesktop.Notifications D-Bus interface (dunst, mako, swaync, ...).
package notify

import (
	"sync"

	"github.com/godbus/dbus/v5"
)

const (
	dest  = "org.freedesktop.Notifications"
	path  = "/org/freedesktop/Notifications"
	iface = "org.freedesktop.Notifications"
)

type Urgency byte

const (
	Low      Urgency = 0
	Normal   Urgency = 1
	Critical Urgency = 2
)

type Notification struct {
	Summary string
	Body    string
	Urgency Urgency
	// Actions are key/label pairs, e.g. {"add5", "+5m"}.
	Actions []string
	// Timeout in ms; -1 = server default, 0 = never expire.
	Timeout int32
}

// Notifier keeps one replaces_id so every notification for the timer reuses
// a single slot. It connects lazily and reconnects after errors, so the daemon
// can start before the notification server does.
type Notifier struct {
	AppName  string
	OnAction func(key string)

	mu   sync.Mutex
	conn *dbus.Conn
	id   uint32
}

func (n *Notifier) connect() (*dbus.Conn, error) {
	if n.conn != nil && n.conn.Connected() {
		return n.conn, nil
	}
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, err
	}
	if err := conn.AddMatchSignal(dbus.WithMatchInterface(iface), dbus.WithMatchMember("ActionInvoked")); err != nil {
		conn.Close()
		return nil, err
	}
	ch := make(chan *dbus.Signal, 8)
	conn.Signal(ch)
	go n.listen(ch)
	n.conn = conn
	return conn, nil
}

func (n *Notifier) listen(ch <-chan *dbus.Signal) {
	for sig := range ch {
		if sig.Name != iface+".ActionInvoked" || len(sig.Body) != 2 {
			continue
		}
		id, _ := sig.Body[0].(uint32)
		key, _ := sig.Body[1].(string)
		n.mu.Lock()
		ours := id == n.id
		n.mu.Unlock()
		if ours && n.OnAction != nil {
			n.OnAction(key)
		}
	}
}

func (n *Notifier) Send(nt Notification) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	conn, err := n.connect()
	if err != nil {
		return err
	}
	actions := nt.Actions
	if actions == nil {
		actions = []string{}
	}
	hints := map[string]dbus.Variant{"urgency": dbus.MakeVariant(byte(nt.Urgency))}
	call := conn.Object(dest, path).Call(iface+".Notify", 0,
		n.AppName, n.id, "alarm-clock", nt.Summary, nt.Body, actions, hints, nt.Timeout)
	if call.Err != nil {
		conn.Close()
		n.conn = nil
		return call.Err
	}
	return call.Store(&n.id)
}

// Close dismisses the current notification, if any.
func (n *Notifier) Close() {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.id == 0 || n.conn == nil {
		return
	}
	n.conn.Object(dest, path).Call(iface+".CloseNotification", 0, n.id)
}
