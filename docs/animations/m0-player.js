(() => {
  'use strict';
  const $ = id => document.getElementById(id);
  const motionPreference = matchMedia('(prefers-reduced-motion: reduce)');
  let reduced = motionPreference.matches;
  let stepIndex = 0, eventIndex = -1, time = 0, playing = false, frame = 0, last = 0, speed = 1, current, cleaned = false;
  const duration = () => story[stepIndex].events.reduce((n, e) => n + e.seconds, 0);
  const eventTime = i => story[stepIndex].events.slice(0, i).reduce((n, e) => n + e.seconds, 0);
  const fmt = t => `${Math.floor(t / 60)}:${String(Math.floor(t % 60)).padStart(2, '0')}`;
  const escape = s => String(s).replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
  const podDefinitions = new Map();
  story.forEach(s => s.events.forEach(e => e.pods.forEach(p => podDefinitions.set(p.id, p))));
  $('pods').innerHTML = [...podDefinitions.values()].map(p => `<article id="pod-${p.id}" class="pod" style="--y:${p.slot * 76}px" aria-hidden="true"><div class="pod-heading"><b>Pod ${p.id}</b><span>${p.image}</span></div><div class="pod-state"><span class="phase"></span><span class="ready"></span></div><div class="pod-rs">隸屬 RS-${p.image}</div></article>`).join('');
  $('steps').innerHTML = story.map((s,i) => `<button data-step="${i}"><span>${String(i+1).padStart(2,'0')}</span>${escape(s.title)}</button>`).join('');
  function stop() { playing = false; cancelAnimationFrame(frame); frame = 0; last = 0; updatePlay(); }
  function updatePlay() {
    $('play').textContent = playing ? '暫停' : time >= duration() ? '重播這一步' : '播放這一步';
    // Reduced motion changes presentation, never the user's playback controls.
    $('play').disabled = false;
    $('replay').disabled = false;
    document.body.dataset.playing = String(playing);
  }
  function play() {
    if (time >= duration()) { time = 0; eventIndex = -1; }
    if (cleaned) toggleCleanup(false);
    playing = true; last = 0; updatePlay();
    if (!frame && !document.hidden) frame = requestAnimationFrame(tick);
  }
  function selectStep(i, updateHash = true) {
    stop(); stepIndex = Math.max(0, Math.min(story.length-1, i)); time = 0; eventIndex = -1;
    toggleCleanup(false);
    const s = story[stepIndex];
    $('number').textContent = `${String(stepIndex+1).padStart(2,'0')} / 12`;
    $('title').textContent = s.title; $('question').textContent = s.question; $('command').textContent = s.command;
    $('previous').disabled = stepIndex === 0; $('next').disabled = stepIndex === story.length-1;
    $('seek').max = duration(); $('end-time').textContent = fmt(duration());
    $('beats').innerHTML = s.events.map((e,i) => `<button data-beat="${i}" aria-label="分鏡 ${i+1}：${escape(e.title)}">${i+1}</button>`).join('');
    document.querySelectorAll('[data-step]').forEach((b,i) => { if(i===stepIndex)b.setAttribute('aria-current','step');else b.removeAttribute('aria-current'); });
    $('aftercare').hidden = stepIndex !== 11;
    $('play-hint').textContent = reduced ? '已減少位移與轉場效果；按「播放這一步」仍會依序播放分鏡。' : '每一步結束會停住，確認看懂後再前進。Space 播放／暫停 · ← → 切換步驟';
    if(updateHash) history.replaceState(null,'',`#step-${stepIndex+1}`);
    draw();
  }
  function draw() {
    let sum = 0, index = story[stepIndex].events.length-1;
    for(const [i,e] of story[stepIndex].events.entries()) { sum += e.seconds; if(time < sum - 1e-7) {index=i;break;} }
    $('time').textContent = fmt(time); $('seek').value = time;
    if (index !== eventIndex) {
      eventIndex = index; current = story[stepIndex].events[index]; render(current);
    }
    movePacket(); updatePlay();
  }
  function render(e) {
    $('quote-scene').hidden = e.mode !== 'quote'; $('build-scene').hidden = e.mode !== 'build'; $('cluster-scene').hidden = e.mode !== 'cluster';
    $('scene-label').textContent = e.mode === 'quote' ? '用途 / 價格試算' : e.mode === 'build' ? '準備 / 程式 → 映像 → 節點' : '系統剖面 / hf-demo namespace';
    $('math').textContent = ['等待請求','200 × 90% − 100','180 − 100 = 80'][e.quote];
    $('math-detail').textContent = e.quote ? '先套用 VIP 折扣，再折抵優惠券。' : 'POST /quote';
    $('answer').classList.toggle('dim', e.quote < 2);
    $('images').classList.toggle('dim', e.build < 1); $('node-cache').classList.toggle('dim', e.build < 2);
    $('cache-result').textContent = e.build < 2 ? '等待載入' : '✓ 三個映像已載入';
    $('desired-image').textContent = e.image; $('desired-count').textContent = e.replicas;
    $('rs-list').innerHTML = [...new Set([e.image,...e.pods.map(p=>p.image)])].map(im => `<span>RS-${im}</span>`).join('');
    $('rs').style.opacity = stepIndex === 2 && eventIndex === 0 ? '.3' : '1';
    for(const p of podDefinitions.values()) {
      const el = $(`pod-${p.id}`), active = e.pods.find(q=>q.id===p.id);
      el.classList.toggle('present',!!active); el.setAttribute('aria-hidden',String(!active));
      if (active) {
        el.dataset.status = active.status;
        el.querySelector('.phase').textContent = active.status === 'pending' ? 'Pending · 等待啟動' : active.status === 'terminating' ? 'Terminating · 退出中' : 'Running · 程序執行中';
        el.querySelector('.ready').textContent = active.status === 'ready' ? '✓ Ready' : active.status === 'terminating' ? '退出端點' : '○ NotReady';
      }
    }
    $('endpoints').innerHTML = e.endpoints.length ? e.endpoints.map(id=>`<b>${id}</b>`).join('') : '<span class="muted">尚無可用端點</span>';
    $('forward-status').textContent = e.forward ? `已連線 → Pod ${e.forward}:8080` : '轉送未連線';
    $('client-action').textContent = e.forward ? 'port-forward 固定連一個 Pod' : 'kubectl / make';
    $('response').textContent = e.result || (e.forward ? `本機通道：Pod ${e.forward}` : '尚無 HTTP 回應');
    $('probe').textContent = e.probe || 'kubelet 持續檢查容器狀態'; $('probe').classList.toggle('bad',e.probe.includes('503'));
    $('observed').textContent = e.observed || '等待期望狀態達成';
    $('counts').textContent = `${e.pods.length} 個 Pod · ${e.endpoints.length} Ready / 期望 ${e.replicas}`;
    document.querySelectorAll('.focus').forEach(el=>el.classList.remove('focus'));
    if(e.focus && $(e.focus)) $(e.focus).classList.add('focus');
    $('event-title').textContent = e.title; $('why').textContent = e.why;
    $('beat-label').textContent = `分鏡 ${eventIndex+1} / ${story[stepIndex].events.length}`;
    $('seek').setAttribute('aria-valuetext',`第 ${stepIndex+1} 步，${e.title}`);
    document.querySelectorAll('[data-beat]').forEach((b,i)=>b.setAttribute('aria-current',String(i===eventIndex)));
    route();
  }
  function route() {
    if (!current || current.mode !== 'cluster' || !current.signal || cleaned) { $('connections').style.visibility='hidden';return; }
    let from, to;
    if(current.signal==='submit'){from=$('client');to=$('deployment');}
    if(current.signal==='create'){from=$('rs');to=$(`pod-${current.pods.find(p=>p.status==='pending'||p.status==='starting')?.id}`);}
    if(current.signal==='probe'){from=$('kubelet');to=$(`pod-${current.probe[0]}`);}
    if(current.signal==='request'){from=$('client');to=$(`pod-${current.forward}`);}
    if(!from||!to){$('connections').style.visibility='hidden';return;}
    const box=$('stage').getBoundingClientRect(), a=from.getBoundingClientRect(), b=to.getBoundingClientRect();
    let x1=a.right-box.left,y1=a.top+a.height/2-box.top,x2=b.left-box.left,y2=b.top+b.height/2-box.top;
    if(current.signal==='probe'||innerWidth<=540&&b.top>a.bottom){x1=a.left+a.width/2-box.left;y1=a.bottom-box.top;x2=b.left+b.width/2-box.left;y2=b.top-box.top;}
    const bend=Math.max(18,Math.abs(x2-x1)*.45);
    let d=`M${x1},${y1} C${x1+bend},${y1} ${x2-bend},${y2} ${x2},${y2}`;
    if(current.signal==='probe') {
      const outer=document.querySelector('.worker-zone').getBoundingClientRect().right-box.left-5;
      x1=a.right-box.left;y1=a.top+a.height/2-box.top;x2=b.right-box.left;y2=b.top+b.height/2-box.top;
      d=`M${x1},${y1} L${outer},${y1} L${outer},${y2} L${x2},${y2}`;
    } else if(current.signal==='request') {
      const zones=document.querySelector('.zones').getBoundingClientRect();
      const left=zones.left-box.left+5,right=zones.right-box.left-5,bottom=zones.bottom-box.top-5;
      x1=a.left-box.left;y1=a.top+a.height/2-box.top;x2=b.right-box.left;y2=b.top+b.height/2-box.top;
      d=`M${x1},${y1} L${left},${y1} L${left},${bottom} L${right},${bottom} L${right},${y2} L${x2},${y2}`;
    } else if(innerWidth<=540&&b.top>a.bottom) {
      d=`M${x1},${y1} C${x1},${y1+16} ${x2},${y2-16} ${x2},${y2}`;
    }
    $('connections').style.visibility='visible'; $('route').setAttribute('d',d); $('route').classList.toggle('control',current.signal!=='request'); movePacket();
  }
  function movePacket() {
    if(!current||!current.signal||current.mode!=='cluster'||cleaned)return;
    const route=$('route'); if(!route.getAttribute('d'))return;
    const local=time-eventTime(eventIndex), phase=reduced?.5:Math.min(1,Math.max(0,(local%4)/2.6));
    const point=route.getPointAtLength(route.getTotalLength()*phase);
    $('packet').setAttribute('cx',point.x);$('packet').setAttribute('cy',point.y);
  }
  function tick(now) {
    frame=0;if(!playing||document.hidden)return;
    if(last)time+=Math.min((now-last)/1000,.1)*speed;last=now;
    if(time>=duration()) {
      time=duration();draw();
      if($('continuous').checked&&stepIndex<story.length-1){selectStep(stepIndex+1);play();return;}
      stop();$('play-hint').textContent=stepIndex===11?'M0 播放完成。你可以重看任一段。':'這一步已完成。確認看懂後，按「下一步」。';return;
    }
    draw();frame=requestAnimationFrame(tick);
  }
  function seek(t){stop();toggleCleanup(false);time=Math.min(duration(),Math.max(0,t));draw();}
  function toggleCleanup(value) {
    cleaned=value;
    document.querySelector('.zones').hidden=value;
    document.querySelector('.cluster-summary').hidden=value;
    $('cleanup-result').hidden=!value;
    $('cleanup').textContent=value?'回到保留叢集的畫面':'查看清理後的畫面';
    if(value){$('connections').style.visibility='hidden';$('stage').classList.add('cleaned');}
    else {$('stage').classList.remove('cleaned');route();}
  }
  $('steps').addEventListener('click',e=>{const b=e.target.closest('[data-step]');if(b){selectStep(+b.dataset.step);if(innerWidth<800)document.querySelector('.lesson').scrollIntoView({block:'start'});}});
  $('beats').addEventListener('click',e=>{const b=e.target.closest('[data-beat]');if(b)seek(eventTime(+b.dataset.beat));});
  $('play').onclick=()=>playing?stop():play();
  $('replay').onclick=()=>{seek(0);play();};
  $('next').onclick=()=>selectStep(stepIndex+1);$('previous').onclick=()=>selectStep(stepIndex-1);
  $('seek').oninput=e=>seek(+e.target.value);$('speed').onchange=e=>{speed=+e.target.value;};
  $('theme').onclick=()=>{document.body.classList.toggle('dark');$('theme').textContent=document.body.classList.contains('dark')?'淺色':'深色';};
  $('cleanup').onclick=()=>{stop();toggleCleanup(!cleaned);};
  document.addEventListener('keydown',e=>{if(e.altKey||e.ctrlKey||e.metaKey||e.target.closest('input,select,button,a,summary,textarea'))return;if(e.code==='Space'){e.preventDefault();playing?stop():play();}if(e.key==='ArrowRight'){e.preventDefault();selectStep(stepIndex+1);}if(e.key==='ArrowLeft'){e.preventDefault();selectStep(stepIndex-1);}});
  document.addEventListener('visibilitychange',()=>{last=0;if(document.hidden){cancelAnimationFrame(frame);frame=0;}else if(playing)frame=requestAnimationFrame(tick);});
  addEventListener('resize',route);
  new ResizeObserver(route).observe($('stage'));
  const motionButton=document.createElement('button');motionButton.id='motion';motionButton.className='motion-toggle';$('play-hint').after(motionButton);
  function applyMotion(value=motionPreference.matches){reduced=value;document.body.classList.toggle('reduced',reduced);$('play-hint').textContent=reduced?'已減少位移與轉場效果；按「播放這一步」仍會依序播放分鏡。':'每一步結束會停住，確認看懂後再前進。Space 播放／暫停 · ← → 切換步驟';motionButton.textContent=reduced?'完整動態效果':'減少動態';motionButton.setAttribute('aria-pressed',String(reduced));updatePlay();}
  motionButton.onclick=()=>applyMotion(!reduced);
  motionPreference.addEventListener('change',()=>applyMotion());
  addEventListener('hashchange',()=>{const match=location.hash.match(/^#step-(\d+)$/);if(match)selectStep(+match[1]-1,false);});
  const initial=location.hash.match(/^#step-(\d+)$/);selectStep(initial?+initial[1]-1:0,false);applyMotion();
})();
