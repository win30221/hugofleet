# M2 實測紀錄

本頁保留當次執行環境與命令。專案後來已搬移，現行路徑與啟動步驟見 [根 README](../README.md)／[M2 教學](../stages/m2/README.md)；以下舊路徑不代表搬移後的驗收。

## Windows 編輯器 `lockDatabase` 修復（2026-10-11）

原因：`lock_linux.go` 只參與 Linux 建置，Windows 的 Go 工具分析 `store.go` 時缺少 `lockDatabase`。新增 `lock_unsupported.go`（`!linux`）提供明確 Linux／WSL2 執行要求，拒絕在沒有程序鎖的情況下開啟 SQLite；Linux `flock` 路徑未修改。此修正僅使非 Linux 平台可編譯／分析，沒有新增原生 Windows Hub 執行支援。

本次實測使用固定 Go 1.27.2，在 Ubuntu WSL2 執行 `go test -count=1 ./stages/m2/hub/repository`（0.071 秒）、`go vet ./stages/m2/hub/repository`、`go build ./stages/m2/cmd/hub`，全部 exit 0。`GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c -o .local/repository-windows.test.exe ./stages/m2/hub/repository` 交叉編譯成功；PowerShell 執行 `& ./.local/repository-windows.test.exe '-test.run=^TestLockDatabaseUnsupportedPlatform$' '-test.v'` exit 0，確認非 Linux 回傳執行平台錯誤、不回傳有效鎖且不建立鎖檔。新增檔案已 gofmt。本次未重新執行 envtest／kind E2E。

## 原有 M2 驗收紀錄

日期：2026-10-11，Asia/Taipei。**M2 已完成本機驗收：Unit／Race／envtest／真實 kind E2E 與 owned cleanup 通過。** 初次完整成功 Run `20261011T101851-425201`；審查修復後最新成功 Run `20261011T121432-487519`，M0／M1／M2 完整回歸結果見文末。M3 尚未開始。

最小垂直切片：Go 模板 UI／REST → Service → 真實 SQLite WAL → durable Operation 背景執行 → 受限本機 kubeconfig → 現有 Application Operator → 真實 Deployment／Service／pricing-api HTTP。只做 M2，M3 Agent、多叢集、mTLS、M4 OIDC／租戶隔離未加入。

檔案變更：stages/m2/hub 的 domain／repository／service／kube／httpapi、stages/m2/cmd/hub、stages/m2/cmd/bootstrap-hub、web、stages/m2/config/hub、SQL migration、OpenAPI／ADR／README、Go DB／HTTP／kind 驗收與 make／CI 腳本；Operator 只增加共用 desired Deployment projection。

版本沿用 M1 Go 1.27.2、kind 0.33.0、Kubernetes／envtest 1.37.0、controller-runtime 0.25.1；新增純 Go modernc.org/sqlite 1.60.1／libc 1.77.1，x/sys 0.48.0／x/sync 0.23.0。詳見 ADR 0007；go.mod／go.sum 固定 checksum。

測試計畫：format／vet／Build／Unit、真實 SQLite migration／去重／CAS／回滾／終態／WAL／lock／restart、HTTP input／CSRF／Host／Origin、real envtest regression、Race，最後在獨立 owned kind 跑實際 Hub／UI／HTTP／部署／故障／回滾／quota／SIGKILL／GC。

已實際確認：

