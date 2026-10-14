#!/usr/bin/env bash
set -Eeuo pipefail
project="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd -P)"
export HF_M5_STATE="${HF_M5_STATE:-$project/.local/tutorials/m5}"
export HF_LEARNING_ROOT="$HF_M5_STATE"
cd "$project"
case "${1:-up}" in
 up|down|hub-up|hub-stop|hub-recreate|status|forward) exec bash stages/m5/scripts/control.sh "${1:-up}" ;;
 *) printf 'Supported: up, down, hub-up, hub-stop, hub-recreate, status, forward\n' >&2;exit 1 ;;
esac
