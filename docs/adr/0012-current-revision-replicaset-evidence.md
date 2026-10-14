# ADR 0012：以目前 revision 的 ReplicaSet 歸屬部署失敗

日期：2026-10-14。狀態：已採納，最新 Unit／race／envtest 及 M1～M5 真實正式驗收通過，結果見 [M5 紀錄](../m5-validation.md)。Hosted CI 未執行。

## 問題與證據

M5 全新正式驗收在記憶體 quota 解除後發布 revision 2，卻立即得到 revision 1 的 FailedCreate。當時 Quota 已釋放；Deployment 的 observedGeneration 已推進，但 ReplicaFailure 仍帶著舊 ReplicaSet 的錯誤。整次驗收為 FAIL，原始紀錄保留於 artifacts/m5-e2e/20261013T215643-43083。

固定 Kubernetes 1.37 的 [Deployment controller](https://github.com/kubernetes/kubernetes/blob/v1.37.0/pkg/controller/deployment/progress.go) 會在新 ReplicaSet 沒有 failure 時，從舊 ReplicaSets 取得 failure；因此不能僅以 Deployment observedGeneration 判斷 ReplicaFailure 屬於新意圖。[目前 ReplicaSet 的匹配](https://github.com/kubernetes/kubernetes/blob/v1.37.0/pkg/controller/deployment/util/deployment_util.go) 以 Pod template 語意相等為準，排除 pod-template-hash。

## 決策

Application revision 同時寫入受管理的 Pod template annotation。新 revision 具有獨立 rollout，即使 image／resources 相同；同 revision 重送不改 template，保留冪等。使用者其他 template metadata 保留，Hub 的獨立 Ready 核對也必須驗證受管理的 revision marker。

Operator 遇到 Deployment ReplicaFailure 時，只讀同 namespace、Application UID label 對應的 ReplicaSets，再核對 Deployment controller owner UID、目前 template 與 ReplicaSet observedGeneration。只有目前 template 的真實 ReplicaSet failure 能成為這個 revision 的 Degraded；舊／foreign／未觀測的 ReplicaSet 不能讓新操作終結為 FAILED，也不能製造 Ready。既有完整副本與 Deployment generation 的 Ready 判斷保留，Hub 的 FAILED 履歷仍不可改寫。

為取得這項證據，Operator 的 namespaced Role 增加 replicasets get/list/watch，供受限 controller cache 使用；不增加任何 ReplicaSet 寫入、Pod exec、Secret、跨 namespace 或 cluster-wide 權限。M1 生成 Role、M4 bootstrap 與 M5 Helm 同步；真實 SubjectAccessReview 增加允許讀取及拒絕寫入／跨租戶的 assertions。

後續真實驗收又發現 Progressing=False／ProgressDeadlineExceeded 也會殘留到新回滾。Kubernetes 的 SetDeploymentCondition 在 status／reason 相同時保留舊 condition；Deployment observedGeneration 推進仍不足以歸屬逾時。目前版本必須有相符且已觀測的 owned ReplicaSet，逾時 condition 的 LastUpdateTime 也必須晚於該 ReplicaSet 建立時間加上設定的 progressDeadlineSeconds，才能判為 Degraded。這是額外的歸屬證據，不自行建立逾時 condition、不解析人類訊息，也不改寫 Deployment status。

只有舊／缺少／未觀測／無法歸屬的逾時證據時，Operator 繼續 Progressing；Hub 仍依原 operation deadline 終結為 TIMED_OUT，不延長期限或改寫既有 FAILED 履歷。若 Kubernetes 一直不更新舊 condition，不能把它當作目前版本的進度逾時證據。真正目前版本的 ProgressDeadlineExceeded 與 ReplicaFailure 仍保留。

## 驗證與限制

先以 Unit fixture 重現舊錯誤、新錯誤、foreign owner、未觀測 ReplicaSet 與 revision marker 缺失，修正前全部 FAIL；這些 fixture 不替代 kind。保留真實 quota 解除後新 revision 必須 SUCCEEDED 的原 assertion，重新執行 Unit／race／envtest、正式 M5 與受影響的 M1～M4。結果見 [M5 紀錄](../m5-validation.md)。新增 revision 會發生 rolling update，需要原本 maxSurge=1 的暫時配額空間。

第二組六項 Unit 涵蓋舊逾時、僅一秒的過早逾時、真實目前逾時、缺少／foreign／未觀測 ReplicaSet；修正前五項 FAIL，真正目前逾時的正向案例 PASS。修正後全部 PASS，既有壞版必須回報 ProgressDeadlineExceeded 的 assertion 保留，補齊實際 API 會提供的 UID／ReplicaSet／時間 fixture。真實回滾必須 SUCCEEDED 的原 assertion 也保留，待全新正式重驗。

M1 kind 測試依新契約核對相同配置與純縮放的新 revision：新的 ReplicaSet UID、Deployment owner UID、template revision、observedGeneration 與完整 Ready／Available 副本都須成立。原 ReplicaSet 不變檢查保留於同 revision 重送，另確認 Deployment generation 不變。M2 配額恢復與 FAILED 履歷保留的 assertions 不變；更新原先「新 revision 重用 ReplicaSet」的舊註解。
