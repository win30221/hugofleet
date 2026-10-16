#!/usr/bin/env bash
set -Eeuo pipefail
project="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd -P)"
export HF_M5_STATE="${HF_M5_STATE:-$project/.local/tutorials/m5}"
export HF_LEARNING_ROOT="$HF_M5_STATE"
cd "$project"
case "${1:-up}" in
 up|down|hub-up|hub-stop|hub-recreate|status|forward) exec bash stages/m5/scripts/control.sh "${1:-up}" ;;
 workload-status) exec bash scripts/lesson-worker.sh m5 a status ;;
 forward-a) exec bash scripts/lesson-worker.sh m5 a forward ;;
 drift|observe) exec bash scripts/lesson-worker.sh m5 a "$1" ;;
 agent-stop-b|agent-start-b) exec bash scripts/lesson-worker.sh m5 b "${1%-b}" ;;
 *) printf 'Supported: up, down, hub-up, hub-stop, hub-recreate, status, forward, forward-a, workload-status, drift, observe, agent-stop-b, agent-start-b\n' >&2;exit 1 ;;
esac
