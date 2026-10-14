# M5 驗收紀錄

本文件保留首次本機交付當時的範圍與結果。使用者之後已授權修正、完整重驗與提交／推送；最新狀態見 [修正與完整驗證紀錄](release-validation-20261014.md)。下文的未授權、未推送與 CI 未執行均指當時情況。

開始日期：2026-10-13，Asia/Taipei。狀態：**2026-10-14 M4／M5 實作與本機驗收、最新 M1～M4 正式回歸均已通過；Hosted CI 未執行，未 commit／push**。

交付範圍確認：2026-10-14 使用者確認本次只要求本機能正常運作與測試通過。**本輪 M0～M5 本機交付完成**，不等待提交／推送授權，不執行遠端 CI。SPEC 的 T18（GitHub Actions 重新啟動環境並測試）仍為未執行，不列為通過；若未來需要遠端驗證，另行授權。

開始門檻：M0～M3 的重新驗證及 [M4 全新正式入口與清理](m4-validation.md) 已通過。M5 延用安全 Hub、SQLite、Worker 主動 mTLS 與真實 Operator，不複製另一套管理服務。

## 最小垂直切片與檔案計畫

1. 共用 M2 repository 增加 schema 5：真實 Worker 觀測快照與 Operation trace context；M3 Proto／projection 補齊 Conditions、generation 與每命令 trace／request correlation。
2. stages/m5/observability 實作 OpenTelemetry、允許清單 JSON span 輸出及有限 labels 的四項 Prometheus metrics。M4 Hub startup／HTML API 提供共用選項，M5 入口啟用完整 Dashboard；舊階段預設行為保留。
3. Dashboard 提供 Overview、Clusters、租戶／叢集篩選、Applications 詳情／Conditions／歷史／確認操作及有界更新；所有查詢仍以目前 DB session／membership 授權。
4. Helm 部署 CRD、受限 Operator／Agent／RBAC，引用已建立的 TLS Secret 與持久 command ledger；安裝／升級／重新安裝不能重設 ledger、接管外部資源或移除租戶隔離。
5. 正式 M5 入口、CI 六階段矩陣、架構／ADR／故障演練／限制與實際操作影片；保留獨立 state／原始日誌。

## 測試計畫

- SQLite migration 保留既有履歷、trace context 在重啟／冪等重送後不變、觀測快照過期時保留時間；Metrics 僅在終態交易成功後記一次。
- 真實 mTLS 命令在同一條 stream 中具有不同 trace IDs，Hub→Agent parent 正確；日誌與 spans 不含 token、密碼、headers／OAuth query。
- Dashboard 的狀態／generation／Conditions 與真實 Kubernetes 相同，離線不冒充新資料；角色、CSRF、危險確認與舊階段 regression。
- 真實 Helm install／upgrade／uninstall／reinstall，精確 RBAC／封包及 ledger／receipt UID／high-water／tombstone 保留。
- fmt／vet／build、Unit／race／envtest、Proto／CRD 一致性、正式新 state kind E2E、根目錄起所有 README 主流程及真實瀏覽器 Demo。
- Hosted CI 尚未執行；沒有 commit／push 授權，不將本機結果冒充線上 CI。

## 實測結果

### 最終本機門檻

| 範圍 | 最新實際結果 |
|---|---|
| 完整 Unit／race／integration／envtest、build／lint／tagged vet | PASS：deadline-verification.log exit 0；Controller 11.624 秒，含目前 ReplicaSet／逾時歸屬正負向測試 |
| M1 正式生命週期／RBAC | PASS：109.55／0.65 秒；artifacts/m1-e2e/20261014T080159-44045/run.log |
| M2 正式 Hub／SQLite／UI／HTTP／恢復與歷史 | PASS：233.20 秒；artifacts/m2-e2e/20261014T085921-79540/run.log |
| M3 正式雙 Worker／mTLS／ledger／重連／故障／GC | PASS：375.42 秒；artifacts/m3-e2e/20261014T091210-89377/run.log |
| M4 正式 OIDC／租戶／SA／Admission／Calico／UI lifecycle／離線及 Hub 重啟 | PASS：408.42 秒；artifacts/m4-e2e/20261014T094026-110240/run.log |
| M5 全新正式安全＋Dashboard／traces／metrics／Helm lifecycle | PASS：390.80／103.55 秒；artifacts/m5-e2e/20261014T081210-52619/run.log |
| 所有本輪 owned cleanup、其他容器 ID／狀態比對 | PASS：各正式入口及 final-m1-deadline-regression.log／deadline-fresh-gate.log／final-m2-m4-forward-regressions.log exit 0；DB／憑證與歷史保留 |
| 真實瀏覽器 | PASS：M5 部署／壞版／確認回滾／角色／跨租戶；另在最新 M4 正式入口驗證 Dex 登入與 HTML 登出 |
| Hosted CI | 未執行：沒有 commit／push 授權，本機結果不等於 CI |

