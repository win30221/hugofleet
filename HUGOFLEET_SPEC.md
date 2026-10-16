# HugoFleet — Go 雲原生多叢集應用管理平台規格書

> **文件用途**：直接交給 Codex、Claude Code、Cursor 等 Coding Agent 逐階段實作；也是 GitHub 專案的產品需求與技術設計基準。
>
> **定位**：以 Go、Kubernetes Operator、gRPC 與真實業務規則打造輕量化內部 PaaS。
>
> **開發條件**：個人筆電，32 GB RAM、Intel 顯示晶片（約 4 GB 顯存）、Docker + kind；**CPU-only**。
>
> **狀態**：規格書，非已完成的程式或經驗證的部署結果。實作時依實測和 ADR 記錄調整。

---

## 0. 專案最高原則（AI 必讀）

1. **先做可運作的完整流程，再增加技術範圍**。禁止先產出一大堆沒有串起來的空架構。
2. **一個階段完成、通過測試、交付說明後，才開始下一階段**。不要一次寫完全部模組。
3. **所有核心功能必須真實執行，不得以 Mock 充當完成**。Mock 僅供單元測試使用；多叢集功能必須使用兩個真實 kind Cluster 驗證。
4. **Operator 必須正確實作 Reconcile**；不能只是呼叫一次 `kubectl apply` 或包裝 Kubernetes CRUD。
5. **不使用 GPU、不執行 LLM、不安裝 KServe、vLLM、Rook/Ceph 或 Istio**；它們不屬於本規格的必要交付。
6. **開發環境安全性與正式安全性要清楚區分**；Dev Auth、開發憑證或測試密碼不能誤稱為正式安全機制。
7. **不自行猜測相容版本**：開工時查官方版本／相容性表，將版本固定於 go.mod、工具腳本與文件；更新須有紀錄。
8. **不為了符合職缺而硬塞技術**。任何新元件要能回答「它解決哪個產品需求」。
9. **使用繁體中文維護說明文件與驗收摘要**；程式命名、API、錯誤碼與註解可使用清楚的英文。
10. **每個階段要交付**：可執行程式、測試、README 更新、技術取捨說明及可複製的驗證指令。
11. **程式語言以 Go 為主**：產品與驗收程式使用 Go；Bash／sh 可用於開發與部署流程。UI 由 Go `html/template` 渲染 HTML／CSS，不引入 SvelteKit／TypeScript 或 Python 工具。

## 1. 產品需求

### 1.1 產品名稱

**HugoFleet — Multi-Cluster Application Delivery Platform**

**專案命名慣例**：GitHub 帳號為 `win30221`，Repository 名稱使用 `hugofleet`（網址：`https://github.com/win30221/hugofleet`），Go module 固定為 `github.com/win30221/hugofleet`；Kubernetes CRD Group 使用 `platform.hugofleet.io`，Namespace 前綴使用 `hf-`，kind Cluster 使用 `hugofleet-a` / `hugofleet-b`，Proto Package 使用 `hugofleet.agent.v1`，Metrics 前綴使用 `hugofleet_`。這些名稱應在程式、Manifest、指令、文件與範例中保持一致。`hugofleet.io` 僅為目前規格中的 API Group 命名，不代表已註冊或擁有該網域。

### 1.2 解決的問題

企業的多個開發團隊需要部署 HTTP API 與背景服務，卻不希望每個工程師都直接擁有 Kubernetes 管理員權限。HugoFleet 允許：

- 建立租戶／團隊（Tenant），設定可使用的叢集及資源額度。
- 建立應用、選擇目標 Cluster、設定映像檔、Port、CPU、Memory、Replicas。
- 發起部署、查看狀態、更新版本、失敗後回滾。
- 由 Kubernetes Operator 持續修復底層工作負載的漂移。
- 透過主動向 Hub 建連線的 Agent 管理多個 kind Cluster，而不需 Hub 持有 Worker 管理員 kubeconfig。
- 使用權限控制與稽核紀錄追蹤「誰在什麼時間對哪個服務做了什麼」。

### 1.3 主要使用者

| 角色 | 主要工作 |
|---|---|
| Platform Admin | 註冊 Cluster、管理租戶與安全政策、查看所有事件 |
| Tenant Owner | 管理自己的團隊成員、資源配額內的應用與版本 |
| Developer | 部署／更新／回滾被授權的應用、查看部署事件 |
| Viewer | 僅可查看被授權租戶的狀態、紀錄 |

> M0–M3 只支援限制在本機的單一管理者開發模式；真實 OIDC 登入與角色授權於 M4 實作。M4 完成前不得對外公開管理 API。

### 1.4 展示用真實業務服務

實作 `pricing-api`（Go HTTP API）作為 HugoFleet 的測試與展示負載：

- `POST /quote` 接受 `amount_cents`、`customer_tier`、`coupon_code`，用**整數分**計算最終價格。
- 示範規則：`vip` 打九折，`SAVE100` 再減 10,000 分（即 100 元），最低為 0。
- M0 輸入契約：`amount_cents` 必填且為 0–1,000,000,000,000 的整數；`customer_tier` 必須為 `standard` 或 `vip`；`coupon_code` 可省略／空字串或為 `SAVE100`，其他值拒絕。
- VIP 計算為 `floor(amount_cents * 9 / 10)`，先向下取整到分再折抵折價券；101 分的 VIP 價格為 90 分；20,000 分加 VIP + SAVE100 的結果為 8,000 分。
- `/quote` 只接受 `application/json`、最多 4 KiB 且單一 JSON 文件；未知欄位、無效 JSON、負數、超限、非整數輸入須有明確錯誤回應。
- `GET /healthz`、`GET /readyz`、`GET /version`。
- `v1` / `v2-good` 正常就緒；`v2-broken` 的 `/healthz` 仍為 200、`/readyz` 永遠為 503，避免混淆 readiness 失敗與程序死亡。版本由建置時注入，不使用 runtime 參數偽裝不同映像版本。
- 產出 `v1`、`v2-good`、`v2-broken` 三種版本；`v2-broken` 的 readiness 永遠失敗，專門用來驗證部署失敗與回滾。
- `pricing-api` 是**被部署的產品服務**；HugoFleet 的真正業務邏輯則是租戶、配額、部署申請、版本與權限管理。
- 可選擇在 M5 增加一個不需要資料庫的 `worker-service`，展示非 HTTP 工作負載；非 MVP 必要項目。

### 1.5 成功標準

五分鐘 Demo 可完成：

1. 從 UI 建立或選擇 Tenant。
2. 在 Cluster A 部署 `pricing-api:v1`，真的呼叫 `/quote` 得到結果。
3. 將 replicas 從 1 調整為 2；手動把 Deployment 改成 1，Operator 自動恢復為 2。
4. 部署 `v2-broken`，看到部署逾時／失敗原因，但服務歷史與上一版資訊仍存在。
5. 從 UI 觸發回滾至 `v1`，確認服務可用。
6. 部署至 Cluster B，展示 gRPC Agent 斷線／重連、狀態同步。
7. 嘗試越權／超出配額操作，得到可解釋、可測試的拒絕結果（M4）。
8. 展示測試、事件、日誌、追蹤與 GitHub Actions（M5）。

## 2. 硬體與功能範圍

### 2.1 必須支援

- 32 GB RAM 普通筆電、CPU-only。
- Docker + kind；每日開發只要一個單節點 Cluster。
- 多叢集 Demo 時啟動兩個單節點 kind Cluster（`hugofleet-a`、`hugofleet-b`）。
- Go Hub、SQLite 與 Go 渲染的 UI 可在本機執行；M3 起 Hub 可打包為 Docker 容器以便 Agent 從 Docker 網路連線。
- `make` 指令控制啟動、載入 Image、測試與關閉。
- kind 不使用 GPU、不下載大型模型。

