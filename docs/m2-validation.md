# M2 驗收範圍

M2 驗證 Go template UI／REST → SQLite durable Operation → 受限本機 kubeconfig → Application Operator → 真實工作負載。操作見 [M2 教學](../stages/m2/README.md)，契約見 [M2 API](m2-api.md)，實測與 CI 證據見 [驗證總覽](validation.md)。

## 如何使用本頁

第一次操作請先讀本階段教學；本頁用來選擇自動驗證與核對既有證據。只修改文件時執行 `make docs-check`，毋須啟動本頁整套測試。

| 命令 | 目的 |
|---|---|
| `make m2-build m2-test` | 編譯與 M2 單元測試 |
| `make test-integration` | 共用 Controller 真實 envtest |
| `make demo-image-build operator-image-build` | 準備價格 API／Operator 映像 |
| `make test-e2e` | M2 UI／REST／SQLite／rollout 與清理 |

下方依序列出本階段需要的工具與驗收命令；已安裝的固定工具可沿用，不需準備其他階段的工具。一次執行一組 E2E，先依[停止流程](../stages/m2/README.md#停止與下次繼續)結束會占用相同連接埠的教學環境。

## 驗收命令

在專案根目錄的 Ubuntu WSL2／Linux Bash 執行：

```bash
make tools operator-tools envtest
make m2-build m2-test
make test-integration
make demo-image-build operator-image-build
make test-e2e
```

## 如何判斷本次執行結果

等待整個命令結束，核對測試與 owned cleanup 均成功且最終退出碼為 0。中途出現 PASS 不表示整體完成；失敗先保留輸出與 state，依[本階段排查](../stages/m2/README.md#操作遇到問題時)定位問題。下列已確認行為來自既有驗收，對應證據與程式提交見驗證總覽，不會自動套用到新修改。

## 已確認的行為

- 真實 SQLite WAL、交易 migration、程序鎖、immutable Release、去重、CAS、單應用併發限制與終態保護。
- UI／REST 建立 v1、真實 quote=8000、good 更新、CR drift 與目前原因持久化、broken FAILED／上一版仍服務、確認回滾建立新 revision。
- 同 key 同 payload 回同一操作；改 payload 回 409；併發請求恰一筆 202、一筆 409。
- RUNNING 時 SIGKILL 後，原 command_id／deadline／CR UID／generation 與操作履歷保留。
- 真實 quota Admission 拒絕、解除後以新 revision 恢復，失敗歷史維持不變。
- TIMED_OUT 後 LATE_READY 可更新目前觀測，不改寫 Operation／FAILED Release／last successful。
- 以真實 SQLite acknowledgement 故障驗證 UID_BOUND、精確 UID 恢復、原期限後唯讀觀測、刪除與 GC；Hub 重啟後仍保留履歷。
- HTTP 驗證 Host／Origin／CSRF、危險操作確認與 HTML escaping；rollout 替換 Pod 後重新建立 port-forward。

本機與對應 Hosted CI 的正式 M2 E2E 均有 PASS 證據，包含 owned cleanup。原始日誌在 `artifacts/m2-e2e/<run>`，整合日誌為 `artifacts/release-validation/m0-m2.log`。全 module race／envtest 的證據見驗證總覽。

## 平台與資料限制

Hub 僅支援 Linux／WSL2；非 Linux 建置提供明確的 unsupported platform 錯誤，不在沒有程序鎖的情況下開啟 DB。此錯誤路徑有 Windows 交叉編譯與執行測試，不代表原生 Windows Hub 可用。

M2 是 loopback 單管理者、單開發租戶與單 kind 模式。SA token 約 8 小時後需重新 bootstrap 及重啟；真正 OIDC／租戶授權從 M4 提供。UI 最多顯示 50 筆歷史、API 詳情最多 100 筆，更舊資料仍留在 DB。

SQLite 優先放在本機 Linux filesystem，不使用網路共享。程序恢復不代表主機斷電、node restart、備份還原或 disk profile 已通過。不能以舊版 Hub 開啟較新 schema，回退需 SQLite-aware 備份，不能手動降低 schema version。

## 查完後返回

本頁的測試不是初次學習的必經步驟。返回 [M2 整課驗證](../stages/m2/README.md#整課驗證與常見問題)核對手動操作；要停止或進下一課，前往 [M2 完成後的選擇](../stages/m2/README.md#完成這一課後去哪裡)。修改其他元件時，依[驗證總覽](validation.md#執行驗證)選擇測試。