M0 價格程式／YAML／入口本輪 M5 未改；保留先前正式 M0 E2E 與 README 教學結果，最新全 module Unit／race／build 仍包含 M0。完整初始 M0～M3 核對見 [重新驗證](revalidation-20261011.md)。後續只改 M2 的 E2E forward lifecycle 與文件，產品 Go 程式在上述最終 race 後未再修改。

最終交付核對：gofmt -l stages tools 沒有輸出；make docs-check **PASS，exit 0**，44 Markdown、603 local file／section links、112 shell examples、三份 typed deployment examples，涵蓋全部 22 份正式 README。日誌 artifacts/m5-development/final-document-delivery-check.log。git public inventory 為 235 份 source／docs／media，runtime／DB／kubeconfig／accounts／subject／key／cert state、.tools／bin／artifacts 排除；逐個 public file 掃描沒有 private-key PEM marker，不將這項掃描誤稱為完整安全稽核。公開 MP4 SHA-256=bd09ee2b304a633e9f7d1dd533d3d85adfca4a6e81cee07a79349ca0124cc9df，與已實際播放及解碼檢查的素材相符。

目前公開來源與 [.github CI workflow](../.github/workflows/ci.yml) 已可檢視；git status 仍全部未提交，遠端 refs 唯讀可連線但沒有 heads。尚未 commit／push，T18 Hosted CI 仍未驗收，不能把本機完工寫成線上 CI 通過。其餘本機交付與已授權修正完成，沒有提前加入 M5 後續骨架。

補入上述 workflow 連結後，最終 journal 再核對 make docs-check **PASS，exit 0**：44 Markdown、604 local file／section links、112 shell examples、三份 typed deployment examples，見 artifacts/m5-development/final-document-journal-check.log。這次只更新紀錄文字與連結，沒有改動 Go／YAML／Bash 產品程式。

各正式門檻結束時外層其他容器 ID／狀態比對均通過。後續最後一次唯讀 docker ps 盤點中，三個非 HugoFleet 容器顯示 exited；Docker backend 仍為 29.8.2，Agent 未對這三個容器執行啟動／停止／重啟。期間比對結果只代表各門檻結束時點，不延伸宣稱其他程序之後持續運行；原因未在本專案範圍內追查。

### 全新正式入口：失敗與修正

2026-10-13 21:55 恢復後執行 make test-e2e-m5，使用全新 state .local/m5-e2e-1791899803-43083，完整日誌 artifacts/m5-e2e/20261013T215643-43083/run.log、外層 fresh-final-gate.log。兩個新 Workers／Calico／CoreDNS、六個真實 Dex 身分、Helm install、Hub／Agents 皆成功。TestM5RealSecurity 在 memory quota 恢復步驟 FAIL（155.70 秒）；TestM5RealDashboardTracesAndHelm 仍完整 PASS（136.49 秒），包含真實 CR 對照、兩操作同 channel 的不同 traces、metrics delta=2、Helm upgrade／uninstall／reinstall 與 ledger／receipt 保留。正式 trap 成功清理這輪 owned Hub／Dex／Workers並保留私有 state，整次 make exit 1，**不得列為整體通過**。

失敗不是略過等待或配額加大可以掩蓋的問題：測試已等待真實 Quota 釋放，Deployment observedGeneration 也已推進，但仍帶著舊 ReplicaSet 的 FailedCreate（910Mi + 128Mi 超過 1Gi）；Operator 錯把舊錯誤套用到新 revision。現場 Pod／ReplicaSet／Quota 保存於 fresh-memory-quota-failure.yaml。2026-10-14 新增回歸 Unit，stale-replicaset-before.log 重現五項 FAIL；修正後 Operator／Hub kube／bootstrap 套件 PASS，stale-replicaset-after.log。

