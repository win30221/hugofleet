# ADR 0002 — 固定版本、Agent 與登入的設計契約

- 日期：2026-10-10
- 狀態：已採納，依使用者接受規格審查建議。
- 交付範圍：只更新規格；相關程式在 M1–M5 依序實作。

2026-10-14 後續決策：[ADR 0012](0012-current-revision-replicaset-evidence.md) 調整第 1 項的實作方式，將 revision 綁定至 Pod template，讓新 revision 觸發獨立 rollout 並精確歸屬 ReplicaSet 失敗。下方保留原始決策背景；實際成功仍需完整配置、generation 與 Ready 副本核對。

## 背景

原規格的 current revision 混合了部署意圖、Worker 寫入與 rollout 成功；依 CR annotation 判斷版本，也可能忽略 metadata 變更不增加 generation 的情況。多叢集控制通道還需要明確處理 Agent 重啟、過期命令與長連線中的追蹤資訊。

## 決策

1. M1 的 Application `spec.revision` 與 `status.observedRevision` 分別表達期望和觀測版本，搭配標準 conditions、Deployment observedGeneration 及實際配置確認結果。相同配置、縮放或重用舊配置不能假設一定產生新 ReplicaSet。
2. M2 的 Hub 分開保存 desired／applied／last successful revision；所有變更用 expected desired revision 做 optimistic concurrency。回滾以舊配置建立新 revision，不倒退序號。TIMED_OUT Operation 保持終態，晚到結果另記觀測。
3. M3 的 Agent 以 operation ID、revision、payload hash、expiry、UID delete precondition 與 session fencing 限制命令。去重／刪除 tombstone 必須在重啟後仍有效；持久化媒介在 M3 決定。
4. gRPC 雙向 stream 的每個命令／結果 envelope 帶自己的 trace context；不能只用建立 stream 時的 metadata 表達之後所有操作。
5. M4 的 Hub 作為 OIDC BFF，管理 state／nonce、伺服器端 session、HttpOnly cookie、CSRF 與角色授權。管理 UI 不以 localStorage 保存管理 bearer token。
6. 配額驗收檢查 Pod 建立被拒絕的 FailedCreate 與 rollout 結果，不能只測 Deployment API 是否回 403。

## 影響

- 契約詳見 HUGOFLEET_SPEC.md 第 6–11 節；本 ADR 不代表這些功能已完成。
- CRD、Protobuf、資料庫 migration、Controller 與安全整合測試在各自里程碑加入，M0 不建立空骨架。
- 保持原架構：Hub 不持有 Worker cluster-admin kubeconfig；Agent 主動建立 mTLS gRPC 控制通道。
