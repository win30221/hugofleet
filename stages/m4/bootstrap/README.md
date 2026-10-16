# Worker bootstrap 與唯讀安全 profile

本頁是元件導讀。實際練習見 [M4 逐步操作](../README.md#逐步操作)；遇到執行問題先查[排查方式](../README.md#操作遇到問題時)，結束或重開環境依[停止與下次繼續](../README.md#停止與下次繼續)。修改後的驗證範圍見 [M4 驗收](../../../docs/m4-validation.md)。

## 什麼時候需要這個工具

第一次學習不需手動 bootstrap，`make m4-up` 會準備固定安全資源。只有本地管理員要理解或調整 Worker 配額／固定材料時，才依 [M4 教學的管理員操作](../README.md#管理-api-與安全邊界)使用具名 action。

| 目的 | 選擇 | 核對結果 |
|---|---|---|
| 第一次組裝整課 | M4／M5 up | 真實 profile 與已驗證 Agent 上線 |
| 修改指定租戶 quota | tenant action | 指定 Worker／tenant 資源與 receipt 一致，再同步 Hub profile |
| 只重建 Hub | hub-recreate | 保留 quota／receipt／ledger，更新 endpoint |

不要用完整 up 代替只恢復 Hub，前者會重套教學預設安全配置。修改程式後從專案根目錄執行 `make m4-test`；Admission、Policy 與真實權限由 M4 E2E 驗證。

## 固定安全資源與 ownership

完整操作見 [M4 教學](../README.md)，設計見 [ADR 0010](../../../docs/adr/0010-oidc-tenants-and-worker-bootstrap.md)。[provision.go](provision.go) 只由本地管理員的 [bootstrap-worker](../cmd/bootstrap-worker/main.go) 呼叫；普通 Agent 與 Hub 沒有 bootstrap 寫入能力。

管理員明確指定 owned kind Worker 與 loopback HTTPS kubeconfig。工具只建立固定的 namespace、Quota、LimitRange、Role／Binding、Operator、Policy、受保護 receipt，或安裝受限 Agent。每個資源都有 manager／cluster／tenant label；更新前核對 ownership，不接管同名 foreign resources。這個工具不接受任意 YAML、shell、exec 或跨專案 kubeconfig。

Tenant ID 是 immutable DNS slug，namespace 固定 hf-<slug>。Quota 五項：requests.cpu／requests.memory／limits.cpu／limits.memory／pods。LimitRange 的 container minimum 為 10m／16Mi，maximum 為 1 CPU／512Mi，並提供 request／limit defaults。Operator 只維護本 namespace 的 Application 狀態與 Deployment／Service；Sample 不掛 SA Token。

Agent 在 hf-system，以固定 allowlist 和 namespaced RoleBinding 操作 Application、讀取必要 status，另對自己的指定 ledger ConfigMap 有 get／update。TLS Secret 只含 CA 與當前 Worker 的 client key pair，投影成 Agent 既有的 cluster-name 檔名；Agent 沒有讀 Secrets API 的權限。重裝不清空 ledger 的 revision／tombstone 高水位。

M4 的 --action endpoint 只修改已驗證 ownership 的 Agent Deployment 中唯一的 --hub 參數，拒絕非 private IP:18443、foreign Deployment 及 M5 模式。hub-recreate 使用此入口保留租戶配額與 receipt；M5 改由 Helm --reset-then-reuse-values 更新 endpoint（先載入新 Chart 預設，再覆蓋既有 release 值及新 endpoint）。完整 tenant action 則是管理員明確要求重設所指定配額的操作，兩者不可混用。

receipt 在完整 bootstrap 最後寫入；Agent 使用共用的 [agentprofile.Reader](../../m3/agentprofile/profile.go) 唯讀核對真實 receipt UID、namespace／cluster／tenant、Quota spec/status，以及 LimitRange 的 container 型別、maximum 與 request／limit defaults。minimum 由本地 bootstrap 建立，profile 回報不包含該欄位。普通 Agent 不能改 receipt、Quota、RBAC 或 Namespace。Hub Admin 向已連線 Agent 取得型別化 profile，核對後才啟用租戶與 Owner；DB 存在不代表 Worker 建立成功。

A Security Profile 的 Calico enforcement 由 [kind lifecycle](../../../scripts/kind.sh) 安裝。Policy 限制 ingress 為同 namespace Pod；B 的 kindnet 不宣稱 enforcement，egress／host／跨叢集隔離也不在本階段承諾內。實際 Go probe、Admission 與 SA 權限測試見 [kind E2E](../test/e2e/security_test.go)。

## 讀完後去哪裡

- 返回 [M4 逐步操作](../README.md#逐步操作)，對照本輪結果。
- 需要停止、換課或再次練習，前往 [M4 完成後的選擇](../README.md#完成這一課後去哪裡)。
- 想繼續理解其他元件，回到 [M4 原理與進階閱讀](../README.md#原理與進階閱讀)。
