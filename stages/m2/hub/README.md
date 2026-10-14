# Hub：從管理請求到部署履歷

完整操作看 [M2 教學](../README.md)，雙叢集操作看 [M3 教學](../../m3/README.md)，安全租戶操作看 [M4 教學](../../m4/README.md)。本資料夾保存共用業務／SQLite 邏輯。M2 使用受限 kubeconfig transport；M3 透過 Agent transport；M4 使用同一遠端通道並另有 OIDC HTTP／UI，Store 在交易內核對資料庫角色。

## 套件分工與閱讀順序

| 位置 | 責任 | 關鍵函式／型別 |
|---|---|---|
| [主程式](../cmd/hub/main.go)／[policy](../cmd/hub/policy.go) | 讀旗標、policy，組裝 DB、transport、HTTP、Runner | run |
| [domain/model.go](domain/model.go) | 請求、Application、Release、Operation、政策與觀測 | Policy.Spec、Operation.Active、Observation |
| [httpapi/server.go](httpapi/server.go)／[auth.go](httpapi/auth.go) | REST／UI 的輸入與回應，管理入口保護 | Handler、deploy、form、render |
| [service/service.go](service/service.go) | 驗證請求、產生去重 hash，背景推進操作 | Deploy、Rollback、Delete、Runner.Tick／step、Transport |
| [repository/store.go](repository/store.go) | SQLite 交易、履歷、UID 與狀態持久化 | Open、Accept、Applied、Finish、Observe、Detail |
| [repository/clusters.go](repository/clusters.go) | M2／M3 模式隔離、M3 registry／心跳資料 | CheckLocal、ConfigureRemote、ClusterConnectivity |
| [domain/security.go](domain/security.go)／[repository/security.go](repository/security.go) | M4 身分、session、membership、scoped 讀取、profile／audit | ConfigureSecurity、Session、VisibleTenants、ScopedDetail、SetMember |
| [kube/transport.go](kube/transport.go)／[credentials.go](kube/credentials.go) | M2 的受限存取與真實 CR／rollout／GC 核對 | Apply、Observe、Delete／DeletionStatus |
| [web](../web/) | Go template 與 CSS，embed 到程式 | index.html、assets.go |

HTTP 不自己寫 Kubernetes，repository 不自己執行 rollout，Controller 不讀 SQLite。這些邊界讓 M3 可以換通道而沿用同一部署規則。

## 跟一次建立請求

以 `POST /api/v1/tenants/demo/apps` 為例：

1. Handler 檢查 Host／Origin 等入口條件，限制 request context；M3 還驗證 Token／本機 gateway。deploy 解碼受限 JSON，取 Idempotency-Key。
2. Service.Deploy 要求 expected_desired_revision，檢查名稱與政策，將已解碼請求產生 hash。建立與發版走同一業務入口。
3. Store.Accept 開交易。M2／M3 查 key；M4 先重新核對受信任 session／當前 membership，才查 key。已授權重送相同意圖回既有 Operation；相同 key 不同內容拒絕，撤權後原意圖也拒絕。
4. 新意圖檢查租戶、叢集 CONNECTED、名稱、基準 revision、活動操作與政策；一起儲存 Application、不可變 Release 快照、Operation、key、QUEUED 事件。
5. 交易提交後 HTTP 回 202／Accepted。部署動作由 Runner 後續執行，並不是 HTTP handler 一路等待 Pod。
6. Runner.step 將已接受的 spec 交給 Transport.Apply，取得 CR UID 後 Store.Applied，接著 Observe 真實工作負載。
7. 只有完整 readiness／刪除結果符合，才 Finish SUCCEEDED；具體故障 FAILED、過原 deadline TIMED_OUT。

```text
UI form 或 REST JSON
→ Service → Store.Accept → 回 202
                 ↓ 持久化的 Operation
             Runner.step
                 ↓ Transport
M2 受限 client / M3 gRPC Agent
                 ↓ Application CR
             M1 Operator
                 ↓ Deployment / Service / Pods
             Observe → Store.Finish
```

202 表示意圖已保存，不是成功部署。CR Apply 回 UID 表示已提交，不是 SUCCEEDED；revision、generation、完整 workload 與副本狀態仍要核對。

## SQLite 每張表為何存在

