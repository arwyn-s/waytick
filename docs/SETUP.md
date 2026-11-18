# Setting up waytick

This guide takes you from a fresh clone to a working timer pill in waybar. Every config snippet below is also in [`examples/`](../examples).

## 1. Requirements

| | Needed for |
|---|---|
| Go ≥ 1.24 | building |
| systemd (user session) | running the daemon |
| waybar | the pill |
| rofi (≥ 1.7, Wayland build) | the input prompt |
| a notification daemon (dunst, mako, swaync, …) | alerts |
| `libnotify` (`notify-send`) | error messages from the rofi script |
| `paplay` (pulseaudio-utils / pipewire-pulse) | optional finish sound |

Arch: `sudo pacman -S go go-task waybar rofi dunst libnotify` (Arch ships the binary as `go-task`; alias it to `task` or use `go-task` below)

## 2. Build and install

```sh
git clone <repo> waytick && cd waytick
task test      # optional
task install
```

`task install` does the following:

| What | Where |
|---|---|
| installs the `waytick` binary | `~/.local/bin/` |
| installs the rofi launcher `waytick.sh` | `~/.config/rofi/launchers/` (override with `ROFIDIR=...`) |
| installs the user service `waytick.service` | `~/.config/systemd/user/` |
| enables and starts the service | it's tied to `graphical-session.target`, so it starts with your session |

To check it worked:

```sh
systemctl --user status waytick   # active (running)
waytick status                    # idle
```

> **Compositor without `graphical-session.target`?** (Check with `systemctl --user is-active graphical-session.target`.) Change `WantedBy=` in the unit to `default.target`, or start the daemon from your compositor's autostart: `exec-once = ~/.local/bin/waytick daemon`.

## 3. Waybar module

Use [`examples/waybar-module.jsonc`](../examples/waybar-module.jsonc).

1. Add `"custom/waytick"` to a module list, e.g. `modules-right`.
2. Paste the `"custom/waytick": {...}` block into your config.
3. Don't add `interval`. `waytick watch` streams updates by itself, once per second while a timer is active.

| Mouse | Action |
|---|---|
| left click | rofi menu |
| middle click | pause / resume |
| right click | cancel |
| scroll up | +1 minute (starts a 1m timer when idle) |

## 4. Styling

Paste [`examples/waybar-style.css`](../examples/waybar-style.css) into `~/.config/waybar/style.css`. If your theme already defines the `@define-color` names at the top, drop the duplicates.

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

## 5. Keybind

| Compositor | Sample |
|---|---|
| Hyprland (Lua config) | [`examples/hyprland.lua`](../examples/hyprland.lua) |
| Hyprland (classic conf) | [`examples/hyprland.conf`](../examples/hyprland.conf) |
| sway | [`examples/sway.conf`](../examples/sway.conf) |

The default is `SUPER+ALT+T`, which opens the rofi prompt. Reload with `hyprctl reload` / `swaymsg reload`.

## 6. Notifications (optional tuning)

waytick sends its notifications under the appname `waytick`, and they all share a single slot, so each new one replaces the last.

| Event | Urgency |
|---|---|
| scheduled | low |
| started | normal |
| 10% left | normal |
| done | critical, never expires, with a **+5m** action button |

[`examples/dunstrc`](../examples/dunstrc) has rules that:
- let waytick through while dunst is in do-not-disturb. dunst only does this for *partial* pause levels, so use `dunstctl set-pause-level 90` instead of `set-paused true`.
- make the done alert red.

## 7. Sound (optional)

```sh
systemctl --user edit waytick
```

Paste the contents of [`examples/waytick.service.d-override.conf`](../examples/waytick.service.d-override.conf), then run:

```sh
systemctl --user restart waytick
```

## 8. Using it

**From the rofi prompt** (`SUPER+ALT+T` or click the pill):

| Type | Result |
|---|---|
| `25m` | starts now |
| `25m @+1m` | starts in 1 minute |
| `1h30m @14:00` | starts at 14:00 (tomorrow if that time has already passed) |

Durations use Go syntax: `45s`, `25m`, `1h30m`, `1m30s`. A bare `25` is rejected. Set `WAYTICK_ROFI_THEME` to use a different rofi theme (the default is `launcher`).

**From a terminal**: the command returns at once, so you can close the terminal straight away.

```sh
waytick start 25m --in 1m
waytick pause | resume | toggle | cancel
waytick add 5m
waytick status          # e.g. "running 󰔛 12:34"
```

## 9. Troubleshooting

| Symptom | Fix |
|---|---|
| `daemon not running` | `systemctl --user start waytick`; logs: `journalctl --user -u waytick` |
| Pill never appears | Is `custom/waytick` in a modules list? Run `~/.local/bin/waytick watch` in a terminal and check JSON is printed |
| No notifications | Is a notification daemon running? `notify-send test` |
| Done alert hidden during DND | See section 6 |
| Timer state looks wrong | `waytick cancel`, or delete `$XDG_RUNTIME_DIR/waytick/state.json` and restart the service |

## Uninstall

```sh
task uninstall
```

Then remove the waybar module, the CSS and the keybind you added.