### 2.1.1 開發作業系統契約

- 主要開發／驗收環境為 Linux amd64；Windows 使用 Ubuntu WSL2 + Docker Desktop Linux containers，Makefile 與腳本在 WSL2 執行。
- 工具固定於 `scripts/versions.env`，以官方下載和 SHA-256 校驗安裝至專案 `.tools`；不覆寫主機全域 Go、kind、kubectl。
- kubeconfig、工具快取、測試產物均放在專案內的 Git 忽略目錄；每個叢集使用獨立 kubeconfig，所有 kubectl 操作明確指定 context。
- 固定 kind release 官方提供的 node image digest，kubectl 與 API server 使用相同 minor。M1 再依 Kubebuilder scaffold 的受支援相依版本確認 Go／controller-runtime／Kubernetes 組合。
- M0 本機使用拋棄式 RAM etcd profile（ADR 0004），Docker node 重啟不保留叢集資料；可選磁碟 profile。M1 起重新評估，持久性與重啟恢復測試必須使用持久環境。

### 2.2 不做的事情（Out of Scope）

- 真正的 LLM 推論、KServe、vLLM、模型訓練、GPU 排程。
- Rook/Ceph、Istio、多可用區與真正的公有雲供應商整合。
- 任意 TCP/SSH 通用反向隧道、遠端任意 Shell 或任意 Kubernetes YAML 執行。
- Hub 的高可用／多副本、跨資料中心生產級災難復原。
- 自動計費、金流、任意使用者註冊、企業級完整 IAM。
- 真正的跨叢集 Kubernetes Scheduler；平台做**有限策略選擇**，最終 Pod 排程仍由各 Worker 的 Kubernetes Scheduler 負責。
- 完整 GitOps（Flux CD）與 Admission Webhook；可作為日後延伸，不是驗收阻塞項。

### 2.3 本機資源策略

- M1/M2：只開 1 個 kind，Hub/前端可在主機執行；`pricing-api` 的 image 使用 `IfNotPresent` 並先 `kind load`。
- M3：開 2 個 kind，各 1 個 control-plane 節點；Hub 以容器接入 kind Docker 網路。
- 所有 Sample Pods 設定合理的 CPU/Memory requests/limits。
- 監控 UI、OIDC Provider、Gateway Controller 需能**按需啟停**。
- README 要記錄實測環境：CPU、OS、Docker/WSL2、kind/K8s 版本、啟動後 `docker stats`／`kubectl top`（若 Metrics API 可用）。
- 資源使用量是實測結果，不可在規格裡宣稱保證固定占用多少 RAM。

## 3. 技術選型與責任

| 層 | 技術 | 職責 |
|---|---|---|
| Backend | Go、`net/http`、可選 chi | 業務 API、命令處理、部署記錄 |
| Database | SQLite（WAL）、`database/sql`、migration | 租戶、應用、版本、操作與稽核 |
| Operator | Go、Kubebuilder、controller-runtime | Reconcile `Application` CRD |
| Cluster | kind、Kubernetes | 真實工作負載及 API |
| Agent RPC | Protobuf + gRPC 雙向串流、mTLS | Agent 主動連 Hub、命令／回報 |
| 前端 | Go html/template + HTML/CSS | 由 Hub 渲染部署表單與狀態頁 |
| Delivery | 多階段 Dockerfile、Helm、Makefile | 安裝、升級、快速 Demo |
| Auth | OIDC（建議 Dex 本機 Demo）、RBAC | 身分認證與租戶授權 |
| Observability | 結構化日誌、Prometheus metrics、OpenTelemetry | 觀測部署流程與跨程序請求 |
| CI | GitHub Actions | Lint、Unit、envtest、kind E2E |
| Routing（延伸） | Kubernetes Gateway API + 相容 Gateway Controller | HTTPRoute 實際流量路由 |

選型限制：ConnectRPC 可在 M5 為管理 API 提供 Protobuf 型別安全入口；**MVP 優先穩定的 REST/JSON Hub API + gRPC Agent**。不要為了同時實作 REST 和 ConnectRPC 而讓 M1–M3 無法完成。

## 4. 系統架構

```text
Browser / Go-rendered HTML
       |
       | HTTP(S) / HTML forms + REST API
       v
+-----------------------------------------+
| HugoFleet Hub (Go, 本機或 Docker 容器)      |
| - HTTP API / auth / RBAC                 |
| - Tenant / app / release business logic  |
| - SQLite + operation ledger              |
| - Agent session manager / gRPC server    |
+-----------------------------------------+
          ^                         ^
          | Agent 主動連線           | Agent 主動連線
          | gRPC stream + mTLS      | gRPC stream + mTLS
          |                         |
+----------------------+  +----------------------+
| kind: hugofleet-a    |  | kind: hugofleet-b    |
| - Go Agent           |  | - Go Agent           |
| - Go Operator        |  | - Go Operator        |
| - Application CRD    |  | - Application CRD    |
| - Deployment/Service |  | - Deployment/Service |
| - pricing-api Pod    |  | - pricing-api Pod    |
+----------------------+  +----------------------+
```

### 4.1 核心邊界

- **Hub**：負責使用者、租戶、部署要求、版本歷史、權限和部署意圖；不負責直接維持 Pod。
- **Agent**：只接受經明確定義的操作（ApplyApplication、DeleteApplication、GetStatus 等），使用本地 ServiceAccount 存取有限的 Kubernetes 資源。
- **Operator**：在**每個有工作負載的 Worker Cluster** 內，以 CRD spec 作為期望狀態，管理 Deployment / Service。
- **Kubernetes**：是工作負載執行狀態的權威來源；Hub 的 SQLite 不是 Kubernetes 狀態替代品。
- **開發初期的例外**：M2 允許 Hub 使用限定 kubeconfig 直連單個 kind；M3 完成後此模式標為 dev-only，正式多叢集路徑不持有 Worker kubeconfig。
- **Namespace 過渡**：M1 的單機範例使用 bootstrap 腳本建立 `hf-demo` namespace；M2 可由本機 bootstrap 建立 `hf-<tenant>` 供開發使用。正式的多租戶 Namespace 建立、權限與配額由 M4 的受控流程落實，不得將 M2 的命名空間隔開就宣稱安全隔離。

### 4.2 狀態來源與離線行為

- Hub SQLite 保存「最後提交的意圖、版本快照、操作履歷」。
- Worker Cluster 的 `Application` CR spec 是當地 Operator 執行時的期望狀態；`Application.status`、Deployment/Pod status 才是實際觀測狀態。
- Agent 斷線：Operator **繼續自動修復**已存在的工作負載；Hub 暫停發送新指令，標記 Cluster `DISCONNECTED`，UI 顯示觀測時間。
- Agent 重連：重新驗證憑證、同步既有資源與狀態，並依 revision 檢查待處理命令；不得重播已過期版本覆蓋新版本。
- Hub 重啟：從 SQLite 恢復 Pending Operations；以 Agent 狀態重新核對，不直接把 Pending 全部標為成功。

### 4.3 反向通道的明確定義

本專案實作 **Agent-outbound 反向控制通道**，不是通用網路層 TCP Tunnel：

