#!/usr/bin/env bash
# Shared development commands. Stage implementation
# belongs to stages/mN/scripts; common kind lifecycle belongs to scripts/kind.sh.
source "$(dirname -- "${BASH_SOURCE[0]}")/common.sh"

case "${1:-help}" in
  help|--help|-h)
    cat <<'HELP'
HugoFleet 操作指南
在專案根目錄的 Ubuntu WSL2／Linux Bash 執行 make <目標名稱>。
例如 make m0-up；工具安裝是 make tools（有 s），沒有 make tool。

第一次使用：從 M0 開始
需要已啟動、可連線的 Docker Linux containers；每個終端都先進專案根目錄。
  1. make tools        下載並校驗固定 Go／kind／kubectl，安裝到 .tools（首次需要）
  2. make doctor       檢查工具版本與 Docker；失敗時先處理顯示的問題
  3. make m0-up        建置映像、建立 M0 kind、載入映像並部署價格 API
                       等待完成並返回命令列；首次建置／啟動可能超過 10 分鐘
  4. make m0-forward   轉送服務至本機 18080；此終端保持執行
  5. 另開 Bash 執行：curl --fail http://127.0.0.1:18080/version
                       預期回傳服務版本；價格計算練習見 stages/m0/README.md
  6. 回轉送終端按 Ctrl+C，再執行 make m0-down 刪除 M0 學習叢集
                       保留工具、主機映像與測試產物

選擇階段：一次練習一個，先停止上一階段，再依該階段 README 操作
  make m0-up   價格 API、原生 Deployment／Service
  make m1-up   Application CRD、Operator、漂移修復
  make m2-up   Hub UI、SQLite、發布與回滾（前景 Hub，需保留終端）
  make m3-up   雙叢集 Agent、gRPC、mTLS（背景 Hub）
  make m4-up   OIDC 登入、租戶授權與配額
  make m5-up   Dashboard、trace／metrics、Helm
  停止：make mN-down，例如 make m1-down；刪除該階段擁有的叢集，保留 DB 等資料。
  M0／M1／M2 轉送：make mN-forward，例如 make m1-forward；Ctrl+C 停轉送。
  M2～M5 啟動後仍須依教學登入或建立應用；M3～M5 轉送依該階段教學。
  M2／M3 UI：http://127.0.0.1:18090；M4 UI：http://127.0.0.1:18094。
  詳細流程與重建資料方式：stages/README.md、stages/mN/README.md。

開發與驗證：以下名稱都以 make 開頭，初次學習不必全部執行
  help                  顯示本指南
  fmt                   執行 gofmt，會修改 Go 原始碼
  lint                  檢查格式、go vet、Bash 語法
  build                 編譯各階段主機程式到 bin，不建 Docker 映像
  docs-check            檢查文件連結、Bash 範例語法與指定 JSON
  m0-build～m5-build     編譯指定階段，例如 make m0-build
  m0-test～m5-test       測試指定階段，例如 make m0-test
  test-unit             全專案單元測試
  envtest               準備 Controller 整合測試所需 Kubernetes 工具
  test-integration      執行 M1 Controller 真實 envtest 整合測試
  test-race             單元／整合 race 測試，需先準備 envtest
  test-e2e-m0 / test-e2e-m1 / test-e2e / test-e2e-m3 / test-e2e-m4 / test-e2e-m5
                        分別驗證 M0／M1／M2／M3／M4／M5，會建立真實測試叢集
  test-oidc             真實 Dex 負向測試，含約 15 分鐘 JWT 自然到期測試
  benchmark             價格 API benchmark

進階操作：先讀對應教學，不需要在 up 前逐一執行
  kind-up / kind-down   建立／刪除 HF_CLUSTER 指定叢集，預設 hugofleet-a
                        不會自動選 M0；M0 學習請用 make m0-up／make m0-down
  demo-image-build / demo-image-load   建置價格 API 映像／載入指定 kind
  operator-tools        安裝 Controller 開發工具
  generate / manifests / verify-generated   生成程式／CRD RBAC／核對生成檔
  operator-image-build / operator-install   建置／安裝 Operator
  m3-hub-up / m3-hub-stop / m3-hub-recreate   啟動／停止／重建 M3 Hub
  m4-hub-up / m4-hub-stop / m4-hub-recreate   啟動／停止／重建 M4 Hub
  m5-hub-up / m5-hub-stop / m5-hub-recreate   啟動／停止／重建 M5 Hub
  m4-status / m5-status   查看對應階段狀態
  m4-forward / m5-forward   轉送應用，參數見對應階段 README
  agent-install / certs-dev   M3 Agent 安裝／開發憑證準備
  proto-tools / proto / proto-check   安裝 Proto 工具／生成／核對生成碼
  helm-tools / m5-chart-test   安裝固定 Helm／執行 chart CLI 整合測試
  cache-native          選用：將 Go 快取放到 WSL Linux filesystem
  斜線分隔不同目標，例如 make kind-up；mN 的 N 要換成階段數字。

完整操作與問題排查：stages/README.md、stages/mN/README.md
共用命令：scripts/README.md；驗證範圍與限制：docs/validation.md
HELP
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
  benchmark) exec bash stages/m0/scripts/control.sh benchmark ;;
  operator-image-build) exec bash stages/m1/scripts/control.sh image-build ;;
  operator-install) exec bash stages/m1/scripts/control.sh install ;;
  operator-tools) exec bash stages/m1/scripts/tools.sh ;;
  envtest) exec bash stages/m1/scripts/tools.sh envtest ;;
  generate|manifests|verify-generated) exec bash stages/m1/scripts/control.sh "$1" ;;
  test-integration) exec bash stages/m1/scripts/control.sh integration ;;
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
