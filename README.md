# Agent Bridge

Run **Claude Code**, **Codex** and **Antigravity** from one web UI, and switch between them in the middle of a task without losing context.

[![CI](https://github.com/nchungdev/agent-bridge/actions/workflows/ci.yml/badge.svg)](https://github.com/nchungdev/agent-bridge/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/nchungdev/agent-bridge)](https://github.com/nchungdev/agent-bridge/releases)
![Platforms](https://img.shields.io/badge/platforms-Linux%20%7C%20macOS-blue)

- **One place for every agent.** Sessions of all three CLIs, grouped by project, in one sidebar.
- **Switch without losing context.** Hand a half-finished task to another agent: Agent Bridge writes the recent conversation and the changed files to `.agent/handoff.md` and the next agent picks it up.
- **Terminals that survive.** Real terminals in the browser, kept alive by tmux across page reloads and restarts.
- **Any device.** Installable web app for desktop, tablet and phone. Self-hosted: it runs on your machine or server, and nothing leaves it.

## Install

**Linux and macOS** (one command; installs the binary and starts it as a background service):

```sh
curl -fsSL https://raw.githubusercontent.com/nchungdev/agent-bridge/main/install.sh | sh
```

Then open <http://127.0.0.1:8088>.

**Docker** (headless server or NAS):

```sh
docker run -d --name agent-bridge -p 127.0.0.1:8088:8088 \
  -e AGENT_BRIDGE_TOKEN="$(openssl rand -hex 24)" \
  -v agent-bridge-data:/data -v "$PWD":/workspace \
  ghcr.io/nchungdev/agent-bridge:latest
```

**From source**: see [docs/development.md](docs/development.md).

Agent Bridge drives the agent CLIs you already have, so install the ones you use (`claude`, `codex`, `agy`) and sign in to them. [tmux](https://github.com/tmux/tmux) is recommended for persistent terminals. Windows is not supported yet; on a phone, use the web app (below).

## Use

1. Open the UI and add a project folder.
2. Start a task with any agent, or resume a session you already have.
3. To change agent, press **Add** in the tab bar and pick another one. It continues from the handoff.
4. On a phone or tablet, open the same address and choose **Add to Home Screen**. Use HTTPS (a reverse proxy, Tailscale or Cloudflare Tunnel) so the browser allows installing and the clipboard.

## Update

```sh
agent-bridge update            # newest release from GitHub, then restarts the service
agent-bridge update --rollback # back to the previous version
```

You can also update from **Settings → Updates**. Terminals keep running through an update.

## Documentation

- [Configuration](docs/configuration.md): environment variables, running as a service, security
- [Remote access](docs/remote-access.md): reach a session from the agents' own web and mobile apps
- [Versioning and releases](docs/versioning.md): how builds are numbered and published
- [Development](docs/development.md): building, testing, project layout

## Security

The terminal is a shell on the machine running Agent Bridge. By default it listens on `127.0.0.1` only. To expose it, set `AGENT_BRIDGE_TOKEN` (the server refuses to listen on another address without one) and put TLS in front of it.

## Contributing

Issues and pull requests are welcome. Please run `go test ./...` and `npm run build` in `web/` before opening a pull request. Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/).
