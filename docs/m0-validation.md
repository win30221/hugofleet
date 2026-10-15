# M0 驗收範圍

M0 驗證 Go pricing-api、三個實際映像與原生 Kubernetes Deployment／Service。操作步驟見 [M0 教學](../stages/m0/README.md)，跨階段實測與 CI 證據見 [驗證總覽](validation.md)。

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
