# Agent Bridge (formerly Agent Bridge)

> **Universal Coding Agent Orchestrator & Multi-CLI Gateway**
> Unifies Google Antigravity (`agy`), Claude Code (`claude`), and OpenAI Codex (`codex`) into a streamlined, high-performance web interface.

---

## 🌟 Highlights
- **Single-Column Document Stream**: Linear turn-by-turn workflow mirroring Google Antigravity.
- **Sticky Pinned Questions**: Automatically locks active turn prompts to the viewport header; auto-updates upon scrolling; collapsed by default.
- **Tool Switcher & Aliasing**: Seamlessly switch between Antigravity, Claude Code, and Codex, with custom alias names (e.g., `Gemini Work`, `Claude Pro`).
- **Real-time Streaming**: Subprocess standard pipes with NDJSON streaming events, zero latency, and zero terminal I/O escape glitches.
- **Queue Management**: Enqueue subsequent prompts while an active agent is executing a task.
- **Zero Configuration DB Reader**: Directly parses local SQLite session histories and `transcript.jsonl` files from Antigravity.

---

## 📦 Install

### Docker (recommended for servers / NAS)
```bash
cp .env.example .env            # set AGENT_BRIDGE_TOKEN (openssl rand -hex 24) and WORKSPACE_DIR
docker compose up -d            # image: ghcr.io/nchungdev/agent-bridge (linux/amd64, linux/arm64)
# open http://localhost:8088/?token=<AGENT_BRIDGE_TOKEN>
```
The image contains Claude Code and Codex (`--build-arg INSTALL_AGENTS=false` for a slim image). Log in once from the
**Terminal** tab (`claude`, `codex login`); logins and session history live in named volumes. Antigravity (`agy`) is a
desktop product, so it is only available when Agent Bridge runs on the host.

### Prebuilt binary (desktop / laptop)
Download `agent-bridge_<version>_<os>_<arch>.tar.gz` from the Releases page (Linux and macOS, amd64/arm64), then:
```bash
./agent-bridge                  # http://127.0.0.1:8088 (override with AGENT_BRIDGE_PORT)
```
Install `tmux` to keep terminals and agents alive across restarts (without it they stop when the server stops).

### Desktop and mobile app (PWA)
Agent Bridge is an installable web app. Open it in Chrome, Edge or Safari and use **Install app** / **Add to Home Screen**:
it gets its own window and icon on desktop, and a full-screen app on iOS and Android. On a phone, reach the server
through HTTPS (a reverse proxy or Tailscale): browsers only install apps and use the clipboard on secure origins.

### Configuration
| Variable | Default | Meaning |
|---|---|---|
| `AGENT_BRIDGE_PORT` | `8080` (`8088` in the image) | listen port |
| `AGENT_BRIDGE_HOST` | `127.0.0.1` (`0.0.0.0` in the image) | bind address. A non-loopback address **requires** `AGENT_BRIDGE_TOKEN` |
| `AGENT_BRIDGE_TOKEN` | unset | require this token (`?token=` once, then a cookie, or `Authorization: Bearer`) |
| `AGENT_BRIDGE_DATA_DIR` | `~/.agent-bridge` | state and uploads |
| `AGENT_BRIDGE_WORKSPACE_ROOTS` | `$HOME` | colon-separated folders the UI may open |
| `AGENT_BRIDGE_TERMINAL` | on | `0` disables the terminal and its shell endpoints |
| `AGENT_BRIDGE_TMUX` | on | `0` runs terminals in-process instead of tmux |

> The terminal is a shell on the machine running Agent Bridge. Never expose it without the token and TLS.

**Running as a systemd service.** Terminals live in tmux, whose server is started by Agent Bridge and therefore sits in
the service's cgroup. By default systemd kills the whole cgroup on restart, which ends every terminal and agent. Keep
them with a drop-in (`/etc/systemd/system/agent-bridge.service.d/killmode.conf`):
```ini
[Service]
KillMode=process
```
then `systemctl daemon-reload`. The tmux socket is `$AGENT_BRIDGE_DATA_DIR/tmux.sock` (attach from a shell with
`tmux -S ~/.agent-bridge/tmux.sock attach -t <id>`), so it survives `PrivateTmp=true` as well.

---

## 🛠 Building Locally

### Requirements
- Node.js >= 20
- Go >= 1.26

### Commands
```bash
# Build both Web & Go binary
make build

# Run locally on port 8088
make run
```

---

## 🐳 Docker Build

```bash
docker build -t ghcr.io/nchungdev/agent-bridge:latest .
docker run -d -p 8088:8088 -v ~/.gemini:/root/.gemini ghcr.io/nchungdev/agent-bridge:latest
```

---

## 🌐 Standalone & Integrations
Agent Bridge is fully decoupled, standalone, and cross-platform. It runs independently on Linux, macOS, Windows, and in Docker containers without host-specific assumptions.
- **Standalone Binary / Docker**: Run directly via pre-built binaries or Docker images with automatic workspace discovery.
- **External Consumers (e.g. ClaraOS)**: Operating environments like ClaraOS can embed or include Agent Bridge via container images or binaries using its standard REST & WebSocket APIs, without any hardcoded coupling in this repository.