修正將 revision 綁定到 Pod template，Operator 只以目前 owned ReplicaSet 的 template／observedGeneration 歸屬 ReplicaFailure；namespaced Role 僅增加 ReplicaSet get/list/watch，新增真實 SA 允許讀取、拒絕寫入／跨租戶 assertions。決策與官方原始碼依據見 [ADR 0012](adr/0012-current-revision-replicaset-evidence.md)。原 memory recovery 必須 SUCCEEDED 的 assertion 保留，最新完整 Unit／race／envtest 已通過，尚待全新 M5 E2E 與 M1～M4 回歸。

2026-10-14 修正後的 go test -count=1 ./...、make manifests verify-generated proto-check lint build m5-chart-test docs-check，以及 go vet -tags=e2e,helmintegration ./... 均 exit 0；文件檢查為 44 Markdown、583 local links、112 shell examples、三份 typed deployment examples，見 artifacts/m5-development/replicaset-verification.log。後續 make test-race 因 Ubuntu WSL 缺少 Docker CLI／socket，無法啟動固定 Go container，整個驗證腳本 exit 1；**此次 race／envtest 未執行，不列通過**。當下 Windows Docker daemon 可連線，但 WSL integration 不可用。使用者回覆後重新檢查，Ubuntu 的 /usr/bin/docker、socket 與 Docker Desktop 4.95.0／Engine 29.8.2 已恢復連線；Agent 未重啟 Docker／WSL 或變更其他容器，接續 race，日誌另存 replicaset-race-resumed.log；全新 M5 與 M1～M4 真實回歸仍待完成。

以下保存各輪成功與失敗的分項；最新全新正式結果見本節後段，前階段回歸仍逐項補齊。舊 FAIL 不因後續修正改寫為 PASS。

環境恢復後 make test-race 全 module Unit／integration／真實 envtest PASS，exit 0，見 artifacts/m5-development/replicaset-race-resumed.log。此輪包含最新 ReplicaSet 歸屬與 Pod template revision 修正；Controller 13.551 秒、repository 94.866 秒、Service 48.509 秒、M4 auth 68.387 秒。全新 M5 正式入口另外執行，不能以 race 結果取代 kind 驗收。

接續全新正式 Run artifacts/m5-e2e/20261014T064301-1268，state .local/m5-e2e-1791931381-1268，兩個 Worker／A Calico／雙 CoreDNS 與六個 Dex Code+PKCE 身分皆完成，但 Hub 的 /healthz 持續 403，啟動 FAIL，尚未執行任何 M5 功能 assertions。正式 trap 已成功清理 owned Hub／Dex／Workers，保留 DB／憑證，make exit 1。Hub 日誌顯示來源限制拒絕；新 Docker kind network 的 IPAM 第一筆為 fc00:f853:ccd:e793::1，第二筆才是實際 IPv4 gateway 172.19.0.1。M3～M5 腳本原本依索引 0 取值，錯將 IPv6 gateway 寫入 HTTPClientIP。

修正為共用 kind_ipv4_gateway，明確選唯一 IPv4；沒有 IPv4、歧義或無效地址均拒絕，Host／loopback port／精確來源 IP 限制保留。七項明確 shell selection fixtures PASS，真實 network 舊 first entry 與新 IPv4 選取核對亦 PASS，見 artifacts/m5-development/ipv4-gateway-check.log；這些不是 cluster E2E。全新 M5 正式入口仍另重跑，外層日誌 ipv4-fresh-gate.log。

新 Run artifacts/m5-e2e/20261014T071412-17856、state .local/m5-e2e-1791933252-17856 確認 gateway 修正有效：Hub /healthz 真實 200、HTTPClientIP=172.19.0.1，兩個 Helm installs／Operators／Agents rollout 與六個 Dex 登入通過。租戶／SA 權限、實際 Calico 封包及 CPU／memory Admission、解除配額後新 revision recovery／GC 皆 PASS，原先記憶體配額問題已得到真實證據。TestM5RealSecurity 後續在 lesson 回滾 FAIL（332.26 秒）：新 Operation 僅 129 ms 就被舊 revision 3 ReplicaSet 的 ProgressDeadlineExceeded 終結。現場 resources 另存 artifacts/m5-development/fresh-rollback-stale-deadline.yaml；稍後真實回滾工作負載已可用，也不能改寫原 FAILED 歷史。

