// Offline teaching snapshots, derived from stages/m1..m5 and their implementations.
export const P = (id, image, rev, slot, status='ready') => ({id,image,rev,slot,status});
const pair=(prefix='a',rev=2,image='v2-good')=>[P(`${prefix}${rev}-1`,image,rev,0),P(`${prefix}${rev}-2`,image,rev,1)];
const W=(name,pods=[],extra={})=>({name,pods,cr:'尚未建立',deployment:'尚未建立',observation:'尚無業務應用',...extra});
const N=(value,detail='',tone='')=>({value,detail,tone});
const R=(rev,image,replicas,status)=>({rev,image,replicas,status});
function author(meta){
 let state={nodes:{},workers:[],records:[],facts:[],flow:[],focus:[],badge:'教學示意',evidence:[],...meta.initial};
 const steps=[];
 const event=(title,why,patch={},seconds=7)=>{
   state={...state,flow:[],focus:[],facts:[],evidence:[],...patch,nodes:{...state.nodes,...patch.nodes}};
   return {title,why,seconds,...structuredClone(state)};
 };
 const step=(title,question,action,events)=>steps.push({title,question,action,events});
 return {event,step,finish:()=>({...meta,initial:undefined,steps})};
}
// Sequential representative rollout with an explicit Terminating scene.
function rollout(a,initial,count,image,rev,workerIndex=0,workers=[],prefix='a'){
 let pods=structuredClone(initial), n=0;
 const events=[],snap=()=>workers.map((w,i)=>i===workerIndex?{...w,pods:structuredClone(pods),cr:`rev ${rev} · ${image} × ${count}`,deployment:`${image} × ${count}`,observation:'更新中'}:w);
 const emit=(title,why)=>events.push(a.event(title,why,{workers:snap(),focus:['workload']}));
 const remove=()=>{
   const old=pods.find(p=>p.rev!==rev&&p.status!=='ready')||pods.find(p=>p.rev!==rev);
   if(!old)throw Error('No old Pod to retire');
   if(old.status==='ready'&&pods.filter(p=>p.status==='ready').length<=count)throw Error('Availability would decrease');
   old.status='terminating';emit(`${old.id} 進入 Terminating`,'縮減舊 ReplicaSet；這個 Pod 退出可用端點。刪除與完全退出是不同時點。');
   pods=pods.filter(p=>p.id!==old.id);emit(`${old.id} 已退出`,'釋放原本的 Pod 位置；舊 Pod 不會被原地換成另一個映像。');
 };
 while(n<count){
   while(pods.length>=count+1)remove();
   const slot=[0,1,2].find(s=>!pods.some(p=>p.slot===s));
   const next=P(`${prefix}${rev}-${++n}`,image,rev,slot,'starting');pods.push(next);
   emit(`新增 ${next.id}，等待 Ready`,'新的 ReplicaSet 建立另一個 Pod。容器開始執行後，還要等 kubelet 的 readiness 探針。');
   next.status='ready';emit(`${next.id} 通過 readiness`,'/readyz 回傳 200，新 Pod 才能成為 Service 的可用端點。');
   if(pods.filter(p=>p.status==='ready').length>count&&pods.some(p=>p.rev!==rev))remove();
 }
 while(pods.some(p=>p.rev!==rev))remove();
 return {events,pods:structuredClone(pods),workers:snap().map((w,i)=>i===workerIndex?{...w,observation:'目標 rollout 完成'}:w)};
}
const nodes1=[['app','Application','你提交的期望配置'],['operator','Go Operator','持續比對與修正'],['deployment','Deployment／Service','Operator 維護的資源'],['status','Application.status','回報目前配置的結果'],['native','Kubernetes 控制器','Deployment → RS → Pod'],['ownership','Owner 與 GC','資源的相依關係']];
function m1(){
 const a=author({id:'m1',name:'讓期望配置持續成立',intro:'看懂 Application、Operator 與 Kubernetes 各自負責什麼，再追蹤漂移、故障與清理。',nodes:nodes1,boundary:'M1 沒有 Hub 發布履歷；Operator 不會修正程式 bug，也不會自行選擇回滾版本。',initial:{workers:[W('M1 · hf-demo')]}}),e=a.event,s=a.step;
 s('為什麼需要 Operator？','底層設定被改掉後，誰記得你原本想要什麼？','用途說明；後面才開始操作。',[
 e('保留一份平台期望','Application 描述映像、副本、埠與探針。Operator 持續維持這份配置，而不只是執行一次部署腳本。',{nodes:{app:N('期望配置','image / replicas / revision'),operator:N('讀取 → 比對 → 修正'),deployment:N('受管理的實際配置')},flow:[['app','operator'],['operator','deployment']]}),
 e('原生控制器繼續負責 Pod','Operator 建立 Deployment／Service；Deployment 與 ReplicaSet 控制器才負責維持 Pod。',{nodes:{native:N('建立、更新與維持 Pod'),status:N('觀測結果，不是願望')},flow:[['deployment','native']],focus:['native']})]);
 s('先安裝型別與管理程式','CRD 定義資源型別；Operator 才是執行控制迴圈的程式。','make m1-up\n# 此命令涵蓋準備、安裝與第一次部署；以下分段解釋。',[
 e('安裝 Application CRD','Kubernetes 現在能接受 Application 物件；只有 CRD 還不會建立價格 API。',{nodes:{app:N('CRD 已註冊','尚無 Application 物件'),operator:N('尚待啟動'),deployment:N('尚未建立'),native:N('尚無業務 Pod')}}),
 e('啟動受限 Operator','Operator 取得管理 hf-demo 工作負載所需的權限。管理程式自己的 Pod 與價格 API 是不同程式。',{nodes:{operator:N('執行中','監看 Application 與受管理資源','good')},focus:['operator']})]);
 s('提交第一份 Application','期望與實際資源，依序建立。','make m1-up\n# 延續同一次啟動，不重跑命令。',[
 e('建立 rev 1：v1、一副本','先保存 CR。命令提交成功，不等於已完成 rollout。',{nodes:{app:N('rev 1 · v1 × 1','spec：使用者設定'),status:N('等待觀測')},workers:[W('M1 · hf-demo',[],{cr:'rev 1 · v1 × 1'})],flow:[['app','operator']]}),
 e('Operator 建立 Deployment 與 Service','依 Application 配置產生資源，並寫入指向 Application UID 的 ownerReference。',{nodes:{deployment:N('v1 × 1','Service 同時建立'),ownership:N('Application → Deployment / Service')},workers:[W('M1 · hf-demo',[],{cr:'rev 1 · v1 × 1',deployment:'v1 × 1'})],flow:[['operator','deployment']]}),
 e('原生控制器建立第一個 Pod','ReplicaSet 建立 a1-1，kubelet 啟動容器，尚待 readiness 通過。',{workers:[W('M1 · hf-demo',[P('a1-1','v1',1,0,'starting')],{cr:'rev 1 · v1 × 1',deployment:'v1 × 1',observation:'Running ≠ Ready'})],flow:[['deployment','native']]})]);
 s('觀測到 Ready，再呼叫服務','Application.status 必須反映目前配置的真實結果。','make m1-status\nmake m1-forward\n# 另一終端\ncurl --fail http://127.0.0.1:18080/version',[
 e('Pod 通過就緒檢查','這時 Deployment 的更新、Ready、Available 副本都符合目標。',{workers:[W('M1 · hf-demo',[P('a1-1','v1',1,0)],{cr:'rev 1 · v1 × 1',deployment:'v1 × 1',observation:'完整 rollout 已核對'})],nodes:{native:N('1 updated / ready / available')}}),
 e('Operator 更新目前 generation 的狀態','核對 observedGeneration 與目前 metadata.generation，再讀 Ready。observedRevision 也要對上提交的 rev 1。',{nodes:{status:N('Ready=True · observedRevision=1','observedGeneration 對上目前 generation','good')},flow:[['native','operator'],['operator','status']]}),
 e('本機轉送驗證價格 API','port-forward 連到 Service 選定的一個 Pod；/version=v1，價格範例得到 8000 分。業務請求不經 Operator。',{facts:[['版本','v1'],['價格','8000 分'],['控制程式','Operator'],['業務程式','pricing-api']]})]);
 s('提交健康更新與兩副本','改 Application；Operator 再把設定傳到 Deployment。','make m1-update-good',[
 e('Application 更新為 rev 2','新意圖是 v2-good、兩副本。舊 a1-1 保留原身分與映像。',{nodes:{app:N('rev 2 · v2-good × 2'),status:N('新 generation 等待觀測')},flow:[['app','operator']]}),
 e('Operator 更新 Pod template','revision 也會寫入 template annotation，因此新的 revision 形成獨立 rollout。',{nodes:{deployment:N('v2-good × 2','template revision=2'),status:N('Ready=False · Progressing=True')},flow:[['operator','deployment']]})]);
 let r=rollout(a,[P('a1-1','v1',1,0)],2,'v2-good',2,0,[W('M1 · hf-demo')]);
 s('新版逐個接手','新 Pod Ready 後，才逐步退出舊 Pod。','make m1-status\n# 完成換版後重新建立 make m1-forward。',[...r.events,e('rev 2 完整就緒','新 revision 必須有自己的完整 rollout 證據。單看一個舊 Pod 還能回應，不足以判定成功。',{workers:r.workers,nodes:{status:N('Ready=True · observedRevision=2','目前 generation 已處理','good'),native:N('2 updated / ready / available')},facts:[['Application','rev 2'],['目標映像','v2-good'],['就緒副本','2'],['Service 端點','a2-1、a2-2']]})]);
 s('故意改壞底層設定','漂移發生在兩層期望之間，Pod 不一定已經減少。','make m1-drift\nmake m1-observe',[
 e('只把 Deployment replicas 改成 1','Application 仍要求 2。這個範例保留兩個 Pod：Operator 可能在縮容發生前就修好。',{nodes:{deployment:N('replicas=1','與 Application 的 2 不一致','warn'),app:N('rev 2 · replicas=2','仍是期望來源'),status:N('上次觀測 Ready=True','底層已變更，等待重新核對')},workers:[W('M1 · hf-demo',r.pods,{cr:'rev 2 · v2-good × 2',deployment:'v2-good × 1',observation:'spec 漂移；Pod 此刻仍有 2'})],focus:['app','deployment']}),
 e('受管理資源變更觸發比對','Operator 讀回 Application 與 Deployment，發現副本數不一致。',{nodes:{operator:N('發現差異：2 ≠ 1','依 CR 重算受管理配置')},flow:[['deployment','operator'],['app','operator']]})]);
 s('Operator 把期望修回來','修正 spec 與補建 Pod，由不同控制器負責。','make m1-status\n# Ctrl+C 結束 observe。',[
 e('Deployment replicas 回到 2','Operator 重寫底層 spec；Application revision 仍是 2，不需使用者重新發布。',{nodes:{deployment:N('replicas=2','已與 Application 一致','good'),operator:N('完成本輪比對')},workers:r.workers,flow:[['operator','deployment']]}),
 e('核對配置與可用副本','本例兩個原 Pod 都保留。如果 Pod 已經先被縮掉，則由 ReplicaSet 補回。不要把 Pod 消失當作必要條件。',{nodes:{status:N('目前配置 Ready','revision 不因漂移增加','good')},facts:[['CR replicas','2'],['Deployment replicas','2'],['Ready Pods','2'],['revision','仍為 2']]})]);
 const bad=[...r.pods,P('a3-1','v2-broken',3,[0,1,2].find(slot=>!r.pods.some(p=>p.slot===slot)),'unready')];
 s('壞版無法靠 Reconcile 修好','Operator 維持使用者指定的壞版，不會自行猜測應回哪一版。','make m1-break\nmake m1-status',[
 e('提交 rev 3：v2-broken','Operator 把新映像與 revision 寫到 Deployment；舊健康 Pod 不會變壞。',{nodes:{app:N('rev 3 · v2-broken × 2'),operator:N('維持 rev 3 配置'),deployment:N('v2-broken × 2'),native:N('等待新 template rollout'),status:N('Ready=False · Progressing=True')},flow:[['app','operator'],['operator','deployment']]}),
 e('額外第三個 Pod NotReady','/healthz=200、/readyz=503。保留兩個舊健康副本，故障 Pod 不加入可用端點。',{workers:[W('M1 · hf-demo',bad,{cr:'rev 3 · v2-broken × 2',deployment:'v2-broken × 2',observation:'舊版可用；新版無法完成'})],focus:['workload']}),
 e('回報目前版本的故障原因','確認逾時證據屬於目前 template／ReplicaSet 後，回報 Ready=False、Degraded=True、ProgressDeadlineExceeded。',{nodes:{native:N('舊 2 Ready／新 1 NotReady'),status:N('Ready=False · Degraded=True','ProgressDeadlineExceeded','warn')},flow:[['native','operator'],['operator','status']]})]);
 s('提交新 revision 恢復','恢復映像仍是一個新意圖，不是把 revision 改回 1。','make m1-restore',[
 e('指定 rev 4：v1、兩副本','新 revision 寫入 template。即使映像叫 v1，也要以 rev 4 的新 ReplicaSet 判斷此次更新。',{nodes:{app:N('rev 4 · v1 × 2'),operator:N('維持 rev 4 配置'),native:N('依 template rev 4 逐步替換'),deployment:N('v1 × 2 · template rev 4'),status:N('等待 rev 4 rollout')},flow:[['app','operator'],['operator','deployment']]}),
 ...rollout(a,bad,2,'v1',4,0,[W('M1 · hf-demo')]).events,
 e('只以新版本結果確認恢復','舊 rev 3 的故障不能污染 rev 4。最後核對 observedRevision=4、目前 generation 與兩個 v1 Ready。',{workers:[W('M1 · hf-demo',pair('a',4,'v1'),{cr:'rev 4 · v1 × 2',deployment:'v1 × 2',observation:'Ready'})],nodes:{native:N('2 updated / ready / available'),status:N('Ready=True · observedRevision=4','','good')},facts:[['恢復映像','v1'],['副本','2'],['新 revision','4'],['驗證','重建轉送後查版本／價格']]})]);
 s('刪除 Application','先刪 owner，然後非同步清理相依資源。','make m1-delete',[
 e('送出 Application 刪除','停止自己的 port-forward，再刪 CR。刪除命令與相依資源消失是不同時點。',{nodes:{app:N('已送出刪除'),ownership:N('等待 Kubernetes GC')},flow:[['app','ownership']]}),
 e('GC 沿 ownerReference 清理','Deployment／Service 由 Application 擁有，RS／Pods 再依相依關係清理。此處展示一種退出時序。',{nodes:{deployment:N('刪除中'),ownership:N('Application → Deployment → RS → Pods')},workers:[W('M1 · hf-demo',pair('a',4,'v1').map(p=>({...p,status:'terminating'})),{cr:'已刪除',deployment:'刪除中',observation:'等待 GC'})],focus:['ownership','workload']})]);
 s('確認留下的是管理程式','被管理的應用消失，不代表 Operator 自己被刪除。','make m1-status',[
 e('價格 API 的相依資源已消失','核對 Application、Deployment、Service、ReplicaSet 與業務 Pod 最終都已刪除。',{nodes:{app:N('已刪除'),deployment:N('Deployment／Service 已刪除'),native:N('業務 RS／Pods 已清除'),status:N('Application 已不存在'),operator:N('仍在執行','可以處理其他 Application','good'),ownership:N('GC 完成')},workers:[W('M1 · hf-demo',[],{cr:'已刪除',deployment:'已刪除',observation:'Operator 自己仍在執行'})],facts:[['業務 Pod','0'],['Operator','保留'],['清理機制','ownerReference / GC'],['下一課','由 Hub 保存每次發布履歷']]})]);
 return a.finish();
}

