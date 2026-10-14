#!/usr/bin/env bash
source "$(dirname -- "${BASH_SOURCE[0]}")/common.sh"

wait_cluster_ready() {
  kube wait --for=condition=Ready nodes --all --timeout=120s
  # CoreDNS is created before an external CNI is ready. Its initial progress
  # deadline can expire during a cold image import even though its Pods are now
  # starting. Require real availability before checking the complete rollout;
  # never rewrite status or accept a failed/partial Deployment as ready.
  kube -n kube-system wait --for=condition=Available deployment/coredns --timeout=300s
  # Node Ready alone does not confirm Deployment controller progress after slow bootstrap.
  kube -n kube-system rollout status deployment/coredns --timeout=180s
}

cache_calico_images() {
  # Keep the fixed release images in the host Docker cache across owned Worker
  # rebuilds. A registry failure happens before creating a new control plane.
  local image
  for image in "quay.io/calico/calico:v$CALICO_VERSION" "quay.io/calico/third-party-cni-plugins:v$CALICO_VERSION" "quay.io/calico/node:v$CALICO_VERSION"; do
    if ! docker image inspect "$image" >/dev/null 2>&1; then
      timeout --foreground 1800 docker pull --platform linux/amd64 "$image"
    fi
  done
}

load_calico_images() {
  assert_owned
  timeout --foreground 900 kind load docker-image \
    "quay.io/calico/calico:v$CALICO_VERSION" \
    "quay.io/calico/third-party-cni-plugins:v$CALICO_VERSION" \
    "quay.io/calico/node:v$CALICO_VERSION" --name "$CLUSTER"
}

wait_calico_ready() {
  # Cold registry pulls can take more than ten minutes on this WSL/Docker setup.
  # Retry readiness on this same owned Worker; do not recreate its control plane
  # or silently convert a different CNI. A partial manifest creation needs an
  # explicit inspected repair rather than adopting unknown resources.
  local image
  image="$(kube -n kube-system get daemonset calico-node -o jsonpath='{.spec.template.spec.containers[0].image}')"
  [[ "$image" == "quay.io/calico/node:v$CALICO_VERSION" ]] || fail 'Owned Security Worker has an unexpected Calico version.'
  kube -n kube-system rollout status daemonset/calico-node --timeout=1200s
  kube -n kube-system rollout status deployment/calico-kube-controllers --timeout=600s
}

