# ADR 0003 — Go 實作產品與驗收程式

- 狀態：已採納。

## 決策

產品服務、Hub、Operator、Agent 與測試／驗收程式使用 Go。Bash／sh 保留為 Makefile 的流程工具，串接 Docker、kind、kubectl；YAML、Dockerfile、HTML／CSS 保留必要的設定與模板。

M0 的 HTTP 與 Kubernetes JSON 驗收使用 stdlib Go `stages/m0/cmd/m0check`，不依賴 Python。M2～M5 的 UI 由 Hub 以 Go `html/template` 渲染，頁面與 REST API 共用 service 層。

## 取捨

Go template UI 與後端共用語言、型別與建置流程，無需 Node.js 工具鏈。互動以伺服器渲染表單及有界頁面更新為主；產品重點為 Kubernetes Operator、gRPC、多叢集、安全與可觀測性。

## 閱讀後返回

本頁用來查設計原因，不是安裝步驟。返回[選課與回訪](../../stages/README.md#回來繼續哪一課)接續原課。其他設計問題見 [ADR 導覽](../README.md#設計問題與-adr-對照)。
