#!/usr/bin/env bash
set -Eeuo pipefail
project="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd -P)"
export HF_M3_STATE="${HF_M3_STATE:-$project/.local/tutorials/m3}"
export HF_LEARNING_ROOT="$HF_M3_STATE"
cd "$project"
case "${1:-up}" in
 up|down|hub-up|hub-stop|hub-recreate) exec bash stages/m3/scripts/control.sh "m3-${1:-up}" ;;
 agent-install|certs-dev) exec bash stages/m3/scripts/control.sh "$1" ;;
 status-a|status-b|forward-a|forward-b|agent-stop-a|agent-start-a|drift-a|observe-a)
  exec bash scripts/lesson-worker.sh m3 "${1##*-}" "${1%-*}" ;;
 *) printf 'Supported: up, down, hub-up, hub-stop, hub-recreate, agent-install, certs-dev, status-a, status-b, forward-a, forward-b, agent-stop-a, agent-start-a, drift-a, observe-a\n' >&2;exit 1 ;;
esac
