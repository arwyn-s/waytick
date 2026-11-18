-- waytick: Hyprland keybind (Lua config, hyprland.lua).
-- Opens the rofi prompt: presets, or type "25m", "25m @+1m", "1h30m @14:00".
local mainMod = "SUPER"

hl.bind(mainMod .. " + ALT + T", hl.dsp.exec_cmd("~/.config/rofi/launchers/waytick.sh"))

-- Optional direct presets, no prompt:
-- hl.bind(mainMod .. " + ALT + P", hl.dsp.exec_cmd("~/.local/bin/waytick start 25m"))
-- hl.bind(mainMod .. " + ALT + SPACE", hl.dsp.exec_cmd("~/.local/bin/waytick toggle"))