1. Agent 主動撥號至 Hub 的 gRPC server。
2. 通道使用長連線、gRPC bidirectional stream。
3. Hub 可以在同一條連線下傳送**受限的型別化命令**，Agent 回送 ACK、進度和狀態事件。
4. 使用 mTLS 並將憑證的 Cluster Identity 與登記的 `cluster_id` 綁定。
5. 支援心跳、斷線偵測、指數退避重連、取消與逾時、有限併發。
6. 不支援 `kubectl exec`、任意 shell、任意檔案傳輸或任意 Kubernetes 操作。

### 4.4 kind 與 Docker 網路

M3 採**可重現的網路方案**：

- 兩個 kind Cluster 使用 kind 建立的 Docker 網路；Hub Docker 容器也加入該網路。
- Bootstrap 腳本**動態解析** Hub 在該 Docker 網路的位址，注入 Agent 設定，不寫死容器 IP。
- Agent 撥號至實際 IP:port，但 TLS 驗證固定 `hub.hugofleet.local` ServerName（Hub 憑證必須包含對應 SAN）。
- 事先執行從 Worker Pod 到 Hub 的實際連線測試；禁止只驗證 Host → Hub 就宣稱跨叢集成功。
- 避免假設 `localhost` 或 Docker Container 名稱可直接從任意 Pod 的 DNS 解析。
- 若目標 OS／Docker 版本下無法連線，須先補一份 ADR 和替代方案（例如可靠的 NodePort/host-gateway 路徑），通過測試再繼續。

## 5. 核心業務模型

### 5.1 實體

| 實體 | 重要欄位 | 備註 |
|---|---|---|
| `tenants` | id, name, slug, created_at | 唯一 slug |
| `users` | id, oidc_subject, email, created_at | M4 加入，OIDC subject 作不可變身分 |
| `tenant_memberships` | tenant_id, user_id, role | 角色不能從使用者自填欄位信任 |
| `clusters` | id, name, state, last_seen_at, registered_at | Cluster ID 不可重複 |
| `applications` | id, tenant_id, cluster_id, name, desired_revision, applied_revision, last_successful_revision, status, last_observed_at | `(tenant_id, name)` 唯一；初版不跨 Cluster 遷移 |
| `releases` | id, app_id, revision, spec_snapshot_json, state, created_by, created_at | 每次部署儲存不可變的完整配置快照 |
| `operations` | id, app_id, release_id, command_id, type, state, error_code, error_message, created_at, finished_at | 非同步任務與重試控制 |
| `idempotency_keys` | tenant_id, key, request_hash, operation_id | 相同請求不可重複建 Release |
| `audit_events` | id, actor_id, tenant_id, action, target_type, target_id, outcome, occurred_at | 稽核，不存 token 或 secret |

`Operation` 是使用者發起的動作；`Release` 是一次不可變的應用規格；`Application.status` 是當下可用性。三者不要混為同一種狀態。

- `desired_revision`：Hub 最後接受的部署意圖，用於 optimistic concurrency 檢查。
- `applied_revision`：Worker 已確認寫入 CR 的版本；不代表 rollout 成功。
- `last_successful_revision`：最後驗證 rollout 完成的版本，首次成功前為空。
- revision 1 成功、revision 2 失敗時，上述欄位依序為 2、2、1；回滾建立 revision 3，成功後變成 3、3、3。
- Release 的配置快照不可變；其生命週期狀態可變。新的成功版本會將上一個 ACTIVE 標記為 SUPERSEDED；回滾來源另以 `rollback_source_revision` 記錄，不能把曾成功的來源版本誤標為失敗。

### 5.2 應用狀態

`PENDING` → `DEPLOYING` → `READY`。

失敗時使用 `DEGRADED`，Agent 失聯且資料過舊時 UI 顯示 `UNKNOWN`（同時保留 `last_observed_at`），刪除時 `DELETING` → `DELETED`。

`operations.state`：`QUEUED`、`DISPATCHED`、`RUNNING`、`SUCCEEDED`、`FAILED`、`TIMED_OUT`。

`releases.state`：`PENDING`、`ACTIVE`、`FAILED`、`SUPERSEDED`。

Cluster state：`CONNECTED`、`DISCONNECTED`。

### 5.3 核心業務規則

**BR-01：部署申請**

- 驗證 tenant、Cluster 存在且使用者有權限。
- 驗證應用名稱符合 DNS-1123 命名規則；image、port、replicas、resources 格式正確。
- 初版只接受明確欄位，不提供「任意 YAML」或「任意 PodSpec」輸入。
- Replicas 預設 1，MVP 上限 3；CPU/Memory 預設與最大值以設定檔管理。
- 提交後產生 `Operation`，回覆 `202 Accepted` + `operation_id`，不得在 HTTP 請求內等待整個 Deployment 完成。

**BR-02：部署版本**

- 每次配置變更都形成不可變 Release：完整 image、replicas、port、resources、probes 快照。
- Revision 從 1 遞增；`(app_id, revision)` 唯一。
- 同一應用**同時間只能有一個進行中的變更操作**；衝突回 `409 CONFLICT`。
- 帶 `Idempotency-Key`：同 key + 同 payload 回同一 operation；同 key + 不同 payload 回 `409`。
- Hub 的 SQLite 交易保證新增 Release/Operation 原子性。
- 所有部署／回滾變更提供 `expected_desired_revision`；與 Hub 現值不同回 409。失敗的 revision 仍消耗序號，不能重新使用。

**BR-03：成功判斷**

- 不能只靠 `Deployment` 存在或 `availableReplicas > 0` 就說新版本部署成功。
- 要核對 `observedGeneration`、新 ReplicaSet/Deployment rollout 完成條件、目標 image 及更新後 replicas 的 readiness。
- 超時或 `ProgressDeadlineExceeded` 必須報告原因；可以保留上一版正在服務，但新 Release 應標 `FAILED`。
- `pricing-api:v2-broken` 應可穩定重現本規則。
- Hub deadline 到期不代表 Kubernetes 停止 rollout。Operation 的 TIMED_OUT 是終態；晚到結果記為新的觀測／事件，不把該 Operation 改成 SUCCEEDED，也不覆蓋較新 revision 的狀態。
- Release 若已因 deadline 被判 FAILED，晚到的 Ready 必須明確顯示為該版本的觀測事實，不默默修改部署歷史；使用者以新的部署／回滾 Operation 重新確認期望版本。

**BR-04：回滾**

- 回滾是**新的部署操作**，用先前「已知可用」Release 的 spec 快照建立**新 revision**，不得修改舊 revision。
- M2 先支援使用者手動回滾；自動回滾非 MVP 必需。
- 回滾後需要重新確認新 revision 變成 `READY`，不能僅以 API 寫入成功當作回滾成功。

**BR-05：資源配額**

- M2 可先有單租戶預檢；M4 實作真正每個 Cluster 每個 Tenant 一個 namespace（如 `hf-team-a`）。
- 由 Kubernetes `ResourceQuota` + `LimitRange` 在 namespace 內強制限制 requests / limits / Pod 數量。
- Hub 預檢作為使用者體驗；**Kubernetes Admission 才是最後一道真正保護**。
- 預檢要考慮 replicas 和 RollingUpdate 的可能額外 Pod；超額時回傳可讀錯誤。
- 本版定義的是**每 Cluster/每 Tenant 配額**；不宣稱已有跨 Cluster 的全球資源池配額。

**BR-06：Cluster 可用性**

- M3 先採「使用者明確選擇 Cluster A/B」；Agent 沒連線時拒絕新的遠端部署，操作不可以默默丟失。Hub 在 M3 使用臨時 Demo 管理 Token 保護管理 API，與 Agent 的 mTLS 是兩條獨立的安全邊界。
- 保留操作履歷和失敗原因；M5 可增加有限的自動分派：過濾連線狀態／租戶允許清單／資源估算，再固定排序選擇。
- Kubernetes Scheduler 對實際 Pod placement 有最終決定權。

