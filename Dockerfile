# syntax=docker/dockerfile:1
# Agent Bridge: web UI + Go server in one image. Multi-arch (linux/amd64, linux/arm64).
#   docker build -t agent-bridge .
#   docker build --build-arg INSTALL_AGENTS=false -t agent-bridge:slim .   # without the agent CLIs

# ---- web bundle (built once on the build host, it is architecture independent)
FROM --platform=$BUILDPLATFORM node:22-bookworm-slim AS web
WORKDIR /app/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# ---- Go server with the bundle embedded (cross-compiled, no emulation)
FROM --platform=$BUILDPLATFORM golang:1.26-bookworm AS server
ARG TARGETOS TARGETARCH
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /app/web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -ldflags="-w -s" -o /out/agent-bridge .

# ---- runtime: glibc (the agent CLIs ship glibc binaries), tmux keeps terminals alive across restarts
FROM node:22-bookworm-slim
ARG INSTALL_AGENTS=true
RUN apt-get update \
 && apt-get install -y --no-install-recommends tmux git bash curl ca-certificates openssh-client tzdata \
 && rm -rf /var/lib/apt/lists/*
# Claude Code and Codex are npm packages. Antigravity (agy) is a desktop product: install it on the host.
RUN if [ "$INSTALL_AGENTS" = "true" ]; then npm install -g @anthropic-ai/claude-code @openai/codex && npm cache clean --force; fi

COPY --from=server /out/agent-bridge /usr/local/bin/agent-bridge

# the base image already has uid 1000 ("node"); work as it, with a real home for CLI logins and sessions
ENV HOME=/home/node \
    SHELL=/bin/bash \
    AGENT_BRIDGE_HOST=0.0.0.0 \
    AGENT_BRIDGE_PORT=8088 \
    AGENT_BRIDGE_DATA_DIR=/data \
    AGENT_BRIDGE_WORKSPACE_ROOTS=/workspace:/home/node
RUN mkdir -p /data /workspace && chown node:node /data /workspace
USER node
WORKDIR /workspace
VOLUME ["/data", "/workspace"]
EXPOSE 8088

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s \
  CMD curl -fsS -H "Authorization: Bearer ${AGENT_BRIDGE_TOKEN}" "http://127.0.0.1:${AGENT_BRIDGE_PORT}/" >/dev/null || exit 1

# AGENT_BRIDGE_TOKEN is required because the server listens on 0.0.0.0 (it refuses to start without one)
ENTRYPOINT ["/usr/local/bin/agent-bridge"]
