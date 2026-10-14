#!/usr/bin/env bash
source "$(dirname -- "${BASH_SOURCE[0]}")/../../../scripts/common.sh"

case "${1:-help}" in
  build)
    need go; mkdir -p bin
    CGO_ENABLED=0 go build -trimpath -o bin/operator ./stages/m1/cmd/operator
    ;;
  image-build)
    need docker; need timeout
    timeout --foreground 1800 docker build --platform linux/amd64 -f stages/m1/build/operator.Dockerfile -t hugofleet-operator:m1 .
    ;;
  install)
    assert_owned
    docker image inspect hugofleet-operator:m1 >/dev/null
    timeout --foreground 300 kind load docker-image hugofleet-operator:m1 --name "$CLUSTER"
    kube apply -f stages/m1/config/crd/bases
    kube wait --for=condition=Established crd/applications.platform.hugofleet.io --timeout=60s
    kube apply -f stages/m1/config/operator/install.yaml
    kube apply -f stages/m1/config/rbac
    # Local m1 tag can be rebuilt; restart so existing Pods use the loaded image.
    kube -n hf-demo rollout restart deployment/hugofleet-operator
    kube -n hf-demo rollout status deployment/hugofleet-operator --timeout=180s
    ;;
  deploy)
    assert_owned
    kube apply -f stages/m1/application.yaml
    generation="$(kube -n hf-demo get application pricing-api -o jsonpath='{.metadata.generation}')"
    kube -n hf-demo wait --for="jsonpath={.status.observedGeneration}=$generation" application/pricing-api --timeout=180s
    kube -n hf-demo wait --for=condition=Ready application/pricing-api --timeout=180s
    ;;
  generate)
    need controller-gen
    controller-gen object paths=./stages/m1/api/...
    ;;
  manifests)
    need controller-gen
    controller-gen crd rbac:roleName=hugofleet-operator paths=./stages/m1/api/... paths=./stages/m1/controller/... \
      output:crd:artifacts:config=stages/m1/config/crd/bases output:rbac:artifacts:config=stages/m1/config/rbac
    ;;
  verify-generated)
    need controller-gen
    mkdir -p "$ROOT/.local"
    temp_dir="$(mktemp -d "$ROOT/.local/generated-XXXXXX")"
    trap 'case "$temp_dir" in "$ROOT"/.local/generated-*) rm -rf -- "$temp_dir" ;; *) fail "Unsafe generated directory" ;; esac' EXIT
    mkdir -p "$temp_dir/stages/m1/api/v1alpha1" "$temp_dir/crd" "$temp_dir/rbac"
    controller-gen object paths=./stages/m1/api/... output:object:dir="$temp_dir/stages/m1/api/v1alpha1"
    controller-gen crd rbac:roleName=hugofleet-operator paths=./stages/m1/api/... paths=./stages/m1/controller/... \
      output:crd:artifacts:config="$temp_dir/crd" output:rbac:artifacts:config="$temp_dir/rbac"
    diff -u stages/m1/api/v1alpha1/zz_generated.deepcopy.go "$temp_dir/stages/m1/api/v1alpha1/zz_generated.deepcopy.go"
    diff -ru stages/m1/config/crd/bases "$temp_dir/crd"
    diff -ru stages/m1/config/rbac "$temp_dir/rbac"
    ;;
  integration)
    need setup-envtest
    export KUBEBUILDER_ASSETS
    KUBEBUILDER_ASSETS="$(setup-envtest use "$ENVTEST_K8S_VERSION" -i --bin-dir "$ROOT/.tools/envtest" -p path)"
    go test -tags=integration -count=1 -timeout=10m -v ./stages/m1/controller
    ;;
  test) need go; go test -count=1 ./stages/m1/... ;;
  *) fail "Unknown stage action: ${1:-help}. See this stage's README.md." ;;
esac