同輪 TestM5RealDashboardTracesAndHelm PASS（120.80 秒），正式 trap 成功清理所有 owned 容器／Workers，保留私有 state，make exit 1；整輪仍為 FAIL。依固定 Kubernetes 原始碼核對 condition 保留行為後，補上逾時版本歸屬：必須有目前 owned／observed ReplicaSet，condition LastUpdateTime 不得早於其 CreationTimestamp + progressDeadlineSeconds。沒有目前證據時繼續等待，Hub deadline 保持不變，不偽造 Ready 或延後終態。五項負向 Unit 在 stale-deadline-before.log 重現 FAIL，真正目前逾時的正向案例仍 PASS；修正後完整 Controller Unit 0.104 秒、exit 0，見 stale-deadline-after.log。原壞版 ProgressDeadlineExceeded 與真實回滾 SUCCEEDED assertions 保留；最新完整 race／envtest、正式 M5 與 M1～M4 回歸需再驗。

逾時歸屬修正後 make lint build docs-check、go vet -tags=e2e,helmintegration ./...、make test-race 全部 exit 0，完整 Unit／integration／真實 envtest PASS，見 artifacts/m5-development/deadline-verification.log；Controller 11.624 秒、repository 190.854 秒、M4 auth 178.364 秒。M1 E2E 依新 revision template 契約增加新的 owned／完整 Ready ReplicaSet 檢查，原 ReplicaSet 不變 assertion 保留在相同 revision 重送並檢查 generation 不變；M1／M2 E2E tagged vet 另 exit 0。先從正式 M1 生命週期驗證，再重跑全新 M5 及 M2～M4；未將編譯當成 E2E 通過。

最新 make operator-image-build test-e2e-m1 **PASS，exit 0**：TestM1KindLifecycle 109.55 秒、TestM1Authorization 0.65 秒。相同配置／純縮放的新 revision 建立新的 owned／完整 Ready ReplicaSet；同 revision 重送不變更 ReplicaSet／Deployment generation；真實 v1／good／broken HTTP、Deployment／Service 漂移及刪除重建、Operator 重啟、真正 ProgressDeadlineExceeded、新 revision 恢復及完整 GC 均通過。ReplicaSet get/list/watch 允許、create/update／跨 namespace 拒絕的真實 SubjectAccessReview 通過。正式入口清理 owned cluster，外層比對其他容器名稱／ID／狀態不變；日誌 artifacts/m1-e2e/20261014T080159-44045/run.log、artifacts/m5-development/final-m1-deadline-regression.log。Operator image sha256:e0059172592d3e685223bfe989c4e7ec31ed385d164de8fa744f1d24bed9f036。全新 M5 與 M2～M4 仍待完成。

根 README 的遠端連線於 2026-10-14 另行唯讀核對：git -c credential.interactive=false ls-remote --heads origin exit 0、未返回任何 branch refs。公開網頁讀取為工具 cache miss，不以它判斷 repository 是否存在；Git refs 連線成功也不等於 Hosted CI。未 commit／push，CI 設定及 hosted 結果仍分開標示。

### 最新全新正式入口通過

make test-e2e-m5 **PASS，exit 0**，完整日誌 artifacts/m5-e2e/20261014T081210-52619/run.log，外層 artifacts/m5-development/deadline-fresh-gate.log；全新 state .local/m5-e2e-1791936730-52619。入口自行建置最新映像、從零準備雙 Worker／A Calico／雙 CoreDNS、六個真實 Dex Code+PKCE 身分、兩次 Helm install、Operators／Agents rollout 與 mTLS 連線。

