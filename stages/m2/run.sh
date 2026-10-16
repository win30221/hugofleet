#!/usr/bin/env bash
export HF_CLUSTER=hugofleet-m2-demo
source "$(dirname -- "${BASH_SOURCE[0]}")/../../scripts/common.sh"
export HF_HUB_KUBECONFIG="$ROOT/.local/tutorials/m2/hub/kubeconfig"
export HF_HUB_DATABASE="${HF_HUB_DATABASE:-$ROOT/.local/tutorials/m2/hub/hub.db}"
export HF_HUB_POLICY="$ROOT/stages/m2/policy.json"
case "${1:-up}" in
 up)
  bash stages/m0/scripts/control.sh image-build
  bash stages/m1/scripts/control.sh image-build
  bash scripts/kind.sh up
  bash stages/m0/scripts/control.sh image-load
  bash stages/m1/scripts/control.sh install
  bash stages/m2/scripts/control.sh bootstrap
  exec bash stages/m2/scripts/control.sh hub-up
  ;;
 forward) exec bash stages/m0/scripts/control.sh forward ;;
 down) exec bash scripts/kind.sh down ;;
 hub-up) exec bash stages/m2/scripts/control.sh hub-up ;;
 status)
  assert_owned
  kube -n hf-demo get applications,deployments,services,pods
  kube -n hf-demo get events --sort-by=.lastTimestamp
  ;;
 *) fail 'Supported learning actions: up, forward, down, hub-up, status.' ;;
esac
