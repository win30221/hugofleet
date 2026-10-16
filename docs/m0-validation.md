# M0 驗收範圍

M0 驗證 Go pricing-api、三個實際映像與原生 Kubernetes Deployment／Service。操作步驟見 [M0 教學](../stages/m0/README.md)，跨階段實測與 CI 證據見 [驗證總覽](validation.md)。

## 如何使用本頁

第一次操作請先讀本階段教學；本頁用來選擇自動驗證與核對既有證據。只修改文件時執行 `make docs-check`，毋須啟動本頁整套測試。

| 命令 | 目的 |
|---|---|
| `make m0-build m0-test` | 編譯 M0 主機程式、執行 M0 單元測試 |
| `make benchmark` | 價格計算 benchmark，不代表叢集效能 |
| `make demo-image-build` | 準備三個價格 API 映像 |
| `make test-e2e-m0` | 真實 HTTP／rollout／恢復與清理 |

下方依序列出本階段需要的工具與驗收命令；已安裝的固定工具可沿用，不需準備其他階段的工具。一次執行一組 E2E，先依[停止流程](../stages/m0/README.md#停止與下次繼續)結束會占用相同連接埠的教學環境。

## 驗收命令

在專案根目錄的 Ubuntu WSL2／Linux Bash 執行：

```bash
make tools
make doctor
make m0-build m0-test
make benchmark
make demo-image-build
make test-e2e-m0
```

## 如何判斷本次執行結果

等待整個命令結束，核對測試與 owned cleanup 均成功且最終退出碼為 0。中途出現 PASS 不表示整體完成；失敗先保留輸出與 state，依[本階段排查](../stages/m0/README.md#操作遇到問題時)定位問題。下列已確認行為來自既有驗收，對應證據與程式提交見驗證總覽，不會自動套用到新修改。

## 已確認的行為

- 價格以整數分計算，VIP 向下取整後再折價；20,000 分搭配 VIP 與 SAVE100 得到 8,000 分。
- HTTP 拒絕缺值、負數、超限、未知欄位、錯誤 JSON／tier／coupon、超大 body 與錯誤 method；日誌不輸出 Authorization。
- v1／v2-good 真實 rollout 與 HTTP 探針成功；v2-broken health=200、ready=503、Pod Ready=false，且不靠 liveness 重啟偽裝故障。
- 壞版得到 ProgressDeadlineExceeded；EndpointSlice 排除未就緒 Pod，上一版仍可服務。原生 image update 恢復 v1 後重新驗證 HTTP。
- 正式入口從零建立 owned kind，等待 Node／CoreDNS，完成 assertions 後清理；拒絕非專案 cluster 與非法 port。
- 本機及對應 Hosted CI 的正式 M0 E2E 均有 PASS 證據。原始日誌在 `artifacts/m0-e2e/<run>`，整合日誌索引見驗證總覽。

## 限制

M0 沒有 Application CRD、Hub 發版履歷或平台回滾；恢復使用原生 Kubernetes image update。RAM etcd profile 的通過結果不代表 disk profile、node 重啟或持久儲存恢復已驗收。port-forward 與 EndpointSlice 核對不等於一般 CNI／kube-proxy 資料路徑測試。

benchmark 只量價格計算函式；Docker memory 取樣只反映取樣狀態，不能當作峰值或整機資源保證。儲存設計見 [ADR 0004](adr/0004-m0-ephemeral-etcd.md)。

## 查完後返回

本頁的測試不是初次學習的必經步驟。返回 [M0 整課驗證](../stages/m0/README.md#整課驗證與常見問題)核對手動操作；要停止或進下一課，前往 [M0 完成後的選擇](../stages/m0/README.md#完成這一課後去哪裡)。修改其他元件時，依[驗證總覽](validation.md#執行驗證)選擇測試。
