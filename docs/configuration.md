# Configuration

Agent Bridge is configured with environment variables. When it runs as a service, put them in
`~/.agent-bridge/agent-bridge.env` (one `NAME=value` per line, not committed anywhere) so you never edit the service
file; the service reads that file at start. Restart after changing it: `agent-bridge service restart`.

| Variable | Default | Meaning |
|---|---|---|
| `AGENT_BRIDGE_PORT` | `8088` | listen port |
| `AGENT_BRIDGE_HOST` | `127.0.0.1` | listen address. Anything other than loopback **requires** `AGENT_BRIDGE_TOKEN` |
| `AGENT_BRIDGE_TOKEN` | unset | when set, the UI and API need it: open `/?token=...` once (it is kept in a cookie), or send `Authorization: Bearer ...` |
| `AGENT_BRIDGE_DATA_DIR` | `~/.agent-bridge` | database, uploads, the tmux socket |
| `AGENT_BRIDGE_WORKSPACE_ROOTS` | `$HOME` | colon-separated folders the UI may browse |
| `AGENT_BRIDGE_TERMINAL` | on | `0` disables the terminal and its shell endpoints |
| `AGENT_BRIDGE_TMUX` | on | `0` runs terminals in the server process instead of tmux (they then end when it restarts) |
| `AGENT_BRIDGE_V2` | on | `0` turns off the v2 transport (accounts, quota, conversations) |
| `AGENT_BRIDGE_MAX_LIVE` | `0` | maximum agent processes kept alive at once (`0` = the built-in default) |
| `AGENT_BRIDGE_UPDATE` | auto | `release` (update from GitHub Releases), `git` (pull and rebuild a checkout), `off` |
| `AGENT_BRIDGE_RESTART_CMD` | auto | command used to restart after an update, when you manage the process yourself |

## Zalo bot

Chat with an agent from Zalo. Off unless `ZALO_BOT_TOKEN`, `ZALO_WEBHOOK_SECRET` and `ZALO_ALLOWED_IDS` are all set.

| Variable | Default | Meaning |
|---|---|---|
| `ZALO_BOT_TOKEN` | unset | bot token from the "Zalo Bot Manager" OA (kept out of logs and errors) |
| `ZALO_WEBHOOK_SECRET` | unset | 8-256 characters; the same value you pass to `setWebhook` as `secret_token`. Zalo sends it in `X-Bot-Api-Secret-Token` and every other request is refused with 403 |
| `ZALO_ALLOWED_IDS` | unset | comma separated Zalo user ids allowed to drive agents; messages from anyone else are ignored (their id is logged so you can add yours) |
| `ZALO_ENGINE` | `claude` | engine used for Zalo conversations |
| `ZALO_MODE` | `plan` | permission mode for new Zalo conversations; `plan` is read-only |
| `ZALO_MODES` | `plan,ask` | modes `/mode` may switch to. A web conversation opened with `/use` that runs with any other mode (such as `bypass`) is lowered to `ZALO_MODE` |
| `ZALO_NAS_UNITS` | unset | comma separated systemd units `/nas` may list and control besides the Docker containers |
| `ZALO_NAS_CONTROL` | off | `1` lets `/nas:<service>` start, stop, restart and update; without it `/nas` only reports |
| `ZALO_NAS_PROTECTED` | `cloudflared,wildcard-gateway,agent-bridge` | name fragments of services that Zalo may never stop, restart or update (the bot depends on them); `start` is always allowed |
| `ZALO_WORKSPACE` | `$HOME` | working folder; must be inside `AGENT_BRIDGE_WORKSPACE_ROOTS` |

Zalo needs a public HTTPS URL, so expose **only** `/api/zalo/webhook` (the route is exempt from
`AGENT_BRIDGE_TOKEN`; the secret header protects it). Register it once:

```sh
curl -X POST "https://bot-api.zaloplatforms.com/bot$ZALO_BOT_TOKEN/setWebhook" \
  -H 'Content-Type: application/json' \
  -d "{\"url\":\"https://YOUR-HOST/api/zalo/webhook\",\"secret_token\":\"$ZALO_WEBHOOK_SECRET\"}"
```

Chat commands (private chats only; one turn runs per chat at a time):

