#!/usr/bin/env bash
# Source this in every new M5 terminal before direct kubectl/bootstrap commands.
project="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd -P)"
export HF_M5_STATE="$(realpath -m "${HF_M5_STATE:-$project/.local/tutorials/m5}")"
case "$HF_M5_STATE" in "$project"/.local/tutorials/m5|"$project"/.local/tutorials/m5/*|"$project"/.local/m5|"$project"/.local/m5-*) ;; *) printf 'M5 state must remain in project runtime folders.\n' >&2;return 1 ;; esac
export HF_LEARNING_ROOT="$HF_M5_STATE"
case "${1:-a}" in
 a|hugofleet-a) export HF_CLUSTER=hugofleet-a ;;
 b|hugofleet-b) export HF_CLUSTER=hugofleet-b ;;
 hugofleet-m5-e2e-a|hugofleet-m5-e2e-b) export HF_CLUSTER="$1" ;;
 *) printf 'Select a or b, or exact M5 E2E Worker.\n' >&2;return 1 ;;
esac
source "$project/scripts/common.sh"
