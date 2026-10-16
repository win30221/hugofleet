# 驗證方式與驗收證據

HugoFleet 的驗證涵蓋 Go Unit、race、真實 envtest、Dex OIDC、Helm CLI 與 M0～M5 kind E2E。各階段的驗收範圍與限制見 [M0](m0-validation.md)、[M1](m1-validation.md)、[M2](m2-validation.md)、[M3](m3-validation.md)、[M4](m4-validation.md)、[M5](m5-validation.md)。操作教學從 [stages](../stages/README.md) 開始。

## 執行驗證

以下命令在專案根目錄、Ubuntu WSL2／Linux amd64 的 Bash 執行。需可連線的 Docker Linux engine；M3～M5 使用非 root 使用者。一次執行一組 kind E2E，避免筆電資源競爭。

先選受影響範圍。下表的命令都在專案根目錄執行，命令返回 0 才表示該項檢查完成；文件中的既有 PASS 不會隨命令自動更新。

| 這次改了什麼 | 先執行 | 能確認什麼 |
|---|---|---|
| Markdown／範例文字 | `make docs-check` | 本機檔案／章節連結、Bash 語法、三份 JSON 檔與規格 §7.1 的型別化建立範例 |
| 某階段 Go 程式 | `make mN-test`、`make mN-build`，N 換成階段數字 | 該課單元測試與主機編譯 |
| M1 API／Controller | 準備 operator-tools／envtest，再 `make verify-generated test-integration` | 生成檔與真實 API server／etcd 行為 |
| Proto | 準備 proto-tools，再 `make proto-check` | 協定生成碼是否一致 |
| Helm Chart | 準備 helm-tools，再 `make m5-chart-test` | 真正 Helm CLI 渲染與負向 values |
| OIDC | `make test-oidc` | 真實 Dex 負向與自然到期，約 15 分鐘 |
| 部署、連線、GC 或隔離流程 | 該階段 kind E2E，見下表 | 真實工作負載與跨元件結果 |

envtest 沒有 kubelet、Deployment controller 或 GC，不能證明 Pod Ready 或 HTTP 可用。docs-check 不執行範例操作。

### 完整回歸的準備與共用檢查

只有要跑完整回歸才依序執行這一組；已安裝的工具不必每次重裝：

```bash
make tools
make operator-tools envtest proto-tools helm-tools
make doctor
make lint test-unit test-integration test-race build
make verify-generated proto-check docs-check m5-chart-test
```

先停止會占用相同 Hub／Dex port 的教學實例，再做 OIDC 與 kind E2E。一次執行一組，不使用 `make -j` 搶占筆電資源：

| 階段 | 命令 | 需要先準備映像嗎 |
|---|---|---|
| M0 | `make test-e2e-m0` | 先 `make demo-image-build` |
| M1 | `make test-e2e-m1` | 先 `make demo-image-build operator-image-build` |
| M2 | `make test-e2e` | 先 `make demo-image-build operator-image-build` |
| M3 | `make test-e2e-m3` | 入口自行建置 |
| M4 | `make test-e2e-m4` | 入口自行建置，先釋放 18094／5556 |
| M5 | `make test-e2e-m5` | 入口自行建置，先釋放 18095／5557 |

`make test-oidc` 自行建立 Dex 與短 session Hub，不需要 Worker；先釋放 5556／18094／18444。各 E2E 使用專用名稱／state，保留日誌並限定清理自己的資源。中斷後用原輸出的 state 與正式 down 恢復清理，不刪 ownership 記錄繞過檢查。

### 如何閱讀結果

檢查命令的最終退出碼、測試失敗與 cleanup 結果，不能只搜尋日誌中間的 PASS。失敗先保留輸出，依對應階段的診斷檢查；修正後重跑受影響項目，再將新命令、程式版本與結果記為新證據。

## 本機驗收證據

驗收環境為 Windows／Ubuntu WSL2、Linux amd64、32 GB RAM、Intel CPU、CPU-only。工具固定版本與校驗碼見 [versions.env](../scripts/versions.env)，Go 相依見 [go.mod](../go.mod)／[go.sum](../go.sum)。WSL 缺少 C compiler 時，正式 race 入口使用固定 Go 容器與真實 envtest。

下表整理已保存的實測結果；不是執行上述命令後自動更新的狀態，也不代表任意後續修改都已通過驗收。日誌路徑為本機純文字索引，`artifacts` 由 Git 忽略，乾淨 checkout 不包含這些檔案。

