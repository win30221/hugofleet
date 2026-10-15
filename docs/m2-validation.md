# M2 驗收範圍

M2 驗證 Go template UI／REST → SQLite durable Operation → 受限本機 kubeconfig → Application Operator → 真實工作負載。操作見 [M2 教學](../stages/m2/README.md)，契約見 [M2 API](m2-api.md)，實測與 CI 證據見 [驗證總覽](validation.md)。

## 驗收命令

在專案根目錄的 Ubuntu WSL2／Linux Bash 執行：

```bash
make tools operator-tools envtest
make m2-build m2-test
make test-integration
make demo-image-build operator-image-build
make test-e2e
```

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
