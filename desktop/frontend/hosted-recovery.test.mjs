// Public, independently frozen allocation bytes adapted only into UI transport models.
// These tests do not assert freshly signed evidence or native ownership authority.
import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';
import {readFileSync} from 'node:fs';
import test from 'node:test';
import vm from 'node:vm';
const bytes=readFileSync(new URL('../../../slugwallet/wallet/testdata/hosted-allocation/reference.json',import.meta.url));
assert.equal(createHash('sha256').update(bytes).digest('hex'),'221fc42f6848fed27912a43454a1fa9ebe2d4e16b383aad9e294c31316424e6b');
const reference=JSON.parse(bytes),clone=v=>JSON.parse(JSON.stringify(v)),id=n=>n.toString(16).padStart(64,'0');
const source=readFileSync(new URL('./stable.js',import.meta.url),'utf8'),html=readFileSync(new URL('./stable.html',import.meta.url),'utf8');
const script=source.slice("import { Call } from '/wails/runtime.js';".length,source.indexOf("for (const kind of ['publish', 'reconcile', 'attest', 'export']) ui[`result-${kind}`].addEventListener"));
class Element {
 constructor(){this.children=[];this.listeners=new Map();this.attributes=new Map();this._text='';this.disabled=false;this.hidden=false;this.style={};}
 set textContent(v){this._text=String(v);this.children=[];}get textContent(){return this._text+this.children.map(c=>c.textContent).join('');}
 set innerHTML(_){throw Error('Unsafe HTML');}append(...v){this.children.push(...v);}setAttribute(k,v){this.attributes.set(k,String(v));}removeAttribute(k){this.attributes.delete(k);}addEventListener(k,v){this.listeners.set(k,v);}remove(){}
}
function fixture(name='zero256') {
 const index=name==='zero256'?0:1,b=reference.bindings[index].binding,m=b.initial_manifest,s=reference.states.find(x=>x.name===name+'/receive-complete').snapshot,link=s.local_links.at(-1);
 const account={state:'signed_in',fixture:name,id:b.account_id,generation:1,hold_until_ms:0,held:false,can_export:true,last_error:''};
 const vault={version:1,...Object.fromEntries(['authority_id','account_id','principal_id','provision_id'].map(k=>[k,b[k]])),...Object.fromEntries(['network','genesis','profile','root_public_key','gym_public_key','gym_id'].map(k=>[k,m[k]]))};
 const allocation={version:1,inventory_id:b.inventory_id,inventory_binding_id:reference.bindings[index].binding_id,current_binding:{version:1,hosted_vault_id:b.hosted_vault_id,vault_binding:vault,retention_id:b.retention_id,wallet_owner_id:b.wallet_owner_id,source_binding_id:b.source_binding_id}};
 const view={version:1,view_id:id(900+index),account_authority_id:b.authority_id,account_id:b.account_id,generation:1,authority_pin:{version:5,network:m.network,genesis:m.genesis,authority_id:id(1000),descriptor_id:'629f941f194c8f127ecf2f7d82b7b972a40501755534854c801fbe1c37f42fcd'},authority_origin:'http://127.0.0.1:32123',transport_trust_id:id(1001)};
 const pool={version:1,mode:'local-hosted-recovery-pool',state:'stopped',attempts:0,hashes_per_second:0,height:0,pending:0,last_error:'',recovery_error:'',wallet_state:'ready',wallet_height:0,payout_address:s.local_links[0].address,gym_id:m.gym_id,gym_public_key:m.gym_public_key,spendable_atoms:0,immature_atoms:0,backup_state:'idle',backup_phase:'',backup_error:'',account:clone(account),view,assignment_count:0,source_binding_id:b.source_binding_id,balance:{state:'unavailable',error:'',value:null}};
 const root={version:1,mode:'local-hosted-recovery-pool',kind:'healthy',account,healthy:{pool,allocation,pending_receive_id:'',issued_receive_id:link.allocation_id},recovery:null,error:''};
 const recovery=clone(root);recovery.kind='recovery_only';recovery.healthy=null;recovery.recovery={view:{version:1,view_id:id(2000+index),account_authority_id:b.authority_id,account_id:b.account_id,generation:1,fixture:name,root_public_key:m.root_public_key,gym_public_key:m.gym_public_key,gym_id:m.gym_id},allocation:clone(allocation),local_wallet:'missing',inventory_revision:s.head.revision,observed_at_ms:1795070717840,backup:{state:'idle',phase:'',error:''}};
 const inactive={version:1,mode:root.mode,kind:'inactive',account:{state:'signed_out',fixture:'',id:'',generation:0,hold_until_ms:0,held:false,can_export:false,last_error:''},healthy:null,recovery:null,error:''};
 const address={address:link.address,script:link.script,purpose:84,coin:0,account:0,branch:0,index:link.selected_index,inventory_revision:s.head.revision};
 const job=(state='ready',action=id(3000),operation=link.allocation_id)=>({version:1,action_id:action,operation_id:operation,view_id:view.view_id,allocation:clone(allocation),state,error:state==='unknown'?'outcome_unknown':state==='needs_resume'?'allocation_resume_required':state==='cancelled'?'cancelled':state==='failed'?'allocation_unavailable':'',address:state==='ready'?clone(address):null});
 return {root,recovery,inactive,job,address};
}
function harness(name='zero256') {
 const f=fixture(name),elements=new Map([...html.matchAll(/\bid="([^"]+)"/g)].map(m=>[m[1],new Element()])),calls=[],handlers={},copied=[];
 const context=vm.createContext({document:{getElementById:id=>elements.get(id),createElement:()=>new Element(),hidden:false},window:{matchMedia:()=>({matches:false,addEventListener(){}})},navigator:{clipboard:{async writeText(v){copied.push(v);}}},Call:{async ByName(name,...args){const method=name.slice(9);calls.push({method,args:clone(args)});assert.ok(handlers[method],`Unexpected ${method}`);return clone(await handlers[method](...args));}},Intl,Date,btoa:s=>Buffer.from(s).toString('base64'),setTimeout(){return 1;},clearTimeout(){}});
 vm.runInContext(script,context);const run=s=>vm.runInContext(s,context),set=(name,v)=>{context.input=clone(v);run(`${name}=input`);};
 run('enabled=true;poolMode=true;hostedPoolMode=true;hostedRecoveryMode=true');
 // Pin the supported descriptor from the existing closed tuple table, not an invented tuple.
 context.root=f.root;run('root.healthy.pool.view.authority_pin.descriptor_id=DESCRIPTOR');f.root=clone(context.root);
 set('hostedRoot',f.root);set('current',f.root.healthy.pool);run('normalizeHosted(hostedRoot)');
 handlers.StableSnapshot=()=>({id:1,ok:true,snapshot:{version:1,state:'unavailable',error:'',view:null,status:null,active_job_id:'',queued_job_id:'',last_job_id:'',pending_operation_id:''}});
 handlers.WithdrawalSnapshot=()=>{throw Error('Unavailable modeled withdrawal');};
 return {f,run,set,context,elements,calls,handlers,copied};
}
function refusal(code){return Error(`The hosted recovery request could not complete (${code}).`);}

test('typed inactive and missing-wallet roots never poll or fabricate a wallet child',async()=>{
 for(const name of ['zero256','incrementing256'])for(const kind of ['inactive','recovery']){const h=harness(name);h.handlers.HostedRecoverySnapshot=()=>h.f[kind];await h.run('fetchSnapshots()');assert.deepEqual(h.calls.map(x=>x.method),['HostedRecoverySnapshot']);assert.equal(h.run('current'),null);assert.equal(h.run('stable'),null);assert.equal(h.elements.get('spendable').textContent,'—');assert.equal(h.elements.get('pool-balance').hidden,true);assert.equal(h.elements.get('mining').disabled,true);assert.equal(h.elements.get('hosted-receive-panel').hidden,true);assert.equal(h.elements.get('verify-backup').disabled,kind==='inactive');assert.equal(h.elements.get('export-recovery').disabled,kind==='inactive');}
});
test('healthy union reads the root before existing stable and wallet surfaces',async()=>{
 const h=harness();h.handlers.HostedRecoverySnapshot=()=>h.f.root;await h.run('fetchSnapshots()');assert.deepEqual(h.calls.slice(0,2).map(x=>x.method),['HostedRecoverySnapshot','StableSnapshot']);assert.equal(h.calls.some(x=>x.method==='PoolSnapshot'),false);assert.equal(h.run('current.mode'),'local-hosted-recovery-pool');assert.equal(h.elements.get('hosted-receive-address').textContent,'');assert.equal(h.elements.get('hosted-receive-show').disabled,false);
});
test('union rejects mode, arm, fixture and complete allocation-scope substitutions',()=>{
 const mutations=[x=>x.mode='local-hosted-fixture-pool',x=>x.recovery={},x=>x.account.id=id(888),x=>x.healthy.pool.mode='local-pool-pilot',x=>x.healthy.allocation.current_binding.source_binding_id=id(888),x=>x.healthy.allocation.current_binding.vault_binding.gym_id=id(888),x=>x.healthy.pending_receive_id=x.healthy.issued_receive_id];
 for(const change of mutations){const h=harness(),bad=clone(h.f.root);change(bad);h.context.value=bad;assert.throws(()=>h.run('normalizeHosted(value)'));}
 const h=harness();for(const change of [x=>x.account.id=id(88),x=>x.healthy=h.f.root.healthy,x=>x.mode='local-pool-pilot']){const bad=clone(h.f.inactive);change(bad);h.context.value=bad;assert.throws(()=>h.run('normalizeHosted(value)'));}
});
test('recovery hold preserves verify and forbids export without enabling mining or payments',async()=>{
 const h=harness(),r=h.f.recovery;r.account.held=true;r.account.can_export=false;r.account.hold_until_ms=1795157117840;r.recovery.backup={state:'running',phase:'syncing',error:''};h.handlers.HostedRecoverySnapshot=()=>r;await h.run('fetchSnapshots()');assert.equal(h.elements.get('export-recovery').disabled,true);assert.equal(h.elements.get('verify-backup').disabled,false);assert.match(h.elements.get('verify-backup').textContent,/Cancel/);assert.match(h.elements.get('backup').textContent,/Sync/);assert.equal(h.run('hostedBackup().state'),'running');assert.equal(h.elements.get('withdrawal-prepare').disabled,true);
});
test('explicit receive retains exact native action and displays only acknowledged independent address',async()=>{
 const h=harness();h.handlers.NewReceiveAddress=()=>h.f.job('running');h.handlers.ReceiveAddressStatus=()=>h.f.job();await h.run("beginHostedReceive('NewReceiveAddress')");assert.equal(h.run('hostedReceiveBusy()'),true);assert.equal(h.elements.get('hosted-receive-address').textContent,'');await h.run('fetchHostedReceive()');assert.equal(h.run('hostedReceiveBusy()'),false);assert.equal(h.elements.get('hosted-receive-address').textContent,h.f.address.address);await h.run('copyHostedReceive()');assert.deepEqual(h.copied,[h.f.address.address]);const n=h.calls.length;await h.run('fetchHostedReceive()');assert.equal(h.calls.length,n);
});
test('resume and redisplay use explicit original allocation pointers and a fresh native action',async()=>{
 for(const method of ['ResumeReceiveAddress','RedisplayReceiveAddress']){const h=harness();h.run("hostedRoot.healthy.pending_receive_id=hostedRoot.healthy.issued_receive_id;hostedRoot.healthy.issued_receive_id=''");if(method==='RedisplayReceiveAddress')h.run("hostedRoot.healthy.issued_receive_id=hostedRoot.healthy.pending_receive_id;hostedRoot.healthy.pending_receive_id=''");h.handlers[method]=()=>h.f.job();await h.run(`beginHostedReceive('${method}')`);assert.equal(h.run('hostedReceive.state'),'ready');assert.deepEqual(h.calls.map(x=>x.method),[method]);}
 const h=harness();h.handlers.RedisplayReceiveAddress=()=>h.f.job('ready',id(3000),id(888));await h.run("beginHostedReceive('RedisplayReceiveAddress')");assert.equal(h.run('hostedReceive'),null);assert.equal(h.run('hostedReceiveUnknown'),true);
});
test('lost begin recovers only the exact current native action and rejects prior terminal reuse',async()=>{
 const h=harness();h.set('hostedReceive',h.f.job());h.handlers.NewReceiveAddress=()=>{throw Error('Lost response');};await h.run("beginHostedReceive('NewReceiveAddress')");h.handlers.ReceiveAddressStatus=()=>h.f.job();await h.run('fetchHostedReceive()');assert.equal(h.run('hostedReceive'),null);assert.equal(h.run('hostedReceiveUnknown'),true);
 h.handlers.ReceiveAddressStatus=()=>h.f.job('ready',id(3001),id(4001));await h.run('fetchHostedReceive()');assert.equal(h.run('hostedReceive.action_id'),id(3001));assert.equal(h.calls.filter(x=>x.method==='NewReceiveAddress').length,1);
});
test('only fixed native exact-not-found or resume-required resolves uncertain dispatch',async()=>{
 for(const code of ['allocation_not_found','allocation_resume_required','allocation_unavailable']){const h=harness();h.handlers.NewReceiveAddress=()=>{throw refusal('busy');};await h.run("beginHostedReceive('NewReceiveAddress')");h.handlers.ReceiveAddressStatus=()=>{throw refusal(code);};await h.run('fetchHostedReceive()');assert.equal(h.run('hostedReceiveUnknown'),code==='allocation_unavailable');assert.equal(h.calls.filter(x=>x.method==='NewReceiveAddress').length,1);}
 const h=harness();h.handlers.NewReceiveAddress=()=>{throw Error('no dispatch');};await h.run("beginHostedReceive('NewReceiveAddress')");h.handlers.ReceiveAddressStatus=()=>{throw Error('allocation_not_found private path');};await h.run('fetchHostedReceive()');assert.equal(h.run('hostedReceiveUnknown'),true);assert.doesNotMatch(h.elements.get('hosted-receive-status').textContent,/private path/);
});
test('cancel remains busy until the exact terminal response and never releases on lookup noise',async()=>{
 const h=harness();h.set('hostedReceive',h.f.job('running'));h.handlers.CancelReceiveAddress=()=>h.f.job('running');await h.run('fetchHostedReceive(true)');assert.equal(h.run('hostedReceiveBusy()'),true);h.handlers.ReceiveAddressStatus=()=>h.f.job('cancelled');await h.run('fetchHostedReceive()');assert.equal(h.run('hostedReceiveBusy()'),false);assert.equal(h.elements.get('hosted-receive-address').textContent,'');
});
test('old view, allocation scope and terminal regressions cannot publish receive replies',async()=>{
 for(const change of [x=>x.view_id=id(888),x=>x.allocation.inventory_binding_id=id(888),x=>x.address.index=0,x=>x.address.branch=1,x=>x.address.script='00',x=>{x.state='running';}]){const h=harness(),bad=h.f.job();change(bad);h.handlers.NewReceiveAddress=()=>bad;await h.run("beginHostedReceive('NewReceiveAddress')");assert.equal(h.run('hostedReceive'),null);assert.equal(h.elements.get('hosted-receive-address').textContent,'');}
 const h=harness();h.set('hostedReceive',h.f.job());h.context.value=h.f.job('running');assert.throws(()=>h.run('hostedReceiveValue(value,hostedReceiveContext())'));
});
test('logout/root invalidation drops delayed begins and clears copyable addresses',async()=>{
 for(const method of ['NewReceiveAddress','ReceiveAddressStatus']){const h=harness();let release;h.handlers[method]=()=>new Promise(r=>{release=r;});if(method==='ReceiveAddressStatus')h.set('hostedReceive',h.f.job('running'));const pending=h.run(method==='NewReceiveAddress'?"beginHostedReceive('NewReceiveAddress')":'fetchHostedReceive()');h.run('forgetView();hostedRoot=null;current=null;renderHostedReceive()');release(h.f.job());await pending;assert.equal(h.run('hostedReceive'),null);assert.equal(h.elements.get('hosted-receive-address').textContent,'');await h.run('copyHostedReceive()');assert.equal(h.copied.length,0);}
});
test('mining and backup block new allocation while a held View still permits receive',async()=>{
 for(const statement of ["current.state='running'","current.backup_state='running'"]){const h=harness();h.run(statement);await h.run("beginHostedReceive('NewReceiveAddress')");assert.equal(h.calls.length,0);h.run('renderHostedReceive()');assert.equal(h.elements.get('hosted-receive-new').disabled,true);}
 const h=harness();h.run('current.account.held=true;current.account.can_export=false');h.handlers.NewReceiveAddress=()=>h.f.job();await h.run("beginHostedReceive('NewReceiveAddress')");assert.equal(h.run('hostedReceive.state'),'ready');
});
test('native cached definite refusal resolves only as its exact correlated terminal job',async()=>{
 for(const code of ['busy','allocation_capacity','allocation_conflict']){const h=harness();h.handlers.NewReceiveAddress=()=>{throw refusal(code);};await h.run("beginHostedReceive('NewReceiveAddress')");const terminal=h.f.job('failed');terminal.error=code;h.handlers.ReceiveAddressStatus=()=>terminal;await h.run('fetchHostedReceive()');assert.equal(h.run('hostedReceiveBusy()'),false);assert.equal(h.run('hostedReceive.error'),code);assert.equal(h.elements.get('hosted-receive-address').textContent,'');assert.equal(h.calls.filter(x=>x.method==='NewReceiveAddress').length,1);}
});
test('pending and failed new action retain only a labeled same-view prior address; copying waits',async()=>{
 const h=harness();h.handlers.NewReceiveAddress=()=>h.f.job();await h.run("beginHostedReceive('NewReceiveAddress')");assert.equal(h.elements.get('hosted-receive-address').textContent,h.f.address.address);
 let release;h.handlers.NewReceiveAddress=()=>new Promise(r=>{release=r;});const pending=h.run("beginHostedReceive('NewReceiveAddress')");assert.equal(h.run('hostedReceive'),null);assert.equal(h.elements.get('hosted-receive-address').textContent,h.f.address.address);assert.match(h.elements.get('hosted-receive-status').textContent,/Previously issued.*not the outcome/);assert.equal(h.elements.get('hosted-receive-copy').disabled,true);await h.run('copyHostedReceive()');assert.equal(h.copied.length,0);
 const failed=h.f.job('failed',id(3002),id(4002));release(failed);await pending;assert.equal(h.run('hostedReceive.state'),'failed');assert.equal(h.elements.get('hosted-receive-address').textContent,h.f.address.address);assert.match(h.elements.get('hosted-receive-status').textContent,/Previously issued/);await h.run('copyHostedReceive()');assert.deepEqual(h.copied,[h.f.address.address]);h.run('forgetView();renderHostedReceive()');assert.equal(h.elements.get('hosted-receive-address').textContent,'');assert.equal(h.run('hostedReceiveDisplayed'),null);
});