**BR-07：刪除**

- 刪除應用先建立 Delete Operation，再移除 Worker 的 `Application` CR。
- 對 Operator 建立的 Deployment / Service 設 `OwnerReference`；無外部資源時優先用 Kubernetes GC，不濫用 Finalizer。
- 刪除完成以真實 K8s 狀態確認；SQLite 保留 Audit/Release 歷史，不做硬刪。

**BR-08：安全**

- 不接受任意 shell、任意 cluster-admin kubeconfig、hostPath、privileged workload。
- 容器盡可能以非 root 執行；Sample Image 預設 non-root。
- 租戶操作必須對 tenant 資源做 server-side ownership / membership 檢查，不信任前端傳來的 tenant ID。
- 日誌與 Audit 不記錄 JWT、OIDC secret、私鑰、完整 Authorization header。

## 6. Kubernetes `Application` CRD

### 6.1 設計

- Group：`platform.hugofleet.io`
- Version：`v1alpha1`
- Kind：`Application`
- Scope：Namespaced
- Plural：`applications`
- Short Name（可選）：`hfapp`
- **Worker CRD 不含 `clusterRef`**：目標 Cluster 由 Hub 的 `applications.cluster_id` 選定，Agent 只在當前 Worker 建立 CR。

### 6.2 預期 YAML

```yaml
apiVersion: platform.hugofleet.io/v1alpha1
kind: Application
metadata:
  name: pricing-api
  namespace: hf-team-a
spec:
  revision: 1
  image: pricing-api:v1
  replicas: 1
  port: 8080
  resources:
    requests:
      cpu: 100m
      memory: 64Mi
    limits:
      cpu: 300m
      memory: 128Mi
  readinessPath: /readyz
  livenessPath: /healthz
```

> 這是期望格式；使用 Kubebuilder 產生 CRD/schema/deepcopy，**不得手改產出物而不同步 Go 型別**。

### 6.3 CRD 驗證

- M1 Application 名稱限制為最多 63 字元的 DNS label，確保可以安全用於工作負載 labels。
- `image` 不可空，不能使用空白或無效 image reference。
- `replicas` 1–3（此數值為可配置 Demo Policy；CRD schema 可設合理上限）。
- `port` 1–65535。
- `revision` 為正整數；M1 獨立範例明確提供 revision，M2 起由 Hub 產生。不可只用 annotation 作部署版本的判定依據。
- Requests/limits 必須是有效 Kubernetes `resource.Quantity` 且 limits 不小於 requests。
- Probe path 必須以 `/` 開始，禁止 URL 或完整主機位址。
- CRD spec 改動不應修改不能變更的 Service `clusterIP` 等 immutable 欄位。

### 6.4 Operator Reconcile 契約

每次 Reconcile：

1. 取得 `Application`；不存在則安全退出。
2. 檢查 spec 合法性並更新 Condition（有錯誤時不能無限快速重試）。
3. `CreateOrUpdate` 或 Server-Side Apply 管理同 namespace Deployment 和 ClusterIP Service；既有同名物件若不屬於當前 Application UID，回報 OwnershipConflict，不可接管或覆寫。
4. Deployment 應設定 ownerReference、labels/selectors、image、replicas、resources、probes、合理的 RollingUpdate 策略與 `progressDeadlineSeconds`。
5. 避免寫入不可變欄位，保留非本 Controller 擁有的 metadata。
6. 觀察 Deployment 現有 rollout 狀態，更新 CRD status（必要時 RequeueAfter；避免無限 update loop）。
7. Status 至少包含 `observedGeneration`、`observedRevision`、`readyReplicas`、`availableReplicas`、`image`、`conditions`；每個 Condition 使用標準 `observedGeneration`、`reason`、`message`、`lastTransitionTime`。
8. Conditions 至少包含 `Ready`、`Progressing`、`Degraded`，`reason/message` 要能定位問題。
9. Deployment／Service 漂移要被 Watch 或 Requeue 偵測並修復。
10. Worker 與 Hub 斷線時 Reconcile 仍要能獨立執行。

**Reconcile 禁忌**：不要只因新舊 Object 的無關時間戳不同而一直 Update；不要吞掉錯誤；不要以睡眠（sleep）等待 Deployment ready；不要只在 CRD Create 時才處理。

CRD 啟用 `/status` subresource。Deployment 的受管理配置須攜帶應用身分／revision，Controller 核對 CR revision、實際套用的 Deployment 配置及其 observedGeneration 後才能更新 observedRevision／Ready。相同配置但新 revision、只修改 replicas、回滾到曾經用過的配置都必須有測試；不能僅以 ReplicaSet 存在判斷成功。目前實作將 revision 同時寫入 Pod template，讓新 revision 觸發獨立 rollout，並核對目前 ReplicaSet 的錯誤歸屬；同 revision 重送維持冪等，見 [ADR 0012](docs/adr/0012-current-revision-replicaset-evidence.md)。

### 6.5 部署策略

MVP 採 RollingUpdate；為了維持可用性，預設 `maxSurge: 1, maxUnavailable: 0`，但必須說明額外 Pod 會占用 quota。M2 用足夠的 Demo quota 驗證，並測試 quota 不足造成 rollout 失敗時的可觀測性。

## 7. Hub REST API（M2 起）

API prefix：`/api/v1`，HTTP JSON；所有非健康檢查 API 都必須有清楚的輸入驗證、錯誤回應與 request_id。

| Method | Path | 功能 |
|---|---|---|
| GET | `/healthz` | 程序存活 |
| GET | `/readyz` | Hub DB／核心依賴就緒 |
| GET | `/api/v1/clusters` | 查詢已註冊 Cluster 與心跳狀態 |
| GET | `/api/v1/tenants` | 查詢可見租戶 |
| POST | `/api/v1/tenants` | 建立租戶（M4 限 Admin） |
| GET | `/api/v1/tenants/{tenantId}/apps` | 列出租戶應用 |
| POST | `/api/v1/tenants/{tenantId}/apps` | 建立應用並啟動首次部署 |
| GET | `/api/v1/tenants/{tenantId}/apps/{appId}` | 查詢應用、版本與觀測狀態 |
| POST | `/api/v1/tenants/{tenantId}/apps/{appId}/releases` | 建立新的部署版本 |
| POST | `/api/v1/tenants/{tenantId}/apps/{appId}/rollback` | 以過去版本快照建立新版本 |
| DELETE | `/api/v1/tenants/{tenantId}/apps/{appId}` | 非同步刪除 |
| GET | `/api/v1/operations/{operationId}` | 操作進度／錯誤 |
| GET | `/api/v1/audit-events` | 可見範圍內稽核紀錄（M4） |

### 7.1 建立應用請求範例

```json
{
  "name": "pricing-api",
  "cluster_id": "hugofleet-a",
  "image": "pricing-api:v1",
  "replicas": 1,
  "port": 8080,
  "resources": {
    "requests": { "cpu": "100m", "memory": "64Mi" },
    "limits": { "cpu": "300m", "memory": "128Mi" }
  },
  "readiness_path": "/readyz",
  "liveness_path": "/healthz"
}
```

**HTTP request header**：`Idempotency-Key: <unique-client-generated-key>`（建立／部署／回滾時必填）。

**成功回應**：`202 Accepted`，JSON 至少有：

```json
{
  "application_id": "app_...",
  "operation_id": "op_...",
  "revision": 1,
  "state": "QUEUED"
}
```

