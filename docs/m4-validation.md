# M4 驗收紀錄

開始日期：2026-10-12；完成日期：2026-10-13，Asia/Taipei。狀態：**本機驗收通過**。Hosted CI 尚未執行。

M0～M3 的檢查、修正與重新驗證已完成，證據見 [搬移後重新驗證](revalidation-20261011.md)。本階段依使用者授權實作 OIDC、資料庫角色授權、租戶 namespaces／RBAC、真實 Quota／LimitRange、NetworkPolicy 及稽核；M4 通過後才開始 M5。

## 最小垂直切片與變更計畫

1. 在 stages/m4 新增 OIDC／server-side session、管理 API／Go template 頁面、獨立 Hub／bootstrap／Demo 工具與正式操作入口。
2. 共用 repository 增加安全 migration、issuer + sub 使用者、membership、tenant profile、session／登入交易與 audit；在交易內授權，禁止舊模式開啟安全 DB。
3. 共用 Application transport／Agent／型別化 Proto 增加受信任租戶 allowlist、namespace／ledger 綁定，以及唯讀的真實 bootstrap profile 核對；維持 M0～M3 的原本行為與入口。
4. 獨立 Go bootstrap 建立精確 namespaces／RBAC／quota／limits／Policy／Operator，管理員 kubeconfig 留在終端；Hub 不掛 Worker 憑證。
5. 加入 enforcing Calico Security Profile，Go probe 真實測試網路，不使用 shell／exec 偽裝連線。

架構與固定版本見 [ADR 0010](adr/0010-oidc-tenants-and-worker-bootstrap.md)。檔案完成後於階段 README 補齊地圖與逐步操作。

## 本輪環境與資源邊界

Windows 專案根目錄 `C:\Users\H\Documents\GitHub\win30221\hugofleet`，Ubuntu WSL2／Linux amd64，以 UID 1000 執行；32 GB／Intel／CPU-only。固定版本為 Go 1.27.2、kind 0.33.0、Kubernetes 1.37.0、kubectl 1.37.1、controller-runtime 0.25.1、grpc-go 1.82.1、SQLite Go driver 1.60.1、Dex 2.46.0、Calico 3.33.0；來源、checksums／digest 以 [versions.env](../scripts/versions.env) 與 [go.mod](../go.mod) 為準。

開發 state 為 `.local/tutorials/m4/development-20261012`，使用獨立 m4-e2e-a／m4-e2e-b Workers 與 development Hub／Dex。Hub 實際 Docker inspect 僅有五個 bind mounts：DB 可寫、可信設定及 CA／Hub 憑證唯讀；不掛 Worker kubeconfig、Worker private key、CA signing key、Docker socket 或 workspace。四個 namespaced Operators 與兩個 hf-system Agents 實際受限運作。Hub image `sha256:94c46c8d8901d4fa7c13fae33d69ae434a552911097056421b6861ea2c8653a9`，Agent image `sha256:ebf77e0c3a10e99be8f9cbd5cddb1b026a7c6eb2ca197cf2a98b4edb4622fcda`。

`final-docker-stats.txt` 是驗收後的一次實際取樣：Hub 約 8.7 MiB、Dex 約 28 MiB、A node 約 1.92 GiB、B node 約 1.21 GiB；不是峰值或效能保證。Hub 限 2 CPU／512 MiB、Dex 0.5 CPU／256 MiB，開發／race 限制及控制流程沒有新增重型常駐觀測系統。

## 測試計畫

- Unit／SQLite：migration 保留歷史、舊模式拒絕、安全角色／撤權／冪等前授權、email 更新保持 sub 身分、最後 Owner 保護、session 過期／登出／audit 範圍與秘密排除。
- 真實 Dex OIDC：Code + PKCE 登入、一次性 state／browser binding／nonce／錯誤 verifier／redirect 拒絕、JWKS／issuer／audience／expiry 驗證；ID／access token 不能作管理 session，所有變更驗 CSRF。
- kind E2E：真實 mTLS Worker Pod、各租戶 CR／Deployment／Service／HTTP、跨租戶讀寫／rollback／Operation／audit 拒絕、精確 SA 權限、CPU 與 memory Admission 拒絕傳到 CR／Hub／UI。
- enforcing CNI：同租戶連線成功、跨租戶隔離，使用實際 Go 網路 probe 與前後正向控制。
- 相關 build／fmt／vet／Unit／race／envtest／Proto 生成一致性，必要的 M0～M3 回歸，完整 README 教學、owned cleanup 與 secrets ignore 核對。

