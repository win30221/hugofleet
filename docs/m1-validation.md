# M1 實測紀錄

日期：2026-10-11，Asia/Taipei。

**M1 本機 RAM profile 驗收通過。** 本階段停止，不提前開發 M2。

最小垂直切片：Application CRD → namespace-scoped Go Operator → Deployment／Service → 真實 pricing-api HTTP → 更新／漂移修復 → ownerReference／GC。檔案包含 stages/m1/api/v1alpha1、stages/m1/controller、stages/m1/cmd/operator、config、build、Go envtest／kind 測試及既有腳本調整。Hub／Agent／UI 未加入。

版本：Go 1.27.2、Kubebuilder 4.16.0、controller-gen 0.22.0、controller-runtime 0.25.1、k8s.io/*／envtest 1.37.0；kind 0.33.0 與 M0 相同 node digest。

本機 M1 沿用 RAM etcd 拋棄式 profile，只驗證 Operator 程序重啟；Docker node 持久性、disk profile 完整 E2E 與 GitHub CI 未驗證。取捨見 [ADR 0005](adr/0005-m1-operator.md)。

測試計畫：

- `make operator-tools generate manifests verify-generated`。
- `make fmt lint build test-unit test-race`。
- `make envtest test-integration`：真實 API schema／CEL／default／status、Controller 收斂／ownership／metadata／ClusterIP／no-op。
- `make demo-image-build operator-image-build test-e2e`：真實 Pod／HTTP、revision／縮放／image／漂移／重建／Operator 重啟／故障／恢復／GC／限定叢集清理。

以下均為本機實際執行結果；GitHub hosted CI 未執行。

目前已確認：

- 固定 controller-gen／setup-envtest 與 Kubebuilder binary 已安裝；Kubebuilder SHA-256 相符。
- `make envtest` exit 0；執行檔實際回報 kube-apiserver v1.37.0、etcd 3.7.0。
- Kubebuilder `init --namespaced --fetch-deps=false` 與 `create api ... --make=false` exit 0，在 `.local/scaffold-m1` 隔離產生；僅保留此 Application API 所需 PROJECT metadata，沒有覆寫 M0。
- `make manifests fmt lint build test-unit verify-generated` exit 0；五個有測試的 packages 通過，stages/m1/cmd/operator 編譯通過（沒有 unit test）。完整 stdout 保存 `artifacts/m1-checks/unit.log`。
- `make cache-native` exit 0；原生快取的 Go SDK 實際回報 go1.27.2，來源為同一個官方 SHA-256 校驗的 tar.gz。
- `go mod tidy` exit 0；`make test-integration` exit 0，TestEnvtest 80.57 秒。defaults／status subresource、15 組無效 spec、create／update／repair／ClusterIP／metadata／no-op、未擁有 Deployment／Service／舊 UID 衝突均通過，保存 `artifacts/m1-checks/envtest.log`。沒有測 Pod Ready 或 GC。
- `make fmt lint test-race` exit 0；五個有測試的 packages 通過，Controller 的真實 envtest + Race 12.922 秒，沒有偵測到 data race。WSL 缺 C compiler，使用固定 Go container、4 CPUs／4 GiB、network=none 與唯讀來源。完整 stdout 保存 `artifacts/m1-checks/race.log`。
- `make demo-image-build operator-image-build` exit 0；三個 pricing-api 與 hugofleet-operator:m1 映像建置成功，使用固定 Go base digest、multi-stage／scratch 與 non-root。完整 stdout 保存 `artifacts/m1-checks/images.log`。
- `make test-e2e` exit 0；獨立 hugofleet-m1-e2e 從零建立，Node／CoreDNS／Operator Ready，Application 真正建立 Deployment／Service。TestM1KindLifecycle 254.91 秒，全部通過，含真實 HTTP v1／v2-good／v2-broken、價格 8000、相同配置新 revision、純縮放、image 更新、Deployment／Service 漂移、刪除後重建、Operator 程序重啟、故障版的 ProgressDeadlineExceeded、健康版 Ready endpoints、以新 revision 恢復 v1，以及 Application／Deployment／Service／ReplicaSet／Pod 的實際 GC。診斷及含清理的最終 PASS 保存在 `artifacts/m1-e2e/20261011T075152-333405/`。
- 在該次叢集已安裝 Role、尚未清理時，`go test -tags=e2e -run '^TestM1Authorization$' -v -count=1 -timeout=2m ./stages/m1/test/e2e` exit 0（0.065 秒）。使用真正 SubjectAccessReview，hf-demo 的 Deployment update／Service create 允許，Secrets get、pods/exec get／create、namespaces get、default 的 Deployment update 全部拒絕；保存 `artifacts/m1-checks/rbac-go.log`。這項 Go 測試是在本次 E2E binary 編譯後加入，因此本次分開執行；後續 `make test-e2e` 會自動編譯、執行兩項 Go 測試。原 Bash 的授權檢查由這組更完整的 Go assertions 取代。
- 清理前最後一次 Docker stats：node 901.6 MiB／15.48 GiB、CPU 35.14%；這只是一個時間點，不是峰值或平台效能基準。
- 最後 `make lint build verify-generated`、`go vet -tags=integration,e2e ./...`、`CGO_ENABLED=0 go test -c -tags=e2e -o bin/m1-e2e ./stages/m1/test/e2e` 全部 exit 0。`bin/m1-e2e -test.list TestM1` 確認兩項 Go E2E 已註冊；Docker 容器清單確認測試節點不存在，原有 lab／consul／postgres 容器仍在，未重啟。保存 `artifacts/m1-checks/final.log`。
- 原始碼掃描未找到私鑰、GitHub token、AWS access key 或 GitHub 帳號佔位文字；工具／快取／kubeconfig／binary／診斷均由 .gitignore 排除。沒有 commit／push 或建立 GitHub Repository。

遇到的問題與修正：

1. 相依下載完成前呼叫產生器，缺 go.sum 且產生不完整 DeepCopy；移除不完整產物，完成固定依賴及 checksum 後重新產生。
2. Go module 下載遇 TLS handshake timeout；保留快取重試，未改用非官方 mirror 或放棄校驗。Kubebuilder binary 慢速下載先達 300 秒上限，改為有界續傳並核對 SHA-256。
3. 在 Bash 腳本執行中修改同一檔案，造成讀取偏移的 EOF／未知命令錯誤；整次命令判失敗。停止修改執行中的腳本後從穩定檔案重跑，不能將該次當成 lint PASS。
4. RBAC markers 直接貼在函式 doc comment，controller-gen 不將其視為 package markers；改為獨立 package marker 區塊，成功產生 hf-demo Role，未改成 ClusterRole。
5. NTFS／WSL 路徑的測試連結耗時數分鐘，當時 link CPU 約 2%、I/O full 約 20%；採用 ADR 0006 的專案專用 Linux 快取，保持同一份原始碼與固定版本。沒有搬移 Repo 或修改全域環境。
6. 首次 envtest 提示未配置 Controller logger；已設定 JSON stdout logger 供失敗診斷，完整 Race 已使用此設定重跑整合 assertions。
7. Race 容器的 network=none 沒有 default route，API server 自動選址失敗；明確 advertise loopback 後，1.37 預設 lease endpoint reconciler 又拒絕 loopback。根據官方參數與原始碼，envtest 使用 advertise-address=127.0.0.1、endpoint-reconciler-type=none；這個環境沒有 Service dataplane，schema／Controller assertions 不變。前兩次完整 Race 均判失敗，保留 race-first-failure.log／race-second-failure.log／race-loopback-failure.log；修正後完整 Race exit 0。kind 部署沒有改此設定。
8. kind E2E 第一次清理遇 Docker Desktop 未收到 exit event；腳本等待並核對原 Docker ID，再次刪除成功，最後 exit 0。沒有重啟 Docker／WSL 或清理其他專案容器。

M1 的六項驗收條件全部通過。尚未驗證：Docker node 重啟後資料持久性、disk profile 完整 E2E、GitHub hosted CI、HA、多租戶、多叢集。M2–M5 未開始。

## 2026-10-11 M2 審查後回歸

上方範圍記錄原始 M1 驗收時點；目前停止於 M2，見 [M2 紀錄](m2-validation.md)。補齊三階段 CI matrix 後，`make test-e2e-m1` 再次從零建立真實 kind，exit 0，Run `20261011T115041-467968`。

`TestM1KindLifecycle` 321.36 秒、`TestM1Authorization` 0.06 秒，所有生命週期／HTTP／漂移修復／重建／Operator 程序重啟／壞版失敗及恢復／GC／受限 RBAC Assertion 通過。Docker 初次移除未收到 exit event，原有 ownership 核對與第二次刪除成功。紀錄 `artifacts/m1-e2e/20261011T115041-467968/run.log`；GitHub hosted CI 尚未執行。

## 2026-10-14 M5 共用 Operator 修正後回歸

上方原始驗收時點保留；後續 M5 真實驗收發現舊 ReplicaSet FailedCreate／Deployment ProgressDeadlineExceeded 被誤歸到新 revision，決策見 [ADR 0012](adr/0012-current-revision-replicaset-evidence.md)。目前 revision 同時綁定 Pod template，失敗須有目前 owned／observed ReplicaSet 與對應時間證據。相同配置及純縮放的新 revision 建立獨立 rollout；同 revision 重送仍保持 ReplicaSet／Deployment generation 不變，kind assertions 同步保護兩種行為。

make operator-image-build test-e2e-m1 exit 0，TestM1KindLifecycle 109.55 秒、TestM1Authorization 0.65 秒。全部生命週期、真實 HTTP／Ready Pods、漂移與刪除重建、Operator 重啟、真正壞版 ProgressDeadlineExceeded、新 revision 恢復、GC 通過。新增 ReplicaSet get/list/watch 允許、create/update／跨 namespace 拒絕的 SubjectAccessReview 亦通過；原 Secret／exec／Namespace／其他 namespace 拒絕保留。正式入口清理 owned cluster，外層其他容器 ID／狀態比對不變。證據 artifacts/m1-e2e/20261014T080159-44045/run.log、artifacts/m5-development/final-m1-deadline-regression.log；最新完整 Unit／race／envtest／build／lint 結果見 [M5 紀錄](m5-validation.md)。Hosted CI 尚未執行。