- TestM5RealSecurity **PASS 390.80 秒**：原 CSRF／Origin、租戶／角色／撤權／可信 actor、真正 SA 權限及實際 Calico packets；CPU／memory Pod Admission 拒絕、完整原因傳回、配額解除後新 revision recovery／GC；實際 HTML forms 的 v1／good／replicas drift／broken／FAILED 履歷、確認回滾新 revision SUCCEEDED／HTTP、確認刪除／GC；B 斷線／重連及服務持續、Hub 原 DB／session 重啟、無 Hub 時自主漂移修復全部通過。原記憶體恢復與回滾 assertions 保留。
- TestM5RealDashboardTracesAndHelm **PASS 103.55 秒**：Dashboard 真實 CR UID／revision／generation／完整 Conditions 對照與 scope／Admin telemetry；兩個真實 OIDC／SQLite／mTLS operations 在同一 channel 有不同 trace IDs、Hub→Agent parents 相符、committed metrics delta=2；真正 Helm upgrade／uninstall／reinstall 後原 ledger UID／完整 high-water／tombstones、receipt UID／data、Admission 材料保留，Agent 實際重連。
- 正式 trap **PASS owned cleanup**：Hub／Dex／兩個 Workers 移除，私有 DB／帳號／subject／certs 保留。外層其他容器名稱／ID／狀態比對不變，exit 0。最新 Operator image 與上述 M1 回歸相同，Agent image sha256:7163cdd690287022d9aeb03fd2fcd827d4a46ff19b0ecf5f50450b2895dc9188。

目前 M1 與 M5 真實正式門檻已通過，M2～M4 另以最新程式從正式入口回歸。完整本機交付狀態仍須等這些結果與文件收尾；Hosted CI 未執行。

### 前階段回歸：M2 轉送測試工具修正

make test-e2e 首輪 Run artifacts/m2-e2e/20261014T084505-69644 **FAIL 284.66 秒、exit 1**。真實受限 RBAC、UI v1／quote、CR drift、broken FAILED／上一版仍可用、rollback revision 3 SUCCEEDED 已完成，但 HTTP 驗證沿用最初綁住舊 Pod 的 kubectl port-forward；新 revision 獨立 rollout 移除舊 v1 Pod 後，forward.log 明確回報 lost connection to pod，連線拒絕，等待逾時。resources.yaml 中當前 CR observedRevision=3／RolloutComplete，不能把轉送程序故障誤稱為產品回滾失敗；整輪 E2E 仍列 FAIL，owned cluster 已清理。

只修正 Go E2E 的程序 lifecycle：回滾 SUCCEEDED 後停止／reap 自己建立的舊 forward，重新建立 Service 轉送；版本與 Original／Tier／Coupon／Final=20000／2000／10000／8000 assertions 保留，沒有更改產品程式。M2 README 原已要求 rollout／舊 Pod 退出後重建 forward；M3 helper 每次 HTTP 驗證本來就建立／關閉自己的 forward。新 M2 tagged vet exit 0，正式 M2～M4 重跑日誌 artifacts/m5-development/final-m2-m4-forward-regressions.log；先前失敗的外層日誌保留。

M2 正式重跑 **PASS，exit 0**，TestM2HubKind 233.20 秒，artifacts/m2-e2e/20261014T085921-79540/run.log。原本全部 assertions 通過：真實 UI／HTTP／價格、Spec drift／目前原因、壞版／新 revision 回滾、重送／併發衝突、RUNNING crash recovery、Quota／恢復／FAILED 歷史、TIMED_OUT+LATE_READY、lost UID acknowledgement／SIGKILL／原期限後唯讀精確 UID／generation 恢復、delete／GC／持久歷史。正式 cleanup 及外層其他容器 ID／狀態比對通過。最新 M1／M2／M5 正式門檻已通過，M3／M4 接續驗證。

M3 正式回歸 **PASS，exit 0**，TestM3DualKindOutboundAgents 375.42 秒，artifacts/m3-e2e/20261014T091210-89377/run.log。真實 Worker Pod 主動 mTLS 雙通道、Hub 無 Worker kubeconfig、雙叢集 HTTP／quote、離線觀測保留／自主修復、另一叢集發版、ACK loss／restart ledger／stale-conflicting revisions／expired delete／GC tombstone replay、重連不重複 Apply、Hub restart／broken／新 revision rollback、Pending／TIMED_OUT／LATE_READY 保留終態與最後 GC 通過。正式 owned cleanup 與其他容器 ID／狀態比對通過。M3→M4 銜接時 make 出現一行 getcwd 訊息；後續唯讀 cd／pwd -P／Makefile／規格檔確認現行 root 正常，M4 正式腳本繼續建置，不將這行當作 M4 成功證據。最新 M1／M2／M3／M5 已通過，只剩最新 M4 與文件收尾。

