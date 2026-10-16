# M1 驗收範圍

M1 驗證 Application CRD → Operator Reconcile → Deployment／Service、狀態收斂與 Kubernetes GC。操作步驟見 [M1 教學](../stages/m1/README.md)，實測與 CI 證據見 [驗證總覽](validation.md)。

## 如何使用本頁

第一次操作請先讀本階段教學；本頁用來選擇自動驗證與核對既有證據。只修改文件時執行 `make docs-check`，毋須啟動本頁整套測試。

| 命令 | 目的 |
|---|---|
| `make m1-build m1-test` | 編譯與 M1 單元測試 |
| `make verify-generated test-integration` | 核對生成檔與真實 envtest |
| `make demo-image-build operator-image-build` | 準備價格 API／Operator 映像 |
| `make test-e2e-m1` | 真實漂移／故障／GC／權限與清理 |

下方依序列出本階段需要的工具與驗收命令；已安裝的固定工具可沿用，不需準備其他階段的工具。一次執行一組 E2E，先依[停止流程](../stages/m1/README.md#停止與下次繼續)結束會占用相同連接埠的教學環境。

## 驗收命令

在專案根目錄的 Ubuntu WSL2／Linux Bash 執行：

```bash
make tools operator-tools envtest
make m1-build m1-test
make verify-generated test-integration
make demo-image-build operator-image-build
make test-e2e-m1
```

## 如何判斷本次執行結果

等待整個命令結束，核對測試與 owned cleanup 均成功且最終退出碼為 0。中途出現 PASS 不表示整體完成；失敗先保留輸出與 state，依[本階段排查](../stages/m1/README.md#操作遇到問題時)定位問題。下列已確認行為來自既有驗收，對應證據與程式提交見驗證總覽，不會自動套用到新修改。

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

## 查完後返回

本頁的測試不是初次學習的必經步驟。返回 [M1 整課驗證](../stages/m1/README.md#整課驗證與常見問題)核對手動操作；要停止或進下一課，前往 [M1 完成後的選擇](../stages/m1/README.md#完成這一課後去哪裡)。修改其他元件時，依[驗證總覽](validation.md#執行驗證)選擇測試。
