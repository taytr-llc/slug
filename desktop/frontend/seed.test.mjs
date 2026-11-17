// Frozen independent transport/DOM-state controls, not a browser rendering claim.
import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';
import {readFileSync} from 'node:fs';
import test from 'node:test';
import vm from 'node:vm';
const raw=readFileSync(new URL('../../gymseed/testdata/reference/seed_v1.json',import.meta.url));
assert.equal(createHash('sha256').update(raw).digest('hex'),'2b8d78e8ef665e372543eb87bb5d25a975026a0ae946b15a871ac97bbf2153f0');
const ref=JSON.parse(raw), old=JSON.parse(readFileSync(new URL('../../gymevent/testdata/reference/event_v1.json',import.meta.url))), clone=v=>JSON.parse(JSON.stringify(v));
const source=readFileSync(new URL('./stable.js',import.meta.url),'utf8'),html=readFileSync(new URL('./stable.html',import.meta.url),'utf8');
const script=source.slice("import { Call } from '/wails/runtime.js';".length,source.indexOf("ui['event-refresh'].addEventListener"));
class Element {
 constructor(tag='div'){this.tagName=tag;this.children=[];this.attributes=new Map();this.listeners=new Map();this.hidden=false;this.disabled=false;this._text='';}
 set textContent(v){this._text=String(v);this.children=[];} get textContent(){return this._text+this.children.map(c=>c.textContent).join('');}
 set innerHTML(_){throw Error('Unsafe HTML');} setAttribute(k,v){this.attributes.set(k,String(v));} removeAttribute(k){this.attributes.delete(k);} addEventListener(k,f){this.listeners.set(k,f);} append(...nodes){this.children.push(...nodes);}
}
function dto(kind,name){const d=ref.dtos.find(d=>d.kind===kind&&d.name===name);assert.ok(d,`${kind}/${name}`);return clone(d.value);}
function record(predicate=()=>true){const values=[...ref.dtos.filter(d=>d.kind==='Record').map(d=>d.value),...ref.record_states.map(d=>JSON.parse(d.canonical))];const d=values.find(predicate);assert.ok(d,'Record');return clone(d);}
function harness(fixture='zero256'){
 const f=old.fixtures.find(f=>f.fixture===fixture),elements=new Map([...html.matchAll(/\bid="([^"]+)"/g)].map(m=>[m[1],new Element()]));
 const handlers={},calls=[],document={getElementById:id=>elements.get(id),createElement:tag=>new Element(tag),hidden:false};let id=1000,active=0,peak=0;
 const Call={async ByName(name,...args){const method=name.slice(9);calls.push({method,args:clone(args)});assert.ok(handlers[method],`Unexpected ${method}`);active++;peak=Math.max(peak,active);try{return clone(await handlers[method](...args));}finally{active--;}}};
 const context=vm.createContext({document,window:{matchMedia:()=>({matches:false,addEventListener(){}})},Call,Intl,Date,btoa:s=>Buffer.from(s,'ascii').toString('base64'),setTimeout(){return 1;},clearTimeout(){}});
 vm.runInContext(script,context,{filename:'stable.js'});
 const h={context,elements,handlers,calls,fixture:f,get peak(){return peak;},run:s=>vm.runInContext(s,context),set(n,v){context.inputValue=clone(v);vm.runInContext(`${n}=inputValue`,context);},count:m=>calls.filter(c=>c.method===m).length};
 h.set('current',{account:{state:'signed_in',id:f.current_view.account_id,fixture,held:false,can_export:true},view:f.current_view,gym_id:f.participant_id});
 h.set('stable',{state:'ready',view:f.current_view,status:{pin:f.entry_pin.stable_pin,store_id:f.entry_pin.stable_store_id,caught_up:true,pending_operation_id:'',slots:Array(10).fill('')}});
 h.run('enabled=true;rememberedView=currentKey()');
 const event=old.dtos.find(d=>d.kind==='EventSnapshot'&&d.fixture===fixture&&d.value.state==='ready').value;
 h.set('eventAuthority',{pin:old.pin.value,storeID:old.pin.store_id,createdMS:event.coordinator_created_at_ms});h.set('eventSnapshot',event);
 h.reply=(field,value)=>({id:++id,ok:true,view:clone(f.current_view),store_id:ref.pin.store_id,snapshot:null,status:null,record:null,history:null,job:null,error:'',[field]:clone(value)});
 h.refusal=error=>({id:++id,ok:false,view:null,store_id:'',snapshot:null,status:null,record:null,history:null,job:null,error});
 context.initial=h.reply('snapshot',dto('SeedSnapshot',fixture+'/ready'));h.run("seedSnapshot=seedEnvelope(initial,'snapshot').value;entryDate='2026-11-21'");return h;
}
const deferred=()=>{let resolve;const promise=new Promise(r=>{resolve=r;});return {resolve,promise};};

test('every frozen accepted seed response reaches the bounded presentation consumer',()=>{
 let count=0;
 for(const c of ref.protocol_cases.filter(c=>c.accepted&&c.response.ok)){
  const fixture=c.name.startsWith('incrementing256/')?'incrementing256':'zero256',h=harness(fixture),r=clone(c.response),field=['snapshot','status','record','history','job'].find(k=>r[k]!==null);
  if(field==='snapshot'&&r.snapshot.state==='setup_incomplete')h.run('seedAuthority=null');
  if(c.current_view){h.set('current.view',c.current_view);h.set('stable.view',c.current_view);h.set('seedSnapshot.view',c.current_view);}
  h.context.input=r;assert.doesNotThrow(()=>h.run(`seedEnvelope(input,'${field}')`),c.name);count++;
 }
 assert.ok(count>=20);
});
test('snapshot polling never publishes, attests or reconciles',async()=>{
 const h=harness();h.handlers.SeedSnapshot=()=>h.reply('snapshot',dto('SeedSnapshot','zero256/ready'));await h.run('fetchSeedSnapshot()');
 assert.deepEqual(h.calls.map(c=>c.method),['SeedSnapshot']);assert.equal(h.peak,1);
});
test('held and nonpublisher gyms can read but cannot publish or attest',async()=>{
 for(const fixture of ['zero256','incrementing256']){const h=harness(fixture);if(fixture==='zero256')h.run('current.account.held=true');
  await h.run("seedBegin('publish')");await h.run("seedBegin('attest')");assert.equal(h.calls.length,0);h.run('renderSeed()');assert.equal(h.elements.get('seed-publish').disabled,true);assert.equal(h.elements.get('seed-attest').disabled,true);assert.equal(h.elements.get('seed-reconcile').disabled,false);
 }
});
test('unknown publication never retries and preserves explicit reconcile',async()=>{
 const h=harness();h.handlers.BeginSeedPublish=()=>h.refusal('outcome_unknown');await h.run("seedBegin('publish')");await h.run("seedBegin('publish')");assert.equal(h.count('BeginSeedPublish'),1);assert.equal(h.run('seedUnknown'),true);h.run('renderSeed()');assert.equal(h.elements.get('seed-reconcile').disabled,false);
});
test('an unresolved deadline remains visible without changing the seed',()=>{
 const h=harness(),r=record(r=>r.incident_at_ms>0);h.context.input=h.reply('record',r);h.run("seedDetail=seedEnvelope(input,'record').value;renderSeed()");
 assert.match(h.elements.get('seed-incident').textContent,/Chain incident first recorded/);assert.match(h.elements.get('seed-finality').textContent,/No substitute seed|First finality recorded/);assert.equal(h.calls.length,0);
});
test('metadata poll time never replaces the last verified chain observation',()=>{
 const h=harness(),r=record(r=>r.observation.chain_observed_at_ms>0);const chain=r.observation.chain_observed_at_ms;r.observation.observed_at_ms+=1000;r.observation.node_state='offline';
 h.context.input=h.reply('record',r);h.run("seedDetail=seedEnvelope(input,'record').value;renderSeed()");assert.ok(h.elements.get('seed-observation').textContent.includes(new Date(chain).toISOString()));assert.equal(h.calls.length,0);
});
test('late result after logout cannot repopulate seed evidence',async()=>{
 const h=harness(),j=dto('SeedJob','zero256/prepare/completed/'),gate=deferred();h.set('seedWatch',{id:j.job_id,kind:j.kind,saturday:j.saturday,eventKey:j.event_key});h.handlers.SeedJob=()=>gate.promise;
 const pending=h.run(`seedFetchJob('${j.job_id}')`);h.run('epoch++;clearSeed();current=null;stable=null');gate.resolve(h.reply('job',j));await pending;assert.equal(h.run('seedDetail'),null);assert.equal(h.run('seedWatch'),null);
});
test('date navigation rejects a late selected-record reply',async()=>{
 const h=harness(),r=record(),gate=deferred();h.handlers.SeedRecord=()=>gate.promise;h.run('render=renderSeed;command="record";background=Promise.resolve()');const pending=h.run("seedRead('record')");h.run("entryNavigate('2026-11-28')");gate.resolve(h.reply('record',r));await pending;assert.equal(h.run('seedDetail'),null);
});
test('cache refusal preserves existing detail before dispatch',async()=>{
 const h=harness(),r=record();h.set('seedDetail',r);h.run("previewImages.set('bounded-test-stub','x'.repeat(850000))");await h.run("seedBegin('prepare')");assert.equal(h.calls.length,0);assert.equal(h.run('seedDetail.event_key'),r.event_key);assert.equal(h.run('seedWatch'),null);assert.match(h.run('seedMessage'),/capacity/);
});
test('a completed reply that cannot fit preserves the prior bounded display',async()=>{
 const h=harness(),j=dto('SeedJob','zero256/prepare/completed/');h.handlers.SeedJob=()=>h.reply('job',j);h.run("previewImages.set('bounded-test-stub','x'.repeat(1040000))");await h.run(`seedFetchJob('${j.job_id}')`);assert.equal(h.run('seedDetail'),null);assert.match(h.run('seedMessage'),/capacity/);
});
test('wrong view and extra record fields fail before display',()=>{
 for(const mutate of [r=>{r.view.generation++;},r=>{r.record.seed_substitute=true;},r=>{r.record.source_receipt.input.event.cutoff_ms++;}]){const h=harness(),r=h.reply('record',record());mutate(r);h.context.input=r;assert.throws(()=>h.run("seedEnvelope(input,'record')"));assert.equal(h.run('seedDetail'),null);}
});
test('cleared presentation retains its authority but no evidence or render keys',()=>{
 const h=harness();h.set('seedDetail',record());h.run('renderSeed();clearSeed();renderSeed()');assert.equal(h.run('seedDetail'),null);assert.equal(h.run('seedHistory'),null);assert.equal(h.elements.get('seed-history-list')._key,'');assert.ok(h.run('seedAuthority'));assert.equal(h.elements.get('seed-identity').textContent,'');
});

test('a newer selected status replaces older detail without hiding a journaled incident',async()=>{
 const h=harness(),r=record(r=>r.incident_at_ms===0&&!r.finality),s=clone(ref.scenarios.flatMap(x=>x.steps).map(x=>x.status).find(s=>s&&s.incident_at_ms>0&&s.event.saturday===r.source_receipt.input.event.saturday));
 assert.ok(s);h.set('seedDetail',r);h.handlers.SeedStatus=()=>h.reply('status',s);await h.run("seedRead('status')");
 assert.equal(h.run('seedDetail'),null);assert.equal(h.run('seedStatus.incident_at_ms'),s.incident_at_ms);
 assert.match(h.elements.get('seed-incident').textContent,/Chain incident first recorded/);
 assert.ok(h.elements.get('seed-incident').textContent.includes(new Date(s.incident_at_ms).toISOString()));
 assert.deepEqual(h.calls.map(c=>c.method),['SeedStatus']);
});
test('selected record and completed job replace stale status as one coherent observation',async()=>{
 for(const source of ['record','job']){
  const h=harness(),j=dto('SeedJob','zero256/prepare/completed/');h.set('seedStatus',dto('Status','conflict'));
  if(source==='record'){h.handlers.SeedRecord=()=>h.reply('record',j.result);await h.run("seedRead('record')");}
  else {h.handlers.SeedJob=()=>h.reply('job',j);await h.run(`seedFetchJob('${j.job_id}')`);}
  assert.equal(h.run('seedStatus'),null);assert.equal(h.run('seedDetail.event_key'),j.result.event_key);
  assert.equal(h.elements.get('seed-state').textContent,h.run('label(seedRecordPhase(seedDetail))'));
 }
});
test('initial snapshot capacity refusal rolls back the newly learned authority too',async()=>{
 const h=harness();h.run('seedAuthority=null;seedSnapshot=null;seedStatus=null;seedDetail=null;seedHistory=null');
 // Synthetic retained-byte stub tests transactional budget admission; it does
 // not claim an image of this size passed the independent portrait validator.
 h.run("previewImages.set('near-capacity-owned-image','x'.repeat(Math.floor((CACHE_MAX-previewRetainedBytes()-entryRetainedBytes()-eventRetainedBytes()-seedRetainedBytes()-JSON.stringify(pageItems).length-1024)/2)))");
 assert.equal(h.run('previewCapacity()'),true);
 h.handlers.SeedSnapshot=()=>h.reply('snapshot',dto('SeedSnapshot','zero256/ready'));await h.run('fetchSeedSnapshot()');
 assert.equal(h.run('seedSnapshot'),null);assert.equal(h.run('seedAuthority'),null);assert.equal(h.run('previewCapacity()'),true);
});
test('known reconcile uncertainty describes evidence only for begin, job and cancel',async()=>{
 for(const source of ['begin','job','cancel']){
  const h=harness(),j=dto('SeedJob','zero256/reconcile/failed/outcome_unknown');
  if(source==='begin'){h.handlers.BeginSeedReconcile=()=>h.refusal('outcome_unknown');await h.run("seedBegin('reconcile')");}
  else {
   h.set('seedWatch',{id:j.job_id,kind:j.kind,saturday:j.saturday,eventKey:j.event_key});
   if(source==='job'){h.handlers.SeedJob=()=>h.reply('job',j);await h.run(`seedFetchJob('${j.job_id}')`);}
   else {h.handlers.CancelSeedJob=()=>h.refusal('outcome_unknown');await h.run('seedCancel()');}
  }
  assert.match(h.run('seedMessage'),/observation may have been recorded/);assert.match(h.run('seedMessage'),/No transaction was sent/);assert.doesNotMatch(h.run('seedMessage'),/recorded or sent/);
  assert.equal(h.run('seedUnknown'),true);assert.equal(h.calls.length,1);
 }
});
test('known reconcile transport loss stays observation-only while unknown job kind stays generic',async()=>{
 for(const known of [true,false]){
  const h=harness(),j=dto('SeedJob','zero256/reconcile/queued');
  if(known)h.set('seedWatch',{id:j.job_id,kind:j.kind,saturday:j.saturday,eventKey:j.event_key});
  h.handlers.SeedJob=()=>{throw Error('Lost pipe reply');};await h.run(`seedFetchJob('${j.job_id}')`);
  assert.equal(h.run('seedUnknown'),true);
  if(known)assert.match(h.run('seedMessage'),/No transaction was sent/);else assert.match(h.run('seedMessage'),/recorded or sent/);
 }
});
