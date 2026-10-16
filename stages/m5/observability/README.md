# M5 可觀測性

## 先看頁面，再收集跨元件 trace

先完成 [M5 教學](../README.md)的一次部署或回滾，讓程序產生實際 spans。以 Admin 登入後，`/metrics` 看程序指標，`/traces` 看 Hub 程序的近期 spans；Owner／Viewer 無全域 telemetry 權限。

需要追查同一次操作跨 Hub／Agent／Operator 的父子關係時，依下面「收集日誌並匯出」操作。所有命令在專案根目錄執行，使用本輪 state；輸出是本機 HTML，不會再發起部署。

## 收集日誌並匯出

先從本次 owned 元件收集 logs，再用同一組輸入產生 HTML。以下適用教學預設 Hub 名稱及 A Worker；自訂 state 先 export HF_M5_STATE，再 source，Hub 名稱需與該次 ownership 記錄一致：

```bash
source stages/m5/env.sh a
mkdir -p artifacts/m5-observability
docker logs hugofleet-hub-m5 > artifacts/m5-observability/hub.log 2>&1
kubectl --kubeconfig "$KUBECONFIG" --context "kind-$CLUSTER" \
  -n hf-system logs deployment/hugofleet-agent --tail=20000 > artifacts/m5-observability/agent.log
kubectl --kubeconfig "$KUBECONFIG" --context "kind-$CLUSTER" \
  -n hf-team-a logs deployment/hugofleet-operator --tail=20000 > artifacts/m5-observability/operator.log
go run ./stages/m5/cmd/traceview \
  --input artifacts/m5-observability/hub.log \
  --input artifacts/m5-observability/agent.log \
  --input artifacts/m5-observability/operator.log \
  --output artifacts/m5-observability/trace-view.html
```

目的 HTML 必須不存在；另取新檔名即可保留上一輪證據。輸入最多八份／每份 128 MiB／每行 1 MiB、合計最多 20,000 筆 records，沒有即時訂閱或任意管理操作。Hub 程序的 3,000 筆 ring 會淘汰舊 spans；舊 trace 篩選無結果不代表該操作沒有 trace，需使用事前收集的 logs。實際 browser rollback 的跨元件 parent 比對與離線匯出見 [驗收紀錄](../../../docs/m5-validation.md) 及 [公開 trace 範例](../../../docs/media/m5-rollback-trace.html)。

## 確認結果、結束與再次使用

匯出命令應返回 0 並產生指定 HTML。以瀏覽器開啟後，找出剛才操作的 operation／request 關聯，確認包含預期元件的 spans；只有檔案存在不代表跨元件資料已收齊。

若目的檔已存在，改用新檔名；找不到操作時，核對本輪 state、Worker、租戶與日誌是否包含該次操作。只收集 A 的日誌無法證明 B 的 trace；必要時切到 B 收集並加入輸入。

此流程沒有常駐 viewer，匯出後可關閉頁面，日誌與 HTML 留在 `artifacts/m5-observability`。下次若要保留原始證據，先替新的輸入日誌與輸出 HTML 選不同路徑，避免覆寫。Hub／Workers 是否結束，依 [M5 停止流程](../README.md#停止與下次繼續)選擇。

## 指標與 Viewer

| 指標 | 語意 |
|---|---|
| hugofleet_operations_total | 此程序成功提交的終態轉換，type／state labels |
| hugofleet_operation_duration_seconds | 接受到終態提交的時間；同樣 type／state |
| hugofleet_agent_connections | 登記叢集的實際連線，0 或 1 |
| hugofleet_reconcile_errors_total | Go reconcile 回傳 error 的次數；業務 FAILED 另計 operations |

Metrics 不用 app、user、request IDs 當 labels。終態 hook 在 SQLite Commit 後才觸發；失敗寫入與重複 Finish 不計數。這些指標重啟會重設，不能當成持久稽核資料。

Admin 可從 Dashboard 開啟 /metrics、/traces；每次仍核對目前 DB session／角色，Viewer／Owner 不能讀全域 telemetry。Hub trace viewer 顯示最多 100 筆、程序保留 3,000 筆，Agent／Operator spans 從各自 JSON 日誌按需收集，不要求常駐 Collector。

Agent／Operator metrics 只監聽 Pod 內 127.0.0.1:8082，需要由主機管理員對這個 owned namespace／Pod 做明確診斷 port-forward；不建立公開 Service。

[離線 viewer](../traceview/view.go) 只讀已收集的 JSON logs，過濾到安全 span records 後產生 HTML；沒有 Hub 檔案讀取或 Worker kubeconfig 功能。使用 [Go CLI](../cmd/traceview/main.go)，依下方步驟先收集、再匯出，目的檔必須不存在。

## 原理與程式

這個共用套件不 import Hub／Agent／Kubernetes 業務套件，避免循環依賴。M5 入口啟用 OTel SDK 1.44.0 與 Prometheus client_golang 1.24.0；早期入口預設不啟用 runtime telemetry。設計見 [ADR 0011](../../../docs/adr/0011-observability-and-delivery.md)。

HTTP 請求建立 span，接受 operation 時將 TraceContext 寫進 SQLite；Runner 從持久 context 建立 operation span，**每個** Apply／Delete／Snapshot／Profile envelope 再建立 command span。Agent 各自提取並建立 child，不能把整條長 stream 當成所有操作的共同 trace。channel_id 僅識別已驗證控制通道，不是平台 session credential。

Typed CR annotations 將最近接受的意圖關聯傳給 Operator；後續自主 reconcile 的 request／operation IDs 指向該意圖，不代表有人從 Hub 發起每一次漂移。完整 generation／Conditions／transition time 與最後觀測另存 SQLite，離線不重寫時間或失敗 operation history。

## 秘密與容量邊界

Exporter 只序列化固定 span names 與允許的 correlation IDs／status；不序列化 events、Resource attributes、HTTP headers／query／body、baggage、JWT、私鑰或原始錯誤 payload。UUID／slug 格式驗證限制輸出，queue 512／batch 64／export timeout 2 秒。HTTP 輸入只接收有效 traceparent，外部 tracestate／baggage 不進入持久 context。

[telemetry_test.go](telemetry_test.go) 驗證 parent／不同 trace IDs／秘密排除／有限 labels，[SQLite 測試](../../m2/hub/repository/observability_test.go) 驗證真實重啟與冪等。實際 mTLS／kind 的 trace 關聯仍以 [M5 紀錄](../../../docs/m5-validation.md) 為準。

## 讀完後去哪裡

- 返回 [M5 逐步操作](../README.md#逐步操作)，對照本輪結果。
- 需要停止、換課或再次練習，前往 [M5 完成後的選擇](../README.md#完成這一課後去哪裡)。
- 想繼續理解其他元件，回到 [M5 原理與進階閱讀](../README.md#原理與進階閱讀)。
