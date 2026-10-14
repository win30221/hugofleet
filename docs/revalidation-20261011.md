# M0～M3 搬移後重新驗證

開始日期：2026-10-11，Asia/Taipei；驗證持續至 2026-10-12。使用者已授權恢復測試，並要求從根目錄開始核對所有 README。這份紀錄保存本輪結果，舊 m0～m3-validation 不覆寫為新的 PASS。

目前狀態：**M0～M3 本輪檢查、修正與重新驗證完成**。四階段正式 E2E、各 README 獨立操作流程及清理均已通過；未執行的可選項目見「實測邊界」。

## 範圍與方法

- 逐份閱讀根 README、stages 導航、M0～M3 操作指南、五份元件 README、scripts／tools／deploy／docs README，共 15 份正式 README；另核對 `.local/README.md` 的資料說明。
- 從正式 Make／stages 入口執行固定工具、編譯、Unit、race、envtest、生成檔一致性與四階段真實 kind E2E。
- 另外依各階段教學執行獨立啟動、HTTP／REST、更新／故障／恢復、資料保留與停止流程；使用新的 M2 DB／M3 state，保留原學習履歷與憑證。
- 本份報告只記錄 M0～M3 的文件檢查、必要問題修正與重新驗證，不 commit／push、不執行 hosted CI。使用者已另授權在這些檢查與修正全部完成後依序實作 M4、M5，後續各階段另外記錄驗收，不提前列為完成。

## 環境與前置結果

新專案路徑：`C:\Users\H\Documents\GitHub\win30221\hugofleet`；WSL：`/mnt/c/Users/H/Documents/GitHub/win30221/hugofleet`。Ubuntu WSL2，Linux amd64，kernel 6.18.40.1，非 root UID 1000；Docker client／server 24.0.6、Linux containers。

`make demo-image-build operator-image-build` 已 exit 0，從目前 stages 的 Dockerfile 重建三個 Sample 版本與 Operator。原 `lab-control-plane` 及非專案容器保留。

