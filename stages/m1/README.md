# M1：讓 Operator 管理應用

先看流程：[M1 逐步動畫](../../docs/animations/m1.html) · [全部動畫章節](../../docs/animations/index.html)。下載專案後，用瀏覽器開啟 HTML 即可離線播放。

目前位置：M1／M0～M5 學習路線 · [上一課 M0](../m0/README.md#完成這一課後去哪裡) · [選課與回訪](../README.md#回來繼續哪一課)

第一次練習從[開始前的準備](#開始前的準備)進入；上次做到一半則先看[回來繼續時先選擇](#回來繼續時先選擇)。

## 這一課的目標與成果

這一課會帶你提交一份 Application 配置，讓 Go Operator 自動建立 Deployment／Service。接著練習更新版本、增加副本、觀察漂移修復、恢復故障版本，最後刪除應用並確認工作負載清理。

M0 直接修改 Deployment；M1 把 **Application 當成期望配置**，由 Operator 持續比對並修正底層資源。這個反覆比對與修正的過程稱為 Reconcile。

先依序完成操作，再看後面的[檔案地圖](#檔案地圖)與[程式導讀](#本階段的程式導讀)。本階段沿用 M0 的價格 API，啟動入口會自行準備所需元件。

完成後，你會看到 Application 建立真實工作負載、Operator 修復副本漂移，以及刪除 CR 後的相依清理。此課尚無 Hub 部署履歷。

依序閱讀：[開始前的準備](#開始前的準備) → [逐步操作](#逐步操作) → [整課驗證與常見問題](#整課驗證與常見問題) → [停止與下次繼續](#停止與下次繼續)。完成後再查[原理與進階閱讀](#原理與進階閱讀)。

## 開始前的準備

### 1. 從專案資料夾進入 WSL

Windows 使用者先啟動 Docker Desktop，確認使用 Linux containers，並啟用 Ubuntu 的 WSL integration。

1. 在檔案總管開啟下載的 `hugofleet` 資料夾，這一層應包含 `Makefile`、`go.mod` 和 `stages`。
2. 在網址列輸入 `powershell`，按 Enter。
3. 在 PowerShell 執行：

```powershell
wsl -d Ubuntu
```

第一次進入時可確認位置：

```bash
pwd
ls Makefile go.mod stages
```

路徑依下載位置而不同；應能列出上述檔案與資料夾。如果位置不符，先回專案資料夾重新開啟，並檢查 Shell 啟動設定是否切換目錄。

**後續命令都在 WSL 裡、專案根目錄下執行。** 原生 Linux amd64 使用者直接在專案根目錄開啟 Bash 即可。環境需求見[根 README](../../README.md#首次準備環境)。

### 2. 檢查工具並結束上一課

若尚未準備本專案工具，先執行：

```bash
make tools
```

這會準備 WSL／Linux 使用的固定 Go、kind、kubectl，放在專案 `.tools`；Windows 安裝的 Go 不能取代這些 Linux 工具。已在 M0 安裝完成就不用重裝。

檢查工具與 Docker：

```bash
make doctor
```

預期印出版本與 Docker 資訊，沒有錯誤。最後顯示的 `hugofleet-a` 是共用命令的預設選擇；`make m1-up` 會自行使用 `hugofleet-m1-demo`。

若 M0 仍在執行，先在原轉送終端按 Ctrl+C，再執行：

```bash
make m0-down
```

本課使用獨立的 M1 學習叢集。M0 的原生 Deployment 沒有 Application owner，Operator 不會接管它；後面操作都使用 M1 專用 context。

### 3. 準備三個終端

| 終端 | 用途 | 是否持續占用 |
|---|---|---|
| A | 啟動、修改 Application、查看狀態與清理 | 命令完成後可繼續輸入 |
| B | 轉送價格 API 到本機 | 轉送期間保持執行 |
| C | 使用 curl 呼叫 API | 呼叫完成後返回 |

目前的終端是 A。需要 B、C 時，從同一個專案資料夾開啟新的 PowerShell，再執行 `wsl -d Ubuntu`。

## 逐步操作

依下列順序切換終端與瀏覽器。每一步先核對預期結果再繼續；命令失敗時，先看[常見問題](#操作遇到問題時)，不要接著執行後續變更。

### 4. 終端 A：啟動 M1

執行：

```bash
make m1-up
```

| 步驟 | 做什麼 |
|---|---|
| 建置映像 | 準備三個價格 API 版本與 Operator 映像 |
| 建立叢集、載入映像 | 準備單節點 `hugofleet-m1-demo` |
| 安裝 CRD | 讓 Kubernetes 接受 Application 資源 |
| 安裝 Operator 與 RBAC | 啟動 Operator，授予管理 `hf-demo` 工作負載所需權限 |
| 套用 Application | 提交 revision 1、v1、一個副本的配置 |
| 等待狀態 | 等待 Operator 處理目前配置，並回報 Ready |

首次下載、建置與啟動可能超過十分鐘，請等待命令結束。成功時最後應看到：

```text
m1 ready; use make m1-forward in another terminal.
```

命令返回後，Operator 與價格 API 都會繼續在叢集中執行。CRD 定義型別，真正持續處理配置的是 Operator 程式。

**下一步：**確認本步結果後，前往[5. 終端 B：轉送服務](#5-終端-b轉送服務)。 若未符合預期，先到[操作排查](#操作遇到問題時)，處理後回到本步重試。

### 5. 終端 B：轉送服務

執行：

```bash
make m1-forward
```

等待畫面出現包含以下內容的訊息：

```text
Forwarding from 127.0.0.1:18080
```

轉送暫時把叢集中的價格 API 接到本機 `18080`，供 curl 測試，沒有建立 Ingress。**保持終端 B 執行**；按 Ctrl+C 只停止連線，Operator 與 API 都會繼續執行。

**下一步：**確認本步結果後，前往[6. 終端 C：確認價格 API 能用](#6-終端-c確認價格-api-能用)。 若未符合預期，先到[操作排查](#操作遇到問題時)，處理後回到本步重試。

### 6. 終端 C：確認價格 API 能用

查版本與就緒狀態：

```bash
curl --fail http://127.0.0.1:18080/version
curl --include --fail http://127.0.0.1:18080/readyz
```

預期版本包含 `v1`，就緒檢查回傳 HTTP `200`。再計算價格：

```bash
curl --fail http://127.0.0.1:18080/quote \
  -H 'Content-Type: application/json' \
  --data '{"amount_cents":20000,"customer_tier":"vip","coupon_code":"SAVE100"}'
```

原價 200 元，VIP 打九折後折抵 100 元，預期回應的 `final_amount_cents` 為 `8000`，即 80 元。完整輸入規則見 [M0 HTTP 契約](../m0/README.md#pricing-api-的-http-契約)。

**下一步：**確認本步結果後，前往[7. 終端 A：查看 Application 與工作負載](#7-終端-a查看-application-與工作負載)。 若未符合預期，先到[操作排查](#操作遇到問題時)，處理後回到本步重試。

### 7. 終端 A：查看 Application 與工作負載

查看你要求的配置，以及 Operator 建立的工作負載：

```bash
make m1-status
```

命令會自動選擇本課叢集，列出 Application、兩邊的期望副本與工作負載。初始預期 revision 為 1、映像為 `pricing-api:v1`、一個價格 API 副本就緒。Operator 自己也有 Deployment／Pod，請分清楚名稱。

Application 是你要求的配置，Operator 依它設定 Deployment 與 Service，Kubernetes 再執行 Pod。後面改需求時，要修改 Application。

<details>
<summary>展開：底層 kubectl 指令與詳細核對</summary>

先設定目前終端的 M1 工具與 kubeconfig：

```bash
export HF_CLUSTER=hugofleet-m1-demo
unset HF_LEARNING_ROOT
source scripts/common.sh
```

`HF_CLUSTER` 選擇 M1 叢集；`unset` 清除可能沿用的自訂學習路徑；`source` 在目前終端載入工具與隔離 kubeconfig。換新的終端操作 kubectl 時，要重新執行這三行。`make m1-up` 內部的設定不會留在父終端。

查看資源：

```bash
kubectl --context kind-hugofleet-m1-demo -n hf-demo \
  get applications,deployments,services,pods
```

| 資源 | 預期結果 |
|---|---|
| Application `pricing-api` | 存在，作為 Operator 的期望配置 |
| Deployment `hugofleet-operator` | READY `1/1`，持續執行控制程式 |
| Deployment `pricing-api` | READY `1/1`，由 Operator 建立 |
| Service `pricing-api` | 存在，供轉送連線 |
| Pod | 一個 Operator Pod、一個就緒的價格 API Pod |

再看 Application：

```bash
kubectl --context kind-hugofleet-m1-demo -n hf-demo \
  get application pricing-api -o yaml
```

先認識接下來會用到的欄位：

| 欄位 | 誰寫入 | 怎麼讀 |
|---|---|---|
| `spec.revision` | 你 | 本次提交的配置版本，本課依序用 1、2、3、4 |
| `spec.image`、`spec.replicas` | 你 | 期望映像與副本數 |
| `metadata.generation` | Kubernetes | spec 變更的代數，與 revision 是不同欄位 |
| `status.observedGeneration` | Operator | 已處理到哪一代 Application |
| `status.observedRevision`、`status.image` | Operator | 底層 Deployment controller 已觀測的配置，不代表已就緒 |
| `status.readyReplicas`、`status.availableReplicas` | Operator | 目前就緒與可用副本數 |
| `status.conditions` | Operator | Ready、Progressing、Degraded 及原因，每個 Condition 都有對應 generation |

初始預期 revision 為 1、image 為 `pricing-api:v1`、一個副本就緒，Ready 為 True。`observedGeneration` 應對應目前 `metadata.generation`。

</details>

**下一步：**確認本步結果後，前往[8. 終端 A：更新健康版本與副本數](#8-終端-a更新健康版本與副本數)。 若未符合預期，先到[操作排查](#操作遇到問題時)，處理後回到本步重試。

### 8. 終端 A：更新健康版本與副本數

這次正式改需求：「使用健康新版 v2-good，並執行兩份價格 API。」先在終端 B 按 Ctrl+C 停止轉送，終端 A 執行：

```bash
make m1-update-good
```

命令修改 **Application**，提交 revision 2，等待 Operator 處理目前配置並回報 Ready，最後列出配置與 Pod。預期目前 generation 已被觀測、observedRevision 為 2、映像為 `pricing-api:v2-good`，價格 API Deployment READY 為 `2/2`。

終端 B 執行 `make m1-forward`，終端 C 用 `curl --fail http://127.0.0.1:18080/version` 確認為 `v2-good`。每次換版後都要重建轉送。

本課短命令自動選擇 M1 練習叢集與專案工具，依序使用 revision 1 → 2 → 3 → 4。revision 表示提交配置的編號；不符合步驟時命令會停止。如果配置已提交但等待逾時，先用 `make m1-status` 查原因，不必重送。自行修改過 revision 時，使用下方完整指令並選新的 revision。

<details>
<summary>展開：底層 kubectl 指令與詳細核對</summary>

本課以下命令假設依序操作、沒有插入其他 Application 修改。如果已自行練習，先讀取目前 revision，再選新的 revision。

先在終端 B 按 Ctrl+C 停止轉送，再在終端 A 提交新版配置：

```bash
kubectl --context kind-hugofleet-m1-demo -n hf-demo \
  patch application pricing-api --type=merge \
  -p '{"spec":{"revision":2,"image":"pricing-api:v2-good","replicas":2}}'
```

這次提交 revision 2、健康新版、兩個副本。**日常更新修改 Application，Operator 會依它更新 Deployment／Service。**

修改命令成功只代表配置已接受，接著等待 Operator 處理這一代配置：

```bash
generation="$(kubectl --context kind-hugofleet-m1-demo -n hf-demo \
  get application pricing-api -o jsonpath='{.metadata.generation}')"
kubectl --context kind-hugofleet-m1-demo -n hf-demo wait \
  --for="jsonpath={.status.observedGeneration}=$generation" \
  application/pricing-api --timeout=180s
```

`generation` 是從 Kubernetes 讀取的目前代數，不是把 revision 2 直接當作 generation。再等待應用就緒：

```bash
kubectl --context kind-hugofleet-m1-demo -n hf-demo wait \
  --for=condition=Ready application/pricing-api --timeout=180s
```

兩個 wait 命令預期顯示 `condition met`。它們只等待狀態，不會修改配置；逾時不會停止 Operator 繼續處理，請先查原因再往下。

核對實際結果：

```bash
kubectl --context kind-hugofleet-m1-demo -n hf-demo \
  get application pricing-api -o yaml
kubectl --context kind-hugofleet-m1-demo -n hf-demo \
  get deployments,pods
```

確認以下項目，再繼續：

- Application 的 `status.observedGeneration` 及 Ready Condition 的 `observedGeneration` 對應目前 `metadata.generation`。
- `status.observedRevision` 為 2，`status.image` 為 `pricing-api:v2-good`，Ready 為 True。
- 價格 API Deployment READY 為 `2/2`，兩個新版 Pod 就緒，舊版已退出。

只看到 Ready=True 可能是更新前暫留的狀態，因此要一起核對 generation、revision 與副本。

終端 B 重新執行 `make m1-forward`，終端 C 查版本：

```bash
curl --fail http://127.0.0.1:18080/version
```

預期變成 `v2-good`。port-forward 連著特定 Pod，健康更新或恢復後需重建轉送。

</details>

**下一步：**確認本步結果後，前往[9. 終端 A：觀察漂移修復](#9-終端-a觀察漂移修復)。 若未符合預期，先到[操作排查](#操作遇到問題時)，處理後回到本步重試。

### 9. 終端 A：觀察漂移修復

前一步的 Application 要求兩份服務。這次故意只把底下的 Deployment 改成一份，觀察 Operator 是否依 Application 修回來：

```bash
make m1-drift
make m1-observe
```

| 時間點 | Application 要求 | Deployment 設定 |
|---|---:|---:|
| 修改前 | 2 | 2 |
| 刻意改動後 | 2 | 1 |
| Operator 修復後 | 2 | 2 |

兩邊配置不一致就叫「漂移」。這一步驗證 Operator 會持續維持要求。修復可能很快，看不到中間的 1 也正常；最後價格 API READY 應回到 `2/2`。

觀察命令持續執行；按 Ctrl+C 只停止觀察。接著執行：

```bash
make m1-status
```

輸出的 Application 與 Deployment 期望副本都應為 2，revision 仍為 2。若真的要長期改成一份，應修改 Application。

<details>
<summary>展開：底層 kubectl 指令與詳細核對</summary>

目前 Application 要求兩個副本。這次刻意直接把底層 Deployment 改成一個：

```bash
kubectl --context kind-hugofleet-m1-demo -n hf-demo \
  patch deployment pricing-api --type=merge -p '{"spec":{"replicas":1}}'
```

開始觀察：

```bash
kubectl --context kind-hugofleet-m1-demo -n hf-demo \
  get deployment pricing-api -w
```

Operator 應把 Deployment 的期望副本修復為 2，最終 READY 回到 `2/2`。修復可能很快，開始 watch 時不一定看得到中間的一個副本狀態。

觀察完按 Ctrl+C，只會停止 watch。再核對兩邊的期望副本：

```bash
kubectl --context kind-hugofleet-m1-demo -n hf-demo \
  get application pricing-api -o jsonpath='{.spec.replicas}{"\n"}'
kubectl --context kind-hugofleet-m1-demo -n hf-demo \
  get deployment pricing-api -o jsonpath='{.spec.replicas}{"\n"}'
```

兩者都應為 `2`。Application 沒有改，這次修復不會新增 revision。

M0 的 Deployment controller 會維持 Deployment 自己的副本設定；M1 又多了一層 Operator，依 Application 修正 Deployment 的配置。若希望長期改成一個副本，應修改 Application。

</details>

**下一步：**確認本步結果後，前往[10. 終端 A：部署故障版本](#10-終端-a部署故障版本)。 若未符合預期，先到[操作排查](#操作遇到問題時)，處理後回到本步重試。

### 10. 終端 A：部署故障版本

這次讓 Application 明確要求故障版，觀察 Operator 如何回報部署問題：

```bash
make m1-break
```

命令提交 revision 3、故障版 v2-broken、兩個副本，等待 Operator 回報 **Degraded（降級）**，並列出狀態與事件。這裡等待成功表示「已觀測到異常」。

預期目前 generation 的 Ready=False、Degraded=True，原因包含 `ProgressDeadlineExceeded`；故障 Pod 為 Running，但未 Ready，因為 `/readyz` 固定回 503。等待逾時時先查看 `make m1-status` 與下方事件命令，不可直接認定驗證成功。

這次 Application 本身就要求壞版，Operator 會照要求部署並回報失敗；恢復版本由下一步指定。

<details>
<summary>展開：底層 kubectl 指令與詳細核對</summary>

提交 revision 3，改用故障映像：

```bash
kubectl --context kind-hugofleet-m1-demo -n hf-demo \
  patch application pricing-api --type=merge \
  -p '{"spec":{"revision":3,"image":"pricing-api:v2-broken"}}'
```

這個版本的程序仍存活，但 `/readyz` 固定回 HTTP 503。等待 Operator 回報降級：

```bash
kubectl --context kind-hugofleet-m1-demo -n hf-demo wait \
  --for=condition=Degraded application/pricing-api --timeout=180s
```

**這裡預期 wait 成功**，因為等的是 Degraded=True，代表已觀測到部署失敗；不是等成功就緒。若等待逾時，先看下方狀態與事件，不能只憑逾時認定已得到預期結果。

```bash
kubectl --context kind-hugofleet-m1-demo -n hf-demo \
  get application pricing-api -o yaml
kubectl --context kind-hugofleet-m1-demo -n hf-demo \
  get deployment pricing-api -o yaml
kubectl --context kind-hugofleet-m1-demo -n hf-demo get pods -o wide
kubectl --context kind-hugofleet-m1-demo -n hf-demo \
  get events --sort-by=.lastTimestamp
```

預期目前 generation 的 Ready=False、Degraded=True，原因包含 `ProgressDeadlineExceeded`；故障 Pod 顯示 Running，但未 Ready。`observedRevision=3` 表示配置已被觀測，並不代表部署成功。

資源足夠時，先前的健康 Pod 繼續服務，故障 Pod 不會成為 Service 的就緒端點。

</details>

**下一步：**確認本步結果後，前往[11. 終端 A：用新 revision 恢復 v1](#11-終端-a用新-revision-恢復-v1)。 若未符合預期，先到[操作排查](#操作遇到問題時)，處理後回到本步重試。

### 11. 終端 A：用新 revision 恢復 v1

由你提交一份新配置，把映像恢復為健康的 v1：

```bash
make m1-restore
```

命令提交 revision 4、v1、兩個副本，等待目前配置 Ready。預期 observedRevision 為 4、映像為 `pricing-api:v1`，價格 API READY 為 `2/2`，故障 Pod 最終退出。

終端 B 按 Ctrl+C，再執行 `make m1-forward`；終端 C 驗證：

```bash
curl --fail http://127.0.0.1:18080/version
curl --include --fail http://127.0.0.1:18080/readyz
```

預期為 v1、HTTP 200。再執行第 6 節價格請求得到 8000。revision 是提交的編號，因此恢復 v1 仍使用新的 revision 4。

<details>
<summary>展開：底層 kubectl 指令與詳細核對</summary>

恢復時提交新的 revision 4，指定 v1，保留兩個副本：

```bash
kubectl --context kind-hugofleet-m1-demo -n hf-demo \
  patch application pricing-api --type=merge \
  -p '{"spec":{"revision":4,"image":"pricing-api:v1","replicas":2}}'
```

等待目前 generation 被處理並就緒：

```bash
generation="$(kubectl --context kind-hugofleet-m1-demo -n hf-demo \
  get application pricing-api -o jsonpath='{.metadata.generation}')"
kubectl --context kind-hugofleet-m1-demo -n hf-demo wait \
  --for="jsonpath={.status.observedGeneration}=$generation" \
  application/pricing-api --timeout=180s
kubectl --context kind-hugofleet-m1-demo -n hf-demo wait \
  --for=condition=Ready application/pricing-api --timeout=180s
```

確認結果：

```bash
kubectl --context kind-hugofleet-m1-demo -n hf-demo \
  get application pricing-api -o yaml
kubectl --context kind-hugofleet-m1-demo -n hf-demo get deployments,pods
```

預期目前 generation 的 Ready=True，observedRevision=4、image=`pricing-api:v1`，價格 API 的兩個副本都就緒。確認故障 Pod 與舊版退出後，終端 B 按 Ctrl+C，再執行 `make m1-forward`。

終端 C 驗證：

```bash
curl --fail http://127.0.0.1:18080/version
curl --include --fail http://127.0.0.1:18080/readyz
```

預期為 v1、HTTP 200；重新執行第 6 節價格請求，最終金額仍為 8000。

M1 的恢復是手動提交配置，沒有保存完整成功快照或提供 Hub 回滾介面。revision 會寫入 Pod template，新 revision 即使配置相同也會觸發獨立 rollout；同 revision 重送維持冪等。原因見 [ADR 0012](../../docs/adr/0012-current-revision-replicaset-evidence.md)。

</details>

**下一步：**確認本步結果後，前往[12. 終端 A：刪除 Application，觀察相依資源清理](#12-終端-a刪除-application觀察相依資源清理)。 若未符合預期，先到[操作排查](#操作遇到問題時)，處理後回到本步重試。

### 12. 終端 A：刪除 Application，觀察相依資源清理

最後練習移除整個價格 API。先在終端 B 按 Ctrl+C 停止轉送，再執行：

```bash
make m1-delete
```

命令刪除本課 pricing-api Application，等待 Kubernetes 清理它擁有的 Deployment 與 Service，最後列出剩餘資源。價格 API 的 ReplicaSet／Pod 也應最終消失；若還在 Terminating，稍候再查。

Operator 自己的 Deployment／Pod 保留是正常的：管理程式與被管理的價格 API 是兩個不同程式。重新建立價格 API 可執行 `make m1-up`，從 revision 1 開始。

<details>
<summary>展開：底層 kubectl 指令與詳細核對</summary>

先在終端 B 按 Ctrl+C 停止轉送，再刪除 Application：

```bash
kubectl --context kind-hugofleet-m1-demo -n hf-demo \
  delete application pricing-api
```

Application 是工作負載的 owner。刪除後，Kubernetes 垃圾回收（GC）會非同步清理相依資源。等待 Deployment 與 Service 消失：

```bash
kubectl --context kind-hugofleet-m1-demo -n hf-demo wait \
  --for=delete deployment/pricing-api --timeout=180s
kubectl --context kind-hugofleet-m1-demo -n hf-demo wait \
  --for=delete service/pricing-api --timeout=180s
```

查看剩下的資源：

```bash
kubectl --context kind-hugofleet-m1-demo -n hf-demo \
  get applications,deployments,services,replicasets,pods
```

確認 pricing-api 的 Application、Deployment、Service、ReplicaSet、Pod 最終都消失；若 Pod 尚在 Terminating，等待後再查。**Operator 的 Deployment／Pod 繼續存在是正常的**，它與被管理的價格 API 是不同程式。

</details>

**下一步：**前往[整課驗證](#整課驗證與常見問題)，核對這一輪的結果。 若未符合預期，先到[操作排查](#操作遇到問題時)，處理後回到本步重試。

## 整課驗證與常見問題

完成主線後，核對以下結果；若有一項未達成，先排查再繼續。

- Application 產生 Deployment／Service，價格 API 回傳 8000。
- 底層 Deployment 被縮為一個副本後，Operator 恢復兩個副本。
- 新 revision 恢復 v1，Ready 對應目前 generation；刪除 CR 後相依工作負載清除。

這些是本輪操作的檢查點，不是新的驗收證據。自動化測試與既有結果見 [M1 驗收](../../docs/m1-validation.md)。


**核對完成後：**若已達成本課成果，前往[停止與下次繼續](#停止與下次繼續)。想繼續進階實作時先保留環境，從[原理與進階閱讀](#原理與進階閱讀)選擇練習；純閱讀不需保持服務運作。尚有問題則先用下方排查，修正後回到失敗步驟。
### 操作遇到問題時

在新的查詢終端：

```bash
export HF_CLUSTER=hugofleet-m1-demo
unset HF_LEARNING_ROOT
source scripts/common.sh
kubectl --context kind-hugofleet-m1-demo -n hf-demo get application pricing-api -o yaml
kubectl --context kind-hugofleet-m1-demo -n hf-demo get deployments,services,pods
kubectl --context kind-hugofleet-m1-demo -n hf-demo get events --sort-by=.lastTimestamp
kubectl --context kind-hugofleet-m1-demo -n hf-demo logs deployment/hugofleet-operator --tail=100
```

| 現象 | 怎麼判讀與處理 |
|---|---|
| no matches for kind Application | CRD 尚未安裝／Established，或查錯叢集；先核對 context 與 operator-install |
| schema／quantity 驗證拒絕 | 查 image、replicas、resources、probe path；API server 拒絕的配置尚未進入 Reconcile |
| CR 已更新但 Deployment 未變 | 查 Operator Pod／日誌與當前 observedGeneration；確認沒有把 Operator 縮到 0 |
| OwnershipConflict | 同名資源的 controller owner UID 不匹配；不以刪除外來資源解決，回專用 M1 環境 |
| Ready 仍是 True 但 /version 是舊版 | 先核對目前 generation／revision／所有副本，再重建 port-forward |
| ProgressDeadlineExceeded | 新 Pod 無法完成 rollout；壞版練習是預期，其他情況查探針／events／資源 |
| 刪除 CR 後仍有 Pod | GC 非同步；等待被管理的 pricing-api 消失，Operator Pod 應繼續存在 |
| 修改 YAML 後沒變 | YAML 只是本機檔案，要 apply；直接 patch 不會同步改 YAML |

反覆閱讀／呼叫不用重跑 up。`make m1-up` 會重新 apply application.yaml，預設把配置帶回 rev 1／v1／1 副本；保留練習進度時只查狀態、重新 forward。M1 本身沒有 Hub／Agent 的 revision 持久去重防護，本課每次更新採用新 revision 是操作慣例，不把它說成 M3 ledger 的舊命令拒絕。

修改 CR 型別時，先改 api/v1alpha1 中的 Go／markers，再於專案根目錄用 `make generate manifests` 更新生成檔。修改 controller Go 後須重建 Operator 映像、重新安裝／重啟才會作用於 Pod；`make m1-build` 單獨產生的 bin/operator 不會自動替換叢集中的映像。驗證命令與測試範圍見 [驗證總覽](../../docs/validation.md)。

## 停止與下次繼續

### 回來繼續時先選擇

先依原環境的情況選一條路；下方停止與重建命令不是需要依序全部執行的清單。

| 目前情況 | 下一步 |
|---|---|
| 原叢集與應用仍正常，只停了轉送 | 重新執行 `make m1-forward`，再回[逐步操作](#逐步操作)確認 API；不用重跑 up。 |
| 已 down 或應用已刪除 | 依下方結束與重建說明，從初始狀態重新練習。 |
| Docker／WSL 曾重啟，狀態不確定 | 先看[操作排查](#操作遇到問題時)，不要假設 RAM etcd 的資料還在。 |

### 13. 結束練習與下次使用

停止所有 watch／port-forward 後，執行：

```bash
make m1-down
```

這會刪除核對 ownership 的 M1 學習叢集。原始碼、工具、主機 Docker 映像與測試產物保留。切換到 [M2](../m2/README.md) 前先完成清理。

下次從初始狀態練習，執行 `make m1-up`。它會重新套用 application.yaml，預設回到 revision 1、v1、一個副本。

若只停止轉送、仍保有正常運作的叢集與應用，重新執行 `make m1-forward` 即可。若已完成第 12 節刪除，可等 GC 完成後重新 `make m1-up` 建立應用。

預設 RAM etcd 不保證 Docker 節點／WSL 重啟後的叢集資料；先檢查，必要時用 `make m1-down` 限定清理，再啟動。

### 完成這一課後去哪裡

- **進入下一課：**先停止本課的 watch／轉送，以原階段的環境執行 `make m1-down`；完成後前往 [M2：Hub 發版與履歷](../m2/README.md#開始前的準備)。
- **下次繼續／重新練習：**回到[本輪環境選擇](#回來繼續時先選擇)，依原叢集與資料是否仍有效決定恢復方式。
- **深入本課：**閱讀[原理與進階閱讀](#原理與進階閱讀)。要做需要叢集的進階演練，先保留本輪環境；完整 down 後須依重建流程重新準備。

## 原理與進階閱讀

以下依需要查閱；進階演練若需要仍在執行的環境，請在完整 down 前操作。開發測試入口不屬於首次教學的必做步驟。

### 基本名詞

| 名稱 | 本課中的意思 |
|---|---|
| CRD | 讓 Kubernetes 認得 Application 這種資源的型別定義 |
| Application CR | 你建立的一個 Application 實例，例如 pricing-api |
| spec | 你希望使用的映像、副本數與資源配置 |
| status | Operator 觀察到的部署進度與結果 |
| Operator | 持續讀取 Application，建立、更新與修復 Deployment／Service 的程式 |

### Application 欄位與限制

本課的 [application.yaml](application.yaml) 包含 image、revision、replicas、port、resources 與探針路徑。revision、image、CPU／Memory requests／limits 必填；replicas 預設 1、範圍 1～3；port 預設 8080；探針預設 `/readyz`、`/healthz`。資源數量須為正且 limit ≥ request，探針只能是本機絕對 path，不含 host／query。映像格式接受小寫 repository、可選 registry port／tag／sha256 digest。

CR 的 `metadata.uid` 由 Kubernetes 產生；同名刪除重建會取得新 UID。Operator 核對 controller owner UID，拒絕接管外來同名資源，並保留不屬於自己的 metadata 與既有 Service ClusterIP。

### 檔案地圖

| 位置 | 裡面做什麼 | 怎麼使用／修改 |
|---|---|---|
| [run.sh](run.sh)／[scripts/control.sh](scripts/control.sh) | 學習入口與展開操作 | up／forward／down；control 負責安裝與生成 |
| [application.yaml](application.yaml) | 本課提交給 Operator 的期望配置 | 可改範例；apply 才影響 CR，檔案不存執行 status |
| [api/v1alpha1](api/v1alpha1/) | Go spec／status、驗證、Scheme、DeepCopy | 改型別與 markers；DeepCopy 是工具產物 |
| [controller](controller/README.md) | Reconcile、工作負載投影與測試 | 改控制流程從這裡讀 |
| [cmd/operator/main.go](cmd/operator/main.go) | Manager、Scheme、Controller 組裝 | Operator 執行入口 |
| [config/crd/bases](config/crd/bases/) | Kubernetes 可接受的 Application schema | 由 controller-gen 產生；改 API markers 後生成 |
| [config/rbac](config/rbac/) | Operator 的 namespace Role | 由程式 RBAC markers 產生 |
| [config/operator/install.yaml](config/operator/install.yaml) | Namespace、SA／Binding、Operator Deployment | 手寫安裝模板；不是被管理的 pricing-api |
| [build/operator.Dockerfile](build/operator.Dockerfile) | Operator 映像 | 修改 Go 後重建，再載入／重啟 Operator |
| [scripts/tools.sh](scripts/tools.sh) | controller-gen、setup-envtest 等工具 | 開發 CRD／integration 時使用 |
| [scripts/e2e.sh](scripts/e2e.sh)／[test/e2e](test/e2e/) | 真實 rollout、漂移、GC、權限驗收 | 使用獨立測試環境；測試範圍見階段驗收文件 |

M1 新增的是 CRD 與 Operator；實際業務服務仍是 [M0 pricing-api](../m0/pricing-api/README.md)。共用 kind／工具從 scripts 與 deploy 取得；執行產物在 `.local/tutorials/m1/`。

### 本階段的程式導讀

M1 新增 Application API 與 Go Operator，沿用 [M0 pricing-api](../m0/README.md)。獨立入口會準備 Operator 與 Application，不用先執行 M0。

| 閱讀順序 | 檔案 | 要理解的事情 |
|---|---|---|
| 1 | [application_types.go](api/v1alpha1/application_types.go) | spec／status、revision、condition 與 CRD markers |
| 2 | [validation.go](api/v1alpha1/validation.go) | 允許的 image、資源與欄位限制 |
| 3 | [operator/main.go](cmd/operator/main.go) | 註冊 Scheme、建立 Manager、啟動 Controller |
| 4 | [application_controller.go](controller/application_controller.go) | Reconcile、ownership、狀態、更新與漂移修復 |
| 5 | [workload.go](controller/workload.go) | 從 Application 產生 Deployment／Service 配置 |
| 6 | [envtest_test.go](controller/envtest_test.go) | 真實 API server／etcd 的 schema、status 與控制器測試 |
| 7 | [m1_test.go](test/e2e/m1_test.go)、[rbac_test.go](test/e2e/rbac_test.go) | 真實 Pod、rollout、GC、最小權限驗收 |

[config/crd](config/crd/bases) 與 [config/rbac](config/rbac) 由 controller-gen 產生；[config/operator/install.yaml](config/operator/install.yaml) 是安裝設定；[build/operator.Dockerfile](build/operator.Dockerfile) 是映像。教學及驗收的範例 CR 使用本資料夾的 [application.yaml](application.yaml)。

單階段開發可用 make m1-build／m1-test，integration 與完整驗收見「測試與驗收」。這些與首次 up 是不同用途，驗收範圍見 [驗證總覽](../../docs/validation.md)。

M2／M3 直接沿用這個 Operator 與 CRD，沒有另一套 Reconcile。API／controller 不放在 Go 的 internal 目錄，讓跨階段套件能合法共用。讀完後應能追出「改 Application → Reconcile → 工作負載 → status」，並理解為何手動改 Deployment 會被修復，再往 [M2](../m2/README.md) 走。逐函式說明、ownership 與測試邊界見 [Controller 深入導讀](controller/README.md)。

### 進階：觀察 Service 自動重建

在第 9 節之後、刪除 Application 之前，可以刻意刪除自己的 Demo Service：

```bash
kubectl --context kind-hugofleet-m1-demo -n hf-demo delete service pricing-api
kubectl --context kind-hugofleet-m1-demo -n hf-demo get services -w
```

Operator 會依仍存在的 Application 重建 Service。觀察後按 Ctrl+C。新 Service 的 UID／ClusterIP 可能不同；「保留 ClusterIP」指更新既有 Service 時，並不保證刪除重建後 IP 不變。

Deployment 被刪除後也會重建，但會中斷該工作負載；日常修改以 Application 為準。

### 進階：暫停與恢復 Operator

若要比較 Operator 停止前後的行為，在 Application 仍存在時執行：

```bash
kubectl --context kind-hugofleet-m1-demo -n hf-demo \
  scale deployment/hugofleet-operator --replicas=0
```

既有價格 API 仍由 Kubernetes 執行，但 Application 新配置與底層漂移暫時不由 Operator 收斂。恢復：

```bash
kubectl --context kind-hugofleet-m1-demo -n hf-demo \
  scale deployment/hugofleet-operator --replicas=1
kubectl --context kind-hugofleet-m1-demo -n hf-demo \
  rollout status deployment/hugofleet-operator --timeout=180s
```

M1 使用單實例、沒有 leader election／HA。Operator SA 不可讀 Secrets、exec、建立 Namespace 或寫其他 Namespace；租戶登入與授權見 M4。

### 進階：拆開啟動步驟

這是 `make m1-up` 的替代路徑，第一次學習可略過；已啟動就不需再跑一遍。

```bash
export HF_CLUSTER=hugofleet-m1-demo
unset HF_LEARNING_ROOT
source scripts/common.sh
make demo-image-build
make operator-image-build
make kind-up
make demo-image-load
make operator-install
kubectl --context kind-hugofleet-m1-demo apply -f stages/m1/application.yaml
generation="$(kubectl --context kind-hugofleet-m1-demo -n hf-demo \
  get application pricing-api -o jsonpath='{.metadata.generation}')"
kubectl --context kind-hugofleet-m1-demo -n hf-demo wait \
  --for="jsonpath={.status.observedGeneration}=$generation" \
  application/pricing-api --timeout=180s
kubectl --context kind-hugofleet-m1-demo -n hf-demo wait \
  --for=condition=Ready application/pricing-api --timeout=180s
```

| 命令 | 用途 |
|---|---|
| `demo-image-build` | 建置三個價格 API 映像 |
| `operator-image-build` | 建置 Operator 映像 |
| `kind-up` | 建立或檢查 HF_CLUSTER 指定叢集 |
| `demo-image-load` | 將價格 API 映像載入 kind |
| `operator-install` | 載入 Operator 映像、安裝 CRD／RBAC／Operator 並等待啟動 |
| `kubectl apply` | 提交 Application，讓 Operator 建立工作負載 |

安裝 CRD 是叢集管理者的準備工作。學習 up 不要求先執行 `operator-tools`；該命令是修改 API 生成檔或做整合測試時使用。

### 測試與驗收

以下是開發驗證選項，不是第一次學習的前置；驗收範圍見階段驗收文件。只測 M1 可用 make m1-test，下方全專案命令範圍較大。

進入 M2 前，先完成理解檢查：

| 問題 | 用本階段操作核對 |
|---|---|
| spec 與 status 誰更新？ | 你提交 spec；Operator 回報 observedGeneration、revision 與 Conditions |
| 為何手改 Deployment 會被修復？ | Application spec 仍是 Operator 的期望配置 |
| Operator 停止後會怎樣？ | 既有服務可繼續；Application 的新配置與漂移暫時不由 Operator 收斂 |
| 刪除 CR 為何會刪除工作負載？ | ownerReferences 讓 Kubernetes GC 清理相依資源 |

能說明結果後，再前往 [M2](../m2/README.md)。

先執行 M1 單元測試：

```bash
make m1-test
```

需要生成檔、全專案與整合檢查時，再依序執行：

```bash
make operator-tools
make envtest
make lint build test-unit verify-generated
make test-race
make test-integration
make demo-image-build operator-image-build
make test-e2e-m1
```

上方全專案 Unit／race 使用目前 repo，包含 M0～M5 packages；m1-test 才只測 M1。envtest 使用真實 etcd／API server，驗證 schema／CEL／defaults／CRUD／ownership／metadata／ClusterIP 與 no-op；沒有 kubelet／Deployment controller／GC，不能宣稱 Pod Ready 或完整刪除成功。

kind E2E 使用獨立 hugofleet-m1-e2e，驗證更新／縮放、漂移與刪除重建、Operator 程序重啟、故障／恢復、真實 GC、價格 HTTP 與 namespace RBAC。保存 artifacts/m1-e2e/&lt;run&gt;，並清理本次叢集；同名既有叢集須先檢查，再用 `HF_CLUSTER=hugofleet-m1-e2e make kind-down` 限定清理。`m1-down` 固定作用於學習叢集。

驗收範圍與既有證據見 [M1 驗收](../../docs/m1-validation.md)。

**讀完深入內容後：**回到[本課完成後的選擇](#完成這一課後去哪裡)，接續下一課、重新練習或結束環境。