## 本輪實測

開發 state 的真實完整 E2E、文件 lifecycle、最新 M2／M3 共用程式回歸及正式全新 M4 state 入口均已通過。正式入口包含診斷與 owned cleanup，並核對無關 lab 的完整 Container ID／running 狀態不變。

| 實測 | 結果／證據 |
|---|---|
| 安全 SQLite migration、交易內角色、撤權前冪等檢查、session／login transaction／最後 Owner／quota 預檢 | PASS：`artifacts/m4-development/security-core.log`，以及完整 Unit 日誌 |
| M2／M3 共用核心 Unit、tenant allowlist／ledger 綁定、唯讀 Worker profile、Proto 一致性 | PASS：`artifacts/m4-development/shared-core.log`、`tenant-worker-unit.log`、`proto-check.log`；不等於變更後 kind 回歸通過 |
| M4 OIDC／profile／管理 API／實際 SQLite HTML Unit | PASS：`artifacts/m4-development/full-unit.log`、`api-auth-unit-retest.log` |
| 目前完整 Go module Unit、真實 envtest、生成一致性、tagged vet | PASS：`go test -count=1 ./...`；`make test-integration proto-check verify-generated`；`go vet -tags=e2e,oidcintegration,integration ./...` 全 exit 0。envtest 8.77 秒。這次 Unit／envtest stdout 留於 Codex 命令紀錄，`shared-regression-checks.log` 僅為最後 vet 的無錯誤輸出 |
| 六個真實 Dex Code + PKCE 登入與 JWKS／issuer／audience／expiry／nonce 核對 | PASS：`artifacts/m4-development/dex-code-flow.log`；實際 subject 存於被忽略的私有設定 |
| 真實 Dex + loopback Hub 完整登入／session／登出／過期 | PASS：`artifacts/m4-development/real-hub-oidc.log`，6.82 秒；六個帳號的 elevated groups 未提升 DB 角色 |
| 真實 nonce／state／browser binding 拒絕、state 一次性、Dex 錯誤 PKCE／redirect／code replay 拒絕、ID／access Token 與 session 分離 | PASS：同上；沒有以假的 Provider 回應替代 |
| 真實 JWT 錯誤 issuer／audience／破壞簽章／格式拒絕，以及自然過期後拒絕 | PASS：`artifacts/m4-development/real-jwt-expiry.log`，901.21 秒；使用真實 15 分鐘 Token 及實際時鐘 |
| enforcing Calico A Worker | PASS：`artifacts/m4-development/real-security-e2e.log`，實際同租戶 HTTP 成功、跨租戶在移除 Policy 前後皆成功、啟用 Policy 時三次 timeout；Node／CNI rollout 證據另見 `security-worker-bootstrap-retry.log` |
| 全 module Unit + 真實 envtest race | PASS：`artifacts/m4-development/race-envtest.log`，正式 `make test-race`，固定 Go image、唯讀 source、無網路、4 CPU／4 GiB；envtest 18.16 秒。執行期間新增的 callback regression 另做精確 race 重驗 |
| M4 Docker 映像與真實受限 Worker 安裝 | PASS：`docker-images.log`、`worker-install.log`；A／B Nodes Ready、四個 namespaced Operators 與兩個 hf-system Agents rollout 完成，六個 User 真實 Hub 登入／登出。尚不等於租戶安全 E2E 通過 |
| 全文件靜態核對 | PASS：`docs-check.log`，38 Markdown、488 local links、95 shell blocks、3 typed deployment examples；操作證據另以真實 E2E 為準 |
| 開發 state 真實完整 M4 E2E／UI 教學 | PASS：`real-security-e2e.log`，717.22 秒；真實 CSRF／角色／跨租戶／HTTP／RBAC／Calico／CPU／memory／表單／故障與回滾／GC／Agent 斷線重連／Hub 停止時自主漂移修復／原 session 恢復 |
| README 正式 lifecycle 與轉送 | PASS：`documented-lifecycle.log`；正式 `source stages/m4/env.sh`、`make m4-status m4-hub-stop m4-hub-up m4-hub-recreate` 與 stage forward。實際 OIDC user、CR UID、application／release／operation 履歷及兩個 Agent ledger UID／high-water／tombstones 均保留；127.0.0.1:18080 真實 v1 HTTP／quote=8000 |
| Linux 磁碟快取 Docker 掛載 | PASS：`native-disk-docker-modules.log`，固定 Go image、來源及新快取唯讀、無網路、`go list -mod=readonly -m all`。首次離線列舉因部分未用相依的 metadata 未快取而 FAIL（Codex 命令紀錄），由 Go 下載既有固定 graph metadata 後重驗；未更動依賴版本 |
| M4 最後 lint／docs／Proto／CRD 一致性 | PASS：`final-static-checks.log`，正式 `make lint docs-check proto-check verify-generated` exit 0；38 Markdown、493 links、96 shell blocks、3 typed examples |
| 最後 README 補充後文件核對 | PASS：`readme-final-check.log`，38 Markdown、499 local links、96 shell blocks、3 typed examples；另明確區分 profile 核對的 maximum／defaults 與本地 bootstrap 的 minimum |
| Canonical Auth／Calico cache 更新後最後靜態檢查 | PASS：`current-final-static.log`，正式 `make lint docs-check proto-check verify-generated`，38 Markdown、502 local links、97 shell blocks、3 typed examples，exit 0 |
| 實際 Hub Dev Auth 啟動拒絕 | PASS：`real-startup-dev-auth-guard.log`；實際 `bin/m4-hub` 在 `--listen=0.0.0.0:18994` 與 DEV_AUTH_MODE 設定下 exit 1，核對拒絕原因且沒有建立測試 DB／listener |
| Canonical OIDC endpoint 修正的精確 race／vet／build | PASS：`canonical-endpoint-race.log`、`canonical-endpoint-vet.log`；固定 Go Docker image、無網路／受限資源重驗 Auth + HTTP race，tagged M4 vet 及 native Hub build exit 0 |
| 修正後 Auth 的全新真實 Dex backchannel | PASS：`canonical-real-dex.log`，0.148 秒；全新 private accounts／六次 Code/S256 subjects／官方 frontend assets，更新後 Auth 實際 discovery、token exchange、JWKS、nonce／簽章／issuer/sub、SQLite opaque session 與 state replay 拒絕。獨立 DB，不呼叫既有 Hub callback；精確 owned Dex cleanup 完成 |
| 固定 Calico host image cache | PASS：`calico-node-host-pull.log`、`calico-helper-host-pulls.log`；官方 quay.io 的 v3.33.0 三個映像完成下載，node digest `565d8db3200e443955557f8f28ff44ab8a4a456754bc74a19c4f2f3444af9b1c`、calico digest `4ebc20730127bc9e8bf1db5f5a2d8b58cf37fe0e891b3c7b5471992221d213e5`、third-party-cni-plugins digest `0f32f48279484c0d6f0e0d0e767b2d0e1a6c6ca34cb3421a61b989d992f3df41`。尚不等於新 Worker 預載／完整 E2E 通過 |
| 第二次全新 Worker 的 Calico 預載與 readiness | PASS：`artifacts/m4-e2e/20261012T074113-1347260/run.log`，三個固定映像已由 host cache 載入精確 owned A，Calico node／controllers 與 Node Ready。該次入口後續在 CoreDNS 失敗，不因此將完整 E2E 列 PASS |
| 開發 state owned cleanup／資料保留 | PASS：`fresh-and-regression.log`；正式 `make m4-down` 已移除 Hub／Dex／A／B，核對 DB／native DB／accounts／subjects／certs 保留，原 lab container ID／running 狀態相同。Docker A／B 首次 missing exit event，正式同 ownership 重試後成功 |
| 擴大的 canonical wire URL／private Demo gateway Unit | PASS：`canonical-wire-gateway-unit.log`；Auth + HTTP Unit exit 0，包含三種 discovery endpoint 共 30 個負向路徑、公開 IPv4／IPv6 管理來源拒絕及六種合法 private／loopback／空設定。最新精確 race／真實 Dex 另列 |
| 最新 M2 共用程式真實回歸 | PASS：`artifacts/m2-e2e/20261012T054310-1241140/run.log`，353.97 秒，包含 strict quota release／新 revision、SQLite 寫入失敗／SIGKILL／逾時 UID 恢復、rollback／GC／履歷；正式 owned cleanup 完成。M3 不因此列為通過 |
| 最新 M3 共用程式真實回歸 | PASS：`artifacts/m3-e2e/20261012T065025-1300457/run.log`，507.65 秒，真正雙 Worker Pod mTLS、HTTP、離線自主修復／B 持續更新、ACK loss／ledger／tombstone、重連、Hub restart、rollback、terminal late Ready、GC；正式 owned cleanup 完成，原 lab ID／running 相同。A／B 首次 missing exit event，核對原 ID 後第二次移除成功 |
| 最新 Auth 真實 Dex／公開來源 CLI | PASS：`canonical-wire-gateway-real-dex.log`，0.224 秒，重新啟動真實 Dex，沿用私有帳號／subjects；更新後 Code/S256／JWKS／nonce／issuer/sub／session／state replay 均成功驗證，owned Dex 已移除。`public-demo-gateway-cli.log` 確認實際 bootstrap exit 1 拒絕公開 IP，設定未改寫 |
| 最新 wire URL／gateway 合併修正的 race／vet／build | PASS：`canonical-wire-gateway-race.log`、`canonical-wire-gateway-vet.log`；固定 Go image、無網路、4 CPU／4 GiB，Auth 390.883 秒／HTTP 62.781 秒，M4 tagged vet 與 native Hub build exit 0。此耗時包含 race instrumentation，不作一般請求效能指標 |
| M4 OpenAPI 路由與 references | PASS：`openapi-contract-check.log`，Go strict YAML／117 local/external references、22 個真實 API／登入／health routes、unique operation IDs、session 及 mutation Origin／CSRF。契約在 `docs/openapi-m4.yaml`，共用 response models 引用 `docs/openapi.yaml`；此為靜態核對，不替代真實 HTTP 驗收 |
| 最新 Auth／API 契約與 README 的正式靜態檢查 | PASS：`wire-gateway-openapi-static.log`，正式 `make lint docs-check proto-check verify-generated` exit 0；38 Markdown、506 local links、97 shell blocks、3 typed examples |
| 最新 Auth Docker Hub image | PASS：`wire-gateway-hub-image-build.log`／`wire-gateway-hub-image-id.txt`，實際 build exit 0，Linux amd64，image ID `b045b32f8cd0057bcc1e9746b240313398d1862d6328cdec60c32be861f30daa`。完整全新 Docker／kind 驗收仍另列 |
| 啟動等待／API 路徑／19 份 README 說明更新後 | PASS：`bounded-bootstrap-docs-check.log`，正式 `make docs-check` exit 0；38 Markdown、510 local links、97 shell blocks、3 typed examples；此項不執行應用 Demo |
| CoreDNS 真實 Available 等待修正與文件同步 | PASS：`coredns-availability-static.log`，正式 `make lint docs-check` exit 0；38 Markdown、510 local links、97 shell blocks、3 typed examples。後續全新 state 啟動／安全 E2E 結果如下 |
| 全新正式 M4 入口與 owned cleanup | PASS：`make test-e2e-m4`，`artifacts/m4-e2e/20261012T080511-1356534/run.log`，`TestRealTenantSecurity` 766.55 秒；兩個全新 Worker、Calico cache 預載／CoreDNS 完整 rollout、六個真實 OIDC subjects、雙 Worker mTLS、角色／CSRF／稽核、實際封包、CPU／memory Admission、完整 UI／rollback／GC／離線自主修復與 Hub restart。A／B 首次移除遇到 missing exit event，正式 ownership 核對後第二次清理成功，整個入口 exit 0 |
| 正式驗收資源邊界 | PASS：`artifacts/m4-development/fresh-formal-dns-retry.log` exit 0，原 lab ID／running 未變；Hub／Dex／兩個 owned Workers 已移除，私有 DB／accounts／subjects／credentials 留於 `.local/m4-e2e-1791763511-1356534`。Hub 為最新 `sha256:b045b32f8cd0057bcc1e9746b240313398d1862d6328cdec60c32be861f30daa`，包含 canonical wire URL 與 private gateway 修正 |
| M4 交付門檻的最後靜態檢查 | PASS：`gate-final-static-20261013.log`，正式 `make lint docs-check proto-check verify-generated` exit 0；38 Markdown、510 local links、97 shell blocks、3 typed examples。之後才開始 M5 |

