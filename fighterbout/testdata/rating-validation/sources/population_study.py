#!/usr/bin/env python3
"""Released-only fixed-engine coverage and paired prediction diagnostics."""
import argparse,collections,decimal,hashlib,json,pathlib,platform,sys,time
import common as C
import execution as E
import resources
D=decimal.Decimal;OUT=C.ROOT/'population-results';INC=[D('1'),D('2.5'),D('.2'),D('.4'),D('.8')];ROUND=[D('.9'),D('2.5'),D('.2'),D('.6'),D('.8')]
SOURCE_METHODS=('Decision','Draw','KO','TKO (three knockdowns)','TKO (referee stoppage)','TKO (corner stoppage)')
METHODS=('decision','decision_draw','ko','tko_three_knockdowns','tko_referee','tko_corner','double_corner_stoppage')

def source_fighter(src,f):
 m=f['manifest'];d=m['dna'];p=m['physical']
 return src.Fighter(seed=d['seed'],fid=f['prospect_id'],name=m['name']['display'],tier={'club':'Club','professional':'Professional','contender':'Contender','elite':'Elite','generational':'Generational'}[d['rarity']],attrs={k.title():d['attributes'][k] for k in C.B.ATTRS},height_in=p['height_in'],reach_in=p['reach_in'],stance={'orthodox':'Orthodox','southpaw':'Southpaw'}[p['stance']],weight_lb=p['weight_lb'],wclass=dict(zip(C.CLASSES,('Light','Welter','Middle','Light Heavy','Heavy')))[p['weight_class']],style={'slugger':'Slugger','outfighter':'Outfighter','boxer_puncher':'Boxer-Puncher','pressure':'Pressure','boxer':'Boxer'}[m['style']],prime_weeks=d['prime_weeks'],decline_weeks=d['decline_weeks'])
