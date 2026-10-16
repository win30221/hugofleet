#!/usr/bin/env bash
export HF_CLUSTER=hugofleet-m0-demo
source "$(dirname -- "${BASH_SOURCE[0]}")/../../scripts/common.sh"
case "${1:-up}" in
 up)
  bash stages/m0/scripts/control.sh image-build
  bash scripts/kind.sh up
  bash stages/m0/scripts/control.sh image-load
  bash stages/m0/scripts/control.sh deploy
  printf 'm0 ready; use make m0-forward in another terminal.\n'
  ;;
 forward) exec bash stages/m0/scripts/control.sh forward ;;
 down) exec bash scripts/kind.sh down ;;
 update-good|scale-two|break|restore|status|observe) exec bash stages/m0/scripts/lesson.sh "$1" ;;
 *) fail 'Supported learning actions: up, forward, down, update-good, scale-two, break, restore, status, observe.' ;;
esac
