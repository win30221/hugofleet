# ADR 0001 — M0 使用 WSL2/Linux 與獨立工具、叢集

- 狀態：已採納
- 範圍：M0；不實作 M1–M5。

## 背景

開發主機為 Windows，已有 Docker Desktop、Ubuntu WSL2 和其他 kind／Docker 資源。
命令需要明確的 Shell 契約，M0 的 Sample App 垂直切片不需要 Kubebuilder scaffold。

## 決策

1. 主要命令在 Linux amd64／Ubuntu WSL2 執行；所有入口為 `make` → Bash。
2. `make tools` 把固定版本 Go、kind、kubectl 安裝到 `.tools`，下載校驗 SHA-256，不修改全域安裝。
3. M0 僅包含 pricing-api、原生 Deployment／Service、單節點 kind、測試、基本 CI；Kubebuilder 與 envtest 在 M1 引入。
4. 日常叢集使用 `hugofleet-a`，E2E 使用 `hugofleet-m0-e2e`。kubeconfig 限於 `.local`，不修改使用者預設 context。
5. 建立叢集後記錄 control-plane 容器完整 ID；所有修改／刪除都核對固定叢集名及 ID。拒絕接管同名既有叢集，不執行 Docker prune 或通用清理。
6. Sample Image 使用純 Go 靜態 binary + scratch、非 root、read-only filesystem、無 ServiceAccount token。
7. M0 E2E 的恢復僅為真實 Kubernetes image update，不宣稱已有 HugoFleet Release／Rollback。
8. GitHub release binary 過慢時，可用官方文件列出的固定版本 `go install sigs.k8s.io/kind@v0.33.0`，由 Go checksum database 校驗來源，安裝仍限制在 `.tools/bin`。WSL 缺 C compiler 時，Race test 使用相同固定版本的 Go build container，不修改主機編譯器。
9. API port 使用 loopback 16443／16444，可在建立叢集時覆寫，避免 Windows 保留的高位 port；不修改保留範圍或防火牆。
10. 冷啟動需解壓映像及建立控制平面 sandbox。kubeadm API bootstrap 等待 15 分鐘、kubelet runtime 等待 10 分鐘、控制平面健康等待 8 分鐘，外層 kind 最多 30 分鐘；仍要求 Node、CoreDNS 與應用驗收通過。失敗時保存節點診斷與完整容器 ID，供限定清理。
11. 建立後等待 CoreDNS Deployment rollout 完成，再接受 Sample App 操作；Node Ready 本身不足以確認 Deployment controller 已能持續處理狀態。重型建置／race 與本機 kind E2E 依序執行，以降低磁碟競爭。
12. M0 預設使用 [ADR 0004](0004-m0-ephemeral-etcd.md) 的拋棄式 RAM etcd，保留 etcd／containerd fsync。持久叢集、node／主機重啟與備份恢復需獨立驗收。

## 版本與官方依據

- Go 1.27.2：[官方下載／SHA-256](https://go.dev/dl/?mode=json)。
- kind 0.33.0 + Kubernetes 1.37.0：使用 [kind release 官方預設 node image digest](https://github.com/kubernetes-sigs/kind/releases/tag/v0.33.0)，固定 digest，避免使用其他 minor。
- kubectl 1.37.1：與 API server 相同 minor，遵循 [version skew policy](https://kubernetes.io/releases/version-skew-policy/)；可將已安裝且符合官方 SHA-256 的 binary 複製至專案內，仍核對完整校驗碼。
- Go build stage：Docker Official Image `golang:1.27.2-bookworm`，固定 Linux amd64 manifest digest；不使用浮動 latest。
- Bootstrap timeout：[kubeadm v1beta4 Timeouts](https://kubernetes.io/docs/reference/config-api/kubeadm-config.v1beta4/)，透過 [kind kubeadm config patches](https://kind.sigs.k8s.io/docs/user/configuration/#kubeadm-config-patches) 套用。
- Runtime timeout：[KubeletConfiguration runtimeRequestTimeout](https://kubernetes.io/docs/reference/config-api/kubelet-config.v1beta1/)。
- M1 必須重新確認 [Kubebuilder 的工具相容性／支援平台](https://book.kubebuilder.io/versions_compatibility_supportability.html)，不能從 M0 版本推論所有 Controller library 都相容。

## 取捨

- M0 安裝腳本只支援 Linux amd64；Windows native、arm64 尚未驗收。
- 專案位於 Windows 掛載可直接共用檔案，但首次 Go build／工具解壓可能比 WSL Linux filesystem 慢。
- 本機 Docker 是既有安裝，不在 M0 自動升級；可用性與相容性以實際 kind 測試記錄為準。
- 下載工具和 node image 需要網路；固定來源與校驗碼讓失敗可定位，不能以 Mock 取代部署。