最新 M4 回歸運作期間，另外以真實瀏覽器驗證同一共用 HTTP middleware：正式 18094/login → 本機 Dex Demo admin 登入 → M4 受保護頁與雙 Workers CONNECTED → 真正 HTML logout → OIDC 登入頁，**PASS**。沒有手工補 Origin header，沒有修改 tenant／membership／quota；只登出該瀏覽器 session，臨時 tab 已關閉。證據 m4-browser-authenticated.png／m4-browser-logout.png 位於 artifacts/m5-development，詳見 [M4 紀錄](m4-validation.md)。

M4 正式回歸 **PASS，exit 0**，TestRealTenantSecurity 408.42 秒，artifacts/m4-e2e/20261014T094026-110240/run.log，state .local/m4-e2e-1791942026-110240。真實六身分 OIDC／雙 Worker mTLS、CSRF／Origin、租戶與撤權／可信 actor、SA 權限、Calico packets、CPU／memory Admission 拒絕及新 revision recovery／GC、HTML good／broken／確認 rollback／delete／HTTP、B offline／reconnect／服務持續、Hub 原 DB／session 重啟與無 Hub 自主修復全部通過。正式 cleanup 移除 owned Hub／Dex／雙 Workers、保留私有 state，其他容器 ID／狀態比對不變。連續 M2／M3／M4 wrapper 最後 exit 0；最新 M1～M5 正式本機門檻全部完成。

### 開發與早期分項紀錄

下表保留各次實測時點；當時待重驗的項目已由後續紀錄補齊，最新狀態以上方整輪結果為準。

| 命令／範圍 | 結果與證據 |
|---|---|
| 共用 SQLite／Service／Agent／transport 與新 telemetry 核心 Unit | PASS：`artifacts/m5-development/core-first-unit.log` |
| SQLite trace／觀測快照重啟與冪等保留、終態 hook 交易失敗／重複拒絕；Proto Conditions round-trip；安全 JSON exporter／bounded metrics | PASS：`trace-snapshot-unit.log`。SDK parent 單元測試不等於真實 mTLS E2E |
| M4 API 回歸、M5 Dashboard native build | PASS：`dashboard-first-unit.log`；`go build -o bin/m5-hub ./stages/m5/cmd/hub` exit 0 |
| Operator／Agent telemetry 與舊預設核心回歸 | PASS：`operator-agent-telemetry-unit.log` |
| 真實 SQLite／HTTP／templates 的 Dashboard scope、撤權、Admin metrics／traces、pending 狀態頁及最多 30 次刷新 | PASS：`dashboard-scope-unit.log`；OIDC trusted seam 與 Worker 為明確 Unit fixture，不宣稱 Provider／kind 驗收 |
| 固定 Helm 4.3.0 安裝 | PASS：`helm-tools.log`，官方 archive SHA-256 校驗與 binary version；僅安裝至專案 .tools |
| 實際 Helm CLI render／負向 scope／values | PASS：`chart-integration.log`，8.095 秒，17 個 typed objects、CRD 與 M1 一致、restricted Pods／namespaced RBAC、沒有 Secret／ledger／安全材料模板；9 項不可信 release／namespace／cluster／endpoint／values 拒絕。不是 cluster install 證據 |
| M5 正式 native Make build | PASS：`native-build.log`，`make m5-build` exit 0 |
| 同一通道 correlation 的核心回歸及 M4 安全驗收共用化 | PASS：`channel-correlation-unit.log`、`shared-e2e-compile.log`；M4 原 assertions 保留，M5 明確核對 Helm managed-by／release annotations，沒有以編譯替代 E2E |
| 真實 kind 啟動前的全 module 檢查 | PASS：`pre-kind-full-unit.log`、`pre-kind-vet.log`、`pre-kind-generated.log`；`go test -count=1 ./...`、`go vet -tags=e2e,helmintegration ./...`、`make proto-check verify-generated` exit 0。新增離線 viewer／M5 E2E 原始碼之後需另驗 |
| M5 identity 邊界回歸與更新 Hub image | PASS：`explicit-identity-unit.log`、`identity-hub-build.log`；精確 M5 URI SAN accepted，foreign／query／fragment／path variants rejected，Hub Docker build exit 0 |
| 固定零值 metrics／SQLite／HTTP regression | PASS：`metrics-zero-after.log`、`metrics-hub-build.log`；保留 `metrics-zero-before.log` FAIL。初始化固定 series 不增加計數或 histogram samples |
| 真實 M5 Dashboard／traces／Helm lifecycle | PASS：`metrics-recreate-and-delivery.log`，88.80 秒。Dashboard UID／revision／generation／Conditions 對照實際 CR；兩個真實 OIDC／SQLite／mTLS operations 在同一 channel 有不同 trace IDs、正確 Hub→Agent parent、committed metrics delta=2；Helm upgrade／uninstall／reinstall 後 ledger UID／完整 high-water／tombstones、receipt UID／data 保留，Agent 真正重連 |
| M5 README／links／shell 靜態檢查 | PASS：`readme-docs-check.log`，43 Markdown、551 local links、110 shell blocks、3 typed deployment examples；後續文件新增需再核對 |
| 已收集 viewer Unit 與完整 E2E binary | PASS：`collected-viewer-unit.log`、`m5-e2e-compile.log`；尚未以編譯／fixture 代替實際 viewer 資料收集 |