- `go mod tidy`、`go mod verify` 成功，所有 module 校驗通過。
- `make lint build test-unit verify-generated test-integration` exit 0；真實 TestEnvtest 203.99 秒，schema／CEL／defaults／status、更新／漂移、ownership、no-op 與清理通過。envtest 沒有 kubelet／Deployment controller，不用它宣稱 Pod Ready。
- 第一輪 `make lint build test-unit verify-generated test-race` exit 0；Go 容器 4 CPUs／4 GiB、network=none、唯讀來源。Controller real envtest + Race 80.968 秒；Hub domain／HTTP／SQLite／service 的 Race 通過。其後增加受限 kubeconfig 權限核對、UI 危險操作確認與 live observation query，因此再跑最終檢查。
- `make operator-image-build` exit 0，固定 base image digest；輸出 hugofleet-operator:m1 SHA256 110790c4b2a8fe807dd356c3e4b41310a9dce8bba1876c4e38b73895203798f2。Sample 三版沿用已驗證的 Go 建置映像；M2 E2E 必須實際載入與執行。
- `.local/validate-openapi.go` exit 0，OpenAPI YAML strict syntax 和 46 個本機 reference 可解析；這不是完整 OpenAPI schema validator。
- 最終 `bash .local/checks-m2.sh` exit 0：format／vet／Build／全部 Unit／verify-generated、全部 Unit + real envtest 的 Race、`go vet -tags=integration,e2e ./...`、module verify／OpenAPI syntax 均通過。最新 Controller envtest + Race 154.185 秒；Hub HTTP 14.656 秒、kube credential validation 1.067 秒、SQLite 65.495 秒、service 16.113 秒，沒有 data race。完整紀錄在 artifacts/m2-checks/final.log。

完整 envtest／Race／Build 檢查 stdout 保存 artifacts/m2-checks/final.log；各輪 kind E2E 保存 artifacts/m2-e2e/<run>，成功與失敗各自保留。

UID 恢復修正後 `bash .local/final-uid-checks.sh` exit 0：最新 format／全部 vet／Build、`go vet -tags=integration,e2e ./...`、所有 Hub 與 stages/m2/cmd/hub Unit + Race、`go mod verify` 通過。最新 Race：domain 1.025 秒、HTTP 16.411 秒、kube 1.056 秒、SQLite repository 83.602 秒、service 16.888 秒、stages/m2/cmd/hub 1.082 秒。沒有 data race。紀錄 artifacts/m2-checks/uid-final.log。Controller 沒有再修改，沿用前述完整 real envtest／Race 結果。

第四輪 `make test-e2e`，Run `20261011T101851-425201`：**exit 0，TestM2HubKind 505.31 秒，全部 Assertion 與 owned cleanup 通過。** 真實 UI v1／quote=8000、broken ProgressDeadlineExceeded／v1 持續可用、UI 新 revision 3 回滾、同 key 去重／改 payload 409、併發恰一筆 202 和一筆 409、RUNNING 時 SIGKILL 恢復原 command_id／deadline／CR UID／generation、quota FailedCreate／解除後新 revision、20 秒 TIMED_OUT／LATE_READY 保留失敗歷史、真實 SQLite acknowledgement 故障後 UID_BOUND／GC 恢復，以及最後 App DELETED／8 Release／9 Operation 再次重啟保留都通過。故障注入應用另有 1 Release／2 Operation；不混入 pricing-api 的計數。

第四輪原始日誌、實際 UI HTML、Operator 日誌、Kubernetes 資源／事件、kind 診斷與測試 SQLite 留在 artifacts/m2-e2e/20261011T101851-425201。Docker 第一次刪除仍出現 missing exit event；原有 exact container ID ownership 核對與重試成功，最終 make exit 0。清理後只剩原有 lab-control-plane（Up 38 hours）、consul（Exited 1）與 documents-postgres-1（Exited 0），SA kubeconfig 已移除。

清理前 Docker 單次採樣 node memory 855 MiB／15.48 GiB、CPU 32.11%；此時兩個應用都已刪除，只能作為該時點的採樣，不能當作 rollout 尖峰或整機總 RAM。此前叢集 cold bootstrap／映像 import 偏慢，預留 README 所列等待時間，不以調低驗收條件掩蓋。

第一輪 kind Run `20261011T090645-377314`：叢集／CoreDNS／Operator Ready，真實 Hub SA 權限測試通過，但 Hub 在啟動時發生 `flag redefined: kubeconfig`，所以整次判失敗（TestM2HubKind 182.13 秒），沒有部署成功的宣稱。根因是 controller-runtime 在全域 FlagSet 註冊 kubeconfig，Hub 又重複註冊；改用獨立 FlagSet，加入 CLI 重複解析／help／public-listen 拒絕測試，修正後重新驗收。原始 panic、日誌與 kind 診斷留在該 Run。

