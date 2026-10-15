# Hub Agent Transport：接受連線與派送命令

此套件在 **Hub 程序**執行，實作 [M2 Service.Transport](../../m2/hub/service/service.go)；與 Worker 的 [Agent](../agent/README.md) 共同完成 M3。完整操作與診斷從 [M3 教學](../README.md) 開始。

## 檔案與周邊元件

| 位置 | 責任 |
|---|---|
| [transport.go](transport.go) | TLS gRPC server、session、Call、Apply／Observe／Delete |
| [transport_test.go](transport_test.go) | 握手、身分、心跳、bounded 派送與結果契約 |
| [control.proto](../api/proto/hugofleet/agent/v1/control.proto) | Connect 雙向 stream 與型別訊息 |
| [agenttls/tls.go](../agenttls/tls.go) | TLS 1.3、CA、Worker URI 身分解析 |
| [Hub main.go](../../m2/cmd/hub/main.go) | M3 分支組裝 Transport／listener，不建立 Worker kube client |
| [Hub Runner](../../m2/hub/service/service.go) | 業務 deadline、DB 操作、各 cluster 狀態與推進 |

## 為何 Agent 主動連線仍能收到 Hub 命令

`Connect` 是一條 gRPC 雙向 stream。Worker 先連到 Hub，成功後兩方都能在同一連線送訊息；Hub 不需要反向撥 Worker API，也不保存 Worker kubeconfig。

```text
Worker Agent ──建立 mTLS／Hello──→ Hub.Connect
Worker Agent ←──Welcome────────── Hub.Connect
Worker Agent ──heartbeat／Result→ Hub.Connect
Worker Agent ←──HubEnvelope────── session queue／Call
```

## Connect 接受前檢查什麼

1. gRPC TLS peer 必須有已驗證憑證鏈與 client cert。
2. cert 唯一 URI SAN 必須是 `spiffe://hugofleet.local/cluster/<id>`，並符合受支援 identity。
3. id 必須在本次 Hub registry；5 秒內收到的第一個訊息必須 Hello，cluster_id 與憑證身分完全相同。
4. 同 cluster 不得已有活躍或還在排空的 session；不是靠後來 Hello 把舊 session 偷換掉。

Client 驗 Hub SAN，Hub 驗 client CA／URI／registry／Hello；管理 HTTP 的 Token 是另一路驗證，不能拿 Token 代替 mTLS。

## session 與容量限制

每個 session 有最多 8 筆派送 queue 與受限 pending 呼叫，由單一 stream writer 送命令。Result 用 command_id 找回等待的 Call；未知／晚到結果不會隨意完成別的操作。

heartbeat 更新 last seen，預設約 15 秒沒有心跳就斷線；Hub CLI 的 --heartbeat-timeout 可設定 3s～5m，需大於 Agent 間隔，建議至少三倍。檢查迴圈與 stream writer 分開，使卡住的 HTTP/2 寫入不阻止斷線判斷。斷線後保留 seen／version，並等待舊命令最晚有效期限排空；單次命令有效期最多約 5 秒，避免重連的新舊執行重疊。

Runner 依 cluster 讀連線狀態。A 斷線不會把 B 也標成失敗；全域 /readyz 可能因任一 Agent 離線回非成功，仍應逐 cluster 判讀 B 能否操作。

## 一個 Call 怎麼走

Call 確認 session／心跳、計算 payload hash、登記 command_id 的等待者，再進 queue。等待 CommandResult、session 關閉或 context 到期，結束後撤除 pending。

回來的 status 不能只看 Ready=true：還核對 Application ID／tenant／namespace／name／revision／UID，以及回報 UID 的一致性。Ready 須同時 Applied、非空 UID、正 generation、對應 observedRevision 和 observedGeneration，才能提供給 Runner。

Operation 的持久 command_id／deadline 從 Service.OperationContext 帶入修改命令。單次 RPC 最多約 5 秒，並受整次 Operation 原 deadline 限制；一次 RPC 逾時不表示整個操作立刻失敗，Runner 可在原期限內核對／重試。

## Apply、Observe、Delete 分工

| 方法 | 實際行為 |
|---|---|
| Observe | 發 RequestSnapshot，取得完整目前 spec／rollout 的觀測 |
| Apply | 先 Observe；若已 Applied，回已確認 UID；需要變更才發原持久命令 |
| Delete | 先讀刪除 snapshot、恢復 UID 綁定資訊；有持久 UID 才發 DeleteApplication |
| TenantProfile（M4） | 唯讀 command 核對已 bootstrap 的 tenant／cluster／namespace、receipt UID、Quota／LimitRange；不下傳 namespace／RBAC 寫入 |

Apply 的 snapshot 先行很關鍵：CR 寫入後 ACK 遺失，重啟不應無條件再次覆蓋。Worker ledger 與當地 CR 是重試判斷的一部分，不只依 Hub 記憶體。

CR 已提交後，Runner 仍要 Observe 到真正 Ready 才 Store.Finish SUCCEEDED。Transport 不直接改 SQLite 終態；歷史失敗／逾時不能被晚到結果改寫。

## 協定與修改方式

協定入口是 [control.proto](../api/proto/hugofleet/agent/v1/control.proto)，生成的程式在 api/agent/v1。型別／hash 轉換在 agentprotocol。改協定要同時考慮 Hub／Agent 的欄位與驗證，再用固定工具生成；`make proto`／`proto-check` 的操作見 [M3 導讀](../README.md#本階段的程式導讀)。

TLS、registry 與 Hello 必須一起核對；不新增任意 shell／YAML payload。重啟／ACK 遺失、負向 TLS、雙 kind 的實測見 [M3 紀錄](../../../docs/m3-validation.md)。跨階段驗證方式見 [驗證總覽](../../../docs/validation.md)。
