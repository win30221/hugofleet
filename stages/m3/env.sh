#!/usr/bin/env bash
# Source this file in each terminal before inspecting a Worker.
lesson_project="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd -P)"
case "${1:-a}" in
 a|b) export HF_CLUSTER="hugofleet-${1:-a}" ;;
 *) printf 'Source env.sh with a or b.\n' >&2;return 1 ;;
esac
export HF_M3_STATE="${HF_M3_STATE:-$lesson_project/.local/tutorials/m3}"
export HF_LEARNING_ROOT="$HF_M3_STATE"
source "$lesson_project/scripts/common.sh"
