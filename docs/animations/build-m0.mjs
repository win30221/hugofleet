import { readFileSync, writeFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { story, totalSeconds, validateStory } from './m0-story.mjs';
validateStory();
const read = file => readFileSync(new URL(file, import.meta.url),'utf8').replace(/\r\n/g,'\n');
const output = read('m0-page.html').replace('/* STYLE */',()=>read('m0.css')).replace('/* DATA */',()=>`const story = ${JSON.stringify(story).replace(/</g,'\\u003c')};`).replace('/* SCRIPT */',()=>read('m0-player.js'));
if(process.argv.includes('--check')){
  if(output!==read('m0.html'))throw Error('m0.html is stale');
  console.log('PASS M0 generated page, stable Pod identity, surge safety and final restoration');
}else writeFileSync(fileURLToPath(new URL('m0.html',import.meta.url)),output);
console.log(`M0: ${story.length} steps / ${story.reduce((n,s)=>n+s.events.length,0)} scenes / ${totalSeconds}s`);