### 7.2 回滾請求

```json
{
  "target_revision": 1,
  "expected_desired_revision": 2
}
```

成功建立的是 revision 3（沿用 revision 1 的工作負載配置並注入新 revision），不是讓 revision 倒退；revision 2 即使失敗，expected_desired_revision 仍為 2。

### 7.3 錯誤格式

```json
{
  "error": {
    "code": "QUOTA_EXCEEDED",
    "message": "Tenant team-a CPU quota is insufficient",
    "request_id": "req_..."
  }
}
```

常見錯誤：`INVALID_ARGUMENT`（400/422）、`UNAUTHENTICATED`（401）、`PERMISSION_DENIED`（403）、`NOT_FOUND`（404）、`CONFLICT`（409）、`QUOTA_EXCEEDED`（422）、`CLUSTER_OFFLINE`（503）、`INTERNAL`（500）。對外錯誤不可泄漏敏感堆疊。

### 7.4 實作規則

- 管理 API 不允許同步等待 Pod Ready 才回傳。
- List API 有分頁上限，禁止一次撈出無限筆資料。
- 在資料庫 Transaction 寫入 Release、Operation、idempotency record。
- 禁止 Hub DB update 與遠端 K8s write 假裝是跨系統原子交易；使用 Operation 狀態機與重試補償。
- M2 可先以本機事件輪詢；M5 才加入更即時的 UI 更新。
- API 規格分為 M2／M3 的 `docs/openapi.yaml` 與 M4 OIDC 的 `docs/openapi-m4.yaml`；共用回應模型由後者引用前者，修改 endpoint 要同步修改測試與文件。

## 8. Agent / Hub gRPC 通訊（M3）

Protocol：Protobuf，Agent 作為 gRPC client 主動建立**單一雙向串流**；Hub 是 gRPC server。

建議 proto 契約（內容可依生成工具調整，但語意不可變）：

```proto
syntax = "proto3";
package hugofleet.agent.v1;

service AgentControlService {
  rpc Connect(stream AgentEnvelope) returns (stream HubEnvelope);
}

message AgentEnvelope {
  string message_id = 1;
  oneof payload {
    AgentHello hello = 10;
    Heartbeat heartbeat = 11;
    CommandResult result = 12;
    WorkloadStatus status = 13;
  }
}

message HubEnvelope {
  string command_id = 1;
  oneof payload {
    HubWelcome welcome = 10;
    ApplyApplication apply = 11;
    DeleteApplication delete = 12;
    RequestSnapshot snapshot = 13;
  }
}

// AgentHello: cluster_id、agent_version、capabilities。
// Heartbeat: timestamp / agent health。
// ApplyApplication: tenant/app namespace + validated Application spec + revision。
// CommandResult: command_id、success、error_code、error_message。
// WorkloadStatus: application identity、revision、observedGeneration、conditions。
// 每個命令 envelope：operation_id、application_id、revision／delete precondition、expires_at、payload_hash。
// Trace context 在每個 envelope 帶 traceparent／tracestate，不以單一 stream metadata 代替每個操作的 context。
// 各 message 的欄位型別與完整 proto 應由實作時補齊，並使用生成程式碼。
```

### 8.1 正確性規則

- Server 按客戶端 mTLS 憑證中的 Cluster Identity 驗證；`AgentHello.cluster_id` 必須完全匹配。
- `command_id` 唯一；同 command 重新送達不得造成多次有害副作用。
- Agent 拒絕比已套用 revision **舊**的更新；相同 revision + 相同 payload_hash 安全回 ACK，相同 revision + 不同配置必須拒絕。
- 不從 Hub 接收任意 Kubernetes GVK/manifest，只接收受約束的 Application 操作。
- 心跳建議每 5 秒，連續 3 次未收到時標記 disconnected；閾值可配置並須測試。
- 重連使用帶 jitter 的 exponential backoff，上限例如 30 秒，支援 context cancel 和 graceful shutdown。
- Agent 命令處理需有限併發；不可為每個訊息無限制開 Goroutine。
- ACK 表示「命令已驗證／已提交到 Kubernetes API」，**不等於部署完成**；最終結果由狀態同步判定。
- 同一應用有序操作；Hub 避免並行互相衝突的 Release／Rollback／Delete。
- 已套用 revision／命令去重資訊須能在 Agent 重啟後恢復；存活 CR 可提供 revision，刪除後仍須保留持久化 tombstone／高水位。M3 在 ADR 選定受控 Worker 儲存方式，不能只用記憶體 map。
- Delete 命令攜帶 application_id／Worker CR UID precondition；不能只按 namespace/name 刪除，以免過期命令刪到重建的同名應用。
- 命令有明確 deadline，已過期指令拒絕執行；Hub 重啟／Agent 重連先核對 Worker 狀態，再決定是否重送。同一 Cluster 僅一個有效 session，替換 session 後舊通道不得繼續操作。
- 測試至少涵蓋 Agent 重啟、ACK 丟失、Operation 逾時後晚到回報、同 revision 不同配置、過期 Delete 與重複 session。

### 8.2 mTLS 與開發憑證

- `make certs-dev` 於本機產生 Demo CA、Hub Server Cert（含 DNS SAN `hub.hugofleet.local`）與各 Cluster Client Cert。
- Agent 憑證使用能驗證 `cluster_id` 的 SAN Identity；Hub 驗證憑證鏈、到期、身份與註冊表。
- Agent 驗證 Hub 憑證鏈與指定 ServerName；**不能使用 `InsecureSkipVerify: true`**。
- 憑證、private key、SQLite 資料、測試帳密不得 commit；加到 `.gitignore`。
- Demo 憑證輪替可採手動重新發放；自動輪替屬延伸項，不要假稱已完成。
- 多叢集正式模式中，Hub 不保存 Worker cluster-admin kubeconfig。

## 9. 多租戶、OIDC 與 Kubernetes 安全（M4）

### 9.1 OIDC

- 本機示範建議 Dex，使用標準 Authorization Code + PKCE。
- M4 選定 Hub 處理 login／callback／logout；Hub 驗證 ID Token 後建立 server-side session，瀏覽器以 HttpOnly、SameSite cookie 呼叫管理 API。正式 HTTPS 設 Secure；HTTP loopback Demo 的例外需明確標示。
- 驗證一次性 state、nonce、PKCE verifier 與登記的 redirect URI；session 設過期時間、登出失效，cookie 認證的變更 API 必須有 CSRF 防護。ID Token、Access Token、平台 session 不混用。
- 驗證 issuer、audience、JWKS 簽章、expiration；拒絕無效 JWT。
- 使用 `sub` 綁定 User，不使用可變動 email 當唯一身份。
- UI 只渲染權限允許的操作；**真正的授權在 Hub server-side**。
- M2 允許 `DEV_AUTH_MODE=true`，但 Hub API **只能綁定主機 loopback**，不得視為已完成安全需求或用於遠端部署。
- M3 Hub 進入 Docker/kind 共用網路後，**禁止啟用無驗證 Dev Auth**：HTTP API 至少要求每次本機 Demo 隨機產生的管理 Token，主機 Publish 端口僅綁定 `127.0.0.1`，並設定本機防火牆／容器網路限制。這是暫時性 Demo 防護，不能宣稱是正式 IAM；M4 須改由 OIDC + RBAC。
- Demo OIDC 靜態測試帳號僅限本機，並在文件中標示。

### 9.2 Tenant Namespace / RBAC

