# ADR 0009 — 依教學階段分類原始碼

- 狀態：已採納。

## 背景

每個階段需要可直接開始的教學、可找到的程式與獨立執行資料。共用元件也必須維持單一實作，避免各課複製邏輯後產生差異。

## 決策

Go 程式、API 型別、YAML／JSON、Bash、Dockerfile 與測試集中在 `stages/m0`～`m5`，依元件首次引入的階段分類。各階段根目錄提供 README、run.sh 與範例；scripts/ 放控制、工具與驗收腳本。README 列出新增功能、檔案地圖、閱讀順序、操作、測試與共用依賴。

使用單一 module `github.com/win30221/hugofleet`。M0 pricing-api、M1 API／Controller、M2 Hub 被後續階段共用；跨階段套件不放在限制 importer 的 internal 目錄。pricing-api 私有的 internal/pricing 保留。每個階段使用現行共用程式，不建立多份版本快照。

跨階段工具放在根 scripts／tools／deploy。`scripts/kind.sh` 管理 kind lifecycle 與 ownership，`scripts/hf.sh` 分派 Make 命令；每階段的 scripts/control.sh 與 scripts/e2e.sh 管理自己的操作及驗收。各學習入口自行準備元件，不要求其他階段持續執行。

`.local/tutorials` 保存私有 kubeconfig、DB、憑證與 ownership，建置工具、快取、bin 與 artifacts 由 Git 忽略。docs 保存 API 契約、ADR、驗證方式及證據索引。根 README 提供概覽，stages/README 統一階段選擇、切換與重建導覽。

## 影響與驗證

套件路徑必須與 import、Proto go_package、controller-gen、Docker COPY／build、Make、E2E、envtest CRD 路徑及文件連結一致。修改路徑時檢查這些入口；有功能變更或具體失敗時，再擴大相關測試。

各階段 build／Unit 命令方便單獨閱讀與開發；真實叢集流程由獨立入口及 kind E2E 驗收。測試方式、證據與限制見 [驗證總覽](../validation.md)。

## 閱讀後返回

本頁用來查設計原因，不是安裝步驟。返回[選課與回訪](../../stages/README.md#回來繼續哪一課)接續原課。其他設計問題見 [ADR 導覽](../README.md#設計問題與-adr-對照)。
