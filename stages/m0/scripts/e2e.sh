#!/usr/bin/env bash
export HF_CLUSTER=hugofleet-m0-e2e
source "$(dirname -- "${BASH_SOURCE[0]}")/../../../scripts/common.sh"
require_versions; need docker; need go
CHECKER="$ROOT/bin/m0check"
ARTIFACTS="$ROOT/artifacts/m0-e2e/$(date +%Y%m%dT%H%M%S)-$$"
mkdir -p "$ARTIFACTS"
exec > >(tee "$ARTIFACTS/run.log") 2>&1
PF_PID=''
PF_SEQUENCE=0
CREATED=0

stop_forward() {
  if [[ -n "$PF_PID" ]]; then
    kill "$PF_PID" 2>/dev/null || true
    wait "$PF_PID" 2>/dev/null || true
    PF_PID=''
  fi
}

cleanup() {
  local result=$?
  trap - EXIT
  stop_forward
  if [[ "$CREATED" == 1 ]]; then
    # Diagnostics are best-effort; cleanup and the original test result remain authoritative.
    kube -n hf-demo get deployments,replicasets,pods,services --request-timeout=10s -o yaml > "$ARTIFACTS/resources.yaml" 2>&1 || true
    kube -n hf-demo get events --request-timeout=10s --sort-by=.lastTimestamp > "$ARTIFACTS/events.txt" 2>&1 || true
    kube -n kube-system get pods --request-timeout=10s -o wide > "$ARTIFACTS/system-pods.txt" 2>&1 || true
    kube -n kube-system get events --request-timeout=10s --sort-by=.lastTimestamp > "$ARTIFACTS/system-events.txt" 2>&1 || true
    timeout --foreground 30 docker stats --no-stream "$CLUSTER-control-plane" > "$ARTIFACTS/docker-stats.txt" 2>&1 || true
    timeout --foreground 120 kind export logs "$ARTIFACTS/kind-logs" --name "$CLUSTER" > "$ARTIFACTS/export.log" 2>&1 || true
    if ! bash "$ROOT/scripts/hf.sh" kind-down; then
      printf 'ERROR: E2E cluster cleanup failed; artifacts=%s\n' "$ARTIFACTS" >&2
      result=1
    fi
  fi
  if [[ "$result" == 0 ]]; then
    printf 'PASS M0 kind E2E, including project cluster cleanup; artifacts=%s\n' "$ARTIFACTS"
  fi
  exit "$result"
}

forward() {
  stop_forward
  local target="$1"
  PF_SEQUENCE=$((PF_SEQUENCE+1))
  local log="$ARTIFACTS/forward-$PF_SEQUENCE.log"
  # Create before background launch so the first readiness read cannot race redirection.
  : > "$log"
  # exec makes PF_PID the actual kubectl process, not a shell function's wrapper.
  (exec kubectl --kubeconfig "$KUBECONFIG" --context "kind-$CLUSTER" --request-timeout=180s \
    -n hf-demo port-forward --address 127.0.0.1 "$target" :8080) > "$log" 2>&1 &
  PF_PID=$!
  local line
  for ((attempt=0; attempt<100; attempt++)); do
    kill -0 "$PF_PID" 2>/dev/null || { cat "$log" >&2; fail 'port-forward stopped'; }
    while IFS= read -r line; do
      if [[ "$line" =~ Forwarding\ from\ 127\.0\.0\.1:([0-9]+) ]]; then
        BASE_URL="http://127.0.0.1:${BASH_REMATCH[1]}"
        return
      fi
    done < "$log"
    sleep 0.2
  done
  fail 'port-forward did not become ready within 20 seconds'
}

wait_for_retired_pods() {
  local keep_image="$1" retired
  retired="$(kube -n hf-demo get pods -l app.kubernetes.io/name=pricing-api -o json | "$CHECKER" retired-pods "$keep_image")"
  # kubectl Service port-forward chooses one Pod; a terminating old Pod must be gone first.
  while IFS= read -r pod; do
    [[ -z "$pod" ]] || kube -n hf-demo wait --for=delete "$pod" --timeout=60s
  done <<< "$retired"
}

cluster_exists && fail 'E2E cluster already exists; inspect it and run HF_CLUSTER=hugofleet-m0-e2e make kind-down first.'
# A clean CI checkout has no bin/m0check. Compile before creating the cluster,
# as the M1/M2 harnesses do, so this target can run without a prior make build.
mkdir -p "$ROOT/bin"
CGO_ENABLED=0 go build -trimpath -o "$CHECKER" ./stages/m0/cmd/m0check
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
CREATED=1
bash "$ROOT/scripts/hf.sh" kind-up
bash "$ROOT/scripts/hf.sh" demo-image-load
kube apply -f stages/m0/workload.yaml
kube -n hf-demo rollout status deployment/pricing-api --timeout=120s
forward service/pricing-api
"$CHECKER" http "$BASE_URL" v1
stop_forward

kube -n hf-demo set image deployment/pricing-api pricing-api=pricing-api:v2-good
kube -n hf-demo rollout status deployment/pricing-api --timeout=120s
wait_for_retired_pods pricing-api:v2-good
forward service/pricing-api
"$CHECKER" http "$BASE_URL" v2-good
stop_forward

kube -n hf-demo set image deployment/pricing-api pricing-api=pricing-api:v2-broken
broken_pod=''
for ((attempt=0; attempt<60; attempt++)); do
  broken_pod="$(kube -n hf-demo get pods -l app.kubernetes.io/name=pricing-api -o json | "$CHECKER" broken-pod)"
  [[ -n "$broken_pod" ]] && break
  sleep 1
done
[[ -n "$broken_pod" ]] || fail 'Broken version Pod did not enter Running phase.'
forward "pod/$broken_pod"
"$CHECKER" http "$BASE_URL" v2-broken
stop_forward

reason=''
for ((attempt=0; attempt<120; attempt++)); do
  reason="$(kube -n hf-demo get deployment pricing-api -o json | "$CHECKER" progress)"
  [[ "$reason" == ProgressDeadlineExceeded ]] && break
  sleep 1
done
[[ "$reason" == ProgressDeadlineExceeded ]] || fail 'Broken rollout did not report ProgressDeadlineExceeded.'
kube -n hf-demo get pod "$broken_pod" -o json | "$CHECKER" assert-broken
kube -n hf-demo get endpointslices -l kubernetes.io/service-name=pricing-api -o json | "$CHECKER" assert-endpoints "$broken_pod"
forward service/pricing-api
"$CHECKER" http "$BASE_URL" v2-good
stop_forward

# This is a raw Kubernetes image update, not HugoFleet's future revision rollback.
kube -n hf-demo set image deployment/pricing-api pricing-api=pricing-api:v1
kube -n hf-demo rollout status deployment/pricing-api --timeout=120s
wait_for_retired_pods pricing-api:v1
forward service/pricing-api
"$CHECKER" http "$BASE_URL" v1
printf 'M0 workload assertions passed; collecting diagnostics and cleaning the project cluster.\n'