const hubNodes=[['browser','瀏覽器','部署表單與結果'],['hub','Hub／Runner','接受意圖與非同步執行'],['store','SQLite','Release／Operation'],['cr','Application CR','叢集裡的期望'],['operator','Operator','維持 Deployment／Service'],['status','部署判定','核對實際 rollout']];
function m2(){
 const a=author({id:'m2',name:'讓每次發布留下履歷',intro:'跟著一筆部署從表單、資料庫到真實 Pod，分清接受、套用與成功，再回滾一份完整快照。',nodes:hubNodes,boundary:'M2 是單叢集、本機 dev-only 模式；Hub 使用受限 namespace kubeconfig，尚無 M3 Agent 或 M4 OIDC。',initial:{workers:[W('單叢集 · hf-demo')]}}),e=a.event,s=a.step;
 s('為什麼加上 Hub？','CR 告訴叢集要什麼；使用者還需要知道每次操作發生了什麼。','make m2-up\n# 瀏覽器開啟 http://127.0.0.1:18090',[
 e('用表單提交部署','Hub 提供 UI／REST，把使用者意圖保存成可追蹤的業務操作。',{nodes:{browser:N('部署表單'),hub:N('等待提交'),store:N('尚無此應用履歷'),cr:N('尚未建立'),operator:N('已啟動'),status:N('尚未部署')}}),
 e('把配置與結果分開保存','Release 保存完整配置快照；Operation 追蹤這次執行的狀態與原因。',{facts:[['Application','目前期望／觀測'],['Release','每版完整快照'],['Operation','一次操作的結果'],['SQLite','持久保存']],focus:['store']})]);
 s('按下第一次部署','保存意圖與回應接受，先於真正的 Pod 就緒。','瀏覽器：名稱 pricing-api、image v1、replicas 1、port 8080 → 提交。',[
 e('Hub 在交易中保存 rev 1','Application、Release rev 1 與 Operation 一起保存，讓後續工作可追蹤。',{nodes:{browser:N('送出 v1 × 1'),hub:N('驗證並提交交易'),store:N('rev 1 · PENDING / QUEUED')},records:[R(1,'v1',1,'PENDING')],flow:[['browser','hub'],['hub','store']]}),
 e('HTTP 202：已接受','202 沒有宣告部署完成。此時 Pod 仍可能不存在，Operation 等待 Runner 執行。',{nodes:{browser:N('202 Accepted','尚未部署成功'),hub:N('Operation QUEUED')},facts:[['desired','1'],['applied','尚未確認'],['last successful','無'],['業務 Pod','0']]})]);
 s('Runner 把意圖送進叢集','M2 Hub 用受限 kubeconfig 操作 Application。','瀏覽器：查看 Operation 從 QUEUED 進入執行。',[
 e('Runner 開始處理','Hub 讀取持久意圖，提交具型別的 Application 配置；不是把任意 YAML 或 shell 傳進叢集。',{nodes:{hub:N('Operation RUNNING'),cr:N('提交 rev 1 · v1 × 1')},flow:[['store','hub'],['hub','cr']]}),
 e('applied=1：CR 已寫入','Applied 表示這版意圖已套用，不代表 Pod Ready。Operator 接著維持 Deployment／Service。',{nodes:{cr:N('rev 1 已寫入'),operator:N('建立 Deployment／Service'),status:N('等待 rollout')},workers:[W('單叢集 · hf-demo',[],{cr:'rev 1 · v1 × 1',deployment:'v1 × 1'})],facts:[['desired','1'],['applied','1'],['last successful','無'],['Operation','RUNNING']],flow:[['cr','operator']]})]);
 s('等實際工作負載 Ready','只有完整目標 rollout 成功，才完成這次 Operation。','等待 SUCCEEDED／READY，再 make m2-forward 並呼叫 /version 與 /quote。',[
 e('建立新 Pod，等待探針','原生控制器建立 a1-1；Running 時仍可能 NotReady。',{workers:[W('單叢集 · hf-demo',[P('a1-1','v1',1,0,'starting')],{cr:'rev 1 · v1 × 1',deployment:'v1 × 1',observation:'等待 readiness'})],focus:['workload']}),
 e('真實 rollout 已完成','核對目前 template、generation、updated／ready／available 副本後，Hub 才能完成 Operation。',{workers:[W('單叢集 · hf-demo',[P('a1-1','v1',1,0)],{cr:'rev 1 · v1 × 1',deployment:'v1 × 1',observation:'Ready'})],nodes:{browser:N('rev 1：部署成功'),hub:N('Operation SUCCEEDED'),operator:N('持續 Reconcile Application'),status:N('SUCCEEDED / READY','','good'),store:N('rev 1 · ACTIVE')},records:[R(1,'v1',1,'ACTIVE')],facts:[['desired','1'],['applied','1'],['last successful','1'],['HTTP 驗證','v1／8000 分']],flow:[['operator','status'],['status','store']]})]);
 s('重送與衝突如何處理？','網路重試不該變成兩次發版；過期意圖也不該覆蓋新狀態。','機制補充：依 M2 API 教學使用 Idempotency-Key 與 expected_desired_revision。',[
 e('相同 key、相同內容重送','Hub 回應原本的操作結果，不新增重複 Release；下方履歷仍只有 rev 1。',{nodes:{browser:N('相同 Idempotency-Key 重送'),hub:N('找到原操作','不新增 Release')},flow:[['browser','hub'],['hub','store']]}),
 e('衝突請求回 409','同 key 不同內容、過期的 expected revision，或前一次操作尚未完成時再提交，都有衝突保護；先讀回狀態再決定。',{nodes:{hub:N('409 CONFLICT','新請求被拒絕','warn')},facts:[['相同 key + 相同內容','沿用原操作'],['同 key + 不同內容','409'],['過期提交基準','409'],['同應用已有活動操作','409']]})]);
 s('發布健康新版','rev 2 保存 v2-good 與兩副本的完整快照。','瀏覽器：發布 v2-good、replicas 2，等待結果。',[
 e('保存新的 rev 2','rev 1 快照仍保留；新的 Operation 處理 rev 2。',{nodes:{browser:N('發布 rev 2'),hub:N('執行 rev 2'),store:N('rev 2 · PENDING'),cr:N('rev 2 · v2-good × 2'),status:N('DEPLOYING')},records:[R(1,'v1',1,'ACTIVE'),R(2,'v2-good',2,'PENDING')],facts:[['desired','2'],['applied','2'],['last successful','1'],['目標副本','2']]})]);
 const r=rollout(a,[P('a1-1','v1',1,0)],2,'v2-good',2,0,[W('單叢集 · hf-demo')]);
 s('看新版接手並寫回結果','Pod 的真實更新與 Hub 的結果提交分開呈現。','瀏覽器等待 SUCCEEDED／READY；重新建立轉送驗證 v2-good。',[...r.events,e('rev 2 成功','last successful 推進到 2；舊快照保留，後面可以選它作為回滾來源。',{workers:r.workers,nodes:{browser:N('rev 2：部署成功'),hub:N('Operation SUCCEEDED'),status:N('SUCCEEDED / READY','','good'),store:N('rev 2 · ACTIVE')},records:[R(1,'v1',1,'歷史快照'),R(2,'v2-good',2,'ACTIVE')],facts:[['desired','2'],['applied','2'],['last successful','2'],['就緒副本','2']]})]);
 const bad=[...r.pods,P('a3-1','v2-broken',3,[0,1,2].find(slot=>!r.pods.some(p=>p.slot===slot)),'unready')];
 s('發布壞版，觀察失敗','有舊 Pod 可以服務，不表示這次新發布成功。','瀏覽器：發布 v2-broken、replicas 2，查看原因與事件。',[
 e('rev 3 意圖已接受並套用','desired 與 applied 都可能已經是 3；最後成功版本仍是 2。',{nodes:{browser:N('發布 rev 3'),cr:N('rev 3 · v2-broken × 2'),hub:N('Operation RUNNING'),status:N('等待新版')},records:[R(1,'v1',1,'歷史快照'),R(2,'v2-good',2,'ACTIVE'),R(3,'v2-broken',2,'PENDING')],facts:[['desired','3'],['applied','3'],['last successful','2'],['Operation','RUNNING']]}),
 e('第三個壞 Pod 無法 Ready','額外建立 a3-1，舊 a2-1／a2-2 保持健康。/readyz=503 使 rollout 卡住。',{workers:[W('單叢集 · hf-demo',bad,{cr:'rev 3 · v2-broken × 2',deployment:'v2-broken × 2',observation:'舊版可用；新版卡住'})],focus:['workload']}),
 e('失敗結果保存到履歷','目前版本故障或操作期限到達後，Operation 為 FAILED／TIMED_OUT；Release 3 為 FAILED，last successful 保持 2。',{nodes:{browser:N('rev 3：失敗原因'),hub:N('Operation FAILED / TIMED_OUT'),status:N('FAILED / TIMED_OUT','核對本次原因','warn'),store:N('保留 rev 3 失敗履歷')},records:[R(1,'v1',1,'歷史快照'),R(2,'v2-good',2,'ACTIVE'),R(3,'v2-broken',2,'FAILED')],facts:[['desired','3'],['applied','3'],['last successful','2'],['Release 3','FAILED']]})]);
 s('選成功快照，建立新回滾','教學主線選 rev 2，恢復它的完整設定。','瀏覽器：回滾來源 revision 2 → 核對 v2-good／兩副本 → 確認。',[
 e('取出 rev 2 的完整快照','恢復 image、replicas、port、資源與探針等配置，不只是換 image 字串。',{nodes:{browser:N('確認來源 rev 2'),store:N('讀取 rev 2 配置快照')},flow:[['browser','hub'],['store','hub']]}),
 e('建立 rev 4 與新的 Operation','rev 3 不會消失或被改寫；回滾是以舊配置建立新的部署。',{nodes:{hub:N('執行新的回滾操作'),cr:N('rev 4 · v2-good × 2'),status:N('等待 rev 4 rollout')},records:[R(1,'v1',1,'歷史快照'),R(2,'v2-good',2,'ACTIVE'),R(3,'v2-broken',2,'FAILED'),R(4,'v2-good',2,'PENDING')],facts:[['desired','4'],['applied','4'],['last successful','2'],['來源快照','rev 2']]})]);
 const back=rollout(a,bad,2,'v2-good',4,0,[W('單叢集 · hf-demo')]);
 s('驗證回滾自己的 rollout','相同映像加上新 revision，也必須等待新模板的 Pod Ready。','等待 rev 4 SUCCEEDED，再重建 make m2-forward，驗證版本與價格。',[...back.events,e('回滾完成，失敗記錄仍在','rev 4 成功後，desired／applied／last successful 都是 4；映像仍叫 v2-good。',{workers:back.workers,nodes:{browser:N('rev 4：回滾成功'),hub:N('Operation SUCCEEDED'),store:N('rev 4 · ACTIVE'),status:N('rev 4 · SUCCEEDED / READY','','good')},records:[R(1,'v1',1,'歷史快照'),R(2,'v2-good',2,'歷史快照'),R(3,'v2-broken',2,'FAILED'),R(4,'v2-good',2,'ACTIVE')],facts:[['desired','4'],['applied','4'],['last successful','4'],['服務驗證','v2-good／8000 分']]})]);
 s('重啟後，什麼會留下？','保存同一資料庫，才能讀回相同履歷。','依 M2 教學停止／重啟 Hub，沿用原 SQLite 路徑。',[
 e('Hub 程序暫停','Worker 裡的 Operator 與工作負載繼續運作；資料庫不是程序記憶體。',{nodes:{hub:N('程序暫停'),store:N('SQLite 檔案仍保留'),operator:N('仍在叢集內執行')},focus:['store','operator']}),
 e('讀回歷史，再取得新觀測','Hub 使用同一資料庫恢復履歷，並重新查詢叢集。歷史成功不能取代目前 readiness。',{nodes:{hub:N('重新啟動'),store:N('rev 1～4 履歷可讀回'),status:N('重新核對 Worker 現況')},flow:[['store','hub'],['hub','cr']]})]);
 return a.finish();
}

