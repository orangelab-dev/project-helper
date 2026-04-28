#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$ROOT_DIR"

if [[ -x "./project-helper" ]]; then
  APP="./project-helper"
elif [[ -x "./bin/project-helper" ]]; then
  APP="./bin/project-helper"
else
  echo "Cannot find project-helper executable next to this script or in ./bin."
  echo "Expected one of:"
  echo "  $ROOT_DIR/project-helper"
  echo "  $ROOT_DIR/bin/project-helper"
  exit 1
fi

echo "Working directory: $ROOT_DIR"
echo "Starting: $APP"
echo "Open the configured address after startup. Default: http://localhost:8080"
exec "$APP"
