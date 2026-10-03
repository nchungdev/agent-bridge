# Nexus AI (formerly Agent Hub)

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

## 🛠 Building Locally

### Requirements
- Node.js >= 20
- Go >= 1.23

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
docker build -t ghcr.io/nchungdev/nexus-ai:latest .
docker run -d -p 8088:8088 -v ~/.gemini:/root/.gemini ghcr.io/nchungdev/nexus-ai:latest
```

---

## 📋 ClaraOS Integration
Nexus AI is integrated into ClaraOS via the App Catalog manifest (`nexus-ai.json`). ClaraOS does not contain this codebase directly—it pulls and provisions pre-built container images or binaries independently.

---

## v2: engine-agnostic sessions (`AGENT_HUB_V2=1`)

A second transport that drives every CLI through one protocol, with real approvals, persistent sessions and mid-conversation engine switching. The classic UI is unchanged; open **`/#/v2`** for the new view.

- `internal/core` – events, `Engine`/`Session` contracts, capabilities, session state machine (stdlib only).
- `internal/store` – shared context store in SQLite: append-only event log, bindings, approvals, working state, secret redaction.
- `internal/manager` – one long-lived process per (conversation, engine): bounded pool with LRU suspension, idle reaping, per-binding turn queue, policy-driven approvals ("allow for session"), restart recovery, engine switching with handoff.
- `internal/adapters/{claude,codex,agy,proc}` – Claude Code (`stream-json` + stdio permission prompts), Codex (`app-server` JSON-RPC), Antigravity (`stream-json`; no approval channel, so safety comes from `plan` / `accept-edits` modes). Each adapter translates its CLI's output into the common events.
- `internal/server/v2.go` – `/ws/v2` (same-origin only) and `/api/v2/*`.

Configuration (environment): `AGENT_HUB_V2=1`, `AGENT_HUB_MAX_LIVE` (live CLI processes, default 2), `AGENT_HUB_WORKSPACE_ROOTS` (colon-separated directories the GUI may read; default `$HOME`), `AGENT_HUB_TOKEN` (optional shared secret: Bearer header or `hub_token` cookie via `/?token=…`), `AGENT_HUB_TERMINAL=0` (disable the shell endpoints).

Engines must be logged in on the host. The v2 UI shows each engine's login state, refuses to start an unauthenticated one, and has a **Log in** button that drives `claude auth login` / `codex login --device-auth` (link, device code, pasted code) from the browser.

Tests: `go test ./...` (adapters are tested against scripted fake CLIs under `testdata/`). Design notes: [docs/agent-hub-v2-plan.md](docs/agent-hub-v2-plan.md).
