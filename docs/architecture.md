# Architecture

Wallforge is ~2k lines of Go plus a small HTML/JS/CSS SPA. This doc
explains how the pieces fit together for someone reading the code for
the first time.

## Package graph

```
          ┌─────────────────────┐
          │   cmd/wallforge     │
          │   (CLI entry)       │
          └──────────┬──────────┘
                     │ invokes
    ┌────────────────┼────────────────────────────────────┐
    │                │                                    │
    ▼                ▼                                    ▼
┌─────────┐    ┌──────────┐         ┌──────────────────────────┐
│ apply   │    │ webui    │         │ workspace / watchdog     │
│ ByInput │    │ serve    │         │ long-running daemons     │
└────┬────┘    └────┬─────┘         └─────────┬────────────────┘
     │              │                         │
     │              └─────calls apply.ByInput─┘
     │
     ▼
┌────────────────────────────────────────────────────────┐
│ engine                                                 │
│   Kind, Target, Backend interface                      │
│   Detect(path)          — file/dir → Target            │
│   Select(target, cfg)   — Target → Backend             │
│   StopAll / StopOthers  — clear layer surfaces         │
│                                                        │
│   Backends:  Swww    (awww)                            │
│              Mpvpaper                                  │
│              WallpaperEngine (linux-wallpaperengine)   │
└───┬──────────────────────────────────────────────────┬─┘
    │                                                  │
    ▼                                                  ▼
┌─────────┐                                       ┌─────────┐
│ steam   │                                       │ workshop│
│ Resolve │                                       │ ParseDir│
│ List    │◄───────────reads project.json─────────┤ Project │
└─────────┘                                       └─────────┘
```

Supporting packages (pure I/O, no cross-package calls):

- `config` — loads/saves `~/.config/wallforge/config.json`. `Config.ForLowPower()`
  returns a per-mode-tuned copy (battery_mpv_opts appended, fps_battery substituted).
- `state` — `last.json` is the actually-rendered wallpaper; `pending.json`
  is a paused-mode intent waiting for resume. `ConsumePending` atomically
  loads + clears so the same intent doesn't re-fire on every transition.
- `library` — walks `config.library.roots` and returns indexed items
- `workspace` — Hyprland `.socket2` event parser + bindings store in
  `$XDG_STATE_HOME/wallforge/workspaces.json`
- `power` — `powerprofilesctl get` wrapper with 2s timeout. Returns
  typed `Profile` enum; distinguishes `ErrNotInstalled` from `ErrTimeout`.
- `watchdog` — combined sysfs (AC/battery) + ppd (profile) detection.
  Three-mode FSM: `Normal` / `LowPower` / `Paused`. `EffectiveMode(snap, policy)`
  is a pure function; `Run` polls every 15s and dispatches `OnModeChange`.

## Data flow: `wallforge apply <input>`

```
 user ─── "wallforge apply 3682370294" ─────► main.cmdApply
                                                  │
                                                  ▼
                                           apply.ByInput
                                                  │
                                                  ▼
                                          watchdog.EffectiveMode
                                                  │
                              ┌───────────────────┼───────────────────┐
                              │                   │                   │
                          Paused              LowPower             Normal
                              │                   │                   │
                              ▼                   │                   │
                       state.SavePending   cfg.ForLowPower()           │
                       return ErrPaused           │                    │
                                                  ▼                    ▼
                                           (effective cfg)       (raw cfg)
                                                  │                    │
                                                  └──────────┬─────────┘
                                                             ▼
                                                      engine.Detect
                                                             │
                                                  file: by extension
                                                  dir : workshop.ParseDir
                                                             │
                                                             ▼
                                                      engine.Select
                                                             │
                                                             ▼
                                                  engine.StopOthers
                                                             │
                                                             ▼
                                                      backend.Apply
                                                             │
                                                             ▼
                                                      state.Save
```

The watchdog calls `apply.ByInputForMode` instead of `ByInput` to skip
the redundant ppd probe — it already computed the mode for its own
dispatch decision.

## Data flow: web-UI apply

Same pipeline, different entry:

```
 browser click ── POST /api/apply {input} ─► webui.handleApply
                                                   │
                                        "lib_..."  │  else
                                     translate     │
                                 id → server cache │
                                                   ▼
                                            apply.ByInput
                                                   │
                                                  ...same as CLI
```