保留的失敗與修正：首次安全核心編譯使用不存在的 `crypto/rand.Bytes`，改為標準 `rand.Read` 後重測通過；首次 HTML 測試錯把一般詳情頁當作確認頁，改為實際 GET `?confirm=delete` 並驗證 confirmation form 後重測通過。相關 FAIL 原始日誌保留。Docker 啟動腳本改用獨立 Bash 檔避免 Windows／WSL 引號傳遞丟失變數；失敗前並未啟動容器。首次 Calico 冷啟動下載超過原本 600 秒等待，保留 FAIL，增加有界冷啟動等待與同一 owned Worker 的版本核對／重試，不重建控制平面、不改弱隔離 Assertion。

審查再發現缺少或短格式 browser binding 時，callback／Store 會提早拒絕，卻未消耗已知 state；補回原 binding 後仍可重送。新增 SQLite regression 已重現 FAIL（`login-binding-regression-before.log`），修正為先消耗已知登入交易，再拒絕任何缺失／錯誤 binding；相關 SQLite／OIDC／HTTP Unit 已 PASS（`login-binding-regression-after.log`、`callback-binding-http-unit.log`），新增修正精確 race 也 PASS（`binding-fix-race.log`）。以更新後 Docker Hub、真實 Dex 及獨立 5 秒 session TTL 重驗完整登入流程：缺失／短 binding 與補回原 cookie 後的 state replay 均拒絕，其餘 Code／PKCE／session 負向維持 PASS（`real-hub-oidc-binding-retest.log`，7.01 秒）。之後恢復正常 session TTL，再進行 Worker 安全 E2E。

