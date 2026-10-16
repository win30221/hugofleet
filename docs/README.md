# 技術文件與驗證

完整操作教學與程式導讀位於 [stages](../stages/README.md)。本目錄提供 API 契約、架構決策、驗證方式與驗收證據，是主線之外的查閱入口。第一次操作從[根 README](../README.md#第一次使用從這裡開始)直接進 M0，不需先讀完本目錄。

## 先判斷你需要哪一種文件

| 你要做的事 | 從哪裡開始 | 不需要先做什麼 |
|---|---|---|
| 第一次把服務跑起來 | [根 README](../README.md)、[階段教學](../stages/README.md) | 不必跑整套自動驗收 |
| 修改程式後選擇測試 | [驗證總覽](validation.md#執行驗證) | 不必無差別啟動六課 Demo |
| 用 REST 取代 UI | 下表對應模式的 API／OpenAPI | 不要混用 M2 無登入、M3 Token 與 M4／M5 session |
| 查一個既有 PASS 證明了什麼 | 各階段 validation 的命令、範圍與限制 | 不把舊證據當成新修改的結果 |
| 理解為何採用此設計 | 下方 ADR 對照表 | 不需照設計文件重新安裝環境 |

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

## 設計問題與 ADR 對照

ADR 記錄設計決策與取捨，操作仍以階段教學為準。查到相關決策後，先讀「問題／背景」，再看「決策」及「限制」。

| 你想知道的原因 | 決策 |
|---|---|
| 為什麼使用 WSL／專案固定工具 | [0001 開發環境](adr/0001-m0-development-profile.md) |
| revision、Agent 與驗證身分怎麼約定 | [0002 契約](adr/0002-revision-agent-and-auth-contracts.md) |
| 為什麼核心與驗收使用 Go | [0003 語言範圍](adr/0003-go-language-policy.md) |
| 為什麼重啟 node 後不能假設資料仍在 | [0004 RAM etcd](adr/0004-m0-ephemeral-etcd.md) |
| 為什麼以 Application 管理 Deployment | [0005 Operator](adr/0005-m1-operator.md) |
| WSL 建置快取為何可移到 Linux filesystem | [0006 快取](adr/0006-wsl-native-go-cache.md) |
| 為什麼需要 SQLite、Release 與 Operation | [0007 Hub](adr/0007-m2-hub.md) |
| Hub 沒有 Worker kubeconfig 如何管理叢集 | [0008 主動 Agent](adr/0008-m3-outbound-agent.md) |
| 為什麼依階段放程式卻共用一份 module | [0009 目錄](adr/0009-stage-source-layout.md) |
| 登入角色與 Worker 安全資源如何分工 | [0010 OIDC／租戶](adr/0010-oidc-tenants-and-worker-bootstrap.md) |
| trace、metrics 與 Helm 安裝的邊界 | [0011 觀測與交付](adr/0011-observability-and-delivery.md) |
| 為什麼新 revision 要辨別自己的 ReplicaSet | [0012 失敗證據歸屬](adr/0012-current-revision-replicaset-evidence.md) |

## 維護方式

階段教學統一依序安排「這一課的目標與成果 → 開始前的準備 → 逐步操作 → 整課驗證與常見問題 → 停止與下次繼續 → 原理與進階閱讀」。依操作順序切換終端與瀏覽器，不把同一終端的所有命令集中成一串。

每個步驟說明用途、執行位置、命令與預期結果；尚未達成結果時先排查，不繼續後續變更。共用安裝連回根 README；自訂 state 必須在第一次啟動前選定。日常操作以 make 為入口，底層腳本留在進階說明。根 README 直接帶入 M0；每課主線結尾提供下一課、接續練習與深入閱讀三個方向。支線文件在結尾連回原課或選課入口，不把 API、ADR 或完整驗收當成前進的必要關卡。停止流程區分暫停、完整清理、繼續同輪與重新練習，列明資料保留與限制。

API 文件按請求、回應與錯誤分類；元件 README 按用途、程式流程與修改驗證組織；ADR 與驗收文件維持設計決策及證據用途，不強制套用操作教學結構。版本、UID、status 對照實際欄位，不混用 202、ACK、Ready 與 Release 成功。

共用元件的深入說明只維護一份，後續階段連回原文件並列出使用位置。生成檔標明來源與生成入口，DB／憑證／ownership 標明保存方式。公開文件直接描述功能、設計與操作，不加入開發日期、對話授權或整理歷程；測試證據使用程式提交、CI run 與日誌索引。


## 查完後返回

回到[選課與回訪](../stages/README.md#回來繼續哪一課)選擇原課；每課的「完成這一課後去哪裡」會帶你停止環境或接續下一課。API、ADR 與驗收文件也各有返回教學的入口。