def result_label(result,inp):return 1 if result['winner']==inp['a']['prospect_id'] else (-1 if result['winner']==inp['b']['prospect_id'] else 0)
def simulate():
 resources.enforce();frozen=E.gate();freeze_hash=C.filehash(C.ROOT/'source-input-freeze.json');assert platform.python_version()=='3.14.3';before=C.check_pins();OUT.mkdir(exist_ok=False);start=time.monotonic();source=C.B.load('d75_original_simulator',C.OLD/'inputs/fight_sim.py');continuous=[D(x) for x in C.read_json(C.OLD/'calibration/fit.json')['weights']];count=0
 sys.modules['reference_bout']=C.B;accounting=C.B.load('d75_frozen_accounting',C.OLD/'generate_corpus.py')
 with C.gzwrite(OUT/'full-results.jsonl.gz') as full,C.gzwrite(OUT/'outcomes.jsonl.gz') as small:
  for row in C.rows(C.DATA/'population-inputs.jsonl.gz'):
   if time.monotonic()-start>1800:raise TimeoutError('simulation time cap')
   inp=row['input'];result=C.B.simulate(inp);accounting.check_accounting(inp,result);assert result['input_id']==row['input_id'] and result['method'] in METHODS and sum(result['draw_counts'].values())<=65536
   cs=[C.B.CONDITION.project(1,inp[k]['manifest']['dna'],inp[k]['born_at_ms'],inp['as_of_ms'],inp[k]['pro_xp']) for k in ('a','b')];assert cs==row['canonical_conditions'];fs=[source_fighter(source,inp[k]) for k in ('a','b')];base=[{k.title():c['aged_attributes_tenths'][k]/10 for k in C.B.ATTRS} for c in cs]
   old=source.simulate_bout(fs[0],fs[1],inp['seed'],title=False,featured=False,skill_bonus_a=inp['a']['pro_xp'],skill_bonus_b=inp['b']['pro_xp'],verbose=False,base_a=base[0],base_b=base[1]);ol=0 if old.winner is None else (1 if old.winner.fid==fs[0].fid else -1);assert old.winner is None or old.winner.fid in (fs[0].fid,fs[1].fid)
   oldout={'outcome':ol,'winner':old.winner.fid if old.winner else '', 'method':old.method,'end_round':old.end_round,'score_a':old.score_a,'score_b':old.score_b};label=result_label(result,inp);x=[cs[0]['league_attributes_tenths'][k]-cs[1]['league_attributes_tenths'][k] for k in C.B.ATTRS];preds=[E.dot(w,x) for w in (INC,ROUND,continuous)]
   brief={k:row[k] for k in ('population','panel','class','side','unit','row_index','age_pair_code','xp_gap_code','original_age_positions','original_age_weeks','original_xp','canonical_original_corner_order','incumbent_ratings','ratio_numerator','ratio_denominator','input_id')};brief.update({'outcome':label,'method':result['method'],'end_round':result['end_round'],'end_tick':result['end_tick'],'result_digest':C.B.digest(result),'source':oldout,'league_differences':x,'half_points':dict(zip(('incumbent','rounded','continuous'),(E.halfscore(p,label) for p in preds))),'predictor_gaps':[str(v) for v in preds],'canonical_phases':[c['phase'] for c in cs],'actual_ages':[{'age_weeks':c['age_weeks'],'phase':c['phase'],'elapsed_decline_weeks':c['elapsed_decline_weeks'],'decline_weeks':inp[k]['manifest']['dna']['decline_weeks']} for k,c in zip(('a','b'),cs)],'styles':[inp[k]['manifest']['style'] for k in ('a','b')],'stances':[inp[k]['manifest']['physical']['stance'] for k in ('a','b')],'rarities':[inp[k]['manifest']['dna']['rarity'] for k in ('a','b')],'talent_budgets':[inp[k]['manifest']['dna']['talent_budget'] for k in ('a','b')],'physical':[inp[k]['manifest']['physical'] for k in ('a','b')]})
   full.write(C.jsonbytes({'input_id':row['input_id'],'result':result,'source':oldout})+b'\n');small.write(C.jsonbytes(brief)+b'\n');count+=1
   if count%200==0:resources.enforce(1<<20);print('Simulated',count,'of 20,000 families',round(time.monotonic()-start,3),'seconds',flush=True)
 resources.enforce();assert count==20000 and C.check_pins()==before;assert E.gate()==frozen and C.filehash(C.ROOT/'source-input-freeze.json')==freeze_hash;C.write_json(OUT/'simulation.json',{'status':'completed','at':C.now(),'count':count,'seconds':time.monotonic()-start,'protocol_sha256':C.PROTOCOL,'source_input_freeze_sha256':freeze_hash,'files':{p:C.filehash(OUT/p) for p in ('full-results.jsonl.gz','outcomes.jsonl.gz')},'source_changes':[]})

