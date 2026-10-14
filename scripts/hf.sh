#!/usr/bin/env bash
# Shared development commands and compatibility aliases. Stage implementation
# belongs to stages/mN/scripts; common kind lifecycle belongs to scripts/kind.sh.
source "$(dirname -- "${BASH_SOURCE[0]}")/common.sh"

case "${1:-help}" in
  help|--help|-h)
    printf '%s\n' 'M0-M5 commands (make <command>):' \
      'm0-up m0-forward m0-down / m1-up m1-forward m1-down / m2-up m2-forward m2-down' \
      'm3-up m3-down m3-hub-up m3-hub-stop m3-hub-recreate agent-install certs-dev proto-tools proto proto-check test-e2e-m3' \
      'm0-build m0-test / m1-build m1-test / m2-build m2-test / m3-build m3-test' \
      'm4-up m4-down m4-hub-up m4-hub-stop m4-hub-recreate m4-status m4-forward m4-build m4-test test-e2e-m4' \
      'm5-up m5-down m5-hub-up m5-hub-stop m5-hub-recreate m5-status m5-forward m5-build m5-test helm-tools m5-chart-test test-e2e-m5' \
      'tools doctor fmt lint build docs-check test-unit test-race benchmark' \
      'operator-tools generate manifests verify-generated envtest test-integration' \
      'test-oidc (real Dex negatives and natural 15-minute JWT expiry)' \
      'cache-native (optional WSL build cache on Linux filesystem)' \
      'kind-up kind-down demo-image-build demo-image-load operator-image-build operator-install' \
      'demo-deploy demo-forward demo hub-bootstrap hub-up test-e2e test-e2e-m1 test-e2e-m0'
    ;;
  doctor)
    require_versions; need docker; need make; need timeout
    go version; kind version; kubectl version --client -o yaml
    docker version --format 'Docker client={{.Client.Version}} server={{.Server.Version}} OS={{.Server.Os}}'
    [[ "$(docker info --format '{{.OSType}}')" == linux ]] || fail 'Docker must use Linux containers.'
    printf 'Project cluster=%s; isolated kubeconfig=%s\n' "$CLUSTER" "$KUBECONFIG"
    ;;
  fmt)
    need go
    gofmt -w stages tools
    ;;
  lint)
    need go
    unformatted="$(gofmt -l stages tools)"
    [[ -z "$unformatted" ]] || fail "Unformatted Go files: $unformatted"
    go vet ./...
    for script in scripts/*.sh stages/*/*.sh stages/*/scripts/*.sh; do bash -n "$script"; done
    ;;
  build)
    bash stages/m0/scripts/control.sh build
    bash stages/m1/scripts/control.sh build
    # M3 build also builds its shared M2 Hub.
    bash stages/m3/scripts/control.sh build
    bash stages/m4/scripts/control.sh build
    bash stages/m5/scripts/control.sh build
    ;;
  docs-check) need go; go run ./tools/doccheck ;;
  test-unit) need go; go test -count=1 ./... ;;
  test-race)
    need go; need setup-envtest
    race_assets="$(setup-envtest use "$ENVTEST_K8S_VERSION" -i --bin-dir "$ROOT/.tools/envtest" -p path)"
    if command -v gcc >/dev/null || command -v cc >/dev/null; then
      KUBEBUILDER_ASSETS="$race_assets" CGO_ENABLED=1 go test -race -tags=integration -count=1 -timeout=10m ./...
    else
      need docker; need timeout
      printf 'C compiler unavailable in WSL; running Unit + real envtest race tests in the pinned Go container.\n'
      timeout --foreground 1800 docker run --rm --network none --cpus=4 --memory=4g \
        --name "hugofleet-m2-race-$(date +%s)-$$" \
        --mount "type=bind,source=$ROOT,target=/src,readonly" \
        --mount "type=bind,source=$CACHE_ROOT,target=/cache" \
        -w /src -e GOTOOLCHAIN=local -e GOMAXPROCS=4 \
        -e GOMODCACHE=/cache/go-mod -e GOCACHE=/cache/go-race \
        -e "KUBEBUILDER_ASSETS=/src/${race_assets#"$ROOT/"}" \
        "$GO_BUILD_IMAGE" go test -mod=readonly -race -tags=integration -count=1 -timeout=10m ./...
    fi
    ;;
  cache-native) exec bash scripts/cache-native.sh ;;
  kind-up) exec bash scripts/kind.sh up ;;
  kind-down) exec bash scripts/kind.sh down ;;
  m0-build|m1-build|m2-build|m3-build|m4-build|m5-build|m0-test|m1-test|m2-test|m3-test|m4-test|m5-test)
    exec bash "stages/${1%%-*}/scripts/control.sh" "${1#*-}"
    ;;
  demo-image-build) exec bash stages/m0/scripts/control.sh image-build ;;
  demo-image-load) exec bash stages/m0/scripts/control.sh image-load ;;
  demo-forward) exec bash stages/m0/scripts/control.sh forward ;;
  benchmark) exec bash stages/m0/scripts/control.sh benchmark ;;
  demo-deploy) exec bash stages/m1/scripts/control.sh deploy ;;
  operator-image-build) exec bash stages/m1/scripts/control.sh image-build ;;
  operator-install) exec bash stages/m1/scripts/control.sh install ;;
  operator-tools) exec bash stages/m1/scripts/tools.sh ;;
  envtest) exec bash stages/m1/scripts/tools.sh envtest ;;
  generate|manifests|verify-generated) exec bash stages/m1/scripts/control.sh "$1" ;;
  test-integration) exec bash stages/m1/scripts/control.sh integration ;;
  demo) exec bash stages/m2/scripts/control.sh demo ;;
  hub-bootstrap) exec bash stages/m2/scripts/control.sh bootstrap ;;
  hub-up) exec bash stages/m2/scripts/control.sh hub-up ;;
  test-e2e) exec bash stages/m2/scripts/e2e.sh ;;
  test-e2e-m0) exec bash stages/m0/scripts/e2e.sh ;;
  test-e2e-m1) exec bash stages/m1/scripts/e2e.sh ;;
  test-e2e-m3) exec bash stages/m3/scripts/e2e.sh ;;
  test-e2e-m4) exec bash stages/m4/scripts/e2e.sh ;;
  test-e2e-m5) exec bash stages/m5/scripts/e2e.sh ;;
  helm-tools) exec bash stages/m5/scripts/tools.sh ;;
  m5-chart-test) need helm;go test -tags=helmintegration -count=1 -timeout=3m ./stages/m5/test/chart ;;
  m0-*|m1-*|m2-*|m3-*|m4-*|m5-*) exec bash "stages/${1%%-*}/run.sh" "${1#*-}" ;;
  agent-install|certs-dev) exec bash stages/m3/run.sh "$1" ;;
  proto-tools) exec bash stages/m3/scripts/proto.sh tools ;;
  proto) exec bash stages/m3/scripts/proto.sh generate ;;
  proto-check) exec bash stages/m3/scripts/proto.sh check ;;
  *) fail "Unknown command: $1. Run make help." ;;
esac
