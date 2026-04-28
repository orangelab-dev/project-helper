#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")" && pwd)"
FRONTEND_DIR="$ROOT_DIR/frontend"
BIN_DIR="$ROOT_DIR/bin"

mkdir -p "$BIN_DIR"

(cd "$FRONTEND_DIR" && npm ci && npm run build)
(cd "$ROOT_DIR" && CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -tags prod -o "$BIN_DIR/project-helper.exe" ./cmd/server)

echo "Built $BIN_DIR/project-helper.exe"
