#!/usr/bin/env bash
source "$(dirname -- "${BASH_SOURCE[0]}")/../../../scripts/common.sh"
export HF_M4_A=hugofleet-m4-e2e-a HF_M4_B=hugofleet-m4-e2e-b
export HF_M4_HUB=hugofleet-hub-m4-e2e HF_M4_DEX=hugofleet-dex-m4-e2e
export HF_M4_STATE="$ROOT/.local/m4-e2e-$(date +%s)-$$" HF_ROOT="$ROOT"
export HF_LEARNING_ROOT="$HF_M4_STATE"
export HF_ARTIFACTS="$ROOT/artifacts/m4-e2e/$(date +%Y%m%dT%H%M%S)-$$"
mkdir -p "$HF_ARTIFACTS" "$ROOT/bin"
exec > >(tee "$HF_ARTIFACTS/run.log") 2>&1
printf 'M4 E2E state=%s artifacts=%s\n' "$HF_M4_STATE" "$HF_ARTIFACTS"
require_versions;need docker
for name in "$HF_M4_A-control-plane" "$HF_M4_B-control-plane" "$HF_M4_HUB" "$HF_M4_DEX";do
 if docker inspect "$name" >/dev/null 2>&1;then fail "M4 E2E resource already exists: $name; inspect ownership before cleanup.";fi
done
CGO_ENABLED=0 go test -c -tags=e2e -o "$ROOT/bin/m4-e2e" ./stages/m4/test/e2e
cleanup(){
 local result=$?
 trap - EXIT
 docker logs "$HF_M4_HUB" > "$HF_ARTIFACTS/hub.log" 2>&1 || true
 docker inspect "$HF_M4_HUB" > "$HF_ARTIFACTS/hub-inspect.json" 2>&1 || true
 timeout --foreground 30 docker stats --no-stream "$HF_M4_HUB" "$HF_M4_DEX" "$HF_M4_A-control-plane" "$HF_M4_B-control-plane" > "$HF_ARTIFACTS/docker-stats.txt" 2>&1 || true
 for id in "$HF_M4_A" "$HF_M4_B";do
  export HF_CLUSTER="$id";source "$ROOT/scripts/common.sh"
  kube get nodes --request-timeout=10s -o wide > "$HF_ARTIFACTS/$id-nodes.txt" 2>&1 || true
  kube -n kube-system get pods --request-timeout=10s -o wide > "$HF_ARTIFACTS/$id-system-pods.txt" 2>&1 || true
  kube -n kube-system get events --request-timeout=10s --sort-by=.lastTimestamp > "$HF_ARTIFACTS/$id-bootstrap-events.txt" 2>&1 || true
  for tenant in team-a team-b;do
   kube -n "hf-$tenant" get applications,deployments,replicasets,pods,services,resourcequotas,limitranges,networkpolicies --request-timeout=10s -o yaml > "$HF_ARTIFACTS/$id-$tenant.yaml" 2>&1 || true
   kube -n "hf-$tenant" logs deployment/hugofleet-operator --request-timeout=10s --tail=200 > "$HF_ARTIFACTS/$id-$tenant-operator.log" 2>&1 || true
  done
  kube -n hf-system logs deployment/hugofleet-agent --request-timeout=10s --tail=200 > "$HF_ARTIFACTS/$id-agent.log" 2>&1 || true
 done
 if ! bash stages/m4/scripts/control.sh down;then result=1;fi
 [[ "$result" != 0 ]] || printf 'PASS M4 real Dex + dual Worker mTLS + tenant RBAC/Admission + Calico packets + owned cleanup; artifacts=%s\n' "$HF_ARTIFACTS"
 exit "$result"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
bash stages/m4/scripts/control.sh up
"$ROOT/bin/m4-e2e" -test.v -test.timeout=30m
