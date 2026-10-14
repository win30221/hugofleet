# ADR 0004 — M0 拋棄式 kind 的 etcd 使用 RAM

- 日期：2026-10-10
- 狀態：已採納；2026-10-11 本機完整 E2E 通過；僅限 M0 本機開發／E2E

## 背景

本機 Docker／WSL I/O full pressure 約 40%，etcd WAL fdatasync 超過一秒。多次獨立 kind bootstrap／rollout 已失敗：RBAC 初始化緩慢、controller-manager leader lease 逾時、kubelet 無法同步 CoreDNS ConfigMap cache。Node RAM 約 465–731 MiB，沒有 OOM；單獨執行 E2E 仍遇到延遲。

## 決策

M0 的預設單節點設定以 kubeadm `ClusterConfiguration.etcd.local.dataDir` 指向 `/run/hugofleet-etcd`。kind 0.33.0 本身將 node 的 `/run` 掛成 tmpfs；以 RAM 儲存這個拋棄式開發叢集的真實 etcd 資料，保留標準 etcd fsync，不變更應用或 Kubernetes 驗收條件。

`HF_ETCD_STORAGE=disk make kind-up` 可於新叢集選擇 `/var/lib/etcd`，不遷移任何既有資料。`HF_ETCD_STORAGE` 僅接受 memory／disk。只作用於本專案新建節點，不修改 Docker Desktop、WSL、其他叢集或容器。

## 限制與後續

- RAM profile 在 Docker node 停止／重啟後會失去 etcd 資料，必須 `make kind-down` 再 `make kind-up` 重建；不能當作持久叢集或正式平台。
- 除程序 RAM 外會額外使用 tmpfs 空間；驗收後記錄實際 Docker memory，不保證固定用量。
- 本階段沒有 Hub SQLite 或持久業務資料。M1 起須重新評估 profile；涉及持久性、重啟恢復的驗收必須使用 disk profile 或適當的持久環境。
- `disk` profile 已建立 Node Ready，但尚未完成本機完整 E2E；不能宣稱持久叢集驗收通過。
- 第一次 RAM 試跑確認 tmpfs 與 manifest data-dir 正確，但 CRI CreateContainer 超過四分鐘，API bootstrap 先於控制平面啟動到期。2026-10-11 將 kubeadm API 等待改為 10 分鐘，總 kind timeout 仍為 20 分鐘；不調整應用 assertions。
- 2026-10-12 因最新 M3 的 B 冷啟動再次超過等待，API／runtime／kind 等待更新為 15／10／30 分鐘；原 FAIL 與重驗另記於 [M4 回歸紀錄](../m4-validation.md)。沒有改變 etcd 或 containerd fsync。
- 最終驗收的系統 Pods 全部 Ready／restartCount=0；正常更新、故障版／上一版可用性、恢復 v1 與清理均通過，節點快照約 761.7 MiB。完整證據見 [M0 實測紀錄](../m0-validation.md)。

## 官方依據

- [kubeadm v1beta4 LocalEtcd dataDir](https://kubernetes.io/docs/reference/config-api/kubeadm-config.v1beta4/#LocalEtcd)：支援指定本地 etcd 資料目錄。
- [kind kubeadm config patches](https://kind.sigs.k8s.io/docs/user/configuration/#kubeadm-config-patches)。
- [kind 0.33.0 Docker provision source](https://github.com/kubernetes-sigs/kind/blob/v0.33.0/pkg/cluster/internal/providers/docker/provision.go#L231)：`/run` 使用 tmpfs；本機 source checksum 已驗證，node inspect 亦確認此掛載。
