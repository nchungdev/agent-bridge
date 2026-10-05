#!/usr/bin/env bash
set -e

APP_NAME="agent-bridge"
GO_BIN="/usr/local/go/bin/go"

if [ ! -x "$GO_BIN" ]; then
  GO_BIN="go"
fi

echo "==> [1/2] Building React web bundle..."
cd "$(dirname "$0")/web"
npm run build

echo "==> [2/2] Building Go single-binary with embedded frontend..."
cd ..
"$GO_BIN" build -ldflags="-w -s" -o "$APP_NAME" .

echo "==> Build complete! Output: ./$APP_NAME"