CLI 修正後 `bash .local/cli-checks-m2.sh` exit 0：lint／Build／全部 Unit／integration,e2e vet、stages/m2/cmd/hub Race（1.055 秒）和實際 `bin/hub --help` 通過；紀錄在 artifacts/m2-checks/cli-fix.log。

第二輪 kind Run `20261011T093121-399920`：kind bootstrap exit 1，API server 尚未啟動，Hub E2E 未執行。containerd 日誌顯示預載映像解壓 snapshot commit 被 context cancellation 中止；唯讀診斷顯示 I/O full avg60 約 47.7%、memory OOM／OOM kill 均為 0。既有固定 node 映像的 content 完整，但 apiserver／etcd 尚未 unpack。已輸出 bootstrap／kind 診斷，本次專用 node 限定清理成功，make 正確回報 exit 1，沒有降低任何 M2 Assertion。

另在 E2E 的 WSL／NTFS 檔案系統執行 `TMPDIR=/mnt/d/Dowload/HUGOFLEET_AI_STARTER/.local/tmp-m2 go test -count=1 -run TestWALMigrationLockAndRestart -v ./stages/m2/hub/repository`，exit 0，真實 WAL／migration／程序鎖／Pending operation 重啟保留通過（11.01 秒）。晚到 Ready E2E 使用 20 秒 deadline，先斷言 CR 已寫入且 Operation RUNNING，再等待 TIMED_OUT；不以尚未派送的操作充當晚到完成。

程式檢查另發現 create 已到達 Kubernetes、但 Hub 尚未保存 UID 就中斷的刪除邊界。修正為先交易保存恢復出的 UID 和 UID_BOUND 事件，再進行 CR 刪除與實際 GC；不改 applied_revision／last_successful_revision。`bash .local/deletion-fix-check.sh` exit 0，repository／service／kube／HTTP 全部 Unit、相關 vet／Hub Build 通過。紀錄 artifacts/m2-checks/deletion-fix.log。新增 E2E 以真實 SQLite trigger 阻止 create UID acknowledgement，仍建立真實 CR／Deployment，超時後解除注入並驗證刪除恢復。第三輪尚未完整通過，修正 schema 注入流程後已於第四輪完整驗證；最終使用當前來源重新建置的 Hub 與 Go E2E。

第三輪 kind Run `20261011T094733-404792`：叢集、CoreDNS、Operator 和 Hub 均 Ready。真實受限 RBAC、UI v1／quote=8000、broken FAILED／上一版仍可用、UI 回滾 revision 3、重送去重／併發 202+409、SIGKILL 續接原 command_id／deadline／CR UID 和 generation、quota FailedCreate／新版本恢復、revision 7 TIMED_OUT／LATE_READY 保留終態皆已通過。故障注入也建立真實 workload 且保持 UID 未 acknowledgement；但測試在 Hub 仍持續寫入時移除 SQLite trigger，遇 SQLITE_BUSY，所以整次 TestM2HubKind 判失敗（495.78 秒），尚未驗證刪除與最終履歷。修正測試為 Hub 停止時建立／移除 trigger，測試 SQL 連線上限 1，不改產品成功條件。原始日誌／HTML／kind 診斷留在該 Run。Docker 第一次刪除未收到 exit event，原有 ownership 核對與重試成功移除本次 node，整次 make 保持 exit 1。

`go run .local/sqlite-ddl-probe.go` 在停止 Hub 的第三輪 DB＋WAL 複本上通過，exit 0；可讀完整 2 App／8 Operation，trigger 建立／移除正常。最初只複製 .db 時讀到舊 checkpoint 的 1 App／6 Operation，該探測判失敗；加入對應 WAL 後才確認完整狀態。這只驗證測試 schema 恢復方式，不作為備份還原或 E2E 通過的證據；原始 DB／WAL 未修改。紀錄 artifacts/m2-checks/ddl-probe.log。

第三輪期間另以 in-app browser 讀取真實 Hub 清單／詳細頁與完整畫面，確認表單、目前觀測、desired／applied／last successful、失敗原因、來源 revision、危險操作確認、Release／Operation／事件履歷可顯示；未用靜態 mock 頁面驗收。窄視窗表格可水平捲動。瀏覽器只讀，表單提交仍由 Go E2E 實際執行。

