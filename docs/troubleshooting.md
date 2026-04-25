# Troubleshooting

Common failure modes and what to do about them.

## `wallforge list` says "No Wallpaper Engine workshop dir found"

Wallforge couldn't find your Steam install. Fix one of:

- Subscribe to Wallpaper Engine items in Steam first — an empty
  workshop dir may not exist at all.
- Set `steam.root` in config:
  ```json
  "steam": { "root": "/mnt/games/SteamLibrary" }
  ```
  The path is your Steam **library** root (the one containing
  `steamapps/`), not the Workshop dir itself.

Auto-detected paths, in order:

1. `~/.local/share/Steam`
2. `~/.steam/steam`
3. `~/.var/app/com.valvesoftware.Steam/data/Steam` (flatpak)
4. `~/.var/app/com.valvesoftware.Steam/.local/share/Steam`

## "I click an image in the web-UI and nothing happens"

Typically one of these:

- **A video/scene wallpaper is still rendering on top.** Fixed in
  `a1b2c3d` via `engine.StopOthers` — make sure you're on a binary
  after that commit (`wallforge version` and check against the git
  log).
- **The `wallforge-serve.service` is still running an old binary.**
  After `home-manager switch`:
  ```bash
  systemctl --user restart wallforge-serve
  ```
  Then hard-reload the browser tab (Ctrl+Shift+R) — old CSS/JS may
  be cached.
- **A backend error the UI didn't surface.** Tail the server logs:
  ```bash
  journalctl --user -u wallforge-serve -f
  ```
  Look for `wallforge:` lines on each request.

## "Video wallpaper pegs the CPU"

Add `--hwdec=auto` to `mpvpaper.mpv_opts`:

```json
"mpvpaper": {
  "mpv_opts": "no-audio --loop-file=inf --panscan=1.0 --hwdec=auto"
}
```

If that doesn't help, lower the video's resolution; mpvpaper decodes
whatever the file contains and layer-surface-blits it.

## "`wallforge serve` gives 404 on `/api/library`"

You're running an old binary. The library endpoint landed in
`4e7df31`. Rebuild:

```bash
cd /path/to/wallforge && nix build .#default
./result/bin/wallforge version
```

For Home Manager users: `nix flake lock --update-input wallforge &&
home-manager switch`.

## `wallforge workspace daemon` exits immediately

Usually one of:

- **`HYPRLAND_INSTANCE_SIGNATURE` is unset.** The daemon uses it to
  locate the Hyprland socket. Make sure you're running inside a
  Hyprland session.
- **The socket path doesn't exist.** Double-check:
  ```bash
  ls $XDG_RUNTIME_DIR/hypr/$HYPRLAND_INSTANCE_SIGNATURE/.socket2.sock
  ```
  If the directory exists but `.socket2.sock` doesn't, your Hyprland
  version may not expose event IPC — wallforge currently requires
  Hyprland 0.5x+.

For the systemd user unit:

```bash
systemctl --user status wallforge-workspace
journalctl --user -u wallforge-workspace -n 50
```

## "Workspace bindings don't trigger"

- Verify Hyprland is actually sending the event:
  ```bash
  socat - UNIX-CONNECT:$XDG_RUNTIME_DIR/hypr/$HYPRLAND_INSTANCE_SIGNATURE/.socket2.sock
  ```
  Switch workspaces; you should see `workspace>>…` lines.
- Verify the binding exists:
  ```bash
  wallforge workspace list
  ```
- Named workspaces use their name (`web`), numbered workspaces use
  the decimal ID as a string (`"1"`, not `1`).

## `wallforge watchdog` fires for no reason on a desktop

A desktop with no `/sys/class/power_supply/BAT*` returns `StateAC` and
the initial OnModeChange(Normal) callback fires once on start.
Expected. Subsequent ticks don't re-fire without a transition, so it's
a one-time `wallforge resume` on boot.

## "I switched to power-saver and the wallpaper didn't change"

Two possible reasons:

1. **The watchdog isn't running.** `programs.wallforge.watchdog.enable
   = true` in HM. Or invoke `wallforge watchdog` directly to test.
2. **You're on `power_saver_policy = "ignore"`.** Check via:
   ```bash
   curl -s http://127.0.0.1:7777/api/power | jq .power_saver_policy
   ```
   Default is `"reduce"` which drops into LowPower (re-applies with
   `battery_mpv_opts` + `fps_battery`). `"ignore"` is opt-in and
   keeps Normal-quality rendering.

The watchdog poll is 15s — wait at least that long after toggling
ppd before assuming it didn't work. Tail the journal to confirm:

```bash
journalctl --user -u wallforge-watchdog -f
```

You'll see `low-power (power-saver) — applying ...` lines on each
mode flip.

## "Web-UI says paused but I'm on AC"

`/api/power` reports two pause states:

- `mode == "paused"` — the watchdog says paused (battery, or
  power-saver under `policy = "pause"`).
- `user_paused == true` — you (or someone in another browser tab)
  clicked Pause manually.

These are independent. Manual pause survives until you hit Resume or
a successful Apply. Battery pause clears the moment AC returns.

## "powerprofilesctl is hanging"

ppd's D-Bus interface can lock up briefly during daemon restart. The
2s timeout in `internal/power/profile.go` keeps wallforge from getting
stuck — you'll see logs like:

```
wallforge watchdog: ppd probe failed: power: powerprofilesctl timed out: ...
```

Watchdog continues with `ProfileUnknown` (treated as no-opinion =
Normal) until the next probe succeeds. Restart ppd if it persists:

```bash
sudo systemctl restart power-profiles-daemon
```

## "The hero screenshot looks wrong on my fork"

The `docs/screenshots/hero.png` is committed as a binary asset. If
your fork has different content, regenerate:

```bash
# apply a scene wallpaper
wallforge apply <your-favorite-id>
# open a terminal, run:
wallforge list | head -20
# screenshot the whole desktop (grim on Wayland)
grim ~/wallforge-hero.png
# crop if you like, then:
cp ~/wallforge-hero.png /path/to/wallforge/docs/screenshots/hero.png
```

1920×1080 is the source size — lower resolutions scale fine in the
README preview.

## Preview thumbnails are blank for some library items

The `Image()` probe in `app.js` falls back to the `no preview`
placeholder when:

- The file is an unsupported format (HEIC, TIFF).
- The file path contains bytes the server refuses to encode (e.g.
  an invalid UTF-8 filename).
- The file was deleted between `/api/library` and the preview load.

Rename offending files or convert to PNG/JPG. The scan only respects
standard web extensions.

## Tests complain about `/home/*/.local/state/`

You ran `go test` without the Nix devShell and your `XDG_STATE_HOME`
is unset. Either enter the devShell (`nix develop`) or:

```bash
export XDG_STATE_HOME=$(mktemp -d)
go test ./...
```

`TestMain` in `internal/apply/apply_test.go` already stubs out state
saves for that package, but other packages (state, workspace) create
their own test dirs via `t.Setenv`.

## "I want per-monitor wallpapers"

Not supported yet. `swww` and `mpvpaper` both take a target output
(`--output`, `<target>`), but wallforge's `Apply` currently passes a
single value from config. Watch the [roadmap in README](../README.md#roadmap).
