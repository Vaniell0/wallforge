# Contributing

The project is stdlib-first Go with a tiny vanilla-JS front-end. It's
2 kLOC of Go and change. Any reader should be able to hold the whole
thing in their head.

## Development setup

### With Nix (recommended)

```bash
git clone https://github.com/Vaniell0/wallforge.git
cd wallforge
nix develop
```

The devShell ships Go 1.25, `gopls`, `golangci-lint`, `delve`, plus
runtime deps (`awww`, `mpvpaper`, `linux-wallpaperengine`).

### Without Nix

Install Go 1.25+. Install `swww`, `mpvpaper`, `linux-wallpaperengine`
through your package manager. Then:

```bash
go build ./cmd/wallforge
./wallforge version
```

## Loop

```bash
go test ./...              # full suite, must stay green
go vet ./...               # stdlib static checks
go run ./cmd/wallforge …   # run from source
nix build                  # reproducible binary, ./result/bin/wallforge
```

For web-UI work, `wallforge serve` serves the embedded `static/` from
the binary — restart the process after editing `internal/webui/static/`.

## Conventions

- **Stdlib-first.** New external deps need a paragraph in the PR
  description explaining why. `net/http`, `encoding/json`, `flag`,
  `os/exec`, `sync`, `context` are enough for every problem this
  codebase has hit so far.
- **Go style:** `gofmt -s`; lower-case short package names; no
  unnecessary interfaces; table-driven tests.
- **Comments:** only when the WHY is non-obvious — a hidden
  invariant, a workaround for a bug, a surprise. Self-documenting
  code beats extra commentary.
- **File scope:** keep `cmd/wallforge/*.go` thin; push logic into
  `internal/<package>/`. A new subcommand is one file in `cmd/` plus
  one package in `internal/`.
- **Commits:** imperative subject, 72 chars, no trailing period.
  Body wraps at 72 and explains the WHY. Every commit must compile
  and pass `go test ./...` on its own.

## Testing patterns

### Package-level seams

`apply.ByInput` exposes overridable function variables (`resolveSteam`,
`selectBackend`, `stopOthers`, `saveState`) so tests can swap out each
side-effect independently. Prefer this over DI frameworks — one-line
stub, no indirection in hot paths.

Pattern:

```go
var doStuff = otherPkg.DoStuff

func Foo() error {
    return doStuff()
}

// in _test.go
func TestFoo(t *testing.T) {
    prev := doStuff
    defer func() { doStuff = prev }()
    doStuff = func() error { return nil }
    // …
}
```

### TestMain for cross-cutting stubs

`internal/apply/apply_test.go` uses `TestMain` to disable state
persistence and `stopOthers` across the whole package. A test that
exercises real persistence should `t.Setenv("XDG_STATE_HOME", t.TempDir())`
locally.

### Temp directories

`t.TempDir()` returns a per-test cleanup-on-exit path. Use it for any
filesystem state (library scans, state files, workshop fixtures). Never
write into the test's CWD — that's shared.

### Hyprland / DBus / root devices

For stuff that requires a live session or system device node:

- Hyprland IPC: wrap the socket reader in a function that takes an
  `io.Reader`; tests feed `strings.NewReader(...)`.
- `/sys/class/power_supply`: `watchdog.detectIn(root)` takes the root
  path so tests can build a tmpfs tree.

No test should depend on real Hyprland, real Steam, real battery, or
real DBus being up.

## Adding a new subcommand

1. `cmd/wallforge/<name>.go` — `cmdFoo(cfg, args)` func with stdlib
   `flag.NewFlagSet` for subflags.
2. Wire into the `switch os.Args[1]` in `main.go`.
3. Add a line to `usage()` in `main.go`.
4. Add to `cmds=` in `bashCompletion`, `_describe` in `zshCompletion`,
   and `__fish_use_subcommand` in `fishCompletion` (all in
   `cmd/wallforge/completion.go`).
5. If the subcommand implies a long-running daemon, add a matching
   `programs.wallforge.<name>.enable` option in `module.nix` that
   generates a systemd user service with the same
   `After`/`PartOf=graphical-session.target` shape as the others.

## Adding a new backend

See [backends.md — Adding a new backend](backends.md#adding-a-new-backend).

## Release process

The project is pre-1.0; every commit on `main` is a rolling "alpha".
For a tagged release:

1. Bump `version` in `cmd/wallforge/main.go`.
2. Update `pkg/arch/PKGBUILD`'s `pkgver`.
3. Update the Roadmap in `README.md` — tick off shipped items.
4. Commit: `Release vX.Y.Z-<stage>`.
5. Tag: `git tag vX.Y.Z-<stage> -m "vX.Y.Z-<stage>"`.
6. Push with tags: `git push --tags`.

No prebuilt binaries — the flake, `PKGBUILD`, and `go install` cover
every target distribution wallforge supports.

## Security

Report security issues privately via GitHub security advisories on
[the repository](https://github.com/Vaniell0/wallforge/security).
Don't open a public issue.

The web-UI assumes the same-user trust model: it binds to `127.0.0.1`
by default and has no authentication. Any serious proposal to expose
it on a non-loopback interface needs an auth story.

## Code of conduct

Be decent. The [Contributor Covenant](https://www.contributor-covenant.org/)
applies in spirit; this project is too small for a formal document yet.
