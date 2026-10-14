# M2 API 與部署履歷

完整 HTTP 契約見 [OpenAPI](openapi.yaml)。M2 固定本機開發管理員；單一 demo Tenant／local-kind Cluster。POST /tenants 保留但回 403，M4 才開放 provisioning。

先由 [M2 教學](../stages/m2/README.md) 的 make m2-up 啟動，或使用 [舊單叢集入口](../stages/m2/README.md#舊單叢集開發入口)。命令在專案根目錄的 WSL Bash 執行，使用新的 M2 DB，且尚未由 UI 建立同名應用；UI／REST 建立選一條即可。

本文件 REST 主線是 v1 rev 1 → broken rev 2 → 回滾 rev 3，與 M2 教學先加 v2-good 的四版練習不同。已做其他更新時，改用目前 desired_revision／成功來源，不照抄舊基準。M3 使用相同欄位規則，但需每次帶 Token、初次指定 A／B；見 [M3 REST 範例](../stages/m3/README.md#rest)。

以 CLI 建立初版（與 UI 表單相同業務服務）：

```bash
curl --fail-with-body http://127.0.0.1:18090/api/v1/tenants/demo/apps \
  -H 'Content-Type: application/json' -H 'Idempotency-Key: example-create-1' \
  --data '{"name":"pricing-api","cluster_id":"local-kind","expected_desired_revision":0,"image":"pricing-api:v1"}'
```

202 回傳 application_id／operation_id／revision／state。以回傳的 ID（不要原樣保留下面的範例變數）查詢：

```bash
app_id='<回傳的 application_id>'
operation_id='<回傳的 operation_id>'
curl --fail-with-body "http://127.0.0.1:18090/api/v1/operations/$operation_id"
curl --fail-with-body "http://127.0.0.1:18090/api/v1/tenants/demo/apps/$app_id"
```

成功後，送出壞版。每次 release 是完整配置搭配 defaults，不是 partial patch；未提供欄位會使用啟動時的 policy 和 probe defaults。教學入口使用 stages/m2/policy.json；舊開發入口使用 stages/m2/policy.json。

```bash
curl --fail-with-body "http://127.0.0.1:18090/api/v1/tenants/demo/apps/$app_id/releases" \
  -H 'Content-Type: application/json' -H 'Idempotency-Key: example-broken-2' \
  --data '{"expected_desired_revision":1,"image":"pricing-api:v2-broken"}'
```

查該次 Operation，等 FAILED／TIMED_OUT 終態、讀錯誤，再用已成功 revision 1 快照回滾，得到新 revision 3：

```bash
curl --fail-with-body "http://127.0.0.1:18090/api/v1/tenants/demo/apps/$app_id/rollback" \
  -H 'Content-Type: application/json' -H 'Idempotency-Key: example-rollback-3' \
  --data '{"expected_desired_revision":2,"target_revision":1}'
```

desired_revision 是最後接受的意圖，applied_revision 是 CR 寫入已確認，last_successful_revision 是真實 rollout 已成功。rev1 成功、rev2 壞版失敗時是 2／2／1；回滾 rev3 成功後是 3／3／3。FAILED 也消耗 revision。

同 tenant 的 Idempotency-Key＋同路徑／動作／解碼後 payload 回同一 operation；JSON 空白與欄位順序不影響 hash。相同 key 改 payload、舊 expected_desired_revision、同 App 尚有活動操作皆 409。重播先查已儲存意圖，再執行目前政策；不會因重新啟動的資源 defaults 改變而重建 Release。不同 key 的相同 payload 視為新請求，仍受 CAS 與單一活動操作限制。預設 deadline 180 秒，啟動旗標可調整；重啟保留原始 deadline／command_id。

Operation 的 TIMED_OUT／FAILED 不被晚到 Ready 改成 SUCCEEDED；Release FAILED 也不改 ACTIVE。現在觀測的 Application 仍可能 READY，事件顯示 LATE_READY，但 last_successful_revision 保留原值。要正式確認該版本，提交新的 release／rollback，產生新的 revision。

即使 applied_revision 尚未確認，背景觀測仍會讀取真實 CR。只有 Hub Application ID label、已綁定 UID（若有）及完整 revision 快照相符，才恢復 Kubernetes UID／applied_revision；超過原 deadline 不重新 Apply，也不重寫終態。Deployment／Service 與完整 rollout 仍需獨立驗證才能顯示 READY。

Application 的 `observation_error_code`／`observation_error_message` 是目前觀測的故障原因，例如 APPLICATION_MISSING、SPEC_DRIFT 或 OWNERSHIP_CONFLICT；成功觀測後清除。這與 Operation 的歷史錯誤欄位不同。OBSERVED_DEGRADED／OBSERVED_RECOVERED 只在原因改變或清除時新增，同一故障輪詢不重複新增；故障再次發生可再次記錄。UID_BOUND 是刪除前恢復 UID 的既有事件。連線無法確認時沿用 UNKNOWN 與最後一次觀測，不能將舊原因當成最新叢集結果。

刪除同樣非同步，保留履歷：

```bash
curl --fail-with-body -X DELETE "http://127.0.0.1:18090/api/v1/tenants/demo/apps/$app_id" \
  -H 'Content-Type: application/json' -H 'Idempotency-Key: example-delete' \
  --data '{"expected_desired_revision":3}'
```

應用清單 limit 預設 50、最多 100，after 使用回傳的 next_after；按 Application ID 排序。詳細頁各歷史列表取最近 limit 筆；UI 顯示最近 50 筆。SQLite 保留更舊歷史，本階段沒有歷史搜尋或匯出功能。時間欄位均 Unix milliseconds，0 表示尚無觀測／成功／結束時間。只有 App list 使用 cursor，詳細頁歷史不提供 cursor。

資源預檢為每應用限額與 replicas+1 的 requests，不宣稱總量排程或跨叢集資源池。最終 quota 拒絕由 Kubernetes Admission 執行；rollout 的 FailedCreate／ProgressDeadlineExceeded 會保存在 Operation 與事件中。

M2 模式不使用 Agent／mTLS，也不提供 OIDC 或多租戶隔離；M3 的主動連線 Agent 與 Token 操作見 [M3 教學](../stages/m3/README.md)，已實作的 OIDC／租戶安全功能另見 [M4 教學](../stages/m4/README.md)。本文件的 M2 管理入口只在本機 loopback 使用；專案不提供 HA 或任意 YAML／shell／exec 管理功能。