M5 development state 為 `.local/tutorials/m5/development-20261013`；兩個 Workers、Dex、Hub 與 Helm 已實際運作並完成下列分項。2026-10-13 21:55 恢復工作後，已由 make m5-down 清理本輪容器／Workers、停止精確核對的 port-forward，保留 DB／帳號／subject／憑證。後續全新正式結果見上方紀錄；最初正式啟動的失敗日誌 `artifacts/m5-development/development-up.log` 保留，不追認為 PASS。

最終全階段 make build lint docs-check proto-check verify-generated m5-chart-test 及 go vet -tags=e2e,helmintegration ./... exit 0，見 final-build-static-chart.log；Helm CLI 4.363 秒。此輪文件檢查為 43 Markdown、573 links、111 Shell blocks、三份 typed deployment examples；後續只變更驗收結果文字時仍另驗文件。

### 瀏覽器、影片與最後修正

- 真實瀏覽器以 Dex 登入，Owner 建立 video-pricing：v1 → v2-good／replicas 2；主機將 Deployment 縮成 1，Operator 自主恢復 2。B Agent 停止後 UI 顯示 DISCONNECTED 並保留最後心跳，B 既有服務仍 Ready；恢復 Agent 後重連。v2-broken 的 revision 3 真正 FAILED／ProgressDeadlineExceeded，確認回滾來源 1 後建立 revision 4 SUCCEEDED，失敗履歷保留。新 port-forward 的實際 Service 回傳 v1／quote=8000。
- Viewer 真實 OIDC 登入後沒有修改／回滾控制；已知 Team B URL 與全域 metrics 均 403。1280 寬桌面與 372 寬頁面檢查通過，窄版表格在自己的容器橫向捲動，沒有根頁面溢出。
- 共用 Referrer-Policy 修正後，瀏覽器實際登出／建立／發版／確認回滾均通過。原缺少、null、外站 Origin 與 CSRF 拒絕仍保留。M5 接受操作的提示另修正為只在 Active 時顯示等待 Worker；終態不再誤稱 pending。terminal-notice-before.log 重現 FAIL，terminal-notice-after.log PASS，真實瀏覽器也確認 FAILED／SUCCEEDED 頁面提示正確。
- 修正後 make m5-build m5-test lint docs-check proto-check verify-generated exit 0，見 post-browser-static-native.log。全 module make test-race exit 0，涵蓋 Unit／integration／真實 envtest，見 post-browser-race-envtest.log；包含上述 Origin、終態提示及 CLI buffered writer 修正。
- 實際收集 Hub 2,088、Agent 5,552、Operator 356 筆 spans，核對瀏覽器回滾 trace c71b29b8cfdbf0c857a958b3e3d158c5 的 HTTP→operation→command→Agent→Operator parent 與 correlation，見 actual-collected-parent-check-and-export.log。未緩衝 CLI 在 NTFS 上輸出極慢，第一次取消，trace-view.html 不列成功；改用 64 KiB writer 後實際產生 4,778,180 bytes 的 trace-view-buffered.html，0.977 秒、exit 0。CLI Unit 與最終 race 均通過。
- [示範影片](media/m5-demo.mp4) 為 19 張真實瀏覽器擷取依時間順序剪輯，H.264、1080×800、25 fps、114 秒，**並非連續錄影**，省略等待時間。[字幕](media/m5-demo.vtt) 與 [擷取時間／SHA-256](media/m5-demo-captures.json) 同附。[播放頁](media/m5-demo.html) 已在瀏覽器實際播放：currentTime 從 0.024 增至 27.471 秒、readyState=4、無 media error；截圖見 demo-player-browser.png。FFmpeg 最終 encode exit 0，字幕與畫面抽幀檢查通過。
- [公開回滾 trace](media/m5-rollback-trace.html) 已由瀏覽器核對實際 root／parent／IDs，證據 public-trace-browser.png。Hub 3,000 筆 ring 稍後淘汰該舊 trace，UI 篩選無結果屬容量限制；先前收集的完整 JSON 與離線 viewer 保留，不冒充即時全量歷史。

