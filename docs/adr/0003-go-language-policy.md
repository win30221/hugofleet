# ADR 0003 — Go 實作產品與驗收程式

- 日期：2026-10-10
- 狀態：已採納（使用者指定應用開發以 Go 為主，允許 Bash／sh）

## 決策

產品服務、Hub、Operator、Agent 與測試／驗收程式使用 Go。Bash／sh 保留為 Makefile 的流程工具，串接 Docker、kind、kubectl；YAML、Dockerfile、HTML／CSS 保留必要的設定與模板。

M0 將 Python HTTP 與 Kubernetes JSON 驗收改為 stdlib Go `stages/m0/cmd/m0check`，移除 Python 執行需求。M2／M5 的 UI 選型改為 Hub 以 Go `html/template` 渲染頁面，頁面與 REST API 共用 service 層；本次只更新規格，不提前建立 UI。

## 取捨

Go UI 無需 Node.js 工具鏈，但不展示職缺的 SvelteKit 加分項。保留 Kubernetes Operator、gRPC、多叢集、安全與可觀測性作為後續里程碑重點。