---

## v2: engine-agnostic sessions (`AGENT_BRIDGE_V2=1`)

A second transport that drives every CLI through one protocol, with real approvals, persistent sessions and mid-conversation engine switching. The classic (v1) UI is kept as-is; its chat now runs on this transport (the legacy `/ws` dispatcher is no longer used by the UI). The old input "mode" menu is now the **permission mode** (Ask / Plan / Auto-edit / Full access), enforced by the hub; model and effort are applied per conversation. Pre-v2 chats open read-only and continue on the new transport (their history is imported as context).

- `internal/core` – events, `Engine`/`Session` contracts, capabilities, session state machine (stdlib only).
- `internal/store` – shared context store in SQLite: append-only event log, bindings, approvals, working state, secret redaction.
- `internal/manager` – one long-lived process per (conversation, engine): bounded pool with LRU suspension, idle reaping, per-binding turn queue, policy-driven approvals ("allow for session"), restart recovery, engine switching with handoff.
- `internal/adapters/{claude,codex,agy,proc}` – Claude Code (`stream-json` + stdio permission prompts), Codex (`app-server` JSON-RPC), Antigravity (`stream-json`; no approval channel, so safety comes from `plan` / `accept-edits` modes). Each adapter translates its CLI's output into the common events.
- `internal/server/v2.go` – `/ws/v2` (same-origin only) and `/api/v2/*`.

Chat input: `/` opens the engine's slash commands and skills (Claude `initialize`, Codex `skills/list`, Antigravity skill folders; `GET /api/v2/engines/{id}/commands`). `!cmd` runs a shell command directly in the conversation workspace (no model, 120s limit, output capped) and the result is passed to the agent on its next turn; commands that escalate privileges ask for confirmation.

Sidebar conversations have a ⋮ / right-click menu (Pin, Mark as unread, Rename, Copy link, Fork, Move to group, Archive, Delete; keys P U R C F A D) with Pinned, group and Archived sections; links are `/#/c/<id>` (`GET /api/v2/convs`, `PATCH|DELETE /api/v2/convs/{id}`, `POST /api/v2/convs/{id}/fork`). The old "1 task running" banner is gone: the permission card only appears when approval is needed.

Rolling handoff summary: after every `AGENT_BRIDGE_SUMMARY_EVERY` finished turns (default 5, 0 disables) the hub updates a short summary in the background using the cheapest usable engine/model (Claude Haiku → Antigravity flash-low → Codex mini; signed-out or failing engines are skipped). It is incremental (previous summary + new activity, tool output clipped), stored in the conversation working state, and a fresh engine session starts from the summary plus only what happened after it, so a switch works even when the previous engine has run out of quota.

Provider quota (usage panel): real numbers only, read from each CLI — Claude from its `rate_limit_event` (captured from live sessions, or one tiny Haiku probe), Antigravity from `agy -p /quota`, Codex from the app-server `account/rateLimits/read` (`GET /api/v2/engines/{id}/quota`, cached 90s). Engines without a source show "unavailable" instead of an estimate.

Model lists come from the CLIs (`agy models`, Codex `model/list`) and are cached in `$DATA_DIR/models-cache.json` for 24h (fetched on first open, survives restarts); the model menu has a **Refresh models** button (`POST /api/v2/models/refresh`).

Configuration (environment): `AGENT_BRIDGE_V2=1`, `AGENT_BRIDGE_MAX_LIVE` (live CLI processes, default 2), `AGENT_BRIDGE_WORKSPACE_ROOTS` (colon-separated directories the GUI may read; default `$HOME`), `AGENT_BRIDGE_TOKEN` (optional shared secret: Bearer header or `hub_token` cookie via `/?token=…`), `AGENT_BRIDGE_TERMINAL=0` (disable the shell endpoints).

Engines must be logged in on the host. The v2 UI shows each engine's login state, refuses to start an unauthenticated one, and has a **Log in** button that drives `claude auth login` / `codex login --device-auth` (link, device code, pasted code) from the browser.

Tests: `go test ./...` (adapters are tested against scripted fake CLIs under `testdata/`). Design notes: [docs/agent-bridge-v2-plan.md](docs/agent-bridge-v2-plan.md).

Antigravity account switching uses AGY Manager’s saved `~/.gemini/profiles/profiles.json` and credential snapshots. Select a profile in Accounts or the tool menu; adding, renaming, and deleting these profiles remains in AGY Manager. A switch applies machine-wide, restarts `antigravity-cli-daemon.service` through `systemctl --user`, and suspends idle Agent Bridge AGY processes so they reload the new credentials. Running turns block switching. Credentials and the active profile are restored if the daemon restart fails. The hub must run as the same user as AGY Manager with access to that user’s systemd session.