上述未另註目錄的原始證據均位於 artifacts/m5-development。公開素材只保留合成 Demo 資料與允許的 correlation 欄位；私有 state／原始日誌未加入公開文件。

全 module `make test-race` 的 Unit／integration／真實 envtest 套件結果全部 PASS，見 `final-race-envtest.log`；之後的瀏覽器修正已由 post-browser-race-envtest.log 另行重驗，exit 0。

真實瀏覽器發現 M4／M5 的 HTML 登出 POST 被 Origin 檢查拒絕。共用回應 `Referrer-Policy: no-referrer` 依 [Fetch Origin header 算法](https://fetch.spec.whatwg.org/#origin-header) 將 navigation POST 的 Origin 序列化為 null；HTTP 測試 driver 手動帶 Origin，先前未涵蓋此瀏覽器行為。回歸 `browser-form-policy-before.log` FAIL；protected 頁面改用 same-origin，OAuth 公開回應保留 no-referrer，仍拒絕缺少／null／外站 Origin，CSRF assertion 不變。`browser-form-policy-after.log` HTTP 回歸 PASS；真實瀏覽器重驗已通過（見上節）；全新正式入口結果另列。

第一次 `make m5-up` 為 FAIL：映像建置成功後，Calico 共用腳本的 state allowlist 仍只有 M4，因而在建立 Worker 前拒絕 M5 路徑。保留原日誌；修正為精確加入專案 M5 runtime prefix，仍核對 approved A identity、Container ID 與 CNI version。開發 state 從既有正式 kind／control entrypoints 接續，日誌另存 development-resume.log；後續全新正式入口仍需完整重驗。

接續啟動的兩個 Workers／CNI／CoreDNS、真正 Helm install／四個 Operators／兩個 Agents rollout 及 receipt 已完成，但 /readyz 仍為 503，整次 resume FAIL。實際 Agent 日誌顯示 mTLS SAN parser 的既有 allowlist 未加入 m5-e2e identities，回覆 unregistered cluster identity。只加入兩個精確 M5 IDs，保留 URI／query／fragment／Host／未登記 identity 拒絕；原 state、DB、certs、Helm resources 與失敗日誌保留，重驗結果另列。

更新 Hub 後，真實 security／Calico packets／CPU-memory Admission／UI rollback-GC／Agent offline-reconnect／Hub restart 與自主修復全部完成，Dashboard 的 UID／revision／generation／Conditions 與實際 CR 比對亦 PASS；整次 E2E 最後因 Hub restart 後沒有初始化零值 counter 而 FAIL（identity-recreate-and-e2e.log，340.93 秒）。原 assertion 保留，另以 metrics-zero-before.log 重現 Unit FAIL，再初始化固定 4×3 終態 series；不增加 counter、不偽造 histogram sample。後續 telemetry／Helm scope 與全新完整入口另驗。
