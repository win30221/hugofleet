// Scenarios derived from workload.yaml, controller/workload.go, transport rollout
// checks, ADR 0012 and the stage guides. IDs name teaching objects, not real UIDs.
const pod = (id, slot, image, revision, ready = true) => ({ id, slot, image, revision, ready, phase: "Running", role: "current" });
const stable = (image, revision, count, slots = [0, 1]) => Array.from({ length: count }, (_, i) => pod(`r${revision}-p${i + 1}`, slots[i], image, revision));
const v1 = () => stable("v1", 1, 1);
const good = () => stable("v2-good", 2, 2);
const failed = () => [...good(), pod("r3-p1", 2, "v2-broken", 3, false)];
const clone = (items) => items.map((item) => ({ ...item }));
const state = (title, why, aPods, more = {}) => ({ title, why, aPods: clone(aPods), ...more });

// A ready old Pod is removed only after its replacement is ready. Unready old
// Pods can be scaled down first. This models the configured surge=1/unavailable=0
// for the demonstrated snapshots; terminating Pods and timing are not simulated.
function rolling(initial, desired, image, revision, finalWhy) {
  let pool = clone(initial).map((p) => ({ ...p, role: "old" }));
  const frames = [state("先保存新意圖，舊 Pod 繼續服務", "修改 Pod template 會啟動新的 rollout；原本的 Pod 不會原地更換 image。", pool)];
  let n = 0;
  const removeOld = () => {
    const unavailable = pool.findIndex((p) => p.role === "old" && !p.ready);
    const i = unavailable >= 0 ? unavailable : pool.findIndex((p) => p.role === "old");
    if (i < 0) throw new Error("No old replica can be retired");
    const gone = pool[i]; pool.splice(i, 1);
    frames.push(state(`縮減舊 ReplicaSet：移除 ${gone.id}`, gone.ready ? "新的可用副本已足夠，才逐步縮減舊健康副本。" : "先縮掉前一次 rollout 的 NotReady 副本，釋放 surge 空間。", pool));
  };
  while (n < desired) {
    while (pool.length >= desired + 1) removeOld();
    const slot = [0, 1, 2].find((s) => !pool.some((p) => p.slot === s));
    if (slot === undefined) throw new Error("Surge slot exhausted");
    n++;
    const next = { ...pod(`r${revision}-p${n}`, slot, image, revision, false), role: "new" };
    pool.push(next);
    frames.push(state(`新 ReplicaSet 建立 ${next.id}`, "額外建立新 Pod；Running 仍須等待 readiness，暫不算可用副本。", pool));
    next.ready = true;
    frames.push(state(`${next.id} 通過 readiness`, "同一個新 Pod 從 NotReady 變 Ready；既有健康 Pod 的版本與身分保持不變。", pool));
    if (pool.filter((p) => p.ready).length > desired && pool.some((p) => p.role === "old")) removeOld();
  }
  while (pool.some((p) => p.role === "old")) removeOld();
  frames.push(state("目標 rollout 完成", finalWhy, pool.map((p) => ({ ...p, role: "current" }))));
  return frames.map((f, i) => ({ ...f, targetImage: image, targetRevision: revision, targetReplicas: desired, rolloutComplete: i === frames.length - 1 }));
}
const failedRollout = () => [
  state("收到壞版意圖，舊版仍健康", "期望兩副本；maxSurge=1 允許額外建立一個新 Pod。舊 Pod 不會變成壞版。", good(), { operation: "RUNNING" }),
  state("新 ReplicaSet 額外建立一個壞版 Pod", "兩個 v2-good 仍為 Ready；第三個 v2-broken 程序 Running，/readyz 持續 503。", failed(), { operation: "RUNNING" }),
  state("readiness 卡住，停止替換舊副本", "maxUnavailable=0；新版不可用時，不繼續移除這兩個舊健康副本。壞版不加入一般 Service 的就緒 endpoints。", failed(), { operation: "RUNNING" }),
  state("等待期限後，判定這次 rollout 失敗", "Kubernetes progressDeadlineSeconds=60；Hub 另有 Operation deadline。動畫縮短等待，Pod 仍是 Running／NotReady，不是 Pod phase=Failed。", failed(), { operation: "FAILED／TIMED_OUT" }),
].map((f, i) => ({ ...f, targetImage: "v2-broken", targetRevision: 3, targetReplicas: 2, rolloutFailed: i === 3 }));

