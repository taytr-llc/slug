// Shipped page + independent public DTO/DOM fixtures only. No native window,
// account, credential, HTTP, wallet or scheduled-payment acceptance is claimed.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { createHash } from 'node:crypto';
import test from 'node:test';
import vm from 'node:vm';
const source=readFileSync(new URL('./remote-daily.js',import.meta.url),'utf8');
const html=readFileSync(new URL('./remote-daily.html',import.meta.url),'utf8');
const raw=readFileSync(new URL('../pilot/testdata/daily-wallet-protocol/reference-v1.json',import.meta.url));
assert.equal(createHash('sha256').update(raw).digest('hex'),'b62572b6f1966acad3845348feb446ebb24cb5d1d3c043498a6f95e5b6645f1a');
const corpus=JSON.parse(raw);
const response=name=>{const row=corpus.cases.find(x=>x.name===name&&x.valid&&x.kind==='response');assert.ok(row,name);return JSON.parse(Buffer.from(row.frame_hex,'hex').toString('ascii'));};
class Element {
 constructor(id){this.id=id;this.disabled=true;this.hidden=false;this.textContent='';this.listeners=new Map();this.children=[];}
 set innerHTML(_){throw new Error('HTML interpolation forbidden');}
 addEventListener(name,fn){this.listeners.set(name,fn);}
 replaceChildren(...v){this.children=[...v];}
 appendChild(v){this.children.push(v);}
 async click(){if(!this.disabled)await this.listeners.get('click')?.();}
}
async function harness(){
 const elements=new Map([...html.matchAll(/\bid="([^"]+)"/g)].map(x=>[x[1],new Element(x[1])]));const calls=[],handlers=new Map(),timers=[];let counter=0n,active=false;
 const document={getElementById(id){assert.ok(elements.has(id),id);return elements.get(id);},createElement(tag){assert.equal(tag,'li');return new Element('row');}};
 const command={Snapshot:'snapshot',LoginFixture:'login_fixture',Start:'start',Stop:'stop',Logout:'logout',BeginDailyPayoutRead:'daily_payout_begin',DailyPayoutRead:'daily_payout_read',CancelDailyPayoutRead:'daily_payout_cancel'};
 const context=vm.createContext({document,structuredClone,setTimeout(fn){timers.push(fn);},Call:{async ByName(name,...args){
  assert.ok(name.startsWith('main.DailyRemoteGym.'));const method=name.slice('main.DailyRemoteGym.'.length);assert.ok(Object.hasOwn(command,method));calls.push({method,args:structuredClone(args)});
  let out;if(handlers.has(method))out=await handlers.get(method)(...args);else if(method==='Snapshot')out=response(active?'control-snapshot-result':'initial-signed-out-result');else if(method==='LoginFixture')out=response('control-login_fixture-result');else if(method==='Stop'||method==='Start'||method==='Logout')out=response('control-'+command[method]+'-result');else throw new Error('unexpected fixed native call');
  out=structuredClone(out);out.id=(++counter).toString();out.command=command[method];out.view_id=method==='LoginFixture'?'':method==='Snapshot'?(active?corpus.selection.view.view_id:''):args[0];
  if(method==='LoginFixture')active=true;if(method==='Logout')active=false;return out;
 }}});
 const prefix="import { Call } from '/wails/runtime.js';\n";assert.ok(source.startsWith(prefix));vm.runInContext(source.slice(prefix.length)+'\nglobalThis.uiState=()=>({snapshot,attempt,capture,shown,busy,retired,nextSequence:String(nextSequence)});globalThis.validateCapture=x=>checkedCapture(x);',context,{timeout:1000});
 async function settle(){for(let i=0;i<30;i++)await Promise.resolve();}
 await settle();assert.equal(context.uiState().busy,false);calls.length=0;
 return {$:id=>elements.get(id),calls,handlers,timers,state:context.uiState,validateCapture:x=>context.validateCapture(structuredClone(x)),async tick(){assert.ok(timers.length);await timers.shift()();await settle();},async login(){await elements.get(corpus.selection.view.fixture==='zero256'?'login-a':'login-b').click();assert.equal(context.uiState().snapshot.state,'signed_in');},begin(){handlers.set('BeginDailyPayoutRead',(_view,sequence,capture,before)=>{const r=response('begin-working');Object.assign(r.job,{sequence,capture_sequence:capture,before_batch:before});return r;});}};
}
test('same wallet layout, explicit login and Start, only eight fixed methods',async()=>{
 const h=await harness();assert.match(html,/class="masthead"/);assert.match(html,/class="mining-bar"/);assert.match(html,/href="\/stable.css"/);
 for(const absent of ['transfer-prepare','export-recovery','entry-save','reset'])assert.equal(h.$(absent),undefined);
 await h.login();assert.equal(h.calls.filter(x=>x.method==='Start').length,0);assert.equal(h.calls.filter(x=>x.method==='BeginDailyPayoutRead').length,0);
 await h.$('mining').click();assert.deepEqual(h.calls.at(-1),{method:'Start',args:[corpus.selection.view.view_id]});
});
test('lossless cumulative money is separate from local spendable and never rounded',async()=>{
 const h=await harness();await h.login();const own=h.$('spendable').textContent;h.begin();await h.$('history-refresh').click();
 h.handlers.set('DailyPayoutRead',()=>{const r=response('lossless-paid-9223372036854775807');r.read.job.sequence='1';r.read.capture.sequence='1';return r;});await h.tick();
 assert.equal(h.$('pool-paid').textContent,'92233720368.54775807');assert.equal(h.$('spendable').textContent,own);assert.match(h.$('history-date').textContent,/Captured .*Settlement observed/);assert.equal(h.calls.filter(x=>x.method==='Start').length,0);
});
test('empty window continues exactly, stale cached first stays dated without recapture',async()=>{
 const h=await harness();await h.login();h.begin();await h.$('history-refresh').click();h.handlers.set('DailyPayoutRead',()=>response('fresh-empty-nonfinal'));await h.tick();
 assert.equal(h.$('history-next').disabled,false);assert.match(h.$('history-rows').children[0].textContent,/more slots remain/);
 await h.$('history-next').click();assert.deepEqual(h.calls.at(-1).args,[corpus.selection.view.view_id,'2','1','11']);h.handlers.set('DailyPayoutRead',()=>response('middle-empty-nonfinal'));await h.tick();
 await h.$('history-next').click();h.handlers.set('DailyPayoutRead',()=>{const r=response('retained-old-capture-failed-stale_capture');r.read.job.sequence='3';r.read.job.before_batch='3';r.read.capture.current=response('middle-empty-nonfinal').read.capture.current;return r;});await h.tick();
 assert.equal(h.$('history-next').disabled,true);assert.equal(h.$('history-first').disabled,false);const date=h.state().capture.captured_at_ms;
 await h.$('history-first').click();assert.deepEqual(h.calls.at(-1).args,[corpus.selection.view.view_id,'4','1','19']);h.handlers.set('DailyPayoutRead',()=>{const r=response('completed-cached-stale-first-window');r.read.job.sequence='4';return r;});await h.tick();
 assert.equal(h.state().retired,false);assert.equal(h.state().capture.captured_at_ms,date);assert.equal(h.state().shown.before_batch,'19');assert.equal(h.state().capture.current.before_batch,'11');assert.match(h.$('history-date').textContent,/Refresh explicitly/);
 assert.equal(h.calls.filter(x=>x.method==='BeginDailyPayoutRead'&&x.args[2]==='0').length,1);
});
test('Busy before acknowledgement retains same sequence and never auto-begins',async()=>{
 const h=await harness();await h.login();h.handlers.set('BeginDailyPayoutRead',()=>{throw new Error('The remote gym is busy. Wait for its current request to finish.');});await h.$('history-refresh').click();
 assert.equal(h.state().attempt.sequence,'1');assert.equal(h.state().attempt.state,'unacknowledged');await h.tick();assert.equal(h.calls.filter(x=>x.method==='BeginDailyPayoutRead').length,1);
 h.begin();await h.$('history-refresh').click();assert.deepEqual(h.calls.filter(x=>x.method==='BeginDailyPayoutRead').map(x=>x.args[1]),['1','1']);
 h.handlers.set('CancelDailyPayoutRead',()=>{const r=response('working-no-capture');r.read.job.state='cancelled';return r;});await h.$('history-cancel').click();assert.equal(h.state().attempt.state,'cancelled');assert.equal(h.calls.at(-1).args[1],'1');
});
test('foreign capture, changed View, unsafe numbers and malformed decimals clear results',async()=>{
 for(const defect of ['view','binding','numeric-paid','leading-zero','unsafe-local']){
  const h=await harness();await h.login();
  if(defect==='view'){h.handlers.set('Start',()=>{const r=response('control-start-result');r.snapshot.view.view_id='e'.repeat(64);r.snapshot.remote.view.view_id='e'.repeat(64);return r;});await h.$('mining').click();}
  else if(defect==='unsafe-local'){h.handlers.set('Snapshot',()=>{const r=response('control-snapshot-result');r.snapshot.remote.spendable_atoms=9007199254740992;return r;});await h.tick();}
  else{h.begin();await h.$('history-refresh').click();h.handlers.set('DailyPayoutRead',()=>{const r=response('fresh-empty-nonfinal');if(defect==='binding')r.read.capture.binding.settlement_config_id='f'.repeat(64);if(defect==='numeric-paid')r.read.capture.first.balance.paid_atoms=1;if(defect==='leading-zero')r.read.capture.sequence='01';return r;});await h.tick();}
  assert.equal(h.state().retired,true,defect);assert.equal(h.state().snapshot,null);assert.equal(h.state().capture,null);assert.equal(h.$('pool-paid').textContent,'—');
 }
});
test('all independent successful read shapes fit the exact frontend validator',async()=>{
 const h=await harness();await h.login();
 // The public fixture supplies shape only. Evaluate the same shipped checker
 // on every positive read without inventing independent expected bytes.
 for(const row of corpus.cases.filter(x=>x.valid&&x.kind==='response'&&!x.selection_override)){
  const r=JSON.parse(Buffer.from(row.frame_hex,'hex').toString('ascii'));if(!r.read?.capture)continue;
  assert.doesNotThrow(()=>h.validateCapture(r.read.capture),row.name);
 }
});
