#!/usr/bin/env python3
"""Freeze study indices and fighter inputs only. Never invokes Simulate or fitting."""
import argparse,collections,os,pathlib,shutil,struct,time
import common as C
import resources

def coef_inputs():
 start=time.monotonic();before=C.check_pins();C.DATA.mkdir(exist_ok=True)
 train=[r for r in C.rows(C.OLD/'calibration/outcomes.jsonl.gz') if r['partition']=='train'];assert len(train)==4000
 assert [(r['class'],r['index']) for r in train]==[(cl,i) for cl in C.CLASSES for i in range(800)]
 with open(C.DATA/'training.dat','xb') as f:
  f.write(b'D75TRN01'+C.u32(4000)+C.u32(5))
  for r in train:
   xs=r['league_differences'];assert len(xs)==5 and all(type(n)==int and -2000<=n<=2000 for n in xs)
   label=r['outcome']+1;assert label in (0,1,2);f.write(struct.pack('<5iI',*xs,label))
  f.flush();os.fsync(f.fileno())
 key=bytes.fromhex(C.sha(b'SLUG D75 coefficient bootstrap\0v1\0'));budget=[0,10000000];counts=[]
 with open(C.DATA/'indices.dat','xb') as f:
  f.write(b'D75IDX01'+C.u32(1000)+C.u32(4000))
  for rep in range(1000):
   ctr=[]
   for cl in range(5):
    rng=C.Stream(key,C.u32(rep)+C.u32(cl),budget);ix=[cl*800+rng.uniform(800) for _ in range(800)];f.write(struct.pack('<800H',*ix));ctr.append(rng.counter)
   counts.append(ctr)
  f.flush();os.fsync(f.fileno())
 assert (C.DATA/'training.dat').stat().st_size==96016 and (C.DATA/'indices.dat').stat().st_size==8000016;assert before==C.check_pins()
 C.write_json(C.DATA/'coefficient-input-freeze.json',{'status':'frozen_before_fits','at':C.now(),'protocol_sha256':C.PROTOCOL,'source_sha256':{p:C.filehash(C.ROOT/p) for p in ('common.py','prepare_inputs.py')},'files':{p:C.filehash(C.DATA/p) for p in ('training.dat','indices.dat')},'training_input_ids':[r['input_id'] for r in train],'raw_words':budget[0],'raw_words_by_replica_class':counts,'seconds':time.monotonic()-start,'fits_run':0,'source_changes':[]})
 print('Coefficient input freeze completed',flush=True)

def cells(pop,n):
 age=pop.startswith('mixed-age');xp=pop.endswith('xpvar');out=[]
 for a in range(16 if age else 1):
  for g in range(4 if xp else 1):
   if age and xp:
    if n==400:count=6+int(g==a%4)
    else:assert n==100;count=1+int(g==a%4)+int(g==(a+1)%4)+int(a<4 and g==(a+2)%4)
   elif age:count=n//16+int(a<n%16)
   elif xp:count=n//4
   else:count=n
   out.extend([(a,g)]*count)
 assert len(out)==n
 return out

def gapcode(a,b):
 d=abs(a-b);return 0 if d==0 else (1 if d<=5 else (2 if d<=10 else 3))
