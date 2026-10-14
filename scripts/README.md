# 跨階段共用腳本

各階段的實際操作流程在 [stages](../stages/README.md) 的 scripts/control.sh。這裡保留共同環境與命令入口：

| 檔案 | 用途 |
|---|---|
| [common.sh](common.sh) | 專案路徑、工具／快取環境、隔離 kubeconfig、固定叢集名稱、ownership 核對與共用函式 |
| [kind.sh](kind.sh) | 共用 kind 建立／就緒與限定 ownership 的清理；依 HF_CLUSTER 選擇叢集與 API port |
| [hf.sh](hf.sh) | Make 命令分派、舊命令相容入口，以及跨階段 format／lint／build／驗證命令 |
| [bootstrap-tools.sh](bootstrap-tools.sh) | 安裝固定版本的 Go／kind／kubectl 至專案 .tools |
| [cache-native.sh](cache-native.sh) | 在 WSL 原生 Linux filesystem 建立專案 Go 快取，保留來源檔案位置 |
| [versions.env](versions.env) | 工具版本、SHA-256、Docker image digest 的唯一設定來源 |

學習從各階段 run.sh／README 開始。M0 映像、M1 Operator、M2 Hub、M3 Agent、M4 OIDC／Security Profile、M5 Dashboard／Helm 的控制流程已各自歸位，hf.sh 只把對應命令交給它們。完整驗收留在每階段的 scripts/e2e.sh。

## 使用環境與命令

以下在專案根目錄的 Ubuntu WSL2／Linux Bash 執行。學習一般只用 stages 各課的命令；這裡給需要理解共用環境或開發時查閱。本輪實測結果見 [搬移後重新驗證](../docs/revalidation-20261011.md)。

| 命令 | 實際效果 |
|---|---|
| make tools | 下載校驗固定 Go／kind／kubectl，裝入 .tools；不覆寫全域工具 |
| HF_KIND_SOURCE=1 make tools | kind release asset 下載逾時時，用相同固定版源碼安裝 kind；Go／kubectl 照常校驗 |
| make doctor | 檢查固定版號與 Docker Linux containers，編譯版本檢查程式 |
| make help | 顯示 Make 入口，不建立叢集 |
| make cache-native | 建立專案專用的 WSL Linux filesystem Go 快取，登記到 .local/native-cache.path |
| make fmt | gofmt -w，會修改 Go 原始碼 |
| make lint | 檢查格式、go vet、Bash syntax；不修正程式 |
| make build | 編譯各階段主機程式到 bin，不建 Docker 映像 |
| make test-unit | 全專案 Unit，範圍包含全部已實作階段 |
| make test-race | Unit 與 integration 的 race；需 envtest，無 C compiler 時用固定 Go 容器 |
| make test-oidc | 獨立準備真實 Dex／短 session 本機 Hub，執行 OIDC 負向及約 15 分鐘 JWT 自然到期測試；不建立 Worker |
| make docs-check | 執行 [Go 文件工具](../tools/README.md)；不是完整 Demo 驗證 |

Makefile 保留的舊相容入口：demo-image-* 屬 M0、operator-*／demo-deploy 屬 M1、demo／hub-* 屬 M2。`make demo` 不是全部里程碑共用的啟動；學習請明確使用 m0-up～m5-up。

## common.sh 改哪些環境

source 後會切到專案根目錄，設定固定工具 PATH、GOTOOLCHAIN=local、GOCACHE／GOMODCACHE、KIND provider。HF_CLUSTER 決定 CLUSTER、KUBECONFIG 和 ownership 路徑；支援的叢集名稱受限於程式 allowlist。

例如查看 M1：

```bash
cd /mnt/c/Users/H/Documents/GitHub/win30221/hugofleet
export HF_CLUSTER=hugofleet-m1-demo
unset HF_LEARNING_ROOT
source scripts/common.sh
kubectl --context kind-hugofleet-m1-demo -n hf-demo get applications,pods
```

M5 使用 `source stages/m5/env.sh a`／`b`；M3／M4 分別用 `source stages/m3/env.sh a`／`b`、`source stages/m4/env.sh a`／`b`，讓同一階段的 state 與 Worker kubeconfig 對上。每個新終端重新設定；run.sh 的子程序不會改父終端，環境變數也不會自動跨終端同步。

common.sh 啟用 set -Eeuo pipefail。預期失敗命令用 if 處理；單純 `curl --fail` 遇預期 503 可能讓嚴格 Shell 結束。此檔沒有建立叢集，kind-up 才會做。

common.sh 從腳本所在位置取得 ROOT，stage 腳本沿用它，所以搬移不需要把新絕對路徑寫入程式。這次未搬移 `.tools`／`.cache`／`bin`，也未沿用舊 `.local/native-cache.path`；日後首次準備先 `make tools`，有需要再 `make cache-native` 建立屬於新路徑的 WSL 快取。舊 `/tmp` 快取位於專案資料夾之外，本次未搬移或修改它。

Windows 掛載的套件掃描／編譯較慢時，在沒有該專案 Go 編譯程序執行的終端執行：

```bash
make cache-native
make doctor
```

新快取位於 WSL Linux 磁碟的 `/var/tmp/hugofleet-go-cache.*`，包含固定 SDK、已驗證 module archives、build／race cache 與 `go-tmp`。`common.sh` 每次核對 checkout ownership，並設定 Go 編譯暫存 `GOTMPDIR`；原始碼、bin 與 artifacts 仍在工作目錄。需要預留 Linux 磁碟空間，舊 `/tmp` prefix 保留相容，但 `/tmp` 可能是容量較小的 RAM tmpfs。不要自行清空其它 `/tmp` 資料或借用另一個 checkout 的 cache record；背景與修正見 [ADR 0006](../docs/adr/0006-wsl-native-go-cache.md)。

