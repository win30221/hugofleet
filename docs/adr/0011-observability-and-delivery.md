# ADR 0011：可觀測部署與安全交付

狀態：已採納。功能驗收見 [M5 驗收](../m5-validation.md)，對應程式提交的 Hosted CI 證據見 [驗證總覽](../validation.md)。

## 決策

M5 延用 M4 OIDC／session／授權與 M2 Operation 狀態機。以 SQLite schema 5 保存接受部署時的 trace context 及最後真實 Worker 觀測，與不可變 Release／Operation 履歷分開。離線保留快照和時間，不將延遲 Ready 改寫為原操作成功。

OpenTelemetry Go SDK 固定 1.44.0，Prometheus client_golang 固定 1.24.0。追蹤在每個命令 envelope 注入／提取 W3C TraceContext，單一 stream 不作所有操作的共同 trace。輸出只含固定 span 名稱、IDs、狀態等允許欄位，不輸出 HTTP query／headers／body、Baggage、私鑰或任意錯誤 payload。SDK 於 M5 入口啟用；不要求常駐 Collector／Grafana。Metrics labels 僅使用固定 operation 類型／終態與已登記 cluster，不使用 app／user／request IDs。

Helm 固定 4.3.0，官方相容範圍含 Kubernetes 1.37。固定 release 為 hf-system 中的 hugofleet-worker，Chart 管理 Operator／Agent 的 Deployments、ServiceAccounts、namespaced Roles／RoleBindings 與首次安裝 CRD；不包含 Namespace／Quota／LimitRange／NetworkPolicy／receipt／ledger／Secret。後者由本地受控 bootstrap 準備，驗證 Helm components ownership／完整 rollout 後才完成 receipt，因此 uninstall 不會移除隔離、credentials 或高水位資料。Chart 使用 Helm managed-by 與精確 release annotations；安全材料維持 hugofleet-bootstrap ownership。Secrets 只有既有名稱引用，不放 credentials 於 values，也不使用 take-ownership 接管外部物件。驗收涵蓋真實 install／upgrade／uninstall／reinstall 與 ledger／receipt 保留，範圍見 [M5 驗收](../m5-validation.md)；render 僅驗證渲染結果。

## 官方依據

- [OpenTelemetry Go instrumentation](https://opentelemetry.io/docs/languages/go/instrumentation/) 與 [固定 SDK release](https://github.com/open-telemetry/opentelemetry-go/releases/tag/v1.44.0)。
- [Prometheus client_golang](https://github.com/prometheus/client_golang/releases/tag/v1.24.0)。
- [Helm version support](https://helm.sh/docs/topics/version_skew/) 與 [固定 Helm release](https://github.com/helm/helm/releases/tag/v4.3.0)。

## 限制

單 Hub／SQLite、開發 CA／loopback Dex、拋棄式 kind etcd 仍不是生產 HA。只有 enforcing CNI 的 Worker 能宣稱封包隔離。Prometheus 指標為程序生命期累積值，重啟會重設，持久履歷以 SQLite 為準。Hosted CI 成功須另有實際執行證據。

## 閱讀後返回

本頁用來查設計原因，不是安裝步驟。返回 [M5 原理與進階閱讀](../../stages/m5/README.md#原理與進階閱讀)，或從[選課與回訪](../../stages/README.md#回來繼續哪一課)接續使用此共用設計的其他階段。其他設計問題見 [ADR 導覽](../README.md#設計問題與-adr-對照)。
