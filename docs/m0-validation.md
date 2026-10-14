# M0 實測紀錄

日期：2026-10-10 至 2026-10-11（Asia/Taipei）。

本頁保留當次執行環境與命令。專案後來已搬移，現行路徑與啟動步驟見 [根 README](../README.md)／[M0 教學](../stages/m0/README.md)；以下舊路徑不代表搬移後的驗收。

**M0 已於本機 RAM etcd profile 完整驗收：所有工作負載 assertions 與專案叢集清理通過，`make test-e2e` exit 0。**

## 環境

- Windows 11 Pro，Intel i7-12650H，16 logical CPUs，實體 RAM 31.71 GiB；CPU-only。
- Ubuntu WSL2，kernel 6.18.40.1-microsoft-standard-WSL2。
- Docker Desktop 4.25.0／Engine 24.0.6，Linux amd64；Docker RAM 約 15.48 GiB。
- 專案本地 Go 1.27.2、kind 0.33.0、kubectl 1.37.1；kind node 固定官方 Kubernetes 1.37.0 digest。
- 使用獨立 `.local` kubeconfig；既有 `lab`／`kind` 及其他 Docker 容器不納入清理。

## 已執行檢查

所有命令於 `/mnt/d/Dowload/HUGOFLEET_AI_STARTER` 的 Ubuntu WSL2 執行。

| 命令 | 實際結果 |
|---|---|
| `HF_KIND_SOURCE=1 make tools` | PASS：官方固定來源安裝 kind，Go checksum 驗證；Go／kubectl 校驗官方 SHA-256 |
| `make tools` | PASS：重複執行可重用已校驗的專案工具 |
| `make doctor` | PASS：版本相符、Docker Linux engine 可連線 |
| `make fmt lint build test-unit` | PASS：gofmt、go vet、Bash 語法、Go build 與三個 package 測試，包含 Go 驗收工具 |
| `make test-race` | PASS：WSL 無 C compiler，改在固定 Go build container 實跑 `CGO_ENABLED=1 go test -race -count=1 ./...`，三個 package 通過（1.025s／1.015s／1.031s）；完整命令 exit 0 |
| `make benchmark` | PASS：BenchmarkCalculate-16，3.931 ns/op、0 B/op、0 allocs/op；只代表 domain function |
| `make demo-image-build` | PASS：v1／v2-good／v2-broken 三個獨立映像，各約 6.8 MB |
| 非專案 cluster／非法 port 負向檢查 | PASS：`HF_CLUSTER=lab` 拒絕操作；port 0 在建立容器前拒絕 |
| `scripts/hf.sh kind-up`（由 E2E 使用相同入口） | PASS：從零建立 hugofleet-m0-e2e，安裝 CNI／StorageClass，Node Ready 通過 |
| `scripts/hf.sh demo-image-load`（由 E2E 使用相同入口） | PASS：三個本地映像成功載入隔離 kind node |
| `make test-e2e` | PASS：2026-10-11 RAM etcd profile 從零建立 Node／CoreDNS Ready，完成三映像驗證、正常更新、故障 rollout／Service endpoints、恢復 v1；第二次限定刪除成功，完整命令 exit 0 |

Unit／HTTP 測試涵蓋價格計算順序、取整、上限／負數／缺值／非法 tier 和 coupon、JSON 型別／未知欄位／超大 body、method、錯誤 request ID、三版本探針、日誌不輸出 Authorization、真實 HTTP listener 與 graceful shutdown。

## 最終 kind E2E 證據