後續審查又重現 discovery endpoint 僅做 path prefix 判斷的問題：authorization／token／JWKS 三種 endpoint 的 `/../outside`、`/%2e%2e/outside`、`//outside` 共九項原本被接受（`discovery-path-regression-before.log`，FAIL）。修正為 discovery 與每次 backchannel 都驗證 canonical wire URL，另加入 `/./outside` 與 `/%2foutside`；15 項 regression 與 Auth／HTTP Unit 全 PASS（`discovery-path-regression-after.log`）。此項 Unit 使用明確的 discovery fixture，真實 Dex 重驗另列，尚未以 Unit 代替實際 Provider。

該修正後以 `.local/m4-auth-canonical-20261012` 的全新實際 Dex 成功重驗，命令為 `HF_M4_STATE=... go test -tags=oidcintegration -count=1 -timeout=2m -run TestRealDexCanonicalBackchannel -v ./stages/m4/test/oidc`；精確結果見上表。原開發 E2E 的 Hub digest 是歷史版本，最新完整 Docker／kind E2E 仍以後續全新入口結果為準。

再擴大 wire URL regression 時，`RawPath` 對標準 escaping 可能為空，雙重編碼、標準大寫反斜線 escaping 與空白仍通過原檢查。30 個負向案例中重現 12 個 FAIL（`canonical-wire-encoding-uppercase-before.log`；較早 27 項版本見 `canonical-wire-encoding-before.log`）。補上 `EscapedPath() == Path` 後，Auth／HTTP Unit 全 PASS（`canonical-wire-encoding-after.log`）。另重現 `HTTPClientIP` 接受公開 IPv4／IPv6（`public-demo-gateway-before.log`），修正成 private／loopback；最新合併 Unit 亦 PASS。這些是配置／discovery fixture 的邊界 regression，不宣稱曾在真實 Dex 上利用路徑越界。

