#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT_DIR"

if ! command -v pnpm >/dev/null 2>&1; then
  echo "pnpm not found. Install pnpm first." >&2
  exit 1
fi

if ! command -v go >/dev/null 2>&1; then
  echo "go not found. Install Go first." >&2
  exit 1
fi

if [[ ! -d node_modules || ! -d web/node_modules ]]; then
  pnpm install
fi

# Clean frontend dist to ensure fresh build
rm -rf web/dist

echo "Starting ControlCCX (production-like)..."
echo "URL: http://127.0.0.1:5174"
pnpm start -- "$@"

