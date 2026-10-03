# ==============================================================================
# Multi-stage Dockerfile for Nexus AI (Universal Autonomous Agent Orchestrator)
# ==============================================================================

# Stage 1: Build React Frontend
FROM node:22-alpine AS web-builder
WORKDIR /app/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# Stage 2: Build Go Backend
FROM golang:1.24-alpine AS go-builder
WORKDIR /app
RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .
# Copy built assets into web/dist for go:embed
COPY --from=web-builder /app/web/dist ./web/dist

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o nexus-ai .

# Stage 3: Final Minimal Runtime Image
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata git curl bash

WORKDIR /app
COPY --from=go-builder /app/nexus-ai /usr/local/bin/nexus-ai

ENV PORT=8088
ENV DATA_DIR=/data
EXPOSE 8088

VOLUME ["/data"]

ENTRYPOINT ["/usr/local/bin/nexus-ai"]
