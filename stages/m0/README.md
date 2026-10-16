# M0：把價格 API 部署到 Kubernetes

目前位置：M0／M0～M5 學習路線 · [專案入口／環境準備](../../README.md#首次準備環境) · [選課與回訪](../README.md#回來繼續哪一課)

第一次練習從[開始前的準備](#開始前的準備)進入；上次做到一半則先看[回來繼續時先選擇](#回來繼續時先選擇)。

## 這一課的目標與成果

這一課會帶你啟動 Go 價格 API，把它部署到本機 Kubernetes，再練習呼叫 API、更新版本、增加副本，以及恢復故障版本。

先完成一次操作，再往後閱讀[檔案地圖](#檔案地圖)、[程式導讀](#本階段的程式導讀)與 [HTTP 契約](#pricing-api-的-http-契約)。本課使用原生 Deployment／Service，後續的 Application CRD 與 Operator 見 [M1 教學](../m1/README.md)。

完成後，你會看到價格 API 回傳 8000 分、健康版更新與兩個副本，並能辨識壞版 readiness 失敗及恢復 v1。此課不使用 Hub 或 Operator。

依序閱讀：[開始前的準備](#開始前的準備) → [逐步操作](#逐步操作) → [整課驗證與常見問題](#整課驗證與常見問題) → [停止與下次繼續](#停止與下次繼續)。完成後再查[原理與進階閱讀](#原理與進階閱讀)。

## 開始前的準備

### 1. 從專案資料夾進入 WSL

Windows 使用者先啟動 Docker Desktop，確認使用 Linux containers，並已啟用 Ubuntu 的 WSL integration。前置工具與環境需求見[根 README](../../README.md#首次準備環境)。

1. 在檔案總管開啟下載的 `hugofleet` 資料夾。
2. 確認這一層包含 `Makefile`、`go.mod` 和 `stages`。
3. 在檔案總管網址列輸入 `powershell`，按 Enter。
4. 在開啟的 PowerShell 執行：

```powershell
wsl -d Ubuntu
```

進入 Ubuntu 後，確認目前位置：

```bash
pwd
ls Makefile go.mod stages
```

`pwd` 顯示的路徑依下載位置而不同。`ls` 應能列出上述檔案與資料夾，沒有找不到檔案的錯誤。如果位置不符，先回到專案資料夾重新開啟終端，並檢查 Shell 啟動設定是否切換了目錄。

**後面的命令都在 WSL 裡、專案根目錄下執行。** 原生 Linux amd64 使用者直接在專案根目錄開啟 Bash 即可。

### 2. 首次安裝工具

若剛從根 README 完成工具安裝與 doctor，略過本節，直接到[準備三個終端](#3-準備三個終端)。只有尚未安裝工具時才執行：

```bash
make tools
```

這是在準備 WSL／Linux 執行專案所需的工具：下載、校驗固定版本的 Go、kind、kubectl，安裝到專案內的 `.tools`，不覆寫全域安裝。即使 Windows 已安裝 Go，首次使用仍需準備這些 Linux 版工具。

等待完成後再執行：

```bash
make doctor
```

| 命令 | 何時執行 | 預期結果 |
|---|---|---|
| `make tools` | 首次使用或需要重新準備工具時 | 顯示 `Pinned project-local tools installed successfully.` |
| `make doctor` | 開始操作前 | 印出工具版本與 Docker 資訊，沒有錯誤 |

已完成本專案工具安裝後，不必每開一個終端就重裝。若檢查失敗，先處理錯誤再繼續。

`doctor` 最後可能顯示 `Project cluster=hugofleet-a`，這是共用命令的預設選擇，不代表已建立叢集。下一步的 `make m0-up` 會自行選擇 M0 專用的 `hugofleet-m0-demo`。

### 3. 準備三個終端

| 終端 | 用途 | 是否持續占用 |
|---|---|---|
| A | 啟動、查看、更新與清理 Kubernetes | 命令完成後可繼續輸入 |
| B | 將叢集服務轉送到本機 | 轉送期間保持執行 |
| C | 用 curl 呼叫 API | 呼叫完成後返回 |

目前的終端是 A。需要 B、C 時，從同一個專案資料夾開啟新的 PowerShell，再執行 `wsl -d Ubuntu`。

## 逐步操作

依下列順序切換終端與瀏覽器。每一步先核對預期結果再繼續；命令失敗時，先看[常見問題](#操作遇到問題時)，不要接著執行後續變更。

### 4. 終端 A：啟動 M0

執行：

```bash
make m0-up
```

| 步驟 | 做什麼 |
|---|---|
| 建置映像 | 產生 `pricing-api:v1`、`v2-good`、`v2-broken` 三個版本 |
| 建立叢集 | 建立單節點 kind 叢集 `hugofleet-m0-demo` |
| 載入映像 | 把主機建好的映像放進 kind 節點，供 Kubernetes 使用 |
| 部署 API | 建立 `hf-demo` Namespace、價格 API 的 Deployment 與 Service |
| 等待就緒 | 等待 Deployment 完成部署 |

首次下載、建置與啟動可能超過十分鐘。請等待命令結束，畫面會顯示進度。成功時最後應看到：

```text
m0 ready; use make m0-forward in another terminal.
```

命令返回後，API 仍在 Kubernetes 裡執行。初始應有一個價格 API Pod。

**下一步：**確認本步結果後，前往[5. 終端 B：建立服務轉送](#5-終端-b建立服務轉送)。 若未符合預期，先到[操作排查](#操作遇到問題時)，處理後回到本步重試。

### 5. 終端 B：建立服務轉送

執行：

```bash
make m0-forward
```

等待畫面出現包含以下內容的訊息：

```text
Forwarding from 127.0.0.1:18080
```

「轉送」就是暫時把叢集裡的 API 接到本機連接埠，讓你用 `localhost:18080` 測試。這裡使用 Kubernetes port-forward，經 API 通道連到 Service 選中的 Pod；沒有建立 Ingress，也沒有將服務公開到網際網路。

**保持終端 B 執行。停在這裡等待是正常的。** 按 Ctrl+C 只停止轉送，叢集中的 API 仍會執行。

**下一步：**確認本步結果後，前往[6. 終端 C：確認 API 能用](#6-終端-c確認-api-能用)。 若未符合預期，先到[操作排查](#操作遇到問題時)，處理後回到本步重試。

### 6. 終端 C：確認 API 能用

先查版本：

```bash
curl --fail http://127.0.0.1:18080/version
```

預期回應包含 `v1`。再查健康與就緒狀態：

```bash
curl --include --fail http://127.0.0.1:18080/healthz
curl --include --fail http://127.0.0.1:18080/readyz
```

兩個請求都應回傳 HTTP `200`。接著計算價格：

```bash
curl --fail http://127.0.0.1:18080/quote \
  -H 'Content-Type: application/json' \
  --data '{"amount_cents":20000,"customer_tier":"vip","coupon_code":"SAVE100"}'
```

| 輸入 | 意義 |
|---|---|
| `amount_cents: 20000` | 原價 20,000 分，即 200 元 |
| `customer_tier: vip` | VIP 先打九折 |
| `coupon_code: SAVE100` | 再折抵 100 元 |

預期價格回應包含以下數值，JSON 空白與欄位順序不必相同：

```json
{
  "original_amount_cents": 20000,
  "tier_discount_cents": 2000,
  "coupon_discount_cents": 10000,
  "final_amount_cents": 8000
}
```

計算方式是 `200 元 × 90% − 100 元 = 80 元`。看到版本與正確價格，代表已實際呼叫到叢集中的服務。

**下一步：**確認本步結果後，前往[7. 終端 A：查看 Kubernetes 資源](#7-終端-a查看-kubernetes-資源)。 若未符合預期，先到[操作排查](#操作遇到問題時)，處理後回到本步重試。

### 7. 終端 A：查看 Kubernetes 資源

查看目前的 Deployment、Service、Pod 與事件：

```bash
make m0-status
```

命令會自動選擇本課叢集。預期價格 API Deployment READY 為 `1/1`、一個價格 API Pod 就緒，Service 存在。Deployment 管理 Pod 的版本與份數；Pod 真正執行程式；Service 提供服務入口。

<details>
<summary>展開：底層 kubectl 指令與詳細核對</summary>

接下來直接使用 kubectl。先設定目前終端的 M0 環境：

```bash
export HF_CLUSTER=hugofleet-m0-demo
unset HF_LEARNING_ROOT
source scripts/common.sh
```

| 命令 | 用途 |
|---|---|
| `export HF_CLUSTER=...` | 選擇 M0 學習叢集 |
| `unset HF_LEARNING_ROOT` | 清除可能沿用的自訂學習路徑 |
| `source scripts/common.sh` | 在目前終端載入專案工具與隔離 kubeconfig 設定 |

`make m0-up` 只會替自己的子程序設定環境。換新的終端操作 kubectl 時，要重新執行上述三行。`common.sh` 也會啟用嚴格錯誤處理，後面的預期失敗命令會用 `if` 包住。

查看資源：

```bash
kubectl --context kind-hugofleet-m0-demo \
  -n hf-demo get deployments,services,pods
```

`--context` 指定叢集，`-n hf-demo` 指定 Namespace，`get` 列出資源與狀態。

| 資源 | 預期狀態 | 用途 |
|---|---|---|
| Deployment `pricing-api` | READY `1/1` | 管理映像版本與副本數 |
| Service `pricing-api` | 存在，類型為 ClusterIP | 提供服務名稱與連接埠 |
| 一個價格 API Pod | READY `1/1`、STATUS `Running` | 實際執行程式 |

映像是打包好的程式，Pod 執行程式，Deployment 管理 Pod，Service 提供服務入口。本機 `18080` 經轉送連到 Pod 的 `8080`。

</details>

**下一步：**確認本步結果後，前往[8. 終端 A：更新健康版本](#8-終端-a更新健康版本)。 若未符合預期，先到[操作排查](#操作遇到問題時)，處理後回到本步重試。

### 8. 終端 A：更新健康版本

這次要把價格 API 換成健康新版，副本數保持目前設定。

```bash
make m0-update-good
```

命令會更新映像並等待部署完成，最後列出 Pod。成功後，終端 B 按 Ctrl+C，再執行 `make m0-forward`；終端 C 用 `curl --fail http://127.0.0.1:18080/version` 確認回傳 `v2-good`。轉送連著特定 Pod，因此換版後要重建。

主線的短命令會自動選擇 M0 練習叢集與專案工具，不必先設定 kubectl 環境。

<details>
<summary>展開：底層 kubectl 指令與詳細核對</summary>

將映像改為 `v2-good`：

```bash
kubectl --context kind-hugofleet-m0-demo -n hf-demo \
  set image deployment/pricing-api pricing-api=pricing-api:v2-good
```

這是在告訴 Deployment 使用新的映像；命令成功不代表更新已完成。更新需要時間，執行以下命令等待部署完成，確認成功後再測試新版 API：

```bash
kubectl --context kind-hugofleet-m0-demo -n hf-demo \
  rollout status deployment/pricing-api --timeout=180s
```

預期看到 `successfully rolled out`。`rollout status` 只觀察進度，不會修改部署；等待超過 180 秒會回報錯誤，但不會停止 Kubernetes 繼續更新。

查看 Pod：

```bash
kubectl --context kind-hugofleet-m0-demo -n hf-demo get pods
```

等舊 Pod 退出、新 Pod 就緒後：

1. 終端 B 按 Ctrl+C。
2. 終端 B 重新執行 `make m0-forward`。
3. 終端 C 查版本：

```bash
curl --fail http://127.0.0.1:18080/version
```

預期版本變成 `v2-good`。重新建立轉送，是因為 port-forward 連著特定 Pod，原連線不會自動切換到新版 Pod。

</details>

**下一步：**確認本步結果後，前往[9. 終端 A：增加副本數](#9-終端-a增加副本數)。 若未符合預期，先到[操作排查](#操作遇到問題時)，處理後回到本步重試。

### 9. 終端 A：增加副本數

這次保留同一個版本，請 Kubernetes 同時執行兩份價格 API：

```bash
make m0-scale-two
```

命令會把 Deployment 副本設為 2 並等待更新完成。預期 READY 為 `2/2`，有兩個就緒的價格 API Pod。需要再查一次時執行 `make m0-status`。

<details>
<summary>展開：底層 kubectl 指令與詳細核對</summary>

將副本數改為兩個：

```bash
kubectl --context kind-hugofleet-m0-demo -n hf-demo \
  scale deployment/pricing-api --replicas=2
```

等待副本就緒：

```bash
kubectl --context kind-hugofleet-m0-demo -n hf-demo \
  rollout status deployment/pricing-api --timeout=180s
```

查看結果：

```bash
kubectl --context kind-hugofleet-m0-demo -n hf-demo \
  get deployments,pods
```

預期 Deployment READY 為 `2/2`，有兩個就緒的價格 API Pod。Kubernetes 使用同一個映像啟動兩份程式。

</details>

**下一步：**確認本步結果後，前往[10. 終端 A：部署故障版本](#10-終端-a部署故障版本)。 若未符合預期，先到[操作排查](#操作遇到問題時)，處理後回到本步重試。

### 10. 終端 A：部署故障版本

這次刻意換成故障版，學習「程式正在執行」與「能接收流量」的差別：

```bash
make m0-break
make m0-status
```

第一個命令只提交故障映像，返回不代表更新完成。第二個命令查看 Pod 與事件；探針需要時間，稍候可再查一次。預期新版 Pod 為 Running，但 READY 為 `0/1`，事件顯示 readiness probe 失敗，因為故障版的 `/readyz` 固定回 503。

資源足夠時，舊健康 Pod 繼續服務。若想持續看變化，執行 `make m0-observe`，按 Ctrl+C 停止觀察。僅看到命令成功或等待逾時，都不足以確認故障原因。

<details>
<summary>展開：底層 kubectl 指令與詳細核對</summary>

`v2-broken` 用來展示「程序還活著，但無法通過就緒檢查」：

| 端點 | 回應 | 意義 |
|---|---|---|
| `/healthz` | HTTP 200 | 程序仍存活 |
| `/readyz` | HTTP 503 | 無法通過就緒檢查 |

更新到故障版：

```bash
kubectl --context kind-hugofleet-m0-demo -n hf-demo \
  set image deployment/pricing-api pricing-api=pricing-api:v2-broken
```

等待更新結果：

```bash
if kubectl --context kind-hugofleet-m0-demo -n hf-demo \
  rollout status deployment/pricing-api --timeout=180s; then
  printf '非預期：壞版部署成功，請檢查映像。\n'
else
  printf '預期：壞版無法完成部署，接著查看 Pod 與事件。\n'
fi
```

**這一步預期失敗**，可能顯示 `ProgressDeadlineExceeded` 或等待逾時。使用 `if` 是為了保留終端，繼續查原因。

```bash
kubectl --context kind-hugofleet-m0-demo -n hf-demo get pods
kubectl --context kind-hugofleet-m0-demo -n hf-demo \
  get events --sort-by=.lastTimestamp
```

預期新版 Pod 為 `Running`，但 READY 為 `0/1`，事件顯示 readiness probe 失敗，Deployment 無法完成更新。程序不會因這個 readiness 故障重啟。

本課使用 `maxSurge=1`、`maxUnavailable=0`；資源足夠時，舊健康版會繼續服務，壞版不會成為 Service 的就緒端點。**Running 表示程式正在執行；Ready 表示已通過接收流量的檢查。**

</details>

**下一步：**確認本步結果後，前往[11. 終端 A：恢復到 v1](#11-終端-a恢復到-v1)。 若未符合預期，先到[操作排查](#操作遇到問題時)，處理後回到本步重試。

### 11. 終端 A：恢復到 v1

確認故障後，由你指定換回健康的 v1：

```bash
make m0-restore
```

命令更新映像並等待部署完成，保留兩個副本。確認故障 Pod 退出、健康 Pod 就緒後，終端 B 按 Ctrl+C，再執行 `make m0-forward`。終端 C 驗證：

```bash
curl --fail http://127.0.0.1:18080/version
curl --include --fail http://127.0.0.1:18080/readyz
```

預期為 v1、HTTP 200。再執行第 6 節價格請求，結果仍為 8000。這些操作修改叢集中的 Deployment，不會改寫本機 YAML。

<details>
<summary>展開：底層 kubectl 指令與詳細核對</summary>

將映像改回 `v1`：

```bash
kubectl --context kind-hugofleet-m0-demo -n hf-demo \
  set image deployment/pricing-api pricing-api=pricing-api:v1
```

等待恢復完成：

```bash
kubectl --context kind-hugofleet-m0-demo -n hf-demo \
  rollout status deployment/pricing-api --timeout=180s
kubectl --context kind-hugofleet-m0-demo -n hf-demo get pods
```

確認壞版退出、健康版就緒後，終端 B 按 Ctrl+C，再執行：

```bash
make m0-forward
```

終端 C 確認版本與就緒狀態：

```bash
curl --fail http://127.0.0.1:18080/version
curl --include --fail http://127.0.0.1:18080/readyz
```

預期版本恢復為 `v1`，就緒檢查回傳 HTTP `200`。再執行第 6 節的價格請求，最終金額應為 `8000`。

這次只修改映像，先前設定的兩個副本仍會保留。`set image`、`scale` 改的是叢集內物件，不會修改本機 YAML。這是原生 Kubernetes 更新，沒有 M2 的 Release／Operation 履歷。

</details>

**下一步：**前往[整課驗證](#整課驗證與常見問題)，核對這一輪的結果。 若未符合預期，先到[操作排查](#操作遇到問題時)，處理後回到本步重試。

## 整課驗證與常見問題

完成主線後，核對以下結果；若有一項未達成，先排查再繼續。

- 首次呼叫 `/version` 為 v1，`/quote` 的 `final_amount_cents` 為 8000。
- 健康更新後能觀察 v2-good 與兩個就緒副本。
- 壞版 readiness 失敗，rollout 無法完成；恢復後為 v1、兩個就緒副本。

這些是本輪操作的檢查點，不是新的驗收證據。自動化測試與既有結果見 [M0 驗收](../../docs/m0-validation.md)。


**核對完成後：**若已達成本課成果，前往[停止與下次繼續](#停止與下次繼續)。想繼續進階實作時先保留環境，從[原理與進階閱讀](#原理與進階閱讀)選擇練習；純閱讀不需保持服務運作。尚有問題則先用下方排查，修正後回到失敗步驟。
### 操作遇到問題時

先在查詢終端設定 M0 環境，再查工作負載與事件：

```bash
export HF_CLUSTER=hugofleet-m0-demo
unset HF_LEARNING_ROOT
source scripts/common.sh
kubectl --context kind-hugofleet-m0-demo -n hf-demo get deployments,services,pods
kubectl --context kind-hugofleet-m0-demo -n hf-demo get events --sort-by=.lastTimestamp
kubectl --context kind-hugofleet-m0-demo -n hf-demo logs deployment/pricing-api --tail=50
```

| 現象 | 原因判讀與下一步 |
|---|---|
| 18080 connection refused | 確認 B 終端顯示 Forwarding 且未結束，再確認 Service／Pod 存在 |
| bind address already in use | 停止上一課的轉送；需要第二入口時用 `HF_PORT=18082 make m0-forward`，curl 也改 port |
| ImagePullBackOff | 確認 image 拼字、先建映像再載入此叢集；不能只看 host 有映像 |
| Pending | 看 events 的排程／資源原因 |
| Running 但未 Ready | 看 image 是否 broken、探針 path／port；第 10 節這是預期 |
| 更新後仍看到舊版 | 等 rollout 與舊 Pod 退出，再重建轉送 |
| kubeconfig／ownership mismatch | 確認 HF_CLUSTER 與 source；不手改 cluster.id 接管同名容器 |
| curl 400／415／422 | 對照下方契約，JSON、Content-Type 與業務欄位是不同檢查 |

只想再連原服務時，重新 forward 即可。`make m0-up` 會再次套用初始 YAML，不能把它當成保留手動 image／replicas 的純重連命令。

程式上，一個 /quote 請求經 port-forward 進入 Pod 的 HTTP server，再由 NewHandler 選路由、驗證輸入、呼叫 Calculate、回傳 JSON。跟著 [pricing-api 深入導讀](pricing-api/README.md) 可把 HTTP 行為對到函式與測試。

## 停止與下次繼續

### 回來繼續時先選擇

先依原環境的情況選一條路；下方停止與重建命令不是需要依序全部執行的清單。

| 目前情況 | 下一步 |
|---|---|
| 原叢集與應用仍正常，只停了轉送 | 重新執行 `make m0-forward`，再回[逐步操作](#逐步操作)確認 API；不用重跑 up。 |
| 已 down 或應用已刪除 | 依下方結束與重建說明，從初始狀態重新練習。 |
| Docker／WSL 曾重啟，狀態不確定 | 先看[操作排查](#操作遇到問題時)，不要假設 RAM etcd 的資料還在。 |

### 12. 結束練習與下次使用

終端 B 按 Ctrl+C 停止轉送，再於終端 A 執行：

```bash
make m0-down
```

這會刪除 M0 學習叢集，以及裡面的 Deployment、Service、Pod。專案原始碼、已安裝工具、主機 Docker 映像與測試產物會保留。轉往 M1 前，先完成這個清理，再依 [M1 教學](../m1/README.md) 操作。

下次從初始狀態重新練習：

```bash
make m0-up
```

若只停止轉送、保留了仍正常運作的叢集，重新執行 `make m0-forward` 即可連線。`make m0-up` 會重新套用初始 YAML，將映像與副本數設回 `v1`、`1`。

預設 etcd 使用 RAM 儲存；Docker 節點／WSL 重啟後不能假設資料仍在。先檢查叢集，必要時用 `make m0-down` 限定清理，再重新啟動。清理會核對專案記錄的 container ID。

### 完成這一課後去哪裡

- **進入下一課：**先停止本課的 watch／轉送，以原階段的環境執行 `make m0-down`；完成後前往 [M1：讓 Operator 管理應用](../m1/README.md#開始前的準備)。
- **下次繼續／重新練習：**回到[本輪環境選擇](#回來繼續時先選擇)，依原叢集與資料是否仍有效決定恢復方式。
- **深入本課：**閱讀[原理與進階閱讀](#原理與進階閱讀)。要做需要叢集的進階演練，先保留本輪環境；完整 down 後須依重建流程重新準備。

## 原理與進階閱讀

以下依需要查閱；進階演練若需要仍在執行的環境，請在完整 down 前操作。開發測試入口不屬於首次教學的必做步驟。

### pricing-api 的 HTTP 契約

| 欄位 | 規則 |
|---|---|
| amount_cents | 必填，整數，0–1,000,000,000,000 |
| customer_tier | 必填，standard／vip，區分大小寫 |
| coupon_code | 可省略、空字串或 SAVE100；明確傳入 null 回 400 |

VIP 先以整數分計算九折並向下取整，再用 SAVE100 最多折抵 10,000 分，最低零。例如 101 分 VIP 為 90 分；100 分 VIP 加 SAVE100 為零。

POST /quote 只接受 application/json、單一 JSON 文件、最大 4 KiB。錯誤：無效 JSON／未知欄位 400、業務輸入 422、Content-Type 415、body 過大 413、method 405。回應含 X-Request-ID；錯誤含 code／message／request_id。JSON 日誌記錄請求識別、method、path、status、duration_ms，不記錄 Authorization 或 request body。

| 獨立映像 | /healthz | /readyz | /version |
|---|---|---|---|
| pricing-api:v1 | 200 | 200 | v1 |
| pricing-api:v2-good | 200 | 200 | v2-good |
| pricing-api:v2-broken | 200 | 503 | v2-broken |

版本由建置注入；沒有 runtime 開關偽裝版本。Sample 無認證，僅供本機展示。Pods 設 requests／limits、non-root、唯讀 filesystem、drop ALL capabilities、關閉 workload SA token mount；RollingUpdate 使用 maxSurge=1／maxUnavailable=0，仍需額外 Pod 資源。

### 檔案地圖

| 位置 | 裡面做什麼 | 如何使用／是否手改 |
|---|---|---|
| [run.sh](run.sh) | 固定 M0 學習叢集，組合建置／部署／轉送／清理 | `bash run.sh up`／`forward`／`down`；通常不需修改 |
| [workload.yaml](workload.yaml) | Namespace、原生 Deployment／Service、資源與探針 | 本課可改的宣告範例；修改後要 apply 才影響叢集 |
| [pricing-api/main.go](pricing-api/main.go) | HTTP server、版本注入、關閉流程 | 程式入口；不是 Kubernetes 控制器 |
| [pricing-api/internal/pricing](pricing-api/internal/pricing/) | HTTP 路由與價格計算；同目錄有 Unit | 想改 API 或公式看這裡，詳細流程見 [元件導讀](pricing-api/README.md) |
| [pricing-api/Dockerfile](pricing-api/Dockerfile) | 把同一程式建成三個版本的映像 | build arg 注入版本；修改 Go 後重建映像 |
| [cmd/m0check](cmd/m0check/) | HTTP、rollout、Pod／endpoint 的 Go 驗收工具 | 正式驗收工具；不用修改來操作服務 |
| [scripts/control.sh](scripts/control.sh) | 主機 build、映像 build／load、deploy、forward、Unit／benchmark | 想知道 run.sh 每一步做什麼，對照這裡 |
| [scripts/e2e.sh](scripts/e2e.sh) | 組裝完整 M0 驗收與 owned cleanup | 使用另外的測試叢集，見「測試與證據」 |

本階段沿用 [scripts/common.sh](../../scripts/common.sh) 的工具與 kubeconfig 設定，以及 [kind 模板](../../deploy/README.md)。執行產物在 `.local/tutorials/m0/`，主機程式在 bin，Docker 映像在 Docker image store；它們不是本課原始碼。

### 本階段的程式導讀

M0 新增可執行的 pricing-api 與驗收工具。它直接使用 Kubernetes Deployment／Service；此入口不用 Hub、Operator 或 Agent。

| 閱讀順序 | 檔案 | 要理解的事情 |
|---|---|---|
| 1 | [pricing-api/main.go](pricing-api/main.go) | HTTP server 啟動、版本注入、關閉 |
| 2 | [pricing/http.go](pricing-api/internal/pricing/http.go) | quote／healthz／readyz／version 路由與輸入驗證 |
| 3 | [pricing/quote.go](pricing-api/internal/pricing/quote.go) | 整數價格、VIP 折扣與優惠券 |
| 4 | [Dockerfile](pricing-api/Dockerfile) | 三版映像、固定 Go builder、非 root runtime |
| 5 | [workload.yaml](workload.yaml) | 原生 Deployment、Service、readiness／liveness |
| 6 | [m0check/main.go](cmd/m0check/main.go) | 實際 HTTP、Pod／rollout／endpoints 驗收 |

`*_test.go` 與受測程式放在同一套件。完整 kind 驗收由本資料夾的 [scripts/e2e.sh](scripts/e2e.sh) 串接；教學啟動則用 [workload.yaml](workload.yaml)，可在這份教學範例做練習。

單階段開發可用 make m0-build／m0-test，benchmark 與完整驗收見「測試與證據」。這些與首次 up 是不同用途，驗收範圍見 [驗證總覽](../../docs/validation.md)。

後面階段沿用同一 pricing-api；M0 的工作負載由原生 manifest 建立，M1 起改由 Operator 產生。讀完後應能沿著 HTTP 請求找到價格規則，並解釋壞版為何 healthz 正常而 readyz 失敗，再往 [M1](../m1/README.md) 走。

### 進階：拆開啟動步驟

首次學習可略過此節。這是 `make m0-up` 的替代路徑，已完成啟動就不需再跑一遍。請在 M0 專用環境操作，不要套用到 Hub／Operator 管理的工作負載。

終端 A 先設定 M0 環境，再依序執行：

```bash
export HF_CLUSTER=hugofleet-m0-demo
unset HF_LEARNING_ROOT
source scripts/common.sh
make demo-image-build
make kind-up
make demo-image-load
kubectl --context kind-hugofleet-m0-demo apply -f stages/m0/workload.yaml
kubectl --context kind-hugofleet-m0-demo -n hf-demo \
  rollout status deployment/pricing-api --timeout=180s
```

| 命令 | 用途 |
|---|---|
| `make demo-image-build` | 建置三個映像，同來源於建置時注入不同版本 |
| `make kind-up` | 建立或檢查指定叢集，等待 Node 與 CoreDNS 就緒 |
| `make demo-image-load` | 將主機映像載入 kind 節點 |
| `kubectl apply` | 套用本機 YAML，建立或更新 Kubernetes 資源 |
| `kubectl rollout status` | 等待部署完成 |

`kind-up` 本身不會部署 API，也不會自行選擇 M0 叢集，所以上方必須先設定 `HF_CLUSTER`。啟動診斷日誌位於 `artifacts/bootstrap-hugofleet-m0-demo`。

M0 使用本課的原生 manifest。也可在 `stages/m0` 執行 `bash run.sh up`／`forward`／`down`，但本課詳細命令以專案根目錄為起點。

### 進階：直接觀察故障 Pod

在第 10 節的故障狀態，可以查看 Deployment 與就緒端點：

```bash
kubectl --context kind-hugofleet-m0-demo -n hf-demo get deployment pricing-api -o yaml
kubectl --context kind-hugofleet-m0-demo -n hf-demo \
  get endpointslices -l kubernetes.io/service-name=pricing-api -o yaml
```

若要直接確認故障 Pod 的 HTTP 回應，從 Pod 清單取得實際名稱。在另一個終端設定 M0 環境，用不同的本機 port 直接連到該 Pod：

```bash
export HF_CLUSTER=hugofleet-m0-demo
unset HF_LEARNING_ROOT
source scripts/common.sh
broken_pod='<實際的 Broken Pod 名稱>'
kubectl --context kind-hugofleet-m0-demo -n hf-demo port-forward \
  --address 127.0.0.1 "pod/$broken_pod" 18081:8080
```

保持轉送，在終端 C 執行：

```bash
curl --include http://127.0.0.1:18081/healthz
curl --include http://127.0.0.1:18081/readyz
curl --fail http://127.0.0.1:18081/version
```

預期分別為 HTTP `200`、HTTP `503`、版本 `v2-broken`。此處 readiness 不加 `--fail`，方便查看預期的 503。觀察完按 Ctrl+C 停止這條轉送，再回第 11 節恢復。

### 測試與證據

以下是開發驗證選項，不是閱讀本課的必做前置；驗收範圍見階段驗收文件。

進入 M1 前，先確認自己能回答：

| 問題 | 用本階段操作核對 |
|---|---|
| 映像與 Pod 有什麼差別？ | 同一個 v1 映像可以執行兩個 Pod |
| readiness 與 liveness 為何分開？ | 壞版程序存活，但不能加入就緒服務端點 |
| 誰維持副本數？ | Deployment 控制器維持目前 spec；M0 沒有 Operator 恢復被手改的 spec |
| 服務如何證明真的能用？ | 真實 Pod 回傳 v1 與價格 8000 |

能說明這些結果後，再前往 [M1](../m1/README.md)。

只檢查 M0 Go 程式：

```bash
make m0-test
make benchmark
```

完整 M0 真實叢集 E2E：

```bash
make demo-image-build
make test-e2e-m0
```

E2E 使用獨立 hugofleet-m0-e2e，驗證三個映像的 HTTP、價格、rollout、Broken readiness／無 liveness restart、Ready endpoints 排除與 v1 恢復；保存 artifacts/m0-e2e/&lt;run&gt; 並清理本次叢集。現有同名叢集會讓測試停止；檢查 ownership 與診斷後才以 `HF_CLUSTER=hugofleet-m0-e2e make kind-down` 重試清理。`m0-down` 固定作用於學習叢集，不用來清理 E2E。

驗收範圍見 [M0 驗收](../../docs/m0-validation.md)。port-forward 驗證選定 Pod 的 HTTP，EndpointSlice 驗證就緒資訊，不等於驗證 kube-proxy／CNI 一般資料路徑。

**讀完深入內容後：**回到[本課完成後的選擇](#完成這一課後去哪裡)，接續下一課、重新練習或結束環境。