The library ID → path map is populated by `GET /api/library` and lives
on the server; a client that never scans can't trigger an apply of an
arbitrary filesystem path.

## Overridable seams

`apply.ByInput` exposes package-level function variables so tests can
stub each side-effect independently:

| Var                | Production default     | Purpose                                    |
|--------------------|------------------------|--------------------------------------------|
| `resolveSteam`     | `steam.Resolve`        | numeric ID → path                          |
| `selectBackend`    | `engine.Select`        | Target → Backend                           |
| `stopOthers`       | `engine.StopOthers`    | clear other layers                         |
| `saveState`        | `state.Save`           | persist last-applied (`last.json`)         |
| `savePendingState` | `state.SavePending`    | persist paused-mode intent (`pending.json`)|
| `detectMode`       | live ppd + sysfs probe | resolve current power Mode                 |

`internal/apply/apply_test.go` overrides everything in `TestMain` so
no test accidentally writes to `$XDG_STATE_HOME`, shells out to
backends, or consults the host's real power state. Tests that need a
specific mode use `stubMode(watchdog.ModeLowPower)` etc.

## Power modes

Wallforge tracks one of three operating modes computed from two
signals:

| Signal       | Source                                            |
|--------------|---------------------------------------------------|
| AC / battery | `/sys/class/power_supply/BAT*/status`             |
| ppd profile  | `powerprofilesctl get` (2s timeout, optional)     |

`watchdog.EffectiveMode(snapshot, policy)` is the pure function that
collapses both into one of:

| Mode       | When                                                                |
|------------|---------------------------------------------------------------------|
| `Normal`   | AC + (performance / balanced / unknown)                             |
| `LowPower` | AC + power-saver, when `power_saver_policy = "reduce"` (default)    |
| `Paused`   | Battery (always) — or AC + power-saver under `policy = "pause"`     |

`policy = "ignore"` collapses LowPower into Normal — useful if you
explicitly want wallpapers running unchanged in power-saver.

Mode flips drive `apply.ByInputForMode`:

- **Normal** → render with raw config.
- **LowPower** → render with `cfg.ForLowPower()`. `mpvpaper.battery_mpv_opts`
  is appended to `mpv_opts`; `wpe.fps_battery` replaces `wpe.fps`.
- **Paused** → don't render; if a user requested an Apply, queue it
  to `state.SavePending` (separate file from `last.json`). The next
  resume picks up pending first, falling back to last on miss.

`/api/power` exposes the current mode, reason, profile, and policy
to the web-UI; the front-end paints a status badge and Pause/Resume
buttons.

## Why stdlib-first

Every runtime concern is handled with the standard library:

- HTTP: `net/http` + `ServeMux` (Go 1.22 method+path patterns)
- JSON: `encoding/json` — no `mapstructure`, no validation lib
- Flags: `flag` (plus handwritten shell completion, no `cobra`)
- Embed: `//go:embed` for the web-UI bundle
- Concurrency: `context.Context` + `sync.Mutex`, no framework
- Hyprland IPC: raw `net.Dial("unix")` + `bufio.Scanner`
- DBus: **not used** — watchdog polls `/sys/class/power_supply/BAT*/status`

The whole binary is a single static Go executable (~9 MB stripped,
`CGO_ENABLED=0`). External dependencies are:

- `go.mod` — zero direct deps
- Runtime: the three wallpaper backends (`awww`, `mpvpaper`,
  `linux-wallpaperengine`). Not linked, shell-invoked.

## Systemd user service shape

All long-running components ship as identical `After`/`PartOf`
`graphical-session.target` units. Starting with Home Manager options:

```
programs.wallforge.enable           — just installs the binary + config
programs.wallforge.serve.enable     — wallforge-serve.service       (web-UI)
programs.wallforge.resume.enable    — wallforge-resume.service      (oneshot on login)
programs.wallforge.workspace.enable — wallforge-workspace.service   (Hyprland daemon)
programs.wallforge.watchdog.enable  — wallforge-watchdog.service    (battery poll)
programs.wallforge.shuffle.enable   — wallforge.service             (playlist rotation)
```

Each is opt-in. A bare `programs.wallforge.enable = true` just drops
the binary on `$PATH` with no daemons attached.
