# ADR 0012：以目前 revision 的 ReplicaSet 歸屬部署失敗

狀態：已採納。測試範圍見 [M1 驗收](../m1-validation.md) 與 [M5 驗收](../m5-validation.md)，程式提交與 CI 證據見 [驗證總覽](../validation.md)。

## 問題與證據

記憶體 quota 解除後發布新 revision 時，即使 Deployment observedGeneration 已推進，ReplicaFailure 仍可能帶著舊 ReplicaSet 的 FailedCreate。若直接採用這個狀態，新操作會被舊版本錯誤終結為 FAILED。

固定 Kubernetes 1.37 的 [Deployment controller](https://github.com/kubernetes/kubernetes/blob/v1.37.0/pkg/controller/deployment/progress.go) 會在新 ReplicaSet 沒有 failure 時，從舊 ReplicaSets 取得 failure；因此不能僅以 Deployment observedGeneration 判斷 ReplicaFailure 屬於新意圖。[目前 ReplicaSet 的匹配](https://github.com/kubernetes/kubernetes/blob/v1.37.0/pkg/controller/deployment/util/deployment_util.go) 以 Pod template 語意相等為準，排除 pod-template-hash。

## 決策

Application revision 同時寫入受管理的 Pod template annotation。新 revision 具有獨立 rollout，即使 image／resources 相同；同 revision 重送不改 template，保留冪等。使用者其他 template metadata 保留，Hub 的獨立 Ready 核對也必須驗證受管理的 revision marker。

Operator 遇到 Deployment ReplicaFailure 時，只讀同 namespace、Application UID label 對應的 ReplicaSets，再核對 Deployment controller owner UID、目前 template 與 ReplicaSet observedGeneration。只有目前 template 的真實 ReplicaSet failure 能成為這個 revision 的 Degraded；舊／foreign／未觀測的 ReplicaSet 不能讓新操作終結為 FAILED，也不能製造 Ready。既有完整副本與 Deployment generation 的 Ready 判斷保留，Hub 的 FAILED 履歷仍不可改寫。

為取得這項證據，Operator 的 namespaced Role 增加 replicasets get/list/watch，供受限 controller cache 使用；不增加任何 ReplicaSet 寫入、Pod exec、Secret、跨 namespace 或 cluster-wide 權限。M1 生成 Role、M4 bootstrap 與 M5 Helm 同步；真實 SubjectAccessReview 增加允許讀取及拒絕寫入／跨租戶的 assertions。

Progressing=False／ProgressDeadlineExceeded 也可能殘留到新回滾。Kubernetes 的 SetDeploymentCondition 在 status／reason 相同時保留舊 condition；Deployment observedGeneration 推進仍不足以歸屬逾時。目前版本必須有相符且已觀測的 owned ReplicaSet，逾時 condition 的 LastUpdateTime 也必須晚於該 ReplicaSet 建立時間加上設定的 progressDeadlineSeconds，才能判為 Degraded。這是額外的歸屬證據，不自行建立逾時 condition、不解析人類訊息，也不改寫 Deployment status。

只有舊／缺少／未觀測／無法歸屬的逾時證據時，Operator 繼續 Progressing；Hub 仍依原 operation deadline 終結為 TIMED_OUT，不延長期限或改寫既有 FAILED 履歷。若 Kubernetes 一直不更新舊 condition，不能把它當作目前版本的進度逾時證據。真正目前版本的 ProgressDeadlineExceeded 與 ReplicaFailure 仍保留。

## 驗證與限制

Unit 涵蓋舊／目前 ReplicaFailure、foreign owner、未觀測 ReplicaSet 與 revision marker 缺失。kind 驗證 quota 解除後新 revision 必須 SUCCEEDED，並覆蓋真正壞版與回滾；Unit fixture 不取代實際叢集。新 revision 會 rolling update，需要 maxSurge=1 的暫時配額空間。

逾時 Unit 涵蓋舊逾時、過早逾時、真正目前逾時及缺少／foreign／未觀測 ReplicaSet。真實壞版必須回報 ProgressDeadlineExceeded，回滾必須 SUCCEEDED；兩者與 Unit／envtest／race 均有對應驗收證據。

M1 kind 測試核對相同配置與純縮放的新 revision：新的 ReplicaSet UID、Deployment owner UID、template revision、observedGeneration 與完整 Ready／Available 副本都須成立。同 revision 重送須保持 ReplicaSet 與 Deployment generation 不變。M2 則驗證配額恢復及 FAILED 履歷保留。
