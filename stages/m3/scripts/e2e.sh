#!/usr/bin/env bash
source "$(dirname -- "${BASH_SOURCE[0]}")/../../../scripts/common.sh"
export HF_M3_A=hugofleet-m3-e2e-a HF_M3_B=hugofleet-m3-e2e-b
export HF_M3_HUB=hugofleet-hub-m3-e2e HF_M3_HTTP_PORT=18290
export HF_ROOT="$ROOT"
export HF_ARTIFACTS="$ROOT/artifacts/m3-e2e/$(date +%Y%m%dT%H%M%S)-$$"
export HF_M3_STATE="$ROOT/.local/m3-e2e-$(date +%s)-$$"
mkdir -p "$HF_ARTIFACTS" "$ROOT/bin"
exec > >(tee "$HF_ARTIFACTS/run.log") 2>&1
require_versions;need docker
for id in "$HF_M3_A" "$HF_M3_B";do
 if docker inspect "$id-control-plane" >/dev/null 2>&1;then fail "E2E cluster $id already exists; inspect before owned cleanup.";fi
done
if docker inspect "$HF_M3_HUB" >/dev/null 2>&1;then fail 'E2E Hub already exists; inspect before owned cleanup.';fi
CGO_ENABLED=0 go test -c -tags=e2e -o "$ROOT/bin/m3-e2e" ./stages/m3/test/e2e
cleanup(){
 local result=$?
 trap - EXIT
 docker logs "$HF_M3_HUB" > "$HF_ARTIFACTS/hub.log" 2>&1 || true
 docker inspect "$HF_M3_HUB" > "$HF_ARTIFACTS/hub-inspect.json" 2>&1 || true
 timeout --foreground 30 docker stats --no-stream "$HF_M3_HUB" "$HF_M3_A-control-plane" "$HF_M3_B-control-plane" > "$HF_ARTIFACTS/docker-stats.txt" 2>&1 || true
 for id in "$HF_M3_A" "$HF_M3_B";do
  export HF_CLUSTER="$id";source "$ROOT/scripts/common.sh"
  kube -n hf-demo get applications,deployments,replicasets,pods,services,configmaps --request-timeout=10s -o yaml > "$HF_ARTIFACTS/$id-resources.yaml" 2>&1 || true
  kube -n hf-demo logs deployment/hugofleet-agent --request-timeout=10s --tail=200 > "$HF_ARTIFACTS/$id-agent.log" 2>&1 || true
  kube -n hf-demo logs deployment/hugofleet-operator --request-timeout=10s --tail=200 > "$HF_ARTIFACTS/$id-operator.log" 2>&1 || true
  timeout --foreground 120 kind export logs "$HF_ARTIFACTS/$id-kind" --name "$id" > "$HF_ARTIFACTS/$id-export.log" 2>&1 || true
 done
 if ! bash stages/m3/scripts/control.sh m3-down;then result=1;fi
 [[ "$result" != 0 ]] || printf 'PASS M3 real dual kind + Worker Pod mTLS gRPC + owned cleanup; artifacts=%s\n' "$HF_ARTIFACTS"
 exit "$result"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
bash stages/m3/scripts/control.sh m3-up
"$ROOT/bin/m3-e2e" -test.v -test.timeout=30m
