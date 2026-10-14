#!/usr/bin/env bash
set -Eeuo pipefail
ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"
source "$ROOT/scripts/versions.env"
export CACHE_ROOT="$ROOT/.cache"
if [[ -f "$ROOT/.local/native-cache.path" ]]; then
  candidate="$(cat "$ROOT/.local/native-cache.path")"
  case "$candidate" in
    /tmp/hugofleet-go-cache.*|/var/tmp/hugofleet-go-cache.*)
      if [[ -f "$candidate/.hugofleet-root" ]] && [[ "$(cat "$candidate/.hugofleet-root")" == "$ROOT" ]]; then
        export CACHE_ROOT="$candidate"
      fi
      ;;
    *) printf 'Invalid project native cache record.\n' >&2; exit 1 ;;
  esac
fi
go_root="$ROOT/.tools/go"
[[ ! -x "$CACHE_ROOT/go/bin/go" ]] || go_root="$CACHE_ROOT/go"
export PATH="$ROOT/.tools/bin:$go_root/bin:$PATH"
export GOTOOLCHAIN=local
export GOCACHE="$CACHE_ROOT/go-build"
export GOMODCACHE="$CACHE_ROOT/go-mod"
if [[ "$CACHE_ROOT" != "$ROOT/.cache" ]]; then
  mkdir -p "$CACHE_ROOT/go-tmp"
  export GOTMPDIR="$CACHE_ROOT/go-tmp"
fi
export KIND_EXPERIMENTAL_PROVIDER=docker
CLUSTER="${HF_CLUSTER:-hugofleet-a}"
case "$CLUSTER" in
  hugofleet-a|hugofleet-b|hugofleet-m0-demo|hugofleet-m1-demo|hugofleet-m2-demo|hugofleet-m3-e2e-a|hugofleet-m3-e2e-b|hugofleet-m4-e2e-a|hugofleet-m4-e2e-b|hugofleet-m5-e2e-a|hugofleet-m5-e2e-b|hugofleet-m0-e2e|hugofleet-m1-e2e|hugofleet-m2-e2e) ;;
  *) printf 'Unsupported project cluster name: %s\n' "$CLUSTER" >&2; exit 1 ;;
esac
HF_CLUSTER_STATE="$ROOT/.local"
case "$CLUSTER" in
  hugofleet-m[012]-demo)
    lesson="${CLUSTER#hugofleet-}";lesson="${lesson%-demo}"
    HF_CLUSTER_STATE="$ROOT/.local/tutorials/$lesson/clusters/$CLUSTER"
    ;;
  hugofleet-a|hugofleet-b|hugofleet-m4-e2e-a|hugofleet-m4-e2e-b|hugofleet-m5-e2e-a|hugofleet-m5-e2e-b)
    if [[ -n "${HF_LEARNING_ROOT:-}" ]];then
      lesson_root="$(realpath -m "$HF_LEARNING_ROOT")"
      case "$lesson_root" in
        "$ROOT"/.local/tutorials/m3|"$ROOT"/.local/tutorials/m3/*|"$ROOT"/.local/m3|"$ROOT"/.local/m3-*|"$ROOT"/.local/tutorials/m4|"$ROOT"/.local/tutorials/m4/*|"$ROOT"/.local/m4|"$ROOT"/.local/m4-*|"$ROOT"/.local/tutorials/m5|"$ROOT"/.local/tutorials/m5/*|"$ROOT"/.local/m5|"$ROOT"/.local/m5-*) ;;
        *) printf 'Learning state must remain inside the project stage runtime folders.\n' >&2;exit 1 ;;
      esac
      HF_CLUSTER_STATE="$lesson_root/clusters/$CLUSTER"
    fi
    ;;
esac
if [[ "$HF_CLUSTER_STATE" == "$ROOT/.local" ]];then
  export KUBECONFIG="$ROOT/.local/kubeconfig-$CLUSTER"
  OWNERSHIP_FILE="$ROOT/.local/cluster-$CLUSTER.id"
else
  export KUBECONFIG="$HF_CLUSTER_STATE/kubeconfig"
  OWNERSHIP_FILE="$HF_CLUSTER_STATE/cluster.id"
fi
export HF_CLUSTER_STATE
cd "$ROOT"

fail() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }
need() { command -v "$1" >/dev/null || fail "Missing $1. Run make tools; prerequisites: Linux/WSL2, curl, make, Docker."; }
kube() { kubectl --kubeconfig "$KUBECONFIG" --context "kind-$CLUSTER" --request-timeout=180s "$@"; }

# The management ports are published on 127.0.0.1. Docker may list the
# IPv6 IPAM entry first, so its array position is not the IPv4 client gateway.
kind_ipv4_gateway() {
  local gateways gateway selected=""
  gateways="$(docker network inspect kind --format '{{range .IPAM.Config}}{{println .Gateway}}{{end}}')" || fail 'Cannot inspect kind network gateways.'
  while IFS= read -r gateway; do
    [[ -n "$gateway" && "$gateway" != *:* ]] || continue
    [[ "$gateway" =~ ^[0-9]{1,3}(\.[0-9]{1,3}){3}$ ]] || fail 'Unexpected kind gateway address.'
    [[ -z "$selected" ]] || fail 'Exactly one IPv4 kind gateway is required.'
    selected="$gateway"
  done <<< "$gateways"
  [[ -n "$selected" ]] || fail 'An IPv4 kind gateway is required for loopback management.'
  printf '%s\n' "$selected"
}

build_checker() {
  need go
  mkdir -p "$ROOT/bin"
  CGO_ENABLED=0 go build -trimpath -o "$ROOT/bin/m0check" ./stages/m0/cmd/m0check
}

cluster_exists() {
  local clusters
  clusters="$(kind get clusters)" || fail 'Cannot enumerate kind clusters.'
  [[ $'\n'"$clusters"$'\n' == *$'\n'"$CLUSTER"$'\n'* ]]
}

node_exists() {
  local nodes
  nodes="$(docker ps -a --filter "name=^/$CLUSTER-control-plane$" --format '{{.ID}}')" || fail 'Cannot confirm project container state; ownership record retained.'
  [[ -n "$nodes" ]]
}

assert_owned() {
  local current recorded
  [[ -f "$OWNERSHIP_FILE" ]] || fail "$CLUSTER has no HugoFleet ownership record; refusing to modify it."
  recorded="$(cat "$OWNERSHIP_FILE")"
  current="$(docker inspect --format '{{.Id}}' "$CLUSTER-control-plane")" || fail 'Cannot inspect project cluster.'
  [[ -n "$recorded" && "$current" == "$recorded" ]] || fail "$CLUSTER ownership no longer matches its container."
}

require_versions() {
  need go; need kind; need kubectl
  [[ "$(go env GOVERSION)" == "go$GO_VERSION" ]] || fail "Go $GO_VERSION required; run make tools."
  [[ "$(kind version)" == "kind v$KIND_VERSION "* ]] || fail "kind $KIND_VERSION required; run make tools."
  build_checker
  [[ "$(kubectl version --client -o json | "$ROOT/bin/m0check" client-version)" == "v$KUBECTL_VERSION" ]] || fail "kubectl $KUBECTL_VERSION required; run make tools."
}
