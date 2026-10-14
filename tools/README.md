# 共用 Go 開發工具

[doccheck/main.go](doccheck/main.go) 是文件檢查工具的原始碼：檢查本機檔案／章節連結、Bash 範例語法，以及各階段部署 JSON 的型別／設定。入口為 make docs-check。

這裡放專案自行維護的 Go 工具；[scripts](../scripts/README.md) 放 Bash 流程；.tools 放下載／安裝完成的外部工具。它們分工不同，維持分開。

各階段的 Go 驗收工具與 E2E 仍放在自己的 [stages 資料夾](../stages/README.md)。2026-10-11 已恢復文件與功能檢查，結果見 [本輪紀錄](../docs/revalidation-20261011.md)。

## doccheck 怎麼用與怎麼看結果

在專案根目錄的 WSL Bash 執行 make docs-check。它會 go run 此工具，失敗印出問題檔案／連結／Bash 區塊或 JSON 請求、返回非零；成功列出實際掃描數量。

| 檢查 | 目前實作範圍 |
|---|---|
| Markdown 本機連結 | 根 README／AGENTS／SPEC、docs／stages／scripts／tools／deploy 下的 Markdown，核對檔案／目錄存在與 ATX 標題的章節連結 |
| Bash 範例 | 對 bash／sh fenced code block 使用 bash -n，檢查語法 |
| 建立應用 JSON | M2 create-app、M3 deploy-a／b，按 Go 型別解碼、拒絕未知欄位與檢查初始 revision／spec |

章節檢查支援本專案使用的 `#` 標題、中文與重複標題，忽略 fenced code block 裡的標題。它不請求外部網站、不執行範例命令，也不能證明 kind／Docker／TLS／rollout 可用；文件維護仍需閱讀核對。不要把 docs-check PASS 寫成各階段 Demo 已通過。

文件錯誤修文件；行為與教學預期不一致時先查實際原因，不能只弱化工具或測試讓結果變綠。原始日誌留在 artifacts，驗收摘要寫在 docs 對應 validation。
