#!/usr/bin/env bash
# Source this file to put Node.js 24 (required by the frontend) on PATH.
# The base image ships Node 22 on an early PATH entry, so we rely on nvm's
# Node 24 and prepend it explicitly.
export NVM_DIR="${NVM_DIR:-$HOME/.nvm}"
# shellcheck disable=SC1091
. "$NVM_DIR/nvm.sh"
nvm use 24 >/dev/null 2>&1 || nvm install 24 >/dev/null 2>&1
export PATH="$(dirname "$(nvm which 24)"):$PATH"
