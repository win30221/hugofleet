# M1 驗收範圍

M1 驗證 Application CRD → Operator Reconcile → Deployment／Service、狀態收斂與 Kubernetes GC。操作步驟見 [M1 教學](../stages/m1/README.md)，實測與 CI 證據見 [驗證總覽](validation.md)。

## 驗收命令

在專案根目錄的 Ubuntu WSL2／Linux Bash 執行：

```bash
make tools operator-tools envtest
make m1-build m1-test
make verify-generated test-integration
make demo-image-build operator-image-build
make test-e2e-m1
```

## 已確認的行為

| 層級 | 驗收內容 |
|---|---|
| Unit／envtest | schema／CEL／defaults／status、完整 workload projection、更新與 no-op、foreign ownership 拒絕、Deployment／Service resourceVersion conflict 重試 |
| kind lifecycle | 真實 CR／Ready Pods／HTTP、image／replicas 更新、Deployment／Service 漂移修復與刪除重建、Operator 程序重啟後收斂 |
| revision | 每個新 revision 觸發獨立 rollout；相同 revision 重送保持 ReplicaSet／Deployment generation 不變 |
| 故障與恢復 | 真正壞版 ProgressDeadlineExceeded；舊 ReplicaSet 的失敗不歸入新 revision；新 revision 恢復 v1 後 HTTP 通過 |
| 刪除 | ownerReference 與實際 Kubernetes GC 移除工作負載 |
| 權限 | 允許必要 namespaced 工作負載與 ReplicaSet 讀取；拒絕 Secret／exec／Namespace、ReplicaSet 寫入及跨 namespace 存取 |

本機及對應 Hosted CI 的正式入口均有 PASS 證據，包含 owned cleanup。原始日誌在 `artifacts/m1-e2e/<run>`；衝突重試與正式回歸的整合日誌為 `artifacts/release-validation/ci-fixes-local.log`、`ci-fixes-e2e.log`。

## 限制

envtest 沒有 kubelet／Deployment controller／GC，物件存在不能當作 Pod Ready。RAM etcd 的驗收只涵蓋 Operator 程序重啟，不涵蓋 Docker node 或持久叢集恢復。每個新 revision 都需預留 maxSurge=1 的暫時配額空間。設計見 [ADR 0005](adr/0005-m1-operator.md) 與 [ADR 0012](adr/0012-current-revision-replicaset-evidence.md)。
