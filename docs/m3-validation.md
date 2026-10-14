# M3 實測紀錄

日期：2026-10-11，Asia/Taipei。M3 功能實作及搬移前的完整雙叢集 E2E 已通過；下文保留各輪實測範圍。**目前重新驗證已依使用者指示暫停，先交付資料夾與文件整理供確認。搬移後未完成的入口驗證不列為通過。**

最小垂直切片：UI／REST + Demo Token → SQLite durable Operation → Protobuf mTLS gRPC bidi → Worker Agent → Application CR → 原有 Operator → Deployment／Service／真實 pricing-api。

變更：stages/m3/api/proto 與生成 Go、stages/m3/agent／agentprotocol／agenttls／devcert、Hub remote transport／registry／schema 3／token登入、stages/m3/cmd/agent／certs-dev／proto-tool、Dockerfile／Agent RBAC、M3 bootstrap／E2E、各階段獨立捷徑、教學／ADR／README／CI。

測試計畫：全部 Unit／vet／Build／generated proto與 CRD 一致性、真實 TLS CA／SAN／到期／identity／duplicate session／heartbeat拒絕、race／envtest回歸、真實雙 kind 的部署／HTTP／Agent離線仍由Operator修復／重連／持久化去重／過期刪除／GC／Hub重啟／晚到Ready；最後 M0／M1／M2各自E2E回歸。

初次 Unit 的 TLS負案例在 Connect 建立串流時已遭拒絕，原測試helper只接受 Recv時失敗，造成測試helper誤判。修正負案例接收Connect／Send／Recv任一握手拒絕，不變更必須拒絕不可信CA／錯誤SAN／到期／identity的Assertion。初次履歷測試亦揭露同毫秒UUID排序不穩，改用SQLite rowid保留寫入順序。

後續實測命令與結果於驗收後追加；未執行／失敗的測試不列為通過。

首輪 E2E 在部署前失敗：執行中的 `scripts/hf.sh` 被更新，Bash 在既有 file cursor 讀到更新後內容而解析失敗。Sample 三版與 Operator 映像已建置；Hub／Worker 尚未建立。本輪 E2E exit 2，不列通過；診斷／限定清理確認兩個測試 kind 均不存在。後續固定執行中的腳本，不於驗收過程修改同一檔案，修正後重跑。

`bash .local/final-m3-checks.sh` 已 **exit 0**。`make lint build test-unit proto-check verify-generated`、`go vet -tags=integration,e2e ./...`、`go mod verify`、OpenAPI strict YAML／46 個 local reference、`make test-race test-integration` 全部通過。記錄 `artifacts/m3-checks/final.log`。

Race 使用既有固定 Go container（network none、來源唯讀、4 CPU／4 GiB）：Agent 1.625 秒、Agent transport 2.217 秒、Controller + 真實 envtest 260.068 秒、HTTP 36.741 秒、SQLite 213.902 秒、service 50.763 秒；沒有 data race。獨立 envtest TestEnvtest 12.10 秒（整 package 12.136 秒），schema／status、更新／漂移、foreign ownership／no-op 回歸通過。這不代表 Pod readiness 或 Kubernetes GC 已驗證，後者由 kind E2E 驗收。

實際 TLS／雙向串流 Unit integration 已確認不可信 client CA、錯誤 Server SAN、到期 client、未登記 URI identity／Hello 身分不符、live duplicate session／heartbeat expiry／重新握手受正確處理；實際 Agent Client 在耗時命令執行期間維持心跳。這些測試使用真實 TCP／TLS／gRPC；其中 Workload backend 的 unit fixture 不作為多叢集驗收證據。

