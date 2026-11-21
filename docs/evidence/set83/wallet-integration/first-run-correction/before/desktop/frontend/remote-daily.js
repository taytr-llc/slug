import { Call } from '/wails/runtime.js';
// Only the selected concrete native owner can call the original helper. This
// page supplies public correlation, never a path, token, origin or RPC method.
const $ = id => document.getElementById(id);
const text = (id,value) => { $(id).textContent=String(value??''); };
const need = value => { if (!value) throw new Error('Unverified daily wallet result'); };
const exact = (v,keys) => need(v && typeof v==='object' && !Array.isArray(v) && JSON.stringify(Object.keys(v))===JSON.stringify(keys));
const id = v => typeof v==='string' && /^[0-9a-f]{64}$/.test(v);
const safe = (v,max=Number.MAX_SAFE_INTEGER) => Number.isSafeInteger(v) && v>=0 && v<=max;
const uint = (v,max=18446744073709551615n) => { need(typeof v==='string' && v.length<=20 && /^(0|[1-9][0-9]*)$/.test(v));const n=BigInt(v);need(n<=max);return n; };
const int64 = v => uint(v,9223372036854775807n);
const atoms = v => { const n=typeof v==='bigint'?v:BigInt(v);return `${n/100000000n}.${(n%100000000n).toString().padStart(8,'0')}`; };
const human = v => String(v??'').replaceAll('_',' ').replaceAll('-',' ');
const time = v => v===0?'Not yet observed':new Date(v).toISOString();
const own = v => structuredClone(v);
const same = (a,b) => JSON.stringify(a)===JSON.stringify(b);
const methods=new Set(['Snapshot','LoginFixture','Start','Stop','Logout','BeginDailyPayoutRead','DailyPayoutRead','CancelDailyPayoutRead']);
const call=(name,...args) => { need(methods.has(name));return Call.ByName(`main.DailyRemoteGym.${name}`,...args); };
let snapshot=null,configID='',fixture='',busy=false,retired=false,epoch=0;
let attempt=null,capture=null,shown=null,nextSequence=1n,lastReply=0n,readDeadline=0;
const view=()=>snapshot?.view?.view_id??'';
const context=()=>({view:view(),epoch});
const current=c=>c.epoch===epoch && c.view===view() && !!c.view;
function message(v){text('alert',v);$('alert').hidden=!v;}
function clearHistory(){attempt=capture=shown=null;nextSequence=1n;readDeadline=0;for(const k of ['pool-confirmed','pool-available','pool-reserved','pool-paid'])text(k,'—');for(const k of ['history-message','history-date','history-window','pool-reason'])text(k,'');text('history-state','Not captured');$('history-rows').replaceChildren();}
function clearStatus(){epoch++;snapshot=null;clearHistory();for(const k of ['state','account-state','wallet-state'])text(k,'Unavailable');for(const k of ['attempts','rate','pending','height','spendable','immature','unconfirmed','payout','gym-key'])text(k,'—');}
function controls(){
 const signed=snapshot?.state==='signed_in' && id(view());
 for(const [key,want]of[['login-a','zero256'],['login-b','incrementing256']]){$(key).disabled=busy||retired||!snapshot||signed||snapshot.state!=='signed_out'||fixture!==want;$(key).hidden=!!fixture&&fixture!==want;}
 $('logout').disabled=busy||retired||!signed;
 $('mining').disabled=busy||retired||!signed||!['running','stopped','failed'].includes(snapshot?.remote?.state);
 const working=attempt?.state==='working';
 $('history-refresh').disabled=busy||retired||!signed||working;
 $('history-refresh').textContent=attempt?.state==='unacknowledged'?'Resolve this refresh':'Refresh pool history';
 $('history-cancel').disabled=busy||retired||!signed||!attempt||!['working','unacknowledged'].includes(attempt.state);
 $('history-first').disabled=busy||retired||!signed||working||attempt?.state==='unacknowledged'||!capture||shown?.before_batch===capture.first.before_batch;
 $('history-next').disabled=busy||retired||!signed||working||attempt?.state==='unacknowledged'||!capture||capture.stale||capture.current.done;
}
function envelope(r,command,c){
 exact(r,['version','application','id','command','config_id','view_id','ok','snapshot','job','read','error']);
 need(JSON.stringify(r).length+1<=65536 && r.version===1 && r.application==='SLUG-REMOTE-DAILY-WALLET-1' && r.command===command && id(r.config_id) && (!configID||r.config_id===configID));
 const n=uint(r.id);need(n>lastReply);lastReply=n;
 need(r.view_id===(c?.view??'') && r.ok===true && r.error==='');
 const arm=command==='daily_payout_begin'?'job':command.startsWith('daily_payout_')?'read':'snapshot';
 for(const k of ['snapshot','job','read'])need(k===arm?r[k]!==null:r[k]===null);
 return r[arm];
}
function checkSnapshot(s){
 exact(s,['version','config_id','state','account','view','remote','error']);need(s.version===2&&id(s.config_id)&&(!configID||s.config_id===configID));
 const a=s.account;exact(a,['state','fixture','id','generation','hold_until_ms','held','can_export','last_error']);need(a.state===s.state&&['zero256','incrementing256'].includes(a.fixture)&&(!fixture||fixture===a.fixture)&&safe(a.generation)&&safe(a.hold_until_ms)&&typeof a.held==='boolean'&&a.can_export===false&&a.last_error===s.error);
 if(s.state==='signed_in'){
  const v=s.view;exact(v,['version','view_id','account_authority_id','account_id','generation','fixture','root_public_key','gym_public_key','gym_id']);need(v.version===1&&id(v.view_id)&&id(v.account_authority_id)&&id(v.account_id)&&v.account_id===a.id&&v.generation===a.generation&&v.generation>0&&v.fixture===a.fixture&&id(v.root_public_key)&&id(v.gym_public_key)&&id(v.gym_id)&&s.error===''&&(!a.held||a.hold_until_ms>0));
  const r=s.remote;exact(r,['version','config_id','view','source_binding_id','original_source_binding_id','state','worker_state','last_error','attempts','hashes_per_second_milli','records','pending','wallet_state','wallet_height','spendable_atoms','immature_atoms','unconfirmed_atoms','payout_address']);need(r.version===2&&r.config_id===s.config_id&&same(r.view,v));
  for(const k of ['attempts','hashes_per_second_milli','records','pending'])need(safe(r[k]));need(safe(r.wallet_height,2147483647)&&r.pending<=r.records);
  for(const k of ['spendable_atoms','immature_atoms','unconfirmed_atoms'])need(safe(r[k],2100000000000000));
  need(id(r.source_binding_id)&&id(r.original_source_binding_id)&&['stopped','starting','running','failed'].includes(r.state)&&['ready','stopped','failed','paused','preparing','retrying','hashing','delivering','unavailable'].includes(r.worker_state)&&['unavailable','syncing','ready'].includes(r.wallet_state)&&typeof r.payout_address==='string'&&r.payout_address.length<=100);
  need(r.state==='running'||r.hashes_per_second_milli===0);if(r.wallet_state==='unavailable')need(r.wallet_height===0&&r.spendable_atoms===0&&r.immature_atoms===0&&r.unconfirmed_atoms===0);
 }else{need(['signed_out','expired','failed'].includes(s.state)&&s.view===null&&s.remote===null&&a.id===''&&a.generation===0&&a.hold_until_ms===0&&!a.held);need(s.state==='signed_out'?s.error==='':s.state==='failed'?s.error==='remote_unavailable':['','remote_unavailable'].includes(s.error));}
 return s;
}
function renderSnapshot(s){
 checkSnapshot(s);const old=view();configID=s.config_id;fixture=s.account.fixture;snapshot=own(s);
 if(old!==view()){epoch++;clearHistory();}
 const r=s.remote;text('state',human(r?.state??s.state));text('account-state',human(s.state));text('hold',s.account.held?'This account is on a security hold.':'');
 text('mining-copy',view()?'Mining uses this gym’s existing wallet and selected remote pool.':'Sign in to attach your existing wallet.');text('mining',r?.state==='running'?'Stop mining':'Start mining');
 text('attempts',r?.attempts??0);text('rate',`${((r?.hashes_per_second_milli??0)/1000).toFixed(2)} hashes / second`);text('pending',r?.pending??0);text('height',r?.wallet_height??0);text('wallet-state',human(r?.wallet_state??'closed'));
 for(const [k,f]of[['spendable','spendable_atoms'],['immature','immature_atoms'],['unconfirmed','unconfirmed_atoms']])text(k,r?atoms(r[f]):'—');text('payout',r?.payout_address||'—');text('gym-key',s.view?.gym_public_key||'—');
 if(r?.last_error)message(`Remote mining status: ${human(r.last_error)}. Pending delivery remains retained.`);
}
function fraction(v){exact(v,['numerator','denominator']);for(const k of ['numerator','denominator'])need(typeof v[k]==='string'&&v[k].length<=4096&&/^(0|[1-9][0-9]*)$/.test(v[k]));const n=BigInt(v.numerator),d=BigInt(v.denominator);need(d>0n);let a=n,b=d;while(b){const r=a%b;a=b;b=r;}need(a===1n);return {n,d};}
function fractionText(v){const {n,d}=fraction(v);return atoms(n/d)+(n%d?' + fractional atoms retained':'');}
const reasons=new Set(['','payout-pending','broadcast-unknown','transaction-unavailable','payout-conflict','funding-unavailable','funding-conflict','funding-changed','immature','awaiting-confirmation','below-minimum','destination-unavailable','daily-batch-complete','fee-unavailable','fee-budget','change-dust','signing-unavailable','capacity','transaction-capacity']);
function balance(b,gym){
 exact(b,['gym_id','confirmed','available','reserved_atoms','paid_atoms','reason']);need(b.gym_id===gym&&reasons.has(b.reason));const a=fraction(b.confirmed),v=fraction(b.available);const reserved=int64(b.reserved_atoms);int64(b.paid_atoms);need(a.n>=reserved*a.d&&v.n*a.d<=(a.n-reserved*a.d)*v.d&&(a.n!==0n||b.reason===''));return b;
}
function binding(b){
 exact(b,['view','config_id','source_binding_id','settlement_config_id','recipient_script','issue_profile_id','original_source_binding_id','wallet_owner_id','allocation_binding_id']);
 need(same(b.view,snapshot.view)&&b.config_id===configID&&b.source_binding_id===snapshot.remote.source_binding_id&&b.original_source_binding_id===snapshot.remote.original_source_binding_id);
 for(const k of ['settlement_config_id','issue_profile_id','wallet_owner_id','allocation_binding_id'])need(id(b[k]));need(/^0014[0-9a-f]{40}$/.test(b.recipient_script));
 if(capture)need(same(capture.binding,b));return b;
}
function page(p,b){
 exact(p,['version','binding_id','settlement_config_id','revision','high_batch','before_batch','next_before','done','recipient_script','balance','observed_at_ms','items']);
 need(p.version===2&&p.binding_id===b.source_binding_id&&p.settlement_config_id===b.settlement_config_id&&p.recipient_script===b.recipient_script&&safe(p.observed_at_ms,253402300799999));
 need(uint(p.revision)>0n);const high=uint(p.high_batch,32767n),before=uint(p.before_batch,32768n),next=uint(p.next_before,32768n);need(before>0n&&before<=high+1n&&typeof p.done==='boolean'&&p.done===(before<=9n)&&next===(p.done?0n:before-8n));balance(p.balance,b.view.gym_id);
 need(Array.isArray(p.items)&&p.items.length<=8);if(p.observed_at_ms===0)need(high===0n&&p.items.length===0&&p.balance.confirmed.numerator==='0'&&p.balance.available.numerator==='0'&&p.balance.reserved_atoms==='0'&&p.balance.paid_atoms==='0'&&p.balance.reason==='');let prior=before,day='',created=0,paid=0n,reserved=0n;const seen=new Set();
 for(const row of p.items){exact(row,['batch_sequence','payment']);const seq=uint(row.batch_sequence,32767n);need(seq>0n&&seq<prior&&seq>=(before>9n?before-8n:1n));prior=seq;const x=row.payment;exact(x,['day','created_at_ms','txid','output_index','atoms','recorded_state']);need(/^\d{4}-\d\d-\d\d$/.test(x.day)&&safe(x.created_at_ms,253402300799999)&&x.created_at_ms>0&&x.created_at_ms<=p.observed_at_ms&&id(x.txid)&&!seen.has(x.txid)&&safe(x.output_index,255)&&safe(x.atoms,2100000000000000)&&x.atoms>=100000000&&['prepared','broadcast','confirmed'].includes(x.recorded_state));need(new Date(x.created_at_ms).toISOString().slice(0,10)===x.day&&(!day||(x.day<day&&x.created_at_ms<created)));day=x.day;created=x.created_at_ms;if(x.recorded_state==='confirmed')paid+=BigInt(x.atoms);else{need(seq===high);reserved+=BigInt(x.atoms);}seen.add(x.txid);}need(paid<=int64(p.balance.paid_atoms)&&reserved<=int64(p.balance.reserved_atoms));
 return p;
}
function checkedCapture(x){
 if(x===null)return null;exact(x,['sequence','binding','captured_at_ms','stale','first','current']);need(uint(x.sequence)>0n&&safe(x.captured_at_ms,253402300799999)&&x.captured_at_ms>0&&typeof x.stale==='boolean');const b=binding(x.binding);const a=page(x.first,b),p=page(x.current,b);need(uint(a.before_batch)===uint(a.high_batch)+1n&&uint(p.before_batch)<=uint(a.before_batch)&&(uint(a.before_batch)-uint(p.before_batch))%8n===0n);
 for(const k of ['revision','high_batch','balance','observed_at_ms'])need(same(a[k],p[k]));if(a.before_batch===p.before_batch)need(same(a,p));return own(x);
}
function job(j,want){exact(j,['sequence','capture_sequence','before_batch','state','error']);need(j.sequence===want.sequence&&j.capture_sequence===want.capture_sequence&&j.before_batch===want.before_batch);need(uint(j.sequence)>0n&&uint(j.capture_sequence)<uint(j.sequence)&&((j.capture_sequence==='0')===(j.before_batch==='0')));uint(j.before_batch,32768n);need(['working','done','failed','cancelled'].includes(j.state));need(j.state==='failed'?['busy','rate_limited','stale_capture','unauthorized','unavailable','cancelled'].includes(j.error):j.error==='');return own(j);}
function renderHistory(){
 text('history-state',capture?.stale?'Captured history changed':attempt?human(attempt.state):'Not captured');
 if(!capture)return;
 const p=shown??capture.current,b=p.balance;
 text('pool-confirmed',fractionText(b.confirmed));text('pool-available',fractionText(b.available));text('pool-reserved',atoms(int64(b.reserved_atoms)));text('pool-paid',atoms(int64(b.paid_atoms)));text('pool-reason',b.reason?`Payment status: ${human(b.reason)}.`:'');
 text('history-date',`Captured ${time(capture.captured_at_ms)}. Settlement observed ${time(p.observed_at_ms)}.${capture.stale?' Refresh explicitly to continue.':''}`);
 text('history-window',p.done?'End of this captured history.':'More captured batch slots remain.');
 const list=$('history-rows');list.replaceChildren();
 for(const row of p.items){const x=row.payment,li=document.createElement('li');li.textContent=`${x.day}: ${atoms(x.atoms)} test SLUG — pool record ${human(x.recorded_state)}. Transaction ${x.txid}, output ${x.output_index}.`;list.appendChild(li);}
 if(p.items.length===0){const li=document.createElement('li');li.textContent=p.done?'No payouts in this captured window.':'No payouts for this gym in these batch slots; more slots remain.';list.appendChild(li);}
}
function acceptRead(r,c,want){need(current(c));exact(r,['job','capture']);const j=job(r.job,want),x=checkedCapture(r.capture);if(x)need(uint(x.sequence)<=uint(j.sequence)&&(j.state==='done'||uint(x.sequence)<uint(j.sequence))&&(j.capture_sequence==='0'||x.sequence===j.capture_sequence));if(j.state==='done'){need(x!==null);need(j.capture_sequence==='0'?x.sequence===j.sequence&&!x.stale&&same(x.first,x.current):x.sequence===j.capture_sequence&&(x.first.before_batch===j.before_batch||x.current.before_batch===j.before_batch));}attempt=j;if(x){capture=x;shown=j.before_batch===x.first.before_batch?x.first:x.current;}text('history-message',j.state==='failed'?`Read ${human(j.error)}. Previous verified history, if shown, keeps its own capture date.`:j.state==='cancelled'?'Read cancelled. Previously captured history is retained.':'');renderHistory();}
function readFailure(e){const s=String(e?.message??e??'');if(s==='The remote gym is busy. Wait for its current request to finish.')text('history-message','The original connection is busy. The dated capture remains unchanged.');else{retired=true;clearStatus();message('The original connection needs attention. No request was repeated.');}}
async function begin(captureSequence='0',before='0'){
 const c=context();need(id(c.view));const pending=attempt?.state==='unacknowledged'?attempt:{sequence:nextSequence.toString(),capture_sequence:captureSequence,before_batch:before,state:'unacknowledged',error:''};attempt=pending;text('history-message','Reading the original pool…');
 try{const r=await call('BeginDailyPayoutRead',c.view,pending.sequence,pending.capture_sequence,pending.before_batch);need(current(c));const j=job(envelope(r,'daily_payout_begin',c),pending);attempt=j;nextSequence=uint(j.sequence)+1n;readDeadline=Date.now()+32000;renderHistory();}catch(e){readFailure(e);}
}
async function read(){const c=context(),want=own(attempt);try{const r=await call('DailyPayoutRead',c.view,want.sequence);if(!current(c))return;acceptRead(envelope(r,'daily_payout_read',c),c,want);}catch(e){if(current(c))readFailure(e);}}
async function task(fn){if(busy||retired)return;busy=true;controls();try{await fn();}catch(e){retired=true;clearStatus();message('The original daily wallet result is unavailable. No action was repeated.');}finally{busy=false;controls();}}
function bind(key,fn){$(key).addEventListener('click',()=>task(fn));}
for(const [key,want]of[['login-a','zero256'],['login-b','incrementing256']])bind(key,async()=>{need(!view()&&fixture===want);const r=await call('LoginFixture',want);const s=envelope(r,'login_fixture',{view:''});need(s.state==='signed_in'&&s.remote?.state==='stopped');renderSnapshot(s);});
bind('mining',async()=>{const c=context();need(id(c.view));const method=snapshot.remote.state==='running'?'Stop':'Start';const r=await call(method,c.view);need(current(c));const s=envelope(r,method.toLowerCase(),c);need(s.state==='signed_in'&&same(s.view,snapshot.view));renderSnapshot(s);});
bind('logout',async()=>{const c=context();const r=await call('Logout',c.view);need(current(c));const s=envelope(r,'logout',c);need(s.state==='signed_out');renderSnapshot(s);});
bind('history-refresh',()=>begin());
bind('history-first',()=>begin(capture.sequence,capture.first.before_batch));
bind('history-next',()=>begin(capture.sequence,capture.current.next_before));
bind('history-cancel',async()=>{const c=context(),want=own(attempt);try{const r=await call('CancelDailyPayoutRead',c.view,want.sequence);need(current(c));acceptRead(envelope(r,'daily_payout_cancel',c),c,want);}catch(e){readFailure(e);}});
async function poll(){
 if(!busy&&!retired){busy=true;controls();try{if(attempt?.state==='working'&&Date.now()<=readDeadline)await read();else{const c=context();const r=await call('Snapshot');const s=envelope(r,'snapshot',c);if(c.view&&s.state==='signed_in')need(same(s.view,snapshot.view));if(!c.view)need(s.state!=='signed_in');renderSnapshot(s);}}catch(e){readFailure(e);}finally{busy=false;controls();}}
 if(!retired)setTimeout(poll,2000);
}
controls();poll();
