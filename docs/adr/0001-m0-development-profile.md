# ADR 0001 — M0 使用 WSL2/Linux 與獨立工具、叢集

- 日期：2026-10-10
- 狀態：已採納（依使用者接受規格審查建議）
- 範圍：M0；不實作 M1–M5。

## 背景

開發主機為 Windows，已有 Docker Desktop、Ubuntu WSL2 和其他 kind／Docker 資源。
原規格沒有明確的 Shell 契約，M0 的 Kubebuilder scaffold 也與 Sample App 垂直切片無直接關係。

## 決策

1. 主要命令在 Linux amd64／Ubuntu WSL2 執行；所有入口為 `make` → Bash。
2. `make tools` 把固定版本 Go、kind、kubectl 安裝到 `.tools`，下載校驗 SHA-256，不修改全域安裝。
3. M0 僅包含 pricing-api、原生 Deployment／Service、單節點 kind、測試、基本 CI；Kubebuilder 與 envtest 在 M1 引入。
4. 日常叢集使用 `hugofleet-a`，E2E 使用 `hugofleet-m0-e2e`。kubeconfig 限於 `.local`，不修改使用者預設 context。
5. 建立叢集後記錄 control-plane 容器完整 ID；所有修改／刪除都核對固定叢集名及 ID。拒絕接管同名既有叢集，不執行 Docker prune 或通用清理。
6. Sample Image 使用純 Go 靜態 binary + scratch、非 root、read-only filesystem、無 ServiceAccount token。
7. M0 E2E 的恢復僅為真實 Kubernetes image update，不宣稱已有 HugoFleet Release／Rollback。
8. GitHub release binary 過慢時，可用官方文件列出的固定版本 `go install sigs.k8s.io/kind@v0.33.0`，由 Go checksum database 校驗來源，安裝仍限制在 `.tools/bin`。WSL 缺 C compiler 時，Race test 使用相同固定版本的 Go build container，不修改主機編譯器。
9. Windows 保留的高位 TCP port 曾使隨機 API port 43817 無法 bind。改用仍限於 loopback 的 16443／16444，可在建立叢集時覆寫；不移除 Windows 保留範圍，不改防火牆。
10. 本機 Docker 建立控制平面 sandbox 需數十秒，kubeadm 預設 1 分鐘 API bootstrap timeout 曾先到期；CRI 解壓映像也曾超過 kubelet 預設 2 分鐘等待。依官方欄位設定 5 分鐘 API／runtime、8 分鐘控制平面健康等待，外層 kind 最多 20 分鐘。健康等待 5 分鐘曾先結束，retained node 隨後才通過全部 API readiness；量到 I/O pressure full 約 40%，CPU 未 throttled。仍要求 Node Ready 與所有應用驗收通過。失敗 node 保留並記錄 ID 供診斷及限定清理。
11. 建立後等待 CoreDNS Deployment rollout 完成，再接受 Sample App 操作；Node Ready 本身不足以確認 Deployment controller 已能持續處理狀態。重型建置／race 與本機 kind E2E 依序執行，以降低磁碟競爭。
12. 2026-10-11 後，M0 預設使用 ADR 0004 的拋棄式 RAM etcd；API bootstrap 等待更新為 10 分鐘、runtime 5 分鐘、控制平面健康 8 分鐘、總 kind 建立 20 分鐘。這些取代本 ADR 早期 disk bootstrap 的設定；持久叢集驗收不在 M0 通過結果內。
13. 2026-10-12 的最新 M3 回歸中，B 的 CRI CreateContainer 未在 API 等待期間完成，10 分鐘後 API Server 仍拒絕連線；未開始功能 E2E。保留診斷並完成 owned cleanup 後，將 API／runtime 等待調為 15／10 分鐘，外層 kind 30 分鐘；控制平面 health 仍為 8 分鐘。未關閉 containerd fsync、未更動 Docker daemon 或其它叢集；結果另記於 [M4 回歸紀錄](../m4-validation.md)。

## 版本與官方依據

- Go 1.27.2：[官方下載／SHA-256](https://go.dev/dl/?mode=json)。
- kind 0.33.0 + Kubernetes 1.37.0：使用 [kind release 官方預設 node image digest](https://github.com/kubernetes-sigs/kind/releases/tag/v0.33.0)，本機已快取相同 digest，避免額外下載其他 minor。
- kubectl 1.37.1：與 API server 相同 minor，遵循 [version skew policy](https://kubernetes.io/releases/version-skew-policy/)；可將已安裝且符合官方 SHA-256 的 binary 複製至專案內，仍核對完整校驗碼。
- Go build stage：Docker Official Image `golang:1.27.2-bookworm`，固定已從 registry 查得的 Linux amd64 manifest digest；不使用浮動 latest。
- Bootstrap timeout：[kubeadm v1beta4 Timeouts](https://kubernetes.io/docs/reference/config-api/kubeadm-config.v1beta4/)，透過 [kind kubeadm config patches](https://kind.sigs.k8s.io/docs/user/configuration/#kubeadm-config-patches) 套用。
- Runtime timeout：[KubeletConfiguration runtimeRequestTimeout](https://kubernetes.io/docs/reference/config-api/kubelet-config.v1beta1/)。
- M1 必須重新確認 [Kubebuilder 的工具相容性／支援平台](https://book.kubebuilder.io/versions_compatibility_supportability.html)，不能從 M0 版本推論所有 Controller library 都相容。

## 取捨

- M0 安裝腳本只支援 Linux amd64；Windows native、arm64 尚未驗收。
- 專案位於 `/mnt/d` 可直接共用檔案，但首次 Go build／工具解壓可能比 WSL Linux filesystem 慢。
- 本機 Docker 是既有安裝，不在 M0 自動升級；可用性與相容性以實際 kind 測試記錄為準。
- 下載工具和 node image 需要網路；固定來源與校驗碼讓失敗可定位，不能以 Mock 取代部署。
