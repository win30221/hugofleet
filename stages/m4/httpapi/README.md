# M4 管理 API 與租戶 UI

本頁是元件導讀。實際練習見 [M4 逐步操作](../README.md#逐步操作)；遇到執行問題先查[排查方式](../README.md#操作遇到問題時)，結束或重開環境依[停止與下次繼續](../README.md#停止與下次繼續)。修改後的驗證範圍見 [M4 驗收](../../../docs/m4-validation.md)。

## 要查 API、頁面還是權限

想實際操作，先完成 [M4 教學](../README.md)；要寫 client，先讀 [M4 OpenAPI](../../../docs/openapi-m4.yaml)。本頁把請求對到程式，命令從專案根目錄執行。

| 你要修改什麼 | 位置 | 應核對的結果 |
|---|---|---|
| session／Origin／CSRF 或 JSON | server.go | 未授權、錯誤 Origin／CSRF 都被拒絕 |
| 表單與確認流程 | ui.go、templates | 建立只回已接受，刪除／回滾需確認 |
| 租戶角色與資料可見範圍 | 共用 Store 的 scoped／交易授權 | 跨租戶與撤權後重送仍拒絕 |
| M5 頁面與 telemetry | NewWithOptions 與 Dashboard 測試 | 保持 M4 預設行為，telemetry 僅 Admin |

先執行 `make m4-test`；修改共用 M5 行為也執行 `make m5-test`。Worker 隔離與真實登入另由 E2E 驗證。

## 請求如何進入業務服務

操作與端點表見 [M4 教學](../README.md)。[server.go](server.go) 包含 session／Origin／CSRF middleware、JSON API、audit 與 request_id；[ui.go](ui.go) 將 HTML form 轉成共用 service 的型別化請求；[templates](../web/templates/page.html) 只呈現可用的角色操作。

精確 REST／登入契約見 [M4 OpenAPI](../../../docs/openapi-m4.yaml)，共用 application／release／operation 回應模型引用 [M2／M3 OpenAPI](../../../docs/openapi.yaml)。M4 的管理 API 要求 `hf_session`，變更另要求 Origin／X-CSRF-Token；舊模式的 Bearer／無認證例外不適用。配額端點回傳已核對的 hard／defaults profile，並非即時 usage。

每次請求有新的 request_id 與十秒 context。HTTP LocalDemo 核對精確 Host／本機或 Docker gateway；正常模式要求 TLS。所有非公開入口從 SQLite 查驗不透明 session，再以受信任 actor context 呼叫 Store。安全 headers 包含 no-store、nosniff、CSP；公開 OAuth 頁面使用 Referrer-Policy: no-referrer，受保護頁面使用 same-origin，讓瀏覽器 HTML POST 保留同源 Origin。缺少／null／外站 Origin 與錯誤 CSRF 仍拒絕；audit 不存 Token／body／callback query。

讀取透過 scoped tenants／clusters／apps／detail／operation／audit；變更在 Store 交易內重新授權，且在 idempotency lookup 之前檢查 membership。Viewer 唯讀；Developer 可以 deploy／release／rollback；Owner 增加 delete／members；Platform Admin 可啟用已 bootstrap 的 profile。登入 claims、未受信任的角色 header 或部署欄位不授予權限；角色變更只能經過已授權 Owner／Admin 的 membership 入口。跨租戶指定 app ID 不會繞過授權。

變更 API 要求 JSON 單一文件、明確欄位、64 KiB 上限、精確 Origin 和 CSRF。HTML form 有同樣的大小與 CSRF 邊界，刪除／回滾另要求確認欄位與期望 revision。HTTP 202 表示 SQLite 已接收意圖；UI 顯示 QUEUED 與真實操作連結，不立刻宣稱成功。

Worker Admission 的 FailedCreate／ReplicaFailure 由共用 transport／Operator 傳回。UI 保留 desired／applied／successful revision、immutable Releases、Operation 終態、事件、request_id 與最後觀測時間；晚到 Ready 不重寫失敗履歷。狀態來自真實回報，不用靜態示範資料。

[server_test.go](server_test.go) 使用真實 SQLite 驗證 session、CSRF、角色、scoped HTML 與危險確認；Unit transport 僅測 API 邊界，不列作 Kubernetes 驗收。真正多叢集證據來自 [M4 kind E2E](../test/e2e/security_test.go)。

M5 使用同一個 middleware／form service，經 NewWithOptions 啟用 [Dashboard](../../m5/README.md) 的 assets、HTML 頁面與 Admin-only telemetry；M4 的 New 預設關閉這些選項。共用安全驗收實作在 [suite.go](../test/e2e/suite.go)，M4 測試 wrapper 明確選 m4，M5 另外核對精確 Helm release ownership。新頁面的真實 SQLite／HTTP Unit 在 [dashboard_test.go](dashboard_test.go)。

## 讀完後去哪裡

- 返回 [M4 逐步操作](../README.md#逐步操作)，對照本輪結果。
- 需要停止、換課或再次練習，前往 [M4 完成後的選擇](../README.md#完成這一課後去哪裡)。
- 想繼續理解其他元件，回到 [M4 原理與進階閱讀](../README.md#原理與進階閱讀)。
