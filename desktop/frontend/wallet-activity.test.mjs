// Independently frozen Set70 wire/arithmetic cases plus controlled UI lifetimes.
import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';
import {readFileSync} from 'node:fs';
import test from 'node:test';
import vm from 'node:vm';
const bytes=readFileSync(new URL('./testdata/wallet-activity-wire/wire-v1.json',import.meta.url));
assert.equal(createHash('sha256').update(bytes).digest('hex'),'fd0f28ff06b4cb0502e7822ad401330941cdb71e9a4d4b5e31d5a017f911dc2b');
const corpus=JSON.parse(bytes),clone=v=>JSON.parse(JSON.stringify(v));
const source=readFileSync(new URL('./stable.js',import.meta.url),'utf8'),html=readFileSync(new URL('./stable.html',import.meta.url),'utf8');
const script=source.slice("import { Call } from '/wails/runtime.js';".length,source.indexOf("for (const kind of ['publish', 'reconcile', 'attest', 'export']) ui[`result-${kind}`].addEventListener"));
class Element {
 constructor(){this.children=[];this.listeners=new Map();this.attributes=new Map();this._text='';this.disabled=false;this.hidden=false;this.style={};}
 set textContent(v){this._text=String(v);this.children=[];}get textContent(){return this._text+this.children.map(c=>c.textContent).join('');}
 set innerHTML(_){throw Error('Unsafe HTML');}append(...v){this.children.push(...v);}setAttribute(k,v){this.attributes.set(k,String(v));}removeAttribute(k){this.attributes.delete(k);}addEventListener(k,v){this.listeners.set(k,v);}remove(){}
}
const value=name=>clone(corpus.positives.find(x=>x.name===name).value);
function harness(binding=corpus.bindings.zero256){
 const elements=new Map([...html.matchAll(/\bid="([^"]+)"/g)].map(m=>[m[1],new Element()])),calls=[],handlers={};
 const context=vm.createContext({document:{getElementById:id=>elements.get(id),createElement:()=>new Element(),hidden:false},window:{matchMedia:()=>({matches:false,addEventListener(){}})},Call:{async ByName(name,...args){const method=name.slice(9);calls.push({method,args:clone(args)});assert.ok(handlers[method],`Unexpected ${method}`);return clone(await handlers[method](...args));}},Intl,Date,btoa:s=>Buffer.from(s).toString('base64'),setTimeout(){return 1;},clearTimeout(){}});
 vm.runInContext(script,context);const run=s=>vm.runInContext(s,context),set=(name,v)=>{context.input=clone(v);run(`${name}=input`);};
 const fixture=binding.gym_id===corpus.bindings.zero256.gym_id?'zero256':'incrementing256';run('poolMode=true');set('current',{state:'stopped',backup_state:'idle',account:{state:'signed_in',fixture},view:binding.view,source_binding_id:binding.source_binding_id});
 const response=(arm,v)=>({id:1,version:1,ok:true,snapshot:null,job:null,page:null,detail:null,error:'',[arm]:clone(v)}),error=code=>({id:1,version:1,ok:false,snapshot:null,job:null,page:null,detail:null,error:code});
 const done=value(fixture+'-job-response').job,page=value(fixture+'-page-response').page,detail=value(fixture+'-detail-response').detail;
 return{run,set,context,elements,calls,handlers,response,error,done,page,detail};
}
test('independent typed positives and exact signed atom formatting',()=>{
 let checked=0;for(const row of corpus.positives){if(row.kind!=='response')continue;const h=harness(row.expected_binding),arm=['snapshot','job','page','detail'].find(k=>row.value[k]!==null)||'snapshot';h.context.value=clone(row.value);h.run(`activityEnvelope(value,'${arm}')`);checked++;}assert.ok(checked>45);
 const h=harness();for(const row of corpus.atom_formats)assert.equal(h.run(`activityAtoms(${row.atoms})`),row.slug);
});
test('independent response shape defects and full wallet binding substitutions refuse',()=>{
 let checked=0;for(const row of corpus.negatives){if(row.kind!=='response')continue;
 const bindingChange=row.name.startsWith('foreign-') && !['foreign-job'].includes(row.name);
 if(row.scope!=='shape' && !bindingChange)continue;
 const h=harness(row.expected_binding),arm=['snapshot','job','page','detail'].find(k=>row.value[k]!==null)||'snapshot';h.context.value=clone(row.value);assert.throws(()=>h.run(`activityEnvelope(value,'${arm}')`),row.name);checked++;}assert.ok(checked>55);
});
test('explicit capture includes reciprocal incoming and outgoing rows; details retain exact capture',async()=>{
 const h=harness(),running=value('zero256-begin-response').job;h.handlers.BeginWalletActivity=()=>h.response('job',running);h.handlers.WalletActivityJob=()=>h.response('job',h.done);h.handlers.WalletActivityPage=()=>h.response('page',h.page);h.handlers.WalletActivityDetail=()=>h.response('detail',h.detail);
 await h.run('beginActivity()');assert.equal(h.run('activityBusy()'),true);await h.run('fetchActivity()');assert.equal(h.run('activityBusy()'),false);assert.deepEqual(h.calls.map(x=>x.method),['BeginWalletActivity','WalletActivityJob','WalletActivityPage']);
 assert.match(h.elements.get('wallet-activity-list').textContent,/-1\.00000141 SLUG/);assert.match(h.elements.get('wallet-activity-list').textContent,/1\.00000000 SLUG/);
 h.context.tx=h.detail.item.txid;await h.run('readActivityDetail(activityPage.items.find(x=>x.txid===tx))');assert.match(h.elements.get('wallet-activity-detail').textContent,/Received, including change/);assert.match(h.elements.get('wallet-activity-detail').textContent,/Spent from wallet inputs/);assert.match(h.elements.get('wallet-activity-detail').textContent,/Fee 0\.00000141 SLUG/);
 assert.deepEqual(h.calls.at(-1).args,[h.done.binding.view.view_id,h.done.job_id,h.detail.item.txid]);const count=h.calls.length;await h.run('fetchActivity()');assert.equal(h.calls.length,count,'completed capture must not refresh automatically');
});
test('failed refresh and same-tip changes preserve dated capture until a successful replacement',async()=>{
 const h=harness();h.set('activityCaptureJob',h.done);h.set('activityAttempt',h.done);h.set('activityPage',h.page);h.set('activityDetail',h.detail);
 const before=h.run('JSON.stringify([activityCaptureJob,activityPage,activityDetail])');h.handlers.BeginWalletActivity=()=>value('job-failed-capacity');await h.run('beginActivity()');assert.equal(h.run('JSON.stringify([activityCaptureJob,activityPage,activityDetail])'),before);assert.match(h.elements.get('wallet-activity-status').textContent,/previous dated capture/);
 const next=clone(h.done);next.job_id=corpus.job_ids.later_C;next.capture.captured_at_ms++;const page=clone(h.page);page.job_id=next.job_id;page.capture=next.capture;page.items[0].recorded_at_ms++;
 h.handlers.BeginWalletActivity=()=>h.response('job',next);h.handlers.WalletActivityPage=()=>h.response('page',page);await h.run('beginActivity()');assert.equal(h.run('activityCaptureJob.job_id'),next.job_id);assert.equal(h.run('activityDetail'),null);assert.equal(h.run('activityPage.items[0].recorded_at_ms'),page.items[0].recorded_at_ms);
});
test('lost Begin resolves exact native-held action without another capture or adopting old success',async()=>{
 const h=harness();h.set('activityCaptureJob',h.done);h.set('activityAttempt',h.done);h.set('activityPage',h.page);h.handlers.BeginWalletActivity=()=>h.error('outcome_unknown');await h.run('beginActivity()');assert.equal(h.run('activityUnknown'),true);
 h.handlers.WalletActivitySnapshot=()=>value('snapshot-state-2');await h.run('fetchActivity()');assert.equal(h.run('activityUnknown'),false);assert.equal(h.run('activityCaptureJob.job_id'),h.done.job_id);assert.equal(h.calls.filter(x=>x.method==='WalletActivityJob').length,0);assert.equal(h.calls.filter(x=>x.method==='BeginWalletActivity').length,1);
});
test('cancellation waits for terminal status and exact missing job retires uncertainty',async()=>{
 const h=harness(),running=value('job-running-empty').job;h.set('activityAttempt',running);h.handlers.CancelWalletActivity=()=>h.response('job',running);await h.run('cancelActivity()');assert.equal(h.run('activityBusy()'),true);
 h.handlers.WalletActivityJob=()=>value('job-cancelled-empty');await h.run('fetchActivity()');assert.equal(h.run('activityBusy()'),false);
 h.set('activityAttempt',running);h.handlers.WalletActivityJob=()=>h.error('activity_not_found');await h.run('fetchActivity()');assert.equal(h.run('activityBusy()'),false);
});
test('original-view clearing discards late capture, page and detail replies',async()=>{
 for(const method of ['BeginWalletActivity','WalletActivityPage','WalletActivityDetail']){const h=harness();h.set('activityCaptureJob',h.done);h.set('activityPage',h.page);h.context.tx=h.detail.item.txid;let release;h.handlers[method]=()=>new Promise(r=>{release=r;});
 const promise=h.run(method==='BeginWalletActivity'?'beginActivity()':method==='WalletActivityPage'?'readActivityPage(0)':'readActivityDetail(activityPage.items.find(x=>x.txid===tx))');h.run('clearActivity();epoch++');release(method==='BeginWalletActivity'?value('job-running-empty'):method==='WalletActivityPage'?h.response('page',h.page):h.response('detail',h.detail));await promise;assert.equal(h.run('activityCaptureJob'),null);assert.equal(h.run('activityPage'),null);assert.equal(h.run('activityDetail'),null);}
});
test('detail substitution refuses and backup prevents new reads while preserving old rows',async()=>{
 for(const mutate of [x=>x.job_id=corpus.job_ids.refresh_B,x=>x.capture.captured_at_ms++,x=>x.item.raw_sha256='2'.repeat(64),x=>x.binding=clone(corpus.bindings.incrementing256)]){const h=harness();h.set('activityCaptureJob',h.done);h.set('activityPage',h.page);const bad=clone(h.detail);mutate(bad);h.handlers.WalletActivityDetail=()=>h.response('detail',bad);h.context.tx=h.detail.item.txid;await h.run('readActivityDetail(activityPage.items.find(x=>x.txid===tx))');assert.equal(h.run('activityDetail'),null);}
 const h=harness();h.set('activityCaptureJob',h.done);h.set('activityPage',h.page);h.run("current.backup_state='running';renderActivity()");await h.run('beginActivity()');await h.run('readActivityDetail(activityPage.items[0])');assert.equal(h.calls.length,0);assert.equal(h.elements.get('wallet-activity-refresh').disabled,true);assert.match(h.elements.get('wallet-activity-list').textContent,/Net change/);
});
test('terminal metadata cannot regress and unconfirmed labels make no mempool claim',()=>{
 const h=harness();h.set('activityCaptureJob',h.done);for(const mutate of [x=>{x.state='running';x.capture=null;},x=>x.capture.captured_at_ms++]){const bad=clone(h.done);mutate(bad);h.context.value=bad;assert.throws(()=>h.run('activityJobValue(value)'));}
 h.context.value=value('modeled-zero-atoms').detail.item;assert.equal(h.run('activityConfirmation(value)'),'Unconfirmed wallet record');assert.match(html,/Refresh explicitly to update this view/);
});
