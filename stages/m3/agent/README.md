# Worker Agent：連線、命令與持久化保護

操作從 [M3 教學](../README.md) 開始。M3 Agent 在每個 Worker 的 hf-demo 執行，主動連 Hub；它提交 Application CR，底層 Deployment／Service 仍由 [M1 Operator](../../m1/controller/README.md) 維持。M4 沿用此程式，改用 hf-system ledger、明確 tenant allowlist 與 namespaced RoleBinding；完整操作與邊界見 [M4 bootstrap](../../m4/bootstrap/README.md)。

## 程式與依賴

| 檔案／元件 | 責任 |
|---|---|
| [cmd/agent/main.go](../cmd/agent/main.go) | 讀 hub／cluster／certs，從 Pod 取得當地 client，組裝 Engine／Client |
| [client.go](client.go) | TLS gRPC 撥號、Hello、heartbeat、收命令／送結果、重連 |
| [engine.go](engine.go) | 型別命令驗證、revision／UID、ledger、CR 寫入／刪除 |
| [engine_test.go](engine_test.go) | 命令重送、過期、衝突、ledger 等規則的例子 |
| [agentprotocol](../agentprotocol/projection.go) | proto 與 Application spec／狀態轉換、hash |
| [agenttls](../agenttls/tls.go) | Hub SAN／CA 驗證、leaf client identity |
| [M2 kube transport](../../m2/hub/kube/transport.go) | 共用當地 CR／workload 核對；Agent 使用當地 SA client |
| [install.yaml](../config/agent/install.yaml) | Worker 最小權限、named ConfigMap、Deployment、證書 mount |

## 一條連線怎麼建立與維持

Client.Run 反覆呼叫 connect。connect 使用憑證主動向 Hub endpoint 建 gRPC stream，先送 cluster_id、Agent 版本、能力的 Hello，最多等 5 秒 Welcome。Hub 憑證以固定 ServerName hub.hugofleet.local 驗證，撥號 IP 可以是動態解析出的 Docker IP。

握手後，Agent 預設每 5 秒送 heartbeat。命令接收 channel 容量為 1，同時間只有一個 executor；執行 Engine 時仍可維持 heartbeat。結果與 heartbeat 的 stream.Send 由同一個主迴圈執行，避免多個寫入者互相競爭。

斷線時取消當前 context，等待正在執行的命令收尾，再重新連線。預設退避基準從 1 秒倍增到最多 30 秒，實際等待為基準的 50%～100% jitter。連線穩定超過 30 秒再斷，基準重新從設定的最小值開始。

M3～M5 共用 Agent CLI 的 `--heartbeat`（預設 5s，1s～30s）、`--reconnect-min`（預設 1s，至少 100ms）、`--reconnect-max`（預設 30s，上限 5m，必須不小於 min）。本地管理員可在受管理的 Agent Deployment args 設定這三個具名旗標；M5 可使用 Chart 的 timing values。重新執行完整教學安裝會套回入口的預設值，僅重建 Hub 則保留 Agent 設定。

Hub 的 `--heartbeat-timeout` 預設 15s，允許 3s～5m；應至少為 Agent heartbeat 的三倍，兩端不會自動協商。M3／M4／M5 的正式容器入口分別讀 HF_M3_HEARTBEAT_TIMEOUT／HF_M4_HEARTBEAT_TIMEOUT／HF_M5_HEARTBEAT_TIMEOUT，需重建 Hub 才生效。連線閾值不會延長已持久化的 operation deadline；部署逾時另由 `--operation-timeout` 控制。

這裡是長連線的型別控制通道；不執行遠端 shell、exec 或任意 YAML。

## 跟一次 ApplyApplication

```text
HubEnvelope：command_id／operation_id／expires_at／payload_hash／Application 身分與 spec
→ Engine.Execute：鎖住一個執行流程
→ 檢查命令與身分
→ 載入 named ledger，核對 revision／UID／hash
→ 先保存最新接受的 revision 與配置 hash
→ Workloads.Apply：寫入真正 Application CR
→ 再保存取得的 CR UID
→ CommandResult：Applied=true、UID
→ Hub 再 RequestSnapshot 核對完整 rollout
```

