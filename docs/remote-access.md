# Remote access

The web and mobile apps of each agent only see sessions that have **remote access switched on**, and each CLI
does it differently. The **Remote** button in the header (for the agent of the open tab) does the right thing for
that agent.

| Agent | How | Web app |
|---|---|---|
| Claude Code | `/remote-control` typed into the running session, or `--remote-control` for new ones | claude.ai/code and the Claude mobile app |
| Antigravity | the `agy remote-control` daemon (start, stop, status), or `--remote-control` for new sessions | none: the IDE |
| Codex | one shared daemon: `codex remote-control start`, then pair this machine with a code (`codex remote-control pair`) | chatgpt.com/codex |

The menu also has a switch, **Always on for new sessions**, which launches new Claude and Antigravity sessions with
remote access already enabled.

Switching remote access on registers this machine with your account at the provider, which is why it is never
turned on automatically. You can also use the CLIs directly; the button only runs the same commands.

API: `GET /api/bridge/remote`, `POST /api/bridge/remote/{agent}/{enable|disable|pair}`.