- 命令：`make test-e2e`，Ubuntu WSL2，2026-10-11 06:00–06:16（Asia/Taipei）；exit 0。
- 產物：[`artifacts/m0-e2e/20261011T060031-257733/run.log`](../artifacts/m0-e2e/20261011T060031-257733/run.log)，忽略提交；包含每一步 HTTP／Kubernetes assertion。
- v1、v2-good：Deployment rollout 成功；`/version`、`/healthz`、`/readyz` 正確，`/quote` 完整回應為 20,000／2,000／10,000／8,000 分。
- v2-broken：直接 Pod HTTP 確認 health 200、ready 503、version 正確、quote 8,000；Deployment `ProgressDeadlineExceeded`、Pod Ready=false、restartCount=0。
- Service：EndpointSlice 保留 Ready endpoints，排除 Broken Pod；原有 v2-good HTTP 仍通過。
- 原生 image update 恢復 v1：rollout、舊 Pod 刪除與 HTTP 再驗證均通過。
- 系統診斷：CoreDNS／etcd／API server／controller-manager／scheduler／kindnet／kube-proxy 全部 Ready、restartCount=0。
- 結束前 `docker stats` 快照：761.7 MiB RAM／15.48 GiB limit、45.56% CPU；只代表該時間點，不是固定資源保證。
- 清理：第一次 Docker rm 未收到 exit event；腳本等待、核對 container ID 後，第二次 kind delete 成功。PASS 在確認清理後輸出。

## 遇到的失敗與修正

1. GitHub release binary 下載速度低；使用 kind 官方支援的固定 tag Go source install，未改用未驗證 binary。
2. 第一次 image build 在外層 timeout 到期後回傳失敗，雖 v1 已產出仍不當作三映像完成。調整下載／建置的 bounded timeout 後重新執行，三映像建置完成。
3. WSL 缺 C compiler，原生 `go test -race` 無法啟動。Race target 改為固定 Go build container，之後真正執行且通過；沒有略過 race。
4. kind 隨機 API port 43817 落入 Windows 保留範圍 43750–43849。改用 loopback 16443／16444；已測試 Windows 可綁定這兩個 port，未改防火牆或 port reservations。
5. 控制平面 bootstrap 曾超過 kubeadm 預設 1 分鐘 API 等待；containerd 日誌也顯示映像解壓超過 kubelet 預設 2 分鐘 runtime 等待而取消。Node OOMKilled=false，當時 WSL 尚有約 11 GiB available。改為 API／runtime 5 分鐘後，四個控制平面容器成功啟動，但 RBAC bootstrap 超過控制平面 5 分鐘健康等待；retained node 隨後才通過全部 `/readyz`。量到 I/O pressure full 約 40%、CPU throttled_usec=0；主機 CPU load 約 19%，磁碟空間充足。控制平面健康等待調為 8 分鐘、外層 20 分鐘；後續結果與最終 RAM profile 另列於本紀錄。
6. 初次清理 retained node 回報未收到 exit event；稍後確認該 node 已退出，重試專案限定 `make kind-down` 後成功移除，未重啟 Docker 或清理其他容器。
7. 首次完整 E2E 已通過 v1 HTTP 與 v2-good rollout，但 v2-good HTTP 首次連線逾時。診斷發現 background shell function 的 PID 並非 kubectl PID，舊 port-forward 未確實結束，持續寫入共用 log。改為 subshell `exec kubectl` 並保存每次獨立 log；正常更新後等待舊 Pod 刪除再對 Service port-forward，HTTP transport timeout 設為有界的 15 秒。版本、狀態、價格與 Kubernetes assertion 未放寬。首次失敗完整產物另存 `artifacts/m0-e2e-first-failure`，修正後從零重跑。
8. E2E 首次清理的兩次快速重試仍未等到 Docker exit event。改為每次失敗後最多等待 60 秒確認容器消失、每次核對 ID，至多兩次刪除；Docker 連線失敗不當作「已清理」。原失敗 node 隨後由專案限定命令成功移除，沒有剩餘 kubectl 程序。
9. 第二次 E2E 在首次 port-forward 時，主程序先讀取尚未由 background redirection 建立的日誌檔而失敗。改為啟動背景程序前建立空日誌；每次 run 使用獨立目錄。該次仍判 FAILED，清理經第二次限定重試成功。修正後再次從零驗證，HTTP／Kubernetes 驗收已改由 Go 執行，Bash 保留流程串接。
10. Go 驗收版第一次 E2E 建立 Node Ready 成功，但 v1 rollout 等待 120 秒逾時。Pod 約 13 秒內 Ready、restartCount=0；Deployment 尚未同步可用狀態。控制平面日誌顯示 controller-manager 在 14:05:29 因 lease API 的 5 秒 deadline 失去 leadership，14:07:05 才重新同步。該輪與 Docker race 建置並行，可能加重 I/O；後續改為完全依序執行，且 kind-up 加入 CoreDNS rollout readiness。Node 啟動中量到 464.8 MiB／18.18% CPU，I/O full avg60=40.31%、etcd fdatasync 約 1.23 秒；失敗清理前快照為 730.5 MiB／11.49% CPU。這些是單次快照，不能作為資源保證。
11. 完全依序執行的下一輪仍在 CoreDNS readiness 逾時，沒有部署 Sample。kubelet 日誌確認 CoreDNS／local-path 的 ConfigMap cache 同步逾時；節點清理成功。採用 ADR 0004 的 M0 RAM etcd profile 重跑；不關閉 fsync、不調弱驗收條件。標準 disk profile 完整 E2E 仍未通過。
12. 首次 RAM profile 試跑由 `stat`／etcd manifest 確認真實 data-dir 位於 tmpfs，但失敗前 etcd／API 容器尚未 Start。CRI 在 14:36:31 開始 CreateContainer，直到 14:40:46 才完成 controller-manager CreateContainer；kubeadm API bootstrap 的 5 分鐘等待先到期。當次測試 FAILED、清理成功。2026-10-11 重試前沒有剩餘專案容器，WSL I/O full avg10 約 3.17%；API 等待改為 10 分鐘、總 kind timeout 仍為 20 分鐘，重新驗證。

