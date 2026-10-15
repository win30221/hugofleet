# 技術文件與驗證

完整操作教學與程式導讀位於 [stages](../stages/README.md)。本目錄提供 API 契約、架構決策、驗證方式與驗收證據。

| 文件 | 用途 |
|---|---|
| [驗證總覽](validation.md) | 工具前置、正式驗證命令、各階段結果、對應程式提交的 Hosted CI 與限制 |
| [M0 驗收](m0-validation.md) | 價格 API、三版映像、HTTP 與原生 rollout |
| [M1 驗收](m1-validation.md) | CRD、Operator、狀態、更新、漂移修復與 GC |
| [M2 驗收](m2-validation.md) | UI／REST、SQLite、部署履歷、回滾與程序恢復 |
| [M3 驗收](m3-validation.md) | 雙 Worker、mTLS、斷線修復、ledger 與重連 |
| [M4 驗收](m4-validation.md) | OIDC、租戶角色、Quota、Security Profile 與稽核 |
| [M5 驗收](m5-validation.md) | Dashboard、trace／metrics、Helm lifecycle 與瀏覽器示範 |
| [Demo 播放頁](media/m5-demo.html)／[MP4](media/m5-demo.mp4)／[回滾 trace](media/m5-rollback-trace.html) | 真實操作截圖剪輯與跨 Hub／Agent／Operator 的實際追蹤 |
| [M2 API](m2-api.md)／[OpenAPI](openapi.yaml) | M2／M3 開發模式 API 與部署履歷規則 |
| [M4 OpenAPI](openapi-m4.yaml)／[M5 OpenAPI](openapi-m5.yaml) | OIDC session、Origin／CSRF、租戶授權、Dashboard 與 telemetry 契約 |
| [ADR](adr/) | 版本、權限、通訊、儲存與交付的設計決策 |

產品與里程碑規格以 [HUGOFLEET_SPEC.md](../HUGOFLEET_SPEC.md) 為準。測試結果須對應明確的命令與程式版本；操作教學中的預期結果供讀者核對，不能單憑文件描述宣稱驗收通過。

## 文件分工

| 位置 | 內容 |
|---|---|
| 根 README | 專案定位、環境、主要資料夾與學習入口 |
| stages/README | 階段選擇、元件位置、切換與重建 |
| stages/mN/README | 完整操作、預期結果、排查與結束方式 |
| 元件 README | 函式流程、介面、資料、錯誤與測試邊界 |
| docs API／OpenAPI | 精確請求、回應、狀態與錯誤契約 |
| docs ADR | 需求、設計、替代方案與限制 |
| docs validation | 驗證命令、測試範圍、證據索引與未驗收項目 |

元件導讀：[價格 API](../stages/m0/pricing-api/README.md)、[Controller](../stages/m1/controller/README.md)、[Hub](../stages/m2/hub/README.md)、[Agent](../stages/m3/agent/README.md)、[Hub Transport](../stages/m3/agenttransport/README.md)。目錄設計見 [ADR 0009](adr/0009-stage-source-layout.md)。

## 維護方式

教學列明執行目錄、Shell、環境與終端分工，保留一條首次操作主線；展開腳本作為替代路徑。每個練習說明目的、命令、預期結果、失敗排查與資料保留。版本、UID、status 對照實際欄位，不混用 202、ACK、Ready 與 Release 成功。

共用元件的深入說明只維護一份，後續階段連回原文件並列出使用位置。生成檔標明來源與生成入口，DB／憑證／ownership 標明保存方式。公開文件直接描述功能、設計與操作，不加入開發日期、對話授權或整理歷程；測試證據使用程式提交、CI run 與日誌索引。