驗收工作中曾在 `control.sh` 仍執行時調整重複編譯，導致原 Bash process 讀到位移後的檔案，在所有 Worker rollout 完成後中斷（`hub-normal-session-recreate.log`）。現行檔案 `bash -n` PASS；保留該 FAIL，待原程序 exit 後用穩定入口重跑同一 owned Workers，不重建 cluster／DB、不將未開始的 E2E 宣稱通過。安裝入口目前只編譯一次同一 Go bootstrap，避免在 Windows 掛載上六次重複掃描套件。

穩定入口重新安裝及 `status` 已成功，之後 E2E 編譯遇到 SQLite compiler `no space left on device`，尚未開始測試；失敗 stdout 保留於 Codex 命令紀錄。WSL `/tmp` 實際為 7.8 GiB tmpfs，當時僅剩約 627 MiB。將本專案已核對 ownership 的可再生快取移到 `/var/tmp` Linux 磁碟，並由正式 `cache-native`／common 入口設定專用 `GOTMPDIR`；不清理其它暫存。跨 filesystem move 曾因唯讀 module 目錄無法移除來源殘餘，核對目的 SDK 與專案 ownership 後只清理精確來源快取，保留其它 `/tmp`、舊快取及私有資料。正式 `make cache-native` 重複執行與 `make doctor` PASS（`native-disk-cache-doctor.log`），新快取的 E2E 編譯也 PASS（`native-disk-e2e-build.log`）。Docker 掛載另做獨立驗證；這些環境修正不改變測試 assertions。

