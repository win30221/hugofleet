# 動畫的用途、機制與核對依據

這份分鏡把每課的產品用途、責任邊界與情境狀態對到實作。播放頁是有意選取的教學快照，不是 Kubernetes 排程／時序模擬器；播放秒數不代表叢集實際等待秒數。

## 各課增加的能力

M0 的播放入口為 [十二步分鏡](m0.html)，資料由 [m0-story.mjs](m0-story.mjs) 定義：用途 → 建置映像 → 建立 Deployment／ReplicaSet／Pod → 就緒探針 → 本機呼叫 → 提交新版 → 新版接手 → 擴容 → 額外壞 Pod → 診斷 → 人工恢復 → 驗證與停止轉送。每步逐段推進並預設停住；更新中的 Pod 保留身分，終止過程另有畫面。清理叢集是獨立的可選收尾示意。M1～M5 使用具名架構元件、Pod 與履歷的逐步分鏡，資料見 [course-story.mjs](course-story.mjs)，播放入口見 [動畫教室](index.html)。

| 章節 | 操作者要解決什麼問題 | 新增的責任 | 不應產生的誤解 |
|---|---|---|---|
| M0 | 確認映像中的業務服務真的可部署、可呼叫，分辨活著與就緒 | Go API、映像、Deployment／RS／Pod、Service、probes | Pod 存在或 Running 不代表新版 Ready；本課沒有 Operator 或 Hub |
| M1 | 用平台領域配置持續維持工作負載，避免底層被改後失去期望 | Application CR、Operator、ownerReference、generation／Conditions | Operator 改 Deployment／Service，不取代原生控制器；維持壞版意圖不等於自動回滾 |
| M2 | 保存每次發布的意圖、快照與結果，處理冪等、衝突、恢復與回滾 | SQLite 交易、Application／Release／Operation、Runner、UI／REST | 202、Applied、Ready 是三個時點；Operation 與服務當下可用性不是同一狀態 |
| M3 | 控制多個 Worker，同時保留最小信任與當地自主性 | outbound mTLS bidi stream、typed command、heartbeat、ledger／UID | 控制通道不是服務 HTTP 反向代理；離線 UNKNOWN 不等於當地服務停止 |
| M4 | 分辨身分、租戶權限、資源額度與封包隔離各自的防線 | OIDC、DB membership、Worker RBAC、Quota／LimitRange、CNI policy | 隱藏按鈕不等於授權；403、FailedCreate、readiness 失敗是三種不同結果 |
| M5 | 把一次操作解釋清楚，並讓安裝交付可重現 | Dashboard、request／operation／trace 關聯、metrics、Helm lifecycle | telemetry 不是持久履歷或狀態權威；Chart 渲染不等於安裝 lifecycle 驗收 |

依據：[規格](../../HUGOFLEET_SPEC.md)、[各課入口](../../stages/README.md)、[Hub 分層](../../stages/m2/hub/README.md)、[Agent](../../stages/m3/agent/README.md)、[管理授權](../../stages/m4/httpapi/README.md)、[可觀測性](../../stages/m5/observability/README.md)、[Helm](../../stages/m5/chart/README.md)。

## 滾動更新：新舊 Pod 同時存在

[M0 workload](../../stages/m0/workload.yaml) 與 [Operator workload builder](../../stages/m1/controller/workload.go) 均固定 `RollingUpdate`、`maxSurge=1`、`maxUnavailable=0`、`progressDeadlineSeconds=60`。以下是兩個健康副本更新壞版的情境：

| 時點 | 舊 ReplicaSet | 新 ReplicaSet | 判讀 |
|---|---|---|---|
| 提交新 template | 兩個 v2-good Ready | 尚未建立 Pod | 舊 Pod 的 image／身分不變 |
| 增加 surge | 兩個 v2-good Ready | 額外一個 v2-broken Running／NotReady | 共三個非終止 Pod；不能把舊 Pod 改成壞版 |
| readiness 持續失敗 | 兩個 v2-good Ready | 同一個新 Pod 仍 NotReady | `maxUnavailable=0`，不繼續縮減這兩個健康舊副本 |
| 期限／失敗證據成立 | 舊版仍可用 | 新版 rollout 失敗／逾時 | 失敗的是 rollout／Operation；壞版 Pod 的 phase 仍為 Running |

`/healthz=200`、`/readyz=503` 是 [pricing-api](../../stages/m0/pricing-api/internal/pricing/http.go) 的刻意行為，並不是 CrashLoopBackOff 或容器死亡。Service 設定不公開 NotReady endpoints；[M0 驗收工具](../../stages/m0/cmd/m0check/main.go) 與 [M1 kind E2E](../../stages/m1/test/e2e/m1_test.go) 有對應的 readiness／restart／endpoint 斷言。