kind_up() {
  require_versions; need docker; need timeout
  mkdir -p "$HF_CLUSTER_STATE"
  local cni_profile="${HF_CNI_PROFILE:-kindnet}"
  case "$cni_profile" in
    kindnet) ;;
    calico)
      case "$CLUSTER" in hugofleet-a|hugofleet-m4-e2e-a|hugofleet-m5-e2e-a) ;; *) fail 'Calico Security Profile is limited to an approved Security A Worker.' ;; esac
      case "${HF_LEARNING_ROOT:-}" in "$ROOT"/.local/tutorials/m4|"$ROOT"/.local/tutorials/m4/*|"$ROOT"/.local/m4|"$ROOT"/.local/m4-*|"$ROOT"/.local/tutorials/m5|"$ROOT"/.local/tutorials/m5/*|"$ROOT"/.local/m5|"$ROOT"/.local/m5-*) ;; *) fail 'Calico requires an explicit owned Security stage runtime directory.' ;; esac
      ;;
    *) fail 'HF_CNI_PROFILE must be kindnet or calico.' ;;
  esac
  if cluster_exists; then
    assert_owned
    recorded_cni=kindnet
    [[ ! -f "$HF_CLUSTER_STATE/cni-profile" ]] || recorded_cni="$(cat "$HF_CLUSTER_STATE/cni-profile")"
    [[ "$recorded_cni" == "$cni_profile" ]] || fail 'Existing Worker uses a different CNI; explicitly down/recreate this owned stage first.'
    kind export kubeconfig --name "$CLUSTER" --kubeconfig "$KUBECONFIG"
    if [[ "$cni_profile" == calico ]]; then
      cache_calico_images
      load_calico_images
      wait_calico_ready
    fi
    wait_cluster_ready
    return
  fi
  # Do not adopt an unrelated container that happens to have the same name.
  if docker inspect "$CLUSTER-control-plane" >/dev/null 2>&1; then
    fail "$CLUSTER-control-plane already exists; refusing to adopt it."
  fi
  [[ "$cni_profile" != calico ]] || cache_calico_images
  local api_port=16443
  [[ "$CLUSTER" != hugofleet-m0-e2e ]] || api_port=16444
  [[ "$CLUSTER" != hugofleet-m1-e2e ]] || api_port=16445
  [[ "$CLUSTER" != hugofleet-m2-e2e ]] || api_port=16446
  case "$CLUSTER" in
    hugofleet-b) api_port=16447 ;;
    hugofleet-m0-demo) api_port=16440 ;;
    hugofleet-m1-demo) api_port=16441 ;;
    hugofleet-m2-demo) api_port=16442 ;;
    hugofleet-m3-e2e-a) api_port=16448 ;;
    hugofleet-m3-e2e-b) api_port=16449 ;;
    hugofleet-m4-e2e-a) api_port=16450 ;;
    hugofleet-m4-e2e-b) api_port=16451 ;;
    hugofleet-m5-e2e-a) api_port=16452 ;;
    hugofleet-m5-e2e-b) api_port=16453 ;;
  esac
  api_port="${HF_API_SERVER_PORT:-$api_port}"
  [[ "$api_port" =~ ^[0-9]{4,5}$ ]] && ((10#$api_port >= 1024 && 10#$api_port <= 65535)) || fail 'HF_API_SERVER_PORT must be an integer from 1024 to 65535.'
  api_port="$((10#$api_port))"
  local etcd_data_dir
  case "${HF_ETCD_STORAGE:-memory}" in
    memory) etcd_data_dir=/run/hugofleet-etcd ;;
    disk) etcd_data_dir=/var/lib/etcd ;;
    *) fail 'HF_ETCD_STORAGE must be memory or disk.' ;;
  esac
  local kind_config="$HF_CLUSTER_STATE/kind-$CLUSTER.yaml"
  sed -e "s/apiServerPort: 16443/apiServerPort: $api_port/" \
    -e "s|dataDir: /run/hugofleet-etcd|dataDir: $etcd_data_dir|" \
    "$ROOT/deploy/kind/single-node.yaml" > "$kind_config"
  local ready_wait=240s
  if [[ "$cni_profile" == calico ]];then
    sed -i '/^networking:/a\  disableDefaultCNI: true\n  podSubnet: "192.168.0.0/16"' "$kind_config"
    ready_wait=0s
  fi
  printf '%s\n' "$cni_profile" > "$HF_CLUSTER_STATE/cni-profile"
  local result=0
  mkdir -p "$ROOT/artifacts/bootstrap-$CLUSTER"
  local bootstrap_log="$ROOT/artifacts/bootstrap-$CLUSTER/$(date +%Y%m%dT%H%M%S).log"
  printf 'Creating %s; etcd dataDir=%s; full bootstrap log: %s\n' "$CLUSTER" "$etcd_data_dir" "$bootstrap_log"
  timeout --foreground 1800 kind create cluster --name "$CLUSTER" --config "$kind_config" \
    --image "$KIND_NODE_IMAGE" --kubeconfig "$KUBECONFIG" --wait "$ready_wait" --retain \
    > "$bootstrap_log" 2>&1 || result=$?
  # Record the exact container created by this invocation, including a partial bootstrap.
  if docker inspect --format '{{.Id}}' "$CLUSTER-control-plane" > "$OWNERSHIP_FILE.tmp" 2>/dev/null; then
    mv -- "$OWNERSHIP_FILE.tmp" "$OWNERSHIP_FILE"
  fi
  if [[ "$result" != 0 ]]; then
    tail -n 30 "$bootstrap_log" >&2
    fail "kind bootstrap failed ($result). Inspect $bootstrap_log; make kind-down can remove only this recorded cluster."
  fi
  tail -n 20 "$bootstrap_log"
  if [[ "$cni_profile" == calico ]];then
    load_calico_images
    local manifest="$ROOT/.tools/downloads/calico-v$CALICO_VERSION.yaml"
    if [[ ! -f "$manifest" ]] || ! printf '%s  %s\n' "$CALICO_MANIFEST_SHA256" "$manifest" | sha256sum --check --status;then
      curl -fsSL --retry 2 --connect-timeout 15 --max-time 180 "https://raw.githubusercontent.com/projectcalico/calico/v$CALICO_VERSION/manifests/calico.yaml" -o "$manifest"
    fi
    printf '%s  %s\n' "$CALICO_MANIFEST_SHA256" "$manifest" | sha256sum --check --status || fail 'Calico manifest checksum mismatch.'
    # Initial creation avoids the client-side last-applied annotation size limit
    # on Calico CRDs. No unrelated cluster is reachable through this context.
    kube create -f "$manifest"
    wait_calico_ready
  fi
  wait_cluster_ready
}

kind_down() {
  need kind; need docker
  if ! cluster_exists; then
    printf '%s does not exist.\n' "$CLUSTER"
    return
  fi
  assert_owned
  local deleted=0
  for attempt in 1 2; do
    if timeout --foreground 120 kind delete cluster --name "$CLUSTER" --kubeconfig "$KUBECONFIG"; then
      deleted=1
      break
    fi
    # Docker Desktop can report a missing exit event before an asynchronous removal completes.
    for ((wait_attempt=0; wait_attempt<30; wait_attempt++)); do
      if ! node_exists; then
        deleted=1
        break
      fi
      assert_owned
      sleep 2
    done
    [[ "$deleted" == 0 ]] || break
  done
  [[ "$deleted" == 1 ]] || fail "Could not remove $CLUSTER; ownership record retained for a safe retry."
  rm -f -- "$OWNERSHIP_FILE" "$KUBECONFIG"
}

case "${1:-help}" in
  up) kind_up ;;
  down) kind_down ;;
  *) fail 'Supported shared kind actions: up, down.' ;;
esac