真實雙 Worker E2E 已完整通過上述安全項目及實際 UI 教學／故障恢復，開發證據為 `real-security-e2e.log`。後續全新 state 的正式入口亦已通過；前兩次全新入口的 FAIL 與修正保留如下。

第一次全新正式 `make test-e2e-m4` 的 state 為 `.local/m4-e2e-1791752482-1221013`，原始日誌在 `artifacts/m4-e2e/20261012T050122-1221013/run.log`。同一 A 控制平面雖成功啟動，Calico node 的大型 layer 在 CRI 下載 11 分鐘後仍未完成，20 分鐘 rollout 逾時；未啟動 Dex／Hub／B、未開始安全 E2E，結果為 FAIL。保留 Pod／image pull／containerd 觀測於 `fresh-calico-observation.log`；沒有磁碟不足（節點磁碟約 925 GiB 可用），先前可觀察到的 fdatasync／ext4／jbd2 wait 不是 Windows／Docker 根因的證明。正式 trap 完成 owned cleanup；Docker 第一次缺少 exit event，第二次 ownership 核對後成功清除。後續修正為主機 Docker 快取固定 Calico images、載入 owned Worker，再維持原本 readiness／封包 assertions；重驗另列，不將此 FAIL 覆寫成 PASS。當時 M2／M3 回歸尚未開始，最新結果另列於上表。

Dex／Hub HTTP loopback 是明確的 LocalDemo 例外；正常部署要求 HTTPS 與 Secure cookie。密碼、OIDC Client Secret、Token、Worker 私鑰與 SQLite 不輸出到驗收日誌，也不納入 Git。

