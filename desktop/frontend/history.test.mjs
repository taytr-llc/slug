// Presentation tests against the independently frozen preimplementation corpus.
// Native crypto, source proofs and original-session authorization are separate gates.
import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';
import {readFileSync} from 'node:fs';
import test from 'node:test';
import vm from 'node:vm';
const bytes=readFileSync(new URL('../../gymstable/testdata/career-history-reference/corpus.json',import.meta.url));
assert.equal(createHash('sha256').update(bytes).digest('hex'),'18e51e25c6760f756159e12a92265b517438152533d439d541d4696044e60283');
const reference=JSON.parse(bytes),clone=x=>JSON.parse(JSON.stringify(x));
const source=readFileSync(new URL('./stable.js',import.meta.url),'utf8'),html=readFileSync(new URL('./stable.html',import.meta.url),'utf8');
const script=source.slice("import { Call } from '/wails/runtime.js';".length,source.indexOf("for (const kind of ['publish', 'reconcile', 'attest', 'export']) ui[`result-${kind}`].addEventListener"));
const dto=id=>clone(reference.wire.dtos.find(x=>x.id===id).value), history=name=>reference.history.histories.find(x=>x.name===name);
class Element {
  constructor(tag='div'){this.tagName=tag;this.children=[];this.listeners=new Map();this.attributes=new Map();this._text='';this.disabled=false;}
  set textContent(v){this._text=String(v);this.children=[];}get textContent(){return this._text+this.children.map(c=>c.textContent).join('');}
  set innerHTML(_){throw Error('Unsafe HTML');}append(...nodes){this.children.push(...nodes);}setAttribute(k,v){this.attributes.set(k,String(v));}removeAttribute(k){this.attributes.delete(k);}addEventListener(k,v){this.listeners.set(k,v);}
}
function harness(name='genuine_win'){
  const c=clone(history(name).expected.capture),j=dto('job/done');j.capture=c;j.store_id=c.status.store_id;j.activation_id=c.activation.activation_id;j.prospect_id=c.assignment.prospect_id;
  Object.assign(j.view,{account_id:c.status.pin.account_id,account_authority_id:c.status.pin.account_authority_id,authority_pin:clone(c.status.pin.source)});
  const s={...dto('snapshot/ready'),view:clone(j.view),store_id:j.store_id,activation:clone(c.activation),last_job_id:j.job_id};
  const elements=new Map([...html.matchAll(/\bid="([^"]+)"/g)].map(m=>[m[1],new Element()])),calls=[],handlers={};
  const context=vm.createContext({document:{getElementById:id=>elements.get(id),createElement:tag=>new Element(tag),hidden:false},window:{matchMedia:()=>({matches:false,addEventListener(){}})},Call:{async ByName(name,...args){const method=name.slice(9);calls.push({method,args:clone(args)});assert.ok(handlers[method],`Unexpected ${method}`);return clone(await handlers[method](...args));}},Intl,Date,btoa:s=>Buffer.from(s).toString('base64'),setTimeout(){return 1;},clearTimeout(){}});
  vm.runInContext(script,context);const run=s=>vm.runInContext(s,context),set=(name,value)=>{context.input=clone(value);run(`${name}=input`);};
  set('current',{account:{state:'signed_in',fixture:c.status.pin.identity.gym_id==='a227587d837507dbcb5f9d781967339d816070fb7bd574931c690ba6937b88e5'?'zero256':'incrementing256'},view:j.view});set('stable',{state:'ready',view:j.view,status:c.status});set('careerMode',true);set('careerActivation',c.activation);set('historySnapshot',s);set('historyJob',j);set('historySelection',{id:j.prospect_id,name:'Captured fighter',assignment:c.assignment});
  const response=(kind,value)=>({id:1,version:1,ok:true,snapshot:null,job:null,page:null,error:'',[kind]:clone(value)});
  const error=code=>({id:1,version:1,ok:false,snapshot:null,job:null,page:null,error:code});
  return {run,set,context,elements,c,j,s,calls,handlers,response,error};
}
test('all independent captures and page boundaries retain chronological totals in newest-first presentation',()=>{
  let pages=0;for(const item of reference.history.histories){const h=harness(item.name);h.run('historyCapture(historyJob.capture)');for(const p of item.expected.pages){h.context.input=clone(p.value);h.context.after=p.after;h.context.limit=p.limit;h.run('historyPageValue(input,after,limit)');pages++;}}assert.equal(pages,80);
});
test('whole Capture, exact requested length and older-page totals cannot be swapped',()=>{
  const h=harness('count_256'),p=clone(history('count_256').expected.pages.find(x=>x.after>0&&x.value.items.length).value);h.context.after=p.after;h.context.limit=8;h.context.input=clone(p);h.run('historyPageValue(input,after,limit)');
  for(const change of [v=>v.capture.status.observed_source_count++,v=>v.items.pop(),v=>v.items[0].totals_after=clone(v.capture.totals),v=>v.items[0].xp_applied=1,v=>v.after++]){const bad=clone(p);change(bad);h.context.input=bad;assert.throws(()=>h.run('historyPageValue(input,after,limit)'));}
});
test('empty history and XP cap render distinctly from provisional fights and rankings',()=>{
  const h=harness('empty');h.set('historyPage',history('empty').expected.pages[0].value);h.run('renderHistory()');assert.match(h.elements.get('history-list').textContent,/No finalized fights/);assert.match(h.elements.get('history-result').textContent,/Current status not checked/);
  const max=harness('count_256');max.set('historyPage',history('count_256').expected.pages[0].value);max.run('renderHistory()');assert.match(max.elements.get('history-list').textContent,/Earned XP 1 · Applied XP 0/);assert.match(max.elements.get('history-totals').textContent,/Base points \(not ranking\)/);assert.equal(max.calls.length,0);
});
test('typed history envelope rejects missing/null arms, terminal mutation and foreign full View',()=>{
  const h=harness();h.context.input=h.response('job',h.j);h.run("historyEnvelope(input,'job')");
  for(const change of [v=>delete v.page,v=>v.page={},v=>v.job.view.transport_trust_id='f'.repeat(64),v=>{v.job.capture.as_of_ms++;v.job.capture.status.last_as_of_ms++;},v=>{v.job.state='running';v.job.capture=null;},v=>v.job.error='made_up']){const bad=h.response('job',h.j);change(bad);h.context.input=bad;assert.throws(()=>h.run("historyEnvelope(input,'job')"));}
});
test('page requests reauthorize each time; a failed read retains only the previous dated page',async()=>{
  const h=harness(),p=history('genuine_win').expected.pages[0].value;h.handlers.CareerHistoryPage=()=>h.response('page',{view:h.j.view,store_id:h.j.store_id,job_id:h.j.job_id,page:p});
  await h.run('readHistoryPage(0)');await h.run('readHistoryPage(0)');assert.equal(h.calls.length,2);assert.ok(h.calls.every(c=>c.method==='CareerHistoryPage'));assert.equal(h.run('historyPage.items.length'),1);
  h.handlers.CareerHistoryPage=()=>{const bad=clone(p);bad.items[0].earned.xp=0;return h.response('page',{view:h.j.view,store_id:h.j.store_id,job_id:h.j.job_id,page:bad});};await h.run('readHistoryPage(0)');assert.equal(h.run('historyPage.items[0].earned.xp'),1);assert.match(h.elements.get('history-message').textContent,/previous captured page/);
});
test('late page cannot restore a cleared selection or survive original View loss',async()=>{
  for(const invalidate of ['clearHistory()','epoch++']){const h=harness();let release;h.handlers.CareerHistoryPage=()=>new Promise(r=>{release=()=>r(h.response('page',{view:h.j.view,store_id:h.j.store_id,job_id:h.j.job_id,page:history('genuine_win').expected.pages[0].value}));});const pending=h.run('readHistoryPage(0)');h.run(invalidate);release();await pending;assert.equal(h.run('historyPage'),null);}
});
test('known observation failure clears pageable history; busy preserves it',async()=>{
  const h=harness(),p=history('genuine_win').expected.pages[0].value;h.set('historyPage',p);h.handlers.CareerHistoryPage=()=>h.error('busy');await h.run('readHistoryPage(0)');assert.equal(h.run('historyPage.items.length'),1);h.handlers.CareerHistoryPage=()=>h.error('history_observation_unknown');await h.run('readHistoryPage(0)');assert.equal(h.run('historyPage'),null);assert.equal(h.run('historyJob'),null);
});
test('unknown begin is reconciled from Snapshot without automatic repeat',async()=>{
  const h=harness();h.set('historyJob',null);h.set('historyUnknown',true);h.handlers.CareerHistorySnapshot=()=>h.response('snapshot',h.s);h.handlers.CareerHistoryJob=()=>h.response('job',h.j);h.handlers.CareerHistoryPage=()=>h.response('page',{view:h.j.view,store_id:h.j.store_id,job_id:h.j.job_id,page:history('genuine_win').expected.pages[0].value});await h.run('fetchHistorySnapshot()');assert.deepEqual(h.calls.map(c=>c.method),['CareerHistorySnapshot','CareerHistoryJob','CareerHistoryPage']);assert.equal(h.run('historyUnknown'),false);assert.equal(h.run('historyPage.items.length'),1);
});
test('history cache is charged alongside every existing family and refuses page growth without eviction',async()=>{
  const h=harness(),p=history('genuine_win').expected.pages[0].value;h.set('historyPage',p);const original=h.run('JSON.stringify(historyPage)');h.run("pageItems=['x'.repeat(1048000)]");h.handlers.CareerHistoryPage=()=>h.response('page',{view:h.j.view,store_id:h.j.store_id,job_id:h.j.job_id,page:p});await h.run('readHistoryPage(0)');assert.equal(h.run('JSON.stringify(historyPage)'),original);assert.match(h.elements.get('history-message').textContent,/cache is full/);assert.equal(h.run('pageItems[0].length'),1048000);
});
test('clear and Stop invalidate history; full joint design review remains separate',()=>{
  const h=harness();h.set('historyPage',history('genuine_win').expected.pages[0].value);h.run('clearHistory();renderHistory()');assert.equal(h.run('historyPage'),null);assert.equal(h.run('historyJob'),null);assert.equal(h.run('historySelection'),null);assert.match(source,/method === 'Stop'\) \{ clearHistory\(\)/);assert.doesNotMatch(source,/\.innerHTML\s*=|crypto\.subtle/);
});

async function begin(h,handler){
  h.handlers.CareerHistorySnapshot=()=>h.response('snapshot',h.s);h.handlers.BeginCareerHistory=handler;
  h.run('enabled=true;background=Promise.resolve();render=()=>{}'); // Isolate this queued action from the already-tested polling scheduler.
  h.context.item={assignment:clone(h.c.assignment),manifest:{name:{display:'Chosen fighter'}}};await h.run('selectHistory(item)');await h.run('tasks.shift()()');
}
test('denied begin and local capacity refusal preserve the former capture',async()=>{
  for(const full of [false,true]){const h=harness(),p=history('genuine_win').expected.pages[0].value;h.set('historyPage',p);if(full)h.run("pageItems=['x'.repeat(1048000)]");await begin(h,()=>h.error('busy'));assert.equal(h.run('historyJob.job_id'),h.j.job_id);assert.equal(h.run('historyPage.capture.history_id'),p.capture.history_id);assert.equal(h.run('historyUnknown'),false);assert.equal(h.calls.filter(c=>c.method==='BeginCareerHistory').length,full?0:1);}
});
test('lost begin preserves former label and capture until the exact new job is reconciled',async()=>{
  const h=harness(),p=history('genuine_win').expected.pages[0].value;h.set('historyPage',p);await begin(h,()=>{throw Error('lost reply');});assert.equal(h.run('historyUnknown'),true);assert.equal(h.run('historySelection.name'),'Captured fighter');assert.equal(h.run('historyPage.capture.history_id'),p.capture.history_id);
  const j={...clone(h.j),job_id:'b'.repeat(64)},s={...clone(h.s),last_job_id:j.job_id};h.handlers.CareerHistorySnapshot=()=>h.response('snapshot',s);h.handlers.CareerHistoryJob=()=>h.response('job',j);h.handlers.CareerHistoryPage=()=>h.response('page',{view:j.view,store_id:j.store_id,job_id:j.job_id,page:p});await h.run('fetchHistorySnapshot()');assert.equal(h.run('historyUnknown'),false);assert.equal(h.run('historyJob.job_id'),j.job_id);assert.equal(h.run('historySelection.name'),'Chosen fighter');assert.equal(h.calls.filter(c=>c.method==='BeginCareerHistory').length,1);
});
test('failed explicit result read retains award and never starts a chain or publication action',async()=>{
  const h=harness(),p=history('genuine_win').expected.pages[0].value;h.set('historyPage',p);h.handlers.CareerHistoryResult=()=>({id:1,ok:false,view:null,store_id:'',snapshot:null,status:null,record:null,job:null,error:'result_not_found'});await h.run('historyReadResult(1)');assert.equal(h.run('historyPage.items.length'),1);assert.deepEqual(h.calls.map(c=>c.method),['CareerHistoryResult']);assert.equal(h.run('historyOverlay'),null);assert.match(h.elements.get('history-result').textContent,/Current status not checked/);
});

const linkBytes=readFileSync(new URL('./testdata/history-result-link.json',import.meta.url));
assert.equal(createHash('sha256').update(linkBytes).digest('hex'),'d52156952f224ff627f242858c4a118d811864e699a323b778f06cda56834eb2');
const linkInput=JSON.parse(linkBytes);
function resultLinkHarness(stage){
  const h=harness(),value=clone(linkInput[stage]),snap=clone(linkInput.snapshot),p=snap.pin,src=value.record.statement.statement.source,ep=p.source_pin.source_pin.source_pin.source_pin;
  snap.view=clone(h.j.view); // Fresh same-owner presentation binding, not a new source fact.
  h.set('historyPage',history('genuine_win').expected.pages[0].value);
  h.set('eventAuthority',{pin:ep,storeID:p.source_pin.source_pin.source_pin.source_store_id,createdMS:1});
  h.set('seedAuthority',{pin:p.source_pin.source_pin.source_pin,storeID:p.source_pin.source_pin.source_store_id,createdMS:1});
  h.set('cardAuthority',{pin:p.source_pin.source_pin,storeID:p.source_pin.source_store_id,createdMS:1});
  h.set('bookedAuthority',{pin:p.source_pin,storeID:p.source_store_id,sourceCreatedMS:1,createdMS:src.store_created_at_ms});
  h.set('bookedSnapshot',{version:1,state:'ready',error:'',view:h.j.view,store_id:p.source_store_id,pin:p.source_pin,source_created_at_ms:1,created_at_ms:src.store_created_at_ms,active_job_id:'',queued_job_id:'',last_job_id:''});
  h.set('publicationAuthority',{pin:p,storeID:snap.store_id,sourceCreatedMS:snap.source_created_at_ms,createdMS:snap.created_at_ms});h.set('publicationSnapshot',snap);
  h.handlers.CareerHistoryResult=()=>({id:1,ok:true,view:h.j.view,store_id:snap.store_id,snapshot:null,status:null,record:value,job:null,error:''});
  return h;
}
test('explicit finalized and later-conflict links retain original award and only a dated current overlay',async()=>{
  for(const stage of ['finalized','conflict']){const h=resultLinkHarness(stage),before=h.run('JSON.stringify(historyPage)');await h.run('historyReadResult(1)');assert.ok(h.run('historyOverlay'),h.run('historyMessage'));assert.equal(h.run('historyOverlay.conflict'),stage==='conflict');assert.equal(h.run('JSON.stringify(historyPage)'),before);assert.deepEqual(h.calls.map(c=>c.method),['CareerHistoryResult']);assert.equal(h.run('publicationRecord'),null);assert.match(h.elements.get('history-result').textContent,/Local evidence read/);if(stage==='conflict')assert.match(h.elements.get('history-result').textContent,/Conflict reported; finalized award retained/);}
});
test('a cold publication view or mismatched result never supplies an unchecked overlay',async()=>{
  for(const cold of [true,false]){const h=resultLinkHarness('conflict');if(cold)h.set('publicationSnapshot',null);else h.run("historyPage.items[0].event.result_digest='f'.repeat(64)");await h.run('historyReadResult(1)');assert.equal(h.run('historyOverlay'),null);assert.equal(h.run('historyPage.items.length'),1);assert.match(h.elements.get('history-result').textContent,/Current status not checked/);assert.deepEqual(h.calls.map(c=>c.method),['CareerHistoryResult']);}
});
