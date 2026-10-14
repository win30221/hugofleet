# Worker bootstrap 與唯讀安全 profile

完整操作見 [M4 教學](../README.md)，設計見 [ADR 0010](../../../docs/adr/0010-oidc-tenants-and-worker-bootstrap.md)。[provision.go](provision.go) 只由本地管理員的 [bootstrap-worker](../cmd/bootstrap-worker/main.go) 呼叫；普通 Agent 與 Hub 沒有 bootstrap 寫入能力。

管理員明確指定 owned kind Worker 與 loopback HTTPS kubeconfig。工具只建立固定的 namespace、Quota、LimitRange、Role／Binding、Operator、Policy、受保護 receipt，或安裝受限 Agent。每個資源都有 manager／cluster／tenant label；更新前核對 ownership，不接管同名 foreign resources。這個工具不接受任意 YAML、shell、exec 或跨專案 kubeconfig。

Tenant ID 是 immutable DNS slug，namespace 固定 hf-<slug>。Quota 五項：requests.cpu／requests.memory／limits.cpu／limits.memory／pods。LimitRange 的 container minimum 為 10m／16Mi，maximum 為 1 CPU／512Mi，並提供 request／limit defaults。Operator 只維護本 namespace 的 Application 狀態與 Deployment／Service；Sample 不掛 SA Token。

Agent 在 hf-system，以固定 allowlist 和 namespaced RoleBinding 操作 Application、讀取必要 status，另對自己的指定 ledger ConfigMap 有 get／update。TLS Secret 只含 CA 與當前 Worker 的 client key pair，投影成 Agent 既有的 cluster-name 檔名；Agent 沒有讀 Secrets API 的權限。重裝不清空 ledger 的 revision／tombstone 高水位。

M4 的 --action endpoint 只修改已驗證 ownership 的 Agent Deployment 中唯一的 --hub 參數，拒絕非 private IP:18443、foreign Deployment 及 M5 模式。hub-recreate 使用此入口保留租戶配額與 receipt；M5 改由 Helm --reset-then-reuse-values 更新 endpoint（先載入新 Chart 預設，再覆蓋既有 release 值及新 endpoint）。完整 tenant action 則是管理員明確要求重設所指定配額的操作，兩者不可混用。

receipt 在完整 bootstrap 最後寫入；Agent 使用共用的 [agentprofile.Reader](../../m3/agentprofile/profile.go) 唯讀核對真實 receipt UID、namespace／cluster／tenant、Quota spec/status，以及 LimitRange 的 container 型別、maximum 與 request／limit defaults。minimum 由本地 bootstrap 建立，profile 回報不包含該欄位。普通 Agent 不能改 receipt、Quota、RBAC 或 Namespace。Hub Admin 向已連線 Agent 取得型別化 profile，核對後才啟用租戶與 Owner；DB 存在不代表 Worker 建立成功。

A Security Profile 的 Calico enforcement 由 [kind lifecycle](../../../scripts/kind.sh) 安裝。Policy 限制 ingress 為同 namespace Pod；B 的 kindnet 不宣稱 enforcement，egress／host／跨叢集隔離也不在本階段承諾內。實際 Go probe、Admission 與 SA 權限測試見 [kind E2E](../test/e2e/security_test.go)。
