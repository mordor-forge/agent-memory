#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
CONTAINER_NAME="${CONTAINER_NAME:-agent-memory-cockroach}"
VOLUME_NAME="${VOLUME_NAME:-agent-memory-cockroach-data}"

if ! command -v podman >/dev/null 2>&1; then
  echo "podman is required but was not found in PATH" >&2
  exit 1
fi

"${SCRIPT_DIR}/dev-cockroach-down.sh"

if podman volume inspect "${VOLUME_NAME}" >/dev/null 2>&1; then
  echo "Removing volume ${VOLUME_NAME}..."
  podman volume rm -f "${VOLUME_NAME}" >/dev/null
else
  echo "Volume ${VOLUME_NAME} does not exist."
fi

echo "Local CockroachDB state reset complete for ${CONTAINER_NAME}."
