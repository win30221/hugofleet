# 技術文件與驗收證據

逐階段教學、程式、YAML／JSON 與 Bash 集中在 [stages](../stages/README.md)。本資料夾保留跨階段的技術依據與測試證據：

| 文件 | 用途 |
|---|---|
| [stages 索引](../stages/README.md) | 教學與階段選擇、獨立啟動、切換／重建統一在此 |
| [m0-validation.md](m0-validation.md)～[m3-validation.md](m3-validation.md) | 實測命令、結果、失敗修正與限制；保留各次驗收當時的狀態 |
| [revalidation-20261011.md](revalidation-20261011.md) | 新 GitHub 目錄下的 M0～M3 重新驗證，以及全部正式 README 核對結果 |
| [audit-m0-m5-20261014.md](audit-m0-m5-20261014.md) | M0～M5 稽核發現、原檢查結果與後續修正追蹤；修正不等於已重驗通過 |
| [release-validation-20261014.md](release-validation-20261014.md) | 本次稽核修正後的 M0～M5 完整重驗與 Hosted CI 進度；只記本輪新執行結果 |
| [m4-validation.md](m4-validation.md)／[M4 教學](../stages/m4/README.md) | OIDC、租戶／Quota／Security Profile 的分項實測與操作；全新正式入口與 owned cleanup 已通過 |
| [m5-validation.md](m5-validation.md)／[M5 教學](../stages/m5/README.md)／[M5 OpenAPI](openapi-m5.yaml) | Dashboard／telemetry／Helm 的全新正式本機驗收與最新 M1～M4 回歸已通過；當時尚未執行 Hosted CI，後續結果見本次完整重驗紀錄 |
| [Demo 播放頁](media/m5-demo.html)／[MP4](media/m5-demo.mp4)／[回滾 trace](media/m5-rollback-trace.html) | 真實操作截圖的時間順序剪輯（非連續錄影）與實際三元件 spans；擷取時間及雜湊另附於播放頁 |
| [m2-api.md](m2-api.md) | 單叢集模式的 API 範例與部署履歷規則 |
| [openapi.yaml](openapi.yaml)／[openapi-m4.yaml](openapi-m4.yaml) | M2／M3 開發模式與 M4 OIDC REST 契約；M4 必須使用 session／Origin／CSRF，不能沿用舊模式的認證例外 |
| [adr](adr/) | 版本、權限、通訊與儲存選型的架構決策 |

產品與里程碑規格以根目錄 [HUGOFLEET_SPEC.md](../HUGOFLEET_SPEC.md) 為準。歷史驗收中的「當時尚未開始」描述保留原意，最新完成程度看 [README](../README.md) 與對應階段驗收紀錄。

找程式請看 [stages 索引](../stages/README.md)，來源分類決策見 [ADR 0009](adr/0009-stage-source-layout.md)。

## 哪一層寫哪種文件

| 位置 | 誰會讀／寫什麼 |
|---|---|
| 根 README | 第一次打開專案的人；定位、環境、主要資料夾與學習入口 |
| stages/README | 想選課、找新增元件、切換／重建的人 |
| stages/mN/README | 真的跟著操作的人；首次啟動、理解練習、診斷與結束 |
| 重要元件的 README | 想追程式或修改的人；函式流程、介面、資料、錯誤與測試邊界 |
| docs API／OpenAPI | 寫 client 的人；精確請求、回應、狀態與錯誤契約 |
| docs ADR | 想知道設計原因的人；需求、決策、替代方案與限制 |
| docs validation | 想知道實際結果的人；環境、命令、失敗／取消、原始證據 |

元件導讀：[M0 價格 API](../stages/m0/pricing-api/README.md)、[M1 Controller](../stages/m1/controller/README.md)、[M2 Hub](../stages/m2/hub/README.md)、[M3 Agent](../stages/m3/agent/README.md)、[M3 Hub Transport](../stages/m3/agenttransport/README.md)。小資料夾由階段檔案地圖說明即可。

## 教學維護方式

階段教學先寫「學什麼、增加什麼」，列出檔案用途與共用依賴，明確給執行目錄、Shell、環境與終端分工。第一次操作保留一條主線；展開腳本標成替代路徑，避免快捷啟動與手動建立又各跑一遍。

每個練習說明目的、命令、預期結果、結果含義、失敗從哪查。版本／UID／status 對照真實欄位，不能把 202、ACK、Ready 或歷史成功混用。最後寫停止、保留資料、重建、反覆練習及理解檢查。

共用元件的深入說明只維護一份，後面階段連回原文件，另列新增或修改位置。生成檔標明來源與生成入口；執行資料標明 DB／憑證／ownership，不當作原始碼或隨手清理的快取。

使用文件描述「目前應怎麼操作」；validation 保存「那一次真的做了什麼」。舊紀錄不因新文件或路徑整理更新為 PASS。先前整理沒有執行編譯、文件工具、測試或 Demo；2026-10-11 已依新授權恢復，結果另存 [本輪紀錄](revalidation-20261011.md)。

2026-10-11 專案搬至 `C:\Users\H\Documents\GitHub\win30221\hugofleet`，目前 WSL 操作路徑為 `/mnt/c/Users/H/Documents/GitHub/win30221/hugofleet`。validation、artifacts 日誌與 `.local/archive` 快照中的舊絕對路徑保留作為當次證據；要跟著操作，請使用 stages 的現行教學。
