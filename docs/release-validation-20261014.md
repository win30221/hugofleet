# M0～M5 修正、完整驗證與 Hosted CI（2026-10-14）

本輪依使用者最新授權，先修正 [文件與功能稽核](audit-m0-m5-20261014.md) 的問題，完成本機驗證後才提交與推送既有 origin，實際測試 GitHub Actions。歷史驗收結果維持原意；本文件只記錄本輪新執行的結果。

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
| 提交／推送與 Hosted CI | 本機門檻已通過，已授權提交與推送；遠端結果待實測 |

本機驗收已完成；Hosted CI 須另取得遠端結果，不能視為 PASS。原始 artifacts、帳號、DB、憑證與 kubeconfig 均由 Git 排除，不隨原始碼推送。

上述簡寫日誌均位於 artifacts/release-validation。最終整合檢查使用 GOMAXPROCS=2、GOFLAGS=-p=2，與 CI 的主機 Go 並行度一致；這不代表本機硬體等同 Hosted runner。WSL 無 C compiler，race 依正式入口使用固定 Go 容器與真實 envtest。

提交前文件檢查再次 exit 0：46 Markdown、642 本機檔案／章節連結、115 shell 範例、三份 typed JSON；日誌 docs-final.log。原 22 份正式 README、規格、ADR 與 API 文件已核對；實際執行範圍以上表與日誌為準，本輪沒有另錄新的瀏覽器影片。

M5 首輪失敗保留於 m3-m5.log：TestM5RealSecurity 在新增加的自訂 Helm 設定回歸中，Pod rollout 完成後立即同步租戶，因 Agent 尚未重建控制通道而收到 CLUSTER_OFFLINE。原安全／表單／斷線測試及後續 Dashboard／trace／metrics／Helm lifecycle 均有各自 PASS，但整個 Make 命令 exit 1，不列為 M5 通過。修正僅在 M5 分支加入有界的實際 Hub CONNECTED 等待，保留原配額與資料斷言；相關 E2E 套件 go vet -tags=e2e exit 0。完整重跑使用全新 state，首輪資源已由正式入口清理。

## Hosted CI 準備

以既有 Git credential helper 進行唯讀 GitHub API 查詢：遠端 win30221/hugofleet 已存在、為 private、未 archived／disabled，尚無 Actions runs。保持原 visibility，不使用自架或付費大型 runner。workflow 使用 ubuntu-24.04、固定 checkout SHA、唯讀 contents 權限；獨立 oidc／unit／envtest 與六階段 kind matrix 共九個 jobs。kind jobs 開始與結束會列印 CPU／RAM／磁碟診斷，不上傳含私有資料的原始 artifacts。