XP_CHOICES={g:[(a,b) for a in range(16) for b in range(16) if gapcode(a,b)==g] for g in range(4)}
def ageweeks(m,pos):
 d=m['dna'];return 0 if pos==0 else d['prime_weeks']+({1:d['decline_weeks']//4,2:d['decline_weeks']//2,3:d['decline_weeks']-1}[pos])

def pop_inputs():
 start=time.monotonic();before=C.check_pins();own_before={p:C.filehash(C.ROOT/p) for p in ('common.py','prepare_inputs.py','resources.py')};resources.enforce();C.DATA.mkdir(exist_ok=True);assert shutil.disk_usage(C.ROOT).free>=1<<30
 budget=[0,1000000];manifest_count=0;families=0;seen_ids=set();seen_inputs=set();summaries=[]
 with C.gzwrite(C.DATA/'population-inputs.jsonl.gz') as f,C.gzwrite(C.DATA/'population-schedules.jsonl.gz') as schedules:
  for pop in C.POPS:
   for panel in C.PANELS:
    for cl in C.CLASSES:
     for side in (('inside','outside') if panel=='d52-edge' else ('',)):
      n=100 if side else 400;unit='/'.join([pop,panel,cl]+([side] if side else []));unitraw=C.frame(unit)
      rng=C.Stream(bytes.fromhex(C.sha(b'SLUG D75 coverage schedule\0v1\0'+unitraw+C.frame('design'))),budget=budget);design=cells(pop,n);rng.shuffle(design)
      xprng=C.Stream(bytes.fromhex(C.sha(b'SLUG D75 coverage schedule\0v1\0'+unitraw+C.frame('xp-pair'))),budget=budget);assigned=[XP_CHOICES[g][xprng.uniform(len(XP_CHOICES[g]))] if pop.endswith('xpvar') else (0,0) for a,g in design]
      schedules.write(C.jsonbytes({'unit':unit,'cells':design,'xp_pairs':assigned,'design_raw_words':rng.counter,'xp_raw_words':xprng.counter})+b'\n');unit_manifest_start=manifest_count;pair_rejections=0;age_xp=collections.Counter()
      for i,((ac,gc),xp) in enumerate(zip(design,assigned)):
       poses=[ac//4,ac%4] if pop.startswith('mixed-age') else [0,0]
       for pair_attempt in range(4096):
        ms=[];attempts=[]
        for corner in range(2):
         for ca in range(4096):
          manifest_count+=1
          if manifest_count>6000000:raise ValueError('global manifest derivation cap')
          if time.monotonic()-start>1800:raise TimeoutError('input construction time cap')
          seed=C.sha(b'SLUG D75 coverage manifest\0v1\0'+unitraw+C.u32(i)+C.u32(pair_attempt)+C.u32(corner)+C.u32(ca));m=C.B.MANIFEST.generate(seed)
          if m['physical']['weight_class']==cl:ms.append(m);attempts.append(ca);break
         else:raise ValueError('class rejection cap')
        ages=[ageweeks(m,p) for m,p in zip(ms,poses)];births=[C.ASOF-a*C.WEEK for a in ages];conds=[C.B.CONDITION.project(1,m['dna'],b,C.ASOF,x) for m,b,x in zip(ms,births,xp)];ratings=[c['rating_hundredths'] for c in conds];lo,hi=min(ratings),max(ratings);assert lo>0
        accept=panel=='natural' or (panel=='close' and 20*hi<=21*lo) or (side=='inside' and 9*lo<=5*hi and hi<=2*lo) or (side=='outside' and hi>2*lo and 5*hi<=11*lo)
        if accept:break
        pair_rejections+=1
       else:raise ValueError('pair rejection cap')
       fs=[{'prospect_id':C.sha(b'SLUG D75 coverage identity\0v1\0'+bytes.fromhex(m['dna']['seed'])+unitraw+C.u32(i)+C.u32(c)),'manifest':m,'born_at_ms':births[c],'pro_xp':xp[c],'training_skill':0} for c,m in enumerate(ms)]
       ids=[v['prospect_id'] for v in fs];assert len(set(ids))==2 and not any(v in seen_ids for v in ids);seen_ids.update(ids)
       seed=C.sha(b'SLUG D75 coverage bout\0v1\0'+unitraw+C.u32(i));inp=C.B.new_input(C.ASOF,seed,*fs);iid=C.B.input_id(inp);assert iid not in seen_inputs;seen_inputs.add(iid);order=[ids.index(inp[k]['prospect_id']) for k in ('a','b')]
       row={'population':pop,'panel':panel,'class':cl,'side':side,'unit':unit,'row_index':i,'age_pair_code':ac,'xp_gap_code':gc,'original_age_positions':poses,'original_age_weeks':ages,'original_xp':list(xp),'canonical_original_corner_order':order,'canonical_conditions':[conds[j] for j in order],'incumbent_ratings':[ratings[j] for j in order],'ratio_numerator':hi,'ratio_denominator':lo,'pair_attempt':pair_attempt,'class_attempts':attempts,'input_id':iid,'input':inp}
       f.write(C.jsonbytes(row)+b'\n');families+=1;age_xp[(ac,xp[0],xp[1])]+=1
      resources.enforce(1<<20);summaries.append({'unit':unit,'count':n,'manifest_derivations':manifest_count-unit_manifest_start,'pair_rejections':pair_rejections,'age_xp_cells':[{'age_pair':a,'xp_a':x,'xp_b':y,'count':count} for (a,x,y),count in sorted(age_xp.items())]});print('Prepared',unit,n,'families; total',families,'derivations',manifest_count,flush=True)
 assert families==20000 and len(seen_inputs)==20000 and len(seen_ids)==40000 and C.check_pins()==before
 own_after={p:C.filehash(C.ROOT/p) for p in own_before};assert own_after==own_before;resources.enforce()
 C.write_json(C.DATA/'population-input-freeze.json',{'status':'frozen_before_outcomes','at':C.now(),'protocol_sha256':C.PROTOCOL,'resource_amendment_sha256':C.RESOURCE_AMENDMENT,'own_source_before':own_before,'own_source_after':own_after,'source_sha256':{p:C.filehash(C.ROOT/p) for p in ('common.py','prepare_inputs.py')},'files':{p:C.filehash(C.DATA/p) for p in ('population-inputs.jsonl.gz','population-schedules.jsonl.gz')},'families':families,'unique_fighter_ids':len(seen_ids),'manifest_derivations':manifest_count,'schedule_raw_words':budget[0],'unit_summaries':summaries,'seconds':time.monotonic()-start,'fights_run':0,'source_changes':[]})
 print('Population input freeze completed',flush=True)

if __name__=='__main__':
 p=argparse.ArgumentParser();p.add_argument('phase',choices=['coefficient','population']);a=p.parse_args();{'coefficient':coef_inputs,'population':pop_inputs}[a.phase]()
