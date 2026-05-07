#!/usr/bin/env bash
set -euo pipefail

CONTAINER_NAME="${CONTAINER_NAME:-agent-memory-cockroach}"

if ! command -v podman >/dev/null 2>&1; then
  echo "podman is required but was not found in PATH" >&2
  exit 1
fi

if podman container exists "${CONTAINER_NAME}"; then
  echo "Stopping and removing ${CONTAINER_NAME}..."
  podman rm -f "${CONTAINER_NAME}" >/dev/null
else
  echo "Container ${CONTAINER_NAME} does not exist."
fi