const lessons = [
  ["先證明被部署的服務本身可靠", "沒有平台以前，先分清 image、Pod、Deployment、Service 與 probes。確認價格 API 在真實容器執行，再辨別「程序活著」與「可以接流量」。", "Deployment 維持副本；ReplicaSet 管 Pod；kubelet 探針決定 Ready。單改 replicas 不會產生新 Pod template。", "壞版 rollout 失敗時，為什麼舊版仍可服務？"],
  ["把領域配置交給持續收斂的控制器", "原生 Deployment 只知道自己的期望。Application 定義平台期望，Operator 持續維護 Deployment／Service，並回報目前 generation 的真實狀態。", "Operator 修復 Deployment spec；Kubernetes 控制器維護 RS／Pods。更新 Application revision 會改 Pod template，純漂移修復則不新增 revision。", "CR 要 2、Deployment 被改成 1，誰負責把哪一層改回來？"],
  ["讓部署成為可追蹤、可恢復的業務操作", "單靠 CR 沒有完整的使用者發布履歷。Hub 以 SQLite 交易保存意圖、不可變 Release 與非同步 Operation，處理衝突、冪等和回滾。", "202=已接收；applied=已提交 CR；SUCCEEDED=完整 rollout 已核對。回滾複製成功快照建立新 revision，不改寫失敗歷史。", "desired、applied、last successful 為什麼可能是 3、3、2？"],
  ["把控制送進 Worker，同時保留當地自主性", "多叢集不讓 Hub 持有 Worker 管理員 kubeconfig。各 Agent 主動建立 mTLS bidi stream，接收受限命令；當地 Operator 不依賴這條連線持續修復。", "Agent 建連：Worker → Hub；同一串流上 Hub 送命令、Agent 回報。ledger 記錄高水位與 UID，ACK／Applied 都不代表 Ready。", "Hub 看見 UNKNOWN 時，為什麼當地服務仍可用、也仍能自我修復？"],
  ["讓身分、授權與叢集防線各負其責", "登入解決你是誰；DB membership 決定能對哪個租戶做什麼；Worker 的 RBAC、Quota 與 NetworkPolicy 在各自邊界強制執行。", "跨租戶／Viewer 變更先被 Hub 拒絕，不產生部署意圖；超額 Pod 則在 Kubernetes Admission 被拒絕建立。這不是同一種失敗。", "403 沒有新 Operation，與 FailedCreate 沒有新 Pod，有什麼不同？"],
  ["讓發布結果可以解釋，也能重現交付", "Dashboard 把部署狀態整理給操作者；trace 追查跨元件因果；metrics 概覽目前程序；Helm 包裝可重現的 Worker 安裝與升級。", "SQLite 保存業務歷史，Worker 是實際狀態來源，telemetry 用來解釋。Hub trace viewer 不會自動收齊 Worker spans；Helm 不接管 bootstrap 安全資源。", "看到失敗後，如何沿 operation／request 找到負責的元件，而非只看紅色狀態？"],
];

