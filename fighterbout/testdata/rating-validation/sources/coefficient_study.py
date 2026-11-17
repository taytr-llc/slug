#!/usr/bin/env python3
"""Released-only coefficient refits with fixed Decimal checks and no simulation."""
import collections,decimal,json,math,os,pathlib,signal,struct,subprocess,sys,time
import common as C
import execution as E
import resources
D=decimal.Decimal;OUT=C.ROOT/'coefficient-results';FIXED=(0,1,2,127,499,998,999)

def data():
 tr=(C.DATA/'training.dat').read_bytes();ix=(C.DATA/'indices.dat').read_bytes();assert len(tr)==96016 and tr[:8]==b'D75TRN01' and struct.unpack('<II',tr[8:16])==(4000,5);assert len(ix)==8000016 and ix[:8]==b'D75IDX01' and struct.unpack('<II',ix[8:16])==(1000,4000)
 rows=list(struct.iter_unpack('<5iI',tr[16:]));assert all(v[-1]<=2 for v in rows);return rows,ix

def decimal_fit(rep):
 E.context();start=time.monotonic();rows,ix=data();indices=range(4000) if rep==-1 else struct.unpack_from('<4000H',ix,16+rep*8000)
 xs=[[D(rows[i][j])/D(500) for j in range(5)] for i in indices];ys=[D(rows[i][-1])/D(2) for i in indices];w=[D(0)]*5
 for _ in range(400):
  grad=[D(0)]*5
  for x,y in zip(xs,ys):
   z=max(D(-30),min(D(30),E.dot(w,x)));p=D(1)/(D(1)+(-z).exp());err=p-y
   for j in range(5):grad[j]=grad[j]+err*x[j]
  w=[a-D('.5')*(g/D(4000)) for a,g in zip(w,grad)]
 resources.enforce(1<<16);result={'replica':rep,'weights':[str(v) for v in w],'normalized':None if E.normalized(w) is None else [str(v) for v in E.normalized(w)],'seconds':time.monotonic()-start};C.write_json(OUT/('decimal-'+str(rep)+'.json'),result);return result

def accelerate(args,name):
 cmd=[str(C.ROOT/'fit_accelerator'),str(C.DATA/'training.dat'),str(C.DATA/'indices.dat'),str(OUT/name)]+args;start=time.monotonic()
 resources.enforce(1<<20);p=subprocess.run(cmd,capture_output=True,text=True,timeout=1800);C.write_json(OUT/(name+'.command.json'),{'command':cmd,'returncode':p.returncode,'stdout':p.stdout,'stderr':p.stderr,'seconds':time.monotonic()-start});assert p.returncode==0,'accelerator failed; partial output preserved'
 resources.enforce();out=[json.loads(line) for line in (OUT/name).read_text().splitlines()]
 for r in out:assert r.get('iterations')==400 and len(r['weights_hex'])==5 and all(math.isfinite(float.fromhex(v)) for v in r['weights_hex'])
 return out

def decimal_checks(targets):
 pending=list(targets);running={};out={}
 try:
  while pending or running:
   while pending and len(running)<2:
    r=pending.pop(0);log=open(OUT/('decimal-'+str(r)+'.log'),'xb');cmd=[sys.executable,'-B','-S',str(C.ROOT/'decimal_refit_worker.py'),str(r)]
    p=subprocess.Popen(cmd,stdout=log,stderr=subprocess.STDOUT,start_new_session=True);running[r]=(p,log,time.monotonic(),cmd)
   for r,(p,log,started,cmd) in list(running.items()):
    if time.monotonic()-started>1800:raise TimeoutError('Decimal subprocess time cap')
    code=p.poll()
    if code is not None:
     log.close();del running[r];C.write_json(OUT/('decimal-'+str(r)+'.command.json'),{'command':cmd,'returncode':code,'seconds':time.monotonic()-started})
     assert code==0,'Decimal child failed; logs preserved'
     result=C.read_json(OUT/('decimal-'+str(r)+'.json'));assert result['replica']==r;out[r]=result;print('Decimal checked replica',r,flush=True)
   if running:time.sleep(.1)
 finally:
  for p,log,started,cmd in running.values():
   if p.poll() is None:
    try:os.killpg(p.pid,signal.SIGTERM)
    except ProcessLookupError:pass
   try:p.wait(timeout=2)
   except subprocess.TimeoutExpired:
    try:os.killpg(p.pid,signal.SIGKILL)
    except ProcessLookupError:pass
    p.wait(timeout=2)
   log.close()
 return out

