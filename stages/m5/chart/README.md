# Worker Helm Chart

## 第一次使用與進階操作

一般學習只需照 [M5 教學](../README.md)執行 `make m5-up`，入口會先 bootstrap 安全材料、產生本輪 values，再用 Helm 安裝。不要直接複製 upgrade 命令到沒有準備過的叢集。

| 你要做什麼 | 閱讀位置 | 預期結果 |
|---|---|---|
| 只改 Chart、先看渲染 | 下方不連叢集檢查 | 真正 Helm CLI 渲染與負向 schema 測試通過 |
| 安裝本課 Worker | M5 up | 實際 Operator／Agent rollout 與 receipt 核對 |
| 調整既有 Worker | 真實安裝與更新 | 使用本輪 values，保留安全資源 ownership |

所有命令在專案根目錄的 WSL／Linux Bash 執行。Chart 檢查不需先啟動 kind；安裝與升級則需要相符的既有 state。

## 不連叢集的實際渲染檢查

```bash
make helm-tools m5-chart-test
```

Go 驗收實際執行固定 Helm CLI，核對 17 個 typed objects、Pod／RBAC／CRD、預設與自訂 timing flags，並拒絕無效 timing 型別／邊界／min > max 及其他負向 schema；不是 Mock，也不等於安裝 E2E。來源在 [test/chart](../test/chart/)。

預期命令返回 0；失敗時查看 Helm schema 或 typed object 的錯誤，先修正再繼續。此檢查不會建立叢集，完成後無需停止服務。

## 真實安裝與更新

正式 control 先完成 owned bootstrap／TLS／ledger，再動態產生 .local 中的 values，執行：

```bash
source stages/m5/env.sh a
helm upgrade --install hugofleet-worker stages/m5/chart \
  --namespace hf-system --kubeconfig "$KUBECONFIG" --kube-context "kind-$CLUSTER" \
  --values "$HF_M5_STATE/$CLUSTER-values.json" --wait=watcher --timeout=5m
```

第一次安裝前須由 M5 正式入口產生上述材料與 values；不要拿文件中的固定 IP 當作真實 endpoint。control 最後核對完整 Operator／Agent rollout 及精確 Helm release annotations，才寫入可供 Hub 核對的 receipt。

以下是獨立的升級練習，應在仍運作的本輪環境執行。假設目前 rollout 仍為初始值，可指定 rollout 2，讓 Pod 實際重新啟動；若已是 2，重送相同值不保證再觸發重啟：

```bash
helm upgrade hugofleet-worker stages/m5/chart \
  --namespace hf-system --kubeconfig "$KUBECONFIG" --kube-context "kind-$CLUSTER" \
  --values "$HF_M5_STATE/$CLUSTER-values.json" --set rollout=2 --wait=watcher --timeout=5m
```

若需調整 Agent timing，以下以既有 release values 為基礎，保留租戶與 rollout 設定。心跳改成 7 秒前先以 `HF_M5_HEARTBEAT_TIMEOUT=21s` 啟動／重建 Hub，然後更新每個 Worker：

```bash
helm upgrade hugofleet-worker stages/m5/chart \
  --namespace hf-system --kubeconfig "$KUBECONFIG" --kube-context "kind-$CLUSTER" \
  --reset-then-reuse-values --set heartbeatSeconds=7,reconnectMinSeconds=2,reconnectMaxSeconds=40 \
  --wait=watcher --timeout=5m
```

`--reset-then-reuse-values` 先載入目前 Chart 預設、再套用原 release values 和明確 overrides；原先沒有 timing keys 的 release 也可取得新預設，既有自訂值保留。`hub-recreate` 使用同一策略只更新 endpoint 並讀回 live values，保留 timing、rollout 與 Worker 的配額／receipt／ledger。不要為了單獨調整 endpoint 重新執行首次 `workers` provision。

## 結束與下次操作

完成後若要繼續同輪練習，保留原 Worker、state 與 Helm release；需要結束整課時，依 [M5 停止流程](../README.md#停止與下次繼續)停止轉送並執行 `make m5-down`。安裝失敗先查 [M5 排查](../README.md#操作遇到問題時)，不要把卸載當成通用修復步驟。

卸載會移除 Helm 所有的 Operator／Agent／RBAC，原應用的 Pods／CR 與 bootstrap 安全材料保留；當地 Operator 在卸載期間停止維護。重新安裝仍使用原 ledger，不能重設高水位或 tombstone。CRD 的首次安裝與後續升級遵循 Helm CRD lifecycle，不能用 uninstall 當作 CRD migration。

真實 install／upgrade／uninstall／reinstall、ledger／receipt UID 及高水位保留的驗收範圍見 [M5 驗收](../../../docs/m5-validation.md)。render 與 cluster lifecycle 的證據分開記錄。


## 架構、設定與限制

固定 Helm 4.3.0／Kubernetes 1.37，release 必須為 hf-system 中的 hugofleet-worker。一般從 [M5 正式入口](../README.md) 使用；[ADR 0011](../../../docs/adr/0011-observability-and-delivery.md) 說明資料與 ownership 邊界。

Chart 安裝 CRD、每租戶的受限 Operator／RBAC、hf-system outbound Agent／ledger RBAC，引用既有 TLS Secret。Namespace、Quota、LimitRange、NetworkPolicy、receipt、TLS Secret 與 command ledger 都由本地 Go bootstrap 管理；Chart 不包含這些物件，不使用 take-ownership。

Operator 對 ReplicaSet 只有同 namespace 的 get/list/watch，用於辨別目前 revision 的真實失敗；沒有 ReplicaSet 寫入或跨租戶讀取。新 Application revision 會更新受管理的 Pod template marker、觸發獨立 rollout；相同 revision 重送維持冪等。見 [ADR 0012](../../../docs/adr/0012-current-revision-replicaset-evidence.md)。

schema 拒絕任意 image／command／Secret 值、未知 keys、公開 Hub endpoint、重複租戶與超過 20 個租戶。TLS 只讀投影為精確 cluster 檔名；Pods non-root／read-only root filesystem／drop ALL，沒有 hostPath、exec 權限或 ClusterRoleBinding。

Agent timing 使用整數秒：`heartbeatSeconds` 預設 5，範圍 1～30；`reconnectMinSeconds`／`reconnectMaxSeconds` 預設 1／30，範圍各為 1～300。schema 檢查型別與邊界，template 額外拒絕 min > max。它們實際傳入 Agent 的 `--heartbeat`／`--reconnect-min`／`--reconnect-max`；重連使用 jitter 與指數退避，設定是退避的起始／上限，並非每次等待的固定秒數。Hub 的 `HF_M5_HEARTBEAT_TIMEOUT`（預設 `15s`，範圍 `3s`～`5m`）應大於 Agent 的心跳間隔，兩端須一起調整。

## 讀完後去哪裡

- 返回 [M5 逐步操作](../README.md#逐步操作)，對照本輪結果。
- 需要停止、換課或再次練習，前往 [M5 完成後的選擇](../README.md#完成這一課後去哪裡)。
- 想繼續理解其他元件，回到 [M5 原理與進階閱讀](../README.md#原理與進階閱讀)。
