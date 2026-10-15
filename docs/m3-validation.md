# M3 驗收範圍

M3 驗證 UI／REST → SQLite Operation → mTLS gRPC 雙向通道 → Worker Agent → Application Operator → 真實雙叢集工作負載。操作見 [M3 教學](../stages/m3/README.md)，通道設計見 [ADR 0008](adr/0008-m3-outbound-agent.md)，實測與 CI 證據見 [驗證總覽](validation.md)。

## 驗收命令

在專案根目錄的 Ubuntu WSL2／Linux Bash，以非 root 使用者執行：

```bash
make tools operator-tools envtest proto-tools
make m3-build m3-test
make proto-check verify-generated test-integration
make test-e2e-m3
```

## 已確認的行為

- 兩個真實 Worker Pod 主動向 Hub 建立 mTLS gRPC 通道；Hub mounts 不含 Worker kubeconfig。
- A／B 分別部署 v1、得到真實 quote=8000；A Agent 停止後顯示 UNKNOWN 並保留觀測時間，當地 Operator 仍修復漂移，B 可獨立發版。
- Agent 重連時相符 CR 不重複 Apply，CR UID／generation 保留；Hub 重啟保留 command_id／deadline，再以 snapshot 核對。
- ACK 遺失、Agent restart、持久 ledger、舊／衝突 revision、過期刪除、UID precondition、GC 與 tombstone replay。
- broken 真正 FAILED，回滾建立新 revision 並驗證 HTTP。Pending／Unschedulable Pod 在原 deadline 後 TIMED_OUT，恢復排程後 LATE_READY 不改寫失敗歷史。
- 真實 TCP／TLS／gRPC 負向測試拒絕不可信 CA、錯誤 SAN、過期 client、未登記 identity、Hello 不符、duplicate session；長命令期間持續心跳並處理到期與重連。
- Worker ServiceAccount 僅具必要權限；命令不接受任意 shell、YAML 或 GVK。

本機及對應 Hosted CI 的正式 M3 E2E 均有 PASS 證據，包含 owned Hub／雙 Workers 清理與私有 state 保留。原始日誌在 `artifacts/m3-e2e/<run>`，整合日誌為 `artifacts/release-validation/m3-m5.log`。

## 限制

M3 使用 loopback Demo Token／session，正式 OIDC 與租戶隔離由 M4 提供。leaf 24 小時、CA 48 小時；到期以新 state 練習，保留既有 DB／Workers 的原地輪替未驗收。ledger 512 KiB 上限與 tombstone 不自動清理；ConfigMap 持久化不等於 RAM etcd 的 node／主機重啟保證。

程序重啟、通道重連、真實雙叢集驗收與單元測試 backend fixture 的範圍分開；不以 fixture 取代 Kubernetes E2E。
