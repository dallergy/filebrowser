#!/usr/bin/env bash
# Idempotent bootstrap for the File Browser dev environment.
# Builds the Vue frontend (embedded into the Go binary) and the Go backend.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Ensure Node 24 + pnpm (frontend requires Node >=24, pnpm >=10).
export NVM_DIR="${NVM_DIR:-$HOME/.nvm}"
# shellcheck disable=SC1091
. "$NVM_DIR/nvm.sh"
nvm install 24 >/dev/null
nvm alias default 24 >/dev/null
export PATH="$(dirname "$(nvm which 24)"):$PATH"
corepack enable
corepack prepare pnpm@10.33.4 --activate

# Frontend: install deps and produce the embedded dist/ assets.
cd "$REPO_ROOT/frontend"
pnpm install --frozen-lockfile
pnpm run build

# Backend: fetch modules and build the binary (embeds frontend/dist).
cd "$REPO_ROOT"
go mod download
go build -o filebrowser .

echo "File Browser environment ready: $(./filebrowser version | head -1)"
