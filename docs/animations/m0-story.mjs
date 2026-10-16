// A representative, resource-sufficient M0 execution. IDs are teaching names.
const pod = (id, image, slot, status = 'ready') => ({ id, image, slot, status });
const A = pod('A', 'v1', 0), B = pod('B', 'v2-good', 1), C = pod('C', 'v2-good', 0), D = pod('D', 'v2-broken', 2, 'unready'), E = pod('E', 'v1', 2), F = pod('F', 'v1', 1);
const change = (p, status) => ({ ...p, status });
let state = { mode: 'quote', pods: [], image: 'v1', replicas: 1, deployed: false, service: false, forward: '', focus: '', signal: '', result: '', endpoints: [], quote: 0, build: 0, observed: '', probe: '' };
function event(title, why, patch = {}, seconds = 8) {
  state = { ...state, signal: '', result: '', focus: '', probe: '', ...patch };
  state.endpoints = state.pods.filter(p => p.status === 'ready').map(p => p.id);
  return { title, why, seconds, ...structuredClone(state) };
}
function step(title, question, command, events) { return { title, question, command, events }; }
export const story = [
  step('我們要部署什麼？', '先認識要讓它持續運作的服務：價格 API。', '# 本步先看用途；稍後才呼叫叢集中的 API。', [
    event('送出一筆價格試算', '顧客買了 200 元商品，是 VIP，並使用 SAVE100 折價券。', { quote: 0 }),
    event('API 依序套用價格規則', 'VIP 先打九折，200 元變成 180 元；再扣除 100 元折價券。', { quote: 1 }),
    event('回傳 80 元', 'HTTP 回應中的 final_amount_cents 是 8000 分。M0 的任務是把這個服務部署到 Kubernetes。', { quote: 2 }),
  ]),
  step('把程式裝進映像', '映像是程式的包裝；Pod 才是正在執行的那一份。', 'make m0-up\n# 此命令包含建置、建立 kind、載入映像與部署；動畫拆開解釋。', [
    event('從程式與 Dockerfile 開始', '同一個價格 API 提供三個教學版本。此時尚未建立業務 Pod。', { mode: 'build', build: 0 }),
    event('建置三個版本的映像', 'v1 與 v2-good 可通過就緒檢查；v2-broken 刻意讓 /readyz 回傳 503。', { build: 1 }),
    event('載入 kind 節點', '主機上建好的映像要載入 kind 節點，節點才能用它們啟動容器。', { build: 2 }),
  ]),
  step('要求一份 v1', '你提交期望設定，由 Kubernetes 的控制器建立工作負載。', 'make m0-up\n# 延續同一次啟動，不需再執行一次。', [
    event('提交 Deployment 與 Service', '設定送到 API Server：映像 v1、副本 1。Service 同時建立，但暫時沒有可用端點。', { mode: 'cluster', deployed: true, service: true, focus: 'deployment', signal: 'submit' }),
    event('Deployment 控制器建立 ReplicaSet', 'ReplicaSet 記住這組 Pod 模板，並負責維持需要的副本數。', { focus: 'rs', observed: 'RS-v1 已建立' }),
    event('ReplicaSet 建立 Pod A', '排程器把 Pod 指派給節點；節點上的 kubelet 用 v1 映像啟動容器。', { pods: [change(A, 'pending')], focus: 'pods', signal: 'create' }),
    event('程式開始執行', 'Pod A 已 Running，但我們還沒有看到 readiness 通過，先不把它列為可用端點。', { pods: [change(A, 'starting')], focus: 'pods' }),
  ]),
  step('確認 Pod 可以接流量', 'Running 和 Ready 是兩個不同的判斷。', 'make m0-status', [
    event('kubelet 發出就緒探測', '節點上的 kubelet 呼叫 Pod A 的 /readyz，不是由 Service 主動探測。', { signal: 'probe', probe: 'A · /readyz → 等待回應', focus: 'kubelet' }),
    event('/readyz 回傳 200', '通過就緒檢查後，Pod A 才成為 Ready。', { pods: [A], signal: 'probe', probe: 'A · /readyz → HTTP 200', focus: 'pods' }),
    event('Service 有了可用端點 A', '控制平面依 Pod 的選擇標籤與 Ready 狀態更新端點；此時部署達到期望的一份可用副本。', { focus: 'service', observed: '部署完成 · v1 × 1' }),
  ]),
  step('從電腦呼叫真正的 API', '用本機轉送，把請求送進剛才啟動的 Pod。', 'make m0-forward\n# 另一終端\ncurl --fail http://127.0.0.1:18080/version\ncurl --fail http://127.0.0.1:18080/quote -H \'Content-Type: application/json\' --data \'{"amount_cents":20000,"customer_tier":"vip","coupon_code":"SAVE100"}\'', [
    event('建立本機轉送', 'kubectl 透過 API 通道連到 Service 所選的 Pod A。這不是經 Service ClusterIP 的負載分流。', { forward: 'A', focus: 'client', signal: 'request' }),
    event('確認版本是 v1', 'GET /version 經 localhost:18080 進入 Pod A:8080，再把版本回傳給你。', { signal: 'request', result: '/version → v1' }),
    event('價格計算也正確', 'POST /quote 得到 final_amount_cents=8000。這次請求真的走到叢集中的服務。', { signal: 'request', result: '/quote → 8000 分（80 元）' }),
  ]),
  step('提交健康新版', '修改 Deployment 的模板，建立新的 Pod；現有 Pod 不會原地換映像。', 'make m0-update-good', [
    event('目標改為 v2-good', '命令提交成功，代表設定被接受；更新尚未完成。Pod A 仍然是 v1。', { image: 'v2-good', focus: 'deployment', signal: 'submit', observed: '更新中 · 等待新版' }),
    event('新 ReplicaSet 建立 Pod B', '允許額外增加一個 Pod，所以先建立新版 B，舊版 A 保持 Ready。', { pods: [A, change(B, 'pending')], focus: 'rs', signal: 'create' }),
    event('新版開始執行，等待檢查', 'A 仍可用；B 是另一個 Pod，Running 並不表示已能接收服務流量。', { pods: [A, change(B, 'starting')], focus: 'pods' }),
  ]),
  step('新版就緒，舊版才退出', '先確認新版可用，再縮減舊 ReplicaSet。', '# 等 make m0-update-good 完成\n# 終端 B：Ctrl+C，再執行\nmake m0-forward\n# 終端 C\ncurl --fail http://127.0.0.1:18080/version', [
    event('Pod B 通過 readiness', 'B 的 /readyz 回傳 200。短暫有 A、B 兩個 Ready Pod。', { pods: [A, B], signal: 'probe', probe: 'B · /readyz → HTTP 200', focus: 'kubelet' }),
    event('舊 Pod A 開始退出', '舊 ReplicaSet 縮減。A 退出可用端點，正在終止；原轉送不會自動改連 B。', { pods: [change(A, 'terminating'), B], forward: '', focus: 'pods' }),
    event('健康換版完成', 'A 結束後，剩下一份 Ready 的 v2-good。新的 Pod 有自己的身分 B。', { pods: [B], observed: '部署完成 · v2-good × 1', focus: 'deployment' }),
    event('重新轉送，驗證新版', '重新執行 m0-forward，這次連到 B；/version 回傳 v2-good。', { forward: 'B', signal: 'request', result: '/version → v2-good' }),
  ]),
  step('把同一版本增加成兩份', '擴容改的是數量，這次沒有更換 Pod 模板。', 'make m0-scale-two\nmake m0-status', [
    event('期望副本 1 → 2', 'Deployment 映像仍是 v2-good，同一個 ReplicaSet 的目標數量增加。', { replicas: 2, focus: 'deployment', signal: 'submit', observed: '擴容中 · 1 / 2 Ready' }),
    event('同一個 ReplicaSet 建立 C', '原本的 Pod B 保留；另外啟動 Pod C，並等待 readiness。', { pods: [B, change(C, 'starting')], focus: 'pods', signal: 'create' }),
    event('B、C 都可用', '現在有兩份相同版本的程式。Service 可用端點為 B、C；現有 port-forward 仍固定連 B。', { pods: [B, C], probe: 'C · /readyz → HTTP 200', signal: 'probe', observed: '部署完成 · v2-good × 2' }),
  ]),
  step('壞版是額外的第三個 Pod', '兩個健康 Pod 保留，故障新版出現在另一個位置。', 'make m0-break\nmake m0-status', [
    event('提交 v2-broken，目標仍是兩份', 'maxSurge=1 允許多出一個 Pod；maxUnavailable=0 要求更新時維持可用副本。', { image: 'v2-broken', signal: 'submit', focus: 'deployment', observed: '更新中 · 新版尚未 Ready' }, 10),
    event('新 ReplicaSet 額外建立 D', 'B、C 的映像、身分與健康狀態都沒變。第三個位置出現 v2-broken Pod D。', { pods: [B, C, change(D, 'pending')], signal: 'create', focus: 'pods' }, 10),
    event('D 開始執行，但無法 Ready', '這個故障版能啟動程序，卻不能通過就緒檢查。它不會取代正在服務的 B、C。', { pods: [B, C, D], focus: 'pods' }, 10),
  ]),
  step('追查更新為什麼卡住', '分別看存活探測、就緒探測與整體 rollout。', 'make m0-observe\n# 觀察後按 Ctrl+C\nmake m0-status', [
    event('存活探測成功', '/healthz 回傳 200，程序仍存活。這不是 CrashLoopBackOff，也沒有因 readiness 失敗而重啟。', { probe: 'D · /healthz → HTTP 200', signal: 'probe', focus: 'kubelet' }, 10),
    event('就緒探測持續失敗', '/readyz 回傳 503。D 維持 Running / NotReady，Service 可用端點仍然只有 B、C。', { probe: 'D · /readyz → HTTP 503', signal: 'probe', focus: 'service' }, 10),
    event('更新進度期限超過', '等待時間已縮短。Deployment 可回報 ProgressDeadlineExceeded；這不會自動回滾，舊健康 Pod 仍可用。', { observed: 'ProgressDeadlineExceeded', focus: 'deployment' }, 10),
    event('舊服務可用，不代表新版成功', '本機轉送仍連到 B，所以 /version 回的是 v2-good。讀取服務成功與新版 rollout 成功是兩件事。', { signal: 'request', result: '/version → v2-good（舊健康版）' }, 10),
  ]),
  step('由你指定恢復 v1', '只改回映像，保留兩副本；由健康 Pod 逐步接手。', 'make m0-restore', [
    event('人工把目標設回 v1', '副本數保持 2。保留下來的 v1 ReplicaSet 可以再利用，但會建立新的 Pod。', { image: 'v1', focus: 'deployment', signal: 'submit', observed: '恢復中 · v1 × 2' }),
    event('先移除不可用的 D', '這是資源足夠時的一種代表性時序。移除故障 Pod 不會減少 B、C 這兩份可用副本。', { pods: [B, C, change(D, 'terminating')], focus: 'pods' }),
    event('建立新的 v1 Pod E', 'D 結束後，v1 ReplicaSet 啟動 E。它不是先前的 A，也不是把 D 改成 v1。', { pods: [B, C, change(E, 'starting')], signal: 'create', focus: 'pods' }),
    event('E 就緒，B 開始退出', 'E 已 Ready，才讓一份舊健康版 B 退出。轉送連著 B，之後必須重建。', { pods: [change(B, 'terminating'), C, E], forward: '', probe: 'E · /readyz → HTTP 200', focus: 'pods' }),
    event('建立第二份 v1：Pod F', 'B 結束後建立 F；此時 C 與 E 保持可用，F 等待就緒。', { pods: [C, E, change(F, 'starting')], signal: 'create', focus: 'pods' }),
    event('F 就緒，C 開始退出', '兩份 v1 都已 Ready，最後一份 v2-good 可以退出。', { pods: [change(C, 'terminating'), E, F], probe: 'F · /readyz → HTTP 200', focus: 'pods' }),
    event('恢復完成：兩份 v1', '最終只剩新的 E、F。這是原生 Deployment 更新，沒有 Hub 的 Release 快照回滾。', { pods: [E, F], observed: '部署完成 · v1 × 2', focus: 'deployment' }),
  ]),
  step('驗證結果，結束本機連線', '停止轉送和刪除叢集，影響完全不同。', '# 終端 B：重新建立轉送\nmake m0-forward\n# 終端 C：查版本、readyz，並重做價格請求\ncurl --fail http://127.0.0.1:18080/version\ncurl --include --fail http://127.0.0.1:18080/readyz\n# 結束轉送：終端 B 按 Ctrl+C\n# 不再保留練習叢集時才執行 make m0-down', [
    event('重新連線並驗證', '轉送可選到 E 或 F；本例選 E。確認版本 v1、readyz 200，再呼叫 quote 核對 8000 分。', { forward: 'E', signal: 'request', result: 'v1 · Ready 200 · 價格 8000 分' }),
    event('Ctrl+C 只停止轉送', '本機通道消失，但叢集中的 E、F 仍然 Ready。停止終端轉送不會刪除服務。', { forward: '', focus: 'client' }),
    event('練習完成，可選擇保留環境', '若要刪除整個練習叢集，另執行 make m0-down。下方可切換清理示意，預設保留目前結果。', { focus: 'pods', result: '完成：兩個 v1 Pod 持續執行' }),
  ]),
];
export const totalSeconds = story.reduce((n, s) => n + s.events.reduce((m, e) => m + e.seconds, 0), 0);
export function validateStory() {
  const ids = new Map();
  for (const s of story) for (const e of s.events) {
    if (new Set(e.pods.map(p => p.slot)).size !== e.pods.length) throw Error('Overlapping Pod slots');
    for (const p of e.pods) {
      if (ids.has(p.id) && ids.get(p.id) !== p.image) throw Error('Pod image mutated');
      ids.set(p.id, p.image);
      if (p.image === 'v2-broken' && p.status === 'ready') throw Error('Broken Pod is Ready');
    }
    if (e.pods.some(p => p.id === 'D') && e.pods.filter(p => p.status === 'ready').length !== 2) throw Error('Old healthy replicas not preserved');
  }
  if (story.length !== 12 || state.pods.length !== 2 || state.pods.some(p => p.image !== 'v1')) throw Error('Incomplete M0 story');
}
