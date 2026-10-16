# ADR 0004 — M0 拋棄式 kind 的 etcd 使用 RAM

- 狀態：已採納；僅限拋棄式本機開發／E2E。

## 背景

Docker／WSL 的磁碟 flush 延遲可能拖慢 etcd WAL、RBAC bootstrap、leader lease 與 ConfigMap cache 同步。拋棄式本機叢集需要降低 I/O 成本，同時保留真實 Kubernetes 與標準 fsync 行為。

## 決策

M0 的預設單節點設定以 kubeadm `ClusterConfiguration.etcd.local.dataDir` 指向 `/run/hugofleet-etcd`。kind 0.33.0 本身將 node 的 `/run` 掛成 tmpfs；以 RAM 儲存這個拋棄式開發叢集的真實 etcd 資料，保留標準 etcd fsync，不變更應用或 Kubernetes 驗收條件。

`HF_ETCD_STORAGE=disk make kind-up` 可於新叢集選擇 `/var/lib/etcd`，不遷移任何既有資料。`HF_ETCD_STORAGE` 僅接受 memory／disk。只作用於本專案新建節點，不修改 Docker Desktop、WSL、其他叢集或容器。

## 限制

- RAM profile 在 Docker node 停止／重啟後會失去 etcd 資料，必須 `make kind-down` 再 `make kind-up` 重建；不能當作持久叢集或正式平台。
- 除程序 RAM 外會額外使用 tmpfs 空間；驗收後記錄實際 Docker memory，不保證固定用量。
- M0 沒有 Hub SQLite。其他階段沿用 RAM etcd 時，涉及持久性、重啟恢復的驗收必須使用 disk profile 或適當的持久環境。
- `disk` profile 已建立 Node Ready，但尚未完成本機完整 E2E；不能宣稱持久叢集驗收通過。
- kubeadm API／kubelet runtime／kind 等待上限為 15／10／30 分鐘；不改變應用 readiness、etcd 或 containerd fsync。
- 三版工作負載、壞版 readiness／上一版可用性、恢復 v1 與清理的測試範圍見 [M0 驗收](../m0-validation.md)。

## 官方依據

- [kubeadm v1beta4 LocalEtcd dataDir](https://kubernetes.io/docs/reference/config-api/kubeadm-config.v1beta4/#LocalEtcd)：支援指定本地 etcd 資料目錄。
- [kind kubeadm config patches](https://kind.sigs.k8s.io/docs/user/configuration/#kubeadm-config-patches)。
- [kind 0.33.0 Docker provision source](https://github.com/kubernetes-sigs/kind/blob/v0.33.0/pkg/cluster/internal/providers/docker/provision.go#L231)：`/run` 使用 tmpfs；本機 source checksum 已驗證，node inspect 亦確認此掛載。

## 閱讀後返回

本頁用來查設計原因，不是安裝步驟。返回 [M0 原理與進階閱讀](../../stages/m0/README.md#原理與進階閱讀)，或從[選課與回訪](../../stages/README.md#回來繼續哪一課)接續使用此共用設計的其他階段。其他設計問題見 [ADR 導覽](../README.md#設計問題與-adr-對照)。