第二輪雙 kind E2E（artifacts/m3-e2e/20261011T142507-540510）在最後晚到 Ready 案例失敗，exit 1。先前雙 Worker Pod 主動 mTLS、真實部署／價格 HTTP、離線修復／B 獨立更新、restricted SA、ACK loss／重啟 ledger／stale revision／expired delete／tombstone、Hub 重啟與壞版回滾 Assertion 均通過；整輪仍不列通過。原因是測試使用 Pod ResourceQuota 暫停 readiness，但 Kubernetes 即時回報 FailedCreate，Operation 正確成為 FAILED，無法期待 TIMED_OUT。改為 cordon 此次測試的單一 owned Node，既有 Operator／Agent 保持執行、新 Pod 真實 Pending／Unschedulable，逾時後恢復 scheduling；不變更終態與晚到結果的 Assertion。cleanup 遇 Docker missing exit event，ownership 核對後重試成功，兩個測試 kind 已移除。

教學拆分後，`bash .local/folder-checks.sh` 已 exit 0：`make lint build test-unit proto-check verify-generated docs-check`、含 integration／e2e build tags 的 vet、module verify 通過，記錄 artifacts/m3-checks/folders.log。當次 docs-check 核對 21 份 Markdown、97 個本機檔案連結、73 個 Bash／sh 範例區塊及三份型別化 DeployRequest。Shell 區塊檢查僅代表語法正確，不替代實際啟動與 HTTP 驗證；後續文字整理還會重跑 docs-check。

`bash .local/folder-path-check.sh` exit 0，確認 M0／M1／M2 與兩個 M3 Worker 的 kubeconfig／ownership 路徑各自隔離、既有驗收叢集路徑保留，以及拒絕工作區外的 M3 state。未修改或清理非本專案 Docker 資源。教學清理指令已改為區分固定學習入口（m0／m1-down）與 E2E 指定 HF_CLUSTER 的 kind-down；M0 預期失敗的 rollout 以 if 保留終端。各指南增加階段理解檢查。

第三輪 `make test-e2e-m3` 已 **exit 0**，`TestM3DualKindOutboundAgents` **PASS 669.69 秒**，記錄 artifacts/m3-e2e/20261011T152809-584709/run.log。兩個真實 Worker Pod 向 Hub 建立 mTLS gRPC；A／B 部署 SUCCEEDED、HTTP v1／quote 8000；A Agent 停止後 UNKNOWN／離線新請求拒絕、當地 Operator 修復、B 獨立發布 v2-good；重連後 CR UID／generation 不變。真實受限 SA、ACK loss／ledger 重啟、舊／衝突 revision、過期刪除、UID 刪除／實際 GC／tombstone，以及 Hub 重啟後 command_id／deadline／generation 保留、壞版 FAILED／回滾 v1 均通過。

晚到 Ready 案例確認新 Pod 因 cordon 真實 Pending／Unschedulable，Operation 在原 deadline 後 TIMED_OUT；uncordon 後目前 Application 回到 READY，但 Operation／FAILED Release／最後成功 revision 與 CR UID／generation 維持原意。A／B 最後的 Hub 刪除操作及 GC 通過。Hub inspect 確認只有 DB、Hub CA／server cert／server key／Token 五個 mount，沒有 Worker kubeconfig；管理 API 未授權請求被拒絕。兩個 kind 清理均遇 Docker missing exit event，ownership 核對後第二次移除成功，Hub 與兩個驗收 kind 已清理；整輪 PASS 包含 owned cleanup。

M0～M2 的本次回歸與四個隔離教學入口的實際啟動驗證接續執行；尚未將它們列為本次通過。

使用者補充要求後，最終目錄集中為 stages/m0～m3：各自的 Go 程式、YAML／JSON、Bash、Dockerfile、測試及完整 README 教學放在同一階段，README 另有新增功能與程式導讀；共用 Go 元件保留一份、M3 修改的 M2 檔案明列。原 tutorials 教學合併至各階段 README；docs 保留 API／ADR／驗收證據。整併先通過 SHA-256 核對 135 份快照，取代的舊檔案存入 .local/stage-collection-backup-20261011/archived。自動審核曾拒絕含移除舊檔的初稿，改為核對備份與可還原 archive 的安全方案後成功執行。