初次 `make tools` 的 Go 官方 archive 已下載並通過固定 SHA-256；kind release asset 在 300 秒只收到 2,175,607／10,530,942 bytes，curl 28 逾時後開始從零重試。停止本輪該下載程序，該次 Make exit 1（Error 143），**不列為成功**。改用既有 `HF_KIND_SOURCE=1 make tools` 已 exit 0，安裝同一固定 kind 0.33.0；來源方式依 [kind 官方說明](https://kind.sigs.k8s.io/docs/user/quick-start/#installing-from-source)，未改版號或跳過校驗。

`make doctor` 已輸出 Go 1.27.2、kind 0.33.0、kubectl 1.37.1 與 Docker Linux，環境檢查成功。

## README 與文件檢查

既有 docs-check 未涵蓋 scripts／tools／deploy README，也未檢查章節連結。已擴大正式 Markdown 掃描範圍，新增 ATX 標題／中文／重複標題與 fenced code 排除的章節驗證，以及缺檔／缺章節回歸案例；未弱化原有 Bash／型別化 JSON 檢查。

初輪 `make docs-check` 已 exit 0：31 份 Markdown、426 個本機檔案／章節連結、84 個 Bash／sh 區塊、三份型別化部署 JSON。更新 M2 的 quota 操作說明與測試後，`make lint docs-check` 再次 exit 0：32 份 Markdown、442 個連結、85 個 Shell 區塊與三份 JSON，日誌為 artifacts/revalidation-20261011/docs-updated.log。更新 M3 E2E 結果與最新授權後，`make docs-check` 已 exit 0：32 份 Markdown、443 個連結、85 個 Shell 區塊與三份 JSON，日誌為 artifacts/revalidation-20261011/docs-after-m3.log。這只證明連結／語法／輸入檢查，不代表 Demo 通過。

正式 README 核對清單如下；每份已逐讀並對照目前程式／腳本。執行狀態以後面的結果表為準。

| README | 核對內容 |
|---|---|
| 根 README、stages/README | WSL 入口、新路徑、各階段可獨立啟動、命令分派與切換／重建資料 |
| stages/m0、m0/pricing-api | 原生 Deployment、HTTP／計價、探針、映像版本、本機 go run |
| stages/m1、m1/controller | CRD／生成入口、revision／status、更新／漂移／重建／GC、測試邊界 |
| stages/m2、m2/hub | 受限憑證、UI／REST、SQLite、完整 Release、冪等／409、故障／回滾／重啟 |
| stages/m3、m3/agent、m3/agenttransport | Worker 主動通道、mTLS、Token 登入、ledger／離線／重連、Hub state／容器恢復 |
| scripts、tools、deploy | 固定版本與 source fallback、工具與快取、docs-check 範圍、kind ownership 與 RAM／disk profile |
| docs | 現行操作文件與歷史證據的分工、API／ADR／validation 導覽 |

另讀 `.local/README.md`、`.local/archive/README.md` 與搬移歸檔索引；archive 裡的舊 README 快照保留為歷史，不作目前可執行教學。已修正正式文件仍描述「暫停驗證」的狀態，並補上 kind 下載逾時後使用相同固定版源碼的既有入口。`make help` 已實際 exit 0，列出的命令與根 Makefile／hf.sh 一致。

另補齊 M0 壞版 Pod 轉送與 M1 ownership／rollout 診斷片段的新終端設定：先進專案、指定該階段 HF_CLUSTER、清除 HF_LEARNING_ROOT，再 source common.sh，避免只指定 kubectl context 卻沿用其他叢集 kubeconfig。M0／M1 主流程已使用相同隔離設定驗證，補充片段的 Shell 語法會由最終 docs-check 再檢查。

## 功能驗證

| 項目 | 本輪結果 | 證據 |
|---|---|---|
| M0～M3 lint／獨立 build／Unit／M0 benchmark | PASS，exit 0 | artifacts/revalidation-20261011/baseline.log |
| 共用 build／Unit、CRD／RBAC 生成檔、tagged vet、Go modules | PASS，exit 0 | artifacts/revalidation-20261011/shared-checks.log |
| envtest／Proto 固定工具安裝 | PASS，exit 0 | artifacts/revalidation-20261011/dev-tools.log |
| Proto 一致性、真實 envtest | PASS，exit 0 | artifacts/revalidation-20261011/integration.log |
| race | PASS，exit 0 | artifacts/revalidation-20261011/race.log |
| M0 kind E2E | PASS，含 owned cleanup | artifacts/m0-e2e/20261011T202202-756229/run.log |
| M1 kind E2E | PASS，含 owned cleanup | artifacts/m1-e2e/20261011T203833-806822/run.log |
| M2 kind E2E | PASS，exit 0，含 owned cleanup；保留前兩輪 FAIL | artifacts/m2-e2e/20261011T215044-891551/run.log |
| M3 雙 kind E2E | PASS，exit 0，含 owned cleanup；功能 assertions 456.85 秒 | artifacts/m3-e2e/20261011T221953-919702/run.log |
| M0 README 獨立操作流程 | PASS，exit 0，含 owned cleanup | artifacts/revalidation-20261011/m0-walkthrough.log |
| M1 README 獨立操作流程 | PASS，含 owned cleanup；保留首輪 bootstrap FAIL | artifacts/revalidation-20261011/m1-walkthrough-serial.log |
| M2 README 獨立操作流程 | PASS，含 owned cleanup，使用新的 DB | artifacts/revalidation-20261011/m2-walkthrough-serial.log |
| M3 README 獨立操作流程 | PASS，exit 0，使用新的 state，含 owned cleanup | artifacts/revalidation-20261011/m3-walkthrough-serial.log |
| Windows 實際瀏覽器 M3 登入／狀態畫面 | PASS，管理 Token 登入與重啟後 A／B CONNECTED、READY／revision 顯示 | artifacts/revalidation-20261011/m3-dual-ready-browser.jpg |
| pricing-api README 本機 go run／HTTP／VIP 計價 | PASS，exit 0 | artifacts/revalidation-20261011/pricing-host.log |

M0 已驗證三映像的 health／readiness／version／quote=8000、v2-broken Running 但不 Ready 且無 liveness restart、ProgressDeadlineExceeded、Ready endpoints 排除壞版、上一健康版仍可服務、恢復 v1。Docker 第一次清理回報 missing exit event；正式 kind.sh 在核對相同 ownership 後第二次刪除成功，E2E 最終 exit 0，沒有殘留本次 M0 叢集。

`make lint docs-check m0-build m0-test m1-build m1-test m2-build m2-test m3-build m3-test benchmark` 已完整 exit 0。包含 gofmt 檢查、go vet、各 Bash syntax、四階段主機執行檔與 Unit；M3 Unit 使用真實 TLS gRPC 握手，負案例拒絕不可信 CA／SAN／到期／identity，測試仍不替代雙 kind E2E。另 `go test -count=1 ./tools/doccheck` 已 PASS。M0 BenchmarkCalculate 此機結果 3.968 ns/op、0 B/op、0 allocs/op，僅為本輪單次量測。

`make build test-unit verify-generated`、`go vet -tags=integration,e2e ./...` 與 `go mod verify` 已 exit 0。`make operator-tools envtest proto-tools` 已 exit 0，真實 envtest archive 與 protoc 安裝完成。

`make proto-check test-integration` 已 exit 0。envtest 啟動真實 API server／etcd，驗證 schema／defaults／status subresource、更新／metadata repair／無重複寫入、外來 Deployment／Service／舊 UID 拒絕接管；Pod Ready 與 GC 仍由 kind 驗證。一次 Kubernetes resourceVersion conflict 經正常 Reconcile 重試收斂，未移除 assertion。

`make test-race` 已 exit 0。WSL 沒有 C compiler，正式入口使用固定 Go image、無網路、唯讀 source、4 CPU／4 GiB 容器，執行全部 Unit 與真實 integration envtest；未回報 data race。

M1 `make test-e2e-m1` 已通過完整 Application 生命週期、同配置新 revision、副本／映像更新、Deployment／Service 漂移與刪除重建、Operator Pod 重啟後修復、壞版真實 HTTP／探針／Degraded／Ready endpoints、恢復及 CR 相依資源 GC。實際 SubjectAccessReview 驗證必要權限與 Secrets／pods exec／跨 namespace 拒絕。生命週期 assertions 421.49 秒，RBAC 0.25 秒；bootstrap、映像載入與清理時間另計。kind 清理同樣遇到 Docker missing exit event，第二次 ownership 核對後刪除成功；最後輸出含 cleanup 的 PASS。E2E harness 有 controller-runtime 未初始化 logger 的非阻塞警告，assertions 與 Operator 日誌不受此警告影響。

M0 教學入口 `make m0-up／m0-forward／m0-down` 已 exit 0：獨立 kubeconfig、無 Application CRD、v1 HTTP、v2-good／兩副本、壞版 ProgressDeadlineExceeded／readiness／endpoints、恢復 v1 HTTP 全部通過。Docker cleanup 第二次成功。

M1 教學單獨重跑已完整通過 `make m1-up／m1-forward／m1-down`：初版 v1 HTTP／quote=8000、rev 2 v2-good／兩副本 HTTP、副本漂移修復、Service 刪除後 UID 改變、rev 3 壞版 Degraded／ProgressDeadlineExceeded、rev 4 v1／兩副本 HTTP、CR 刪除後 Deployment／Service／Pods GC、Operator 縮放恢復。清理第一次 Docker missing exit event、正式 ownership 重試後刪除成功；驗證工具輸出完整 PASS，隨後才開始 M2。證據為 artifacts/revalidation-20261011/m1-walkthrough-serial.log；首輪 bootstrap FAIL 日誌保留。

M2 教學於 2026-10-12 00:13 完整通過：正式 `make m2-up／m2-forward／m2-down`、文件中的 curl／create-app.json、v1 與 v2-good 實際 HTTP／quote=8000、rev 3 壞版 FAILED／ProgressDeadlineExceeded、回滾為 rev 4 SUCCEEDED、原請求重送維持相同 application／operation／revision、同 key 不同 payload 回 409。以同一新 DB 重啟 Hub 後仍讀回四個 Releases，實際 DELETE Operation 成功且 Application 為 DELETED；down 保留 DB。Docker 第一次清理缺少 exit event，正式 ownership 重試後刪除成功，完整 PASS 後才開始 M3。證據為 artifacts/revalidation-20261011/m2-walkthrough-serial.log，第一次 Hub 啟動的完整輸出另保留於 m2-lesson-hub-initial.log，重啟輸出為 m2-lesson-hub.log。

M2 首輪 `make test-e2e` **exit 1，TestM2HubKind FAIL 178.98 秒**，停止後續 M3 排程。已通過受限 SA、UI 建立／quote、目前 drift 原因與恢復、壞版／回滾、去重／409、RUNNING 時 Hub crash 復原與 quota 拒絕。解除 quota 後立即提交同配置 rev 6，仍在 ReplicaSet 舊 FailedCreate Condition 清除前，被正確觀測為 FAILED；事件顯示隨後真實 Pod 已建立，清理 snapshot 中該失敗 Condition 也已清除。整輪不列為 PASS，owned cluster 已清理。

依這些事件與 [Deployment 的 Pod template 行為](https://kubernetes.io/docs/concepts/workloads/controllers/deployment/#updating-a-deployment)，判斷為 E2E 缺少實際恢復的同步前置，而非 quota 移除會同步重置 rollout 的保證。已補上 bounded wait：quota 真正 NotFound、rev 5 實際 READY／原因清除，同時嚴格確認原 FAILED Operation／Release、command_id／deadline、最後成功 rev 4 與履歷數不變，才送 rev 6；原 rev 6 必須 SUCCEEDED 的 assertion 保留。未修改產品成功／失敗判斷，M2 README 也補上這個操作順序。修改後 `go vet -tags=e2e ./stages/m2/test/e2e` 已 exit 0。批次重跑在 bootstrap 失敗，未啟動 M3；其後單獨 `make test-e2e` 完整通過，結果見下方紀錄。

21:34 開始的 M2 E2E 重跑及 M1 教學都在 kind bootstrap 失敗，未進到功能 assertions；兩個節點的 API server 連線被拒。M2 匯出的 kubelet／containerd 紀錄顯示建立 etcd／API server 容器逾時，之後重試遭遇已保留的 container name；當時尚未定位磁碟 flush 等待，不把該輪記成 Hub／Operator 功能 FAIL 或 PASS。日誌保留於 artifacts/revalidation-20261011/m2-m3-retry.log、m1-walkthrough.log，bootstrap 日誌為 artifacts/bootstrap-hugofleet-m2-e2e/20261011T213419.log 及 artifacts/bootstrap-hugofleet-m1-demo/20261011T213451.log；M2 完整節點診斷在 artifacts/m2-e2e/20261011T213249-884650/kind-logs。兩個本輪節點已依 ownership 清理，其他容器未修改。改為一次建立一個叢集，以正式 `make test-e2e` 單獨重跑。

單獨重跑期間，節點 `/var` 使用 29／1007 GiB、`/run` 僅 8.5 MiB，沒有磁碟空間耗盡的證據；containerd 執行緒的核心 stack 停在 `submit_bio_wait → blkdev_issue_flush → ext4_sync_file → fdatasync`。這能定位到磁碟 flush 等待，尚不能證明 Windows／Docker Desktop 的具體根因。記錄於 artifacts/revalidation-20261011/containerd-serial-kernel-wait.txt。containerd 未啟用 debug socket，內建 pprof 讀取失敗，不把該次診斷列為測試 PASS，亦未為診斷重啟 daemon 或修改節點設定。

M2 單獨重跑已通過 `TestM2HubKind` 全部 assertions（355.00 秒）：實際 Hub RBAC、UI／quote、drift 原因／恢復、故障／回滾、冪等／409、RUNNING 時 Hub crash、quota 拒絕與解除後真實恢復、不可改寫的晚到 Ready／timeout、真實 SQLite 寫入故障／丟失 acknowledgement／SIGKILL 的 UID 復原、刪除 GC 與重啟後完整履歷。新增加的 quota 同步檢查及下一 revision SUCCEEDED 皆通過。證據為 artifacts/m2-e2e/20261011T215044-891551/run.log；清理第一次遇到 Docker missing exit event，ownership 核對後第二次成功，整個 `make test-e2e` exit 0。

pricing-api 元件 README 的 `go run ./stages/m0/pricing-api --listen=127.0.0.1:18082` 已實際啟動，health／readiness／version／標準 quote 通過，101 cents VIP 範例得到 90 cents；結束時只停止本輪程序。教學驗證使用忽略的臨時 Go HTTP assertion 與 Bash 編排，呼叫真實程式與正式 stages／Make 入口。

M3 以正式 `make test-e2e-m3` 重建與啟動，run 為 artifacts/m3-e2e/20261011T221953-919702。Hub 映像為 sha256:13bcbada4b1fc4e7b3580dd44d9313d39bbfacfd87741b1bd67cc5783118405d，Agent 為 sha256:1f8ff476461e63d651bd347e0d4480f4aa6ed4515e474aefb12012eac55559d3。`TestM3DualKindOutboundAgents` 功能 assertions PASS（456.85 秒）；正式腳本已保存診斷、清理本輪 Hub 與兩叢集，整輪 exit 0。Docker 兩叢集第一次刪除均遇到 missing exit event，在正式 ownership 重試後刪除成功；原 lab-control-plane 保留。

已實測兩個 Worker Pod 主動建立 mTLS 通道、Hub 僅掛載自己的憑證／DB、未掛載 Worker kubeconfig、未認證管理 API 拒絕；A／B 皆部署 v1 並實際 quote=8000。A Agent 關閉後，Hub 標記 A DISCONNECTED／應用 UNKNOWN 並保留觀測時間，拒絕新的 A 操作；A Operator 仍修復副本，B 仍成功發版／HTTP。真實受限 SA 的 ledger 重啟／ACK 遺失／舊 revision／同 revision 不同配置／過期 Delete／tombstone／GC assertions 通過。Agent 重連與 Hub 重啟保留 CR UID／generation、command_id／deadline；壞版 FAILED、回滾新 revision SUCCEEDED／v1 quote=8000。最後 cordon 本輪節點讓真實新 Pod Pending，Hub deadline 到期後恢復，晚到 Ready 僅更新觀測，原 TIMED_OUT／FAILED 履歷與最後成功版本保持不變；兩 Worker 刪除 GC assertions 皆通過。

M3 教學於 2026-10-12 完整通過，外層依序執行 M1→M2→M3 的驗證命令 exit 0：新的 state、正式 m3-up、文件中 Token stdin curl 與 deploy-a/b.json、A／B v1 HTTP／quote=8000，以及實際 HTML 表單的建立／更新／回滾／刪除均成功。A 離線時 B 更新 v2-good 仍可 HTTP，A Operator 自行修復漂移，重新啟動 Agent 後 CR UID／generation／revision 保持；A rev 3 壞版 FAILED、UI 回滾為 rev 4 SUCCEEDED 並再次確認 v1 HTTP。Hub stop/up 與容器 recreate、兩個 Agent 重新安裝後，資料庫履歷與真實應用狀態仍一致。兩個 DELETE 成功且相依資源 GC；m3-down 清理 Hub／Workers 並保留 DB／Token。Docker 各叢集第一次缺少 exit event，正式 ownership 核對及重試後清理成功，完整流程才列 PASS。

另從 Windows Codex 瀏覽器實際開啟 M3 登入頁、使用本輪管理 Token 登入，重啟後畫面顯示兩叢集 CONNECTED、A READY revision 4／B READY revision 2，與 Go／Kubernetes assertions 一致。JPEG 證據為 artifacts/revalidation-20261011/m3-login-browser.jpg、m3-dashboard-browser.jpg、m3-dual-ready-browser.jpg；未記錄 Token。驗證用瀏覽器頁面已關閉。

全部教學與報告修正後，最後 `make docs-check` exit 0：32 份 Markdown、443 個本機檔案／章節連結、85 個 Shell 區塊、三份型別化部署 JSON，證據為 artifacts/revalidation-20261011/docs-complete.log。Docker／kind 列表只剩原有 lab-control-plane／lab，非專案 consul／documents-postgres-1 保留；本輪 Hub／Workers 與驗證 port-forward 已停止。

曾嘗試新增 `make test-integration` 時，自動核准審查因工作區 credits 不足而失敗，該命令未執行。使用者要求繼續後，新的 `make proto-check test-integration` 已獲准啟動；未執行的那次不列為測試結果。

## 實測邊界

README 核對涵蓋全部正式文件；操作驗證以各階段的主流程及相關 Go／生成／E2E 入口為準，不表示每個可選診斷命令都另做一次完整建置。未執行 hosted CI、HF_ETCD_STORAGE=disk 的節點重啟、整台主機重啟、備份還原或自動憑證輪替。`make cache-native` 本輪也未執行：舊專案的 /tmp 快取保留，新路徑使用自己的 .cache。M3 開發憑證輪替仍須依文件手動操作，不能由 Hub 重啟結果推論自動輪替。

這輪使用新的驗證 DB／state，沿用同一份 DB 測試 Hub 重啟或容器重建；保留既有學習履歷與憑證。RAM etcd profile 的 kind down／重建不等於恢復原叢集；SQLite 履歷保存也不等於原 Worker CR UID 仍存在。舊相容入口已核對分派，但不以原本的私有 DB 再跑一輪舊 make demo。

## 原始日誌

本輪共用日誌在 `artifacts/revalidation-20261011/`，各 E2E 仍由正式腳本另存於 `artifacts/mN-e2e/<run>/`。所有證據、DB、kubeconfig、憑證及 Token 都被 Git 忽略；公開摘要不包含秘密內容。

## 2026-10-14 M4／M5 完成後的共用程式回歸

原 2026-10-11～12 M0～M3 正式入口與教學結果保留。後續 M4／M5 依同一份共用 module 完成真實程式、瀏覽器示範與驗收，最新 Unit／race／envtest／build／lint、M1／M2／M3／M4／M5 正式門檻及 owned cleanup 全部通過，完整範圍及各輪失敗見 [M5 紀錄](m5-validation.md)。

| 階段 | 最新正式結果與原始日誌 |
|---|---|
| M0 | 保留本頁原始 E2E／教學 PASS；價格程式、YAML 與入口未因 M5 修改，最新全 module Unit／race／build 仍覆蓋 |
| M1 | PASS 109.55 秒＋RBAC 0.65 秒；artifacts/m1-e2e/20261014T080159-44045/run.log |
| M2 | PASS 233.20 秒；artifacts/m2-e2e/20261014T085921-79540/run.log |
| M3 | PASS 375.42 秒；artifacts/m3-e2e/20261014T091210-89377/run.log |
| M4 | PASS 408.42 秒；artifacts/m4-e2e/20261014T094026-110240/run.log |
| M5 | PASS 安全 390.80 秒＋Dashboard／trace／Helm 103.55 秒；artifacts/m5-e2e/20261014T081210-52619/run.log |

重要共用修正包括受保護頁面瀏覽器 Origin policy、Pod template revision／目前 ReplicaSet 失敗與逾時歸屬、Docker IPv4 gateway 精確選取，見 [ADR 0012](adr/0012-current-revision-replicaset-evidence.md)。M1 新 revision 獨立 rollout 與同 revision 冪等各有實測；M2 測試工具依 README 在 rollout 後重建自己的 port-forward，版本與完整價格 assertions 保留。舊 FAIL／取消／未執行項目不追認成 PASS。最新 22 份 README 與文件檢查結果以 M5 紀錄為準；Hosted CI 未執行，未 commit／push。
