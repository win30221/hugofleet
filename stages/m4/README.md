# M4：OIDC、租戶權限與真實隔離

本階段延用 M2 的 SQLite／Release／Operation runner、M3 的 Worker 主動 mTLS 通道，增加真實 Dex 登入、server-side session、DB membership、租戶 namespaces、RBAC、Quota、LimitRange、NetworkPolicy 與稽核。**2026-10-13 已完成本機驗收**，包含全新正式入口與 owned cleanup；逐項實測見 [M4 驗收紀錄](../../docs/m4-validation.md)，設計與固定版本見 [ADR 0010](../../docs/adr/0010-oidc-tenants-and-worker-bootstrap.md)。

撰寫 client 請使用 [M4 OpenAPI](../../docs/openapi-m4.yaml)：session、CSRF、角色與租戶 profile 的契約和 M2／M3 開發模式不同。

## 從主目錄開始

先讀 [根 README](../../README.md) 與 [階段導航](../README.md)。Windows 使用 Ubuntu WSL2；所有 Bash 命令以非 root 的 Linux 使用者執行，各終端都從專案根目錄開始：

```bash
cd /mnt/c/Users/H/Documents/GitHub/win30221/hugofleet
make tools
make doctor
```

M4 使用 A／B 兩個 Worker；A 為 Calico Security Profile，B 使用 kindnet。兩個租戶 team-a／team-b 在兩個 Worker 各有獨立 namespace；每 namespace 有自己的受限 Operator，Agent 位於 hf-system。M3／M4 共用預設 Worker 名稱，因此先用原階段、原 state 的 `down` 清理上一課。腳本核對精確 Container ID，拒絕接管其他階段或 unrelated lab。

## 檔案與閱讀順序

| 檔案 | 用途 |
|---|---|
| [run.sh](run.sh)、[env.sh](env.sh)、[scripts/control.sh](scripts/control.sh) | 階段入口、每個終端環境、owned lifecycle、固定 Docker 網路與 loopback port |
| [auth](auth/README.md) | Code + S256 PKCE、state／nonce／session／cookie／CSRF |
| [httpapi](httpapi/README.md)、[web](web/) | 管理 API、DB 角色、Go template 頁面 |
| [bootstrap](bootstrap/README.md) | 分離的本地管理員 bootstrap、唯讀 Worker proof |
| [Hub 入口](cmd/hub/main.go)、[共用 startup](hubapp/run.go) | OIDC Hub，沒有 Worker kubeconfig／舊管理 Token 參數；M4 保留預設行為，M5 以選項啟用 Dashboard |
| [testdriver](testdriver/)、[OIDC 驗收](test/oidc/)、[kind 驗收](test/e2e/) | 真實 Provider、Hub、Pod、Admission 與封包流程 |
| [共用 Store](../m2/hub/repository/)、[Agent](../m3/agent/README.md)、[通道](../m3/agenttransport/README.md) | 本階段延伸的原實作，沒有複製歷史核心 |

## 終端 A：啟動

```bash
bash stages/m4/run.sh up
# 等價於 make m4-up
```

正式入口會依序 build 三種 pricing image、Operator、M4 Hub／Agent／Go probe，建立 A 再建立 B，安裝固定版本 Calico，啟動真實 Dex、核對六個帳號的精確 issuer/sub、產生 mTLS 憑證、安裝租戶安全資源與 Agent，最後讓六個 Demo 帳號各完成真實 Hub 登入／登出。這個登錄步驟只建立已驗證 User，沒有授予租戶角色或建立應用。

首次下載映像與 Docker Desktop 的磁碟 flush 可能花數十分鐘。Calico 三個固定版本映像先存於主機 Docker cache，再載入 owned A Worker；重建叢集會沿用此快取，避免在節點內重複下載。每個 bootstrap log 在 `artifacts/bootstrap-<cluster>/`；Calico readiness 有有界等待，逾時後可對同一 state 重跑入口。版本、CNI 或 ownership 不符會直接拒絕；不要刪除 ownership 檔案強制繞過。

預期顯示兩個真實 Worker 已連線。管理 UI 是 `http://127.0.0.1:18094/login`；Dex issuer 是 `http://127.0.0.1:5556/dex`。HTTP 是明確的 loopback LocalDemo 例外；正常部署須設定 HTTPS，session cookie 會設 Secure。

## 私有資料與帳號

預設 state：`.local/tutorials/m4`。自訂範例：

```bash
export HF_M4_STATE="$PWD/.local/tutorials/m4/my-lesson"
bash stages/m4/run.sh up
```

