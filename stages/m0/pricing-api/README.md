# pricing-api 程式與使用導讀

本元件是被 HugoFleet 部署的業務服務。完整 kind 啟動、更新、壞版與恢復看 [M0 教學](../README.md)；後面的 M1～M3 使用同一服務。這裡把 HTTP 操作對到程式，方便讀懂與修改。

## 每個檔案負責什麼

| 檔案 | 責任 |
|---|---|
| [main.go](main.go) | 建立 listener／HTTP server，接收終止訊號並關閉 |
| [internal/pricing/http.go](internal/pricing/http.go) | 路由、輸入檢查、JSON 回應、request_id 與日誌 |
| [internal/pricing/quote.go](internal/pricing/quote.go) | 輸入業務規則與整數價格計算，不存取 Kubernetes |
| [Dockerfile](Dockerfile) | 固定 builder、build-time 版本、非 root runtime |
| [main_test.go](main_test.go)、[http_test.go](internal/pricing/http_test.go)、[quote_test.go](internal/pricing/quote_test.go) | 分別對照 server、HTTP 契約與價格規則 |

internal/pricing 是此程式的私有套件。其他階段部署它的映像，不需要把價格規則搬到 Hub。

## 跟一次報價請求

```text
POST /quote
→ NewHandler 的外層：產生 X-Request-ID／記錄耗時
→ /quote 路由：檢查 method、Content-Type、body
→ JSON Decoder：未知欄位與多個文件拒絕
→ Calculate：檢查金額／tier／coupon、整數計算
→ writeJSON 或 writeError
→ http_request 日誌
```

Calculate 的 amount_cents 用 *int64，才能分辨「沒填金額」與「填了 0」。上限限制讓中間乘法在 int64 範圍內。VIP 先以 `amount * 9 / 10` 向下取整，再扣最多 10,000 分的券；不使用浮點金額。

20,000 分的請求得到原價 20,000、會員折扣 2,000、券折扣 10,000、最後 8,000。101 分 VIP 得到 90 分，可用來理解向下取整。

HTTP 層先檢查格式，再檢查業務。非整數 JSON 無法解碼到 int64，屬於 400；合法整數但負數或超過上限，在 Calculate 拒絕為 422。完整端點與錯誤碼見 [M0 契約](../README.md#pricing-api-的-http-契約)。

## 三版為何真的不同

main 的 version 預設 v1，Dockerfile 用 APP_VERSION 與 Go linker 注入 v1／v2-good／v2-broken。HTTP 的 ValidVersion 只接受這三種名稱。

三版價格規則一致。v2-broken 的 /readyz 特意回 503，/healthz 仍回 200；這讓後面可以觀察 rollout 失敗，而不靠程序死亡製造故障。服務關閉時 ready flag 先切為 false，再以最多 5 秒做 HTTP Shutdown。

## 只在本機理解 Go API

下面是可選的輕量練習，不建立 kind。命令仍在專案根目錄的 WSL Bash 執行；結果另見 [本輪紀錄](../../../docs/revalidation-20261011.md)：

```bash
cd /mnt/c/Users/H/Documents/GitHub/win30221/hugofleet
source scripts/common.sh
go run ./stages/m0/pricing-api --listen=127.0.0.1:18082
```

保持此終端。另一終端：

```bash
curl --fail http://127.0.0.1:18082/version
curl --fail http://127.0.0.1:18082/quote \
  -H 'Content-Type: application/json' \
  --data '{"amount_cents":101,"customer_tier":"vip"}'
```

預期 v1 與 final_amount_cents=90。Ctrl+C 結束本機程序。這一課只能理解 Go HTTP，不替代 Pod、Service、探針與 rollout 的 M0 練習。

## 修改時從哪裡開始

價格規則改 quote.go，HTTP 欄位或錯誤契約改 http.go 與型別；依變更更新對應測試與 [M0 教學](../README.md)。版本行為在 HTTP 層與 Docker build 定義；不新增 runtime 開關假裝換版。

改完 Go 不會改已存在的 Docker 映像。要在 kind 使用新程式，重建、載入映像並讓新 Pod 使用它；同 tag 的現有 Pod 不會因 kind load 自動重啟。教學使用分開的版本 tag，便於觀察。