第二次全新 M4 入口 state 為 `.local/m4-e2e-1791762073-1347260`，日誌為 `artifacts/m4-e2e/20261012T074113-1347260/run.log`。Calico 預載、官方 manifest、node／controllers 與 Node readiness 均完成，但 CoreDNS 立即回報 ProgressDeadlineExceeded，入口 FAIL；B／Dex／Hub 尚未啟動，安全測試未開始。快照顯示兩個 CoreDNS Pods 仍 ContainerCreating，因 Calico nodename 尚未產生曾重試 sandbox；CNI ready 後映像已在節點、Pods 正在啟動。CoreDNS 在 kind 建立時已存在，CNI 預載／啟動消耗其原本十分鐘 progress budget。正式 trap 已完成 owned A cleanup，B 不存在；保留本次 state／Pod／事件診斷，未聲稱不存在的 DB／帳號已建立。修正為 Node／CNI Ready 後先有界等待真實 CoreDNS Available，再執行原本完整 rollout 檢查；沒有改寫 status、progressDeadline 或放寬應用 assertions。後續新 state 重驗另外記錄。

最新共用程式的第一次 M3 正式回歸在 `artifacts/m3-e2e/20261012T061159-1269400/run.log` 為 FAIL：A 的 Node／CoreDNS 已就緒，B 的 kubeadm admin bootstrap 在 10 分鐘內始終遇到 API connection refused；沒有啟動 Hub／Agent 功能測試。唯讀 CRI 快照顯示尚無 workload container，containerd CreateContainer 持續進行、後續重試遇到 name reserved；I/O pressure full 約 61%，這不是宿主磁碟根因的證明。診斷另留 `latest-m3-b-bootstrap-diagnostic.log`。正式 trap 完成 A／B owned cleanup，A 第一次 missing exit event、第二次成功；無關 lab 保持原 ID／running。M2 PASS 保留，不因本次 M3 失敗改寫。依 [kubeadm 官方 timeouts 欄位](https://kubernetes.io/docs/reference/config-api/kubeadm-config.v1beta4/#timeouts)，將 API／runtime 等待調為 15／10 分鐘，kind 整體上限 30 分鐘，後續順序重驗 M3 與全新 M4。沒有關閉 fsync、移除 readiness 或修改應用 assertions。

後續重跑的 A／B Nodes／CoreDNS、Operator 與真正 Worker Pod mTLS 通道已就緒，`TestM3DualKindOutboundAgents` 完整功能 PASS，507.65 秒；包括雙 Worker HTTP、離線修復、重連、durable retries、rollback、late Ready 與 GC。原始結果在 `artifacts/m3-e2e/20261012T065025-1300457/run.log`；A／B 正式 owned cleanup 均已完成，整個入口 PASS，順序腳本另核對原 lab ID／running 相同，接著啟動全新 M4。唯讀補充 kernel stack／syscall／fd 對照，確認 containerd 2.3.4 曾等待 overlayfs `metadata.db` 及 bolt `meta.db` 的 fdatasync（`latest-m3-retry-a-kernel-stack.log`、`latest-m3-retry-b-flush-target-v2.log`）。這只是實際等待位置；沒有推論特定宿主原因，也沒有改變 metadata sync 設定。

實際瀏覽器畫面保存在 `artifacts/m4-development/admin-login-browser.jpg`、`dual-ready-browser.jpg`、`memory-admission-history-browser.jpg`；分別呈現已驗證 Admin、真實雙 Worker CONNECTED，以及記憶體 Admission 的 FAILED／新 revision 恢復／GC 履歷。沒有將私有帳號設定或密碼帶入畫面。全新 Demo 設定明確使用官方 Dex image 的 `/srv/dex/web` UI assets 路徑，依據為 [固定 tag Dockerfile](https://raw.githubusercontent.com/dexidp/dex/v2.46.0/Dockerfile)。

真實 OIDC 使用獨立 native Hub、原本 private settings 及 5 秒 session TTL，沒有替代 Provider 回應：

```bash
cd /mnt/c/Users/H/Documents/GitHub/win30221/hugofleet
source scripts/common.sh
export HF_M4_STATE="$PWD/.local/tutorials/m4/development-20261012"
# 私有 accounts／subjects 已經過真實 Dex 登入核對後：
go run ./stages/m4/cmd/bootstrap-hub --state "$HF_M4_STATE" \
  --clusters hugofleet-m4-e2e-a,hugofleet-m4-e2e-b
bin/m4-hub --config "$HF_M4_STATE/hub-config.json" \
  --database "$HF_M4_STATE/native-db/hub.db" --certs "$HF_M4_STATE/certs" --session-ttl=5s
# 另一個終端，使用相同 HF_M4_STATE：
go test -tags=oidcintegration -count=1 -timeout=5m -run TestRealDexAndHubOIDC -v ./stages/m4/test/oidc
go test -tags=oidcintegration -count=1 -timeout=18m -run TestRealSignedTokenExpiry -v ./stages/m4/test/oidc
```

後續 Docker／kind 使用不同的 security DB；native Hub 已核對精確 process／DB 路徑後正常 SIGTERM 停止。Private settings／DB 保留；沒有停止 Dex 或其它使用者程序。

## M5 開發期間發現的共用瀏覽器修正

2026-10-13 M5 真實瀏覽器操作發現，共用受保護頁面的 Referrer-Policy: no-referrer 使 HTML navigation POST 的 Origin 成為 null，登出因而被精確 Origin 檢查拒絕。先前 HTTP driver 手動送 Origin，沒有覆蓋這個瀏覽器行為。已將受保護頁面改為 same-origin，OAuth 公開回應仍 no-referrer；Origin／CSRF 拒絕規則不變。Unit red→green、全 module race／envtest 及 M5 真實瀏覽器登出／變更流程通過；證據見 [M5 紀錄](m5-validation.md)。本文件先前的 M4 正式 PASS 保留原次實測範圍，最新共用程式的 M4 正式回歸仍待執行。

2026-10-14 最新 M4 正式 Run artifacts/m4-e2e/20261014T094026-110240 運作期間，另外以真實 in-app browser 從 http://127.0.0.1:18094/login 點擊 OIDC 登入，使用本輪私有生成的 Demo admin 登入本機 Dex，返回 M4 已驗證使用者頁，兩個 Workers 均 CONNECTED。點擊真正 HTML 登出 form 後正常返回 OIDC 登入頁，沒有 Origin 403；未修改租戶／membership／配額，僅本次瀏覽器 session 登出，測試 driver 的 session 不受影響，臨時頁籤已關閉。截圖 artifacts/m5-development/m4-browser-authenticated.png、m4-browser-logout.png。完整 M4 正式結果另行補齊，瀏覽器分項不等於整輪 PASS。

## 2026-10-14 最新完整正式回歸通過

make test-e2e-m4 **PASS，exit 0**，TestRealTenantSecurity 408.42 秒，artifacts/m4-e2e/20261014T094026-110240/run.log，state .local/m4-e2e-1791942026-110240。此輪使用最新共用 HTTP policy、schema 5、Proto／完整觀測、目前 ReplicaSet／ProgressDeadlineExceeded 歸屬及唯一 IPv4 gateway 修正，原安全與業務 assertions 保留。

真實六個 Dex 身分、雙 Worker mTLS、CSRF／Origin 拒絕、tenant／Viewer／撤權／可信 actor／audit、namespaced SA 必要權限與 ReplicaSet 禁寫／跨 namespace 拒絕、實際 Calico 封包、CPU／memory Pod Admission 拒絕＋新 revision 恢復／GC；實際 HTML 部署／good／replicas drift／broken FAILED 履歷／確認 rollback 新 revision SUCCEEDED＋HTTP／確認 delete＋GC；B offline／reconnect／服務不中斷、原 DB／session Hub restart、無 Hub 時 Operator 自主修復全部通過。

正式 cleanup 清理本輪 owned Hub／Dex／雙 Workers，私有 DB／subject／certs 保留，外層其他容器名稱／ID／狀態比對不變，final-m2-m4-forward-regressions.log 最後 exit 0。最新 M1／M2／M3／M5 同樣通過，彙總見 [M5 紀錄](m5-validation.md)。本機不等於 Hosted CI，尚未 commit／push 或執行線上 CI。
