// waytick: a countdown timer for waybar.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/arwyn-s/waytick/internal/daemon"
	"github.com/arwyn-s/waytick/internal/waybar"
)

func main() {
	if err := cliApp(time.Now, call).Run(context.Background(), os.Args); err != nil {
		fmt.Fprintln(os.Stderr, "waytick:", err)
		os.Exit(1)
	}
}

// sender delivers a request to the daemon; swapped out in tests.
type sender func(daemon.Request) (daemon.Response, error)

func cliApp(now func() time.Time, send sender) *cli.Command {
	// simple builds a command that sends {Cmd: name} and nothing else.
	simple := func(name, usage string) *cli.Command {
		return &cli.Command{
			Name:  name,
			Usage: usage,
			Action: func(_ context.Context, _ *cli.Command) error {
				_, err := send(daemon.Request{Cmd: name})
				return err
			},
		}
	}
	inFlag := &cli.DurationFlag{Name: "in", Usage: "delay before starting", Validator: positive}
	atFlag := &cli.StringFlag{Name: "at", Usage: "start at the next `HH:MM`"}

	return &cli.Command{
		Name:                  "waytick",
		Usage:                 "a countdown timer for waybar",
		Description:           "Durations use Go syntax: 45s, 25m, 1h30m, 1m30s.",
		EnableShellCompletion: true,
		Commands: []*cli.Command{
			{
				Name:      "start",
				Usage:     "start a timer (replaces any current one)",
				ArgsUsage: "<dur>",
				MutuallyExclusiveFlags: []cli.MutuallyExclusiveFlags{
					{Flags: [][]cli.Flag{{inFlag}, {atFlag}}},
				},
				Action: func(_ context.Context, c *cli.Command) error {
					total, err := oneDur(c)
					if err != nil {
						return err
					}
					req := daemon.Request{Cmd: "start", Total: total}
					switch {
					case c.IsSet("in"):
						req.StartAt = now().Add(c.Duration("in"))
					case c.IsSet("at"):
						if req.StartAt, err = parseAt(c.String("at"), now()); err != nil {
							return err
						}
					}
					_, err = send(req)
					return err
				},
			},
			{
				Name:      "add",
				Usage:     "extend the timer (starts one if idle)",
				ArgsUsage: "<dur>",
				Action: func(_ context.Context, c *cli.Command) error {
					d, err := oneDur(c)
					if err != nil {
						return err
					}
					_, err = send(daemon.Request{Cmd: "add", Dur: d})
					return err
				},
			},
			simple("pause", "pause the timer"),
			simple("resume", "resume a paused timer"),
			simple("toggle", "pause or resume"),
			simple("cancel", "stop the timer"),
			{
				Name:  "status",
				Usage: `print the current state ("<status> <text>")`,
				Flags: []cli.Flag{&cli.BoolFlag{Name: "json", Usage: "print waybar JSON instead"}},
				Action: func(_ context.Context, c *cli.Command) error {
					resp, err := send(daemon.Request{Cmd: "status"})
					if err != nil {
						return err
					}
					if c.Bool("json") {
						os.Stdout.Write(append(resp.Output.JSON(), '\n'))
						return nil
					}
					fmt.Println(strings.TrimSpace(resp.Output.Alt + " " + resp.Output.Text))
					return nil
				},
			},
			{
				Name:  "watch",
				Usage: "stream waybar JSON (module exec)",
				Action: func(context.Context, *cli.Command) error {
					watch()
					return nil
				},
			},
			{
				Name:  "daemon",
				Usage: "run the server (systemd service)",
				Action: func(context.Context, *cli.Command) error {
					return daemon.Run()
				},
			},
		},
	}
}

// oneDur reads the single positional duration argument.
func oneDur(c *cli.Command) (time.Duration, error) {
	if c.NArg() != 1 {
		return 0, fmt.Errorf("usage: waytick %s %s", c.Name, c.ArgsUsage)
	}
	return parseDur(c.Args().First())
}

func positive(d time.Duration) error {
	if d <= 0 {
		return fmt.Errorf("duration %v must be positive", d)
	}
	return nil
}

// parseDur is time.ParseDuration restricted to positive values.
func parseDur(s string) (time.Duration, error) {
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, err
	}
	return d, positive(d)
}

// parseAt resolves HH:MM to the next occurrence: today, or tomorrow if that
// time has already passed.
func parseAt(s string, now time.Time) (time.Time, error) {
	hm, err := time.Parse("15:04", s)
	if err != nil {
		return time.Time{}, fmt.Errorf("--at %q: want HH:MM", s)
	}
	t := time.Date(now.Year(), now.Month(), now.Day(), hm.Hour(), hm.Minute(), 0, 0, now.Location())
	if !t.After(now) {
		t = t.AddDate(0, 0, 1)
	}
	return t, nil
}

func dial() (net.Conn, error) {
	c, err := net.DialTimeout("unix", daemon.SocketPath(), 2*time.Second)
	if err != nil {
		return nil, fmt.Errorf("daemon not running (systemctl --user start waytick): %w", err)
	}
	return c, nil
}

func call(req daemon.Request) (daemon.Response, error) {
	var resp daemon.Response
	c, err := dial()
	if err != nil {
		return resp, err
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(5 * time.Second))
	if err := json.NewEncoder(c).Encode(req); err != nil {
		return resp, err
	}
	if err := json.NewDecoder(c).Decode(&resp); err != nil {
		return resp, err
	}
	if !resp.OK {
		return resp, errors.New(resp.Error)
	}
	return resp, nil
}

// watch streams waybar lines forever, reconnecting while the daemon is down.
func watch() {
	idle := append(waybar.Output{Alt: "idle", Class: []string{"idle"}}.JSON(), '\n')
	w := bufio.NewWriter(os.Stdout)
	shownIdle := false
	for {
		c, err := dial()
		if err == nil {
			json.NewEncoder(c).Encode(daemon.Request{Cmd: "watch"})
			sc := bufio.NewScanner(c)
			for sc.Scan() {
				w.Write(sc.Bytes())
				w.WriteByte('\n')
				if w.Flush() != nil {
					os.Exit(0) // waybar went away
				}
				shownIdle = false
			}
			c.Close()
		}
		if !shownIdle {
			w.Write(idle)
			if w.Flush() != nil {
				os.Exit(0)
			}
			shownIdle = true
		}
		time.Sleep(2 * time.Second)
	}
}