搬移後第一次階段 Unit 檢查在 M2 HTTP 契約測試失敗：../../../docs/openapi.yaml 已不符合新目錄深度。只將路徑改為 ../../../../docs/openapi.yaml，保留 event enum 與實際發送事件的完整 Assertion，重新執行全部階段檢查。

`bash .local/stage-layout-checks.sh` 已 **exit 0**：`make lint m0-build m1-build m2-build m3-build m0-test m1-test m2-test m3-test`、`make proto-check verify-generated docs-check test-integration`、含 integration／e2e tags 的 vet、`go mod verify`、OpenAPI YAML／46 local refs 全數通過。記錄 artifacts/m3-checks/stage-layout-checks.log。docs-check 核對 24 份 Markdown、207 個本機連結、78 個 Shell 範例及三份型別化部署 JSON；獨立 TestEnvtest PASS 80.54 秒（整 package 80.575 秒）。OpenAPI refs 檢查不是完整 OpenAPI schema validator。

M0 本次真實 kind E2E 已 PASS（含 owned cleanup），記錄 artifacts/m0-e2e/20261011T162340-628843/run.log：三版 HTTP／quote、broken readiness／rollout／Service endpoints、恢復 v1 全數通過。此輪是在程式搬移前執行；為了套用使用者新增的分類要求，待 M0 完成及清理後刻意停止父 Make 排程，因此舊 remaining-validation driver exit 1 不代表 M0 失敗，尚未執行的 M1／M2／教學入口改由新目錄重跑。

`bash .local/final-kind-validation.sh` 正在新目錄執行 M1／M2 kind 回歸及四個獨立教學入口。先前完整 M3 E2E 的 PASS 是目錄搬移前的實測證據；搬移後會另外核對新 Bash／Docker 路徑及真實雙 Worker 的教學 JSON／HTTP。尚未把進行中的項目列為通過。

新目錄 `make test-e2e-m1` 已 PASS（含 owned cleanup），記錄 artifacts/m1-e2e/20261011T165806-659727/run.log。TestM1KindLifecycle PASS 334.55 秒：真實 CR／Pod／HTTP、revision、replicas、image 更新、Deployment／Service 漂移與重新建立、Operator 重啟、壞版 readiness／ProgressDeadlineExceeded、恢復 v1、刪除及 Kubernetes GC。TestM1Authorization PASS 0.09 秒：允許本 namespace 工作負載操作，拒絕 Secret／exec／namespace／跨 namespace。測試 client 的 logger 提示沒有影響 Assertion；Docker 清理 missing exit event 在核對 ownership 後重試成功。

新目錄 `make test-e2e` 已 PASS（含 owned cleanup），記錄 artifacts/m2-e2e/20261011T171602-672290/run.log。TestM2HubKind PASS 567.05 秒：受限 SA、UI／REST v1 與真實 quote=8000、漂移的目前原因與觀測恢復、壞版／回滾、冪等／衝突、Hub 重啟與 SIGKILL、配額拒絕、原 deadline 與終態保留、晚到 Ready、lost acknowledgement／SQLite UID 回應中斷的 exact UID 恢復、實際刪除／GC 及 durable history 全數通過。Docker 清理第二次成功；學習入口的 UI／服務 ports preflight 通過，開始從各階段資料夾執行 run.sh。

資料夾入口 smoke 記錄 artifacts/learning-smoke/20261011T173916-684090/run.log。M0 已 PASS：在 stages/m0 執行 bash run.sh up／forward／down，從新源碼路徑重建三個 pricing-api 映像、使用本資料夾 workload.yaml、核對專用 kubeconfig 路徑、確認沒有 Application CRD、真實 HTTP v1／quote=8000，最後 owned cleanup 成功。背景轉送 log 建立前的一次 grep 提示未影響等待與 HTTP Assertion；M1／M2 入口仍接續執行。

