# Controller：從 Application 到工作負載

完整操作從 [M1 教學](../README.md) 開始。這個套件執行持續的 Reconcile；M2／M3 使用同一 Operator，改變的是誰提交 Application，不是重新做另一套 Controller。

## 檔案與閱讀順序

| 順序 | 檔案 | 看什麼 |
|---|---|---|
| 1 | [Application 型別](../api/v1alpha1/application_types.go)／[validation.go](../api/v1alpha1/validation.go) | 期望配置、status 與允許欄位 |
| 2 | [Operator 入口](../cmd/operator/main.go) | Scheme、Manager、namespace 與 Controller 註冊 |
| 3 | [application_controller.go](application_controller.go) | Reconcile、ownership、rolloutStatus、status patch、watch |
| 4 | [workload.go](workload.go) | 期望 Deployment／Service 的生成與 metadata 保留 |
| 5 | [application_controller_test.go](application_controller_test.go) | 以測試例子理解狀態／漂移規則 |
| 6 | [envtest_test.go](envtest_test.go)／[kind E2E](../test/e2e/) | API server 與真實 Pod／GC 的不同驗證範圍 |

## 跟一次更新

當你把 CR 改成 v2-good、replicas=2、新 revision：

1. API server 先以 CRD schema／CEL 檢查。合法配置才寫進 Application，spec 變更推進 metadata.generation。
2. SetupWithManager 的 `For(Application)` 將事件排入此物件的 Reconcile；`Owns(Deployment／Service)` 也監看相依物件。
3. Reconcile 在最多 30 秒的 context 中讀 CR，檢查名稱與 Go Spec.Validate。
4. 先讀同名 Deployment 與 Service、核對 controller owner UID，避免改了其中一個才發現另一個不是自己的。
5. CreateOrUpdate 再核對 ownership，mutateDeployment／mutateService 將 CR 投影成期望配置，SetControllerReference 設 ownerReferences。
6. rolloutStatus 讀 Deployment 的實際狀態，patchStatus 回報 CR；之後事件或 10 秒重新排程再核對。

Reconcile 返回不表示 Pod 已 Ready。它提交物件後，Kubernetes 的 Deployment controller／scheduler／kubelet 仍要建立 Pod、排程、執行與探針。

```text
Application spec
   ↓ Reconcile 的 CreateOrUpdate
Deployment spec / Service spec
   ↓ Kubernetes 內建控制器與 kubelet
Deployment status / Pod readiness
   ↓ rolloutStatus / patchStatus
Application status
```

## 漂移如何觸發修復

CR 要 2 個副本，你只把 Deployment spec.replicas 改成 1。owned Deployment 的事件觸發 Reconcile，mutateDeployment 依原 CR 重新產生 replicas=2；Kubernetes 再收斂副本。

如果改的是 Application spec.replicas=1，期望配置本身已變，Operator 會維持 1。它不會查 Hub SQLite 幫你把 CR 改回去。M2／M3 因此要從 Hub 發版，避免 CR 與 Hub 的快照不一致。

同值 Reconcile 不需要反覆寫入。CreateOrUpdate 比對資源；patchStatus 以語意相等檢查跳過相同 status，改動時使用樂觀鎖，避免無意覆蓋同時更新。

## Ready 判斷比「有 Pod」嚴格

rolloutStatus 先比對完整受管理的 Deployment spec、UID label 與 revision annotation。revision 也寫入 Pod template；每個新 revision 有自己的 rollout，相同 revision 重送則不重建 template。Deployment controller 必須已觀測目前 dep.Generation，才推進 observedRevision／image。

Ready 還要求 updatedReplicas、replicas、readyReplicas、availableReplicas 都等於 CR 期望副本，unavailableReplicas=0。只有舊版副本可用、或新 Pod Running 卻未 Ready，都不等於目標配置成功。

| 狀況 | 主要 Condition |
|---|---|
| 等控制器觀測或新副本就緒 | Ready=False、Progressing=True |
| 完整目標 rollout 可用 | Ready=True、Progressing=False、Degraded=False |
| ProgressDeadlineExceeded／目前 ReplicaSet 的 ReplicaFailure | Ready=False、Degraded=True，保留具體 reason／message |
| ownership／規格問題 | problem 回報 Degraded，不能假裝正常接管 |

Condition 同時記錄 observedGeneration，讀狀態時要對照目前 CR generation。status.observedRevision 表示配置被觀測，不是成功版本履歷；M2 才另存 last_successful_revision。

Deployment 可能保留舊 ReplicaSet 的 ReplicaFailure。Operator 以同 namespace、UID label、Deployment owner UID、目前 Pod template（排除 pod-template-hash）與 ReplicaSet observedGeneration 核對錯誤歸屬，不能把舊 quota failure 判成新 release 失敗。Role 僅增加 namespaced ReplicaSet get/list/watch，不寫入 ReplicaSet；變更理由與失敗證據見 [ADR 0012](../../../docs/adr/0012-current-revision-replicaset-evidence.md)。沒有目前失敗證據時繼續等待，Ready 的完整副本條件不放寬。

## ownership、保留與刪除

checkOwner 要求既有資源的 controller owner 與此 Application 的 UID、GroupVersion、Kind、Name 一致。相同名稱不夠，UID 才能辨認刪除重建後的不同物件。

workload.go 控制整個工作負載 spec，同時保留非本元件的 metadata、Pod template 額外 labels／annotations；更新 Service 時保留已配置的 ClusterIP／IP family。刪除再建立 Service 則可能換 IP。

本實作使用 ownerReferences 和 Kubernetes GC 清理相依物件，沒有另做自訂 finalizer 刪除程序。CR 不存在時 Reconcile 忽略 NotFound；在刪除中的 CR 不再建立新工作負載。GC 完成需要真實 Kubernetes 控制器，envtest 不能代替它。

## 想修改與驗證時

改資源形狀看 workload.go；改更新／ownership／status 看 application_controller.go；改允許配置看 API 型別與 validation。DesiredDeploymentSpec 也供 Hub 的 transport 獨立核對 rollout，修改時需一起考慮。

CRD／RBAC 由 markers 生成，命令見 [M1 程式導讀](../README.md#本階段的程式導讀)。Operator 不是以任意 YAML／shell 管理工作負載；維持型別化 Application 邊界。

ProgressDeadlineExceeded 也須歸屬目前 owned／observed ReplicaSet：condition LastUpdateTime 不得早於 ReplicaSet CreationTimestamp 加上設定的 progressDeadlineSeconds。舊逾時不能立即讓新回滾失敗；無法歸屬時維持 Progressing，Hub 仍受原 operation deadline 限制。Operator 不自行改寫 Deployment status 或製造 Ready；完整決策見 [ADR 0012](../../../docs/adr/0012-current-revision-replicaset-evidence.md)。

Unit 可用可控物件測比對與錯誤，envtest 用真實 API server 測 schema／status／ownership，kind 才驗證 Pod、GC、漂移與真實 HTTP。現有測試保留，本輪結果另存於 [搬移後重新驗證](../../../docs/revalidation-20261011.md)；歷史範圍看 [M1 紀錄](../../../docs/m1-validation.md)。
