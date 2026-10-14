#!/usr/bin/env bash
export HF_CLUSTER=hugofleet-m2-e2e
source "$(dirname -- "${BASH_SOURCE[0]}")/../../../scripts/common.sh"
require_versions; need docker
export HF_ROOT="$ROOT"
export HF_ARTIFACTS="$ROOT/artifacts/m2-e2e/$(date +%Y%m%dT%H%M%S)-$$"
export HF_HUB_KUBECONFIG="$ROOT/.local/hub-kubeconfig-$CLUSTER"
mkdir -p "$HF_ARTIFACTS" "$ROOT/bin"
exec > >(tee "$HF_ARTIFACTS/run.log") 2>&1
CREATED=0
cleanup() {
  local result=$?
  trap - EXIT
  if [[ "$CREATED" == 1 ]]; then
    kube -n hf-demo get applications,deployments,replicasets,pods,services,resourcequotas --request-timeout=10s -o yaml > "$HF_ARTIFACTS/resources.yaml" 2>&1 || true
    kube -n hf-demo logs deployment/hugofleet-operator --request-timeout=10s --tail=300 > "$HF_ARTIFACTS/operator.log" 2>&1 || true
    kube -n hf-demo get events --request-timeout=10s --sort-by=.lastTimestamp > "$HF_ARTIFACTS/events.txt" 2>&1 || true
    timeout --foreground 30 docker stats --no-stream "$CLUSTER-control-plane" > "$HF_ARTIFACTS/docker-stats.txt" 2>&1 || true
    timeout --foreground 120 kind export logs "$HF_ARTIFACTS/kind-logs" --name "$CLUSTER" > "$HF_ARTIFACTS/export.log" 2>&1 || true
    if ! bash "$ROOT/scripts/hf.sh" kind-down; then result=1; fi
    rm -f -- "$HF_HUB_KUBECONFIG"
  fi
  [[ "$result" != 0 ]] || printf 'PASS M2 real Hub + SQLite + kind E2E including owned cleanup; artifacts=%s\n' "$HF_ARTIFACTS"
  exit "$result"
}
cluster_exists && fail 'M2 E2E cluster exists; inspect before a project-owned kind-down.'
CGO_ENABLED=0 go build -trimpath -o "$ROOT/bin/hub" ./stages/m2/cmd/hub
CGO_ENABLED=0 go build -trimpath -o "$ROOT/bin/bootstrap-hub" ./stages/m2/cmd/bootstrap-hub
CGO_ENABLED=0 go test -c -tags=e2e -o "$ROOT/bin/m2-e2e" ./stages/m2/test/e2e
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
CREATED=1
bash "$ROOT/scripts/hf.sh" kind-up
bash "$ROOT/scripts/hf.sh" demo-image-load
bash "$ROOT/scripts/hf.sh" operator-install
kube apply -f stages/m2/config/hub/rbac.yaml
"$ROOT/bin/bootstrap-hub" --kubeconfig "$KUBECONFIG" --context "kind-$CLUSTER" --output "$HF_HUB_KUBECONFIG"
"$ROOT/bin/m2-e2e" -test.v -test.timeout=25m
printf 'M2 assertions passed; collecting diagnostics and cleaning owned cluster.\n'