整課的 up／status／hub-stop／hub-up／hub-recreate／down 必須使用同一 state。`.local` 被 Git 忽略；`accounts.json` 含隨機密碼與 Client Secret，`subjects.json` 保存實際驗證的 subject，`certs/` 含本機憑證與私鑰，`db/hub.db` 為 SQLite。不要貼到 issue、截圖、影片或驗收日誌。既有帳號不會因再次 up 而被靜默換掉。每個新終端先 export 同一 state，再 `source stages/m4/env.sh a`（或 b），才直接使用 kubectl／bootstrap。

在自己的本地編輯器打開 `accounts.json`，選擇單一帳號的 email／password 登入 Dex：admin、owner-a、developer-a、owner-b、viewer-a、outsider。Platform Admin 僅綁定已驗證的 admin subject；所有帳號故意帶有相同 elevated groups，Hub 仍只信 DB 角色。帳號與密碼僅為本機 Demo，不代表企業 IAM。

## 瀏覽器：啟用租戶與角色

1. 使用 admin 登入。首頁列出真實 A／B Agent 心跳與版本，以及已登入的 User。
2. 在「啟用已 bootstrap 的租戶」填 slug/name `team-a`、選 Cluster A、Owner 選 owner-a。Hub 向 Agent 核對實際 namespace、receipt UID、Quota／LimitRange，再啟用資料庫 profile。用相同 name／Owner 啟用 team-a 的 B profile。
3. 同樣以 owner-b 啟用 team-b 的 A／B profile。未 bootstrap 或離線 Worker 不能靠 DB row 偽裝啟用成功。
4. 登出 admin，登入 owner-a；team-a 的成員頁可將已登入 User ID 授予 Developer／Viewer。User ID 從 admin 的已驗證 User 清單或本人頁面取得，email 只是顯示資訊。
5. 用 developer-a 登入部署 team-a；用 viewer-a 登入只有查看操作。owner-a 無法移除／降級最後一位 Owner。撤銷 Developer 後，連原本的冪等請求重送也會重新授權並拒絕。

每個變更表單都有 CSRF；刪除／回滾先進確認頁。送出部署先顯示 QUEUED／DEPLOYING，等待真實 Worker 回報後才會 READY／SUCCEEDED。錯誤頁及 Operation 保留 request_id、原因、版本歷史與觀測時間。

可跟著做的權限練習：

1. 已授予角色後，用 developer-a 登入並在瀏覽器直接開 `/tenants/team-b`。預期 HTTP 403／PERMISSION_DENIED，錯誤頁有 request_id；看不到另一租戶的應用內容。
2. 登入 viewer-a，開 team-a 與應用詳情。可以讀取配置、履歷及觀測時間，頁面不提供提交部署、更新成員、回滾或刪除表單；伺服器仍會拒絕自行構造的變更請求。
3. 登入 owner-a，在成員表單輸入自己頁面的 User ID，將自己改為 Viewer。若自己是最後一位 Owner，預期 409／CONFLICT，角色不會改變。恢復到租戶頁仍可看到 Owner 權限。
4. 移除 developer-a 的 membership，再以該帳號開 `/tenants/team-a`，預期拒絕；需要再練習時由 owner-a 重新授予 Developer。此例只操作本機 Demo 身分，不使用真實企業帳號。

精確的 JSON 重送撤權檢查、CPU／memory 實際 Admission 和 Policy 前後封包測試由下方真實 E2E 入口執行；不要把頁面沒有按鈕當成授權檢查已完成。

## 建立與呼叫真實服務

在 team-a 建立 `pricing-api`，選 Cluster A、image `pricing-api:v1`、replicas 1、port 8080、requests 100m／64Mi、limits 300m／128Mi。等待 READY，再在終端 B：

```bash
export HF_M4_STATE="$PWD/.local/tutorials/m4" # 自訂 state 時改成同一個路徑
HF_CLUSTER=hugofleet-a HF_TENANT=team-a HF_APP=pricing-api bash stages/m4/run.sh forward
```

終端 C：

```bash
curl -fsS http://127.0.0.1:18080/version
curl -fsS -H 'Content-Type: application/json' \
  -d '{"amount_cents":20000,"customer_tier":"vip","coupon_code":"SAVE100"}' \
  http://127.0.0.1:18080/quote
```

預期版本 v1、`final_amount_cents=8000`。Ctrl+C 結束只會停止自己的 port-forward。更新 v2-good／replicas 2、v2-broken 失敗及回滾時，仍遵循 M2 的 immutable Release／Operation 規則；回滾產生新 revision，不刪除失敗紀錄。

## 管理 API 與安全邊界

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

## 測試與證據

```bash
make m4-build
make operator-tools envtest proto-tools
make m4-test
make proto-check
make test-integration
make test-e2e-m4
```