function m3(){
 const nodes=[['hub','Hub','集中管理 A／B'],['agentA','Worker A · Agent','由 Worker 主動建連'],['agentB','Worker B · Agent','獨立的控制通道'],['ledger','Agent ledger','命令高水位與身分'],['operatorA','A · Operator','當地持續修復'],['operatorB','B · Operator','當地持續修復']];
 const a=author({id:'m3',name:'連得到，也能各自運作',intro:'跟著 A、B 兩個 Worker 看命令與狀態如何往返，再停掉 A 的通道，觀察管理與服務的差別。',nodes,boundary:'Hub 不持有 Worker kubeconfig；動畫中的業務請求不走 Agent 控制通道。斷線觀測要搭配最後心跳與觀測時間。',initial:{workers:[W('Worker A · pricing-a'),W('Worker B · pricing-b')]}}),e=a.event,s=a.step;
 s('為什麼由 Worker 主動連線？','Hub 集中管理，Worker 保有當地的執行與修復能力。','make m3-up',[
 e('Hub 等待兩個 Worker','A、B 各有 Agent 與 Operator。Hub 不持有它們的管理員 kubeconfig。',{nodes:{hub:N('等待 Agent 通道','無 Worker kubeconfig'),agentA:N('準備連線'),agentB:N('準備連線'),operatorA:N('當地控制器已啟動'),operatorB:N('當地控制器已啟動'),ledger:N('各 Worker 持久保存')}}),
 e('A 的 Agent 主動撥向 Hub','mTLS 驗證兩端身分與叢集綁定，建立長連線的雙向 gRPC 控制通道。',{nodes:{agentA:N('CONNECTED','Agent → Hub 建連','good')},flow:[['agentA','hub']]}),
 e('B 建立自己的通道','B 的連線與 A 獨立。不可信 CA、錯誤 SAN 或叢集身分不能成為有效通道。',{nodes:{agentB:N('CONNECTED','Agent → Hub 建連','good')},flow:[['agentB','hub']]})]);
 s('向 A 提交一次部署','建連方向與命令方向不同：命令走既有的雙向通道。','瀏覽器建立 pricing-a：Worker A、v1、一副本。',[
 e('Hub 沿 A 的通道送出 Apply','只有型別化、受限的命令進入 Agent，沒有任意 shell 或 kubectl exec。',{nodes:{hub:N('pricing-a · rev 1 Apply'),agentA:N('接收 A 的命令')},flow:[['hub','agentA']]}),
 e('Agent 核對 ledger 後寫 CR','核對命令版本與身分，再將意圖套用當地 Application。ACK 不是 Pod Ready。',{nodes:{ledger:N('A：rev 1／CR UID 已記錄'),operatorA:N('收到當地 Application')},workers:[W('Worker A · pricing-a',[],{cr:'rev 1 · v1 × 1',deployment:'待 Operator 建立'}),W('Worker B · pricing-b')],flow:[['agentA','ledger'],['agentA','operatorA']]}),
 e('當地 Operator 建立工作負載','Deployment／RS 建立 Pod；Agent 之後回報觀測。Hub 不直接建立 Pod。',{workers:[W('Worker A · pricing-a',[P('a1-1','v1',1,0,'starting')],{cr:'rev 1 · v1 × 1',deployment:'v1 × 1',observation:'Applied，但尚未 Ready'}),W('Worker B · pricing-b')],focus:['workload']})]);
 s('A Ready，再建立 B','每個應用都有獨立 revision；A 的更新不推進 B。','等待 A READY，再在 Worker B 建立 pricing-b：v1、一副本。',[
 e('A 的就緒觀測回到 Hub','A 的真實 rollout 完成，經 Agent Snapshot 回報，Hub 才確認成功。',{workers:[W('Worker A · pricing-a',[P('a1-1','v1',1,0)],{cr:'rev 1 · v1 × 1',deployment:'v1 × 1',observation:'Hub：READY'}),W('Worker B · pricing-b')],flow:[['operatorA','agentA'],['agentA','hub']]}),
 e('B 走自己的部署流程','Hub 對 B 送 Apply，B Agent 寫 CR；B Operator 建立工作負載。',{workers:[W('Worker A · pricing-a',[P('a1-1','v1',1,0)],{cr:'rev 1 · v1 × 1',deployment:'v1 × 1',observation:'Hub：READY'}),W('Worker B · pricing-b',[P('b1-1','v1',1,0,'starting')],{cr:'rev 1 · v1 × 1',deployment:'v1 × 1',observation:'等待 readiness'})],flow:[['hub','agentB'],['agentB','operatorB']]}),
 e('兩邊都 Ready，分別呼叫 API','A 轉送 18080，B 轉送 18081，各自取得 8000 分。請求不經 Hub 的控制串流。',{workers:[W('Worker A · pricing-a',[P('a1-1','v1',1,0)],{cr:'rev 1 · v1 × 1',deployment:'v1 × 1',observation:'Hub：READY'}),W('Worker B · pricing-b',[P('b1-1','v1',1,0)],{cr:'rev 1 · v1 × 1',deployment:'v1 × 1',observation:'Hub：READY'})],facts:[['A API','18080 → 選定 A Pod'],['B API','18081 → 選定 B Pod'],['價格','兩邊均 8000 分'],['應用版本','各自 rev 1']]})]);
 const wb=W('Worker B · pricing-b',[P('b1-1','v1',1,0)],{cr:'rev 1 · v1 × 1',deployment:'v1 × 1',observation:'Hub：READY'});
 const up=rollout(a,[P('a1-1','v1',1,0)],2,'v1',2,0,[W('Worker A · pricing-a'),wb]);
 s('先讓 A 有兩個副本','本課新增 revision，純副本變更也會更新 template marker。','瀏覽器：pricing-a 改為 replicas 2，等待成功。',[...up.events,e('A rev 2，B 仍 rev 1','A 完成兩副本後才進入斷線演練；兩邊版本與狀態分開讀。',{workers:up.workers,nodes:{ledger:N('A：rev 2／B：rev 1','各自保存 CR UID 與命令高水位')},facts:[['A revision','2'],['A Ready','2'],['B revision','1'],['B Ready','1']]})]);
 s('只停止 A 的通訊 Agent','Agent、Operator、價格 API 是三個不同的程式。','make m3-agent-stop-a',[
 e('A Agent 停止','只關閉 A 的通訊角色。當地 Operator 與兩個 API Pod 仍在運作。',{nodes:{agentA:N('Agent 已停止','通道中斷','warn'),operatorA:N('仍在執行','','good')},workers:up.workers,focus:['agentA','operatorA']}),
 e('Hub 確認斷線與觀測過期','等待斷線或心跳逾時後，A 為 DISCONNECTED、應用現況為 UNKNOWN；保留最近觀測時間。',{nodes:{hub:N('A：DISCONNECTED / UNKNOWN','B：CONNECTED'),agentA:N('DISCONNECTED','','warn')},workers:up.workers.map((w,i)=>i===0?{...w,observation:'Hub：UNKNOWN／舊觀測；當地仍有兩個 Ready'}:w),facts:[['A 通道','DISCONNECTED'],['A Hub 現況','UNKNOWN'],['A 當地 Pod','仍可用'],['B 通道','CONNECTED']]})]);
 s('離線時，A 的新變更被拒絕','不可把無法送達的操作顯示成正在順利部署。','瀏覽器：觀察 A 斷線；先不要恢復 Agent。',[
 e('新的 A 變更回 503','部署、更新、回滾或刪除要求會被拒絕，不會默默排隊等待送達。',{nodes:{hub:N('A 新變更：503','既有服務不受此拒絕影響','warn')},focus:['hub']}),
 e('UNKNOWN 不等於服務當機','Hub 缺少最新證據；當地 Pod 的 Running／Ready 是另一個觀測面。',{nodes:{operatorA:N('持續依 Application 修復')},focus:['operatorA','workload']})]);
 const ar={...up.workers[0],observation:'Hub：UNKNOWN；當地兩副本可用'};
 const submitB=e('Hub 對 B 送出更新','A 斷線不應擋住 B。B 自己的 revision 前進到 2。',{nodes:{hub:N('執行 B rev 2'),agentB:N('接收 B 更新','','good')},flow:[['hub','agentB'],['agentB','operatorB']]});
 const br=rollout(a,wb.pods,2,'v2-good',2,1,[ar,wb],'b');
 s('A 離線時，B 繼續發布','B 的命令和觀測走另一條已驗證的控制通道。','瀏覽器：pricing-b 發布 v2-good／兩副本，等待成功。',[
 submitB,...br.events,
 e('B 成功，A 仍 UNKNOWN','重新建立 B 的轉送，查到 v2-good。A 的管理現況仍要等待重連後的新 Snapshot。',{workers:br.workers,nodes:{hub:N('B：SUCCEEDED / READY','A：UNKNOWN'),ledger:N('A：rev 2／B：rev 2','各自 CR UID 與高水位保留'),agentB:N('CONNECTED／新觀測已回報','','good'),operatorB:N('維持 B rev 2 兩副本')},flow:[['agentB','hub']]})]);
 s('離線 A 仍能修復漂移','修復路徑完全在 Worker A 當地。','make m3-drift-a\nmake m3-observe-a',[
 e('A Deployment 改成一副本','A Application 仍要求兩份。本例 Operator 在 Pod 縮減前發現差異，兩個健康 Pod 保留。',{workers:[{...ar,deployment:'v1 × 1'},br.workers[1]],nodes:{operatorA:N('CR=2，Deployment=1','發現當地差異','warn')},focus:['operatorA']}),
 e('當地 Operator 把 spec 修回 2','不經 Hub，也不需要 Agent 通道。若 Pod 已先消失，再由 ReplicaSet 補回。',{workers:[ar,br.workers[1]],nodes:{operatorA:N('Deployment replicas=2','修復完成，revision 仍是 2','good')},facts:[['A CR','2 副本'],['A Deployment','修回 2'],['Agent','仍停止'],['Hub 現況','仍 UNKNOWN']]})]);
 s('恢復 Agent，重新核對','Pod Ready 與通道 CONNECTED，要分別確認。','make m3-agent-start-a',[
 e('Agent Pod 已啟動','Agent 自己的 Pod Ready，不代表 mTLS 串流已建立。',{nodes:{agentA:N('Pod Ready','等待通道握手')}}),
 e('重新握手與身分驗證','A 再次主動連向 Hub；信任與叢集身分通過才接受控制通道。',{nodes:{agentA:N('CONNECTED','','good')},flow:[['agentA','hub']]}),
 e('核對 ledger 與現有 CR','重連讀回高水位、spec 與 UID；不盲目重播過期命令，不因連線恢復新增 revision。',{nodes:{ledger:N('原 CR UID／rev 2 保留')},flow:[['agentA','ledger']],facts:[['A revision','仍為 2'],['CR UID','保留'],['過期命令','不覆蓋新狀態'],['新發版','無']]})]);
 s('讓 Hub 看見新觀測','觀測恢復後才把 UNKNOWN 更新為實際結果。','make m3-status-a\nmake m3-status-b',[
 e('A 新 Snapshot 抵達','A 的實際兩副本 Ready 回報到 Hub。這次修復與重連沒有新增發布版本。',{workers:[{...ar,observation:'Hub：READY／新 Snapshot'},br.workers[1]],nodes:{hub:N('A／B CONNECTED','兩邊結果均已核對','good')},flow:[['operatorA','agentA'],['agentA','hub']]}),
 e('控制集中，修復留在當地','Hub 管意圖與履歷；Agent 運送受限命令與觀測；Operator 維持當地工作負載。',{facts:[['Hub','管理意圖／履歷'],['Agent','通道／ledger／Snapshot'],['Operator','當地 Reconcile'],['Kubernetes','RS／Pod／探針']]})]);
 return a.finish();
}

