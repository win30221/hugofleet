#!/usr/bin/env bash
export HF_CLUSTER=hugofleet-m1-e2e
source "$(dirname -- "${BASH_SOURCE[0]}")/../../../scripts/common.sh"
require_versions; need docker
export HF_ROOT="$ROOT"
export HF_ARTIFACTS="$ROOT/artifacts/m1-e2e/$(date +%Y%m%dT%H%M%S)-$$"
mkdir -p "$HF_ARTIFACTS"
exec > >(tee "$HF_ARTIFACTS/run.log") 2>&1
CREATED=0
cleanup() {
  local result=$?
  trap - EXIT
  if [[ "$CREATED" == 1 ]]; then
    kube -n hf-demo get applications,deployments,replicasets,pods,services --request-timeout=10s -o yaml > "$HF_ARTIFACTS/resources.yaml" 2>&1 || true
    kube -n hf-demo logs deployment/hugofleet-operator --request-timeout=10s --tail=300 > "$HF_ARTIFACTS/operator.log" 2>&1 || true
    kube -n hf-demo get events --request-timeout=10s --sort-by=.lastTimestamp > "$HF_ARTIFACTS/events.txt" 2>&1 || true
    kube -n kube-system get pods --request-timeout=10s -o wide > "$HF_ARTIFACTS/system-pods.txt" 2>&1 || true
    timeout --foreground 30 docker stats --no-stream "$CLUSTER-control-plane" > "$HF_ARTIFACTS/docker-stats.txt" 2>&1 || true
    timeout --foreground 120 kind export logs "$HF_ARTIFACTS/kind-logs" --name "$CLUSTER" > "$HF_ARTIFACTS/export.log" 2>&1 || true
    if ! bash "$ROOT/scripts/hf.sh" kind-down; then result=1; fi
  fi
  [[ "$result" != 0 ]] || printf 'PASS M1 kind E2E including owned cluster cleanup; artifacts=%s\n' "$HF_ARTIFACTS"
  exit "$result"
}
cluster_exists && fail 'M1 E2E cluster exists; inspect it before a project-owned kind-down.'
# Compile before cluster creation to avoid competing with etcd/CRI disk I/O.
CGO_ENABLED=0 go test -c -tags=e2e -o "$ROOT/bin/m1-e2e" ./stages/m1/test/e2e
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
CREATED=1
bash "$ROOT/scripts/hf.sh" kind-up
bash "$ROOT/scripts/hf.sh" demo-image-load
bash "$ROOT/scripts/hf.sh" operator-install
# Go E2E verifies real SubjectAccessReviews, including both get/create pods/exec.
kube apply -f stages/m1/application.yaml
"$ROOT/bin/m1-e2e" -test.v -test.timeout=20m
printf 'M1 assertions passed; collecting diagnostics and cleaning the owned cluster.\n'
