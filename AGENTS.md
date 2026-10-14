# AGENTS.md — HugoFleet AI 開發協作規則

本檔提供給 Codex、Claude Code、Cursor 等 Coding Agent。**產品與架構規格以 `HUGOFLEET_SPEC.md` 為準**。

**GitHub 專案路徑**：`https://github.com/win30221/hugofleet`；**Go module**：`github.com/win30221/hugofleet`。請勿再使用 GitHub 帳號佔位文字。

## 最高優先原則

- 只開發使用者指定的目前里程碑；勿提前實作後續階段或產生無用空骨架。
- 應用程式、Controller、Hub、Agent 與驗收工具以 Go 實作；允許 Bash／sh 開發與部署腳本。UI 使用 Go html/template；HTML／CSS、YAML、Dockerfile、Makefile 為必要模板與設定。
- 硬體預算：32 GB RAM、Intel 顯示晶片、kind、CPU-only；不要加入 LLM / KServe / vLLM / Istio / Rook-Ceph。
- 一定要以可運作的真實程式完成端到端流程，不能用 Mock 偽裝完成。
- Operator 必須實際 Reconcile Kubernetes `Application` CRD → Deployment + Service，具備狀態、更新、刪除、漂移修復。
- M3 真正多叢集的 Hub **不得持有 Worker cluster-admin kubeconfig**；Worker Agent 主動向 Hub 建立受 mTLS 驗證的 gRPC 雙向控制通道。
- 不引入可執行任意 shell、kubectl exec、任意 YAML 或擴權的「管理便利功能」。
- 僅使用官方文件確認過的相容版本並固定版號，改架構要寫 ADR。
- 禁止宣稱未在當前環境成功執行的測試／Demo 已通過。

## 工作流程

1. 先讀 `HUGOFLEET_SPEC.md`、`README.md`、已有的程式碼與當前測試。
2. 列出本里程碑的**最小垂直切片**、檔案變更與測試計畫。
3. 以小變更開發，保持 API、資料庫與 Controller 的邊界清晰。
4. 運行相關 `go test`、envtest、kind E2E（視里程碑而定）。
5. 失敗時先找原因；不要移除測試或改弱 Assertion 以取得綠燈。
6. 更新文件、列出實測命令與結果、說明已知限制。
7. 停止於本里程碑，等待下一項具體任務。

## Done 標準

- [ ] 完成本階段驗收條件
- [ ] 程式可格式化、Lint、Build
- [ ] 相關 Unit/Integration/E2E 已執行且報告結果
- [ ] 有錯誤處理、Context/Timeout、必要測試
- [ ] README 包含實際啟動與驗證命令
- [ ] 未提交 Token、密碼、憑證私鑰、SQLite 資料或其他 Secrets
- [ ] 未執行任何超出專案範圍的破壞性清理

## 授權與整理歷程

M0／M1／M2 已完成本機驗收；紀錄見 `docs/m0-validation.md`、`docs/m1-validation.md`、`docs/m2-validation.md`。使用者於 2026-10-11 已明確授權開始 **M3**，並要求各階段可獨立執行與逐階段理解。M3 功能實作及搬移前的真實雙叢集 E2E 已通過，實際範圍見 `docs/m3-validation.md`，勿將它等同於搬移後全部入口已驗證。

**當時只整理資料夾與文件。** 使用者當時指示：「你先整理到我覺得可以的程度，我們再開始重新驗證吧」。程式／YAML／JSON／Bash／Dockerfile／教學集中於 `stages/m0`～`m3`，先交付可閱讀的整理結果；等使用者確認結構並授權恢復驗證，才執行編譯、測試、Demo 或重建驗證叢集。既有驗收紀錄保留，未完成或取消的項目不得列為通過。當時停止於 M3；後續授權以「目前開始指令」為準。

使用者接著授權整理與清除不必要的內容，目的是專注重要檔案。本輪已將階段 Bash 歸位、精簡根 README、合併階段導航，並清除明確可再生的殘留；搬移備份與歷史診斷歸入 `.local/archive`。此授權不代表恢復編譯、測試或 Demo；當時仍維持暫停驗證。重新驗證時從正式 Make／stages 入口進行，archive 工具只作歷史參考。