| 範圍 | 實測結果 | 本機證據（位於 artifacts/release-validation） |
|---|---|---|
| lint／Unit／build／race／envtest／生成檔 | PASS；完整命令 exit 0；包含 Deployment／Service resourceVersion 衝突回歸 | ci-fixes-local.log |
| 文件／Helm CLI／型別化範例 | PASS；包含乾淨 checkout 的文件檢查與負向 values 驗證 | final-checks.log、docs-clean-checkout-final.log |
| 真實 Dex OIDC | PASS；canonical backchannel、JWT 自然到期、Hub 負向與 session 到期，含資源清理 | oidc-entry.log |
| M0／M1／M2 kind E2E | PASS；正式 Make 入口及 owned cleanup | m0-m2.log；M1 衝突修正驗收另見 ci-fixes-e2e.log |
| M3 雙 Worker E2E | PASS；mTLS、離線修復、ACK 遺失、ledger、Hub 重啟與回滾 | m3-m5.log |
| M4 雙 Worker E2E | PASS；OIDC、租戶權限、CPU／memory Admission、Calico、表單與自訂配額保留 | m3-m5.log |
| M5 安全／Dashboard／trace／metrics／Helm | PASS；正式入口與 owned cleanup | m5-retry.log、ci-fixes-e2e.log |

各 E2E 的完整 Kubernetes 快照、HTTP／HTML、程序日誌與診斷另在 `artifacts/mN-e2e/<run>`。帳號、DB、kubeconfig、憑證與私鑰存於 Git 忽略的私有 state，不隨原始碼發布。

## Hosted CI 證據

[成功的 GitHub Actions run](https://github.com/win30221/hugofleet/actions/runs/38649805794) 對應程式提交 `e8d20d6cc6e6126cbb90a54ea00559d927460cac`，九個 jobs 全部 success，涵蓋規格 T18。此結果僅對應該提交；文件編輯或後續程式修改不會自動取得相同結果。

| Job | 已驗收內容 |
|---|---|
| unit | 生成檔／Proto、Helm CLI、lint、build、文件、Unit 與 race |
| envtest | 真實 API server，包含 Deployment／Service 衝突重試 |
| oidc | 真實 Dex、Code／PKCE、負向認證、JWT／session 自然到期與清理 |
| kind M0 | 三版 HTTP 工作負載、原生 rollout 與清理 |
| kind M1 | Operator lifecycle／authorization 與清理 |
| kind M2 | Hub／SQLite／部署回滾與清理 |
| kind M3 | 雙 Worker mTLS、ACK 遺失、ledger、離線修復與回滾 |
| kind M4 | OIDC、租戶隔離、Admission、Calico、表單及 Hub 重建保留 |
| kind M5 | 安全、Dashboard、trace／metrics、Helm lifecycle 與清理 |

Workflow 使用 GitHub-hosted ubuntu-24.04；六階段 kind jobs 依序執行。設定見 [ci.yml](../.github/workflows/ci.yml)。遠端結果與日誌另存於本機 `artifacts/release-validation/github-run-38649805794-result.jsonl` 及 `github-run-38649805794-logs.zip`。

## 回歸保護

回歸測試覆蓋下列邊界。通過結果取自完成全部斷言與清理的執行；`artifacts/release-validation/conflict-before.log` 是衝突問題的失敗重現證據，不列為 PASS。

- Hub 重建只更新受 ownership 保護的 Agent endpoint，保留自訂 quota、Hub profile、session、receipt、ledger 與 M5 Helm 設定。
- 新 revision 綁定 Pod template；舊 ReplicaSet 的 FailedCreate／ProgressDeadlineExceeded 不得終結新操作。設計見 [ADR 0012](adr/0012-current-revision-replicaset-evidence.md)。
- Kubernetes resourceVersion conflict 交由 workqueue 重試，不能直接寫成 Degraded；ownership 衝突仍拒絕接管。
- Agent rollout 後等待真實 Hub CONNECTED，再進行需要控制通道的操作。
- 文件本機連結只指向版本控制中的檔案，原始 artifacts 以純文字路徑索引，避免乾淨 checkout 的連結失效。
- 真實 OIDC 負向與自然到期測試有獨立正式入口；一般 Unit 不取代 Provider 驗收。

## 驗證邊界

- kind 的 RAM etcd 只適合拋棄式環境。程序重啟測試不等於 Docker node／WSL／主機斷電或備份還原已通過。
- leaf 憑證 24 小時、CA 48 小時。未以等待 24 小時或保留原 DB／Workers 的原地輪替作驗收；到期練習使用新的 state。
- E2E 正常／失敗路徑的 owned cleanup 有實測，未額外宣稱所有強制終止情境都能自動清理。
- M4／M5 只有啟用 Calico 的 A Worker 宣稱 NetworkPolicy enforcement；B 不作相同保證。
- `kubectl port-forward service` 連到選定 Pod，不替代一般 Service dataplane 驗收。
- 單一 Hub／SQLite、開發憑證與 loopback HTTP Demo 不構成生產級 HA／IAM／災難復原保證。


## 查完後返回

如果只是跟著教學操作，回到[選課與回訪](../stages/README.md#回來繼續哪一課)的原課，不必為了前往下一課先執行完整回歸。若正在修改程式，依本頁選擇受影響的測試，完成後核對結果與限制。