Execute 先拒絕空／超長 command_id、過期期限、payload hash 不符、不支援型別、錯 namespace／tenant／Application ID／名稱。修改還要求 operation_id，期限被帶進 context；M3 保持固定 demo／hf-demo，M4 則核對受信任 allowlist 與固定 hf-<tenant> namespace。ledger 加入 tenant 綁定，禁止同 app_id 跨租戶重用；M4 可另執行唯讀 profile command，不增加 namespace／quota／RBAC 寫入能力。

為何先存 ledger 再 Apply？假如新 revision 的命令收到了，還沒來得及回 ACK 就斷線／重啟，Worker 仍知道自己已接受哪個最高 revision，不會被舊指令覆蓋。

為何 Apply 後還要再存 UID？同名 CR 可能刪除重建，UID 讓下一次核對知道是否還是原物件。第二次保存失敗就回錯誤；不能僅因 CR 寫入成功而假裝完整 ACK。

Applied=true 只代表 CR 已提交。Ready、revision／generation、Deployment spec 與全部副本仍由觀測流程確認，最後才由 Hub 結束 Operation。

## ledger 保存在哪裡

KubeLedger 只讀／更新預先建立的 `hf-demo/configmap/hugofleet-agent-ledger`，JSON 在 data 的 ledger.json。資料以 Application ID 為 key，包含：

| 欄位 | 用途 |
|---|---|
| revision／spec_hash | 拒絕舊 revision、同 revision 不同配置 |
| uid／name | 同名外來物件不能被接管或刪除 |
| command_id／command_hash | 同命令 ID 不同 payload 拒絕 |
| deleted | 刪除 tombstone，防止舊 Apply 令應用復活 |

ConfigMap resourceVersion 保護更新；衝突回錯，下一命令重新載入。JSON 超過 512 KiB 回 LEDGER_FULL，採拒絕變更，不自動丟掉 tombstone。

ledger 持久化是指 Agent 程序／Pod 重啟後從 Worker Kubernetes 讀回；它不保證整個 kind 或預設 RAM etcd 被刪除／重啟後仍存在。新 Worker 要新 Hub state，不能把舊 DB UID 當作新資源。

## Delete 與 Snapshot

Delete 要求已綁定的 CR UID，先 DeletionStatus 核對當地身分，再保存 tombstone，之後以 UID precondition 刪 CR。重送同一刪除可繼續核對；其他命令試圖覆蓋 tombstone 或復活 Application 會拒絕。CR 刪除後還要等相依 Deployment／Service／ReplicaSet／Pod 的 GC。

Snapshot 不改 ledger 的 revision 或工作負載。帶 spec 時讀實際配置／rollout；不帶 spec 時讀刪除進度。Hub 以 snapshot 核對再決定是否重送原意圖。

| 錯誤 | 想告訴你什麼 |
|---|---|
| EXPIRED | 這個命令的期限已過或命令身分無效 |
| INVALID_ARGUMENT | payload hash／配置／必要欄位不合法 |
| STALE_REVISION | 比 Worker 已接受的 revision 舊 |
| CONFLICT | 同 revision／command ID 的內容不一致，或刪除衝突 |
| OWNERSHIP_CONFLICT | ledger／CR 的名稱或 UID 不一致 |
| DELETED | tombstone 拒絕復活 |
| LEDGER_FULL | 不再接受新 ledger 寫入，需受控處理容量 |
| UNAVAILABLE | 當地 Kubernetes 存取／保存無法完成；不能當成成功 |

這些是 gRPC CommandResult 的 error_code，不都是 HTTP status；Hub 再轉為 Operation 與 API 的結果。

## 權限與修改位置

Agent SA 可操作 hf-demo 的 Application、讀必要工作負載狀態，以及 get／update 唯一 ledger。它不寫 Deployment、不讀一般 Secrets、不 exec、不建立 namespace。憑證由 bootstrap 掛入 Pod，不等於 SA 有 Secrets API 讀取權。

改連線與退避看 client.go，改命令接受／去重看 engine.go，改 wire 型別看 proto／projection，改 CR 核對看共用 kube transport。不要直接改生成的 pb.go，也不要藉 RBAC 擴大成管理員工具。

診斷命令與離線練習見 [M3](../README.md#操作遇到問題時)。Unit 的可控 backend 用來測特定規則；真實 TLS／雙 Worker／恢復與 GC 的實測看 [M3 紀錄](../../../docs/m3-validation.md)；跨階段驗證方式見 [驗證總覽](../../../docs/validation.md)。
