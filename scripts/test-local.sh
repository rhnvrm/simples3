#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
source scripts/seaweedfs-env.sh
bash scripts/seaweedfs-setup.sh
SIMPLES3_SSE_C_INTEGRATION=1 SIMPLES3_SSE_C_LOOPBACK_PROXY=1 go test -race -count=1 -v ./...