- 每個 `(cluster, tenant)` 對應獨立 namespace（`hf-<slug>`）。
- Namespace bootstrap 與一般 Application 寫入權限分離：Admin 流程建立 Namespace、ResourceQuota、LimitRange、RoleBinding。
- Agent 僅擁有授權 namespace 中 Application CR 的必要權限；Operator 只擁有維護自身管理的 Deployment/Service 所需權限。
- 不賦予 Sample App ServiceAccount `cluster-admin`；若不需存取 Kubernetes API，則不授予額外權限。
- 使用 `Role` + `RoleBinding` 給租戶內工作負載／Agent 精準授權，避免把任意 cluster-admin 權限暴露給 Hub。
- Tenant A 的 Developer 無法透過 API 查看、修改或回滾 Tenant B 的應用。

### 9.3 ResourceQuota / LimitRange

至少對 `requests.cpu`、`requests.memory`、`limits.cpu`、`limits.memory` 及 `pods` 設約束。驗收為 ReplicaSet 建立 Pod 時被 Admission 拒絕，Deployment／ReplicaSet 的 FailedCreate／ReplicaFailure 與 exceeded quota 原因正確傳回 CR、Hub、UI；不能假設建立 Deployment 的 API 請求本身會因 Pod 配額失敗。

### 9.4 NetworkPolicy 的驗收限制

- kind 預設 CNI 不一定具備 NetworkPolicy enforcement。
- **只建立 NetworkPolicy 物件不算實現流量隔離**。
- M4 使用支援 NetworkPolicy 的 CNI（可於獨立單叢集 Security Profile 安裝）；實際測試 Tenant A → Tenant B 的連線被拒絕，且同 Tenant 正常連線。
- 如果當前環境只完成 Policy YAML，必須在 README 標示 `configuration only / not enforced`，不可在履歷聲稱網路隔離已驗證。

## 10. Web UI（M2 基礎版、M5 完整版）

Go `html/template` 由 Hub 提供頁面，使用 HTML 表單與 CSS；頁面處理與業務 API 共用 Go service 層。優先實用，不追求過度複雜設計。

必備頁面：

1. **Overview**：Cluster 總數、Connected 狀態、Application READY/DEGRADED 數量、最近操作。
2. **Clusters**：Cluster A/B 心跳、最後回報時間、Agent 版本、當前錯誤。
3. **Applications**：按 Tenant／Cluster 篩選、建立／部署表單、Image、Resources、Replicas、Conditions。
4. **Application Detail**：目前版本、歷史 Releases、Operations、事件、更新及回滾按鈕。
5. **Tenant / Members / Quota**（M4）：角色、配額、可見範圍。

UX 規則：

- 操作發起後先顯示 `QUEUED / DEPLOYING`，不可立刻顯示成功。
- 失敗顯示具體 reason 和 request_id。
- Cluster 失聯時顯示「資料最後更新時間」，不可顯示假即時資料。
- 危險操作（刪除、回滾）需確認。
- 初版以重新整理／有界的頁面自動更新取得狀態，不需要另建 JavaScript 應用；若要增加 SSE/WebSocket，必須先有明確效益與測試。

## 11. 可觀測性與交付（M5）

### 11.1 日誌／指標／追蹤

- Hub、Agent、Operator 使用結構化 JSON Logging，帶 `request_id / operation_id / cluster_id / tenant_id / app_id`（有值時）。
- Metrics 至少有：`hugofleet_operations_total`、`hugofleet_operation_duration_seconds`、`hugofleet_agent_connections`、`hugofleet_reconcile_errors_total`。
- 透過 OpenTelemetry 在 Hub → Agent 每個命令 envelope 帶 `traceparent`／`tracestate`；接收時提取並建立 Operation span，重試可用 Span Link。單一長期 bidi stream 的 RPC metadata 只描述該 RPC，不能代表其內所有命令的 Trace。
- Metrics／Tracing Viewer 可按需啟動，避免每次日常開發都開 Grafana、Prometheus、Collector。

### 11.2 Gateway API（非 MVP 但建議）

- M5 可支援 `HTTPRoute`，讓 Sample HTTP Service 有可訪問路由。
- **只有安裝 Gateway API CRD 並不足以真的路由**；必須部署相容 Gateway Controller 並以 HTTP 請求實測通過。
- 若 Gateway Controller 對筆電負擔太大，MVP 可用 `kubectl port-forward` 展示真實服務；Gateway API 的驗證放獨立 Demo Profile。

### 11.3 Docker / Helm / CI

- Go 服務使用 multi-stage Dockerfile、指定非 root Runtime（若映像支援）。
- Helm Chart 提供 Operator、Agent、必要 RBAC／CRD；Hub 可為 Docker Compose 或 Helm 的可選部署。
- kind 使用本機 Build + `kind load docker-image`，不強迫使用付費 Registry。
- GitHub Actions：format/lint、Go Unit、envtest、Proto generation check、建置、kind E2E（較重測試可獨立 job）。
- 基礎 format／vet／Unit／Build／M0 kind smoke CI 從 M0 建立，M1 增加 envtest／Operator E2E，M3 增加 Proto check；M5 整合交付與可觀測性，不延後已有測試的 CI。
- Secrets 不放進 Helm values Git 追蹤檔。

## 12. Repository 結構

為方便逐階段學習，實際程式、設定、Dockerfile 與 Go 測試依最初引入的階段分類；保留單一 Go module。入口與修改對照見 [程式索引](stages/README.md)／[ADR 0009](docs/adr/0009-stage-source-layout.md)。

```text
hugofleet/
├── AGENTS.md / README.md / HUGOFLEET_SPEC.md
├── go.mod / go.sum / Makefile
├── stages/
│   ├── README.md                     # 每階段多了什麼、舊路徑對照
│   ├── m0/                           # pricing-api、cmd/m0check
│   ├── m1/                           # api、controller、cmd/operator、config、build、test
│   ├── m2/                           # hub、cmd/hub/bootstrap-hub、web、config、build、test
│   ├── m3/                           # agent、profile/protocol/TLS/transport、api/proto、cmd、config、build、test
│   ├── m4/                           # OIDC/session、租戶 API/template、受控 bootstrap、真實安全驗收
│   └── m5/                           # Dashboard、telemetry、Helm／交付與驗收
├── tools/doccheck/                   # 共用文件驗證工具
├── scripts/                         # 共用開發／Docker／kind 工具與固定版本
├── deploy/kind/                     # 共用 kind node 設定
└── docs/                            # API／OpenAPI、ADR、驗收紀錄；階段操作與切換在 stages
```

M1 API／controller 與 M2 Hub 套件跨階段共用，不放在會阻止跨階段 import 的 internal 目錄。M3／M4 延用並修改 M2 Hub，導讀列明修改檔案；不複製歷史實作。M0～M5 的實作與測試範圍見各階段教學及 [驗證總覽](docs/validation.md)。運行資料位於 Git 忽略的 .local/tutorials 對應階段，建置工具、快取及 artifacts 也忽略。

每個階段自己的 README.md 集中完整教學及程式導讀，根目錄放 run.sh 與可修改 YAML／JSON，scripts/ 放該階段的啟動／工具／驗收 Bash。各階段所需檔案可由同一資料夾找到。

重要元件的深入 README 放在程式旁：M0 pricing-api、M1 controller、M2 hub、M3 agent／agenttransport。階段文件給完整操作、終端、預期與診斷；元件文件解釋函式、介面、狀態與修改位置。共用元件維護一份導讀，後面階段連回原位置。文件更新與實測證據分開記錄。

## 13. 開發階段與驗收條件（強制順序）

### M0 — 基礎環境與 Sample App

