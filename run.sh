#!/usr/bin/env bash
set -e
cd "$(dirname "$0")"
export PATH="$HOME/.local/go/bin:$PATH"
if [ ! -f go.sum ]; then
  go mod tidy
fi
exec go run ./cmd/redthread