使用者再授權全面更新用法，要求 stages 的實際操作寫得詳細。文件分為根概覽、stages 導航、各階段完整操作與重要元件深入 README；需依現有腳本／程式核對命令、預期結果與資料保留方式。此次是文件授權，不恢復編譯、文件檢查工具、測試或 Demo；不得將新教學當成新的實測證據。

使用者接著授權將專案搬到 `C:\Users\H\Documents\GitHub\win30221\hugofleet` 並更新寫死路徑，另指定可重新產生的內容不要帶入。現行 WSL 根路徑為 `/mnt/c/Users/H/Documents/GitHub/win30221/hugofleet`；保留新位置既有 Git 設定，舊 Git 歸檔於 `.local/archive`，不帶入 `.tools`／`.cache`／`bin` 及舊 native-cache.path。證據與私有資料仍由 `.gitignore` 排除；本次授權不含 commit、push 或恢復編譯／測試／Demo。

## 目前開始指令

2026-10-15 已完成本輪授權：F01～F10 修正、本機 M0～M5 驗證、提交與推送，以及 Hosted CI 實測。修正提交 e8d20d6 的第二輪 GitHub Actions 九個 jobs 全部 success，T18 通過；首次失敗與修復證據見 `docs/release-validation-20261014.md`。後續文件收尾提交與被測程式 SHA 分開記錄。停止於 M5；下列未授權／未執行敘述為歷史紀錄。

2026-10-14 最新授權：使用者要求修正稽核問題、完整驗證 M0～M5 流程與文件，完成後提交並推送至既有 origin，實際驗證 Hosted CI。此授權取代下列歷史紀錄中的「尚未授權 commit／push／Hosted CI」限制；仍須排除私有資料並保留真實測試結果。不得以本機 PASS 宣稱遠端通過。

使用者於 2026-10-11 新授權：「幫我測試 M0～M3 有沒有正常」，並要求依各自文件、從主目錄開始看所有 README 是否正確可用、順便檢查問題。**驗證已恢復**：可從正式 Make／stages 入口安裝本地固定工具、編譯、文件檢查、Unit／race／envtest、真實 kind E2E 與各階段教學操作；必要問題修正需重驗。使用新的 M2 DB／M3 state 保留原履歷與憑證，不碰非專案資源。新結果寫入 `docs/revalidation-20261011.md`，歷史紀錄保留原意。

使用者接著明確授權：「M0～M3 如果都檢查完也修正完之後，開始把 M4 M5 都一次做完」。先完成本輪 M0～M3 檢查、修正與重新驗證，再依 `HUGOFLEET_SPEC.md` 順序完成 M4 與 M5 的真實程式、文件與驗收；無須再次詢問開始這兩階段的許可。M4 通過後才進入 M5，不提前建立空骨架。尚未授權 commit／push；不得宣稱未執行的 hosted CI 已通過。

2026-10-14 已完成 **M4／M5 實作與本機驗收**：最新完整 Unit／race／envtest／build、M5 全新雙 Worker 安全／Dashboard／trace／metrics／Helm 真實正式入口、最新 M1～M4 回歸及 owned cleanup、M4／M5 真實瀏覽器流程皆通過，結果見 `docs/m4-validation.md`、`docs/m5-validation.md`。停止於 M5，不提前開發後續功能。**尚未授權 commit／push；Hosted CI 未執行，不能列為通過**。若要完成 Hosted CI，需使用者另行明確授權提交及推送。

2026-10-14 使用者進一步確認本次只要求**本機能正常運作與測試通過**。此範圍已完成，結束本輪 M0～M5 工作；不等待 commit／push 授權，不執行 Hosted CI。保留 CI workflow 與 T18 未執行的紀錄，不能將其列為通過；未來提交、推送或遠端驗證須由使用者另行提出並授權。