| Command | Does |
|---|---|
| `/new`, `/stop` | start a fresh conversation, stop the running turn |
| `/ok`, `/no` | answer an approval request the agent forwarded |
| `/status` | engine, mode, folder and state of the conversation |
| `/engine [name]` | list engines or switch to one |
| `/mode [name]` | list or change the permission mode (only the modes in `ZALO_MODES`) |
| `/ws <folder>` | open a new conversation in another folder (must be inside `AGENT_BRIDGE_WORKSPACE_ROOTS`) |
| `/convs`, `/use <n>` | list recent conversations and continue one from the web UI |
| `/nas ...` | machine commands, see below |
| `/help` | show this list |

### Machine commands (`/nas`)

These run directly on the machine, never through an agent. Every command is started without a shell, and a
service name is accepted only if it matches a Docker container or a unit in `ZALO_NAS_UNITS`.

| Command | Does |
|---|---|
| `/nas` | machine report: uptime, CPU (load, temperature), RAM, every drive with temperature and SMART health (bad sectors are flagged), disk space |
| `/nas service-list` | name and one-line description of every Docker container and allowed systemd unit, with its state |
| `/nas:<service>` | state of one service (image, uptime, restarts, ports, CPU/RAM; or the unit's state) |
| `/nas:<service> start\|stop\|restart` | control the service (needs `ZALO_NAS_CONTROL=1`) |
| `/nas:<service> update` | Docker only: `docker compose pull`, then recreate the container if the image changed. Refused when the compose labels are missing or name another service |
| `/nas help` | list these commands |

Drive temperatures and SMART need passwordless `sudo smartctl`; systemd units need passwordless `sudo systemctl`.

### Scheduled reports and alerts

The same code serves cron or an OpenMediaVault scheduled task (run as root, with `ZALO_BOT_TOKEN` and `ZALO_CHAT_ID`
in a root-only env file):

```sh
agent-bridge nas report [--zalo] [--env-file FILE]                     # the /nas report
agent-bridge nas alerts [--zalo] [--env-file FILE] [--state-file FILE] [--quiet 00:00-06:00]
```

`alerts` lists what is wrong: a hard disk from 53 °C (urgent from 60 °C), an NVMe from 72 °C, the CPU from 85 °C,
a drive whose SMART verdict is not PASSED, pending/reallocated/uncorrectable sectors, and a filesystem 95 % full.
With `--zalo` it sends only what is new: a lasting problem is repeated after 30 minutes (10 minutes above 60 °C),
6 hours (SMART verdict), 12 hours (full disk) or 24 hours (bad sectors), and at once when its text changes, for
example when the count of bad sectors grows. `--state-file` remembers what was sent; a problem that went away
is forgotten and announced again if it returns. A failed delivery is retried by the next run.

`--quiet` sets quiet hours in local time. During them only urgent alerts (heat, a failing drive) are sent; the
others wait and go out when the window ends. A good schedule is the report once an hour by day and the alerts every
15 minutes around the clock.

While an agent works the bot shows the typing indicator and, on long turns, says which tools it used. A
conversation attached to the chat (created from Zalo or picked with `/use`) is announced on Zalo when it
finishes a turn, even if the turn was started from the web UI. Images sent to the bot are saved to
`<data dir>/uploads` (https only, no private addresses, up to 10 MB, jpg/png/gif/webp) and their path is given to
the agent.

## Running as a service

```sh
agent-bridge service install      # Linux: a systemd user unit; macOS: a launchd agent
agent-bridge service status
agent-bridge service restart
agent-bridge service uninstall    # keeps your data
```

Options: `--port`, `--host`, `--data-dir`. On Linux, `--system` installs a system-wide unit instead (needs root,
add `--user NAME`). If a service file already exists and was not generated by Agent Bridge, the command refuses to
replace it unless you pass `--force` (the old file is kept as `*.bak`).

The generated unit uses `KillMode=process`: restarting the service ends only the server, and the terminals and
agents running in tmux stay alive.

## Security

- The terminal is a shell on this machine. Keep the default loopback address unless you need remote access.
- For remote access, set `AGENT_BRIDGE_TOKEN`, use TLS (a reverse proxy, Tailscale or Cloudflare Tunnel), and prefer
  a VPN over exposing the port.
- The folders the UI can open are limited by `AGENT_BRIDGE_WORKSPACE_ROOTS`; credential folders such as `~/.ssh`
  are never served.
