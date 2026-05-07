#!/usr/bin/env bash
set -euo pipefail

CONTAINER_NAME="${CONTAINER_NAME:-agent-memory-cockroach}"
VOLUME_NAME="${VOLUME_NAME:-agent-memory-cockroach-data}"
IMAGE="${IMAGE:-docker.io/cockroachdb/cockroach:v26.1.3}"
SQL_PORT="${SQL_PORT:-26257}"
HTTP_PORT="${HTTP_PORT:-18081}"

if ! command -v podman >/dev/null 2>&1; then
  echo "podman is required but was not found in PATH" >&2
  exit 1
fi

if podman container exists "${CONTAINER_NAME}"; then
  status="$(podman inspect -f '{{.State.Status}}' "${CONTAINER_NAME}")"
  case "${status}" in
    running)
      echo "Container ${CONTAINER_NAME} is already running."
      ;;
    *)
      echo "Starting existing container ${CONTAINER_NAME}..."
      podman start "${CONTAINER_NAME}" >/dev/null
      ;;
  esac
else
  if ! podman volume inspect "${VOLUME_NAME}" >/dev/null 2>&1; then
    echo "Creating Podman volume ${VOLUME_NAME}..."
    podman volume create "${VOLUME_NAME}" >/dev/null
  fi

  echo "Starting CockroachDB container ${CONTAINER_NAME}..."
  podman run -d \
    --name "${CONTAINER_NAME}" \
    --network host \
    -v "${VOLUME_NAME}:/cockroach/cockroach-data:Z" \
    "${IMAGE}" \
    start-single-node \
      --insecure \
      --listen-addr="127.0.0.1:${SQL_PORT}" \
      --http-addr="127.0.0.1:${HTTP_PORT}" >/dev/null
fi

echo "Waiting for CockroachDB to accept SQL connections..."
ready=0
for _ in $(seq 1 60); do
  if podman exec "${CONTAINER_NAME}" cockroach sql --insecure --host=127.0.0.1:26257 -e "select 1;" >/dev/null 2>&1; then
    ready=1
    break
  fi
  sleep 1
done

if [[ "${ready}" != "1" ]]; then
  echo "CockroachDB did not become ready in time." >&2
  exit 1
fi

echo "Enabling vector indexes cluster setting..."
podman exec "${CONTAINER_NAME}" cockroach sql --insecure --host=127.0.0.1:26257 \
  -e "SET CLUSTER SETTING feature.vector_index.enabled = true;" >/dev/null

cat <<EOF
CockroachDB is ready.

SQL DSN:
  postgresql://root@127.0.0.1:${SQL_PORT}/defaultdb?sslmode=disable

Admin UI:
  http://127.0.0.1:${HTTP_PORT}

Suggested next steps:
  ./scripts/dev-env.sh
  go run ./cmd/memoryd migrate
  go run ./cmd/agent-memory-mcp
EOF
