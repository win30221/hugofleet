# ADR 0002 — 固定版本、Agent 與登入的設計契約

- 狀態：已採納。
- 範圍：M1～M5 的版本、命令與登入契約。

revision 與 ReplicaSet 的完整歸屬規則見 [ADR 0012](0012-current-revision-replicaset-evidence.md)。

## 背景

單一 current revision 容易混淆部署意圖、Worker 寫入與 rollout 成功；只依 CR annotation 判斷，也無法涵蓋 metadata 變更不增加 generation 的情況。多叢集控制通道還需要明確處理 Agent 重啟、過期命令與長連線中的追蹤資訊。

## 決策

1. M1 的 Application `spec.revision` 與 `status.observedRevision` 分別表達期望和觀測版本，搭配標準 conditions、Deployment observedGeneration 及實際配置確認結果。revision 同時寫入 Pod template，使新 revision 觸發獨立 rollout；相同 revision 重送維持冪等。
2. M2 的 Hub 分開保存 desired／applied／last successful revision；所有變更用 expected desired revision 做 optimistic concurrency。回滾以舊配置建立新 revision，不倒退序號。TIMED_OUT Operation 保持終態，晚到結果另記觀測。
3. M3 的 Agent 以 operation ID、revision、payload hash、expiry、UID delete precondition 與 session fencing 限制命令。去重／刪除 tombstone 必須在重啟後仍有效；M3 使用受限 ConfigMap ledger。
4. gRPC 雙向 stream 的每個命令／結果 envelope 帶自己的 trace context；不能只用建立 stream 時的 metadata 表達之後所有操作。
5. M4 的 Hub 作為 OIDC BFF，管理 state／nonce、伺服器端 session、HttpOnly cookie、CSRF 與角色授權。管理 UI 不以 localStorage 保存管理 bearer token。
6. 配額驗收檢查 Pod 建立被拒絕的 FailedCreate 與 rollout 結果，不能只測 Deployment API 是否回 403。

## 影響

- 契約詳見 HUGOFLEET_SPEC.md 第 6–11 節；測試範圍見各階段驗收文件。
- CRD、Protobuf、資料庫 migration、Controller 與安全整合測試在各自里程碑加入，M0 不建立空骨架。
- 保持原架構：Hub 不持有 Worker cluster-admin kubeconfig；Agent 主動建立 mTLS gRPC 控制通道。

## 閱讀後返回

本頁用來查設計原因，不是安裝步驟。返回 [M3 原理與進階閱讀](../../stages/m3/README.md#原理與進階閱讀)，或從[選課與回訪](../../stages/README.md#回來繼續哪一課)接續使用此共用設計的其他階段。其他設計問題見 [ADR 導覽](../README.md#設計問題與-adr-對照)。
