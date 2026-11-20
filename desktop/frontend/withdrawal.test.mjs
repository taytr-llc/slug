// Relational UI/lifetime tests over independently frozen public transaction
// inputs. Synthetic namespace/action IDs are transport models, not authority.
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {createHash} from 'node:crypto';
import test from 'node:test';
import vm from 'node:vm';
const payouts=JSON.parse(readFileSync(new URL('./testdata/payout-history-wire/wire-v1.json',import.meta.url)));
const tx=JSON.parse(readFileSync(new URL('../../walletwithdraw/testdata/reference/journal-reference/wallet-reference-v1.json',import.meta.url))).cases[0];
const clone=v=>JSON.parse(JSON.stringify(v)),id=c=>c.repeat(64);
const source=readFileSync(new URL('./stable.js',import.meta.url),'utf8'),html=readFileSync(new URL('./stable.html',import.meta.url),'utf8');
const script=source.slice("import { Call } from '/wails/runtime.js';".length,source.indexOf("for (const kind of ['publish', 'reconcile', 'attest', 'export']) ui[`result-${kind}`].addEventListener"));
class Element{constructor(){this.children=[];this.listeners=new Map();this.value='';this.disabled=false;this.hidden=false;this.style={};this.attributes=new Map();}set textContent(v){this._text=String(v);this.children=[];}get textContent(){return(this._text||'')+this.children.map(x=>x.textContent).join('');}set innerHTML(_){throw Error('unsafe markup')}append(...v){this.children.push(...v)}addEventListener(k,v){this.listeners.set(k,v)}setAttribute(k,v){this.attributes.set(k,v)}removeAttribute(k){this.attributes.delete(k)}remove(){}}
function harness(){
 const p=payouts.bindings.zero256,binding={view:p.view,source_binding_id:p.source_binding_id,journal_id:id('e'),withdrawal_binding_id:id('f')},elements=new Map([...html.matchAll(/\bid="([^"]+)"/g)].map(m=>[m[1],new Element()])),handlers={},calls=[];
 const context=vm.createContext({document:{getElementById:k=>elements.get(k),createElement:()=>new Element(),hidden:false},window:{matchMedia:()=>({matches:false,addEventListener(){}})},Call:{async ByName(name,...args){const method=name.slice(9);calls.push({method,args:clone(args)});assert.ok(handlers[method],`unexpected ${method}`);return clone(await handlers[method](...args));}},Intl,Date,btoa:s=>Buffer.from(s).toString('base64'),setTimeout(){return 1},clearTimeout(){}});
 vm.runInContext(script,context);const run=s=>vm.runInContext(s,context),set=(k,v)=>{context.input=clone(v);run(`${k}=input`)};
 run('poolMode=true');set('current',{state:'stopped',backup_state:'idle',account:{state:'signed_in',fixture:'zero256',held:false},view:p.view,source_binding_id:p.source_binding_id});set('withdrawalBinding',binding);
 const now=Date.now(),d=tx.draft,draft={version:1,draft_digest:tx.draft_digest,unsigned_txid:tx.expected_signed.txid,destination:d.recipient,recipient_script:d.recipient_script,amount_atoms:d.amount_atoms,fee_atoms:d.fee_atoms,total_debit_atoms:d.amount_atoms+d.fee_atoms,input_count:d.inputs.length,input_atoms:d.total_input_atoms,recipient_output_index:d.recipient_output_index,change_atoms:d.change.atoms,change_output_index:d.change.output_index,prepared_at_ms:now-2000,observed_tip:d.tip_hash,observed_height:d.tip_height};
 const operation={version:1,operation_id:tx.request.operation_id,journal_id:binding.journal_id,withdrawal_binding_id:binding.withdrawal_binding_id,sequence:1,revision:2,created_at_ms:now-3000,updated_at_ms:now-2000,prepared_at_ms:now-2000,stage:'prepared',intent:{destination:d.recipient,recipient_script:d.recipient_script,amount_atoms:d.amount_atoms,fee_rate_atoms_per_kb:1000,fee_limit_atoms:100000},intent_digest:id('a'),draft,txid:'',signed_sha256:'',dispatch:null,observation:null};
 const review={version:1,binding,operation_id:operation.operation_id,action_id:id('b'),review_id:id('c'),kind:'send',draft,txid:'',reviewed_at_ms:now-1,expires_at_ms:now+10000};
 const snapshot={version:1,state:'idle',error:'',binding,active_action_id:'',last_action_id:'',unresolved_operation_id:operation.operation_id,latest_operation_id:operation.operation_id,can_prepare:false,can_review:true};
 const response=(arm,v)=>({id:1,version:1,ok:true,snapshot:null,operation:null,review:null,action:null,error:'',[arm]:clone(v)}),error=code=>({id:1,version:1,ok:false,snapshot:null,operation:null,review:null,action:null,error:code});
 return{run,set,context,elements,handlers,calls,binding,operation,review,snapshot,response,error};
}
test('withdrawal review uses native confirmation and exact integer display',async()=>{
 const h=harness();h.set('withdrawalSaved',h.operation);h.handlers.ReviewWithdrawal=()=>h.response('review',h.review);await h.run("invokeWithdrawal('review')");
 assert.deepEqual(h.calls.map(x=>x.method),['ReviewWithdrawal']);assert.equal(h.run('withdrawalReview.review_id'),h.review.review_id);assert.match(h.elements.get('withdrawal-detail').textContent,/Recipient 1\.00000000 SLUG\. Fee 0\.00000144 SLUG\. Total debit 1\.00000144 SLUG/);assert.match(h.elements.get('withdrawal-status').textContent,/native confirmation/);
 assert.doesNotMatch(source,/call\(['"](?:ApproveWithdrawal|withdrawal_approve)/);
});
test('zero-fee and foreign source review responses fail closed',()=>{
 const h=harness();h.set('withdrawalSaved',h.operation);const invalid=clone(h.review);invalid.draft.fee_atoms=0;invalid.draft.total_debit_atoms=invalid.draft.amount_atoms;invalid.draft.input_atoms=invalid.draft.total_debit_atoms+invalid.draft.change_atoms;h.context.value=invalid;assert.throws(()=>h.run('withdrawalReviewValue(value)'));
 const foreign=clone(h.review);foreign.binding.source_binding_id=id('9');h.context.value=foreign;assert.throws(()=>h.run('withdrawalReviewValue(value)'));
});
test('lost prepare cannot turn an older saved operation into the new action',async()=>{
 const h=harness();h.set('withdrawalSaved',h.operation);h.elements.get('withdrawal-destination').value=h.operation.intent.destination;h.elements.get('withdrawal-amount').value='1';h.handlers.BeginWithdrawal=()=>{throw Error('lost')};await h.run("invokeWithdrawal('prepare')");assert.equal(h.run('withdrawalUnknown'),true);
 h.handlers.WithdrawalSnapshot=()=>h.response('snapshot',h.snapshot);await h.run('fetchWithdrawal()');assert.equal(h.run('withdrawalUnknown'),false);assert.equal(h.run('withdrawalAction'),null);assert.equal(h.calls.filter(x=>x.method==='BeginWithdrawal').length,1);assert.equal(h.calls.filter(x=>x.method==='WithdrawalAction').length,0);assert.match(h.elements.get('withdrawal-status').textContent,/not started/);
});
test('lost pre-intent response recovers only exact native action with empty operation selector',async()=>{
 const h=harness();h.set('withdrawalUnknown',true);const snapshot={...h.snapshot,unresolved_operation_id:'',latest_operation_id:'',last_action_id:id('b')};h.handlers.WithdrawalSnapshot=()=>h.response('snapshot',snapshot);const a={version:1,binding:h.binding,operation_id:h.operation.operation_id,action_id:id('b'),kind:'prepare',state:'failed',review_id:'',error:'busy',operation:null};h.handlers.WithdrawalAction=()=>h.response('action',a);await h.run('fetchWithdrawal()');
 assert.deepEqual(h.calls.find(x=>x.method==='WithdrawalAction').args,[h.binding.view.view_id,'',id('b')]);assert.equal(h.run('withdrawalAction.state'),'failed');assert.equal(h.run('withdrawalUnknown'),false);
});
test('held view keeps transaction checks but disables new payment and retries',()=>{
 const h=harness();const o=clone(h.operation);o.stage='signed';o.txid=o.draft.unsigned_txid;o.signed_sha256=tx.signed_sha256;h.set('withdrawalSaved',o);h.run('current.account.held=true;renderWithdrawal()');assert.equal(h.elements.get('withdrawal-prepare').disabled,true);assert.equal(h.elements.get('withdrawal-retry').disabled,true);assert.equal(h.elements.get('withdrawal-observe').disabled,false);assert.equal(h.elements.get('withdrawal-refresh').disabled,false);
});
test('account change blanks withdrawal details and rejects late native result',async()=>{
 const h=harness();h.set('withdrawalSaved',h.operation);let release;h.handlers.ReviewWithdrawal=()=>new Promise(resolve=>{release=resolve});const pending=h.run("invokeWithdrawal('review')");await Promise.resolve();h.run('clearWithdrawal();epoch++');release(h.response('review',h.review));await pending;assert.equal(h.run('withdrawalReview'),null);assert.equal(h.run('withdrawalSaved'),null);assert.equal(h.elements.get('withdrawal-detail').textContent,'');
});

const wireBytes=readFileSync(new URL('./testdata/withdrawal-wire/wire-v1.json',import.meta.url));
assert.equal(createHash('sha256').update(wireBytes).digest('hex'),'60fe12e20e0c842a43cfc8f41892a8fcf4a7c991592363b17f87a8180098af9c');
const wire=JSON.parse(wireBytes);
function wireHarness(row){const h=harness(),binding=row.expected_binding;h.set('withdrawalBinding',binding);h.set('current',{state:'stopped',backup_state:'idle',account:{state:'signed_in',held:false},view:binding.view,source_binding_id:binding.source_binding_id});const v=row.value;if(v.review)h.set('withdrawalSaved',{operation_id:v.review.operation_id,draft:v.review.draft});return h;}
test('independent withdrawal wire response positives',()=>{
 let count=0;for(const row of wire.positives){if(row.kind!=='response')continue;const h=wireHarness(row),arm=['snapshot','operation','review','action'].find(k=>row.value[k]!==null)??'snapshot';h.context.value=clone(row.value);assert.doesNotThrow(()=>h.run(`withdrawalEnvelope(value,'${arm}')`),row.name);count++;}assert.equal(count,wire.positives.filter(x=>x.kind==='response').length);
});
test('independent withdrawal wire response negatives',()=>{
 let count=0;for(const row of wire.negatives){if(row.kind!=='response')continue;const h=wireHarness(row),arm=['snapshot','operation','review','action'].find(k=>row.value[k]!==null)??'snapshot';h.context.value=clone(row.value);assert.throws(()=>h.run(`withdrawalEnvelope(value,'${arm}')`),row.name);count++;}assert.equal(count,wire.negatives.filter(x=>x.kind==='response').length);
});
