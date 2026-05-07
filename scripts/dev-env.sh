#!/usr/bin/env bash
set -euo pipefail

SQL_PORT="${SQL_PORT:-26257}"
DATABASE_URL="postgresql://root@127.0.0.1:${SQL_PORT}/defaultdb?sslmode=disable"

if [[ "${BASH_SOURCE[0]}" != "$0" ]]; then
  export MEMORY_DATABASE_URL="${DATABASE_URL}"
  export MEMORY_HTTP_ADDR=':8080'
  export MEMORY_HTTP_AUTH_MODE='disabled'
  export MEMORY_EMBEDDER_PROVIDER='deterministic'
  export MEMORY_EMBEDDER_DIMENSIONS='1536'
  return 0
fi

cat >&2 <<'EOF'
Tip: this script prints shell exports. To apply them to your current shell, use one of:
  eval "$(./scripts/dev-env.sh)"
  source ./scripts/dev-env.sh
EOF

cat <<EOF
export MEMORY_DATABASE_URL='${DATABASE_URL}'
export MEMORY_HTTP_ADDR=':8080'
export MEMORY_HTTP_AUTH_MODE='disabled'
export MEMORY_EMBEDDER_PROVIDER='deterministic'
export MEMORY_EMBEDDER_DIMENSIONS='1536'
EOF