def summarize_rows(rows):
 n=len(rows);methods=dict.fromkeys(METHODS,0);source_methods=collections.Counter(dict.fromkeys(SOURCE_METHODS,0));scores=collections.Counter();ratios=[]
 comparisons={name:{'improved':0,'worse':0,'unchanged':0,'half_point_delta_sum':0} for name in ('rounded_minus_incumbent','continuous_minus_incumbent')};xp_gap_buckets=dict.fromkeys(('0','1..5','6..10','11..15'),0)
 xp_levels={k:dict.fromkeys(map(str,range(16)),0) for k in ('original_a','original_b')};round_counts=dict.fromkeys(('1','2','3'),0);source_round_counts=dict.fromkeys(('1','2','3'),0)
 hist={'age_pair':collections.Counter(dict.fromkeys(map(str,range(16)),0)),'xp_pair':collections.Counter({str(a)+','+str(b):0 for a in range(16) for b in range(16)}),'xp_gap':collections.Counter(dict.fromkeys(map(str,range(16)),0)),'age_xp':collections.Counter({str(r)+','+str(a)+','+str(b):0 for r in range(16) for a in range(16) for b in range(16)}),'canonical_phases':collections.Counter({a+','+b:0 for a in ('prime','decline','retired') for b in ('prime','decline','retired')}),'rarities':collections.Counter(dict.fromkeys(('club','professional','contender','elite','generational'),0)),'talent_budgets':collections.Counter(),'actual_age_weeks':collections.Counter(),'actual_decline_fractions':collections.Counter(),'styles':collections.Counter(dict.fromkeys(('slugger','outfighter','boxer_puncher','pressure','boxer'),0)),'stances':collections.Counter(dict.fromkeys(('orthodox','southpaw'),0))}
 fin=draw=rounds=oldfin=olddraw=oldrounds=aw=bw=0;gaps=[[],[],[]];physical={k:[10**9,-1] for k in ('height_in','reach_in','weight_lb')}
 for r in rows:
  scores.update(r['half_points']);methods[r['method']]+=1;source_methods[r['source']['method']]+=1;fin+=r['method'] not in ('decision','decision_draw');draw+=r['outcome']==0;rounds+=r['end_round'];oldfin+=r['source']['method'] not in ('Decision','Draw');olddraw+=r['source']['outcome']==0;oldrounds+=r['source']['end_round'];aw+=r['outcome']==1;bw+=r['outcome']==-1
  for predictor,name in (('rounded','rounded_minus_incumbent'),('continuous','continuous_minus_incumbent')):
   delta=r['half_points'][predictor]-r['half_points']['incumbent'];assert delta in (-2,-1,0,1,2);comparisons[name]['improved' if delta>0 else ('worse' if delta<0 else 'unchanged')]+=1;comparisons[name]['half_point_delta_sum']+=delta
  gap=abs(r['original_xp'][0]-r['original_xp'][1]);xp_gap_buckets['0' if gap==0 else ('1..5' if gap<=5 else ('6..10' if gap<=10 else '11..15'))]+=1
  xp_levels['original_a'][str(r['original_xp'][0])]+=1;xp_levels['original_b'][str(r['original_xp'][1])]+=1;round_counts[str(r['end_round'])]+=1;source_round_counts[str(r['source']['end_round'])]+=1
  hist['age_pair'][str(r['age_pair_code'])]+=1;hist['xp_pair'][','.join(map(str,r['original_xp']))]+=1;hist['xp_gap'][str(abs(r['original_xp'][0]-r['original_xp'][1]))]+=1;hist['canonical_phases'][','.join(r['canonical_phases'])]+=1;hist['rarities'].update(r['rarities']);hist['talent_budgets'].update(map(str,r['talent_budgets']))
  hist['actual_age_weeks'].update(str(a['age_weeks']) for a in r['actual_ages']);hist['actual_decline_fractions'].update(a['phase']+':'+str(a['elapsed_decline_weeks'])+'/'+str(a['decline_weeks']) for a in r['actual_ages'])
  hist['styles'].update(r['styles']);hist['stances'].update(r['stances']);hist['age_xp'][str(r['age_pair_code'])+','+','.join(map(str,r['original_xp']))]+=1
  for j,v in enumerate(r['predictor_gaps']):gaps[j].append(abs(D(v))/D(10))
  for p in r['physical']:
   for k,v in physical.items():v[0]=min(v[0],p[k]);v[1]=max(v[1],p[k])
 return {'n':n,'half_point_denominator':2*n,'half_points':dict(scores),'predictor_comparisons':comparisons,'xp_gap_bucket_totals':xp_gap_buckets,'xp_level_totals':xp_levels,'last_entered_round_counts':round_counts,'source_last_entered_round_counts':source_round_counts,'methods':methods,'source_methods':dict(source_methods),'finish_count':fin,'draw_count':draw,'round_sum':rounds,'source_finish_count':oldfin,'source_draw_count':olddraw,'source_round_sum':oldrounds,'canonical_a_wins':aw,'canonical_b_wins':bw,'histograms':{k:dict(sorted(v.items())) for k,v in hist.items()},'absolute_predictor_gap_summaries':[{'predictor':('incumbent','rounded','continuous')[j],'units':'current weighted attribute points' if j<2 else 'raw logistic-fit weighted attribute units (arbitrary scale; not directly comparable to D65 gap magnitude)','mean':str(sum(v,D(0))/D(n)),'min':str(min(v)),'median':str((sorted(v)[(n-1)//2]+sorted(v)[n//2])/D(2)),'max':str(max(v))} for j,v in enumerate(gaps)],'physical_ranges':physical}

def preparation_counts(rows,preparation):
 units=sorted(set(r['unit'] for r in rows));entries=[preparation[u] for u in units]
 out={'families':sum(r['count'] for r in entries),'manifest_derivations':sum(r['manifest_derivations'] for r in entries),'pair_rejections':sum(r['pair_rejections'] for r in entries),'units':units};out['class_rejections']=out['manifest_derivations']-2*(out['families']+out['pair_rejections']);assert out['class_rejections']>=0;return out

def stratum(r):return (C.CLASSES.index(r['class']),('inside','outside').index(r['side']) if r['panel']=='d52-edge' else 0,r['age_pair_code'] if r['population'].startswith('mixed-age') else 0,r['xp_gap_code'] if r['population'].endswith('xpvar') else 0)
def bootstrap():
 resources.enforce();frozen=E.gate();freeze_hash=C.filehash(C.ROOT/'source-input-freeze.json');before=C.check_pins();sim=C.read_json(OUT/'simulation.json')
 for p,h in sim['files'].items():assert C.filehash(OUT/p)==h
 data=list(C.rows(OUT/'outcomes.jsonl.gz'));assert len(data)==20000;start=time.monotonic();budget=[0,250000000];reports={};allrep={};preparation={r['unit']:r for r in C.read_json(C.DATA/'population-input-freeze.json')['unit_summaries']}
 for pop in C.POPS:
  for panel in C.PANELS:
   group=[r for r in data if r['population']==pop and r['panel']==panel];n=len(group);assert n==(1000 if panel=='d52-edge' else 2000);cells=collections.defaultdict(list)
   for r in group:cells[stratum(r)].append(r)
   groups=[]
   for k,rs in sorted(cells.items()):
    assert [r['row_index'] for r in sorted(rs,key=lambda r:r['row_index'])]==sorted(r['row_index'] for r in rs)
    rs.sort(key=lambda r:r['row_index']);vals=[]
    for r in rs:
     h=r['half_points'];vals.append((h['incumbent'],h['rounded'],h['continuous'],int(r['method'] not in ('decision','decision_draw'))-int(r['source']['method'] not in ('Decision','Draw')),int(r['outcome']==0)-int(r['source']['outcome']==0),r['end_round']-r['source']['end_round']))
    groups.append((k,vals))
   name=pop+'/'+panel;key=bytes.fromhex(C.sha(b'SLUG D75 population bootstrap\0v1\0'+C.frame(name)));samples={k:[] for k in ('incumbent','rounded','continuous','rounded_minus_incumbent','continuous_minus_incumbent','finish_difference','draw_difference','round_difference')};prior_words=budget[0]
   for rep in range(10000):
    a=b=c=f=d=t=0
    for si,(keycell,vals) in enumerate(groups):
     rng=C.Stream(key,C.u32(rep)+C.u32(si),budget);size=len(vals)
     for _ in range(size):
      va,vb,vc,vf,vd,vt=vals[rng.uniform(size)];a+=va;b+=vb;c+=vc;f+=vf;d+=vd;t+=vt
    for k,v in zip(samples,(a,b,c,b-a,c-a,f,d,t)):samples[k].append(v)
    if time.monotonic()-start>1800:raise TimeoutError('population bootstrap time cap')
   intervals={k:[sorted(v)[249],sorted(v)[9749]] for k,v in samples.items()};simultaneous=[sorted(samples['rounded_minus_incumbent'])[31],sorted(samples['rounded_minus_incumbent'])[9968]] if panel!='d52-edge' else None
   summary=summarize_rows(group);summary.update({'classes':{cl:summarize_rows([r for r in group if r['class']==cl]) for cl in C.CLASSES},'bootstrap':{'replicas':10000,'raw_words':budget[0]-prior_words,'ordinary_percentile_indices':[249,9749],'ordinary_intervals':intervals,'prediction_half_point_denominator':2*n,'outcome_difference_denominator':n,'simultaneous_challenger_indices':[31,9968] if panel!='d52-edge' else None,'simultaneous_challenger_interval':simultaneous,'stratum_sizes':[{'cell':list(k),'size':len(v)} for k,v in groups],'singleton_strata':sum(len(v)==1 for k,v in groups),'interpretation':'descriptive unstable edge intervals; singleton cells do not establish zero uncertainty' if panel=='d52-edge' else 'approximate fixed-design paired bootstrap with coarse cell support'},'ordinary_positive_challenger_evidence':intervals['rounded_minus_incumbent'][0]>0 if panel!='d52-edge' else None})
   summary['input_preparation']=preparation_counts(group,preparation)
   for cl in C.CLASSES:summary['classes'][cl]['input_preparation']=preparation_counts([r for r in group if r['class']==cl],preparation)
   if panel=='d52-edge':
    summary['edge_sides']={}
    for side in ('inside','outside'):
     side_rows=[r for r in group if r['side']==side];side_summary=summarize_rows(side_rows);side_summary['input_preparation']=preparation_counts(side_rows,preparation);side_summary['interpretation']='descriptive side summary only; no side bootstrap or superiority gate';side_summary['classes']={}
     for cl in C.CLASSES:
      class_rows=[r for r in side_rows if r['class']==cl];s=summarize_rows(class_rows);s['input_preparation']=preparation_counts(class_rows,preparation);side_summary['classes'][cl]=s
     summary['edge_sides'][side]=side_summary
   resources.enforce(1<<20);reports[name]=summary;allrep[name]=samples;print('Bootstrapped',name,'10,000 replicas',round(time.monotonic()-start,3),'seconds',flush=True)
 resources.enforce(len(C.jsonbytes(allrep))+(1<<20));C.write_json(OUT/'bootstrap-replicas.json',allrep);assert C.check_pins()==before;assert E.gate()==frozen and C.filehash(C.ROOT/'source-input-freeze.json')==freeze_hash;required={k:v['ordinary_positive_challenger_evidence'] for k,v in reports.items() if not k.endswith('/d52-edge')};assert len(required)==8
 resources.enforce(len(C.jsonbytes(reports))+(1<<20));C.write_json(OUT/'report.json',{'status':'completed','scope':'fresh fixed-predictor coverage; not permission to change D65','protocol_sha256':C.PROTOCOL,'source_input_freeze_sha256':freeze_hash,'panels':reports,'eight_panel_positive_evidence':required,'all_eight_positive':all(required.values()),'incumbent_remains_authoritative':True,'raw_resampling_words':budget[0],'bootstrap_seconds':time.monotonic()-start,'source_changes':[],'original_source_baseline':'unchanged prototype; float/RNG/draw-margin differences retained','scope_limitations':['standard three-round fights only','synthetic class-balanced cohorts, not actual player prevalence','no fresh retired, training-varied, title or featured population','edge intervals are descriptive with small/singleton cells','approximate bootstrap coverage does not prove causal attribute effects'],'files':{p:C.filehash(OUT/p) for p in ('simulation.json','full-results.jsonl.gz','outcomes.jsonl.gz','bootstrap-replicas.json')}})
if __name__=='__main__':
 p=argparse.ArgumentParser();p.add_argument('phase',choices=['simulate','bootstrap']);a=p.parse_args();{'simulate':simulate,'bootstrap':bootstrap}[a.phase]()
