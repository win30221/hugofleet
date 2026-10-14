#!/usr/bin/env bash
# Source this in every new M4 terminal before direct kubectl/bootstrap commands.
project="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd -P)"
export HF_M4_STATE="$(realpath -m "${HF_M4_STATE:-$project/.local/tutorials/m4}")"
case "$HF_M4_STATE" in "$project"/.local/tutorials/m4|"$project"/.local/tutorials/m4/*|"$project"/.local/m4|"$project"/.local/m4-*) ;; *) printf 'M4 state must remain in project runtime folders.\n' >&2;return 1 ;; esac
export HF_LEARNING_ROOT="$HF_M4_STATE"
case "${1:-a}" in
 a|hugofleet-a) export HF_CLUSTER=hugofleet-a ;;
 b|hugofleet-b) export HF_CLUSTER=hugofleet-b ;;
 hugofleet-m4-e2e-a|hugofleet-m4-e2e-b) export HF_CLUSTER="$1" ;;
 *) printf 'Select a or b, or exact M4 E2E Worker.\n' >&2;return 1 ;;
esac
source "$project/scripts/common.sh"