健康更新則是「新增新 Pod → 新 Pod Ready → 逐步縮減舊 RS → 全部目標副本 Ready」。同一個既有 Pod 不會在圖中原地更換 image。動畫省略 Terminating 過程；實際終止中的 Pod 可能使瞬間物件總數超過 `replicas + maxSurge`，因此圖中的三格不是通用的叢集物件數上限。[Kubernetes Deployment 文件](https://kubernetes.io/docs/concepts/workloads/controllers/deployment/) 說明 RollingUpdate 與 terminating replicas 的邊界。

## 更新、縮放、漂移與回滾各不相同

| 操作 | 變更哪一層 | 動畫如何呈現 |
|---|---|---|
| M0 只改 replicas | 原生 Deployment 副本，Pod template 不變 | 同一 RS 增加獨立 Pod；沒有新 template RS |
| M1／Hub 提交新 Application revision | revision marker 寫入 Pod template | 即使 image 相同或只改副本，新 revision 也有新的 rollout |
| 手動修改底層 Deployment spec.replicas | CR=2，Deployment spec 短暫=1 | 保留兩個 Pod 的可能快照，Operator 把 spec 修回 2；不保證一定先看見 Pod 消失 |
| M0 恢復 v1 template | 原生 Deployment template 回復 | 若舊 v1 RS 仍保留，可以重用 RS；新 Pod 仍重新建立 |
| M2／M5 回滾成功 rev 2 | 以來源完整快照建立 rev 4 | 保留失敗 rev 3；恢復 v2-good／兩副本，並核對 rev 4 自己的 rollout |

依據：[教學腳本 M0](../../stages/m0/scripts/lesson.sh)、[M1](../../stages/m1/scripts/lesson.sh)、[revision marker ADR](../adr/0012-current-revision-replicaset-evidence.md)、[完整 Ready 檢查](../../stages/m2/hub/kube/transport.go)、[持久交易與回滾](../../stages/m2/hub/repository/store.go)。

舊 RS 的 FailedCreate／ProgressDeadlineExceeded 不能直接判定新的回滾失敗。必須核對目前 template、owner UID、observedGeneration 與失敗時間歸屬。Hub 的 TIMED_OUT 為終態，不會中止 Kubernetes rollout，也不因晚到 Ready 被改寫成 SUCCEEDED；見 [Runner](../../stages/m2/hub/service/service.go) 與 [ADR 0012](../adr/0012-current-revision-replicaset-evidence.md)。

## 三種失敗的位置

| 失敗 | 哪裡拒絕／失敗 | 新意圖或 Pod |
|---|---|---|
| Viewer 變更、跨租戶操作 | Hub membership／授權邊界 | 不接受新的部署意圖，不新增 Operation／CR revision／Pod |
| Quota FailedCreate | Kubernetes Admission 拒絕 Pod CREATE | 意圖可能已提交、RS 已存在，但新 Pod 物件未建立；既有健康 Pod 保留 |
| 壞版 readiness | Pod 已建立，程序 Running，但探針失敗 | 有額外新 Pod；不可成為一般 Service 的 Ready endpoint |

M4 配額段是獨立的「已有兩副本，surge 空間不足」案例，不是前一步被拒絕的跨租戶請求產生的新副本。NetworkPolicy 是封包路徑限制，另以真實封包檢查；本專案僅對啟用相應 CNI 的 A Worker 作 enforcement 驗收。依據：[管理 API](../../stages/m4/httpapi/README.md)、[安全 E2E](../../stages/m4/test/e2e/security_test.go)、[bootstrap](../../stages/m4/bootstrap/README.md)。

## 控制、資料與觀測的路徑

- M2 控制：瀏覽器 → Hub／SQLite → 受限 kube transport → Application → Operator → Deployment／Service。
- M3 起控制：Agent 主動向 Hub 建 mTLS bidi stream；Hub 在同一串流派送型別命令，Agent 交給當地 CR API，Operator 維持底層資源。重連先核對 ledger 高水位、hash、UID，不盲目重播舊意圖。
- 服務 HTTP：一般 Service dataplane 使用就緒 endpoints；本課 `kubectl port-forward service` 實際選定一個 Pod，不能拿它證明一般 Service 的負載平衡。服務流量不經 Agent 控制通道。
- 觀測：Worker 提供實際狀態；SQLite 保存意圖與業務履歷；metrics／trace 解釋操作。Hub 看不到新 Snapshot 時顯示 UNKNOWN，不能把最後一次 READY 畫成新的觀測。

依據：[Agent engine](../../stages/m3/agent/engine.go)、[通道](../../stages/m3/agenttransport/README.md)、[觀測](../../stages/m5/observability/README.md)、[驗證邊界](../validation.md)。動畫模型與瀏覽器檢查不能代替這些 kind 驗收。
