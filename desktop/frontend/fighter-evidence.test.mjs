// Independent preimplementation wire/capture fixtures; no live native file or
// session authority is inferred from these presentation-only contexts.
import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';
import {readFileSync} from 'node:fs';
import test from 'node:test';
import vm from 'node:vm';
const bytes=readFileSync(new URL('./testdata/fighter-evidence-wire/wire-v1.json',import.meta.url));
assert.equal(createHash('sha256').update(bytes).digest('hex'),'c2489601d6c58440e001839c4ed14ce17dbaec43203a36ca197fa9ff89fc6dcb');
const corpus=JSON.parse(bytes),clone=x=>JSON.parse(JSON.stringify(x));
const coreBytes=readFileSync(new URL('../../fighterevidence/testdata/reference/reference-v1.json',import.meta.url));
assert.equal(createHash('sha256').update(coreBytes).digest('hex'),corpus.core_corpus_sha256);
const core=JSON.parse(coreBytes),source=readFileSync(new URL('./stable.js',import.meta.url),'utf8'),html=readFileSync(new URL('./stable.html',import.meta.url),'utf8');
const script=source.slice("import { Call } from '/wails/runtime.js';".length,source.indexOf("for (const kind of ['publish', 'reconcile', 'attest', 'export']) ui[`result-${kind}`].addEventListener"));
class Element {
  constructor(){this.children=[];this.listeners=new Map();this.attributes=new Map();this._text='';this.disabled=false;}
  set textContent(v){this._text=String(v);this.children=[];}get textContent(){return this._text+this.children.map(c=>c.textContent).join('');}
  set innerHTML(_){throw Error('Unsafe HTML');}append(...v){this.children.push(...v);}setAttribute(k,v){this.attributes.set(k,String(v));}removeAttribute(k){this.attributes.delete(k);}addEventListener(k,v){this.listeners.set(k,v);}
}
function harness(name=corpus.contexts[0].name){
  const c=clone(corpus.contexts.find(x=>x.name===name)),archive=JSON.parse(core.positives.find(x=>x.name===name).canonical),capture=archive.capture;
  const elements=new Map([...html.matchAll(/\bid="([^"]+)"/g)].map(m=>[m[1],new Element()])),calls=[],handlers={};
  const context=vm.createContext({document:{getElementById:id=>elements.get(id),createElement:()=>new Element(),hidden:false},window:{matchMedia:()=>({matches:false,addEventListener(){}})},Call:{async ByName(name,...args){const method=name.slice(9);calls.push({method,args:clone(args)});assert.ok(handlers[method],`Unexpected ${method}`);return clone(await handlers[method](...args));}},Intl,Date,btoa:s=>Buffer.from(s).toString('base64'),setTimeout(){return 1;},clearTimeout(){}});
  vm.runInContext(script,context);const run=s=>vm.runInContext(s,context),set=(key,v)=>{context.input=clone(v);run(`${key}=input`);};
  run('poolMode=true;careerMode=true');set('current',{state:'stopped',backup_state:'idle',account:{state:'signed_in',fixture:capture.status.pin.identity.gym_id==='a227587d837507dbcb5f9d781967339d816070fb7bd574931c690ba6937b88e5'?'zero256':'incrementing256'},view:c.view});
  set('stable',{state:'ready',view:c.view,status:capture.status,active_job_id:'',queued_job_id:''});set('careerActivation',capture.activation);set('historySnapshot',{activation:capture.activation});
  set('historyJob',{version:1,job_id:c.history_job_id,state:'done',view:c.view,store_id:c.store_id,activation_id:capture.activation.activation_id,prospect_id:c.prospect_id,error:'',capture});
  set('historySelection',{id:c.prospect_id,name:'Captured fighter',assignment:capture.assignment});run('evidenceIntent=evidenceBinding()');
  const value=(suffix,kind='job')=>clone(corpus.positives.find(x=>x.context_name===name&&x.name===name+'/'+suffix&&x.kind===kind).value);
  const response=(v,kind='job')=>({id:1,version:1,ok:true,snapshot:null,job:null,error:'',[kind]:clone(v)});
  const error=e=>({id:1,version:1,ok:false,snapshot:null,job:null,error:e});
  return{run,set,context,c,archive,capture,elements,calls,handlers,value,response,error};
}
test('independent metadata and response positives preserve each exact v1/v2 original capture',()=>{
  let count=0;for(const row of corpus.positives){if(row.kind==='request')continue;const h=harness(row.context_name);h.context.value=clone(row.value);if(row.previous)h.set('evidenceJob',row.previous);
    const fn=row.kind==='summary'?'evidenceSummary(value)':row.kind==='snapshot'?'evidenceSnapshotValue(value)':row.kind==='job'?'evidenceJobValue(value)':`evidenceEnvelope(value,'${row.command==='fighter_evidence_snapshot'?'snapshot':'job'}')`;
    h.run(fn);count++;}assert.equal(count,94);
});
test('independent foreign metadata, leakage and terminal mutation controls fail closed',()=>{
  let count=0;for(const row of corpus.negatives){if(row.kind==='request'||row.name==='wrong_response_correlation')continue;const h=harness(row.context_name);h.context.value=clone(row.value);if(row.previous)h.set('evidenceJob',row.previous);else if(row.name==='unknown_latest_not_adopted')h.set('evidenceJob',h.value('job_running'));
    const fn=row.kind==='summary'?'evidenceSummary(value)':row.kind==='snapshot'?'evidenceSnapshotValue(value)':row.kind==='job'?`evidenceJobValue(value,'${h.c.job_id}')`:`evidenceEnvelope(value,'job','${h.c.job_id}')`;
    assert.throws(()=>h.run(fn),row.name);count++;}assert.equal(count,26);
});
test('native begin receives only original View and history job; done shows checksum without a verification claim',async()=>{
  for(const c of corpus.contexts){const h=harness(c.name);h.handlers.BeginFighterEvidenceExport=()=>h.response(h.value('job_running'));await h.run('beginEvidence()');
    assert.deepEqual(h.calls,[{method:'BeginFighterEvidenceExport',args:[h.c.view.view_id,h.c.history_job_id]}]);assert.equal(h.run('evidenceBusy()'),true);assert.equal(h.elements.get('history-export').textContent,'Cancel evidence export');
    h.handlers.FighterEvidenceJob=()=>h.response(h.value('job_done'));await h.run('fetchEvidenceSnapshot()');assert.equal(h.run('evidenceBusy()'),false);assert.match(h.elements.get('history-export-status').textContent,/not an offline verification report/);assert.ok(h.elements.get('history-export-status').textContent.includes(c.summary.file_sha256));
    assert.equal(h.run('historyJob.capture.history_id'),c.history_id);assert.doesNotMatch(h.elements.get('history-export-status').textContent,/ownership verified|complete career|admission time verified/i);
  }
});
test('lost begin reconciles native-held operation without resending or adopting another capture',async()=>{
  const h=harness();h.handlers.BeginFighterEvidenceExport=()=>{throw Error('lost acknowledgement');};await h.run('beginEvidence()');assert.equal(h.run('evidenceUnknown'),true);assert.match(h.elements.get('history-export-status').textContent,/Checking whether/);
  h.handlers.FighterEvidenceSnapshot=()=>h.error('outcome_unknown');await h.run('fetchEvidenceSnapshot()');assert.equal(h.run('evidenceUnknown'),true);
  h.handlers.FighterEvidenceSnapshot=()=>h.response(h.value('snapshot_idleTrue','snapshot'),'snapshot');h.handlers.FighterEvidenceJob=()=>h.response(h.value('job_done'));await h.run('fetchEvidenceSnapshot()');
  assert.equal(h.run('evidenceJob.state'),'done');assert.equal(h.calls.filter(x=>x.method==='BeginFighterEvidenceExport').length,1);
  const other=clone(h.value('job_done'));other.history_id='f'.repeat(64);h.set('evidenceJob',null);h.set('evidenceUnknown',true);h.handlers.FighterEvidenceJob=()=>h.response(other);await h.run('fetchEvidenceSnapshot()');assert.equal(h.run('evidenceJob'),null);assert.equal(h.run('evidenceUnknown'),true);
});
test('dialog cancellation and definitive admission refusal never leave phantom work',async()=>{
  for(const response of [null,'busy','evidence_capacity']){const h=harness();h.handlers.BeginFighterEvidenceExport=()=>response===null?null:h.error(response);await h.run('beginEvidence()');assert.equal(h.run('evidenceBusy()'),false);assert.equal(h.run('evidenceJob'),null);assert.equal(h.run('evidenceIntent'),null);}
});
test('mining, backup, stable tasks and full display cache refuse new exports before native dialog',async()=>{
  for(const change of ["current.state='running'","current.backup_state='running'","stable.active_job_id='a'.repeat(64)","watched={id:'a'.repeat(64)}","pageItems=['x'.repeat(1048000)]"]){const h=harness();h.run(change);await h.run('beginEvidence()');assert.equal(h.calls.length,0);assert.equal(h.run('evidenceBusy()'),false);}
});
test('cancel waits for terminal metadata and publication uncertainty takes priority',async()=>{
  const h=harness();h.set('evidenceJob',h.value('job_running'));h.handlers.CancelFighterEvidence=()=>h.response(h.value('job_failedevidence_export_uncertain'));await h.run('cancelEvidence()');
  assert.deepEqual(h.calls,[{method:'CancelFighterEvidence',args:[h.c.view.view_id,h.c.job_id]}]);assert.equal(h.run('evidenceJob.state'),'failed');assert.match(h.elements.get('history-export-status').textContent,/may already be saved/);assert.equal(h.run('evidenceBusy()'),false);
});
test('old responses cannot cross logout, reset or selected history lifetime',async()=>{
  for(const invalidate of ['clearHistory()','epoch++',"historyJob={...historyJob,job_id:'e'.repeat(64)}"]){const h=harness();let release;h.handlers.BeginFighterEvidenceExport=()=>new Promise(r=>{release=()=>r(h.response(h.value('job_done')));});const pending=h.run('beginEvidence()');h.run(invalidate);release();await pending;assert.equal(h.run('evidenceJob'),null);}
});
test('a known job can never regress, change checksum, disclose extra report data or switch source',async()=>{
  for(const alter of [v=>{v.state='running';v.summary=null;},v=>{v.summary.file_sha256='e'.repeat(64);},v=>{v.summary.checks={};},v=>{v.view.authority_pin.descriptor_id='e'.repeat(64);}]){const h=harness();h.set('evidenceJob',h.value('job_done'));const prior=h.run('JSON.stringify(evidenceJob)'),bad=h.value('job_done');alter(bad);h.handlers.FighterEvidenceJob=()=>h.response(bad);await h.run(`fetchEvidenceJob('${h.c.job_id}')`);assert.equal(h.run('JSON.stringify(evidenceJob)'),prior);assert.equal(h.run('evidenceUnknown'),true);}
});
test('authorization errors clear private view metadata and keep fixed limits/copy',async()=>{
  const h=harness();h.set('evidenceJob',h.value('job_running'));h.handlers.FighterEvidenceJob=()=>h.error('stale_view');await h.run('fetchEvidenceSnapshot()');assert.equal(h.run('evidenceJob'),null);assert.equal(h.run('historyJob'),null);
  assert.match(html,/do not prove admission time, actual awards, ownership or a complete career/);assert.match(source,/const CACHE_MAX = 1 << 20/);assert.doesNotMatch(source,/\.innerHTML\s*=|crypto\.subtle/);
});

test('a second uncertain export never adopts the previous same-history terminal action',async()=>{
  const h=harness();h.set('evidenceJob',h.value('job_done'));
  h.handlers.BeginFighterEvidenceExport=()=>{throw Error('lost new action');};await h.run('beginEvidence()');
  assert.equal(h.run('evidencePriorJobID'),h.c.job_id);assert.equal(h.run('evidenceJob'),null);
  h.handlers.FighterEvidenceSnapshot=()=>h.response(h.value('snapshot_idleTrue','snapshot'),'snapshot');
  await h.run('fetchEvidenceSnapshot()');assert.equal(h.run('evidenceJob'),null);assert.equal(h.run('evidenceUnknown'),true);
  assert.equal(h.calls.filter(x=>x.method==='FighterEvidenceJob').length,0);
  // Exact native lookup found no new action. Generic prior last-job state is
  // filtered by native, and only the validated empty snapshot reaches JS.
  h.handlers.FighterEvidenceSnapshot=()=>h.response(h.value('snapshot_idleFalse','snapshot'),'snapshot');
  await h.run('fetchEvidenceSnapshot()');assert.equal(h.run('evidenceUnknown'),false);assert.equal(h.run('evidenceJob'),null);
  assert.match(h.elements.get('history-export-status').textContent,/No evidence export was started/);
  assert.equal(h.calls.filter(x=>x.method==='BeginFighterEvidenceExport').length,1);
  // Explicit user retry, new modeled transport-only operation ID. The public
  // history/capture is unchanged; no signed evidence or golden is relabeled.
  const nextID=corpus.contexts[1].job_id,newJob=h.value('job_done');newJob.job_id=nextID;
  const resolved=h.value('snapshot_idleTrue','snapshot');resolved.last_job_id=nextID;
  await h.run('beginEvidence()');h.handlers.FighterEvidenceSnapshot=()=>h.response(resolved,'snapshot');h.handlers.FighterEvidenceJob=()=>h.response(newJob);
  await h.run('fetchEvidenceSnapshot()');assert.equal(h.run('evidenceJob.job_id'),nextID);assert.equal(h.run('evidenceJob.history_id'),h.c.history_id);
  assert.equal(h.calls.filter(x=>x.method==='BeginFighterEvidenceExport').length,2);
  assert.deepEqual(h.calls.filter(x=>x.method==='FighterEvidenceJob').map(x=>x.args),[[h.c.view.view_id,nextID]]);
});
test('old same-history begin success and pre-dispatch failure cannot report a new export saved',async()=>{
  for(const directOld of [true,false]){const h=harness();h.set('evidenceJob',h.value('job_done'));
    h.handlers.BeginFighterEvidenceExport=()=>{if(directOld)return h.response(h.value('job_done'));throw Error('native preflight failed before remember');};
    await h.run('beginEvidence()');assert.equal(h.run('evidenceJob'),null);assert.equal(h.run('evidenceUnknown'),true);
    h.handlers.FighterEvidenceSnapshot=()=>h.response(h.value('snapshot_idleFalse','snapshot'),'snapshot');
    await h.run('fetchEvidenceSnapshot()');assert.equal(h.run('evidenceUnknown'),false);assert.equal(h.run('evidenceJob'),null);
    assert.doesNotMatch(h.elements.get('history-export-status').textContent,/Public evidence saved/);
  }
});