**目標**：使一個 kind Cluster 能穩定跑 `pricing-api`。

**實作**：

- 初始化單一 Go repo、format／vet、Makefile；Kubebuilder Scaffold 明確留到 M1。
- 一個單節點 kind 設定。
- `pricing-api:v1/v2-good/v2-broken`。
- Dockerfile / Makefile / 初版 README。
- 能用 port-forward 呼叫 `/quote`；實際計算符合測試。
- 使用原生 Deployment + Service manifest 部署 Sample App；M0 不建立 Application CRD 或 Operator。
- 建置版本在各自映像內；提供獨立、可清理的 kind smoke E2E，驗證三個映像與 readiness／liveness 行為。

**驗收**：

- `make kind-up` 能建立 Cluster。
- `make demo-image-load` 能把本地 Image 載入 kind。
- 可查 `pricing-api` 版本與價格計算結果。
- `make test-unit` 全綠。
- `v1` / `v2-good` rollout 完成，真實 `/version` 與 `/quote` 結果正確；`v2-broken` 程序存活、readiness 失敗、新 rollout 回報 ProgressDeadlineExceeded。
- M0 CI 涵蓋價格規則與三版映像的 kind E2E；跨階段 CI 的執行方式及對應提交的證據見 [驗證總覽](docs/validation.md)。

### M1 — 核心 Kubernetes Operator（第一個重要里程碑）

**目標**：`Application` CRD 真實自動建立並維持 Deployment + Service。

**實作**：

- CRD Type、Validation、DeepCopy、Manifest。
- Reconciler：Create/Update/Repair/Status/Deletion。
- `envtest` + kind E2E。
- CR 更新與 Deployment 漂移自動修復。

**驗收**：

- `kubectl apply -f stages/m1/application.yaml` 後自動產生 Deployment/Service。
- 改 image／replicas 後資源收斂。
- 手動篡改底層 Deployment replicas，最終恢復。
- status 使用 observedGeneration 和 Conditions 反映真實情況。
- 刪除 Application 後，由 ownerReference/GC 完成清理（kind E2E 驗證）。
- envtest 不拿來假裝測試真實 Pod Ready／GC；它沒有完整內建控制器。

### M2 — 真實業務 Hub + SQLite + 基礎 UI

**目標**：從管理 API/UI 完成首次部署、版本更新與手動回滾。

**實作**：

- SQLite migration + Domain / Service / Repository 分層。
- Tenant、Application、Release、Operation、Idempotency。
- REST API、OpenAPI、基本 Go html/template 頁面。
- 單叢集 dev-only kubeconfig transport（介面化，M3 替換）。
- 非同步部署與 timeout/failed event。

**驗收**：

- 用 UI 部署 v1 並成功呼叫 `/quote`。
- 發佈 `v2-broken` 會正確顯示部署失敗。
- 回滾以**新 revision** 恢復可用服務。
- API 重複相同 Idempotency-Key 不新增重複 Release。
- 同時對同一 App 兩次更新，第二次依政策被 409 拒絕。
- Hub 重啟後可以讀回已儲存的 Release/Operation。

### M3 — 兩個 kind、多叢集 Agent 與 mTLS

**目標**：Hub 不持有 Worker kubeconfig，仍可控制 A/B。

**實作**：

- kind A / B 啟動腳本；各裝 Agent、Operator。
- Hub gRPC server、Agent bidi stream、mTLS。
- Cluster Registry、Heartbeat、Command/ACK、狀態同步。
- 動態 Docker 網路 Bootstrap、斷線重連、逾時、命令去重。
- 刪除操作與重連後的狀態核對。

**驗收**：

- Hub API 可選 A 或 B，兩者都實際部署成功。
- 停止 Agent：Hub 顯示斷線，Operator 仍在 Worker 修復已部署服務。
- 重啟 Agent：重新握手，狀態回報恢復，沒有重複或過期命令覆蓋。
- 使用不可信 CA、錯誤 SAN、錯誤 cluster identity 都拒絕連線。
- Hub 未配置任何 Worker cluster-admin kubeconfig。
- E2E 真實測試 Worker Pod → Hub 的網路通道，不只檢查 Mock gRPC。
- 完成本階段即可作為公開履歷專案的核心 Demo。

### M4 — OIDC + 真正的租戶隔離／配額

**目標**：證明有實際的身份、授權、隔離與額度防護。

**實作**：

- OIDC 登入和 server-side RBAC。
- Tenant namespace、Role/RoleBinding、ResourceQuota、LimitRange。
- Security Profile：可執行的 NetworkPolicy 測試。
- 不同租戶讀寫隔離、稽核紀錄。

**驗收**：

- Tenant A 的 Developer 不能讀寫 Tenant B 的應用。
- 超出 CPU/Memory Quota 的部署被 K8s Admission 拒絕並反映到 UI。
- 無效／過期認證、session 與 CSRF 驗證失敗時被拒絕；state／nonce／PKCE／登出有實際登入流程測試。
- `NetworkPolicy` 必須有實際封包阻擋測試才算完成。
- Dev Auth 無法在非 loopback／非本機安全配置下啟用。

### M5 — 生產工程展示與 GitHub 包裝

**目標**：把已實作能力整理成可觀測、可重現的專案作品。

**實作**：

- 結構化日誌、OpenTelemetry Traces、Prometheus metrics。
- CI pipeline、Helm Chart、Docker 多階段建置。
- 完整 Go html/template Dashboard、Demo 操作影片。
- GitHub README、架構圖、技術 ADR、故障演練、限制清單。
- 有餘力才做 Gateway API / ConnectRPC 擴充。

**驗收**：

- 新環境依 README 可重現 Demo。
- Unit + envtest + kind E2E 全綠；CI 同步驗證。
- Dashboard 狀態與實際 Kubernetes 狀態一致。
- Demo 可展示故障、修復、回滾、多叢集連線與權限拒絕。
- README 寫清楚非生產級限制、各功能完成程度，沒有未驗證的行銷式宣稱。

## 14. 測試矩陣

| ID | 測試情境 | 測試層 | 最晚完成 |
|---|---|---|---|
| T01 | pricing-api 整數價格規則 | Go Unit | M0 |
| T01b | 三個 Sample 映像、HTTP 驗證、readiness/liveness 與原生 rollout | HTTP Integration / kind E2E | M0 |
| T02 | CRD 欄位驗證與預設值 | Unit / envtest | M1 |
| T03 | Application → Deployment + Service | envtest + kind | M1 |
| T04 | 更新 image/replicas，Operator 修復漂移 | kind E2E | M1 |
| T05 | status generation 和 rollout 真實性 | kind E2E | M1 |
| T06 | 刪除及 GC | kind E2E | M1 |
| T07 | 同一 Idempotency-Key 只產生一個操作 | Go/DB Integration | M2 |
| T08 | 同一 App 併發更新被拒絕 | Go/DB Integration | M2 |
| T09 | v2-broken 失敗，回滾建立新 revision | kind E2E | M2 |
| T10 | 雙 Cluster 分別部署 | kind E2E | M3 |
| T11 | Agent 斷線後 Operator 仍自動修復 | kind E2E | M3 |
| T12 | 錯誤 mTLS 憑證被拒絕 | gRPC Integration | M3 |
| T13 | Agent 重連、命令重送不覆蓋新 revision | Integration / E2E | M3 |
| T14 | Tenant A 無權操作 Tenant B | API Integration | M4 |
| T15 | 超額配置被 Kubernetes 拒絕 | kind E2E | M4 |
| T16 | NetworkPolicy 真實隔離 | kind + enforcing CNI E2E | M4 |
| T17 | Trace 能跨 Hub / Agent 串聯 | Integration | M5 |
| T18 | GitHub Actions 重新啟動環境並測試 | CI | M5 |