| 表 | 保存什麼 | 操作上的用途 |
|---|---|---|
| tenants／clusters | M2／M3 的 demo 範圍、M4／M5 租戶與已登記 Worker、目標狀態 | 決定可提交的目標；M3 增加 Agent 版本等資料 |
| applications | 現在的意圖版本、UID、觀測 | UI 目前狀態與版本欄位 |
| releases | 每次完整 spec 快照、revision、生命週期 | 回滾來源與不可變配置歷史 |
| operations | 某次 CREATE／RELEASE／ROLLBACK／DELETE、command_id、deadline、結果 | 非同步進度、重啟恢復、歷史錯誤 |
| idempotency_keys | 租戶 key、request hash、Operation ID | 網路重送不新增部署 |
| operation_events | 過程與觀測變化 | QUEUED、UID 綁定、故障／恢復、晚到 READY 等 |
| security_settings／users | M4 精確 issuer + sub、已驗證 User／固定 Admin | 身分不因 email 更新而改變；JWT groups 不提升角色 |
| tenant_memberships／tenant_profiles | M4 Owner／Developer／Viewer、每 Worker 已核對的 namespace／Quota profile | 授權及提前配額估算；實際 Admission 仍是權威 |
| sessions／login_transactions | 平台 session hash、CSRF／期限、一次性 state／binding／nonce／PKCE | 每請求查驗、登出撤銷、登入 replay 拒絕；不保存原 OIDC Token |
| audit_events | M4 可信 Actor、操作目標、結果與 request_id | 租戶 scoped 履歷；不保存密碼、Token 或請求 body |

[001_initial.sql](repository/001_initial.sql) 是基礎結構，資料庫約束保護不可變快照與每應用單一活動操作；[002_observations.sql](repository/002_observations.sql) 分開目前故障與可重複的觀測事件；[003_agent.sql](repository/003_agent.sql) 是 M3 的 Cluster 擴充，留在同套件供 embed。

[004_security.sql](repository/004_security.sql) 增加安全資料與持久化 Operation request_id。只有 M4／M5 OIDC Hub 明確 ConfigureSecurity 才啟用安全模式，且必須使用無既有應用／遠端歷史的新 DB；M2／M3 入口拒絕開啟已啟用安全模式的 DB，不能把 schema 已存在視為 OIDC／RBAC 已設定。

[005_observability.sql](repository/005_observability.sql) 是目前共用 schema 5，保存 Operation 的 trace context 與 Application 的完整觀測快照，供 [M5 Dashboard／telemetry](../../m5/README.md) 使用。它保留既有履歷，schema migration 本身不啟用 OIDC 或 telemetry；功能仍由各入口明確配置。Hub 重啟可保留快照與 trace parent，Prometheus 程序內 metrics 則重新計數，不能把兩者混為持久歷史。

程式啟動時做 migration，不手動修改 live DB 的 revision／state 去繞過規則。SQLite WAL、鎖檔與 DB 屬於同一份執行資料；同一 DB 不啟動兩個 Hub。

## 發版、回滾、刪除的差別

發版是完整配置搭配啟動時 defaults，並非部分 patch。UI 預填最新快照，REST 必須明確決定自己要送什麼；省略 replicas 會用 1，不是沿用之前的 2。

回滾只接受已成功來源（ACTIVE／SUPERSEDED），複製整份快照，重新檢查政策，建立新 revision。刪除另建 DELETE Operation、保留版本與履歷，**不增加 deployment revision，也不建立新的 Release**；要等待 UID 綁定資源與 GC 消失才算成功。

名稱在相同 tenant 中唯一，含刪除歷史；因此刪除後想從頭做，換名稱或換新一輪 DB。M3 也不能把既有應用直接移到另一叢集。

## 重啟、斷線與晚到 READY

Runner.Tick 定期讀待處理 Operation，先核對各 cluster 的連線，逐個推進。M3 A 離線時，其 step 不發新命令；B 仍可推進。HTTP 接受新意圖前也檢查連線，離線目標回 503，而不是無限制排隊。

重啟沿用原 command_id／deadline；不能每次啟動就重新給完整 timeout。M3 Apply 重試前會先由 Agent snapshot 核對當地狀態，避免 ACK 遺失後重複變更。

Application 現況與 Operation 終態分開。過期操作之後 CR 變 Ready，可以更新現在觀測並記 LATE_READY，但不把 TIMED_OUT／FAILED 改成功，也不推進 last_successful_revision。需要提交新發版／回滾，才產生新的成功確認。

## 修改與閱讀測試

改業務限制看 domain／service，改交易和履歷看 repository，改 UI／HTTP 看 httpapi 與 web，改 CR 核對看 kube 與 [M1 workload.go](../../m1/controller/workload.go)。改 M3 通道看 [agenttransport](../../m3/agenttransport/README.md)，不要讓 Hub M3 分支取得 Worker kubeconfig。

M4 的公開請求必須使用 [OIDC HTTP API](../../m4/httpapi/README.md) 的 scoped 入口；Runner 的內部 App／Operation 讀取不作公開授權 API。修改安全模式時另跑 security tests 與 [M4 真實 E2E](../../m4/README.md#測試與證據)，不能以舊本地管理者模式的 PASS 代替角色／租戶驗收。

同目錄 *_test.go 用來理解 key／revision／回滾／錯誤／權限契約；真實 HTTP、SQLite 恢復、rollout／GC 的證據在 [M2 驗收](../../../docs/m2-validation.md) 與 [M3 驗收](../../../docs/m3-validation.md)。測試完整度與當次環境結果分開，本輪實測結果另見 [搬移後重新驗證](../../../docs/revalidation-20261011.md)。