def main():
 start=time.monotonic();resources.enforce();frozen=E.gate();freeze_hash=C.filehash(C.ROOT/'source-input-freeze.json');OUT.mkdir(exist_ok=False);before=C.check_pins()
 orig=accelerate(['--original'],'original.jsonl');allrows=accelerate(['--range','0','1000'],'bootstrap.jsonl');assert len(orig)==1 and orig[0]['replica']==-1 and [r['replica'] for r in allrows]==list(range(1000))
 raw={r['replica']:[D.from_float(float.fromhex(x)) for x in r['weights_hex']] for r in orig+allrows};fallback=[]
 for r,w in raw.items():
  if r==-1:continue
  n=E.normalized(w);mean=sum(w,D(0))/D(5)
  if abs(mean)<=D('.000001') or any(abs(v)<=D('.000001') for v in w) or n is None or any(abs(v)<=D('.000001') or abs((v-D('.05'))*D(10)-((v-D('.05'))*D(10)).to_integral_value())<=D('.00001') for v in n):fallback.append(r)
 assert len(set(fallback)-set(FIXED))<=64,'rounding/sign fallback cap; unresolved numerical resources'
 targets=sorted(set((-1,)+FIXED+tuple(fallback)));decimal_results={}
 decimal_results=decimal_checks(targets)
 assert decimal_results[-1]['weights']==C.read_json(C.OLD/'calibration/fit.json')['weights'],'original Decimal reproduction mismatch'
 checks=[]
 for r in targets:
  exact=[D(v) for v in decimal_results[r]['weights']];a=raw[r];n=E.normalized(a);en=E.normalized(exact);rawerr=max(abs(x-y) for x,y in zip(a,exact));normerr=None if n is None or en is None else max(abs(x-y) for x,y in zip(n,en))
  assert rawerr<=D('1e-8') and ((n is None and en is None) or (normerr is not None and normerr<=D('1e-8'))),'numeric validation error'
  if r==-1 or r in FIXED:assert n is None or E.rounded(n)==E.rounded(en),'fixed reference rounded tuple mismatch'
  checks.append({'replica':r,'raw_max_abs_error':str(rawerr),'normalized_max_abs_error':None if normerr is None else str(normerr),'rounded_equal':n is None or E.rounded(n)==E.rounded(en),'used_decimal_in_statistics':r in fallback})
  if r in fallback:raw[r]=exact
 results=[];freq=collections.Counter();flags=collections.Counter();changed=[0]*5;original_round=C.read_json(C.OLD/'calibration/fit.json')['rounded']
 for r in range(1000):
  w=raw[r];n=E.normalized(w);valid=n is not None and all(v>0 for v in n);rounded=None if n is None else E.rounded(n);flag='valid' if valid else ('undefined-normalization' if n is None else 'nonpositive-normalized-coefficient');flags[flag]+=1;freq[','.join(rounded) if rounded else 'undefined']+=1
  if rounded:
   for j in range(5):changed[j]+=rounded[j]!=original_round[j]
  results.append({'replica':r,'raw':[str(v) for v in w],'raw_mean':str(sum(w,D(0))/D(5)),'normalized':None if n is None else [str(v) for v in n],'rounded':rounded,'validity':flag,'numerical_source':'Decimal50' if r in fallback else 'validated-binary64'})
 C.write_json(OUT/'replicates.json',results);assert C.check_pins()==before;assert E.gate()==frozen and C.filehash(C.ROOT/'source-input-freeze.json')==freeze_hash
 incumbent=[D('1.0'),D('2.5'),D('.2'),D('.4'),D('.8')]
 report={'status':'completed','scope':'training-pair coefficient uncertainty; no weight adoption','protocol_sha256':C.PROTOCOL,'source_input_freeze_sha256':freeze_hash,'replicas':1000,'fit_iterations':400,'checks':checks,'fixed_decimal_replicas':[-1,*FIXED],'decimal_fallback_replicas':fallback,'fit_validity':dict(flags),'raw_statistics':E.matrix_summary([raw[r] for r in range(1000)]),'normalized_statistics':E.matrix_summary([E.normalized(raw[r]) for r in range(1000)]),'incumbent_mean_one':[str(v/D('.98')) for v in incumbent],'original_continuous_normalized':C.read_json(C.OLD/'calibration/fit.json')['normalized'],'rounded_tuple_frequency':[{'tuple':k.split(',') if k!='undefined' else None,'count':v} for k,v in sorted(freq.items(),key=lambda x:(-x[1],x[0]))],'per_coefficient_rounding_changes_from_original_fit':changed,'seconds':time.monotonic()-start,'source_changes':[],'files':{p.name:C.filehash(p) for p in sorted(OUT.iterdir()) if p.is_file()}}
 resources.enforce(len(C.jsonbytes(report))+(1<<16));C.write_json(OUT/'report.json',report);resources.enforce();print('Coefficient study completed',report['fit_validity'],flush=True)
if __name__=='__main__':
 def terminate(signum,frame):raise SystemExit('terminated; owned children will be joined')
 signal.signal(signal.SIGTERM,terminate)
 main()
