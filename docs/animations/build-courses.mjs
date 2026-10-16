import {readFileSync,writeFileSync} from 'node:fs';
import {courses} from './course-story.mjs';
const read=f=>readFileSync(new URL(f,import.meta.url),'utf8').replace(/\r\n/g,'\n');
const esc=s=>String(s).replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
for(const course of courses){
 const ids=new Map(),nodes=new Set(course.nodes.map(n=>n[0]));let events=0,seconds=0;
 for(const step of course.steps){if(!step.question||!step.action||!step.events.length)throw Error('Incomplete step');for(const e of step.events){
  events++;seconds+=e.seconds;
  if(!e.title||!e.why||!(e.seconds>0))throw Error('Incomplete event');
  for(const [x,y]of e.flow)if(!nodes.has(x)||!nodes.has(y))throw Error(`Unknown route ${x}/${y}`);
  for(const [i,w]of e.workers.entries()){
   if(new Set(w.pods.map(p=>p.slot)).size!==w.pods.length)throw Error('Overlapping slots');
   for(const p of w.pods){const key=`${i}-${p.id}`;if(ids.has(key)&&ids.get(key)!==`${p.image}/${p.rev}`)throw Error('Pod mutated in place');ids.set(key,`${p.image}/${p.rev}`);if(p.slot<0||p.slot>2||!['ready','starting','unready','terminating'].includes(p.status))throw Error('Invalid Pod');if(p.image==='v2-broken'&&p.status==='ready')throw Error('Broken Pod Ready');}
   if(w.pods.some(p=>p.image==='v2-broken')&&w.pods.filter(p=>p.status==='ready').length!==2)throw Error('Broken rollout must preserve two healthy Pods');
  }
  if(e.badge.includes('配額')&&e.workers.some(w=>w.pods.length!==2||w.pods.some(p=>p.status!=='ready')))throw Error('Admission rejection created a Pod');
  if(e.records.some(r=>r.rev===4)){const source=e.records.find(r=>r.rev===2),back=e.records.find(r=>r.rev===4);if(!source||back.image!==source.image||back.replicas!==source.replicas||e.records.find(r=>r.rev===3)?.status!=='FAILED')throw Error('Rollback changed source or erased failure');}
 }}
 const nav=Array.from({length:6},(_,i)=>`<a href="m${i}.html"${course.id===`m${i}`?' aria-current="page"':''}>M${i}</a>`).join('');
 const output=read('course-page.html').replace('<!-- TITLE -->',()=>esc(`${course.id.toUpperCase()} ${course.name}`)).replace('<!-- NAV -->',()=>nav).replace('/* STYLE */',()=>read('m0.css')+'\n'+read('course.css')).replace('/* DATA */',()=>`const course=${JSON.stringify(course).replace(/</g,'\\u003c')};`).replace('/* SCRIPT */',()=>read('course-player.js'));
 const file=`${course.id}.html`;
 if(process.argv.includes('--check')){if(output!==read(file))throw Error(`${file} is stale`);}else writeFileSync(new URL(file,import.meta.url),output);
 console.log(`PASS ${course.id.toUpperCase()}: ${course.steps.length} steps / ${events} scenes / ${seconds}s; identity, surge, rejection, rollback validated`);
}
