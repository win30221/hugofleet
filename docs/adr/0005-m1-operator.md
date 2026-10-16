# ADR 0005 — M1 Operator 與驗證環境

- 狀態：已採納；驗收結果另記錄。

revision 與 ReplicaSet 的歸屬規則見 [ADR 0012](0012-current-revision-replicaset-evidence.md)。

M1 使用 Kubebuilder 4.16.0 的 Go v4 佈局與 markers，由 controller-gen 0.22.0 產生 CRD／DeepCopy。固定 controller-runtime 0.25.1、k8s.io/* 0.37.0，沿用 Go 1.27.2、kind Kubernetes 1.37.0；patch 版 0.25.1 包含 upstream race 修正。只保留本階段需要的產物，不加入 Webhook、Hub、Agent 或 UI。

Operator 為單一執行個體，只 watch bootstrap 建立的 hf-demo；ServiceAccount 使用該 namespace 的 Role，沒有 secrets、exec、namespace 建立或 cluster-admin 權限。CRD 安裝由本機 bootstrap 管理者執行。M4 再擴展受控 namespace 與租戶政策，M1 不宣稱多租戶隔離。

CRD 用結構化 schema、CEL quantity library 與預設值驗證；Controller 再以 Go 驗證，拒絕無效 image reference 與非正數／逆序資源額度。Application name 限制為最多 63 字元的 DNS label，避免長 CR 名稱無法放入 workload labels。只管理固定的 Deployment／ClusterIP Service，UID ownership 衝突不得接管。無外部資源，因此刪除依 ownerReference 與 Kubernetes GC，不加入 finalizer；Role 的 applications/finalizers update 僅供設定 blockOwnerDeletion 的 ownership permission check。

Deployment annotations 記錄 Application UID／revision，Pod template 同時綁定 revision；每個新 revision 觸發獨立 rollout，相同 revision 重送維持冪等。Ready 必須符合目標配置、Deployment observedGeneration、updated／ready／available replicas 與完整 rollout；相同配置的新 revision、純縮放及重用舊配置均須測試。observedRevision 表達 Deployment controller 已觀測的套用版本，故障版本可已 observed 但仍 Ready=false。

重新評估 ADR 0004 後，本機 M1 的拋棄式 kind 驗收沿用 RAM etcd：只測 Operator 程序重啟後重新收斂，不測 Docker node 重啟或持久儲存恢復。涉及節點重啟的持久性驗收必須另用 disk 環境；這不是正式部署 profile。envtest 啟動真實 etcd／API server，但不宣稱具备 Pod／Deployment controller／GC。

WSL 缺 C compiler 時，Race 在固定 Go container 執行 Unit 與真實 envtest。專案來源與 envtest assets 唯讀掛載，只允許寫入專案快取（預設 .cache，或 ADR 0006 的專用 Linux 快取）；network=none、4 CPUs／4 GiB 上限。API server 明確 advertise 127.0.0.1，避免在只有 loopback、沒有 default route 的容器內自動選址失敗；envtest 使用 endpoint-reconciler-type=none，因為預設 lease reconciler 拒絕 loopback，而這個環境沒有 Service dataplane。這只改測試控制平面，不修改 kind／產品部署設定，也不減少 schema、Controller 或 ownership assertions。使用已校驗的本地 module cache，避免另下載整份相依或與 kind 並行建置。cache 不完整即失敗，不能略過 Race。

官方依據：

- [Kubebuilder 4.16.0 scaffold go.mod](https://github.com/kubernetes-sigs/kubebuilder/blob/v4.16.0/testdata/project-v4/go.mod)、[Makefile](https://github.com/kubernetes-sigs/kubebuilder/blob/v4.16.0/testdata/project-v4/Makefile)。
- [controller-runtime 0.25.1 go.mod](https://github.com/kubernetes-sigs/controller-runtime/blob/v0.25.1/go.mod)、[相容性表](https://github.com/kubernetes-sigs/controller-runtime#compatibility)。
- [envtest 官方說明](https://book.kubebuilder.io/reference/envtest)、[Kubernetes CEL quantity library](https://kubernetes.io/docs/reference/using-api/cel/#kubernetes-resource-quantity-library)。
- [kube-apiserver endpoint-reconciler-type 參數](https://kubernetes.io/docs/reference/command-line-tools-reference/kube-apiserver/)、[Kubernetes 1.37 endpoint IP 驗證原始碼](https://github.com/kubernetes/kubernetes/blob/v1.37.0/pkg/controlplane/instance.go)。

## 閱讀後返回

本頁用來查設計原因，不是安裝步驟。返回 [M1 原理與進階閱讀](../../stages/m1/README.md#原理與進階閱讀)，或從[選課與回訪](../../stages/README.md#回來繼續哪一課)接續使用此共用設計的其他階段。其他設計問題見 [ADR 導覽](../README.md#設計問題與-adr-對照)。
