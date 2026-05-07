#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd -- "${SCRIPT_DIR}/.." && pwd)"

export MEMORY_DATABASE_URL="${MEMORY_DATABASE_URL:-postgresql://root@127.0.0.1:26257/defaultdb?sslmode=disable}"
export MEMORY_EMBEDDER_PROVIDER="${MEMORY_EMBEDDER_PROVIDER:-deterministic}"
export MEMORY_EMBEDDER_DIMENSIONS="${MEMORY_EMBEDDER_DIMENSIONS:-1536}"

if [[ -x "${ROOT_DIR}/bin/agent-memory-mcp" ]]; then
  exec "${ROOT_DIR}/bin/agent-memory-mcp"
fi

exec go run "${ROOT_DIR}/cmd/agent-memory-mcp"