`test-e2e-m4` 使用獨立 m4-e2e A／B、私有新 state、真實 Dex／mTLS Pod／SQLite，測試 T14 跨租戶權限、T15 CPU／memory Pod Admission 及 T16 enforcing Calico。它以真正 Go probe Pod 比較 Policy 前後與同租戶正向控制，不使用 shell／exec；測試結束只清除精確 owned resources，保留私有資料與日誌。此命令需先清掉佔用 18094／5556 的 M4 教學實例；有殘留 E2E 名稱會拒絕啟動。

同一真實驗收也提交伺服器實際渲染的表單，使用頁面的 CSRF／冪等鍵／期望版本：建立服務、v2-good／replicas 2、手動 Deployment 漂移、v2-broken 的實際 readiness 故障、確認回滾與刪除／GC。再停止 B Agent 驗證斷線與當地服務延續，恢復連線；停止／恢復 owned Hub，驗證 Worker 自主修復、原 DB／session 與重新同步。是否通過仍以本輪驗收紀錄為準。

`make test-oidc` 是獨立的真實 Dex 負向驗證入口，建立新的私有 state、Dex 與 session TTL=5s 的本機 Hub，執行 state／nonce／binding／PKCE、JWT 簽章／issuer／audience、約 15 分鐘自然到期及 canonical backchannel 測試，再清理本次程序與容器。需先釋放 5556／18094／18444，不需 Worker；日誌在 artifacts/m4-oidc，帳號／DB 保留於 .local/m4-oidc-*。CI 另有 oidc job；Unit 並不涵蓋這些真實 Provider 測項。

E2E 因中斷留下資源時，從根目錄指定**原執行輸出的 state**及所有 E2E 名稱，再使用正式 down。先把下方 `原執行編號` 換成該次實際目錄名稱，核對其中 hub.id／dex.id／clusters 的 ownership；不可原樣貼上佔位路徑、改用教學 state 或移除 ownership 記錄：

```bash
export HF_M4_STATE="$PWD/.local/m4-e2e-原執行編號"
HF_M4_A=hugofleet-m4-e2e-a HF_M4_B=hugofleet-m4-e2e-b \
HF_M4_HUB=hugofleet-hub-m4-e2e HF_M4_DEX=hugofleet-dex-m4-e2e \
bash stages/m4/run.sh down
```

## 停止與恢復

```bash
bash stages/m4/run.sh status
bash stages/m4/run.sh hub-stop
bash stages/m4/run.sh hub-up
# 新 Hub 容器、原 DB 與身份；更新 Agent 的動態 Hub IP，ledger 不重置：
bash stages/m4/run.sh hub-recreate
# 完整 owned cleanup，保留 DB／帳號／憑證：
bash stages/m4/run.sh down
```

Hub 停止時 Worker Operator 持續 Reconcile；Hub 恢復後以真實 Agent 回報同步，不能把 pending 操作直接標成成功。down 不刪資料、不做 global prune、不重啟 Docker，也不碰 lab。RAM etcd profile 的 node 重建不能當成持久叢集恢復測試。

hub-recreate 只更新既有 Agent 的 Hub endpoint，保留租戶 quota／receipt 與 ledger；不要用完整 up／workers 代替，它們會重新套用教學的預設安全設定。Hub 心跳逾時可透過 HF_M4_HEARTBEAT_TIMEOUT 設定（預設 15s，3s～5m），變更後 hub-recreate 才套用。

開發 mTLS leaf 憑證有效 24 小時、CA 48 小時。up／hub-recreate 不會續期；過期後的新握手會失敗，Hub／Agent 日誌可見 certificate expired。若重新練習，依下方先以原 state down 再建立新 state，保留舊履歷；不要直接刪除 certs 或 DB。保留原叢集與履歷的原地憑證輪替尚未提供驗收流程。

若 down 後要重建 Worker 並重新練習部署，先以舊 state 完成 down，再指定新 state：

```bash
export HF_M4_STATE="$PWD/.local/tutorials/m4/session-02"
bash stages/m4/run.sh up
```

舊 DB 的 Application 綁定舊 Worker CR UID，不能當作新叢集的恢復資料；舊履歷與帳號仍保留在舊 state。單純 hub-stop／hub-up 或 hub-recreate 會保留同一批 Worker，適合驗證 Hub 恢復，不需要新 DB。

## 限制

A 的 Calico Security Profile 驗證 ingress 跨租戶隔離；B 的 kindnet Policy 僅為 configuration，沒有 enforcement 宣稱。Policy 允許同 namespace ingress；此階段不承諾 egress／host／跨叢集網路隔離。單 Hub／SQLite、本機 Dex 靜態帳號、手動憑證管理、RAM etcd 與同 Docker 網路仍為展示環境；正式部署的 HTTPS／issuer／subject／網路入口須由管理員供應。
