# Changelog

## Unreleased — power modes

### New

- **Three-mode power state machine** (`watchdog.Mode`): Normal /
  LowPower / Paused. Battery → Paused unconditionally; AC +
  power-saver profile → policy-driven via `watchdog.power_saver_policy`
  (`reduce` (default) → LowPower; `pause` → Paused; `ignore` → Normal).
- **Per-mode backend tuning**: `mpvpaper.battery_mpv_opts` is appended
  to `mpv_opts` in LowPower (so user base flags survive); `wpe.fps_battery`
  replaces `fps`. Defaults add hwdec + reduced cache + display-vdrop
  for mpv and 15 fps for lwpe.
- **Backend niceness**: `mpvpaper.nice` and `wpe.nice` (default 10)
  apply via `setpriority(PRIO_PGRP)` after spawn — catches CEF
  helpers and decoder threads, not just the main process.
- **ppd integration**: `internal/power` wraps `powerprofilesctl get`
  with a 2s timeout; missing tool falls back gracefully to AC/battery
  detection.
- **Web-UI power surface**: `GET /api/power` (mode/reason/profile/
  policy/last_applied), `POST /api/power/{pause,resume}`. Status
  badge + Pause/Resume controls in the UI.
- **Pending-intent state**: paused-mode Apply queues to
  `pending.json` separately from `last.json`. Watchdog and
  `wallforge resume` consume pending first, fall back to last.

### Changed

- `apply.ByInput` auto-detects the current mode and returns
  `ErrPaused` (typed error) when refusing to render. Web-UI surfaces
  this as 202 Accepted — a queued intent is not a failure.
- Removed `mpvpaper -f` self-fork flag (was making `nice` a silent
  no-op). Detachment now via Setsid + Process.Release.

### Security

- CSRF guard on state-mutating endpoints (`/api/apply`, `/api/stop`,
  `/api/power/{pause,resume}`): rejects cross-site Sec-Fetch-Site
  and cross-host Origin headers; allows curl / native clients.
- `webui.New` warns to stderr when bound to non-loopback addr.

### Fixed

- Data race on `webui.Server.lastApplied` (concurrent /api/apply +
  /api/power reads).
- `state.Save` no longer overwrites `last.json` when Apply is
  refused due to Paused mode.
- `setpriority` no longer fires against the wrong PID for mpvpaper
  (parent exited within milliseconds, leaving the daemonized child
  at default nice).

## 0.1.0-alpha — 2026-04-24

Feature-complete alpha. Everything in the roadmap up to "per-monitor
bindings" is shipped; the broad strokes below capture it.

### New

- **swww / awww backend** for static images (`.png .jpg .jpeg .webp
  .bmp`). Daemon lifecycle managed by wallforge; transition type and
  duration configurable.
- **mpvpaper backend** for video and animated wallpapers (`.mp4
  .webm .mkv .mov .gif`). Detached via `setsid`; prior instances
  killed by matching `/proc/<pid>/exe`.
- **linux-wallpaperengine backend** for Wallpaper Engine scene and web
  content. In-tree Nix derivation (`nix/linux-wallpaperengine.nix`)
  builds end-to-end including CEF pre-fetch.
- **Steam Workshop integration** — reads the local Steam cache
  directly, no proxy services. `wallforge apply <id>` resolves
  numeric IDs against `steamapps/workshop/content/431960/`.
  Flatpak Steam paths are auto-detected.
- **Local library index** — `config.library.roots` directories are
  scanned (depth-4, bounded) and merged with Steam items in the
  web-UI.
- **Web-UI** (`wallforge serve`) — `net/http` + `//go:embed` static
  bundle. REST surface:
  - `GET /api/items` (Workshop subscriptions)
  - `GET /api/library` (scanned local items)
  - `GET /api/status`
  - `GET /preview/{id}` (both sources, zip-slip-safe)
  - `POST /api/apply`, `POST /api/stop`
- **Per-workspace daemon** (`wallforge workspace daemon`) — listens
  on Hyprland's `.socket2`, applies bound wallpapers on workspace
  switch. Bindings persist in `$XDG_STATE_HOME/wallforge/workspaces.json`.
- **Battery watchdog** (`wallforge watchdog`) — polls
  `/sys/class/power_supply/BAT*/status`, stops backends on battery
  and resumes them on AC.
- **Resume on login** (`wallforge resume`) — re-applies the last
  wallpaper from `$XDG_STATE_HOME/wallforge/last.json`.
- **Shuffle** (`wallforge shuffle`) — playlist rotation by type or
  explicit ID list, configurable interval (Go duration syntax).
- **Shell completion** (`wallforge completion <bash|zsh|fish>`).
- **Home Manager module** — `programs.wallforge.enable` plus opt-in
  flags for each daemon (`serve`, `resume`, `workspace`, `watchdog`,
  `shuffle`). Nixpkgs overlay exposes `pkgs.wallforge` and
  `pkgs.linux-wallpaperengine`.
- **Arch PKGBUILD** (`pkg/arch/PKGBUILD`) for AUR distribution.

### Fixed during alpha cycle

- `engine.StopOthers` clears competing layer surfaces before Apply
  (otherwise an image rendered over a live mpvpaper stayed invisible
  behind the video).
- Preview `background-size: cover` as an explicit property instead of
  the shorthand — previous shorthand was reverting to spec default
  when `.style.backgroundImage` was set inline.
- "No preview" placeholder via `Image()` probe — previews that fail
  to load show a labeled placeholder instead of a black rectangle.
- `nixpkgs.swww → awww` runtime dep (silenced eval warning).
- Module path renamed to `github.com/Vaniell0/wallforge` to match the
  GitHub profile for public distribution.

### Architecture notes

- Stdlib-only. Zero direct `go.mod` dependencies.
- Static binary (`CGO_ENABLED=0`, `-trimpath -ldflags='-s -w'`).
- `apply.ByInput` is the single entry point shared by CLI, web-UI,
  workspace daemon, watchdog, and shuffle. Overridable seams
  (`resolveSteam`, `selectBackend`, `stopOthers`, `saveState`) for
  testability.
- 10 packages × table-driven tests. Green on every commit.

### Known limitations

- No per-monitor bindings. Both swww and mpvpaper take `--output` /
  positional target, but wallforge currently passes a single value
  from config.
- `wallforge watchdog` polls `/sys/class/power_supply` instead of
  subscribing to UPower / power-profiles-daemon via DBus. Cheap and
  stdlib-only, but reacts with up to `Interval` latency (15 s default).
- `linux-wallpaperengine` upstream main branch is Xorg; wallforge
  ships against the Wayland fork. Scene content on multi-monitor
  setups picks the first output via `wpe.screen` — untested beyond
  single-monitor.
