# M0～M5 修正、完整驗證與 Hosted CI（2026-10-14）

本輪依使用者最新授權，先修正 [文件與功能稽核](audit-m0-m5-20261014.md) 的問題，完成本機驗證後才提交與推送既有 origin，實際測試 GitHub Actions。歷史驗收結果維持原意；本文件只記錄本輪新執行的結果。

**2026-10-15 完成：F01～F10 已修正，本機正式驗證與第二輪 Hosted CI 全部九個 jobs 通過。** 遠端測試提交為 e8d20d6cc6e6126cbb90a54ea00559d927460cac；首次 CI 的兩項失敗及修復證據保留於下方。原地憑證輪替、等待 24 小時到期及額外強制終止注入仍不屬於本輪已驗收項目。

## 修正範圍

- F01：M4 重建 Hub 僅更新受 ownership 保護的 Agent endpoint；M5 沿用 Helm release values 更新 endpoint，保留 quota／receipt／ledger／rollout。真實雙 Worker E2E 增加自訂配額與 Hub profile 一致性回歸。
- F02～F05：補齊 E2E 殘留的精確資源名稱、獨立測試工具前置、revision／ReplicaSet 現行行為、開發憑證期限及新 state 恢復說明。
- F06：新增 make test-oidc，獨立真實 Dex／短 session Hub、負向認證與約 15 分鐘 JWT 自然到期；GitHub Actions 加入 oidc job。
- F07：Hub 心跳閾值與 Agent 心跳／重連退避提供有界 CLI 設定；預設維持 15s／5s／1s～30s。
- F08：ADR 0011 改記本機已完成，遠端結果另列。

## 本輪驗證進度

| 項目 | 狀態與證據 |
|---|---|
| 格式、lint、Unit、build | PASS，exit 0；初輪 checks.log，整合後 final-checks.log |
| OIDC 真實負向與自然到期 | PASS，exit 0；canonical backchannel 0.18s、JWT 自然到期 900.35s、Hub 負向／session 到期 7.00s，含 owned cleanup；oidc-entry.log |
| M0／M1／M2 正式 kind E2E | PASS，整個正式 Make 命令 exit 0，三階段均含 owned cleanup；M1 lifecycle 227.58s／authorization 0.64s、M2 274.76s；m0-m2.log |
| M3 正式雙 Worker E2E | PASS，含真實 mTLS／斷線重連／ACK 遺失／ledger／Hub 重啟／故障回滾／owned cleanup；m3-m5.log，artifacts/m3-e2e/20261014T174031-10687 |
| M4 正式雙 Worker E2E | PASS，TestRealTenantSecurity 435.69s，含真實 Dex／租戶權限／CPU、memory Admission／Calico 封包／頁面表單／斷線與重啟／自訂配額保留及 owned cleanup；m3-m5.log，artifacts/m4-e2e/20261014T180603-20667 |
| M5 正式 kind E2E | PASS，完整重跑 exit 0；安全與 Hub 重建保留 451.85s，Dashboard／trace／metrics／Helm upgrade-uninstall-reinstall 106.53s，含 owned cleanup；m5-retry.log，artifacts/m5-e2e/20261014T224145-36249 |
| race／envtest／Helm／生成檔／文件檢查 | PASS；doctor verify-generated proto-check test-integration exit 0，controller envtest 10.006s；整合後 lint／Unit／race／Helm CLI／生成檔／docs-check／build 同一命令 exit 0；46 Markdown／635 links／115 shell blocks／3 JSON；integration.log、final-checks.log |
| 提交／推送與 Hosted CI | 首次提交 164cfb2：七個 jobs PASS、兩個 FAIL；修正提交 e8d20d6：第二輪全部九個 jobs PASS，詳見下方 |

上述本機項目為首次推送前的驗收；Hosted CI 發現的新問題已完成修正與重驗，最新結果見下方。原始 artifacts、帳號、DB、憑證與 kubeconfig 均由 Git 排除，不隨原始碼推送。

上述簡寫日誌均位於 artifacts/release-validation。最終整合檢查使用 GOMAXPROCS=2、GOFLAGS=-p=2，與 CI 的主機 Go 並行度一致；這不代表本機硬體等同 Hosted runner。WSL 無 C compiler，race 依正式入口使用固定 Go 容器與真實 envtest。

提交前文件檢查再次 exit 0：46 Markdown、642 本機檔案／章節連結、115 shell 範例、三份 typed JSON；日誌 docs-final.log。原 22 份正式 README、規格、ADR 與 API 文件已核對；實際執行範圍以上表與日誌為準，本輪沒有另錄新的瀏覽器影片。

M5 首輪失敗保留於 m3-m5.log：TestM5RealSecurity 在新增加的自訂 Helm 設定回歸中，Pod rollout 完成後立即同步租戶，因 Agent 尚未重建控制通道而收到 CLUSTER_OFFLINE。原安全／表單／斷線測試及後續 Dashboard／trace／metrics／Helm lifecycle 均有各自 PASS，但整個 Make 命令 exit 1，不列為 M5 通過。修正僅在 M5 分支加入有界的實際 Hub CONNECTED 等待，保留原配額與資料斷言；相關 E2E 套件 go vet -tags=e2e exit 0。完整重跑使用全新 state，首輪資源已由正式入口清理。

## Hosted CI 準備

