#!/usr/bin/env bash
source "$(dirname -- "${BASH_SOURCE[0]}")/../../../scripts/common.sh"

case "${1:-help}" in
  build)
    need go; mkdir -p bin
    CGO_ENABLED=0 go build -trimpath -o bin/hub ./stages/m2/cmd/hub
    CGO_ENABLED=0 go build -trimpath -o bin/bootstrap-hub ./stages/m2/cmd/bootstrap-hub
    ;;
  bootstrap)
    hub_kubeconfig="${HF_HUB_KUBECONFIG:?Set HF_HUB_KUBECONFIG via stages/m2/run.sh or the M2 restart instructions.}"
    assert_owned
    kube apply -f stages/m2/config/hub/rbac.yaml
    CGO_ENABLED=0 go build -trimpath -o bin/bootstrap-hub ./stages/m2/cmd/bootstrap-hub
    "$ROOT/bin/bootstrap-hub" --kubeconfig "$KUBECONFIG" --context "kind-$CLUSTER" \
      --output "$hub_kubeconfig"
    ;;
  hub-up)
    hub_kubeconfig="${HF_HUB_KUBECONFIG:?Set HF_HUB_KUBECONFIG via stages/m2/run.sh or the M2 restart instructions.}"
    hub_database="${HF_HUB_DATABASE:?Set HF_HUB_DATABASE via stages/m2/run.sh or the M2 restart instructions.}"
    assert_owned
    [[ -f "$hub_kubeconfig" ]] || fail 'Prepare the restricted kubeconfig with stages/m2/run.sh up or the M2 restart instructions.'
    CGO_ENABLED=0 go build -trimpath -o bin/hub ./stages/m2/cmd/hub
    printf 'Open http://127.0.0.1:18090 to deploy. Ctrl+C stops the Hub; SQLite history remains.\n'
    exec "$ROOT/bin/hub" --kubeconfig "$hub_kubeconfig" \
      --database "$hub_database" --policy "${HF_HUB_POLICY:-$ROOT/stages/m2/policy.json}"
    ;;
  test) need go; go test -count=1 ./stages/m2/... ;;
  *) fail "Unknown stage action: ${1:-help}. See this stage's README.md." ;;
esac
