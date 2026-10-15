# M5 驗收範圍

M5 驗證安全 Hub 的 Dashboard、OpenTelemetry、Prometheus metrics 與 Helm 交付，沿用 SQLite、Worker 主動 mTLS 與 Application Operator。操作見 [M5 教學](../stages/m5/README.md)，設計見 [ADR 0011](adr/0011-observability-and-delivery.md)，實測提交與 Hosted CI 證據見 [驗證總覽](validation.md)。

## 驗收命令

在專案根目錄的 Ubuntu WSL2／Linux Bash，以非 root 使用者執行：

```bash
make tools operator-tools envtest proto-tools helm-tools
make m5-build m5-test
make m5-chart-test
make proto-check verify-generated test-integration
make test-e2e-m5
```

## 已確認的行為

- 完整 M4 安全、CPU／memory Admission、Calico、實際表單與回滾／GC、離線服務延續、Hub 原 DB／session 恢復及自主修復。
- Dashboard 的 UID／revision／generation／完整 Conditions 對照真實 CR；租戶／叢集篩選、Viewer scope、撤權、Admin telemetry 與有界更新。
- 兩個真實 OIDC／SQLite／mTLS operations 在同一通道具有不同 trace IDs，Hub→Agent parent 相符；HTTP／operation／command／Agent／Operator correlation 可串聯，對應 T17。
- SQLite trace context／觀測快照在重啟與冪等重送後保留。終態 hook 僅於 Commit 後計數，失敗寫入與重複 Finish 不增加 metrics；真實操作的 committed metrics delta=2。
- 實際 Helm CLI typed render、schema 與負向 values；真實 install／upgrade／uninstall／reinstall 保留 ledger UID、high-water、tombstones、receipt UID／data，Agent 實際重連。
- Hub 重建保留自訂 quota、profile、session、rollout／timing 設定；Agent rollout 後等待真實控制通道 CONNECTED。
- Controller 只歸屬目前 ReplicaSet 的失敗／逾時；resourceVersion conflict 可重試，不能將操作固定為 FAILED。

本機及對應 Hosted CI 的正式 M5 E2E 均有 PASS 證據，包含 owned cleanup；T18 的程式提交與九個 jobs 見驗證總覽。原始日誌在 `artifacts/m5-e2e/<run>`，整合日誌為 `artifacts/release-validation/m5-retry.log`、`ci-fixes-e2e.log`。M0～M4 各正式入口也有獨立驗收證據。

## 瀏覽器與示範素材

真實瀏覽器操作驗證 Owner 的 v1 → good／replicas 2 → 漂移修復 → Agent 斷線／重連 → broken FAILED → 確認回滾新 revision → quote=8000，以及 Viewer／跨租戶拒絕、登入與登出。桌面與窄版頁面已檢查，表格在自己的容器水平捲動。

[示範 MP4](media/m5-demo.mp4)／[播放頁](media/m5-demo.html) 使用 19 張真實瀏覽器截圖依操作順序剪輯，H.264、1080×800、25 fps、114 秒，省略等待片段，並非連續錄影。影片 SHA-256 為 `bd09ee2b304a633e9f7d1dd533d3d85adfca4a6e81cee07a79349ca0124cc9df`；[來源 metadata](media/m5-demo-captures.json) 保留擷取證據與雜湊。素材已實際播放與抽幀核對，文件更新不會重新產生影片。

[回滾 trace](media/m5-rollback-trace.html) 來自同一實際操作，trace ID 為 `c71b29b8cfdbf0c857a958b3e3d158c5`，保留 HTTP→operation→command→Agent→Operator parents。原始收集與匯出證據在 `artifacts/m5-development/actual-collected-parent-check-and-export.log`。公開素材只含 Demo 資料與允許的 correlation 欄位。

## 限制

metrics 隨程序重啟歸零；Hub trace ring 有 3,000 筆上限，不能當作持久全量稽核。離線 viewer 保存已收集資料，不代表即時狀態。影片畫面呈現實際操作資料，不能當作目前平台狀態。

leaf 憑證到期後的新 state 練習不等於原 DB／Workers 的原地輪替已驗收。RAM etcd、單 Hub／SQLite、Calico enforcement 與 LocalDemo 安全邊界沿用 [M4 限制](m4-validation.md#資源與限制)。HA、Gateway API、ConnectRPC、額外 worker-service、自動派發及完整備份還原不在交付範圍。
