#!/usr/bin/env bash
source "$(dirname -- "${BASH_SOURCE[0]}")/../../../scripts/common.sh"

case "${1:-help}" in
  build)
    need go; mkdir -p bin
    CGO_ENABLED=0 go build -trimpath -o bin/hub ./stages/m2/cmd/hub
    CGO_ENABLED=0 go build -trimpath -o bin/bootstrap-hub ./stages/m2/cmd/bootstrap-hub
    ;;
  bootstrap)
    assert_owned
    kube apply -f stages/m2/config/hub/rbac.yaml
    CGO_ENABLED=0 go build -trimpath -o bin/bootstrap-hub ./stages/m2/cmd/bootstrap-hub
    "$ROOT/bin/bootstrap-hub" --kubeconfig "$KUBECONFIG" --context "kind-$CLUSTER" \
      --output "${HF_HUB_KUBECONFIG:-$ROOT/.local/hub-kubeconfig-$CLUSTER}"
    ;;
  hub-up)
    assert_owned
    hub_kubeconfig="${HF_HUB_KUBECONFIG:-$ROOT/.local/hub-kubeconfig-$CLUSTER}"
    [[ -f "$hub_kubeconfig" ]] || fail 'Run make hub-bootstrap first.'
    CGO_ENABLED=0 go build -trimpath -o bin/hub ./stages/m2/cmd/hub
    printf 'Open http://127.0.0.1:18090 to deploy. Ctrl+C stops the Hub; SQLite history remains.\n'
    exec "$ROOT/bin/hub" --kubeconfig "$hub_kubeconfig" \
      --database "${HF_HUB_DATABASE:-$ROOT/.local/hub/$CLUSTER.db}" --policy "${HF_HUB_POLICY:-$ROOT/stages/m2/policy.json}"
    ;;
  demo)
    bash scripts/hf.sh doctor
    bash scripts/hf.sh demo-image-build
    bash scripts/hf.sh operator-image-build
    bash scripts/kind.sh up
    bash scripts/hf.sh demo-image-load
    bash scripts/hf.sh operator-install
    bash scripts/hf.sh hub-bootstrap
    bash scripts/hf.sh hub-up
    ;;
  test) need go; go test -count=1 ./stages/m2/... ;;
  *) fail "Unknown stage action: ${1:-help}. See this stage's README.md." ;;
esac
