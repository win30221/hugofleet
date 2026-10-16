# 各階段教學與程式

先用 [M0～M5 動畫教室](../docs/animations/index.html) 理解每章用途與流程，再依下方教學實作。下載專案後，用瀏覽器開啟 HTML 即可離線播放。

本頁是選課、跳課與回訪入口。第一次使用從[根 README](../README.md#第一次使用從這裡開始)完成環境準備後，直接進入 [M0](m0/README.md)，不需先讀完整份索引。每課會帶你完成、清理，再進下一課。

## 先選你要做的事

| 你目前的進度 | 下一步 | 完成時應看到什麼 |
|---|---|---|
| 還沒操作過 Kubernetes | [M0：部署價格 API](m0/README.md) | 真實 API 回傳 v1、價格 8000 分 |
| 已理解 Deployment／Service | [M1：Operator](m1/README.md) | 改小的 Deployment 副本被修復 |
| 想從頁面管理部署 | [M2：Hub](m2/README.md) | 發版、故障、回滾有獨立履歷 |
| 想操作兩個 Worker | [M3：Agent](m3/README.md) | A 離線時 B 仍能部署，A 當地仍修復 |
| 想區分帳號與團隊權限 | [M4：租戶安全](m4/README.md) | 可授權部署，跨租戶／低權限操作被拒絕 |
| 想看部署觀測與交付 | [M5：Dashboard](m5/README.md) | 真實狀態、trace／metrics 與 Helm 元件 |

不需要先手動啟動所有前置階段，各課 up 會準備自己的依賴。筆電一次練習一課；切換前先停止轉送與原 Hub，再用原階段、原 state 執行 down。

第一次操作只需讀該課主線，程式地圖、元件導讀與自動驗收按需要查閱。下方目錄樹是找檔案用，不是待執行清單。

## 回來繼續哪一課

先選上次練習的階段，再判斷原環境還在，還是已執行 down。沿用原 state 核對；不要直接重跑 up 或更換 DB。

| 階段 | 第一次練習 | 上次做到一半／重新練習 | 遇到問題 |
|---|---|---|---|
| M0 | [開始本課](m0/README.md#開始前的準備) | [恢復或重新練習](m0/README.md#回來繼續時先選擇) | [排查問題](m0/README.md#操作遇到問題時) |
| M1 | [開始本課](m1/README.md#開始前的準備) | [恢復或重新練習](m1/README.md#回來繼續時先選擇) | [排查問題](m1/README.md#操作遇到問題時) |
| M2 | [開始本課](m2/README.md#開始前的準備) | [恢復或重新練習](m2/README.md#回來繼續時先選擇) | [排查問題](m2/README.md#操作遇到問題時) |
| M3 | [開始本課](m3/README.md#開始前的準備) | [恢復或重新練習](m3/README.md#回來繼續時先選擇) | [排查問題](m3/README.md#操作遇到問題時) |
| M4 | [開始本課](m4/README.md#開始前的準備) | [恢復或重新練習](m4/README.md#回來繼續時先選擇) | [排查問題](m4/README.md#操作遇到問題時) |
| M5 | [開始本課](m5/README.md#開始前的準備) | [恢復或重新練習](m5/README.md#回來繼續時先選擇) | [排查問題](m5/README.md#操作遇到問題時) |

只想換課時，先依原課停止流程清理，再點下一課的「開始本課」。元件與程式位置在下方，需要讀程式時再查。

## 這些文件怎麼使用

| 想做什麼 | 文件層級 |
|---|---|
| 選課、知道後面會增加什麼、切換環境 | 本頁 |
| 從零開始操作某階段、解讀結果與排查問題 | 各階段根目錄的 README |
| 追一個請求、理解程式內部設計、找修改位置 | [pricing-api](m0/pricing-api/README.md)、[Controller](m1/controller/README.md)、[Hub](m2/hub/README.md)、[Agent](m3/agent/README.md)、[Hub Transport](m3/agenttransport/README.md) 的 README |
| 查精確 API 欄位、設計原因、驗收範圍與證據 | [docs](../docs/README.md) |

每課依相同順序閱讀：

1. 這一課的目標與成果：先知道完成時要看到什麼。
2. 開始前的準備：工具、目錄、上一課清理與本輪 state。
3. 逐步操作：按實際流程切換終端與瀏覽器，每步核對結果。
4. 整課驗證與常見問題：確認整條流程，再查故障原因。
5. 停止與下次繼續：區分暫停、完整清理、接續與重新練習。
6. 原理與進階閱讀：架構、程式、替代操作與開發測試。

先跟著操作，再沿程式導讀理解原因。教學中的「預期結果」是操作時要核對的行為；測試證據另在 docs，不把文件更新當成新一輪驗收。

```text
stages/
├── README.md                 ← 所有階段的索引（本文件）
├── m0/
│   ├── README.md             ← M0 完整教學與程式導讀
│   ├── pricing-api/          ← Go API、價格規則、Dockerfile、Unit
│   ├── workload.yaml         ← Deployment／Service
│   ├── run.sh                ← M0 啟動／轉送／結束入口
│   └── cmd/、scripts/        ← 驗收程式與 Bash
├── m1/
│   ├── README.md             ← M1 完整教學與程式導讀
│   ├── api/、controller/     ← Application 型別與 Operator 邏輯
│   ├── application.yaml     ← 本階段練習用 CR
│   ├── run.sh                ← M1 啟動／轉送／結束入口
│   └── cmd/、config/、build/、scripts/、test/
├── m2/
│   ├── README.md             ← M2 完整教學與程式導讀
│   ├── hub/、web/            ← Hub Go 程式、SQLite、UI
│   ├── policy.json、create-app.json
│   ├── run.sh                ← M2 啟動／轉送／結束入口
│   └── cmd/、config/、build/、scripts/、test/
├── m3/
│   ├── README.md             ← M3 完整教學與程式導讀
│   ├── agent/、agenttransport/、agentprotocol/、agenttls/、devcert/
│   ├── api/                  ← Protobuf 與生成的 Go 程式
│   ├── deploy-a.json、deploy-b.json
│   ├── run.sh、env.sh         ← M3 操作入口與 A／B 終端環境
│   └── cmd/、config/、build/、scripts/、test/
├── m4/
│   ├── README.md、run.sh      ← 真實 OIDC、雙 Worker／Security Profile 操作
│   ├── auth/、httpapi/、web/   ← 登入／session、角色 API、租戶 UI
│   ├── bootstrap/            ← 分離的本地管理員安全資源建立
│   └── cmd/、build/、scripts/、testdriver/、test/
└── m5/
    ├── README.md、run.sh、env.sh ← Dashboard／雙 Worker 的獨立操作
    ├── observability/、traceview/、web/ ← telemetry、離線 viewer、UI
    └── chart/、cmd/、build/、scripts/、test/
```

| 階段 | 本階段新增的程式 | 從哪裡開始讀 | 獨立啟動 | 本階段編譯／Unit |
|---|---|---|---|---|
| M0 | pricing-api、HTTP／工作負載驗收工具 | [M0 教學](m0/README.md) | `make m0-up` | `make m0-build m0-test` |
| M1 | Application 型別、Operator、Reconcile | [M1 教學](m1/README.md) | `make m1-up` | `make m1-build m1-test` |
| M2 | Hub、REST／UI、SQLite、部署與回滾 | [M2 教學](m2/README.md) | `make m2-up` | `make m2-build m2-test` |
| M3 | Worker Agent、gRPC 協定、mTLS、Hub 遠端 Transport | [M3 教學](m3/README.md) | `make m3-up` | `make m3-build m3-test` |
| M4 | OIDC／session、租戶 RBAC／Quota、Calico Security Profile | [M4 教學](m4/README.md) | `make m4-up` | `make m4-build m4-test` |
| M5 | Dashboard／trace／metrics、Helm 交付 | [M5 教學](m5/README.md) | `make m5-up` | `make m5-build m5-test` |

以上命令都在專案根目錄、Ubuntu WSL2 Bash 執行。本階段編譯會包含它使用的共用套件；本階段 Unit 不替代 envtest／kind E2E。前面的學習入口可以各自啟動，不必先把其他階段跑起來。

`mN-build` 產生主機 bin 執行檔，不建 Docker 映像；`mN-up` 才組裝該課的真實環境。M3 build 也編譯共用的 M2 Hub。這些開發命令不用當成每次閱讀／學習的必做前置。

也可進入 `stages/m0` 等資料夾執行 `bash run.sh up`／`bash run.sh down`；M0～M2 支援 `bash run.sh forward`。M3 的 A／B 轉送依自己的 README 操作。每個資料夾的 `scripts/` 放本階段驗收／工具腳本，`run.sh` 是學習入口。

同一個 Go module 依最初引入的階段分類。M1～M3 沿用 M0 應用；M2／M3 沿用 M1 Operator；M3 沿用並擴充 M2 Hub。共用程式只維護一份，後續修改的位置會在各導讀標明。若只學 M0，先看 m0 即可。

「可獨立啟動」指每個入口自行準備需要的元件，並使用分開的叢集與資料；程式是目前版本的共用來源，各階段共用同一份現行實作。M2／M3 不要求前面各 Demo 持續執行。

| 想找什麼 | 現在的位置 |
|---|---|
| 價格 API、HTTP／工作負載驗收 | [m0](m0/README.md) |
| Application 型別、Operator、M1 config／E2E | [m1](m1/README.md) |
| Hub、bootstrap-hub、UI、M2 config／E2E | [m2](m2/README.md) |
| Agent 協定／Proto、devcert、Hub transport、M3 cmd／config／E2E | [m3](m3/README.md) |
| 文件檢查程式 | [tools/doccheck](../tools/doccheck/main.go) |
| 每階段可以修改的 YAML／JSON、操作 Bash | 本階段資料夾的根目錄與 scripts/，見各 README |
| 執行後產生的 kubeconfig／DB／憑證 | `.local/tutorials/m0`～`m5`，Git 忽略 |
| 驗收證據、API 契約、設計決策 | [docs](../docs/README.md) |

跨階段共用的版本／Docker／kind 工具留在 [scripts](../scripts/)、[tools](../tools/)，kind 節點設定在 [deploy/kind](../deploy/kind/)；各階段文件都有連結。所有 Go 程式位於 stages 或 tools。目錄決策見 [ADR 0009](../docs/adr/0009-stage-source-layout.md)，切換與重建看下方說明。各階段驗收與 Hosted CI 證據見 [驗證總覽](../docs/validation.md)。

各階段「檔案地圖」會標明可修改的範例、手寫程式、生成檔案與執行產物。Dockerfile 的 build context 使用完整專案根目錄，因為共用同一 Go module；不要只拿某個子資料夾單獨當完整 build context。

## 選擇階段、切換與重建

所有命令在專案根目錄的 Linux amd64／Ubuntu WSL2 Bash 執行。已安裝工具不用重跑 tools，先檢查：

```bash
make doctor
```

Windows 從專案資料夾開 PowerShell，再執行 `wsl -d Ubuntu`；詳細方式見根 README。各終端從相同專案位置開始，自訂 state 時沿用相同設定。

`run.sh` 在子程序設定環境，不會替父終端設定 kubeconfig。之後自己輸入 kubectl 時，要照該課重新 export／source。`common.sh` 會切回專案根目錄並啟用嚴格錯誤處理；預期失敗的練習用 if 包起來，避免 Shell 因非零退出而結束。

| 階段 | 啟動 | 另一終端呼叫服務 | 結束 | 先理解什麼 |
|---|---|---|---|---|
| M0 | `make m0-up` | `make m0-forward`，curl 18080 | Ctrl+C 停轉送，`make m0-down` | Go API、映像、Pod、Deployment、Service、readiness |
| M1 | `make m1-up` | `make m1-forward`，curl 18080 | Ctrl+C 停轉送，`make m1-down` | CRD、Reconcile、期望狀態、漂移修復 |
| M2 | `make m2-up`（前景 Hub） | UI 建立 pricing-api 後 `make m2-forward` | Ctrl+C 停 Hub／轉送，`make m2-down` | UI／API、SQLite、Release、Operation、回滾 |
| M3 | `make m3-up`（背景 Hub 容器） | 登入 UI 選 A／B；[轉送命令](m3/README.md) | 停轉送，`make m3-down` | 主動連線 Agent、gRPC、mTLS、斷線與重連 |
| M4 | `make m4-up`（背景 Hub／Dex） | OIDC 登入、啟用租戶、建立應用；[轉送命令](m4/README.md) | 停轉送，`make m4-down` | DB 角色、租戶 RBAC、真實 Admission／NetworkPolicy |
| M5 | `make m5-up`（背景 Hub／Dex） | Dashboard／OTel／metrics；[轉送命令](m5/README.md) | 停轉送，`make m5-down` | 真實觀測、跨元件 trace、Helm lifecycle |

M2／M3 共用本機 UI port 18090，轉送預設 18080；筆電建議一次只開一個階段，切換前先關閉上個階段。啟動會真實建置映像與建立 kind；此機 WSL 準備叢集可能超過 10 分鐘，腳本會輸出進度及 bootstrap log 路徑。

M4 UI 是 18094、Dex 是 5556；Worker 預設也使用 hugofleet-a／hugofleet-b，且 A 的 CNI 與 M3 不同。切換前以原階段、原 state 執行 down；M4 up 只準備環境及真實登入 User，租戶啟用／角色與應用仍依 [M4 教學](m4/README.md) 操作。

M0／M1 up 完成後返回命令列；M2 up 在前景持續執行 Hub，需保留該終端；M3 Hub 是背景 Docker 容器，up 完成後返回。M2／M3 up 只準備環境，仍要由 UI 或 REST 建立應用。不同資料夾隔開執行資料，不能解決共用本機 port 的衝突。

M0–M2 捷徑使用 `hugofleet-m0-demo`／`hugofleet-m1-demo`／`hugofleet-m2-demo`；M3 用 `hugofleet-a`／`hugofleet-b`。kubectl 每次明確指定隔離 kubeconfig 與 context，例如 M1：

```bash
export HF_CLUSTER=hugofleet-m1-demo
unset HF_LEARNING_ROOT
source scripts/common.sh
kubectl --context kind-hugofleet-m1-demo -n hf-demo get applications,deployments,services,pods
```

新終端記得重新 source。M0～M2 使用表內的專用學習叢集；M3 使用 hugofleet-a／hugofleet-b，依各階段 up／down 切換。

每階段完成下面練習再繼續：

1. M0：呼叫 `/quote` 得到 8000；能說明 image／Pod／Service 的關係；知道 broken 版 healthz 正常、readyz 失敗。[詳細練習](m0/README.md)
2. M1：由 Application 建立工作負載；把 Deployment replicas 改小，看 Operator 修復；能區分 CR spec 與 status。[詳細練習](m1/README.md)
3. M2：由 UI 發布 v1、壞版、回滾；能說明 desired／applied／last successful 和 Operation／Release 的不同。[教學](m2/README.md)、[API](../docs/m2-api.md)
4. M3：把不同應用部署至 A／B；停 A Agent，確認 B 仍可發布、A Operator 仍修復；恢復 Agent 看狀態回來。[M3 指南](m3/README.md)
5. M4：以管理員核對已 bootstrap 的租戶，用 Owner 授予 Developer／Viewer，部署應用並驗證跨租戶拒絕、配額失敗與隔離。[M4 指南](m4/README.md)

共同服務呼叫：

```bash
curl --fail http://127.0.0.1:18080/version
curl --fail http://127.0.0.1:18080/quote \
  -H 'Content-Type: application/json' \
  --data '{"amount_cents":20000,"customer_tier":"vip","coupon_code":"SAVE100"}'
```

`*-down` 刪除該階段 owned kind，保留 DB／工具／測試產物。重建 M2 叢集需新 DB：

```bash
export HF_HUB_DATABASE="$PWD/.local/tutorials/m2/hub/m2-session-02.db"
make m2-up
```

M3 重建兩個 Worker 時也使用新 state（含新 DB／憑證）：

```bash
export HF_M3_STATE="$PWD/.local/tutorials/m3/session-02"
make m3-up
```

後續 M3 操作及 `m3-down` 使用相同 HF_M3_STATE；每個新終端先 export 同一路徑，再 source M3 env.sh。保留叢集僅指 node 未停止／重啟；預設 RAM etcd 不提供 Docker 重啟後的持久保證。可以反覆學習；目前狀態看根 README 與對應驗收報告。

M3 新 session 不改變 kind／Hub 容器名稱；先用**舊 state**執行 m3-down，才切換新 state。M2／M3 的新 DB 是新的一輪練習，舊履歷仍在原檔案，不會自動合併或恢復至新叢集。

M4 重建 Worker 也先以舊 HF_M4_STATE 執行 m4-down，再指定 `.local/tutorials/m4/session-02` 等新 state。只停止／恢復或重建 Hub 容器時則沿用原 state、DB／憑證；詳見該課的停止與恢復章節。

## 執行資料與原始碼的分工

| 類型 | 路徑 | 怎麼使用 |
|---|---|---|
| 教學原始碼／範例 | stages/m0～m5 | 主要閱讀與修改位置；各課程標明手寫／生成檔案 |
| M0～M2 叢集資料 | `.local/tutorials/mN/clusters/<cluster>/` | kubeconfig、cluster.id、生成的 kind YAML，由腳本管理 |
| M2 Hub | `.local/tutorials/m2/hub/` | 受限 kubeconfig、DB／WAL／鎖；同輪沿用，重建叢集換新 DB |
| M3 state | `.local/tutorials/m3/` 或其 session 子目錄 | DB、憑證、Token、生成的 Agent YAML、Hub ownership、雙 Worker kubeconfig |
| M4 state | `.local/tutorials/m4/` 或其 session 子目錄 | 私有 Dex 帳號／Client Secret、驗證的 subject、mTLS、DB、Hub／Dex ownership 與雙 Worker kubeconfig |
| M5 state | `.local/tutorials/m5/` 或其 session 子目錄 | M4 同類私有材料、SQLite 觀測／trace context、Helm values、雙 Worker ownership；原始 spans 收集至 artifacts |
| 主機執行檔／映像 | bin；Docker image store | 從正式原始碼建置；修改 Go 後須重建對應程式／映像 |
| 工具／快取 | .tools、.cache 或已登記的 WSL 原生快取 | 共用環境，由 scripts 管理 |
| 原始日誌 | artifacts | 找故障與既有實測證據，摘要在 docs |

本機產物被 Git 忽略。不要先手動刪 kubeconfig／cluster.id／hub.id，它們是腳本辨認所屬叢集或容器的依據。DB、憑證、Token 也不當成一般快取清理。down 限定清理所屬的執行資源，保留主機資料。

Kubernetes API ports：M0 16440、M1 16441、M2 16442、M3 A／B 16443／16447。這些讓 kubectl 連 API server，與 Hub UI 18090、價格服務轉送 18080／18081 用途不同。

M4 預設 A／B 也使用 16443／16447，獨立 M4 E2E 則使用 16450／16451；它們與 18094 的 Hub UI、5556 的 Dex issuer 分開。

M5 UI 使用 18095、Dex 使用 5557；預設 Worker 名稱仍為 hugofleet-a／hugofleet-b。正式 M5 E2E 的 API ports 為 16452／16453。切換時先用原階段及原 state 執行 down；新練習使用 HF_M5_STATE 指向新的 .local/tutorials/m5 子目錄，保留舊 DB／憑證／ownership。

## 本階段操作腳本在哪裡

| 階段 | 真正執行操作的 Bash |
|---|---|
| M0 | [scripts/control.sh](m0/scripts/control.sh)：編譯、映像建置／載入、原生 YAML 部署、轉送、Unit／benchmark |
| M1 | [scripts/control.sh](m1/scripts/control.sh)：Operator 編譯／安裝、CR 部署、型別／CRD 生成、envtest／Unit；[tools.sh](m1/scripts/tools.sh) 安裝工具 |
| M2 | [scripts/control.sh](m2/scripts/control.sh)：Hub 編譯、受限憑證 bootstrap、啟動、Unit |
| M3 | [scripts/control.sh](m3/scripts/control.sh)：Agent 編譯、憑證、Hub／Worker 組裝、Agent 安裝、Unit；[proto.sh](m3/scripts/proto.sh) 管理協定生成 |
| M4 | [scripts/control.sh](m4/scripts/control.sh)：OIDC／Dex、分離的 Worker bootstrap、Calico、Hub／Agent、真實 enrollment；[e2e.sh](m4/scripts/e2e.sh) 驗收／owned cleanup |

M5 的 [scripts/control.sh](m5/scripts/control.sh) 組裝 Dashboard、telemetry、Dex 與 Helm Workers；[scripts/e2e.sh](m5/scripts/e2e.sh) 驗證完整安全套件及 traces／Helm lifecycle。

每階段 run.sh 選定自己的學習叢集／執行資料，呼叫以上腳本。共用 kind 建立與 ownership 清理集中在 [scripts/kind.sh](../scripts/kind.sh)；[scripts/hf.sh](../scripts/hf.sh) 保留 Make 命令分派與跨階段開發命令。各階段 scripts/e2e.sh 保留完整驗收流程；驗收範圍見 [驗證總覽](../docs/validation.md)。
