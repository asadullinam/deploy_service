#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "$SCRIPT_DIR/../.." && pwd)"
COMPOSE_FILE="$REPO_ROOT/deploy-service/tests/system/docker-compose.system.yml"

export SYSTEM_TEST_REPO_ROOT="$REPO_ROOT"

docker compose -f "$COMPOSE_FILE" down -v --remove-orphans >/dev/null 2>&1 || true
cleanup() {
  local status=$?
  if [[ $status -eq 0 || "${SYSTEM_TEST_ALWAYS_CLEANUP:-0}" == "1" ]]; then
    docker compose -f "$COMPOSE_FILE" down -v --remove-orphans >/dev/null 2>&1 || true
  else
    echo "system tests failed; leaving docker compose resources for log collection" >&2
  fi
}
trap cleanup EXIT

docker compose -f "$COMPOSE_FILE" up --build --abort-on-container-exit --exit-code-from system-tests system-tests
