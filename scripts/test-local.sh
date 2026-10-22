#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
source scripts/garage-env.sh
bash scripts/garage-setup.sh
go test -race -v ./...
