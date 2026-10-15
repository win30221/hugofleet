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

## 專案範圍與文件原則

- 功能範圍為 M0～M5，不提前開發後續功能；各階段保持獨立操作入口。
- 教學從根 README 與 stages 導覽開始；API 契約、ADR 與驗收範圍放在 docs。
- 文件直接描述功能、設計與操作，不加入開發日期、對話授權、搬移或重跑經過。
- 驗收證據以命令、程式提交、CI run、結果與限制呈現；不得把既有 PASS 當作文件編輯或後續程式修改的新驗收。
- 跨階段驗證方式與證據見 [驗證總覽](docs/validation.md)；各階段細節見 docs/m0-validation.md～m5-validation.md。
- 主要操作環境為 Ubuntu WSL2／Linux amd64。Windows checkout 位於 `C:\Users\H\Documents\GitHub\win30221\hugofleet` 時，WSL 路徑為 `/mnt/c/Users/H/Documents/GitHub/win30221/hugofleet`。
- 工具、快取與 bin 可重新產生；DB、憑證、kubeconfig、ownership 與原始驗收證據由 Git 忽略，不當作一般快取清除。

## 任務範圍

依使用者指定的任務選擇變更與驗證範圍。文件修改執行文件檢查；程式修改執行受影響的測試。新增功能、重建環境、提交與推送依該任務的授權處理，不將已結束任務的授權延伸到新工作。
