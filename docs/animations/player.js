(() => {
  "use strict";
  const $ = (id) => document.getElementById(id);
  const escapeHTML = (value) => String(value).replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c]);
  const formatTime = (seconds) => `${String(Math.floor(seconds / 60)).padStart(2, "0")}:${String(Math.floor(seconds % 60)).padStart(2, "0")}`;
  const reducedPreference = matchMedia("(prefers-reduced-motion: reduce)");
  let reduced = reducedPreference.matches, chapterIndex = 0, seconds = 0, speed = 1;
  let playing = !reduced, frame = 0, last = 0, onScreen = true, renderedStep = -1, renderedEvent = "", refs = {}, chapter;
  let copyTimeout = 0;
  const duration = () => chapterSeconds(chapter);

  function setPlaying(value) {
    playing = Boolean(value) && !reduced;
    $("play-text").textContent = playing ? "暫停" : "播放";
    $("play-icon").textContent = playing ? "Ⅱ" : "▶";
    $("play").setAttribute("aria-label", playing ? "暫停動畫" : "播放動畫");
    $("play").disabled = reduced;
    if (playing) run(); else stopLoop();
  }
  function stopLoop() { if (frame) cancelAnimationFrame(frame); frame = 0; last = 0; }
  function setReduced(value) {
    reduced = Boolean(value);
    $("motion").textContent = reduced ? "恢復動態" : "減少動態";
    $("motion").setAttribute("aria-pressed", String(reduced));
    setPlaying(false);
    draw();
  }
  function setTheme(value) {
    const theme = value === "dark" ? "dark" : "light";
    document.body.dataset.theme = theme;
    document.querySelector(".machine-card").dataset.theme = theme;
    $("theme").textContent = theme === "light" ? "深色模式 ◐" : "淺色模式 ◑";
    $("theme").setAttribute("aria-label", theme === "light" ? "切換深色模式" : "切換淺色模式");
  }
  function selectChapter(index, updateHash = true) {
    if (index === 0) { location.replace("m0.html"); return; }
    chapterIndex = clamp(index, 0, chapters.length - 1);
    chapter = chapters[chapterIndex]; seconds = 0; renderedStep = -1; renderedEvent = "";
    document.title = `HugoFleet · ${chapter.id.toUpperCase()} ${chapter.short}`;
    $("chapter-title").textContent = chapter.name;
    $("chapter-intro").textContent = chapter.intro;
    $("purpose").textContent = chapter.purpose;
    $("problem").textContent = chapter.problem;
    $("responsibility").textContent = chapter.responsibility;
    $("question").textContent = `學完應能回答：${chapter.question}`;
    $("chapter-number").textContent = String(chapterIndex).padStart(2, "0");
    $("chapter-eyebrow").textContent = `${String(chapterIndex + 1).padStart(2, "0")} / 06 · ${chapter.english}`;
    $("figure-number").textContent = `FIG. ${String(chapterIndex + 1).padStart(2, "0")}`;
    $("figure-subject").textContent = chapter.english;
    $("legend").innerHTML = chapter.legend.map((label) => `<span><i aria-hidden="true"></i>${escapeHTML(label)}</span>`).join("");
    $("machine").innerHTML = scenes[chapterIndex].svg;
    $("machine").querySelector("svg").setAttribute("aria-label", chapter.metaphor + "。流程示意，非即時狀態。");
    refs = Object.fromEntries([...$("machine").querySelectorAll("[data-part]")].map((el) => [el.dataset.part, el]));
    $("steps").innerHTML = chapter.steps.map((step, i) => `<li><button class="step-button" type="button" data-step="${i}" aria-label="第 ${i + 1} 步：${escapeHTML(step.title)}"><span class="step-index">${String(i + 1).padStart(2, "0")}</span><span><strong>${escapeHTML(step.title)}</strong><small>${formatTime(stepStart(chapter, i))} — ${formatTime(stepStart(chapter, i) + stepSeconds(step))}</small></span></button></li>`).join("");
    $("tutorial").href = chapter.tutorial;
    $("boundary").textContent = chapter.note;
    $("setup").textContent = chapter.setup;
    $("stop-command").textContent = chapter.stop;
    $("seek").max = duration();
    $("duration").textContent = formatTime(duration());
    document.querySelectorAll(".chapter-tab").forEach((button, i) => { if (i === chapterIndex) button.setAttribute("aria-current", "page"); else button.removeAttribute("aria-current"); });
    if (updateHash && location.hash !== `#${chapter.id}`) location.hash = chapter.id;
    draw();
  }
  function draw() {
    const model = sample(chapter, seconds), { state, index, step } = model;
    if (renderedStep !== index) {
      renderedStep = index;
      $("step-counter").textContent = `${String(index + 1).padStart(2, "0")} / ${String(chapter.steps.length).padStart(2, "0")}`;
      $("step-label").textContent = `STEP ${String(index + 1).padStart(2, "0")} / ${chapter.id.toUpperCase()}`;
      $("step-title").textContent = step.title;
      $("step-description").textContent = step.text;
      $("expected").textContent = step.expected;
      $("action-location").textContent = step.action.where;
      $("action-code").textContent = step.action.code;
      $("action-kind").textContent = step.action.kind === "shell" ? "TERMINAL / BASH" : "BROWSER / 操作指引";
      $("copy").textContent = "複製";
      $("events").innerHTML = (step.sequence || []).map((event, i) => `<button type="button" data-event="${i}" aria-label="變化 ${i + 1}：${escapeHTML(event.title)}">${String(i + 1).padStart(2, "0")}</button>`).join("");
      document.querySelectorAll(".step-button").forEach((button, i) => { if (i === index) button.setAttribute("aria-current", "step"); else button.removeAttribute("aria-current"); button.classList.toggle("is-complete", i < index); });
      Object.values(refs).forEach((el) => el.classList.remove("is-focus"));
      refs[state.focus]?.classList.add("is-focus");
      $("announcement").textContent = `${chapter.id.toUpperCase()}，第 ${index + 1} 步：${step.title}。${step.expected}`;
      $("seek").setAttribute("aria-valuetext", `第 ${index + 1} 步，${step.title}`);
    }
    const eventKey = `${index}:${model.eventIndex}`;
    if (renderedEvent !== eventKey) {
      renderedEvent = eventKey;
      $("readouts").innerHTML = state.metrics.map((entry) => { const [label, ...value] = entry.split("|"); return `<div class="readout"><span>${escapeHTML(label)} · 示意</span><strong>${escapeHTML(value.join("|"))}</strong></div>`; }).join("");
      $("event-title").textContent = model.event.title || step.title;
      $("event-why").textContent = model.event.why || step.text;
      $("events").querySelectorAll("button").forEach((b, i) => b.setAttribute("aria-pressed", String(i === model.eventIndex)));
      const aPods = state.aPods || [], bPods = state.bPods || [];
      $("replica-contract").textContent = chapterIndex === 0 || chapterIndex === 1 || chapterIndex === 2 || chapterIndex === 5 ? "RollingUpdate · maxSurge 1 / maxUnavailable 0" : "Hub 觀測與 Worker 執行分開閱讀";
      const crText = chapterIndex === 0 ? "此課尚無" : state.crState || (aPods.length ? `期望副本 ${state.crTarget ?? state.targetReplicas ?? state.desired}` : "尚未提交／尚未建立");
      const depText = state.deploymentState || (aPods.length ? `${state.deploymentTarget ?? state.targetReplicas ?? state.desired} 副本` : "尚未建立");
      $("control-facts").innerHTML = `<span>Application CR：${escapeHTML(crText)}</span><span>Deployment spec：${escapeHTML(depText)}</span><span>當地工作負載：${aPods.filter(p => p.ready).length} Ready / ${aPods.length} 個 Pod</span><span>Hub：${chapterIndex < 2 ? "此課尚無" : state.hubRejected ? "授權拒絕；未建立新意圖" : state.connected === false ? "UNKNOWN；只保留最後觀測" : state.operation || "以 Snapshot／完整 rollout 核對"}</span>`;
      const renderWorker = (name, pods) => {
        const groups = new Map();
        for (const p of pods) { const key = `${p.revision}:${p.image}`; if (!groups.has(key)) groups.set(key, []); groups.get(key).push(p); }
        return `<div class="worker-detail"><h3>${name}</h3>${pods.length ? [...groups.values()].map(group => `<div class="rs-group"><span class="rs-name">ReplicaSet · ${escapeHTML(group[0].image)} <small>template ${escapeHTML(group[0].revision)}</small></span><div class="pod-list">${group.map(p => `<div class="pod-token ${p.ready ? "ready" : "not-ready"}" data-pod-id="${escapeHTML(p.id)}"><strong>${escapeHTML(p.id)} · ${escapeHTML(p.image)}</strong><span>phase=${escapeHTML(p.phase)} · Ready=${p.ready ? "True" : "False"}</span><small>${p.ready ? "可用副本／一般就緒 endpoint" : p.image === "v2-broken" ? "healthz 200 · readyz 503 · 不接一般 Service 流量" : "等待 readiness；尚未進入一般就緒 endpoints"}</small></div>`).join("")}</div></div>`).join("") : '<p class="empty-pods">此快照沒有業務 Pod。</p>'}</div>`;
      };
      $("pod-lineage").innerHTML = renderWorker(chapterIndex >= 3 ? "Worker A" : "本課叢集", aPods) + (chapterIndex === 3 ? renderWorker("Worker B", bPods) : "");
      $("pod-conclusion").textContent = state.quotaDenied ? "Quota 拒絕的是新 Pod 的 CREATE：沒有新 Pod 物件。不要把它畫成已存在的壞 Pod。" : state.hubRejected ? "Hub 授權先拒絕：既有工作負載不變，不新增 Operation、CR revision 或 Pod。" : aPods.some(p => p.image === "v2-broken") ? "舊健康 Pod 與新壞版 Pod 同時存在。新 Pod 的 phase 仍為 Running；readiness=False，rollout 才會失敗／逾時。" : "每張卡匣對應一個獨立 Pod；換版本透過建立新 Pod 與縮減舊 ReplicaSet，不會原地把舊 Pod 改成新映像。";
      Object.values(refs).forEach((el) => el.classList.remove("is-focus"));
      refs[state.focus]?.classList.add("is-focus");
    }
    const progress = seconds / duration() * 100;
    $("seek").value = seconds;
    $("seek").style.setProperty("--progress", `${progress}%`);
    $("elapsed").textContent = formatTime(seconds);
    $("phase").textContent = model.event.title || state.phase;
    $("previous").disabled = index === 0 && seconds === 0;
    $("next").disabled = index === chapter.steps.length - 1 && chapterIndex === chapters.length - 1;
    const translate = (name, x, y, z = 0) => {
      const el = refs[name]; if (!el) return;
      const a = projection.azimuth * Math.PI / 180, e = projection.elevation * Math.PI / 180, scale = projection.scale * Math.sqrt(1.6);
      const dx = (x * Math.sin(a) - y * Math.cos(a)) * scale;
      const dy = ((x * Math.cos(a) + y * Math.sin(a)) * Math.sin(e) - z * Math.cos(e)) * scale;
      el.setAttribute("transform", `translate(${dx.toFixed(3)} ${dy.toFixed(3)})`);
    };
    // Snapshot changes are discrete. Only the operation shuttle and scanner move
    // within the step; displayed replica/revision values always agree with slots.
    const delivery = reduced ? 1 : model.delivery;
    const blocked = state.gate === false && chapterIndex === 4;
    translate("shuttle", (blocked ? 57 : chapterIndex >= 3 ? 295 : 230) * delivery, 0);
    translate("link", state.connected ? 0 : 10, 0);
    translate("barrier", 0, 0, state.gate ? 32 : 0);
    translate("scanner-a", reduced ? 0 : model.scan, 0);
    translate("receipt", 0, chapterIndex >= 2 ? -3 * ease(model.within / 0.3) : 0);
    for (const worker of ["a", "b"]) {
      const pods = worker === "a" ? state.aPods || [] : state.bPods || [];
      for (let i = 0; i < 3; i++) {
        const el = refs[`pod-${worker}-${i}`]; if (!el) continue;
        const p = pods.find(p => p.slot === i);
        el.style.opacity = p ? "1" : "0";
        el.dataset.failed = String(Boolean(p && !p.ready));
        el.dataset.podId = p?.id || "";
        el.dataset.image = p?.image || "";
      }
    }
  }
  function tick(now) {
    frame = 0;
    if (!playing || !onScreen || document.hidden || reduced) { last = 0; return; }
    const dt = last ? Math.min((now - last) / 1000, 0.1) : 0; last = now;
    seconds += dt * speed;
    if (seconds >= duration()) {
      seconds = duration(); draw();
      if ($("continuous").checked && chapterIndex < chapters.length - 1) selectChapter(chapterIndex + 1);
      else { setPlaying(false); return; }
    } else draw();
    frame = requestAnimationFrame(tick);
  }
  function run() { if (!frame && playing && onScreen && !document.hidden && !reduced) { last = 0; frame = requestAnimationFrame(tick); } }
  function seek(value, pause = true) { if (pause) setPlaying(false); seconds = clamp(value, 0, duration()); draw(); }
  function nextStep() {
    const i = sample(chapter, seconds).index;
    if (i === chapter.steps.length - 1 && chapterIndex < chapters.length - 1) { setPlaying(false); selectChapter(chapterIndex + 1); }
    else seek(stepStart(chapter, i + 1));
  }
  function previousStep() { seek(stepStart(chapter, Math.max(0, sample(chapter, seconds).index - 1))); }

  $("chapters").addEventListener("click", (event) => { const button = event.target.closest("[data-chapter]"); if (button) { setPlaying(false); selectChapter(Number(button.dataset.chapter)); } });
  $("steps").addEventListener("click", (event) => { const button = event.target.closest("[data-step]"); if (button) seek(stepStart(chapter, Number(button.dataset.step))); });
  $("events").addEventListener("click", (event) => { const button = event.target.closest("[data-event]"); if (button) { const i = sample(chapter, seconds).index; seek(stepStart(chapter, i) + chapter.steps[i].sequence[Number(button.dataset.event)].at * stepSeconds(chapter.steps[i])); } });
  $("play").addEventListener("click", () => { if (seconds >= duration()) seek(0); setPlaying(!playing); });
  $("previous").addEventListener("click", previousStep);
  $("next").addEventListener("click", nextStep);
  $("restart").addEventListener("click", () => { seek(0); setPlaying(true); });
  $("seek").addEventListener("input", (event) => seek(Number(event.target.value)));
  $("speed").addEventListener("change", (event) => { speed = Number(event.target.value); });
  $("motion").addEventListener("click", () => setReduced(!reduced));
  reducedPreference.addEventListener("change", (event) => setReduced(event.matches));
  $("theme").addEventListener("click", () => setTheme(document.body.dataset.theme === "light" ? "dark" : "light"));
  document.addEventListener("visibilitychange", () => { if (document.hidden) stopLoop(); else run(); });
  new IntersectionObserver((entries) => { onScreen = entries[entries.length - 1].isIntersecting; if (onScreen) run(); else stopLoop(); }, { rootMargin: "120px 0px" }).observe($("player"));
  window.addEventListener("hashchange", () => { const index = chapters.findIndex((c) => `#${c.id}` === location.hash); if (index >= 0 && index !== chapterIndex) { setPlaying(false); selectChapter(index, false); } });
  document.addEventListener("keydown", (event) => {
    if (event.altKey || event.ctrlKey || event.metaKey || event.target.closest("input,select,textarea,button,a,summary,[contenteditable]")) return;
    if (event.code === "Space") { event.preventDefault(); if (seconds >= duration()) seek(0); setPlaying(!playing); }
    else if (event.key === "ArrowRight") { event.preventDefault(); nextStep(); }
    else if (event.key === "ArrowLeft") { event.preventDefault(); previousStep(); }
    else if (event.key === "Home") { event.preventDefault(); seek(0); }
    else if (event.key === "End") { event.preventDefault(); seek(duration()); }
  });
  $("copy").addEventListener("click", async () => {
    const text = sample(chapter, seconds).step.action.code;
    clearTimeout(copyTimeout);
    try {
      if (navigator.clipboard && window.isSecureContext) await navigator.clipboard.writeText(text);
      else {
        const range = document.createRange(); range.selectNodeContents($("action-code"));
        const selection = window.getSelection(); selection.removeAllRanges(); selection.addRange(range);
        $("copy").textContent = "已選取，請複製"; return;
      }
      $("copy").textContent = "已複製";
    } catch { $("copy").textContent = "請手動選取複製"; }
    copyTimeout = setTimeout(() => { $("copy").textContent = "複製"; }, 1800);
  });
  const initial = chapters.findIndex((c) => `#${c.id}` === location.hash);
  if (initial === 0) { location.replace("m0.html"); return; }
  selectChapter(initial < 0 ? 1 : initial, false);
  $("total-duration").textContent = formatTime(chapters.reduce((sum, c) => sum + chapterSeconds(c), 0));
  $("motion").textContent = reduced ? "恢復動態" : "減少動態";
  $("motion").setAttribute("aria-pressed", String(reduced));
  setPlaying(playing);
})();