準備時以既有 Git credential helper 唯讀查詢，遠端 win30221/hugofleet 為 private、尚無 Actions runs；實際推送後 API 顯示為 public，未 archived／disabled。本任務沒有變更 repository visibility。workflow 使用標準 GitHub-hosted ubuntu-24.04、固定 checkout SHA、唯讀 contents 權限；獨立 oidc／unit／envtest 與六階段 kind matrix 共九個 jobs。kind jobs 開始與結束會列印 CPU／RAM／磁碟診斷，不上傳含私有資料的原始 artifacts。

首次提交與推送均 exit 0，2026-10-14 23:12（UTC+8）由 push 觸發 [Hosted CI run 38587225013](https://github.com/win30221/hugofleet/actions/runs/38587225013)，head SHA 為 164cfb241a620dcdcbf460072e0df347cf5050d6。2026-10-15 00:03 結束，總結果 failure：envtest／OIDC／M0～M4 共七個 jobs success，unit 與 M5 failure。完整遠端日誌保存於忽略提交的 github-run-38587225013-logs.zip。

## Hosted CI 發現與修正

- F09：unit job 在 docs-check 發現 M0 歷史證據連到 Git 忽略的 artifacts 日誌；本機檔案存在而乾淨 checkout 不存在。改為保留本機證據路徑的純文字說明，未放寬文件檢查。修正後以 Git archive 內容加本次文件變更建立乾淨副本，46 Markdown／641 links／115 shell／3 JSON PASS。首次驗證呼叫曾受 PowerShell 引號轉譯影響而失敗，改用保存的 Bash 腳本後取得此結果，不算專案測試失敗。
- F10：M5 quota-cpu 建立後的 Deployment 並行更新產生 resourceVersion conflict，Controller 錯誤寫入 Degraded，Hub 因而將操作固定為 FAILED。新增真實 envtest 的 Deployment／Service 競爭寫入案例，修正前兩者都穩定失敗；修正為回傳原 conflict 交由 workqueue 重試，不修改 CR status。ownership 衝突的拒絕規則保持不變。

2026-10-15 修正後 `make lint test-unit test-integration test-race build verify-generated proto-check` 整體 exit 0；新增真實衝突案例全部通過，完整 envtest 9.469s，race 包含 Unit 與真實 envtest。日誌 ci-fixes-local.log；修正前失敗證據為 conflict-before.log。乾淨副本文件檢查亦 exit 0，日誌 docs-clean-checkout-final.log。

修正後 `make operator-image-build test-e2e-m1 test-e2e-m5` 循序完整重跑 exit 0：M1 lifecycle 99.75s、authorization 0.67s；M5 安全／實際表單／Hub 重建保留 468.48s，Dashboard／trace／metrics／Helm lifecycle 102.94s。兩階段正式入口均完成 owned cleanup。日誌 ci-fixes-e2e.log；證據目錄為 artifacts/m1-e2e/20261015T060501-44061 與 artifacts/m5-e2e/20261015T061155-47952。

## 第二輪 Hosted CI：PASS

第二次提交與推送均 exit 0，修正提交為 e8d20d6cc6e6126cbb90a54ea00559d927460cac。2026-10-15 06:45:36～07:33:15（UTC+8）push 觸發的 [Hosted CI run 38649805794](https://github.com/win30221/hugofleet/actions/runs/38649805794) 已 completed／success，九個 jobs 全部 success；SPEC 的 T18 已取得實際遠端驗收證據。

| 遠端 job | 結果 | 核對內容 |
|---|---|---|
| unit | PASS | 生成檔／proto、真實 Helm CLI、lint、build、文件、Unit 與 race；乾淨 checkout 的 46 Markdown／641 links／115 shell／三份 typed JSON 通過 |
| envtest | PASS | 真實 API server；新增 Deployment／Service 衝突重試案例均成功，TestEnvtest 7.92s |
| oidc | PASS | 真實 Dex、Code／PKCE、負向認證、自然 JWT／session 到期及 owned cleanup |
| kind M0 E2E | PASS | 正式 M0 入口與資源清理 |
| kind M1 E2E | PASS | 正式 Operator lifecycle／authorization 入口與資源清理 |
| kind M2 E2E | PASS | 正式 Hub／SQLite／部署回滾入口與資源清理 |
| kind M3 E2E | PASS | 雙 Worker mTLS、ACK 遺失、ledger、離線修復、回滾及清理；主測試 141.42s |
| kind M4 E2E | PASS | OIDC、租戶隔離、CPU／memory Admission、Calico、實際表單及 Hub 重建保留；149.89s，含清理 |
| kind M5 E2E | PASS | 安全與 Hub 重建 155.40s；Dashboard／trace／metrics／Helm lifecycle 15.82s；含清理 |

完整遠端結果與日誌已存於本機忽略提交的 artifacts/release-validation/github-run-38649805794-result.jsonl 與 github-run-38649805794-logs.zip。九個 jobs 使用標準 GitHub-hosted ubuntu-24.04，六階段 kind 依序執行；不需要使用者提供常駐 runner。

本次收尾提交只更新 Markdown 驗證紀錄與狀態說明，以 `[skip ci]` 避免為記錄同一結果再啟動完整測試。上表對應的是明列的 e8d20d6 程式提交，沒有將文件收尾提交宣稱為另一次 Hosted CI 通過。

收尾文件在 Git archive 加本次 Markdown 變更的乾淨副本中再次檢查，exit 0：46 Markdown／643 本機檔案與章節連結／115 shell 範例／三份 typed JSON；日誌 docs-hosted-complete.log。原始碼、測試、workflow 與其他非 Markdown 檔案均與被測提交相同。