function m4(){
 const nodes=[['identity','OIDC／Dex','驗證你是誰'],['hub','Hub session／授權','檢查身分、角色與租戶'],['membership','DB membership','誰能對哪個租戶做什麼'],['agent','受限 Agent','傳遞允許的部署意圖'],['admission','Kubernetes Admission','資源建立的邊界'],['network','NetworkPolicy／CNI','封包通行的邊界']];
 const a=author({id:'m4',name:'每個操作，都有身分與邊界',intro:'把登入、租戶角色、部署配額與封包隔離分開看。每種拒絕都發生在不同位置，也留下不同結果。',nodes,boundary:'本頁不展示真實帳密或 session。配額與封包動畫僅解釋驗收情境；NetworkPolicy 的封包阻擋以 A Worker 的實際 CNI 驗證為範圍。',initial:{workers:[W('Worker A · hf-team-a')]}}),e=a.event,s=a.step;
 s('先分清三種問題','登入成功、可以部署、封包能通，並不是同一件事。','make m4-up\n# 使用本輪本機帳號；不把密碼或憑證放進動畫。',[
 e('你是誰？','OIDC 驗證身分；平台還要檢查 session 與角色，才能決定是否接受操作。',{nodes:{identity:N('身分驗證'),hub:N('操作授權'),membership:N('租戶角色'),agent:N('受限 Worker 通道'),admission:N('CPU／Memory 配額'),network:N('租戶封包隔離')},focus:['identity','hub']}),
 e('你能做什麼？資源容許嗎？','Hub 的拒絕發生在部署意圖之前；Admission 的拒絕發生在 Worker 建立資源時。',{facts:[['OIDC','身分'],['Hub RBAC','租戶與操作'],['Quota','資源建立'],['NetworkPolicy','封包']]})]);
 s('完成 OIDC 登入','身分從已驗證的登入流程取得，不靠任意傳入使用者名稱。','瀏覽器：開啟 M4 /login，使用本輪帳號完成登入。',[
 e('瀏覽器導向 Dex','登入流程使用 state／nonce／PKCE 等檢查；頁面不展示或保存範例密碼。',{nodes:{identity:N('登入與回呼驗證'),hub:N('等待驗證')},flow:[['hub','identity']]}),
 e('Hub 建立平台 session','驗證成功後建立 session。OIDC groups 不直接賦予平台角色，角色由 DB membership 決定。',{nodes:{hub:N('身分已驗證','仍需租戶授權'),membership:N('查詢 DB membership')},flow:[['identity','hub'],['hub','membership']]})]);
 s('啟用已準備好的租戶','管理頁要核對 Worker 的真實材料，不能只新增資料庫列。','admin：啟用已 bootstrap 的 team-a／A 與 B；指定 owner-a。',[
 e('Worker 已由 bootstrap 準備','Namespace、受限 RBAC、Quota／LimitRange 與 receipt 由本機管理員的 bootstrap 準備。',{nodes:{admission:N('hf-team-a Quota／LimitRange 已準備'),agent:N('可提供 Worker proof')},focus:['admission']}),
 e('Hub 向 Agent 核對證據','核對 namespace、receipt UID 與配置，通過後才啟用租戶 profile。離線或未 bootstrap 的 Worker 不能假裝啟用成功。',{nodes:{hub:N('核對 Worker proof'),agent:N('回傳受限證據')},flow:[['hub','agent'],['agent','hub']]}),
 e('Owner 授予租戶角色','owner-a 授予 Developer 與 Viewer。相同使用者在不同租戶可能有不同角色。',{nodes:{membership:N('team-a：Owner／Developer／Viewer'),hub:N('profile 已啟用','','good')},facts:[['Owner','管理租戶成員'],['Developer','部署應用'],['Viewer','唯讀'],['角色來源','DB membership']]})]);
 s('Developer 部署自己的應用','允許變更後，才建立發布意圖並送往 Worker。','developer-a：team-a／A 建立 v1、一副本。',[
 e('Hub 檢查變更請求','核對 session、Origin／CSRF、租戶 membership、角色，以及部署的冪等鍵與預期版本。',{nodes:{hub:N('Developer / team-a：允許'),membership:N('team-a Developer 已確認')},flow:[['membership','hub']]}),
 e('接受意圖，Agent 套用 CR','操作進入 QUEUED／DEPLOYING；認證成功本身仍不表示應用 Ready。',{nodes:{agent:N('Apply → Application'),admission:N('資源符合限制')},workers:[W('Worker A · hf-team-a',[P('a1-1','v1',1,0,'starting')],{cr:'rev 1 · v1 × 1',deployment:'v1 × 1',observation:'等待 readiness'})],flow:[['hub','agent'],['agent','admission']]}),
 e('Worker 回報真正的成功','Pod 通過探針、完整 rollout 核對後才 READY／SUCCEEDED；轉送測試價格為 8000 分。',{workers:[W('Worker A · hf-team-a',[P('a1-1','v1',1,0)],{cr:'rev 1 · v1 × 1',deployment:'v1 × 1',observation:'READY／SUCCEEDED'})],nodes:{hub:N('部署成功','','good')},facts:[['操作者','Developer'],['租戶','team-a'],['就緒副本','1'],['價格','8000 分']]})]);
 s('Viewer 可以看，不能改','隱藏按鈕只是介面；真正的限制在伺服器。','切換 viewer-a，查看 team-a 應用與版本履歷。',[
 e('讀取允許','Viewer 可看配置、狀態與履歷；介面不提供部署、回滾與刪除表單。',{nodes:{membership:N('team-a Viewer'),hub:N('讀取：允許')},flow:[['membership','hub']]}),
 e('自行構造變更仍被拒絕','伺服器授權拒絕變更。沒有新增 Operation、CR revision 或 Pod；原有服務維持。',{nodes:{hub:N('變更：403 PERMISSION_DENIED','','warn'),agent:N('沒有新部署命令')},facts:[['新增 Operation','0'],['新增 CR revision','0'],['新增 Pod','0'],['既有服務','持續執行']]})]);
 s('登入 team-a，不代表能看 team-b','授權必須包含租戶範圍。','developer-a：嘗試開啟 /tenants/team-b。',[
 e('身分有效，但沒有 team-b 權限','擁有 team-a Developer 角色，不代表取得 team-b 的任何資料讀取權。',{nodes:{identity:N('session 有效'),membership:N('只有 team-a 角色'),hub:N('檢查 team-b membership')},flow:[['membership','hub']]}),
 e('查詢邊界回 403','拒絕不暴露另一租戶的應用內容；保留 request_id 供診斷。這次是讀取拒絕，不是部署故障。',{nodes:{hub:N('403 PERMISSION_DENIED','保留 request_id','warn')},facts:[['請求租戶','team-b'],['已有角色','team-a Developer'],['資料揭露','拒絕'],['Worker 變更','無']]})]);
 s('最後一位 Owner 也有保護','角色管理不能把租戶留成沒有 Owner。','owner-a：依教學嘗試把最後一位 Owner 自己改為 Viewer。',[
 e('嘗試降低最後一位 Owner 的角色','這個變更會使租戶失去管理成員的人。',{nodes:{membership:N('team-a：最後一位 Owner'),hub:N('檢查角色變更規則')}}),
 e('409，角色維持不變','這是成員管理的衝突保護；與跨租戶 403、Worker quota 失敗不同。',{nodes:{hub:N('409 CONFLICT','最後一位 Owner 保護','warn'),membership:N('Owner 角色保留')},facts:[['原角色','Owner'],['變更結果','拒絕'],['實際角色','仍為 Owner'],['部署影響','無']]})]);
 const healthy=pair();
 s('切換情境：配額不足','獨立的 E2E 機制範例：既有兩個健康副本，surge 所需空間不足。','依 M4 配額 E2E 教學；此段不是前面被拒絕的請求所產生。',[
 e('先看配額演練的起點','這是另一個兩副本場景，兩個健康 Pod 已存在。新版本要額外建立一個 surge Pod。',{badge:'獨立情境 · 配額 Admission',workers:[W('Worker A · hf-team-a',healthy,{cr:'新 revision · 兩副本',deployment:'更新中 · 兩副本',observation:'既有兩個健康 Pod'})],nodes:{hub:N('Developer 意圖已接受'),membership:N('team-a Developer','此獨立情境的操作者'),agent:N('新 CR 已套用'),admission:N('剩餘配額不足以建立 surge')},facts:[['既有 Ready','2'],['需要額外 Pod','1'],['不足資源','CPU／Memory'],['失敗位置','Worker Admission']]}),
 e('CREATE 被 Admission 拒絕','Quota 檢查發生在 Pod 建立時。請求被拒絕，因此沒有第三個壞 Pod 物件出現。',{nodes:{admission:N('拒絕 Pod CREATE','ResourceQuota 超額','warn')},flow:[['agent','admission']],focus:['admission','workload']}),
 e('目前 ReplicaSet 回報 FailedCreate','Operator 只採用目前 template 的故障證據，再經 Agent 回到 Hub；既有兩個健康 Pod 仍在。',{nodes:{hub:N('部署失敗：配額原因','不是使用者權限 403','warn')},facts:[['新增 Pod 物件','0'],['既有 Ready','2'],['原因','FailedCreate／quota'],['觀測來源','目前 ReplicaSet']]})]);
 s('封包隔離是另一道邊界','登入與配額都不能證明 NetworkPolicy 有效。','依 M4 E2E 驗證 A Worker 的實際封包通行與阻擋。',[
 e('先有政策與可執行它的 CNI','A Worker 使用可執行這些 NetworkPolicy 的 CNI；不能只看到 YAML 就說隔離成功。',{badge:'獨立情境 · 封包隔離',nodes:{network:N('A：CNI＋NetworkPolicy'),admission:N('配額是另一種檢查')},focus:['network']}),
 e('對允許與拒絕路徑送出封包','實際連線結果才是隔離證據。本動畫的示意不替代 E2E，也不替 B Worker 宣稱相同保證。',{nodes:{network:N('允許流量通過／禁止流量被擋','需以實際封包測試驗證')},evidence:['允許路徑：連線應成功','禁止路徑：連線應被阻擋','範圍：A Worker；不是只檢查 policy 存在']})]);
 s('用拒絕位置判讀問題','看到錯誤時，先問它發生在使用者授權、資源建立還是網路。','對照 M4 狀態、Operation 原因與 E2E 證據。',[
 e('三種拒絕，三種結果','Hub 403 不建立新意圖；Quota 拒絕不建立新 Pod；NetworkPolicy 阻擋特定封包。',{nodes:{hub:N('403：授權拒絕'),admission:N('FailedCreate：資源拒絕'),network:N('封包：路徑阻擋')},facts:[['Hub 拒絕','沒有新 Operation'],['Quota 拒絕','沒有被拒絕的 Pod 物件'],['網路隔離','需測實際封包'],['診斷','先找拒絕的那一層']],evidence:[]})]);
 return a.finish();
}