export function prepareChapters(chapters) {
  for (const [c, chapter] of chapters.entries()) {
    const [purpose, problem, responsibility, question] = lessons[c];
    Object.assign(chapter, { purpose, problem, responsibility, question });
    for (const step of chapter.steps) {
      step.seconds = 14;
      step.sequence = [state(step.title, step.text, stable("v1", 1, step.state.pods || 0))];
    }
  }
  const set = (c, i, sequence, seconds = 18) => {
    const step = chapters[c].steps[i];
    step.sequence = sequence.map((s, j) => ({ ...s, at: j / sequence.length }));
    step.seconds = seconds;
  };
  set(0, 0, [state("Deployment → ReplicaSet → 新 Pod", "Deployment／RS 決定要建立什麼，kubelet 啟動容器並執行探針。", stable("v1", 1, 1).map((p) => ({ ...p, ready: false }))), state("Pod Ready，首次 rollout 完成", "程序啟動與 readiness 都確認後，才核對真實 HTTP 服務。", v1())]);
  set(0, 1, [state("業務請求與控制路徑分開", "port-forward service 會選定一個 Pod；本次 /quote 呼叫不經 Hub，也不是一般 Service 負載平衡的驗收。", v1())]);
  set(0, 2, rolling(v1(), 1, "v2-good", 2, "新 v2-good Ready 後才縮掉舊 v1；原 v1 Pod 不會原地換映像。"), 28);
  set(0, 3, [state("只修改 Deployment replicas", "M0 的 Pod template 未變，沿用同一個 v2-good ReplicaSet 增加副本。", stable("v2-good", 2, 1)), state("ReplicaSet 增建另一個 Pod", "新 Pod 仍須通過探針，並非複製既有 Pod 的記憶體或執行狀態。", [...stable("v2-good", 2, 1), pod("r2-p2", 1, "v2-good", 2, false)]), state("同一 ReplicaSet 的兩副本都 Ready", "這是手動擴副本；沒有自動擴縮或 Application Operator。", good())]);
  set(0, 4, failedRollout(), 28);
  set(0, 5, rolling(failed(), 2, "v1", "restore", "原生 M0 恢復既有 v1 template 時，可以重用保留的 v1 ReplicaSet；實際 Pod 仍是重新建立。"), 30);
  set(1, 0, [state("Application 先描述期望", "Operator 建立帶 ownerReference 的 Deployment／Service；不是直接替代 Kubernetes 的 Pod controller。", v1())]);
  set(1, 1, rolling(v1(), 2, "v2-good", 2, "Application rev 2 寫入 Pod template marker，因此形成新的 rollout；完整 updated／ready／available 都吻合才 Ready。"), 30);
  const drift = state("CR replicas=2，Deployment spec 被改為 1", "畫面保留兩個健康 Pod：Operator 可能快到在 ReplicaSet 縮副本前就修好，不能保證一定看見 Pod 消失。", good(), { crTarget: 2, deploymentTarget: 1, metrics: ["CR spec.replicas|2", "Deployment spec.replicas|1", "這一刻 Pod|可能仍是 2", "變更類型|spec 漂移"] });
  set(1, 2, [drift]);
  set(1, 3, [state("Operator 修正 Deployment spec", "把 Deployment 期望副本改回 2；若原生控制器已縮掉 Pod，再由 ReplicaSet 補回。Application revision 不增加。", good(), { crTarget: 2, deploymentTarget: 2 })]);
  set(1, 4, [...failedRollout(), ...rolling(failed(), 2, "v1", 4, "rev 4 即使映像回到 v1，Pod template 的 revision marker 也不同，必須以這一版 ReplicaSet 判斷成功；舊壞版的錯誤不得污染新操作。")], 46);
  set(1, 5, [state("刪除 Application，觸發 owner GC", "刪 CR 與相依資源消失是不同時點；Deployment → RS → Pods 由 Kubernetes GC 逐層清理。", stable("v1", 4, 2), { crState: "已送出刪除", deploymentState: "等待相依 GC" }), state("相依工作負載清理完成", "Operator 控制器本身仍在執行；GC 不等於停止整個 Worker。", [], { deleted: false, crState: "已刪除", deploymentState: "已刪除" })]);
  set(2, 0, [state("先建立管理能力，還沒有業務 Pod", "Hub／SQLite／Operator 準備好，不代表 pricing-api 已建立。M2 以受限 kubeconfig 直連單叢集。", [])]);
  set(2, 1, [state("SQLite 交易提交，回覆 202", "Application、完整 Release 快照與 Operation 原子保存；此時還沒有 Pod，重送同一冪等鍵不應再建 Release。", [], { operation: "QUEUED" })]);
  set(2, 2, [state("CR 已提交，仍在 rollout", "applied=1 只證明 Worker 已接受 CR，不代表新 Pod 可用。", stable("v1", 1, 1).map((p) => ({ ...p, ready: false })), { metrics: ["desired / applied|1 / 1", "last successful|尚無", "Operation|RUNNING", "Application|DEPLOYING"] }), state("完整觀測吻合，才結束 Operation", "核對目前 generation、目標映像、updated／ready／available 副本與完整配置；再設 last successful=1。", v1())]);
  set(2, 3, rolling(v1(), 2, "v2-good", 2, "rev 2 的不可變快照保存兩副本。與 M0 純縮放不同，此處每個新 Application revision 都有 template marker。"), 30);
  set(2, 4, failedRollout().map((s, i) => ({ ...s, metrics: ["desired / applied|3 / 3", "last successful|2", `Release 3|${i === 3 ? "FAILED" : "PENDING"}`, `Operation|${i === 3 ? "FAILED／TIMED_OUT" : "RUNNING"}`] })), 28);
  set(2, 5, rolling(failed(), 1, "v1", 4, "以 rev 1 的完整快照建立 rev 4，包含 replicas=1。失敗 rev 3 仍保留；回滾不是重新命名舊 Pod，也不是把舊 Release 改成 ACTIVE。"), 36);
  set(3, 0, [state("Worker 主動建立控制通道", "兩個 Agent 各自向 Hub 撥號並驗證 mTLS；Hub 只在已驗證的串流上派送型別命令，沒有 Worker kubeconfig。", good(), { bPods: stable("v1", 1, 1) })]);
  set(3, 1, [state("控制面失聯，當地工作負載未停止", "A Agent 停止後，Hub 只能顯示最後觀測與 UNKNOWN；這是觀測過期，不能推論 Pod 已壞或停止。", good(), { bPods: stable("v1", 1, 1), connected: false })]);
  set(3, 2, rolling(v1(), 2, "v2-good", 2, "B 的發布與 A 的連線問題獨立；A 當地仍保留原工作負載。").map((s) => ({ ...s, bPods: s.aPods, aPods: good(), connected: false })), 30);
  set(3, 3, [{ ...drift, connected: false, bPods: good() }]);
  set(3, 4, [state("A 的當地 Operator 修正 spec", "不經 Hub、不新增 deployment revision；若 Pod 已被縮掉才由 RS 補齊。Hub 仍 UNKNOWN，當地可確認兩副本 Ready。", good(), { bPods: good(), connected: false, deploymentTarget: 2 })]);
  set(3, 5, [state("重新驗證 mTLS 並核對 ledger", "先讀回高水位、spec hash 與 CR UID；重連不盲目重播過期命令，也不產生新 Release。", good(), { bPods: good(), connected: true }), state("新 Snapshot 更新 Hub 觀測", "Agent Pod Ready 不等於通道 CONNECTED；Applied ACK 不等於 rollout 成功，最後以真實 Snapshot 核對。", good(), { bPods: good(), connected: true })]);
  set(4, 0, [state("OIDC 驗證身分，尚未授權租戶", "Code／PKCE／session 處理登入；Hub 角色仍來自 DB，不從 OIDC groups 自動升權。", [])]);
  set(4, 1, [state("核對 Worker profile，才啟用租戶", "admin 啟用前先核對已 bootstrap 的 namespace、receipt、Quota／LimitRange；不是按下按鈕就任意建立命名空間或擴權。", [])]);
  set(4, 2, [state("授權成功後才接受部署意圖", "Developer／team-a 的變更經 CSRF、membership、policy 檢查，再交給受限 Agent／Operator。", []), state("Worker 正常建立 Pod 並回報 Ready", "應用可用性來自 Worker 真實觀測；成功登入本身不能證明應用已啟動。", v1())]);
  set(4, 3, [state("Viewer 變更在 Hub 被擋下", "唯讀資料可看；自行送出變更一樣拒絕。沒有新 Operation、沒有新 CR revision、更沒有新 Pod。", v1(), { hubRejected: true })]);
  set(4, 4, [state("跨租戶在查詢邊界拒絕", "已登入 team-a 的 Developer 也不能讀取 team-b；回 403 並保留 request_id，不暴露外租戶資料。", v1(), { hubRejected: true })]);
  set(4, 5, [state("意圖通過 Hub，但新 Pod 建立遭 Admission 拒絕", "以 rollout 的暫時 surge 超額為例：既有兩個健康 Pod 保留，新 Pod CREATE 被 quota 拒絕，因此沒有第三個壞 Pod。", good(), { quotaDenied: true, gate: true, focus: "a", metrics: ["既有健康 Pod|2", "新 Pod 物件|未建立", "Worker 原因|FailedCreate／quota", "失敗位置|Kubernetes Admission"] }), state("目前 ReplicaSet 的 failure 回報到 Hub", "只採用目前 revision 的 FailedCreate，不能讓舊 ReplicaSet 的 quota 錯誤誤判後續恢復版本。NetworkPolicy 封包測試是另一條驗收路徑。", good(), { quotaDenied: true, gate: true, focus: "a", metrics: ["既有健康 Pod|2", "新 Pod 物件|未建立", "失敗證據|目前 ReplicaSet", "NetworkPolicy|另驗證 A 的封包"] })], 24);
  set(5, 0, [state("先安裝平台元件，再啟用租戶", "Helm 管理 Agent／Operator 及其受限 RBAC；TLS、Quota、receipt、ledger 等安全材料由 bootstrap 管理。", [])]);
  set(5, 1, rolling(v1(), 2, "v2-good", 2, "Dashboard 的 Ready 必須對應這次 rollout；不能只看某個舊 Pod 還能回應。"), 30);
  set(5, 2, failedRollout().map((s, i) => ({ ...s, metrics: ["desired / applied|3 / 3", "last successful|2", `Release 3|${i === 3 ? "FAILED" : "PENDING"}`, "追查依據|request／operation"] })), 28);
  set(5, 3, rolling(failed(), 1, "v1", 4, "來源快照一副本，rev 4 要等自己的新 ReplicaSet 完整 Ready；保留 rev 3 失敗與同一次 request／operation 的關聯。"), 36);
  set(5, 4, [state("用關聯追查，分清三種資料來源", "SQLite=持久操作履歷；Worker=實際狀態；telemetry=解釋因果。/metrics 重啟歸零；/traces 僅當前 Hub spans。", stable("v1", 4, 1)), state("收集跨元件日誌，組出完整 trace", "以同一 operation 的 context 關聯 Hub command → Agent Execute → CR annotations → Operator reconcile；不是把整條 gRPC stream 當一筆部署。", stable("v1", 4, 1))]);
  set(5, 5, [state("Chart 渲染與真實 lifecycle 分開核對", "render 只能證明產物；install／upgrade／uninstall／reinstall 須核對 ownership、ledger 與 bootstrap 材料保留，不能把卸載當作清資料。", stable("v1", 4, 1))]);
  // Teach causes first; commands remain an optional way to reproduce the scene.
  chapters[1].steps[2].text = "Application 要求兩副本，但人為把 Deployment spec.replicas 改成 1。這是兩層期望不一致；Pod 不一定來得及減少。";
  chapters[1].steps[2].expected = "核對 CR=2、Deployment spec 短暫=1，再由 Operator 改回 2；不要把「一定有 Pod 消失」當必要證據。";
  chapters[3].steps[3].text = "A Agent 停止時，修改的是當地 Deployment 的期望副本。當地 Operator 仍能從 Application 修正；Pod 可能始終維持兩個。";
  chapters[4].steps[5].text = "角色授權通過也可能部署失敗：Quota 會在 Kubernetes Admission 拒絕新 Pod CREATE。此情境沒有新 Pod 物件，與 readiness 失敗不同。";
  chapters[4].steps[5].title = "獨立案例：surge 配額不足";
  chapters[4].steps[5].text += " 本段切換為已有兩健康副本的獨立案例，不是前一步被拒絕的跨租戶請求造成變更。";
  for (const c of [0, 1]) for (const step of chapters[c].steps) for (const f of step.sequence) {
    if (f.targetReplicas !== undefined) f.metrics = [`目標映像|${f.targetImage}`, `Pod 總數 / Ready|${f.aPods.length} / ${f.aPods.filter(p => p.ready).length}`, `Deployment 期望|${f.targetReplicas}`, `rollout|${f.rolloutComplete ? "完整就緒" : f.rolloutFailed ? "進度逾時／失敗" : "進行中；舊版仍保留"}`];
  }
  for (const [c, i, revision, previous] of [[2,3,2,1],[2,5,4,2],[5,1,2,1],[5,3,4,2]]) {
    const sequence = chapters[c].steps[i].sequence;
    sequence.forEach((f, j) => { f.metrics = [`desired / applied|${revision} / ${j === 0 ? revision - 1 : revision}`, `last successful|${f.rolloutComplete ? revision : previous}`, `Operation|${f.rolloutComplete ? "SUCCEEDED" : j === 0 ? "QUEUED" : "RUNNING"}`, `Pod 總數 / Ready|${f.aPods.length} / ${f.aPods.filter(p => p.ready).length}`]; });
  }
  for (const step of chapters[0].steps.slice(3)) for (const f of step.sequence) f.targetReplicas ??= 2;
  return chapters;
}
