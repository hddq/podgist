#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/../web"
pnpm install --frozen-lockfile
pnpm build

rm -rf ../internal/webui/dist
cp -r build ../internal/webui/dist
