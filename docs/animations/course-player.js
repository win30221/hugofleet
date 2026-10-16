(() => {
 'use strict';
 const $=id=>document.getElementById(id), esc=s=>String(s).replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
 const pref=matchMedia('(prefers-reduced-motion: reduce)');
 let reduced=pref.matches, si=0, ei=-1, time=0, playing=false, frame=0,last=0,speed=1,current,routes=[];
 const duration=()=>course.steps[si].events.reduce((n,e)=>n+e.seconds,0), start=i=>course.steps[si].events.slice(0,i).reduce((n,e)=>n+e.seconds,0), fmt=t=>`${Math.floor(t/60)}:${String(Math.floor(t%60)).padStart(2,'0')}`;
 const podDefs=new Map(),recordEls=new Map();
 course.steps.forEach(s=>s.events.forEach(e=>e.workers.forEach((w,i)=>w.pods.forEach(p=>podDefs.set(`${i}-${p.id}`,{...p,worker:i})))));
 $('course-title').textContent=course.name;$('intro').textContent=course.intro;$('eyebrow').textContent=`${course.id.toUpperCase()} / HUGOFLEET LEARNING PATH`;
 $('guide').href=$('tutorial').href=`../../stages/${course.id}/README.md`;$('boundary').textContent=course.boundary;
 $('total').textContent=`${course.steps.length} 步 · ${fmt(course.steps.reduce((n,s)=>n+s.events.reduce((m,e)=>m+e.seconds,0),0))}（不含停頓）`;
 $('steps').innerHTML=course.steps.map((s,i)=>`<button data-step="${i}"><span>${String(i+1).padStart(2,'0')}</span>${esc(s.title)}</button>`).join('');
 $('nodes').innerHTML=course.nodes.map(([id,name,role])=>`<article class="graph-node" id="node-${id}"><h3>${esc(name)}</h3><p class="node-role">${esc(role)}</p><div class="node-value"></div><div class="node-detail"></div></article>`).join('');
 const count=Math.max(...course.steps.flatMap(s=>s.events.map(e=>e.workers.length)));
 $('workers').style.setProperty('--workers',count);
 $('workers').innerHTML=Array.from({length:count},(_,i)=>`<section class="worker" id="worker-${i}"><h3></h3><div class="worker-config"><div><span>Application 期望</span><b class="cr"></b></div><div><span>Deployment 配置</span><b class="deployment"></b></div></div><div class="rack"><div class="empty-rack">尚無業務 Pod</div>${[...podDefs.values()].filter(p=>p.worker===i).map(p=>`<article class="course-pod" id="pod-${i}-${p.id}" style="--slot:${p.slot}" aria-hidden="true"><b>${esc(p.id)}</b><span class="image">${esc(p.image)}</span><span class="rs">RS rev ${esc(p.rev)}</span><span class="state"></span></article>`).join('')}</div><div class="worker-endpoints"></div><p class="worker-observation"></p></section>`).join('');
 const next=Number(course.id.slice(1))+1;$('next-course').href=next<=5?`m${next}.html`:'m0.html';$('next-course').textContent=next<=5?`下一章 M${next} →`:'回到 M0 →';
 function updatePlay(){ $('play').textContent=playing?'暫停':time>=duration()?'重播這一步':'播放這一步';document.body.dataset.playing=String(playing); }
 function stop(){playing=false;cancelAnimationFrame(frame);frame=0;last=0;updatePlay();}
 function play(){if(time>=duration()){time=0;ei=-1;}playing=true;last=0;updatePlay();if(!frame&&!document.hidden)frame=requestAnimationFrame(tick);}
 function hint(){ $('play-hint').textContent=reduced?'已減少位移與轉場效果；播放按鈕仍會依序播放分鏡。':'每步播完會停住。Space 播放／暫停 · ← → 切換步驟'; }
 function select(i,hash=true){stop();si=Math.max(0,Math.min(course.steps.length-1,i));time=0;ei=-1;const s=course.steps[si];$('number').textContent=`${String(si+1).padStart(2,'0')} / ${course.steps.length}`;$('title').textContent=s.title;$('question').textContent=s.question;$('command').textContent=s.action;$('seek').max=duration();$('end-time').textContent=fmt(duration());$('previous').disabled=si===0;$('next').disabled=si===course.steps.length-1;$('beats').innerHTML=s.events.map((e,j)=>`<button data-beat="${j}" aria-label="分鏡 ${j+1}：${esc(e.title)}">${j+1}</button>`).join('');document.querySelectorAll('[data-step]').forEach((b,j)=>{if(j===si)b.setAttribute('aria-current','step');else b.removeAttribute('aria-current');});if(hash)history.replaceState(null,'',`#step-${si+1}`);hint();draw();}
 function draw(){let sum=0,index=course.steps[si].events.length-1;for(const [i,e]of course.steps[si].events.entries()){sum+=e.seconds;if(time<sum-1e-7){index=i;break;}}$('seek').value=time;$('time').textContent=fmt(time);if(ei!==index){ei=index;current=course.steps[si].events[ei];render(current);}movePackets();updatePlay();}
 function render(e){
  $('badge').textContent=e.badge;$('event-title').textContent=e.title;$('why').textContent=e.why;$('beat-label').textContent=`分鏡 ${ei+1} / ${course.steps[si].events.length}`;
  $('seek').setAttribute('aria-valuetext',`第 ${si+1} 步：${e.title}`);document.querySelectorAll('[data-beat]').forEach((b,j)=>b.setAttribute('aria-current',String(j===ei)));
  for(const[id]of course.nodes){const el=$(`node-${id}`),n=e.nodes[id];el.classList.toggle('inactive',!n);el.classList.toggle('focus',e.focus.includes(id));el.dataset.tone=n?.tone||'';el.querySelector('.node-value').textContent=n?.value||'稍後說明';el.querySelector('.node-detail').textContent=n?.detail||'';}
  for(let i=0;i<count;i++){const el=$(`worker-${i}`),w=e.workers[i];el.hidden=!w;if(!w)continue;el.classList.toggle('focus',e.focus.includes('workload'));el.querySelector('h3').textContent=w.name;el.querySelector('.cr').textContent=w.cr;el.querySelector('.deployment').textContent=w.deployment;el.querySelector('.worker-observation').textContent=w.observation;el.querySelector('.empty-rack').hidden=!!w.pods.length;el.querySelector('.worker-endpoints').textContent=`可用端點：${w.pods.filter(p=>p.status==='ready').map(p=>p.id).join('、')||'無'} · ${w.pods.filter(p=>p.status==='ready').length} Ready`;
   for(const p of [...podDefs.values()].filter(p=>p.worker===i)){const card=$(`pod-${i}-${p.id}`),active=w.pods.find(q=>q.id===p.id);card.classList.toggle('present',!!active);card.setAttribute('aria-hidden',String(!active));if(active){card.style.setProperty('--slot',active.slot);card.dataset.status=active.status;card.querySelector('.state').textContent=active.status==='ready'?'Running · ✓ Ready':active.status==='terminating'?'Terminating · 退出中':'Running · ○ NotReady';}}
  }
  $('facts').hidden=!e.facts.length;$('facts').innerHTML=e.facts.map(([k,v])=>`<div class="fact"><span>${esc(k)}</span><strong>${esc(v)}</strong></div>`).join('');
  $('ledger').hidden=!e.records.length;
  for(const [rev,el]of recordEls){if(!e.records.some(r=>r.rev===rev)){el.remove();recordEls.delete(rev);}}
  for(const r of e.records){let el=recordEls.get(r.rev);if(!el){el=document.createElement('article');el.className='record';el.innerHTML='<strong></strong><span class="record-image"></span><small></small><span class="record-status"></span>';$('records').append(el);recordEls.set(r.rev,el);}el.dataset.status=r.status;el.querySelector('strong').textContent=`rev ${r.rev}`;el.querySelector('.record-image').textContent=r.image;el.querySelector('small').textContent=`${r.replicas} 副本 · 完整配置快照`;el.querySelector('.record-status').textContent=r.status;}
  $('evidence-panel').hidden=!e.evidence.length;$('evidence').innerHTML=e.evidence.map(v=>`<li>${esc(v)}</li>`).join('');
  $('flow-label').innerHTML=e.flow.length?e.flow.map(([x,y])=>`<span>${esc(course.nodes.find(n=>n[0]===x)[1])} → ${esc(course.nodes.find(n=>n[0]===y)[1])}</span>`).join(''):'<span>觀察目前配置與結果 · 此分鏡沒有新命令傳遞</span>';
  route();
 }
 function route(){if(!current)return;const base=$('graph').getBoundingClientRect();$('paths').innerHTML='';routes=[];for(const[x,y]of current.flow){const a=$(`node-${x}`).getBoundingClientRect(),b=$(`node-${y}`).getBoundingClientRect();let x1,y1,x2,y2,d;const row=Math.abs(a.top-b.top)<3;
   if(row){const right=b.left>a.left;x1=(right?a.right:a.left)-base.left;x2=(right?b.left:b.right)-base.left;y1=a.top+a.height/2-base.top;y2=b.top+b.height/2-base.top;if(Math.abs(a.left-b.left)>a.width+45){y1=a.top-base.top;x1=a.left+a.width/2-base.left;y2=b.top-base.top;x2=b.left+b.width/2-base.left;d=`M${x1},${y1} L${x1},${y1-16} L${x2},${y1-16} L${x2},${y2}`;}else d=`M${x1},${y1} L${x2},${y2}`;
   }else{const down=b.top>a.top;x1=a.left+a.width/2-base.left;x2=b.left+b.width/2-base.left;y1=(down?a.bottom:a.top)-base.top;y2=(down?b.top:b.bottom)-base.top;const mid=(y1+y2)/2;d=`M${x1},${y1} L${x1},${mid} L${x2},${mid} L${x2},${y2}`;}
   const path=document.createElementNS('http://www.w3.org/2000/svg','path');path.setAttribute('d',d);path.setAttribute('marker-end','url(#arrow)');const dot=document.createElementNS('http://www.w3.org/2000/svg','circle');dot.setAttribute('r','4');$('paths').append(path,dot);routes.push({path,dot});}movePackets();}
 function movePackets(){for(const {path,dot}of routes){const phase=reduced?.5:Math.min(1,Math.max(0,((time-start(ei))%3)/2.1));const p=path.getPointAtLength(path.getTotalLength()*phase);dot.setAttribute('cx',p.x);dot.setAttribute('cy',p.y);}}
 function tick(now){frame=0;if(!playing||document.hidden)return;if(last)time+=Math.min((now-last)/1000,.1)*speed;last=now;if(time>=duration()){time=duration();draw();if($('continuous').checked&&si<course.steps.length-1){select(si+1);play();return;}stop();$('play-hint').textContent=si===course.steps.length-1?'本章完成；可重播任何一步或前往下一章。':'這一步已完成，確認看懂後按「下一步」。';return;}draw();frame=requestAnimationFrame(tick);}
 function seek(t){stop();time=Math.max(0,Math.min(duration(),t));draw();}
 function motion(v=pref.matches){reduced=v;document.body.classList.toggle('reduced',v);$('motion').textContent=v?'完整動態效果':'減少動態';$('motion').setAttribute('aria-pressed',String(v));hint();}
 $('steps').onclick=e=>{const b=e.target.closest('[data-step]');if(b){select(+b.dataset.step);if(innerWidth<800)document.querySelector('.lesson').scrollIntoView({block:'start'});}};
 $('beats').onclick=e=>{const b=e.target.closest('[data-beat]');if(b)seek(start(+b.dataset.beat));};
 $('play').onclick=()=>playing?stop():play();$('replay').onclick=()=>{seek(0);play();};$('previous').onclick=()=>select(si-1);$('next').onclick=()=>select(si+1);$('seek').oninput=e=>seek(+e.target.value);$('speed').onchange=e=>{speed=+e.target.value;};$('motion').onclick=()=>motion(!reduced);pref.addEventListener('change',()=>motion());
 $('theme').onclick=()=>{document.body.classList.toggle('dark');$('theme').textContent=document.body.classList.contains('dark')?'淺色':'深色';};
 document.addEventListener('keydown',e=>{if(e.altKey||e.ctrlKey||e.metaKey||e.target.closest('input,select,button,a,summary,textarea'))return;if(e.code==='Space'){e.preventDefault();playing?stop():play();}if(e.key==='ArrowRight'){e.preventDefault();select(si+1);}if(e.key==='ArrowLeft'){e.preventDefault();select(si-1);}});
 document.addEventListener('visibilitychange',()=>{last=0;if(document.hidden){cancelAnimationFrame(frame);frame=0;}else if(playing)frame=requestAnimationFrame(tick);});
 addEventListener('resize',route);new ResizeObserver(route).observe($('graph'));
 addEventListener('hashchange',()=>{const m=location.hash.match(/^#step-(\d+)$/);if(m)select(+m[1]-1,false);});
 const initial=location.hash.match(/^#step-(\d+)$/);select(initial?+initial[1]-1:0,false);motion();
})();
