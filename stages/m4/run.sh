#!/usr/bin/env bash
set -Eeuo pipefail
project="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd -P)"
export HF_M4_STATE="${HF_M4_STATE:-$project/.local/tutorials/m4}"
export HF_LEARNING_ROOT="$HF_M4_STATE"
cd "$project"
case "${1:-up}" in
 up|down|hub-up|hub-stop|hub-recreate|status|forward) exec bash stages/m4/scripts/control.sh "${1:-up}" ;;
 *) printf 'Supported: up, down, hub-up, hub-stop, hub-recreate, status, forward\n' >&2;exit 1 ;;
esac
