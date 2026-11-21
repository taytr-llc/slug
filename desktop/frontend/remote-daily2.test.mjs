// Shipped page + independent public DTO/DOM fixtures only. No native window,
// account, credential, HTTP, wallet or scheduled-payment acceptance is claimed.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { createHash } from 'node:crypto';
import test from 'node:test';
import vm from 'node:vm';
const source=readFileSync(new URL('./remote-daily2.js',import.meta.url),'utf8');
const html=readFileSync(new URL('./remote-daily2.html',import.meta.url),'utf8');
const raw=readFileSync(new URL('../pilot/testdata/daily-wallet-activity/reference-v1.json',import.meta.url));
assert.equal(createHash('sha256').update(raw).digest('hex'),'65bc7cb4119e2dabae9260112745ef3fd4cd0541398b209c6b0816acf29c1e8a');
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
 const document={getElementById(id){assert.ok(elements.has(id),id);return elements.get(id);},createElement(tag){assert.ok(['li','button'].includes(tag));return new Element('row');}};
 const command={Snapshot:'snapshot',LoginFixture:'login_fixture',Start:'start',Stop:'stop',Logout:'logout',BeginDailyPayoutRead:'daily_payout_begin',DailyPayoutRead:'daily_payout_read',CancelDailyPayoutRead:'daily_payout_cancel',BeginWalletActivityRead:'wallet_activity_begin',WalletActivityRead:'wallet_activity_read',CancelWalletActivityRead:'wallet_activity_cancel',WalletActivityPage:'wallet_activity_page',WalletActivityDetail:'wallet_activity_detail',ObserveDailyPayout:'daily_payout_observe'};
 const context=vm.createContext({document,structuredClone,setTimeout(fn){timers.push(fn);},Call:{async ByName(name,...args){
  assert.ok(name.startsWith('main.DailyRemoteGym2.'));const method=name.slice('main.DailyRemoteGym2.'.length);assert.ok(Object.hasOwn(command,method));calls.push({method,args:structuredClone(args)});
  let out;if(handlers.has(method))out=await handlers.get(method)(...args);else if(method==='Snapshot')out=response(active?'wire2-control-snapshot-result':'wire2-initial-signed-out-result');else if(method==='LoginFixture')out=response('wire2-control-login_fixture-result');else if(method==='Stop'||method==='Start'||method==='Logout')out=response('wire2-control-'+command[method]+'-result');else throw new Error('unexpected fixed native call');
  out=structuredClone(out);out.id=(++counter).toString();out.command=command[method];out.view_id=method==='LoginFixture'?'':method==='Snapshot'?(active?corpus.selection.view.view_id:''):args[0];
  if(method==='LoginFixture')active=true;if(method==='Logout')active=false;return out;
 }}});
 const prefix="import { Call } from '/wails/runtime.js';\n";assert.ok(source.startsWith(prefix));vm.runInContext(source.slice(prefix.length)+'\nglobalThis.uiState=()=>({snapshot,attempt,capture,shown,busy,retired,nextSequence:String(nextSequence),activityAttempt,activityCap,activityPage,activityDetail,payoutObservation});globalThis.validateCapture=x=>checkedCapture(x);globalThis.validators={ownCapture,ownJob,ownPage,ownDetail,checkedObservation,signedAtoms};',context,{timeout:1000});
 async function settle(){for(let i=0;i<30;i++)await Promise.resolve();}
 await settle();assert.equal(context.uiState().busy,false);calls.length=0;
 return {$:id=>elements.get(id),calls,handlers,timers,state:context.uiState,validateCapture:x=>context.validateCapture(structuredClone(x)),validators:context.validators,async tick(){assert.ok(timers.length);await timers.shift()();await settle();},async login(){await elements.get(corpus.selection.view.fixture==='zero256'?'login-a':'login-b').click();assert.equal(context.uiState().snapshot?.state,'signed_in',elements.get('alert').textContent);},begin(){handlers.set('BeginDailyPayoutRead',(_view,sequence,capture,before)=>{const r=response('wire2-begin-working');Object.assign(r.job,{sequence,capture_sequence:capture,before_batch:before});return r;});}};
}
function beginOwn(h){h.handlers.set('BeginWalletActivityRead',(_v,seq)=>{const r=response('activity-begin-working-empty');r.activity_job.sequence=seq;return r;});}
async function captureEmpty(h){beginOwn(h);await h.$('activity-refresh').click();h.handlers.set('WalletActivityRead',(_v,seq)=>{const r=response('activity-read-done-retained-empty');r.activity_read.job.sequence=seq;r.activity_read.capture=response('activity-page-empty-response').activity_page.capture;r.activity_read.capture.sequence=seq;return r;});h.handlers.set('WalletActivityPage',()=>response('activity-page-empty-response'));await h.tick();}
test('same layout with fourteen fixed methods and explicit mining only',async()=>{
 const h=await harness();assert.match(html,/class="masthead"/);assert.match(html,/class="mining-bar"/);assert.match(html,/href="\/stable.css"/);
 await h.login();assert.equal(h.calls.filter(x=>['Start','BeginDailyPayoutRead','BeginWalletActivityRead'].includes(x.method)).length,0);
 await h.$('mining').click();assert.deepEqual(h.calls.at(-1),{method:'Start',args:[corpus.selection.view.view_id]});
});
test('wide cumulative money keeps all digits separately from local spendable',async()=>{
 const h=await harness();await h.login();const own=h.$('spendable').textContent;h.begin();await h.$('history-refresh').click();h.handlers.set('DailyPayoutRead',()=>{const r=response('wire2-lossless-paid-9223372036854775807');r.read.job.sequence='1';r.read.capture.sequence='1';return r;});await h.tick();assert.equal(h.$('pool-paid').textContent,'92233720368.54775807');assert.equal(h.$('spendable').textContent,own);assert.equal(h.state().retired,false);
});
test('empty original activity capture stays dated through failed refresh and joined cancellation',async()=>{
 const h=await harness();await h.login();await captureEmpty(h);assert.equal(h.state().activityCap.sequence,'1');assert.match(h.$('activity-page').textContent,/No wallet transactions/);const date=h.state().activityCap.metadata.captured_at_ms;
 beginOwn(h);await h.$('activity-refresh').click();h.handlers.set('WalletActivityRead',()=>{const r=response('activity-read-failed-retained-capacity');r.activity_read.capture=structuredClone(h.state().activityCap);return r;});await h.tick();assert.equal(h.state().activityAttempt.sequence,'2');assert.equal(h.state().activityCap.sequence,'1');assert.equal(h.state().activityCap.metadata.captured_at_ms,date);assert.match(h.$('activity-state').textContent,/previous dated capture/);
 await h.$('activity-refresh').click();h.handlers.set('CancelWalletActivityRead',(_v,seq)=>{const r=response('activity-cancel-retains-original');r.activity_read.job.sequence=seq;r.activity_read.capture=structuredClone(h.state().activityCap);return r;});await h.$('activity-cancel').click();assert.equal(h.calls.at(-1).args[1],'3');assert.equal(h.state().activityAttempt.state,'cancelled');assert.equal(h.state().activityCap.sequence,'1');assert.equal(h.calls.filter(x=>x.method==='Start').length,0);
});
test('unknown activity acknowledgement keeps exact sequence without automatic restart',async()=>{
 const h=await harness();await h.login();h.handlers.set('BeginWalletActivityRead',()=>{throw new Error('The remote gym is busy. Wait for its current request to finish.');});await h.$('activity-refresh').click();await h.tick();assert.equal(h.state().activityAttempt.sequence,'1');assert.equal(h.state().activityAttempt.state,'unacknowledged');assert.equal(h.calls.filter(x=>x.method==='BeginWalletActivityRead').length,1);beginOwn(h);await h.$('activity-refresh').click();assert.deepEqual(h.calls.filter(x=>x.method==='BeginWalletActivityRead').map(x=>x.args[1]),['1','1']);
});
test('all independent positive payloads preserve safe signed amounts, exact capture and output selection',async()=>{
 const h=await harness();await h.login();let pages=0,details=0,observations=0;
 for(const c of corpus.cases.filter(x=>x.valid&&x.kind==='response')){const r=JSON.parse(Buffer.from(c.frame_hex,'hex').toString('ascii'));
  if(r.activity_job)assert.doesNotThrow(()=>h.validators.ownJob(r.activity_job,r.activity_job.sequence),c.name);
  if(r.activity_read?.capture)assert.doesNotThrow(()=>h.validators.ownCapture(r.activity_read.capture),c.name);
  if(r.activity_page&&r.activity_page.limit===8){pages++;const p=r.activity_page;assert.doesNotThrow(()=>h.validators.ownPage(p,p.capture,p.after),c.name);}
  if(r.activity_detail){details++;const d=r.activity_detail;assert.doesNotThrow(()=>h.validators.ownDetail(d,d.capture,d.item),c.name);assert.match(h.validators.signedAtoms(d.item.net_atoms),/^-?\d+\.\d{8}$/);}
  if(r.payout_observation){observations++;const o=r.payout_observation;const cap={sequence:o.capture_sequence,binding:o.binding,captured_at_ms:o.captured_at_ms,stale:o.stale,first:{revision:o.revision,high_batch:o.high_batch,observed_at_ms:o.settlement_observed_at_ms}};assert.doesNotThrow(()=>h.validators.checkedObservation(o,cap,{batch_sequence:o.batch_sequence,payment:o.payment}),c.name);}
 }
 assert.ok(pages>=5&&details>=11&&observations>=8);
});
test('selected activity detail is fetched by exact capture and txid, never invented amount',async()=>{
 const h=await harness();await h.login();const d=response('activity-detail-zero256-detail-response').activity_detail,other=response('activity-detail-zero256-incoming-detail').activity_detail;
 // Same inherited original two-row capture; sorting is a conditional DOM fixture,
 // while every expected summary/output remains independently frozen.
 const items=[d.item,other.item].sort((a,b)=>{if((a.block===null)!==(b.block===null))return a.block===null?-1:1;return (a.block===null?b.recorded_at_ms-a.recorded_at_ms:b.block.height-a.block.height)||(a.txid<b.txid?-1:1);});
 beginOwn(h);h.handlers.set('WalletActivityRead',()=>response('activity-read-done-retained-empty'));h.handlers.set('WalletActivityPage',()=>{const r=response('activity-page-empty-response');r.activity_page={capture:structuredClone(d.capture),after:0,limit:8,next:2,has_more:false,items:structuredClone(items)};return r;});h.handlers.set('WalletActivityDetail',(_v,_seq,tx)=>response(tx===d.item.txid?'activity-detail-zero256-detail-response':'activity-detail-zero256-incoming-detail'));
 await h.$('activity-refresh').click();await h.tick();assert.equal(h.$('activity-rows').children.length,2);await h.$('activity-rows').children[0].children[0].click();assert.deepEqual(h.calls.at(-1).args,[corpus.selection.view.view_id,'1',items[0].txid]);assert.match(h.$('activity-detail').textContent,/Raw SHA256/);assert.match(h.$('activity-detail').textContent,/received|Received/);assert.equal(h.state().retired,false);
});
test('foreign activity binding, replacement capture and unsafe amount retire original display',async()=>{
 for(const defect of ['binding','capture','amount']){const h=await harness();await h.login();beginOwn(h);h.handlers.set('WalletActivityRead',()=>{const r=response('activity-read-done-retained-empty');if(defect==='binding')r.activity_read.capture.binding.source_binding_id='f'.repeat(64);return r;});h.handlers.set('WalletActivityPage',()=>{const r=response('activity-mixed-order-page');if(defect==='capture')r.activity_page.capture.sequence='2';if(defect==='amount')r.activity_page.items[0].received_atoms=9007199254740992;return r;});await h.$('activity-refresh').click();await h.tick();assert.equal(h.state().retired,true,defect);assert.equal(h.state().activityCap,null);assert.equal(h.state().snapshot,null);}
});
test('selected payout output uses retained tuple and shows wallet observation separately',async()=>{
 const h=await harness();await h.login();const o=response('selected-wallet_confirmed').payout_observation;
 const original=corpus.context_cases.find(x=>x.name==='selected-original-window').capture;
 h.begin();await h.$('history-refresh').click();h.handlers.set('DailyPayoutRead',()=>{const r=response('wire2-fresh-empty-nonfinal');r.read.job={sequence:'1',capture_sequence:'0',before_batch:'0',state:'done',error:''};r.read.capture=structuredClone(original);return r;});await h.tick();
 h.handlers.set('ObserveDailyPayout',()=>response('selected-wallet_confirmed'));const row=h.$('history-rows').children.find(x=>x.textContent.includes(o.payment.txid));assert.ok(row);await row.children[0].click();assert.deepEqual(h.calls.at(-1).args,[corpus.selection.view.view_id,o.capture_sequence,o.batch_sequence,o.payment.txid,o.payment.output_index]);assert.match(h.$('payout-observation').textContent,/Wallet observed .*Pool record remains/);assert.equal(h.state().capture.first.revision,original.first.revision);assert.equal(h.calls.filter(x=>x.method==='Start').length,0);
 await h.$('logout').click();assert.equal(h.state().activityCap,null);assert.equal(h.state().payoutObservation,null);assert.equal(h.$('payout-observation').textContent,'');
});