已知限制：僅 Linux／WSL、單一 Hub／單一開發租戶／單 kind、loopback 固定開發管理員；SA token 約 8 小時後要重新 bootstrap 和重啟。UI 最新 50 筆歷史／API 詳細頁至多 100 筆；更舊歷史仍保留 DB，但尚無搜尋／匯出。SQLite 需要本機檔案系統，優先 WSL ext4，不用網路共享。

只驗證 Hub 程序與 Operator 程序恢復，不代表 WSL／主機斷電／Docker node restart／backup restore 通過。kind 沿用 RAM etcd，不能用於持久性保證；GitHub hosted CI 未執行。`kubectl port-forward service` 選定一個 Pod，不能當作 kube-proxy／CNI 一般路由的測試。

## M0／M1／M2 審查後修復（2026-10-11）

本次使用者授權修復審查發現的四項問題，範圍停止於 M2。最小切片是「Kubernetes 已接受 CR，但 SQLite 尚未記錄 acknowledgement 就中止，重啟時原 deadline 已過」仍能唯讀恢復目前觀測；另保存並顯示目前故障、補齊三階段 CI 與 OpenAPI 事件契約。

變更包含 domain／repository schema 2 migration／runner／Kubernetes observation／UI／OpenAPI、CI matrix、Go 回歸測試及既有 README／API／ADR。migration 以交易保存 schema 1 的 App／command／事件資料；不允許 foreign UID 綁定或舊 observation 覆寫新的意圖。故障原因與歷史終態分開，輪詢同一故障不累積重複事件，恢復後再次故障可再次記錄。UID_BOUND 已加入契約，並以實際程式事件輸出檢查 OpenAPI enum。

新增 CI 的 M0 job 另揭露 `test-e2e-m0` 原本依賴既有 bin/m0check。修復 M0 腳本在建立叢集前自行編譯 Go 驗收工具，使乾淨 checkout 不依賴手動先跑 Build；M0 應用程式與驗收 Assertion 未變更。

修改前新增的 `TestRestartAfterDeadlineStillObservesUnacknowledgedCR` 與 `TestObservationFaultSurvivesRestartWithoutRewritingSuccess` 均失敗：前者 Observe 次數為 0，後者未保存故障原因／事件。修改後 `go test -count=1 ./stages/m2/hub/... ./stages/m2/cmd/hub` exit 0。

後續 `make lint build test-unit verify-generated`、`go vet -tags=integration,e2e ./...`、`go mod verify` 均 exit 0。測試包含真實 SQLite schema 1 → 2 升級及 lifecycle 去重、故障原因跨重啟／重複故障、UID／新意圖保護、過期恢復保持 command／deadline／FAILED／TIMED_OUT／last successful，以及真實 HTTP UI 目前原因與 HTML escaping。

`make test-race test-integration` 兩個 target 都成功，已進入後續映像建置。Race 使用固定 Go container、network none／4 CPU／4 GiB，包含所有 Unit 與真實 envtest；controller 172.899 秒、repository 157.411 秒，沒有 race report。獨立 envtest `TestEnvtest` 9.55 秒、exit 0。新增 E2E 故障情境經 `go vet -tags=e2e ./stages/m2/test/e2e` 再次檢查通過。

`make demo-image-build operator-image-build` 成功；隨後 `make test-e2e-m0` exit 0，Run `20261011T113245-453358`。從零建立叢集，v1／v2-good／v2-broken 真實 HTTP、quote=8000、壞版 readiness=false／無 liveness restart、Service 保留可用 endpoints 並排除壞 Pod、回滾 v1 與 owned cleanup 均通過。Docker 第一次清理回報未收到 exit event，原有 ownership 核對與第二次刪除成功；不是略過清理。原始紀錄 `artifacts/m0-e2e/20261011T113245-453358/run.log`。