**測試原則**：

- `envtest` 沒有 kubelet／Deployment controller／GC，不把它的 object existence 當成 Running Pod 證據。
- Domain 的純邏輯可 Mock Store，但最終驗收必須跑真實 K8s／SQLite／gRPC。
- 每個 Bug 先補能重現的測試再修復。
- E2E assertion 不得依賴預先知道的 Sample App image ID／digest、Docker node IP 或手動建立的 Cluster；工具與 kind node 的依賴版本仍依第 4 節固定 digest。

## 15. Makefile 與開發命令契約

M0–M5 目前提供的命令（不得以空 target 假裝執行成功；完成程度以各階段驗收紀錄為準）：

```bash
make doctor             # 檢查 Go、Docker、kind、kubectl 等版本和網路
make tools              # M0: 安裝專案本地固定版本工具（Linux amd64 / WSL2）
make kind-up            # 啟動 hugofleet-a
make kind-down          # 清理目前 HF_CLUSTER 指定的 owned kind
make generate           # 生成 Go deepcopy
make manifests          # 生成 CRD / RBAC
make proto              # M3: 生成 Protobuf Go / gRPC
make demo-image-build   # 建立本地 sample images
make demo-image-load    # 載入目前已存在的 kind Cluster
make operator-install   # 在目標 Cluster 安裝 Operator
make m0-up              # 獨立 M0 學習叢集；m0-forward / m0-down
make m1-up              # 獨立 M1 學習叢集；m1-forward / m1-down
make m2-up              # 獨立 M2 學習叢集／DB；m2-forward / m2-down
make m3-up              # M3: 兩個 Worker + Docker Hub + Agents
make m3-hub-up          # 恢復同一個 M3 Hub 容器
make m3-down            # 清理本次 state 記錄的 owned M3 Hub／Workers
make agent-install      # M3: 安裝 Agent、憑證與配置
make certs-dev          # M3: 產生 Demo CA 與 Server/Client 憑證
make test-unit
make test-integration
make test-e2e
make test-e2e-m3
make m4-up              # M4: 真實 Dex／OIDC Hub、雙 Worker、租戶安全資源
make m4-status          # 核對本次 owned Hub／Dex／Workers
make m4-hub-stop        # 停止同一 Hub；m4-hub-up 恢復
make m4-hub-recreate    # 同一 state 重建 Hub／重裝 Agents，保留 DB／憑證／ledgers
make m4-forward         # 指定 tenant/app 的 loopback Service port-forward
make m4-down            # 清理本次 owned resources，保留私有 state
make m4-build m4-test
make test-e2e-m4        # 獨立新 state，真實 T14／T15／T16 與 owned cleanup
make helm-tools m5-chart-test # M5: 固定 Helm、typed render 與負向 values 驗證
make m5-up              # M5: Dashboard／telemetry Hub、雙 Worker 與 Helm
make m5-status          # 本次 owned Hub／Dex／Workers
make m5-hub-stop        # m5-hub-up 恢復；m5-hub-recreate 保留 DB／ledger
make m5-forward         # 自己 tenant/app 的 loopback Service
make m5-down            # owned cleanup，保留私有 state
make m5-build m5-test
make test-e2e-m5        # 全新 M5 security／Dashboard／trace／Helm 真實驗收
make docs-check         # 文件連結、Shell 語法與型別化範例
```

所有指令要有 `--help` 或 Makefile help、錯誤訊息和清理方法；`kind-down` 只能刪除本專案命名的 Cluster，不能動到使用者其他 Docker/Kubernetes 環境。

## 16. 非功能性要求

- Hub / Agent / Operator 優雅關閉，處理 SIGINT/SIGTERM。
- HTTP 和 gRPC 呼叫都有 `context`、timeout 和取消機制。
- 可配置心跳／重試／部署逾時，測試時可縮短以避免等很久。
- 日誌可讀、含 correlation IDs，敏感資料遮蔽。
- SQLite 使用 migration、foreign keys、適當 unique index 與 WAL；版本／操作更新不得出現競態重複建立。
- 不在程式碼中硬編 Docker network IP、Token、Credentials、CA private key。
- 不把各項服務依賴耦合成「一個不能單獨測試的大 main.go」。
- 專案默認只有**一個 Hub 執行個體**；SQLite / 單實例設計不能宣稱天然支援 HA。
- Sample Apps 使用輕量 image，避免大型下載與過度消耗資源。
- API 成功必須對應實際結果；不能只有前端 Toast 顯示成功。
- 禁止無聲的 catch-and-ignore、略過測試、把核心測試 `t.Skip()` 後宣稱通過。

## 17. AI Coding Agent 交付規範

### 17.1 每階段的標準工作方式

1. 先檢查現有程式碼與 README，列出**本階段最小可執行垂直切片**。
2. 提出要新增／修改的檔案、核心 Interface、測試案例。
3. 逐個功能實作，務必跑自動化測試。
4. 如果無法在目前環境啟動 Docker/kind，要明確標記 `NOT VERIFIED`，提供驗證指令；不可偽稱驗收通過。
5. 完成後更新 README、docs、ADR（發生架構調整時）。
6. 提供簡明交付摘要：完成內容、執行的測試及結果、未完成項、下一階段建議。
7. **未經要求，不要提前開發下一階段**。

### 17.2 Definition of Done

一項功能只有同時符合下列條件才算完成：

- 真正可運作，不是只有 Interface/Mock。
- 符合本規格的業務規則與安全邊界。
- 有至少對應的自動化測試。
- 真正需要 Kubernetes 的功能已在 kind 驗證，或誠實標記未驗證。
- README 有重現步驟，失敗情境有處理。
- 程式能格式化、Lint、Build，且無私鑰、密碼被提交。

### 17.3 專案決策優先順序

1. 使用者在對話中明確的新決策。
2. 本規格書和已核准的 ADR。
3. 既有 Repo 中已通過測試的 Interface / 資料模型。
4. 官方文件中的相容版本及 API 慣例。
5. AI 自己的實作偏好（不可擅自改變前四項）。

## 18. 官方參考（實作時仍須確認版本）

- Kubebuilder Book：https://book.kubebuilder.io/
- Kubebuilder envtest：https://book.kubebuilder.io/reference/envtest
- kind Quick Start：https://kind.sigs.k8s.io/docs/user/quick-start
- kind Config：https://kind.sigs.k8s.io/docs/user/configuration/
- Kubernetes ResourceQuota：https://kubernetes.io/docs/concepts/policy/resource-quotas/
- Kubernetes LimitRange：https://kubernetes.io/docs/concepts/policy/limit-range/
- Kubernetes NetworkPolicy：https://kubernetes.io/docs/concepts/services-networking/network-policies/
- ConnectRPC：https://connectrpc.com/docs/
- Gateway API：https://gateway-api.sigs.k8s.io/

## 19. 開發任務範圍

M0～M5 各有可執行實作、教學與驗收入口。修改前先確認指定階段與受影響的共用元件，再依第 17 節選擇相關驗證；不要重新建立空骨架或提前擴展功能。實際命令、程式提交與驗收限制見 [驗證總覽](docs/validation.md)。

---

**專案最終期待**：一個 32 GB RAM、無獨立 GPU 的筆電即可重現的 Go + Kubernetes + gRPC 平台，能以真實業務流程展示自動部署、控制器收斂、版本回滾、多叢集 Agent 安全通訊與租戶保護。
