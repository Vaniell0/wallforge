# Backends

Wallforge dispatches to one of three external tools. Each is a separate
process wallforge owns; the `Backend` interface is three methods:

```go
type Backend interface {
    Name() string
    Apply(path string) error
    Stop() error
}
```

`engine.Detect(path)` classifies content by extension or
`project.json`, `engine.Select(target, cfg)` picks the backend,
`engine.StopOthers(keep, cfg)` clears competing layer surfaces before
the chosen backend paints.

## swww / awww — static images

| Extensions | `.png` `.jpg` `.jpeg` `.webp` `.bmp`          |
|------------|-----------------------------------------------|
| Binary     | `awww` + `awww-daemon` (nixpkgs) or `swww` + `swww-daemon` (other distros) |
| Process    | Long-lived daemon, CLI talks to it over a Unix socket |
| Stop       | `awww kill`                                    |

### How wallforge uses it

`Swww.Apply(path)`:

1. `ensureDaemon()` — `awww query` succeeds → daemon is up, return.
   Otherwise `awww-daemon` is spawned and polled for readiness (50 ms
   tick, 3 s timeout).
2. `awww img --transition-type <cfg.swww.transition> --transition-duration <cfg.swww.duration> <path>`.

The daemon keeps the image loaded; the cost is a few MB of RAM while
idle. No GPU use once the transition finishes.

### Why swww was renamed to awww in nixpkgs

Upstream swww was dormant for a while; the Nix maintainers forked it
as `awww` to keep the version current. The CLI and daemon protocol are
identical — only the binary name changed. A symlink `awww → swww` (or
vice versa) works in either direction if you want a uniform
configuration across machines.

## mpvpaper — video and animated content

| Extensions | `.mp4` `.webm` `.mkv` `.mov` `.gif`           |
|------------|-----------------------------------------------|
| Binary     | `mpvpaper`                                    |
| Process    | One mpvpaper process per output, detached via `setsid` |
| Stop       | `procutil.killByExeName("mpvpaper")` — matches `/proc/<pid>/exe` basename |

### How wallforge uses it

`Mpvpaper.Apply(path)`:

1. Kill any existing mpvpaper instances (same tool, same output would
   leak GPU memory).
2. `mpvpaper -o "<cfg.mpvpaper.mpv_opts>" <cfg.mpvpaper.target> <path>`
3. `cmd.Process.Release()` + `SysProcAttr{Setsid: true}` — detach from
   wallforge so the child lives past CLI exit, in its own session and
   process group.
4. `setNicePGroup(pid, cfg.mpvpaper.nice)` — `setpriority(PRIO_PGRP)`
   on the child's pgid lowers every helper thread mpv forks (decoder,
   GL submitter, etc.).

We deliberately omit mpvpaper's `-f` self-fork flag. With `-f`, the
short-lived parent we niced exits within milliseconds and the
daemonized child inherits default priority — the niceness adjustment
becomes a silent no-op. Letting mpvpaper run "in foreground" plus our
own Setsid + Release achieves the same detachment without losing the
priority change.

### The `killByExeName` dance

`pkill -x mpvpaper` doesn't work on NixOS because Nix wraps binaries
via `makeWrapper`, so the running process is `.mpvpaper-wrapped`, not
`mpvpaper`. Worse, `/proc/<pid>/comm` is truncated to 15 chars
(`.mpvpaper-wrapp`).

The fix is in `internal/procutil` — read `/proc/<pid>/exe` symlink,
take its basename, compare against the exact target name. Works on any
distro, wrapped or not.

### Performance notes

- Default `--panscan=1.0` crops rather than letterboxes for
  aspect-ratio mismatches. Drop to `0` if you want bars preserved.
- Add `--hwdec=auto` to `mpv_opts` for hardware decode. High-resolution
  videos without it will burn a core.
- Default `--loop-file=inf` loops forever. Wallpaper semantics, not
  playback.
- `cfg.mpvpaper.nice` (default 10) lowers the process-group priority
  so foreground stays responsive on a CPU-bound build.
- `cfg.mpvpaper.battery_mpv_opts` is appended in LowPower mode (AC +
  power-saver profile). Default adds `--hwdec=auto --cache-secs=2
  --video-sync=display-vdrop` to whatever your base `mpv_opts` is.

## linux-wallpaperengine — scene / web Wallpaper Engine content

| Detection  | Directory with a `project.json`, `type` = `scene` or `web` |
|------------|------------------------------------------------------------|
| Binary     | `linux-wallpaperengine` (Almamu; nixpkgs derivation lives in-tree at `nix/linux-wallpaperengine.nix`) |
| Process    | One long-lived process; killed and respawned on each Apply |
| Stop       | `procutil.killByExeName("linux-wallpaperengine")`         |

### Architecture

Wallpaper Engine "scene" content is a Unity-like mini scene (particles,
shaders, layered sprites). "Web" content is an HTML bundle rendered
via Chromium Embedded Framework. Both are driven by Almamu's
`linux-wallpaperengine`, which embeds a CEF runtime (~200 MB
uncompressed).

### The Nix story

`nix/linux-wallpaperengine.nix`:

1. Pre-fetches CEF 135.0.17 as a separate derivation (Nix can't pin a
   live download during build, and upstream's CMake wanted to fetch
   it on the fly).
2. Patches `CMakeModules/DownloadCEF.cmake` to point at the pre-fetched
   path.
3. `auto-patchelf` fixes every RPATH in the CEF blob and the main
   binary.
4. `makeWrapper` exposes the right `LD_LIBRARY_PATH` + `--assets-dir`.

End result: `nix build .#linux-wallpaperengine` produces
`result/bin/linux-wallpaperengine` that Just Works.

### Performance notes

- `fps: 30` is the default. Lower (`fps: 15`) doesn't obviously
  degrade perceived smoothness but halves energy.
- `cfg.wpe.fps_battery` (default 15) replaces `fps` in LowPower mode.
  lwpe's `--fps` is a single scalar so this substitutes, not appends.
- `silent: true` disables scene audio — wallpaper.
- `screen: ""` picks the first output. Set to `eDP-1` etc. to pin it.
- `cfg.wpe.nice` (default 10) is applied via `PRIO_PGRP` after spawn
  so it catches every CEF helper (renderer, gpu, utility) — these
  forks happen right after launch and would otherwise stay at default
  priority.

## Why stopping others matters

An image rendered by swww and a video rendered by mpvpaper are both
Wayland *layer surfaces*. The compositor stacks them. If you run swww
over an already-playing mpvpaper, the user typically sees mpvpaper
still on top — the image technically applied but the video surface
keeps painting in front.

`engine.StopOthers(keep, cfg)` runs before every Apply. It swallows
individual backend Stop errors because "nothing to stop" is the common
case, not a problem.

## Adding a new backend

1. Implement the `Backend` interface in `internal/engine/<yourname>.go`.
2. Add a `Kind` constant for the content type.
3. Wire detection:
   - If extension-based, add to `detectFile` in `engine.go`.
   - If directory-based, extend `detectDir`.
4. Wire selection in `engine.Select`.
5. Add a `XxxConfig` type to `internal/config/config.go` and a field
   on `Config`. Default in `Default()`.
6. Add the runtime dep to `flake.nix` `runtimeDeps`.
7. Table tests in `internal/engine/engine_test.go`.

Keep the backend struct small. Long shell-out lists go into helper
functions, not config structs.
