# 共用 Go 開發工具

修改文件後，從專案根目錄的 WSL／Linux Bash 執行：

```bash
make docs-check
```

前置為已執行 `make tools`。預期成功時輸出 PASS 與實際掃描數量，命令返回 0；失敗時依輸出的檔案／連結／區塊修正，再重跑。數量會隨文件內容變動，不應固定比對舊數字。

命令結束即完成，沒有常駐服務要停止；再次檢查直接重跑相同命令。教學順序與每步預期結果仍需人工核對，維護方式見 [文件分工](../docs/README.md#維護方式)。

這是文件檢查，不會建立叢集、呼叫範例 API 或驗證部署成功。程式修改要依[驗證總覽](../docs/validation.md)另選相關測試。

## 工具放在哪裡

[doccheck/main.go](doccheck/main.go) 是文件檢查工具的原始碼：檢查本機檔案／章節連結、Bash 範例語法，以及各階段部署 JSON 的型別／設定。入口為 make docs-check。

這裡放專案自行維護的 Go 工具；[scripts](../scripts/README.md) 放 Bash 流程；.tools 放下載／安裝完成的外部工具。它們分工不同，維持分開。

各階段的 Go 驗收工具與 E2E 仍放在自己的 [stages 資料夾](../stages/README.md)。驗證方式與證據見 [驗證總覽](../docs/validation.md)。

## doccheck 怎麼用與怎麼看結果

在專案根目錄的 WSL Bash 執行 make docs-check。它會 go run 此工具，失敗印出問題檔案／連結／Bash 區塊或 JSON 請求、返回非零；成功列出實際掃描數量。

| 檢查 | 目前實作範圍 |
|---|---|
| Markdown 本機連結 | 根 README／AGENTS／SPEC、docs／stages／scripts／tools／deploy 下的 Markdown，核對檔案／目錄存在與 ATX 標題的章節連結 |
| Bash 範例 | 對 bash／sh fenced code block 使用 bash -n，檢查語法 |
| 建立應用 JSON | M2 create-app、M3 deploy-a／b 與規格 §7.1，按 Go 型別解碼、拒絕未知欄位與檢查初始 revision／spec |

章節檢查支援本專案使用的 `#` 標題、中文與重複標題，忽略 fenced code block 裡的標題。它不請求外部網站、不執行範例命令，也不能證明 kind／Docker／TLS／rollout 可用；文件維護仍需閱讀核對。不要把 docs-check PASS 寫成各階段 Demo 已通過。

文件錯誤修文件；行為與教學預期不一致時先查實際原因，不能只弱化工具或測試讓結果變綠。原始日誌留在 artifacts，驗收摘要寫在 docs 對應 validation。


## 查完後返回

若從環境準備前來，回到[根 README](../README.md#首次準備環境)完成檢查，再[開始 M0](../stages/m0/README.md)。若正在練習某一課，從[選課與回訪](../stages/README.md#回來繼續哪一課)返回原課；沿用原環境與 state，不需因閱讀本頁重新啟動整課。
