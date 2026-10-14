#!/usr/bin/env bash
source "$(dirname -- "${BASH_SOURCE[0]}")/../../../scripts/common.sh"

case "${1:-help}" in
  build)
    need go; mkdir -p bin
    CGO_ENABLED=0 go build -trimpath -o bin/pricing-api ./stages/m0/pricing-api
    build_checker
    ;;
  image-build)
    need docker; need timeout
    for version in v1 v2-good v2-broken; do
      timeout --foreground 1800 docker build --platform linux/amd64 --build-arg "APP_VERSION=$version" \
        -f stages/m0/pricing-api/Dockerfile -t "pricing-api:$version" .
    done
    ;;
  image-load)
    assert_owned
    for version in v1 v2-good v2-broken; do docker image inspect "pricing-api:$version" >/dev/null; done
    timeout --foreground 300 kind load docker-image pricing-api:v1 pricing-api:v2-good pricing-api:v2-broken --name "$CLUSTER"
    ;;
  forward)
    assert_owned
    kube -n hf-demo port-forward --address 127.0.0.1 service/pricing-api "${HF_PORT:-18080}:8080"
    ;;
  test) need go; go test -count=1 ./stages/m0/... ;;
  benchmark) need go; go test -run '^$' -bench BenchmarkCalculate -benchmem ./stages/m0/pricing-api/internal/pricing ;;
  deploy)
    assert_owned
    kube apply -f stages/m0/workload.yaml
    kube -n hf-demo rollout status deployment/pricing-api --timeout=180s
    ;;
  *) fail "Unknown stage action: ${1:-help}. See this stage's README.md." ;;
esac