function m5(){
 const nodes=[['dashboard','Dashboard','使用者看到的結果'],['hub','Hub／SQLite','意圖與持久履歷'],['agent','Worker Agent','命令與真實觀測'],['operator','Operator','當地 workload 狀態'],['telemetry','Metrics／Traces','解釋一次操作'],['delivery','Bootstrap／Helm','安裝與交付邊界']];
 const a=author({id:'m5',name:'把一次操作追到結果',intro:'從 Dashboard 發版與故障開始，追到 Worker、回滾履歷、跨元件 trace，最後看懂 Helm 交付的範圍。',nodes,boundary:'Dashboard 是觀測入口；SQLite 保存履歷，Worker 提供真實狀態。Hub viewer 只有當前 Hub spans；跨元件 trace 需要另外收集日誌。',initial:{workers:[W('Worker A · pricing-api'),W('Worker B · 尚未建立業務應用')]}}),e=a.event,s=a.step;
 s('先把三種資料分開','目前能不能用、曾經做過什麼、為什麼失敗，各有不同來源。','make m5-up\n# 依教學登入、啟用租戶並授予 Owner。',[
 e('Dashboard 組合三種資訊','同一頁呈現狀態與履歷，但它們不是同一種證據。',{nodes:{dashboard:N('應用／版本／觀測時間'),hub:N('SQLite：意圖與歷史'),agent:N('Worker：真實 Snapshot'),operator:N('Application Conditions'),telemetry:N('telemetry：解釋因果'),delivery:N('已準備平台元件')},facts:[['目前狀態','Worker 觀測'],['發布歷史','SQLite'],['原因追查','logs / traces / metrics'],['尚未有','業務應用']]}),
 e('平台啟動，不等於業務已部署','Agent 與 Operator 準備好之後，仍須登入、啟用租戶並提交應用。本例 A、B 都尚無業務 Pod。',{focus:['agent','operator','workload'],facts:[['平台元件','已準備'],['業務 Pod','0'],['下一步','提交第一版'],['驗證依據','之後的真實 rollout']]})]);
 s('部署第一版並驗證','先有真實服務，後面的故障追查才有對象。','owner-a：team-a／A 建立 v1、一副本，等待成功；再用 make m5-forward-a 驗證。',[
 e('保存 rev 1，送到 Worker','Dashboard 提交之後，先看到 QUEUED／DEPLOYING。Agent 寫 CR，Operator 維持資源。',{nodes:{dashboard:N('rev 1 · DEPLOYING'),hub:N('rev 1 / Operation 已保存'),agent:N('Apply rev 1'),operator:N('建立 v1 workload')},records:[R(1,'v1',1,'PENDING')],workers:[W('Worker A · pricing-api',[P('a1-1','v1',1,0,'starting')],{cr:'rev 1 · v1 × 1',deployment:'v1 × 1',observation:'等待 Ready'}),W('Worker B · 尚未建立業務應用')],flow:[['dashboard','hub'],['hub','agent'],['agent','operator']]}),
 e('Ready 回報，版本與價格都正確','Worker rollout 核對後完成 Operation；本機 API 回應 v1／8000 分。',{nodes:{dashboard:N('READY / SUCCEEDED','','good')},records:[R(1,'v1',1,'ACTIVE')],workers:[W('Worker A · pricing-api',[P('a1-1','v1',1,0)],{cr:'rev 1 · v1 × 1',deployment:'v1 × 1',observation:'READY'}),W('Worker B · 尚未建立業務應用')],flow:[['operator','agent'],['agent','hub'],['hub','dashboard']]})]);
 const wb=W('Worker B · 尚未建立業務應用');
 const submitGood=e('提交 rev 2','保存完整快照與 Operation，last successful 暫時仍是 1。',{nodes:{dashboard:N('rev 2 · DEPLOYING'),hub:N('rev 2 執行中'),agent:N('Apply rev 2'),operator:N('更新 v2-good × 2')},records:[R(1,'v1',1,'ACTIVE'),R(2,'v2-good',2,'PENDING')]});
 const good=rollout(a,[P('a1-1','v1',1,0)],2,'v2-good',2,0,[W('Worker A · pricing-api'),wb]);
 s('建立健康的 rev 2','更新為兩副本，讓後續有已知成功的恢復來源。','Dashboard：發布 v2-good／兩副本，等待成功。',[
 submitGood,...good.events,
 e('rev 2 成為最新成功版本','核對兩個新 Pod Ready 與完整目標 rollout；版本快照可供回滾。',{workers:good.workers,nodes:{dashboard:N('rev 2 · READY','','good')},records:[R(1,'v1',1,'歷史快照'),R(2,'v2-good',2,'ACTIVE')],facts:[['desired','2'],['applied','2'],['last successful','2'],['Ready Pods','2']]})]);
 s('用 Dashboard 觀察漂移修復','修復發生在 Worker，並不產生新的發布履歷。','make m5-drift\nmake m5-workload-status\n# 完整命令依 M5 教學。',[
 e('底層 Deployment 被改成 1','Application 要求 2。這裡保留原 Pod，表示修復可能早於真正縮容。',{workers:[{...good.workers[0],deployment:'v2-good × 1',observation:'spec 漂移'},wb],nodes:{operator:N('Application=2，Deployment=1','發現差異','warn')},focus:['operator']}),
 e('修回 2，再回報觀測','Operator 修正 Deployment；Agent 回報真實 Snapshot，Dashboard 顯示新觀測。revision 仍是 2。',{workers:good.workers,nodes:{operator:N('spec 已修復'),dashboard:N('兩副本 Ready／新觀測時間')},flow:[['operator','agent'],['agent','hub'],['hub','dashboard']]})]);
 const bad=[...good.pods,P('a3-1','v2-broken',3,[0,1,2].find(slot=>!good.pods.some(p=>p.slot===slot)),'unready')];
 s('發布故障版','這次要追查的是 rev 3 的操作，而不是所有舊 Pod。','Dashboard：發布 v2-broken，保留兩副本。',[
 e('保存 rev 3 與關聯資訊','Operation／request 的關聯讓後續可以追到這次意圖。畫面使用教學代號，不代表真實 ID。',{nodes:{hub:N('rev 3 / op-demo-3 / req-demo-3'),dashboard:N('DEPLOYING'),agent:N('Apply rev 3'),operator:N('維持 rev 3 目標')},records:[R(1,'v1',1,'歷史快照'),R(2,'v2-good',2,'ACTIVE'),R(3,'v2-broken',2,'PENDING')],evidence:['教學代號：op-demo-3','教學代號：req-demo-3']}),
 e('第三個 Pod 的 readiness 失敗','a3-1 Running／NotReady，兩個健康 rev 2 Pod 保留。服務仍能回應舊版，不能算 rev 3 成功。',{workers:[W('Worker A · pricing-api',bad,{cr:'rev 3 · v2-broken × 2',deployment:'v2-broken × 2',observation:'/readyz 503；新版卡住'}),wb],focus:['workload']}),
 e('Dashboard 顯示失敗原因','Operation FAILED／TIMED_OUT，Release 3 FAILED；最後成功仍是 2。原因要連回目前版本的 Worker 證據。',{nodes:{dashboard:N('FAILED / TIMED_OUT','保留原因與 request_id','warn'),operator:N('目前版本故障證據')},records:[R(1,'v1',1,'歷史快照'),R(2,'v2-good',2,'ACTIVE'),R(3,'v2-broken',2,'FAILED')],facts:[['desired','3'],['applied','3'],['last successful','2'],['故障 Pod','a3-1']]})]);
 s('沿關聯追查這次操作','相同 request／operation 將意圖、命令與觀測串起來。','應用詳情：查看 Operation、原因、request_id 與事件。',[
 e('先從 Operation 履歷讀原因','SQLite 保存終態與錯誤，不因目前 Pod 還活著就覆寫那次失敗。',{nodes:{hub:N('op-demo-3：失敗履歷保留')},flow:[['dashboard','hub']]}),
 e('追到 Worker 的失敗證據','Agent Snapshot 回報 CR／Deployment 的狀態，Operator 的原因必須屬於這次 template。',{nodes:{agent:N('Snapshot：目前 generation／Conditions'),operator:N('rev 3 readiness／rollout 故障')},flow:[['operator','agent'],['agent','hub']]}),
 e('不要把舊錯誤當成新故障','目前故障與歷史 Operation 錯誤分開讀。新版本恢復後，歷史 rev 3 仍保留失敗。',{facts:[['歷史終態','保存於 SQLite'],['目前狀態','來自新 Snapshot'],['故障歸屬','目前 template／RS'],['觀測時間','必須一起讀']]})]);
 s('確認回滾來源 rev 2','完整來源配置會成為新的 rev 4。','Dashboard：選成功的 v2-good／兩副本快照，確認回滾。',[
 e('確認來源與目標配置','選 rev 2，核對 image、replicas、port、資源與探針，不只核對映像名稱。',{nodes:{dashboard:N('回滾確認：來源 rev 2'),hub:N('建立 rev 4，保留 rev 3'),agent:N('Apply rev 4'),operator:N('等待 rev 4 rollout')},records:[R(1,'v1',1,'歷史快照'),R(2,'v2-good',2,'ACTIVE'),R(3,'v2-broken',2,'FAILED'),R(4,'v2-good',2,'PENDING')],facts:[['desired','4'],['applied','待確認'],['last successful','2'],['來源','rev 2']],evidence:[]})]);
 const back=rollout(a,bad,2,'v2-good',4,0,[W('Worker A · pricing-api'),wb]);
 s('回滾完成後，再驗證服務','新 revision 的 Pod 必須真正 Ready。','等待 rev 4 SUCCEEDED；重建 make m5-forward-a，核對 v2-good、readyz 200、價格 8000 分。',[...back.events,e('rev 4 成功，rev 3 仍 FAILED','失敗歷史保留；最新成功改為新的回滾 revision。',{workers:back.workers,nodes:{dashboard:N('rev 4 · READY / SUCCEEDED','','good'),hub:N('rev 4 成功／rev 3 失敗履歷保留'),operator:N('rev 4 完整 rollout 已核對'),agent:N('回報 rev 4 Ready')},records:[R(1,'v1',1,'歷史快照'),R(2,'v2-good',2,'歷史快照'),R(3,'v2-broken',2,'FAILED'),R(4,'v2-good',2,'ACTIVE')],facts:[['desired','4'],['applied','4'],['last successful','4'],['API','v2-good／8000 分']]})]);
 s('B 的連線狀態另行觀察','這條教學主線尚未在 B 建立業務應用。','make m5-agent-stop-b\n# 確認 DISCONNECTED 後\nmake m5-agent-start-b',[
 e('B Agent 停止，等待斷線判定','A 的業務 Pod 維持。B 只是在演練控制通道，不能宣稱驗證了不存在的 B 業務服務。',{nodes:{agent:N('A CONNECTED／B DISCONNECTED'),dashboard:N('B：最後心跳保留')},workers:[back.workers[0],{...wb,observation:'Agent DISCONNECTED；未建立業務應用'}]}),
 e('B Agent Ready，還要等 CONNECTED','恢復 Pod 後完成 mTLS 握手，Dashboard 的 CONNECTED 才表示控制通道已恢復。',{nodes:{agent:N('A／B CONNECTED','','good'),dashboard:N('A 業務 READY／B 通道 CONNECTED')},workers:[back.workers[0],{...wb,observation:'Agent CONNECTED；仍無業務應用'}]})]);
 s('Admin 看 metrics 與 Hub spans','這兩種資料的保存範圍，與 SQLite 履歷不同。','切換 admin：開啟 /metrics、/traces。',[
 e('metrics 描述目前程序','操作次數、耗時與 Agent 連線等指標反映目前程序生命期；Hub 重啟會重設。',{nodes:{telemetry:N('Metrics：目前程序數值','不是持久稽核履歷')},facts:[['/metrics 權限','Platform Admin'],['程序重啟','指標重設'],['SQLite 履歷','保留'],['Owner／Viewer','不可讀全域 telemetry']]}),
 e('Hub viewer 只有 Hub spans','用 request／operation 找這次操作，但這裡不會自動收齊 Agent 與 Operator 日誌。',{nodes:{telemetry:N('/traces：當前 Hub spans','Worker spans 要另外收集')},evidence:['Hub HTTP request span','Hub operation span','Hub command span','尚需：Agent／Operator 的對應日誌']})]);
 s('收集跨元件 trace','每個命令有自己的 context；不要把整條長連線當成一次部署。','依 stages/m5/observability/README.md 收集 Hub／Agent／Operator logs，再使用 traceview 匯出。',[
 e('由同一操作找到命令','Hub operation context 隨每個 typed command 傳給 Agent。',{nodes:{telemetry:N('Hub operation → command → Agent'),hub:N('持久 TraceContext'),agent:N('每命令 child span')},flow:[['hub','agent']],evidence:['Hub：operation → Apply command','Agent：提取這筆命令的 context']}),
 e('CR annotations 把意圖帶到 Operator','Operator reconcile 可關聯到最近接受的意圖。自主漂移修復不表示使用者又發起新操作。',{nodes:{operator:N('CR annotations → reconcile span')},flow:[['agent','operator']],evidence:['Hub operation → Apply command','Agent Execute → CR annotations','Operator reconcile：關聯同一意圖']}),
 e('匯出後核對父子關係與元件','離線 viewer 讀已收集日誌；檔案存在不代表資料完整，需確認這次操作包含預期元件。',{nodes:{telemetry:N('離線 trace：核對同一次操作')},facts:[['輸入','已收集 logs'],['關聯','operation / request / trace'],['檢查','Hub、Agent、Operator'],['檔案角色','解釋證據，不發起部署']]})]);
 s('Helm 交付什麼？','安裝管理程式，與準備安全材料是兩種責任。','make m5-chart-test\n# 真實 install／upgrade／lifecycle 依 M5 Chart 教學與驗收入口。',[
 e('bootstrap 先準備安全材料','TLS、租戶 Namespace／Quota、receipt 與 ledger 等依正式 bootstrap 管理；不由瀏覽器任意修改。',{nodes:{delivery:N('Bootstrap：安全材料與 ownership')},evidence:['Bootstrap：TLS／租戶資源／ledger／receipt','Helm：CRD／受限 Operator／Agent／RBAC']}),
 e('Helm 安裝受限元件','Chart 安裝與更新 Operator／Agent 等平台元件；還要等真正 rollout、通道與 receipt 核對。',{nodes:{delivery:N('Helm install / upgrade → rollout 核對')},focus:['delivery']}),
 e('render 通過，不等於 lifecycle 通過','Chart 渲染檢查只證明產物。真實安裝、升級、卸載與重裝要另外核對 ownership 與安全資料保留。',{facts:[['Chart render','模板／schema 檢查'],['Lifecycle','真實叢集操作'],['安全資料','不視為一般快取清除'],['交付證據','命令、結果與限制']],nodes:{dashboard:N('操作可理解，結果可核對'),telemetry:N('與持久履歷、Worker 狀態互相對照')}})]);
 return a.finish();
}

export const courses = [m1(),m2(),m3(),m4(),m5()];
