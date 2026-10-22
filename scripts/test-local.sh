#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
source scripts/seaweedfs-env.sh
bash scripts/seaweedfs-setup.sh
go test -race -count=1 -v ./...
