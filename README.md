# waytick

A countdown timer for [waybar](https://github.com/Alexays/Waybar): a small Go daemon, a rofi prompt to enter times, a coloured waybar pill, and desktop notifications.

- One timer at a time: start, pause/resume, extend, cancel
- Delayed start (`--in 1m`) or a start at a set time (`--at 14:30`)
- Notifications when the timer is scheduled, when it starts, at **10% remaining**, and when it's **done**. The done notification has a `+5m` action.
- The pill colour changes with state and progress
- State is kept in `$XDG_RUNTIME_DIR/waytick/state.json` with absolute timestamps, so it survives a daemon restart, a waybar reload, closing the terminal, and suspend

## Install

```sh
task install   # ~/.local/bin/waytick, ~/.config/rofi/launchers/waytick.sh, systemd user service (enabled)
```

Then add the waybar module, CSS and a keybind. The full walkthrough is in **[docs/SETUP.md](docs/SETUP.md)**.

## Sample configs

| File | For |
|---|---|
| [`examples/waybar-module.jsonc`](examples/waybar-module.jsonc) | waybar module |
| [`examples/waybar-style.css`](examples/waybar-style.css) | pill colours per state/progress |
| [`examples/hyprland.lua`](examples/hyprland.lua) / [`hyprland.conf`](examples/hyprland.conf) / [`sway.conf`](examples/sway.conf) | keybind for the rofi prompt |
| [`examples/dunstrc`](examples/dunstrc) | DND passthrough + red done alert |
| [`examples/waytick.service.d-override.conf`](examples/waytick.service.d-override.conf) | finish sound |
| [`dist/waytick.service`](dist/waytick.service) | systemd user unit (installed by `task install`) |

## Usage

```sh
waytick start 25m              # Go durations: 45s, 25m, 1h30m, 1m30s
waytick start 25m --in 1m      # start in a minute
waytick start 10m --at 14:30   # start at 14:30 (tomorrow if already past)
waytick pause | resume | toggle | cancel
waytick add 5m                 # extend (starts a new timer when idle)
waytick status [--json]
```

The rofi launcher (`waytick.sh`) opens presets, or you can type `<dur> [@+delay | @HH:MM]`, e.g. `25m @+1m`. While a timer is running it shows pause/resume, +5m, cancel and new timer instead.