M1 資料夾入口已 PASS：在 stages/m1 執行 bash run.sh up／forward／down，新的 Operator Docker COPY／build 路徑成功；獨立 hugofleet-m1-demo、使用本資料夾 application.yaml，CR observedGeneration／Ready、HTTP v1／quote=8000，replicas=0 漂移被修復為 1 且 Pod 回到 Ready；owned cleanup 成功。M0 已先關閉，M1 能自行準備所需元件。

使用者要求先整理到可確認的程度，再恢復驗證。已停止後續排程及進行中的 M2 資料夾入口 smoke；該入口尚未完成，不列為 PASS。搬移後的 M3 資料夾雙 Worker smoke 尚未開始；先前完整 M3 E2E 的證據仍是搬移前的那一輪。停止排程時一段 WSL 內嵌指令的引號解析錯誤曾意外啟動重複的 M1 driver，已取消並清理其 owned kind；該輪取消不列為功能測試結果。共用 final-kind-validation.log 曾被重複 driver 覆寫，M1／M2 已完成的實測結果以各自上方列出的獨立 run.log 為準。

取消後的 owned cleanup 已完成：hugofleet-m2-demo 與意外啟動的 hugofleet-m1-e2e 均已移除，沒有本次驗證的 Hub／Worker 容器或排程程序留下；非專案 lab-control-plane 仍執行中。收尾僅補充目錄索引／目前工作指示、明確標示 M3 詳細命令從專案根目錄執行、簡化同資料夾連結，以及移除 hf.sh help 文字前誤留的加號。這些最後整理未重新執行 docs-check、Build 或測試，等待使用者先確認資料夾與文件。

使用者接著授權「該整理的整理、該清除的清除」，以便關注重要內容。本輪將各階段操作 Bash 歸位、共用 kind lifecycle 分出、精簡根 README、合併階段導航，補上共用與本機產物目錄的用途說明。50 項歷史資料歸檔，12 項已確認可再生的殘留移除（檔案內容約 154.3 MiB）；記錄位於 .local/archive/maintenance/20261011/cleanup.log。正式程式、既有測試 Assertion、原 artifacts 證據與 DB／憑證保留。舊 .local 工具／備份／state 現在依 archive/README 分類查找，歷史命令保留當時原意。

這次只做檔案整理／清理與人工閱讀，沒有執行編譯、docs-check、Unit／envtest／kind E2E 或 Demo。階段入口／共用 dispatcher 拆分後的驗證仍待使用者確認並授權；先前 PASS 不等同於本次腳本已驗證。

## 2026-10-14 M5 共用程式與入口回歸

上方整理時點保留；使用者後續已授權恢復 M0～M3 驗證並完成 M4／M5，見 [重新驗證](revalidation-20261011.md) 與 [M5 紀錄](m5-validation.md)。最新共用 Operator／schema 5／Proto 投影及 IPv4 gateway 選取修正後，make test-e2e-m3 **PASS，exit 0**，TestM3DualKindOutboundAgents 375.42 秒。

兩個真實 Worker Pod 的主動 mTLS gRPC 通道、Hub 無 Worker kubeconfig、雙叢集 v1／quote=8000、A 斷線時 UNKNOWN 保留觀測時間與 Operator 自主修復、B 繼續發版／quote、ACK loss／Agent restart durable ledger／stale-conflicting revision／expired delete／GC+delete tombstone replay、重連不重複 Apply、Hub restart／broken FAILED／新 revision rollback／HTTP、真實 Pending Pod／TIMED_OUT／LATE_READY 保留終態及最後 GC 全部通過。

正式入口移除 owned Hub／雙 Workers，私有 state 保留，外層其他容器名稱／ID／狀態比對不變。原始日誌 artifacts/m3-e2e/20261014T091210-89377/run.log、artifacts/m5-development/final-m2-m4-forward-regressions.log。最新程式的 M1／M2／M5 正式門檻也已通過；M4 接續回歸，Hosted CI 未執行。