`make test-e2e-m1` exit 0，Run `20261011T115041-467968`。`TestM1KindLifecycle` 321.36 秒，初版／同配置新 revision／純縮放／image 更新、Deployment／Service 漂移修復與刪除重建、Operator 程序重啟、broken ProgressDeadlineExceeded／新 revision v1 恢復／真實 GC 全部通過；`TestM1Authorization` 0.06 秒，必要權限可用，Secrets／exec／namespace／跨 namespace 寫入受拒絕。Docker 清理仍經 ownership 核對後第二次刪除成功。原始紀錄 `artifacts/m1-e2e/20261011T115041-467968/run.log`。

`make test-e2e` exit 0，Run `20261011T121432-487519`，`TestM2HubKind` **608.30 秒**，全部既有與新增 Assertion 及 owned cleanup 通過。新增真實 CR SPEC_DRIFT／SQLite 原因保存／Hub 重啟後 UI 顯示／原因清除與恢復事件通過；新增真實 SQLite acknowledgement 故障後 SIGKILL、超過原 deadline 才重啟，唯讀觀測恢復 exact UID／applied revision／READY，CR UID／generation、command_id／deadline、TIMED_OUT／FAILED／last successful 均保留，LATE_READY 恰一筆。既有 UID_BOUND 刪除恢復仍保留 applied=0，真實 GC 通過。

原有 UI／quote、壞版／手動回滾、去重／併發 409、RUNNING crash recovery、quota、晚到 Ready、最終 DELETED／8 Release／9 Operation 與重啟履歷皆通過。兩個 acknowledgement 故障應用各有 1 Release／2 Operation，不混入 pricing-api 的計數。Docker 初次清理未收到 exit event，ownership 核對後第二次刪除成功，整個 `make test-e2e-m0 test-e2e-m1 test-e2e` exit 0。紀錄 `artifacts/m2-e2e/20261011T121432-487519/run.log`；GitHub hosted CI 尚未執行。

本次修復停止於 M2。SQLite schema 2 不可直接以舊 Hub 開啟；回退需使用升級前 SQLite-aware 備份，不可手動降低 schema version。備份還原／主機斷電／Docker node restart／disk profile 的驗證限制仍沿用上方紀錄。

## 2026-10-14 M5 共用程式回歸

上方保留原始 M2 驗收與 schema 2 當時狀態；目前共用 module 的最新 schema／Controller 決策見 [M5 紀錄](m5-validation.md) 與 [ADR 0012](adr/0012-current-revision-replicaset-evidence.md)。最新 Operator 先通過 M1 與全新 M5 正式驗收後，再回歸 M2。

首輪 make test-e2e Run artifacts/m2-e2e/20261014T084505-69644，TestM2HubKind **FAIL 284.66 秒、exit 1**。已完成受限 RBAC、UI v1／quote、實際 CR drift 與持久原因、broken FAILED／上一版仍服務、UI rollback revision 3 SUCCEEDED；後續 HTTP 檢查仍使用最初舊 Pod 的 forward，新 revision 已替換 Pod，因此 forward.log 回報 lost connection to pod，連線拒絕。現場 CR observedRevision=3／RolloutComplete 保留在 resources.yaml。正式 cleanup 已移除 owned cluster；不以成功的 Operation 冒充整輪 E2E 通過。

只修正 E2E 轉送 lifecycle：回滾成功後停止／reap 本次建立的程序，再建立新 Service forward。原版本與完整價格結果 assertions 保留；README 的 rollout 後重建轉送規則不變，產品程式未改。tagged vet exit 0，正式重跑結果待補；M1／M5 不因這項測試工具修正重複執行。

正式重跑 make test-e2e **PASS，exit 0**，TestM2HubKind 233.20 秒，Run artifacts/m2-e2e/20261014T085921-79540。原受限 RBAC／真實 UI與 quote、漂移／持久原因、broken／新 revision rollback／quote、去重／併發 202+409、RUNNING SIGKILL／原 command_id+deadline 復原、quota Admission／恢復／FAILED 歷史、TIMED_OUT 後 LATE_READY 不改終態、兩種 SQLite acknowledgement 故障的精確 UID／generation／晚到觀測、delete／真實 GC／最後重啟履歷全部通過。正式 cleanup 移除 owned cluster，外層其他容器名稱／ID／狀態比對不變；artifacts/m5-development/final-m2-m4-forward-regressions.log 保留連續回歸結果。Hosted CI 未執行。
