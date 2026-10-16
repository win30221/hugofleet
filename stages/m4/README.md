# M4：OIDC、租戶權限與真實隔離

先看流程：[M4 逐步動畫](../../docs/animations/m4.html) · [全部動畫章節](../../docs/animations/index.html)。下載專案後，用瀏覽器開啟 HTML 即可離線播放。

目前位置：M4／M0～M5 學習路線 · [上一課 M3](../m3/README.md#完成這一課後去哪裡) · [選課與回訪](../README.md#回來繼續哪一課)

第一次練習從[開始前的準備](#開始前的準備)進入；上次做到一半則先看[回來繼續時先選擇](#回來繼續時先選擇)。

## 這一課的目標與成果

這一課會帶你用真實 Dex 帳號登入，啟用兩個租戶、授予角色，再部署價格 API，觀察不同角色與跨租戶操作的拒絕結果。

M4 沿用 Hub／SQLite 與 Worker 主動 mTLS 通道，新增登入 session、DB membership、租戶 Namespace、Quota 與 NetworkPolicy。先完成下方操作，再查後面的 API／程式與驗收資料。

| 操作順序 | 在哪裡 | 看到什麼才繼續 |
|---|---|---|
| 啟動 M4 | 終端 A | 顯示兩個真實 Worker 已連線，命令返回 |
| 找本輪帳號、登入 admin | 本地編輯器／瀏覽器 | 可看到 A／B 心跳與已驗證 User |
| 啟用 team-a／team-b 的 A／B profile | 瀏覽器 admin | 租戶與 Worker profile 啟用成功 |
| 授予 Developer／Viewer | 瀏覽器 owner-a | 成員角色已列出 |
| 建立 pricing-api | 瀏覽器 developer-a | SUCCEEDED／READY |
| 轉送並呼叫 | 終端 B／C | v1、價格 8000 |
| 練習權限與更新／回滾 | 瀏覽器 | 拒絕有原因，恢復產生新 revision |
| 結束 | 停轉送後 make m4-down | owned 環境移除，私有 state 保留 |

完成後，你會看到成功部署與回滾，也能以不同帳號核對跨租戶拒絕與角色限制。配額 Admission 與封包隔離的完整驗收另由 E2E 執行。

依序閱讀：[開始前的準備](#開始前的準備) → [逐步操作](#逐步操作) → [整課驗證與常見問題](#整課驗證與常見問題) → [停止與下次繼續](#停止與下次繼續)。完成後再查[原理與進階閱讀](#原理與進階閱讀)。

## 開始前的準備

### 從主目錄開始

Windows 從專案資料夾開 PowerShell，執行 `wsl -d Ubuntu`。後續 Bash 命令都在專案根目錄，以非 root 的 Linux 使用者執行；詳見[環境準備](../../README.md#首次準備環境)。尚未安裝專案工具才執行 `make tools`，已完成 M0～M3 者不用重裝。開始前檢查：

```bash
make doctor
```

M4 使用 A／B 兩個 Worker；A 為 Calico Security Profile，B 使用 kindnet。兩個租戶 team-a／team-b 在兩個 Worker 各有獨立 namespace；每 namespace 有自己的受限 Operator，Agent 位於 hf-system。M3／M4 共用預設 Worker 名稱，因此先用原階段、原 state 的 `down` 清理上一課。腳本核對精確 Container ID，拒絕接管其他階段或 unrelated lab。

### 選擇本輪資料目錄

首次使用可保留預設 `.local/tutorials/m4`。若要自訂，在第一次啟動前、尚未建立本輪環境時執行以下設定；兩種方式擇一：

```bash
export HF_M4_STATE="$PWD/.local/tutorials/m4/my-lesson"
```

設定後繼續下方 `make m4-up`，不需額外執行另一個 up。若已有環境，先以原 state 完成 down，才能切換到新的資料目錄。每個終端都沿用本輪相同 state。

### 終端分工

| 終端／介面 | 用途 |
|---|---|
| A | 啟動、status 與結束；up 完成後可繼續輸入命令 |
| B | 轉送價格 API，保持執行 |
| C | curl 呼叫、需要時查詢 Worker |
| 瀏覽器 | 登入、啟用租戶、授權與部署 |

需要新終端時，從同一個專案資料夾開 PowerShell，再執行 `wsl -d Ubuntu`。

## 逐步操作

依下列順序切換終端與瀏覽器。每一步先核對預期結果再繼續；命令失敗時，先看[常見問題](#操作遇到問題時)，不要接著執行後續變更。

### 終端 A：啟動

```bash
make m4-up
```

此入口會準備雙 Worker、登入服務與本輪帳號。完成後應返回命令列，且顯示兩個 Worker 已連線；此時尚未授予租戶角色或部署應用。

首次下載映像與 Docker Desktop 的磁碟 flush 可能花數十分鐘。Calico 三個固定版本映像先存於主機 Docker cache，再載入 owned A Worker；重建叢集會沿用此快取，避免在節點內重複下載。每個 bootstrap log 在 `artifacts/bootstrap-<cluster>/`；Calico readiness 有有界等待，逾時後可對同一 state 重跑入口。版本、CNI 或 ownership 不符會直接拒絕；不要刪除 ownership 檔案強制繞過。

預期顯示兩個真實 Worker 已連線。管理 UI 是 `http://127.0.0.1:18094/login`；Dex issuer 是 `http://127.0.0.1:5556/dex`。HTTP 是明確的 loopback LocalDemo 例外；正常部署須設定 HTTPS，session cookie 會設 Secure。

**下一步：**確認本步結果後，前往[瀏覽器：取得本輪帳號並登入](#瀏覽器取得本輪帳號並登入)。 若未符合預期，先到[操作排查](#操作遇到問題時)，處理後回到本步重試。

### 瀏覽器：取得本輪帳號並登入

整課的 up／status／hub-stop／hub-up／hub-recreate／down 必須使用同一 state。`.local` 被 Git 忽略；`accounts.json` 含隨機密碼與 Client Secret，`subjects.json` 保存實際驗證的 subject，`certs/` 含本機憑證與私鑰，`db/hub.db` 為 SQLite。不要貼到 issue、截圖、影片或驗收日誌。既有帳號不會因再次 up 而被靜默換掉。每個新終端先 export 同一 state，再 `source stages/m4/env.sh a`（或 b），才直接使用 kubectl／bootstrap。

預設使用本輪 `.local/tutorials/m4/accounts.json`；自訂 state 則使用該資料夾的同名檔案。在自己的本地編輯器打開它，選擇單一帳號的 email／password 登入 Dex：admin、owner-a、developer-a、owner-b、viewer-a、outsider。Platform Admin 僅綁定已驗證的 admin subject；所有帳號故意帶有相同 elevated groups，Hub 仍只信 DB 角色。帳號與密碼僅為本機 Demo，不代表企業 IAM。

**下一步：**確認本步結果後，前往[瀏覽器：啟用租戶與角色](#瀏覽器啟用租戶與角色)。 若未符合預期，先到[操作排查](#操作遇到問題時)，處理後回到本步重試。

### 瀏覽器：啟用租戶與角色

1. 使用 admin 登入。首頁列出真實 A／B Agent 心跳與版本，以及已登入的 User。
2. 在「啟用已 bootstrap 的租戶」填 slug/name `team-a`、選 Cluster A、Owner 選 owner-a。Hub 向 Agent 核對實際 namespace、receipt UID、Quota／LimitRange，再啟用資料庫 profile。用相同 name／Owner 啟用 team-a 的 B profile。
3. 同樣以 owner-b 啟用 team-b 的 A／B profile。未 bootstrap 或離線 Worker 不能靠 DB row 偽裝啟用成功。
4. 登出 admin，登入 owner-a；在 team-a 成員頁，將 developer-a 對應的已驗證 User ID 授予 Developer，再將 viewer-a 對應的 User ID 授予 Viewer。User ID 從 admin 的已驗證 User 清單或本人頁面取得，email 只是顯示資訊。
5. 用 developer-a 登入部署 team-a；用 viewer-a 登入只有查看操作。owner-a 無法移除／降級最後一位 Owner。撤銷 Developer 後，連原本的冪等請求重送也會重新授權並拒絕。

每個變更表單都有 CSRF；刪除／回滾先進確認頁。送出部署先顯示 QUEUED／DEPLOYING，等待真實 Worker 回報後才會 READY／SUCCEEDED。錯誤頁及 Operation 保留 request_id、原因、版本歷史與觀測時間。

權限練習放在建立服務之後，先依下一節完成一個真正的部署。

**下一步：**確認本步結果後，前往[建立與呼叫真實服務](#建立與呼叫真實服務)。 若未符合預期，先到[操作排查](#操作遇到問題時)，處理後回到本步重試。

### 建立與呼叫真實服務

使用已取得 Developer 角色的 developer-a 登入，在 team-a 建立 `pricing-api`：

| 欄位 | 填入值 |
|---|---|
| Cluster | A |
| Image | pricing-api:v1 |
| Replicas／Port | 1／8080 |
| Requests | CPU 100m、Memory 64Mi |
| Limits | CPU 300m、Memory 128Mi |

送出只代表請求已接受。等待 Operation SUCCEEDED／Application READY，再於終端 B 執行：

```bash
make m4-forward-a
```

命令選 Worker A，預設 team-a 的 pricing-api 與本機 18080，不需先 export 或 source。若本輪使用自訂資料目錄，仍須先設定同一份 `HF_M4_STATE`。

<details>
<summary>展開：底層指令與環境設定</summary>

若直接使用 kubectl，先在同一終端執行 `source stages/m4/env.sh a`，操作 B 時改用 `b`；自訂 state 先設定同一路徑。短命令會自行載入環境。

```bash
export HF_M4_STATE="$PWD/.local/tutorials/m4" # 自訂 state 時改成同一個路徑
HF_CLUSTER=hugofleet-a HF_TENANT=team-a HF_APP=pricing-api make m4-forward
```

</details>

等待 `Forwarding` 訊息，保持終端 B 執行；終端 C 呼叫：

```bash
curl -fsS http://127.0.0.1:18080/version
curl -fsS -H 'Content-Type: application/json' \
  -d '{"amount_cents":20000,"customer_tier":"vip","coupon_code":"SAVE100"}' \
  http://127.0.0.1:18080/quote
```

預期版本 v1、`final_amount_cents=8000`。Ctrl+C 結束只會停止自己的 port-forward。
#### 瀏覽器：更新、故障與回滾

1. 以 Developer 以上角色在應用詳情發布 `pricing-api:v2-good`、replicas 2，其他配置不變，等待 SUCCEEDED／READY。
2. 停止並重建終端 B 的轉送，終端 C 查 `/version`，應為 v2-good。
3. 發布 `pricing-api:v2-broken`，保留兩副本，等待 Operation FAILED／TIMED_OUT，讀取原因與事件。
4. 按回滾，選剛才成功的 v2-good 快照並確認，等待新 revision SUCCEEDED／READY。
5. 確認舊 Pod 退出後重新轉送，再查版本與價格，應為 v2-good、8000 分。

回滾產生新 revision，保留失敗紀錄。先保留應用，供下一節以 Viewer 查看。

**下一步：**確認本步結果後，前往[權限練習：逐一確認拒絕結果](#權限練習逐一確認拒絕結果)。 若未符合預期，先到[操作排查](#操作遇到問題時)，處理後回到本步重試。

### 權限練習：逐一確認拒絕結果

完成前面的建立與呼叫後再操作；結束時恢復 developer-a 的 Developer 角色。

1. 已授予角色後，用 developer-a 登入並在瀏覽器直接開 `/tenants/team-b`。預期 HTTP 403／PERMISSION_DENIED，錯誤頁有 request_id；看不到另一租戶的應用內容。
2. 登入 viewer-a，開 team-a 與應用詳情。可以讀取配置、履歷及觀測時間，頁面不提供提交部署、更新成員、回滾或刪除表單；伺服器仍會拒絕自行構造的變更請求。
3. 登入 owner-a，在成員表單輸入自己頁面的 User ID，將自己改為 Viewer。若自己是最後一位 Owner，預期 409／CONFLICT，角色不會改變。恢復到租戶頁仍可看到 Owner 權限。
4. 移除 developer-a 的 membership，再以該帳號開 `/tenants/team-a`，預期拒絕；需要再練習時由 owner-a 重新授予 Developer。此例只操作本機 Demo 身分，不使用真實企業帳號。

精確的 JSON 重送撤權檢查、CPU／memory 實際 Admission 和 Policy 前後封包測試由下方真實 E2E 入口執行；不要把頁面沒有按鈕當成授權檢查已完成。

**下一步：**前往[整課驗證](#整課驗證與常見問題)，核對這一輪的結果。 若未符合預期，先到[操作排查](#操作遇到問題時)，處理後回到本步重試。

## 整課驗證與常見問題

完成主線後，核對以下結果；若有一項未達成，先排查再繼續。

- 以本輪帳號完成登入，兩個租戶的 Worker profile 啟用成功。
- Developer 可部署，服務回傳 8000；故障回滾後為 v2-good。
- 跨租戶存取被拒絕，Viewer 只能讀取，最後一位 Owner 無法降級。

這些是本輪操作的檢查點，不是新的驗收證據。自動化測試與既有結果見 [M4 驗收](../../docs/m4-validation.md)。


**核對完成後：**若已達成本課成果，前往[停止與下次繼續](#停止與下次繼續)。想繼續進階實作時先保留環境，從[原理與進階閱讀](#原理與進階閱讀)選擇練習；純閱讀不需保持服務運作。尚有問題則先用下方排查，修正後回到失敗步驟。
### 操作遇到問題時

| 現象 | 下一步 |
|---|---|
| 登入成功但沒有租戶 | admin 先啟用真實 profile，再由 Owner 授予角色 |
| 帳號密碼無效 | 核對目前 state 的 accounts.json，不沿用上一輪帳號 |
| 已登入但操作 403 | 核對 DB 角色；Developer 不能刪除，Viewer 不能部署 |
| Worker 未連線 | make m4-status，再查 Agent 日誌與憑證 |
| Pod 因配額無法建立 | 讀 Operation、CR conditions 與 Worker events，不能只看 Deployment 存在 |
| down ownership 不符 | 回原 state 操作，保留 ownership 記錄供診斷 |

查詢終端回專案根目錄；自訂 state 時沿用同一路徑：

先分開確認「管理平台是否正常」與「應用是否就緒」：

```bash
make m4-status
make m4-workload-status
```

第一個查 Hub／Dex／Agent，第二個查 Worker A 預設 team-a 的 pricing-api、期望副本與事件。若應用尚未建立，找不到指定物件是排查線索，先回 UI 看建立操作。

<details>
<summary>展開：詳細狀態與日誌指令</summary>

```bash
source stages/m4/env.sh a
kubectl --context kind-hugofleet-a -n hf-system logs deployment/hugofleet-agent --tail=100
kubectl --context kind-hugofleet-a -n hf-team-a get applications,deployments,pods
kubectl --context kind-hugofleet-a -n hf-team-a get events --sort-by=.lastTimestamp
```

</details>

## 停止與下次繼續

### 回來繼續時先選擇

先依原環境的情況選一條路；下方停止與重建命令不是需要依序全部執行的清單。

| 目前情況 | 下一步 |
|---|---|
| 原 Worker 正常，只停了轉送 | 先核對同輪 Hub 與 Agent 狀態，再重新建立轉送，回[逐步操作](#逐步操作)接續。 |
| 只執行 hub-stop，Worker 與憑證仍有效 | 使用原 state 的 `make m4-hub-up`，等待 Agent CONNECTED，再繼續操作。 |
| 已 down、Worker 重建或憑證失效 | 依下方重建說明先處理原環境，再選新 state開始新一輪；保留舊資料不等於能恢復舊 Worker。 |
| 不確定上次停在哪裡 | 沿用原 state，先看[操作排查](#操作遇到問題時)，不要直接換資料目錄。 |

### 停止與恢復

以下依目的選用，不要把所有操作當成一串必做步驟。先停終端 B 的轉送，沿用原 state：

| 目的 | 命令 | 預期結果 |
|---|---|---|
| 查看目前環境 | `make m4-status` | 顯示 Hub／Dex running 狀態、Worker Node 與 Agent Deployment |
| 暫停 Hub | `make m4-hub-stop` | UI 停止，Worker 與私有資料保留 |
| 恢復同一 Hub | `make m4-hub-up` | 原容器啟動，Agent 重連 |
| 更新 Hub 容器 | `make m4-hub-recreate` | 保留 DB 與 Worker，更新 Agent endpoint |
| 結束整課 | `make m4-down` | 移除 owned Hub／Dex／Workers，保留帳號、DB 與憑證 |

若要觀察單獨刪除應用，先以 owner-a 登入，在應用確認頁提交，等待 DELETE SUCCEEDED／DELETED。關閉瀏覽器或轉送不會刪除應用。

第一次結束整課，只需停止轉送，再執行 `make m4-down`。之後重建 Worker 要使用下方的新 state 流程。

Hub 停止時 Worker Operator 持續 Reconcile；Hub 恢復後以真實 Agent 回報同步，不能把 pending 操作直接標成成功。down 不刪資料、不做 global prune、不重啟 Docker，也不碰 lab。RAM etcd profile 的 node 重建不能當成持久叢集恢復測試。

hub-recreate 只更新既有 Agent 的 Hub endpoint，保留租戶 quota／receipt 與 ledger；不要用完整 up／workers 代替，它們會重新套用教學的預設安全設定。Hub 心跳逾時可透過 HF_M4_HEARTBEAT_TIMEOUT 設定（預設 15s，3s～5m），變更後 hub-recreate 才套用。

開發 mTLS leaf 憑證有效 24 小時、CA 48 小時。up／hub-recreate 不會續期；過期後的新握手會失敗，Hub／Agent 日誌可見 certificate expired。若重新練習，依下方先以原 state down 再建立新 state，保留舊履歷；不要直接刪除 certs 或 DB。保留原叢集與履歷的原地憑證輪替尚未提供驗收流程。

若 down 後要重建 Worker 並重新練習部署，先以舊 state 完成 down，再指定新 state：

```bash
export HF_M4_STATE="$PWD/.local/tutorials/m4/session-02"
make m4-up
```

舊 DB 的 Application 綁定舊 Worker CR UID，不能當作新叢集的恢復資料；舊履歷與帳號仍保留在舊 state。單純 hub-stop／hub-up 或 hub-recreate 會保留同一批 Worker，適合驗證 Hub 恢復，不需要新 DB。

### 完成這一課後去哪裡

- **進入下一課：**先停止本課的 watch／轉送，以原 state 與環境執行 `make m4-down`；完成後前往 [M5：Dashboard 與觀測](../m5/README.md#開始前的準備)。
- **下次繼續／重新練習：**回到[本輪環境選擇](#回來繼續時先選擇)，依原叢集與資料是否仍有效決定恢復方式。
- **深入本課：**閱讀[原理與進階閱讀](#原理與進階閱讀)。要做需要叢集的進階演練，先保留本輪環境；完整 down 後須依重建流程重新準備。

## 原理與進階閱讀

以下依需要查閱；進階演練若需要仍在執行的環境，請在完整 down 前操作。開發測試入口不屬於首次教學的必做步驟。

### 啟動流程與元件

正式入口會依序 build 三種 pricing image、Operator、M4 Hub／Agent／Go probe，建立 A 再建立 B，安裝固定版本 Calico，啟動真實 Dex、核對六個帳號的精確 issuer/sub、產生 mTLS 憑證、安裝租戶安全資源與 Agent，最後讓六個 Demo 帳號各完成真實 Hub 登入／登出。這個登錄步驟只建立已驗證 User，沒有授予租戶角色或建立應用。

### 管理 API 與安全邊界

API 使用登入後的平台 session cookie，Bearer ID Token／access Token 不能替代。GET `/api/v1/auth/me` 回本人 User／CSRF／到期時間。所有變更 JSON API 同時要求精確 Origin `http://127.0.0.1:18094` 與 `X-CSRF-Token`；部署／回滾／刪除還需 `Idempotency-Key` 和 `expected_desired_revision`。不要以 curl 的回覆 202 直接判斷 rollout 成功。

| 入口 | 角色／用途 |
|---|---|
| GET `/api/v1/clusters`、`/api/v1/tenants` | 只回可見範圍 |
| GET `/api/v1/users`、POST `/api/v1/tenants` | Platform Admin；啟用 Worker 的真實 profile |
| GET `/api/v1/tenants/{tenant}/quota`、`/api/v1/tenants/{tenant}/members` | Viewer 以上；POST members 要 Owner |
| GET `/api/v1/tenants/{tenant}/apps`、`/api/v1/tenants/{tenant}/apps/{app}` | Viewer 以上，跨租戶拒絕 |
| POST `/api/v1/tenants/{tenant}/apps`、`/api/v1/tenants/{tenant}/apps/{app}/releases`、`/api/v1/tenants/{tenant}/apps/{app}/rollback` | Developer 以上 |
| DELETE `/api/v1/tenants/{tenant}/apps/{app}` | Owner 以上 |
| GET `/api/v1/operations/{id}`、`/api/v1/audit-events?tenant=team-a` | 根據 app／tenant 範圍授權；跨租戶拒絕 |
| POST `/logout` | CSRF + Origin；DB session 立即失效 |

平台管理員不能透過 Hub 執行 shell、exec、任意 YAML、讀 Worker secrets 或改 RBAC。Namespace／Quota 修改留在本地管理員 bootstrap：

```bash
source stages/m4/env.sh a
go run ./stages/m4/cmd/bootstrap-worker --action tenant --cluster hugofleet-a \
  --kubeconfig "$HF_M4_STATE/clusters/hugofleet-a/kubeconfig" --tenant team-a \
  --requests-cpu=1 --requests-memory=1Gi --limits-cpu=3 --limits-memory=2Gi --pods=8
```

此工具只更新同 Worker／tenant label 的固定資源，拒絕接管 foreign resources。再以原 Owner／同名 profile 在 admin UI 同步。Hub 的估算是提示，實際 Pod Admission 是配額權威；一個 Deployment 可以建立成功，但 ReplicaSet 的 Pod 因 quota 被拒絕，原因經 CR／Agent 傳回 Hub／UI。

### 檔案與閱讀順序

| 檔案 | 用途 |
|---|---|
| [run.sh](run.sh)、[env.sh](env.sh)、[scripts/control.sh](scripts/control.sh) | 階段入口、每個終端環境、owned lifecycle、固定 Docker 網路與 loopback port |
| [auth](auth/README.md) | Code + S256 PKCE、state／nonce／session／cookie／CSRF |
| [httpapi](httpapi/README.md)、[web](web/) | 管理 API、DB 角色、Go template 頁面 |
| [bootstrap](bootstrap/README.md) | 分離的本地管理員 bootstrap、唯讀 Worker proof |
| [Hub 入口](cmd/hub/main.go)、[共用 startup](hubapp/run.go) | OIDC Hub，沒有 Worker kubeconfig／舊管理 Token 參數；M4 保留預設行為，M5 以選項啟用 Dashboard |
| [testdriver](testdriver/)、[OIDC 驗收](test/oidc/)、[kind 驗收](test/e2e/) | 真實 Provider、Hub、Pod、Admission 與封包流程 |
| [共用 Store](../m2/hub/repository/)、[Agent](../m3/agent/README.md)、[通道](../m3/agenttransport/README.md) | 本階段延伸的原實作，沒有複製歷史核心 |

### 測試與證據

```bash
make m4-build
make operator-tools envtest proto-tools
make m4-test
make proto-check
make test-integration
make test-e2e-m4
```

`test-e2e-m4` 使用獨立 m4-e2e A／B、私有新 state、真實 Dex／mTLS Pod／SQLite，測試 T14 跨租戶權限、T15 CPU／memory Pod Admission 及 T16 enforcing Calico。它以真正 Go probe Pod 比較 Policy 前後與同租戶正向控制，不使用 shell／exec；測試結束只清除精確 owned resources，保留私有資料與日誌。此命令需先清掉佔用 18094／5556 的 M4 教學實例；有殘留 E2E 名稱會拒絕啟動。

同一真實驗收也提交伺服器實際渲染的表單，使用頁面的 CSRF／冪等鍵／期望版本：建立服務、v2-good／replicas 2、手動 Deployment 漂移、v2-broken 的實際 readiness 故障、確認回滾與刪除／GC。再停止 B Agent 驗證斷線與當地服務延續，恢復連線；停止／恢復 owned Hub，驗證 Worker 自主修復、原 DB／session 與重新同步。測試證據見 [M4 驗收](../../docs/m4-validation.md)。

`make test-oidc` 是獨立的真實 Dex 負向驗證入口，建立新的私有 state、Dex 與 session TTL=5s 的本機 Hub，執行 state／nonce／binding／PKCE、JWT 簽章／issuer／audience、約 15 分鐘自然到期及 canonical backchannel 測試，再清理本次程序與容器。需先釋放 5556／18094／18444，不需 Worker；日誌在 artifacts/m4-oidc，帳號／DB 保留於 .local/m4-oidc-*。CI 另有 oidc job；Unit 並不涵蓋這些真實 Provider 測項。

E2E 因中斷留下資源時，從根目錄指定**原執行輸出的 state**及所有 E2E 名稱，再使用正式 down。先把下方 `原執行編號` 換成該次實際目錄名稱，核對其中 hub.id／dex.id／clusters 的 ownership；不可原樣貼上佔位路徑、改用教學 state 或移除 ownership 記錄：

```bash
export HF_M4_STATE="$PWD/.local/m4-e2e-原執行編號"
HF_M4_A=hugofleet-m4-e2e-a HF_M4_B=hugofleet-m4-e2e-b \
HF_M4_HUB=hugofleet-hub-m4-e2e HF_M4_DEX=hugofleet-dex-m4-e2e \
bash stages/m4/run.sh down
```

### 限制

A 的 Calico Security Profile 驗證 ingress 跨租戶隔離；B 的 kindnet Policy 僅為 configuration，沒有 enforcement 宣稱。Policy 允許同 namespace ingress；此階段不承諾 egress／host／跨叢集網路隔離。單 Hub／SQLite、本機 Dex 靜態帳號、手動憑證管理、RAM etcd 與同 Docker 網路仍為展示環境；正式部署的 HTTPS／issuer／subject／網路入口須由管理員供應。

**讀完深入內容後：**回到[本課完成後的選擇](#完成這一課後去哪裡)，接續下一課、重新練習或結束環境。