Bootstrap 診斷位於忽略提交的 `artifacts/bootstrap-<cluster>`；E2E 保存 `artifacts/m0-e2e/<run>`。最後的 PASS 僅在所有 assertion 與叢集清理完成後輸出。

## 範圍與限制

- GitHub Actions 只有本地 workflow 定義；尚未建立／推送遠端 repository，**GitHub CI NOT VERIFIED**。
- M1–M5 尚未開始；沒有 Application Operator、Hub、Agent、mTLS、OIDC 或平台回滾。
- envtest 屬於 M1，目前未加入，不能宣稱已執行。
- M0 Sample App 僅供本機展示；正式管理 API 的認證／授權不在本階段。
- port-forward 驗證實際 Pod HTTP；Service 的 Ready endpoints 由 EndpointSlice 驗證，不等同已測 kube-proxy／CNI 的完整資料路徑。
- M0 RAM etcd 會在 Docker node 停止／重啟後失去資料；需清理、重新建立。磁碟 profile 的完整 E2E 尚未通過，不能聲稱持久性／節點重啟恢復已驗證；M1 起重新評估。

## 2026-10-11 M2 審查後回歸

上方限制記錄原始 M0 驗收時的範圍；目前專案停止於 M2，後續結果見 [M2 紀錄](m2-validation.md)。本次補齊 M0／M1／M2 CI matrix，並讓 M0 E2E 在建立叢集前自行編譯 m0check，不依賴 checkout 已有 bin 檔案。

`make test-e2e-m0` exit 0，Run `20261011T113245-453358`，從零建立真實 kind，全部 HTTP／壞版 rollout／EndpointSlice／回滾 Assertion 與 owned cleanup 通過。Docker 初次移除未收到 exit event，原有 ownership 檢查及第二次刪除成功；未跳過清理。原始紀錄 `artifacts/m0-e2e/20261011T113245-453358/run.log`。這是本機結果，GitHub hosted CI 尚未執行。
