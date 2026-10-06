# Development

## Requirements

- Go 1.26 or newer (see `go.mod`)
- Node.js 22 and npm
- tmux, to exercise the persistent terminals (the tests skip what needs it when it is missing)

## Build and run

```sh
./build.sh            # builds the web UI, then the Go binary with the UI embedded: ./agent-bridge
./agent-bridge        # http://127.0.0.1:8088
```

A binary built this way is a development build (`agent-bridge version` prints `dev`).

For the UI with hot reload, run `npm run dev` in `web/` against a running server.

## Test

```sh
go vet ./... && go test ./...
cd web && npm run build      # type-checks (tsc -b) and bundles
```

`npx tsc --noEmit` alone checks nothing here because the root `tsconfig.json` only references the real configs;
use `npm run build`.

## Layout

| Path | What |
|---|---|
| `main.go`, `cli.go` | entry point and the `version`, `update`, `service` subcommands |
| `internal/server` | HTTP and WebSocket handlers, terminals, updater endpoints |
| `internal/bridge` | session discovery, handoff, remote access commands |
| `internal/adapters` | one adapter per agent CLI (`claude`, `codex`, `agy`) |
| `internal/accounts` | several logins per agent |
| `internal/manager`, `internal/core`, `internal/store` | conversations, engines and their persistence |
| `internal/update`, `internal/release`, `internal/version` | self-update, release tag rules, embedded version |
| `internal/service` | systemd and launchd service files |
| `web/` | React + Vite single-page app (also an installable PWA) |

## Release

See [versioning.md](versioning.md).
