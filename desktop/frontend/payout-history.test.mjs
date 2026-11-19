// Independent frozen wire fixtures and modeled lifetimes; no wallet/node I/O.
import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';
import {readFileSync} from 'node:fs';
import test from 'node:test';
import vm from 'node:vm';
const bytes=readFileSync(new URL('./testdata/payout-history-wire/wire-v1.json',import.meta.url));
assert.equal(createHash('sha256').update(bytes).digest('hex'),'4b3eafc010d515557097357a968bd007571c7126f7c0f15a2cd4184bcc88f802');
const corpus=JSON.parse(bytes),clone=v=>JSON.parse(JSON.stringify(v));
const source=readFileSync(new URL('./stable.js',import.meta.url),'utf8'),html=readFileSync(new URL('./stable.html',import.meta.url),'utf8');
const script=source.slice("import { Call } from '/wails/runtime.js';".length,source.indexOf("for (const kind of ['publish', 'reconcile', 'attest', 'export']) ui[`result-${kind}`].addEventListener"));
class Element {
  constructor(tag='div'){this.tagName=tag;this.children=[];this.listeners=new Map();this.attributes=new Map();this._text='';this.disabled=false;this.hidden=false;this.style={};}
  set textContent(v){this._text=String(v);this.children=[];}get textContent(){return this._text+this.children.map(c=>c.textContent).join('');}
  set innerHTML(_){throw Error('Unsafe HTML');}append(...v){this.children.push(...v);}setAttribute(k,v){this.attributes.set(k,String(v));}removeAttribute(k){this.attributes.delete(k);}addEventListener(k,v){this.listeners.set(k,v);}remove(){}
}
const value=name=>clone(corpus.positives.find(x=>x.name===name).value);
function harness(binding=corpus.bindings.zero256){
  const elements=new Map([...html.matchAll(/\bid="([^"]+)"/g)].map(m=>[m[1],new Element()])),calls=[],handlers={};
  const context=vm.createContext({document:{getElementById:id=>elements.get(id),createElement:tag=>new Element(tag),hidden:false},window:{matchMedia:()=>({matches:false,addEventListener(){}})},Call:{async ByName(name,...args){const method=name.slice(9);calls.push({method,args:clone(args)});assert.ok(handlers[method],`Unexpected ${method}`);return clone(await handlers[method](...args));}},Intl,Date,btoa:s=>Buffer.from(s).toString('base64'),setTimeout(){return 1;},clearTimeout(){}});
  vm.runInContext(script,context);const run=s=>vm.runInContext(s,context),set=(name,v)=>{context.input=clone(v);run(`${name}=input`);};
  run('poolMode=true');const fixture=binding.recipient_script===corpus.bindings.zero256.recipient_script?'zero256':'incrementing256';
  set('current',{state:'stopped',backup_state:'idle',account:{state:'signed_in',fixture},view:binding.view,source_binding_id:binding.source_binding_id});
  const response=(arm,v)=>({id:1,version:1,ok:true,snapshot:null,job:null,page:null,observation:null,error:'',[arm]:clone(v)});
  const error=code=>({id:1,version:1,ok:false,snapshot:null,job:null,page:null,observation:null,error:code});
  const done=value('job_done_A').job,row=value('wallet_confirmed').observation.payment;
  const page=(job=done)=>({version:1,binding:clone(job.binding),job_id:job.job_id,capture:clone(job.capture),after:0,limit:8,next:1,has_more:false,items:[clone(row)]});
  return{run,set,context,elements,calls,handlers,response,error,done,row,page};
}
test('all62 independently frozen response fixtures preserve exact original binding',()=>{
  let count=0;for(const row of corpus.positives){if(row.kind!=='response')continue;const h=harness(row.expected_binding);h.context.value=clone(row.value);const arm=['snapshot','job','page','observation'].find(k=>row.value[k]!==null)??'snapshot';h.run(`payoutEnvelope(value,'${arm}')`);count++;}assert.equal(count,62);
});
test('all43 independently frozen typed response defects reject shape or context substitution',()=>{
  let count=0;for(const row of corpus.negatives){if(row.kind!=='response')continue;const h=harness(row.expected_binding);h.context.value=clone(row.value);const arm=['snapshot','job','page','observation'].find(k=>row.value[k]!==null)??'snapshot';assert.throws(()=>h.run(`payoutEnvelope(value,'${arm}')`),row.name);count++;}assert.equal(count,43);
});
test('independent exact eight-decimal atom formatting does not use wallet float amounts',()=>{
  const h=harness();for(const row of corpus.atom_formats)assert.equal(h.run(`payoutAtoms(${row.atoms})`),row.text);assert.match(html,/Recorded pool state and wallet checks are separate/);
});
test('capture while mining performs only explicit history calls and pages the exact completed job',async()=>{
  const h=harness(),running=value('job_running_B').job,done={...h.done,job_id:running.job_id};h.run("current.state='running'");
  h.handlers.BeginPoolPayoutHistory=()=>h.response('job',running);h.handlers.PoolPayoutJob=()=>h.response('job',done);h.handlers.PoolPayoutPage=()=>h.response('page',h.page(done));
  await h.run('beginPayout()');assert.equal(h.run('payoutBusy()'),true);await h.run('fetchPayout()');assert.equal(h.run('payoutBusy()'),false);
  assert.deepEqual(h.calls.map(c=>c.method),['BeginPoolPayoutHistory','PoolPayoutJob','PoolPayoutPage']);assert.equal(h.run('payoutCaptureJob.job_id'),running.job_id);
  assert.match(h.elements.get('pool-history-list').textContent,/29\.70000000 SLUG/);assert.match(h.elements.get('pool-history-list').textContent,/Pool-recorded confirmed/);
  assert.match(h.elements.get('pool-history-capture').textContent,/Captured .*Pool report/);assert.equal(h.elements.get('pool-history-refresh').disabled,false);
});
test('wallet check binds exact captured transaction and reports separately dated status',async()=>{
  const h=harness();h.set('payoutCaptureJob',h.done);h.set('payoutPage',h.page());h.handlers.ReadPoolPayoutTransaction=()=>value('wallet_confirmed');
  await h.run('readPayoutWallet(payoutPage.items[0])');assert.deepEqual(h.calls,[{method:'ReadPoolPayoutTransaction',args:[h.done.binding.view.view_id,h.done.job_id,h.row.txid,1]}]);
  assert.match(h.elements.get('pool-history-wallet').textContent,/Wallet check .*Wallet-reported confirmed/);assert.equal(h.run('payoutCaptureJob.capture.paid_atoms'),2970000000);
  h.handlers.ReadPoolPayoutTransaction=()=>value('wallet_not_observed');await h.run('readPayoutWallet(payoutPage.items[0])');assert.match(h.elements.get('pool-history-wallet').textContent,/Not observed in the saved wallet transaction view/);assert.doesNotMatch(h.elements.get('pool-history-wallet').textContent,/payment failed|conflict-free|spendable/);
});
test('failed refresh preserves capture and wallet observation without relabeling them current',async()=>{
  const h=harness();h.set('payoutCaptureJob',h.done);h.set('payoutAttempt',h.done);h.set('payoutPage',h.page());h.set('payoutObservation',value('wallet_confirmed').observation);const before=h.run('JSON.stringify([payoutCaptureJob,payoutPage,payoutObservation])');
  h.handlers.BeginPoolPayoutHistory=()=>value('job_failed_payout_unavailable');await h.run('beginPayout()');assert.equal(h.run('JSON.stringify([payoutCaptureJob,payoutPage,payoutObservation])'),before);assert.equal(h.run('payoutBusy()'),false);assert.match(h.elements.get('pool-history-status').textContent,/previous dated capture is retained/);
});
test('lost new begin cannot adopt an older same-view success from snapshot',async()=>{
  const h=harness();h.set('payoutCaptureJob',h.done);h.set('payoutAttempt',h.done);h.set('payoutPage',h.page());h.handlers.BeginPoolPayoutHistory=()=>{throw Error('lost reply');};
  await h.run('beginPayout()');assert.equal(h.run('payoutUnknown'),true);h.handlers.PoolPayoutSnapshot=()=>value('snapshot_idle_capture_A');await h.run('fetchPayout()');
  assert.equal(h.run('payoutUnknown'),false);assert.equal(h.run('payoutAttempt'),null);assert.equal(h.run('payoutCaptureJob.job_id'),h.done.job_id);assert.equal(h.calls.filter(c=>c.method==='PoolPayoutJob').length,0);assert.match(h.elements.get('pool-history-status').textContent,/did not start/);
});
test('lost reply resolves only the native current action without repeating begin',async()=>{
  const h=harness(),done={...h.done,job_id:corpus.job_ids.refresh_B};h.set('payoutCaptureJob',h.done);h.set('payoutAttempt',h.done);h.set('payoutPage',h.page());h.handlers.BeginPoolPayoutHistory=()=>h.error('outcome_unknown');await h.run('beginPayout()');
  h.handlers.PoolPayoutSnapshot=()=>value('snapshot_failed_B_retains_A');h.handlers.PoolPayoutJob=()=>h.response('job',done);h.handlers.PoolPayoutPage=()=>h.response('page',h.page(done));await h.run('fetchPayout()');
  assert.equal(h.run('payoutCaptureJob.job_id'),done.job_id);assert.equal(h.calls.filter(c=>c.method==='BeginPoolPayoutHistory').length,1);assert.equal(h.calls.find(c=>c.method==='PoolPayoutJob').args[1],done.job_id);
});
test('cancellation remains busy until the exact running capture reaches a terminal job',async()=>{
  const h=harness();h.set('payoutAttempt',value('job_running_B').job);h.handlers.CancelPoolPayoutHistory=()=>value('job_running_B');await h.run('cancelPayout()');assert.equal(h.run('payoutBusy()'),true);
  h.handlers.PoolPayoutJob=()=>value('job_cancelled_B');await h.run('fetchPayout()');assert.equal(h.run('payoutBusy()'),false);assert.match(h.elements.get('pool-history-status').textContent,/cancelled/);
});
test('definitive missing current capture releases the read action while preserving dated history',async()=>{
  const h=harness();h.set('payoutCaptureJob',h.done);h.set('payoutPage',h.page());h.set('payoutAttempt',value('job_running_B').job);h.handlers.PoolPayoutJob=()=>h.error('payout_not_found');await h.run('fetchPayout()');
  assert.equal(h.run('payoutBusy()'),false);assert.equal(h.run('payoutCaptureJob.job_id'),h.done.job_id);assert.equal(h.elements.get('pool-history-refresh').disabled,false);
});
test('old begin, page and wallet replies cannot publish after the original view is cleared',async()=>{
  for(const method of ['BeginPoolPayoutHistory','PoolPayoutPage','ReadPoolPayoutTransaction']){
    const h=harness();h.set('payoutCaptureJob',h.done);h.set('payoutPage',h.page());let release;h.handlers[method]=()=>new Promise(r=>{release=r;});
    const action=method==='BeginPoolPayoutHistory'?'beginPayout()':method==='PoolPayoutPage'?'readPayoutPage(0)':'readPayoutWallet(payoutPage.items[0])';const pending=h.run(action);h.run('clearPayout();epoch++');
    release(method==='BeginPoolPayoutHistory'?value('job_running_B'):method==='PoolPayoutPage'?h.response('page',h.page()):value('wallet_confirmed'));await pending;
    assert.equal(h.run('payoutCaptureJob'),null);assert.equal(h.run('payoutObservation'),null);assert.equal(h.run('payoutPage'),null);
  }
});
test('backup blocks admission without hiding existing captured data or changing mining',async()=>{
  const h=harness();h.set('payoutCaptureJob',h.done);h.set('payoutPage',h.page());h.run("current.backup_state='running';renderPayout()");await h.run('beginPayout()');await h.run('readPayoutWallet(payoutPage.items[0])');assert.equal(h.calls.length,0);assert.equal(h.elements.get('pool-history-refresh').disabled,true);assert.match(h.elements.get('pool-history-list').textContent,/29\.70000000/);
});
test('page and wallet responses cannot switch captured identity, amount or source',async()=>{
  for(const mutate of [v=>v.job_id=corpus.job_ids.refresh_B,v=>v.capture.paid_atoms++,v=>v.payment.atoms++,v=>v.binding=clone(corpus.bindings.incrementing256)]){
    const h=harness();h.set('payoutCaptureJob',h.done);h.set('payoutPage',h.page());const bad=value('wallet_confirmed');mutate(bad.observation);h.handlers.ReadPoolPayoutTransaction=()=>bad;await h.run('readPayoutWallet(payoutPage.items[0])');assert.equal(h.run('payoutObservation'),null);assert.equal(h.run('payoutCaptureJob.capture.paid_atoms'),2970000000);
  }
});
test('captured paging retains immutable metadata and never reloads the source',async()=>{
  const h=harness(),first=value('page_count9_after0_limit8').page,last=value('page_count9_after1_limit8').page,job={...h.done,capture:first.capture};h.set('payoutCaptureJob',job);h.set('payoutPage',first);h.handlers.PoolPayoutPage=()=>h.response('page',{...last,after:8,next:9,items:last.items.slice(-1),has_more:false});
  await h.run('readPayoutPage(8)');assert.deepEqual(h.calls.map(c=>c.method),['PoolPayoutPage']);assert.equal(h.run('payoutPage.next'),9);assert.equal(h.run('payoutPage.has_more'),false);
});
test('terminal capture metadata cannot regress or change after disclosure',()=>{
  const h=harness();h.set('payoutCaptureJob',h.done);for(const change of [v=>{v.state='running';v.capture=null;},v=>v.capture.captured_at_ms++,v=>v.capture.paid_atoms++]){const j=clone(h.done);change(j);h.context.value=j;assert.throws(()=>h.run('payoutJobValue(value)'));}
});
