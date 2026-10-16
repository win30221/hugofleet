# ADR 0008：M3 主動連線 Agent 與階段獨立執行

狀態：已採納。範圍：M3 控制通道，M4／M5 共用。

控制通道提供有界 timing 設定，預設為 Agent heartbeat 5s、Hub timeout 15s、重連基準 1s～30s。Agent CLI 接受 heartbeat 1s～30s、100ms ≤ reconnect-min ≤ reconnect-max ≤ 5m；Hub timeout 為 3s～5m，建議至少三倍 heartbeat。M5 Helm 提供整秒 values；命令 deadline、單一 executor 與 ledger 邊界維持原設計。設定入口見 [Agent 說明](../../stages/m3/agent/README.md)。

M2 已有 durable Operation、不可變 Release 與 Transport 介面。M3 沿用同一業務處理器，增加 `m3` 模式：Hub 不建立 Kubernetes client，不掛載 Worker kubeconfig；每個 Worker 以 namespace ServiceAccount 執行 Agent，主動向 Hub 建立單一 mTLS Protobuf gRPC 雙向串流。M2 仍以 `m2` 模式使用受限本機 kubeconfig。

網路沿用規格：兩個 kind 與 Hub 容器加入 kind Docker 網路，bootstrap 每次從 docker inspect 解析 Hub IP。Agent 以 IP 撥號，驗證固定 `hub.hugofleet.local` SAN。Hub 驗證 CA、到期、clientAuth、唯一 URI SAN `spiffe://hugofleet.local/cluster/<cluster_id>`，再核對 Hello 與啟動時登記表。管理 HTTP publish 只綁 `127.0.0.1`；HTTP handler 僅允許 kind 網路 gateway 的精確來源 IP，要求隨機 32-byte Demo Token。UI 使用 Token 登入換取 8 小時 HMAC session cookie，保留 Host、Origin、CSRF 與危險動作確認。這不是正式 IAM。

命令只含 ApplyApplication、DeleteApplication、RequestSnapshot，配置為受約束的 ApplicationSpec 欄位。沒有 YAML／任意 GVK／shell／exec。每個 mutation 保留 SQLite command_id、operation_id 與 deadline；單次嘗試再受 5 秒上限約束。ACK 只表示 CR 已提交；Ready 由 Worker 重用 M2 的 exact spec、CR conditions、Deployment rollout 與 Service 核對。Trace envelope 攜帶每命令的追蹤契約，M5 入口啟用 OpenTelemetry。

Worker 持久化採 **預先建立的單一 `hf-demo/hugofleet-agent-ledger` ConfigMap**。Agent 只能 get／update 此 resourceName，不能 create／delete 或讀寫其他 ConfigMap。Application spec intent／revision 高水位先 durable 保存，再呼叫 Kubernetes；ACK 前保存 UID。資源版本保護 ConfigMap 寫入，發生 conflict 由 Hub 重試。重啟後讀回 ledger，存活 CR 再提供 exact spec／revision／UID 核對。同 revision 配置不同、舊 revision、逾時命令拒絕；刪除先核對 Hub application label、CR UID／revision，再 durable 保存 tombstone，使用 Kubernetes UID delete precondition。tombstone 不自動清除，避免 ACK 丟失與程序重啟後把已刪除應用重新建立。ledger 超過 512 KiB 時 fail closed；容量擴展／移轉留待具體後續任務。

Agent 使用一個 executor 與大小 1 的接收緩衝；Hub 每 session 最多 8 個待回應命令。Agent 每 5 秒 heartbeat，Hub 15 秒逾時；重連採 jitter exponential backoff（1–30 秒）。Hub 拒絕仍有效的重複 session，不搶占 live session。舊 stream 移除後，新 session 必須等先前所有已發送嘗試的 deadline 過期才可建立；Agent 失聯取消 execution context並等待單一 executor 結束。避免舊通道與新通道同時派送。Kubernetes 已接受的 mutation 不能被 RPC cancel 撤回，須在重連後以 snapshot 核對；這正是 intent journaling／UID／revision／tombstone 的用途。

Hub 重啟及 Agent 重連，每次重新下發 mutation 前先 RequestSnapshot。相符 CR 直接恢復 Applied acknowledgement，不重新 update CR；刪除 snapshot 是唯讀且等待實際 GC。過期 Operation 保持 TIMED_OUT，晚到 Ready 只更新目前觀測，保留 FAILED Release／最後成功 revision。003_agent.sql 在交易中加入 agent_version 並保留既有履歷；共用 Store 會繼續套用至 schema 5，功能依入口模式配置，見 [Hub 導讀](../../stages/m2/hub/README.md)。M2 與 M3 使用不同 DB，啟動時拒絕模式混用。

獨立學習入口將 M0／M1／M2 分別放在 hugofleet-m0-demo／hugofleet-m1-demo／hugofleet-m2-demo；M3 為 hugofleet-a／b。各階段不要求先跑前一階段，關閉後可以重跑。各階段 E2E 使用獨立驗收入口。32 GB 筆電建議一次只啟動一個階段。

依 [ADR 0009](0009-stage-source-layout.md)，各階段程式、YAML／JSON、Bash、完整教學及程式導讀集中於 stages/m0～m5；學習入口實際使用各自 manifest／policy。產生的 kubeconfig、ownership、M2 SQLite／受限憑證、M3 SQLite／TLS／Token 位於 .local/tutorials 對應階段，與文件及版本控制隔離。共用元件只維護一份；docs 提供 API／ADR／驗收證據，stages/README 提供階段切換索引。

版本固定：grpc-go 1.82.1（要求 Go 1.25，沿用 Go 1.27.2）、protoc 33.0、protoc-gen-go-grpc 1.6.2（要求 Go 1.25／grpc 1.81）、protoc-gen-go 與 runtime 使用相同 `v1.36.12-0.20260120151049-f2248ac996af`。沿用 controller-runtime 0.25.1／Kubernetes 1.37.0。Compiler release zip 用官方 release asset SHA-256 校驗，Go module 使用 go.sum。官方依據：[grpc-go go.mod](https://github.com/grpc/grpc-go/blob/v1.82.1/go.mod)、[Go gRPC generator go.mod](https://github.com/grpc/grpc-go/blob/cmd/protoc-gen-go-grpc/v1.6.2/cmd/protoc-gen-go-grpc/go.mod)、[protoc release](https://github.com/protocolbuffers/protobuf/releases/tag/v33.0)、[Go generator/runtime 原則](https://github.com/protocolbuffers/protobuf-go/blob/master/README.md)、[Protobuf 相容性](https://protobuf.dev/support/cross-version-runtime-guarantee/)。

M3 使用單一 Hub、單一 demo namespace、人工 registry、SQLite 與 kind RAM etcd。leaf 憑證有效 24 小時，到期使用新 state 練習；保留既有資料的原地輪替未驗收。ConfigMap 是 Worker Kubernetes 儲存，不保證 Docker node／主機重啟後仍存在；完整持久叢集／備份恢復需另驗收。此 ADR 記錄設計；實際測試結果以 M3 validation 為準。

## 閱讀後返回

本頁用來查設計原因，不是安裝步驟。返回 [M3 原理與進階閱讀](../../stages/m3/README.md#原理與進階閱讀)，或從[選課與回訪](../../stages/README.md#回來繼續哪一課)接續使用此共用設計的其他階段。其他設計問題見 [ADR 導覽](../README.md#設計問題與-adr-對照)。
