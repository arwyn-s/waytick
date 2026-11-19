# waytick

A countdown timer for [waybar](https://github.com/Alexays/Waybar): a small Go daemon, a rofi prompt to enter times, a coloured waybar pill, and desktop notifications.

- One timer at a time: start, pause/resume, extend, cancel
- Delayed start (`--in 1m`) or a start at a set time (`--at 14:30`)
- Notifications when the timer is scheduled, when it starts, at **10% remaining**, and when it's **done**. The done notification has a `+5m` action.
- The pill colour changes with state and progress
- State is kept in `$XDG_RUNTIME_DIR/waytick/state.json` with absolute timestamps, so it survives a daemon restart, a waybar reload, closing the terminal, and suspend

## Install

### Requirements

| | Needed for |
|---|---|
| systemd (user session) | running the daemon |
| waybar | the pill |
| rofi (≥ 1.7, Wayland build) | the input prompt |
| a notification daemon (dunst, mako, swaync, …) | alerts |
| `libnotify` (`notify-send`) | error messages from the rofi script |
| `paplay` (pulseaudio-utils / pipewire-pulse) | optional finish sound |

Arch: `sudo pacman -S waybar rofi dunst libnotify`

### From a release (no Go needed)

Download the release tarball, then install the binary, the rofi launcher and the systemd user service:

```sh
VER=0.1.0
curl -fsSL "https://github.com/arwyn-s/waytick/releases/download/v$VER/waytick_${VER}_linux_amd64.tar.gz" | tar -xz
cd "waytick_${VER}_linux_amd64"

install -Dm755 waytick         ~/.local/bin/waytick
install -Dm755 waytick-rofi    ~/.config/rofi/launchers/waytick.sh
install -Dm644 waytick.service ~/.config/systemd/user/waytick.service
systemctl --user daemon-reload
systemctl --user enable --now waytick
```

### With `go install` (Go ≥ 1.26.5)

```sh
GOBIN=~/.local/bin go install github.com/arwyn-s/waytick/cmd/waytick@latest

raw=https://raw.githubusercontent.com/arwyn-s/waytick/main
curl -fsSL --create-dirs -o ~/.config/rofi/launchers/waytick.sh    "$raw/scripts/waytick-rofi"
curl -fsSL --create-dirs -o ~/.config/systemd/user/waytick.service "$raw/dist/waytick.service"
chmod +x ~/.config/rofi/launchers/waytick.sh
systemctl --user daemon-reload
systemctl --user enable --now waytick
```

`GOBIN=~/.local/bin` matters: the service runs `~/.local/bin/waytick`.

Building from a clone instead? See [Building from source](#building-from-source).

### Check it worked

```sh
systemctl --user status waytick   # active (running)
waytick status                    # idle
```

| Installed | Where |
|---|---|
| `waytick` binary | `~/.local/bin/` |
| rofi launcher `waytick.sh` | `~/.config/rofi/launchers/` |
| user service `waytick.service` | `~/.config/systemd/user/`, tied to `graphical-session.target` so it starts with your session |

> **Compositor without `graphical-session.target`?** (Check with `systemctl --user is-active graphical-session.target`.) Change `WantedBy=` in the unit to `default.target`, or start the daemon from your compositor's autostart: `exec-once = ~/.local/bin/waytick daemon`.

## Setup

Every config snippet below is also in [`examples/`](examples).

### Waybar module

Use [`examples/waybar-module.jsonc`](examples/waybar-module.jsonc).

1. Add `"custom/waytick"` to a module list, e.g. `modules-right`.
2. Paste the `"custom/waytick": {...}` block into your config.
3. Don't add `interval`. `waytick watch` streams updates by itself, once per second while a timer is active.

| Mouse | Action |
|---|---|
| left click | rofi menu |
| middle click | pause / resume |
| right click | cancel |
| scroll up | +1 minute (starts a 1m timer when idle) |

### Styling

Paste [`examples/waybar-style.css`](examples/waybar-style.css) into `~/.config/waybar/style.css`. If your theme already defines the `@define-color` names at the top, drop the duplicates.

These are the classes on `#custom-waytick` you can style:

| Class | When |
|---|---|
| `scheduled` | waiting for `--in` / `--at` |
| `running` | counting down |
| `warning` | last 10% of the time |
| `paused` | paused |
| `done` | finished, shown for 10s and then the pill hides |
| `p0` … `p9` | elapsed progress in 10% steps (the sample warms from blue → yellow) |

When the timer is idle, the module's text is empty, so waybar hides the pill.

Reload with `pkill -SIGUSR2 waybar`.

### Keybind

| Compositor | Sample |
|---|---|
| Hyprland (Lua config) | [`examples/hyprland.lua`](examples/hyprland.lua) |
| Hyprland (classic conf) | [`examples/hyprland.conf`](examples/hyprland.conf) |
| sway | [`examples/sway.conf`](examples/sway.conf) |

The default is `SUPER+ALT+T`, which opens the rofi prompt. Reload with `hyprctl reload` / `swaymsg reload`.

### Notifications (optional tuning)

waytick sends its notifications under the appname `waytick`, and they all share a single slot, so each new one replaces the last.

| Event | Urgency |
|---|---|
| scheduled | low |
| started | normal |
| 10% left | normal |
| done | critical, never expires, with a **+5m** action button |

[`examples/dunstrc`](examples/dunstrc) has rules that:
- let waytick through while dunst is in do-not-disturb. dunst only does this for *partial* pause levels, so use `dunstctl set-pause-level 90` instead of `set-paused true`.
- make the done alert red.

### Sound (optional)

```sh
systemctl --user edit waytick
```

Paste the contents of [`examples/waytick.service.d-override.conf`](examples/waytick.service.d-override.conf), then run:

```sh
systemctl --user restart waytick
```

## Usage

**From the rofi prompt** (`SUPER+ALT+T` or click the pill): it opens presets, or you can type a duration. While a timer is running it shows pause/resume, +5m, cancel and new timer instead.

| Type | Result |
|---|---|
| `25m` | starts now |
| `25m @+1m` | starts in 1 minute |
| `1h30m @14:00` | starts at 14:00 (tomorrow if that time has already passed) |

Set `WAYTICK_ROFI_THEME` to use a different rofi theme (the default is `launcher`).

**From a terminal**: the command returns at once, so you can close the terminal straight away.

```sh
waytick start 25m              # Go durations: 45s, 25m, 1h30m, 1m30s (a bare 25 is rejected)
waytick start 25m --in 1m      # start in a minute
waytick start 10m --at 14:30   # start at 14:30 (tomorrow if already past)
waytick pause | resume | toggle | cancel
waytick add 5m                 # extend (starts a new timer when idle)
waytick status [--json]        # e.g. "running 󰔛 12:34"
```

## Troubleshooting

| Symptom | Fix |
|---|---|
| `daemon not running` | `systemctl --user start waytick`; logs: `journalctl --user -u waytick` |
| Pill never appears | Is `custom/waytick` in a modules list? Run `~/.local/bin/waytick watch` in a terminal and check JSON is printed |
| No notifications | Is a notification daemon running? `notify-send test` |
| Done alert hidden during DND | See [Notifications](#notifications-optional-tuning) |
| Timer state looks wrong | `waytick cancel`, or delete `$XDG_RUNTIME_DIR/waytick/state.json` and restart the service |

## Sample configs

| File | For |
|---|---|
| [`examples/waybar-module.jsonc`](examples/waybar-module.jsonc) | waybar module |
| [`examples/waybar-style.css`](examples/waybar-style.css) | pill colours per state/progress |
| [`examples/hyprland.lua`](examples/hyprland.lua) / [`hyprland.conf`](examples/hyprland.conf) / [`sway.conf`](examples/sway.conf) | keybind for the rofi prompt |
| [`examples/dunstrc`](examples/dunstrc) | DND passthrough + red done alert |
| [`examples/waytick.service.d-override.conf`](examples/waytick.service.d-override.conf) | finish sound |
| [`dist/waytick.service`](dist/waytick.service) | systemd user unit |

## Building from source

Needs Go ≥ 1.26.5 and [Task](https://taskfile.dev) (Arch: `sudo pacman -S go go-task`; Arch ships the binary as `go-task`, so alias it to `task` or type `go-task`).

```sh
git clone https://github.com/arwyn-s/waytick && cd waytick
task test      # optional
task install   # builds, installs the three files above, enables and (re)starts the service
```

Override paths with `task install PREFIX=... ROFIDIR=...`.

## Uninstall

From a clone: `task uninstall`. Otherwise:

```sh
systemctl --user disable --now waytick
rm -f ~/.local/bin/waytick ~/.config/rofi/launchers/waytick.sh ~/.config/systemd/user/waytick.service
systemctl --user daemon-reload
```

Then remove the waybar module, the CSS and the keybind you added.
