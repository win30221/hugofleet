#!/usr/bin/env bash
export HF_CLUSTER=hugofleet-m1-demo
source "$(dirname -- "${BASH_SOURCE[0]}")/../../scripts/common.sh"
case "${1:-up}" in
 up)
  bash stages/m0/scripts/control.sh image-build
  bash stages/m1/scripts/control.sh image-build
  bash scripts/kind.sh up
  bash stages/m0/scripts/control.sh image-load
  bash stages/m1/scripts/control.sh install
  bash stages/m1/scripts/control.sh deploy
  printf 'm1 ready; use make m1-forward in another terminal.\n'
  ;;
 forward) exec bash stages/m0/scripts/control.sh forward ;;
 down) exec bash scripts/kind.sh down ;;
 *) fail 'Supported learning actions: up, forward, down.' ;;
esac