## kind 建立、重用與清理

kind.sh 依 HF_CLUSTER 選 API port，將 deploy/kind/single-node.yaml 生成到該叢集的 state，等待 Node 與 CoreDNS。已存在時必須匹配 ownership，才重新匯出 kubeconfig 並檢查就緒。2026-10-12 的冷啟動診斷後，kubeadm API 等待為 15 分鐘、kubelet runtime 等待 10 分鐘，外層 kind 建立上限 30 分鐘；應用／Node／CoreDNS 的驗收條件維持原樣。CoreDNS 先等待真實 Available（最多五分鐘），再保留原本完整 rollout 檢查，避免外部 CNI 初次建置消耗 progress deadline 後立即誤判；沒有改寫 Deployment status。

M4／M5 的 Calico profile 會先把官方固定版本的 calico／third-party-cni-plugins／node 映像快取於主機 Docker，再載入精確 owned Worker；首次 registry 下載逾時時，不會先建立一個等待 CNI 的新控制平面。重建會沿用快取，Node／Calico／CoreDNS 的 readiness 仍須通過。來源 manifest 經 versions.env 的 SHA-256 校驗；kindnet 階段不執行這個 Calico 流程。真實 Security Profile 的封包與首次冷啟動失敗證據見 [M4 驗收](../docs/m4-validation.md)。

若卡在 Starting control-plane，先讀命令印出的 artifacts/bootstrap-<cluster>/<time>.log；本輪曾遇到 Docker 節點內 containerd 建立容器逾時，API server 因而連線被拒，核心診斷停在磁碟 flush。這時應用尚未啟動，不能算 Demo 通過。一次啟動一個階段／驗收叢集，等命令的正式逾時結果；失敗後依該入口的 ownership 清理，再重試。E2E 會保存節點診斷並清理本輪叢集，學習 up 失敗則用同一階段 mN-down。完整失敗與重試紀錄見本輪驗證報告。

清理同樣比較 container 完整 ID 與 cluster.id；不同 ID、未登記或其他專案同名容器都拒絕。不使用 Docker 全域 prune。`mN-down` 對自己的學習叢集；E2E 用另外的叢集與清理方式，不能混用。

本輪 WSL／Docker 清理曾回報「did not receive an exit event」。kind.sh 會限時等待，重新核對同一 container ID 後最多重試一次；請等命令結束，再看退出碼及最後的清理結果。若最後仍非零，ownership 記錄保留供安全重試，不手動刪記錄或執行全域 prune。實際重試結果見 [本輪紀錄](../docs/revalidation-20261011.md)。

M3～M5 的管理 port 發布於 127.0.0.1，Hub 僅接受精確 Docker gateway 來源。common.sh 的 kind_ipv4_gateway 明確選取唯一 IPv4 gateway，不依賴 Docker IPAM 陣列次序；IPv6 entry 可能排在第一筆。沒有 IPv4 或出現多個 IPv4 時停止，不擴大成整個網段或關閉來源檢查。M5 全新啟動曾因取到 IPv6 gateway 而 /healthz 回 403，失敗與重驗見 [M5 紀錄](../docs/m5-validation.md)。

HF_ETCD_STORAGE=memory／disk 在新建 kind 時選 storage；不會替已存在叢集搬資料。這與 Hub SQLite 的保存是兩個層次，詳見 [deploy](../deploy/README.md)。

## 哪些環境變數平常會用

| 變數 | 作用／適用 |
|---|---|
| HF_CLUSTER | 共用手動命令選叢集；各學習 run.sh 會自行選正確名稱 |
| HF_PORT | M0～M2 forward 的本機 port，預設 18080 |
| HF_HUB_DATABASE | M2 自訂 DB，每次同輪重啟沿用 |
| HF_HUB_KUBECONFIG／HF_HUB_POLICY | M2 展開手動流程用；m2-up 固定學習路徑／policy |
| HF_M3_STATE | M3 本輪 DB／憑證／ownership／雙 Worker 的資料目錄，每終端一致 |
| HF_M4_STATE／HF_M5_STATE | M4／M5 本輪私有帳號、DB、憑證、ownership 與雙 Worker 的資料目錄；每終端一致 |
| HF_M3_HEARTBEAT_TIMEOUT／HF_M3_OPERATION_TIMEOUT | M3 Hub 的心跳逾時與新操作期限，預設 15s／180s；既有容器需 hub-recreate 套用 |
| HF_M4_HEARTBEAT_TIMEOUT／HF_M5_HEARTBEAT_TIMEOUT | 對應 Hub 的心跳逾時，預設 15s，允許 3s～5m；既有容器需 hub-recreate 套用 |
| HF_M4_OPERATION_TIMEOUT／HF_M5_OPERATION_TIMEOUT | 對應 Hub 的新操作期限，預設 180s；既有操作保留原 deadline |
| HF_M4_SESSION_TTL／HF_M5_SESSION_TTL | 對應 OIDC Hub 的 session TTL，預設 1h；不延長 ID Token 到期時間 |
| HF_ETCD_STORAGE | 新 kind 的 memory／disk profile；預設 memory |

其他驗收用旗標留在各 E2E 腳本，不作日常教學前置。跨階段切換用 stages 的停止／再啟動流程，不以手動改 ownership 記錄解決衝突。
