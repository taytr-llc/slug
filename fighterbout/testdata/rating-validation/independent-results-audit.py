#!/usr/bin/env python3
"""Read retained D75 results only. Never import study code, refit, or simulate.
Independent checks use stdlib arithmetic, complete retained outputs/replicas,
and ten preselected resampling reconstructions per population/panel.
"""
import collections, decimal, gzip, hashlib, itertools, json, math, pathlib, time
D=decimal.Decimal
decimal.getcontext().prec=50
decimal.getcontext().rounding=decimal.ROUND_HALF_EVEN
ROOT=pathlib.Path('/Users/taytrrip/slug')
STUDY=ROOT/'set42-rating-validation'
OLD=ROOT/'set42-fight-reference'
ATTR=('power','speed','chin','stamina','skill')
CLASSES=('light','welter','middle','light_heavy','heavy')
POPS=('prime-xp0','mixed-age-xp0','prime-xpvar','mixed-age-xpvar')
PANELS=('natural','close','d52-edge')
METHODS=('decision','decision_draw','ko','tko_three_knockdowns','tko_referee','tko_corner','double_corner_stoppage')
SOURCE_METHODS=('Decision','Draw','KO','TKO (three knockdowns)','TKO (referee stoppage)','TKO (corner stoppage)')
FIXED=(0,1,2,127,499,998,999)
RESAMPLE_AUDIT=(0,1,2,17,127,249,499,999,4999,9999)
PROTO='b1e3a27c6dad5956b98dd340efd313998324d656a4d23d7d04b40d3fcc3424d6'
AMEND='38ae55d7b796afda0868fd1d2b6ee96ba02630f2c8c189c575b7386505b9e91c'
ORIGINAL_REPORT='0f9f3261dac4585e646961eb504e036cc98e4a7d66f5eba073d2a5303a0e2b9e'

def check(ok,where):
 if not ok:raise AssertionError(where)
def h(path):
 dig=hashlib.sha256()
 with open(path,'rb') as f:
  for part in iter(lambda:f.read(1<<20),b''):dig.update(part)
 return dig.hexdigest()
def read(path):return json.loads(path.read_bytes())
def canonical(v):return json.dumps(v,ensure_ascii=False,separators=(',',':'),allow_nan=False).encode()
def rows(path):
 with gzip.open(path,'rt',encoding='utf8') as f:
  for line in f:yield json.loads(line)
def lines(path):return [json.loads(v) for v in path.read_text().splitlines()]
def verify_files(base,mapping):
 for path,expected in mapping.items():check(h(base/path)==expected,'file hash '+str(base/path))
def norm(w):
 if any(not x.is_finite() for x in w):return None
 m=sum(w,D(0))/5
 return [x/m for x in w] if m>0 else None
def tenth(w):return None if w is None else [str(v.quantize(D('.1'))) for v in w]
def dot(w,x):
 total=D(0)
 for a,b in zip(w,x):total=total+a*D(b)
 return total

def audit_matrix(vectors,reported):
 if any(v is None or any(not x.is_finite() for x in v) for v in vectors):
  check(reported is None,'undefined aggregate');return '0'
 check(reported['n']==1000 and reported['percentile_indices']==[24,974],'matrix count/quantile')
 worst=D(0)
 with decimal.localcontext() as ctx:
  ctx.prec=80
  means=[sum((v[j] for v in vectors),D(0))/1000 for j in range(5)]
  cov=[[((sum((v[j]*v[k] for v in vectors),D(0))-1000*means[j]*means[k])/999) for k in range(5)] for j in range(5)]
  for j in range(5):
   if all(v[j]==vectors[0][j] for v in vectors):
    for k in range(5):cov[j][k]=cov[k][j]=D(0)
  def near(actual,expected,label):
   nonlocal worst
   e=abs(D(actual)-expected);worst=max(worst,e);check(e<=D('1e-38'),label)
  for j in range(5):
   ordered=sorted(v[j] for v in vectors)
   near(reported['means'][j],means[j],'coefficient mean')
   near(reported['median'][j],(ordered[499]+ordered[500])/2,'coefficient median')
   check([D(v) for v in reported['intervals'][j]]==[ordered[24],ordered[974]],'coefficient quantiles')
   for k in range(5):
    near(reported['covariance'][j][k],cov[j][k],'coefficient covariance')
    if cov[j][j]==0 or cov[k][k]==0:check(reported['correlation'][j][k] is None,'undefined correlation')
    else:near(reported['correlation'][j][k],cov[j][k]/(cov[j][j]*cov[k][k]).sqrt(),'coefficient correlation')
 return str(worst)

def coefficient_audit():
 base=STUDY/'coefficient-results';report=read(base/'report.json');verify_files(base,report['files'])
 check(report['status']=='completed' and report['replicas']==1000 and report['fit_iterations']==400,'coefficient completed/count')
 raw_rows=lines(base/'bootstrap.jsonl');original=lines(base/'original.jsonl')
 check([r['replica'] for r in raw_rows]==list(range(1000)),'raw coefficient replica order')
 check(len(original)==1 and original[0]['replica']==-1,'original coefficient row')
 original_fit=read(OLD/'calibration/fit.json');raw={}
 for r in original+raw_rows:
  check(r['iterations']==400 and len(r['weights_hex'])==5,'raw coefficient shape')
  floats=[float.fromhex(v) for v in r['weights_hex']];check(all(map(math.isfinite,floats)),'finite coefficient')
  raw[r['replica']]=[D.from_float(x) for x in floats]
 fallback=[]
 for r in range(1000):
  w=raw[r];n=norm(w)
  boundary=False if n is None else any(abs(v-((v*10-D('.5')).to_integral_value()+D('.5'))/10)<=D('1e-6') for v in n)
  if abs(sum(w,D(0))/5)<=D('1e-6') or any(abs(v)<=D('1e-6') for v in w) or n is None or any(abs(v)<=D('1e-6') for v in n) or boundary:fallback.append(r)
 check(fallback==report['decimal_fallback_replicas'],'fallback selection')
 check(len(set(fallback)-set(FIXED))<=64,'fallback cap')
 targets=sorted(set((-1,)+FIXED+tuple(fallback)))
 check(report['fixed_decimal_replicas']==[-1,*FIXED],'fixed Decimal references')
 checks={r['replica']:r for r in report['checks']};check(sorted(checks)==targets,'Decimal check set')
 maxraw=maxnorm=D(0)
 for r in targets:
  exact=read(base/('decimal-'+str(r)+'.json'));check(exact['replica']==r,'Decimal ID')
  if r==-1:check(exact['weights']==original_fit['weights'],'exact original Decimal reproduction')
  w=[D(x) for x in exact['weights']];a=raw[r];n,en=norm(a),norm(w)
  rawerr=max(abs(x-y) for x,y in zip(a,w));maxraw=max(maxraw,rawerr)
  normerr=None if n is None or en is None else max(abs(x-y) for x,y in zip(n,en))
  if normerr is not None:maxnorm=max(maxnorm,normerr)
  check(rawerr<=D('1e-8') and ((n is None and en is None) or (normerr is not None and normerr<=D('1e-8'))),'Decimal tolerance')
  same=tenth(n)==tenth(en)
  if r==-1 or r in FIXED:check(same,'fixed Decimal rounded tuple')
  rec=checks[r];check(D(rec['raw_max_abs_error'])==rawerr and (None if rec['normalized_max_abs_error'] is None else D(rec['normalized_max_abs_error']))==normerr,'reported Decimal error')
  check(rec['rounded_equal']==same and rec['used_decimal_in_statistics']==(r in fallback),'reported Decimal behavior')
  if r in fallback:raw[r]=w
 retained=read(base/'replicates.json');check([r['replica'] for r in retained]==list(range(1000)),'retained fit count/order')
 counts=collections.Counter();validity=collections.Counter();changed=[0]*5
 for r in retained:
  w=raw[r['replica']];n=norm(w);rounded=tenth(n)
  check([D(v) for v in r['raw']]==w and D(r['raw_mean'])==sum(w,D(0))/5,'retained raw/mean')
  check((None if r['normalized'] is None else [D(v) for v in r['normalized']])==n and r['rounded']==rounded,'retained normalization/rounding')
  flag='undefined-normalization' if n is None else ('valid' if all(v>0 for v in n) else 'nonpositive-normalized-coefficient')
  check(r['validity']==flag,'fit validity');validity[flag]+=1;counts[None if rounded is None else tuple(rounded)]+=1
  check(r['numerical_source']==('Decimal50' if r['replica'] in fallback else 'validated-binary64'),'numeric source')
  if rounded:
   for j in range(5):changed[j]+=rounded[j]!=original_fit['rounded'][j]
 check(dict(validity)==report['fit_validity'],'validity totals')
 reported={None if r['tuple'] is None else tuple(r['tuple']):r['count'] for r in report['rounded_tuple_frequency']}
 check(dict(counts)==reported and sum(reported.values())==1000,'joint rounded frequency')
 check(changed==report['per_coefficient_rounding_changes_from_original_fit'],'rounding crossings')
 check([D(v) for v in report['incumbent_mean_one']]==[D(v)/D('.98') for v in ('1.0','2.5','.2','.4','.8')],'incumbent scale')
 errors=[audit_matrix([raw[r] for r in range(1000)],report['raw_statistics']),audit_matrix([norm(raw[r]) for r in range(1000)],report['normalized_statistics'])]
 return {'replicates_checked':1000,'Decimal_checks':targets,'fallbacks':fallback,'raw_max_abs_error':str(maxraw),'normalized_max_abs_error':str(maxnorm),'aggregate_high_precision_max_errors':errors,'validity':dict(validity),'joint_rounded_frequencies':report['rounded_tuple_frequency']}

def condition(f,asof):
 dna=f['manifest']['dna'];elapsed=asof-f['born_at_ms'];age=elapsed//604800000;P=dna['prime_weeks'];N=dna['decline_weeks'];d=min(N,max(0,age-P));loss=(20,40,30,30,10);aged={}
 for name,L in zip(ATTR,loss):
  q,r=divmod(dna['attributes'][name]*(100*N-L*d),10*N)
  aged[name]=q+int(2*r>10*N or (2*r==10*N and q%2==1))
 league=dict(aged);league['skill']+=10*f['pro_xp']
 return {'version':1,'projector_id':'slug-fighter-condition-candidate-v1','dna_version':dna['dna_version'],'seed':dna['seed'],'born_at_ms':f['born_at_ms'],'as_of_ms':asof,'age_weeks':age,'phase':'prime' if age<P else ('decline' if age<P+N else 'retired'),'elapsed_decline_weeks':d,'prime_remaining_weeks':max(0,P-age),'prime_remaining_ms':max(0,P*604800000-elapsed),'career_remaining_ms':max(0,(P+N)*604800000-elapsed),'pro_xp':f['pro_xp'],'aged_attributes_tenths':aged,'league_attributes_tenths':league,'rating_hundredths':sum(v*league[k] for k,v in zip(ATTR,(10,25,2,4,8)))}
def score(sign,y):return 1 if y==0 or sign==0 else (2 if (sign>0)==(y>0) else 0)
def label(result,inp):
 w=result['winner'];check(w in ('',inp['a']['prospect_id'],inp['b']['prospect_id']),'winner identity')
 return 0 if w=='' else (1 if w==inp['a']['prospect_id'] else -1)
def metric_checks(group,reported):
 n=len(group);check(reported['n']==n and reported['half_point_denominator']==2*n,'group count')
 check(reported['half_points']=={k:sum(r['half_points'][k] for r in group) for k in ('incumbent','rounded','continuous')},'group scores')
 metrics={'finish_count':sum(r['method'] not in ('decision','decision_draw') for r in group),'draw_count':sum(r['outcome']==0 for r in group),'round_sum':sum(r['end_round'] for r in group),'source_finish_count':sum(r['source']['method'] not in ('Decision','Draw') for r in group),'source_draw_count':sum(r['source']['outcome']==0 for r in group),'source_round_sum':sum(r['source']['end_round'] for r in group),'canonical_a_wins':sum(r['outcome']==1 for r in group),'canonical_b_wins':sum(r['outcome']==-1 for r in group)}
 for k,v in metrics.items():check(reported[k]==v,'group metric '+k)
 methods=collections.Counter(dict.fromkeys(METHODS,0));methods.update(r['method'] for r in group)
 source=collections.Counter(dict.fromkeys(SOURCE_METHODS,0));source.update(r['source']['method'] for r in group)
 check(reported['methods']==dict(methods) and reported['source_methods']==dict(source),'finish tables')
 for pred in ('rounded','continuous'):
  deltas=[r['half_points'][pred]-r['half_points']['incumbent'] for r in group]
  expected={'improved':sum(v>0 for v in deltas),'worse':sum(v<0 for v in deltas),'unchanged':sum(v==0 for v in deltas),'half_point_delta_sum':sum(deltas)}
  check(reported['predictor_comparisons'][pred+'_minus_incumbent']==expected,'paired comparison counts')
 gap=collections.Counter(dict.fromkeys(('0','1..5','6..10','11..15'),0))
 age=collections.Counter();fractions=collections.Counter();agepair=collections.Counter(dict.fromkeys(map(str,range(16)),0));xppair=collections.Counter({str(a)+','+str(b):0 for a in range(16) for b in range(16)});agexp=collections.Counter({str(k)+','+str(a)+','+str(b):0 for k in range(16) for a in range(16) for b in range(16)})
 for r in group:
  x=r['original_xp'];v=abs(x[0]-x[1]);gap['0' if v==0 else ('1..5' if v<=5 else ('6..10' if v<=10 else '11..15'))]+=1
  agepair[str(r['age_pair_code'])]+=1;xppair[str(x[0])+','+str(x[1])]+=1;agexp[str(r['age_pair_code'])+','+str(x[0])+','+str(x[1])]+=1
  for a in r['actual_ages']:age[str(a['age_weeks'])]+=1;fractions[a['phase']+':'+str(a['elapsed_decline_weeks'])+'/'+str(a['decline_weeks'])]+=1
 check(reported['xp_gap_bucket_totals']==dict(gap),'XP gap coverage')
 for key,expected in [('actual_age_weeks',age),('actual_decline_fractions',fractions),('age_pair',agepair),('xp_pair',xppair),('age_xp',agexp)]:check(reported['histograms'][key]==dict(expected),'coverage '+key)
 for side,index in (('original_a',0),('original_b',1)):
  expected=collections.Counter(dict.fromkeys(map(str,range(16)),0));expected.update(str(r['original_xp'][index]) for r in group)
  check(reported['xp_level_totals'][side]==dict(expected),'XP level totals')
 for key,source in (('last_entered_round_counts',False),('source_last_entered_round_counts',True)):
  expected=collections.Counter(dict.fromkeys(('1','2','3'),0));expected.update(str((r['source'] if source else r)['end_round']) for r in group)
  check(reported[key]==dict(expected),'round histograms')

def preparation_checks(group,reported,preparation):
 units=sorted(set(r['unit'] for r in group));values=[preparation[u] for u in units]
 counts={k:sum(v[k] for v in values) for k in ('manifest_derivations','pair_rejections')}
 counts['families']=sum(v['count'] for v in values);counts['units']=units
 counts['class_rejections']=counts['manifest_derivations']-2*(counts['families']+counts['pair_rejections'])
 check(reported==counts and counts['class_rejections']>=0,'input preparation metrics')

def resample_cell(r):return (CLASSES.index(r['class']),0 if r['side'] in ('','inside') else 1,r['age_pair_code'] if r['population'].startswith('mixed-age') else 0,r['xp_gap_code'] if r['population'].endswith('xpvar') else 0)
def selected_resample(name,groups,rep):
 b=name.encode('ascii');key=hashlib.sha256(b'SLUG D75 population bootstrap\0v1\0'+len(b).to_bytes(4,'little')+b).digest();totals=[0]*6;words=0
 for si,group in enumerate(groups):
  counter=0;n=len(group);threshold=(1<<64)%n;prefix=key+rep.to_bytes(4,'little')+si.to_bytes(4,'little')
  for _ in range(n):
   for attempt in range(256):
    word=int.from_bytes(hashlib.sha256(prefix+counter.to_bytes(8,'little')).digest()[:8],'big');counter+=1;words+=1
    if word>=threshold:break
   else:raise AssertionError('resampling rejection exhaustion')
   r=group[word%n];hp=r['half_points'];vals=(hp['incumbent'],hp['rounded'],hp['continuous'],int(r['method'] not in ('decision','decision_draw'))-int(r['source']['method'] not in ('Decision','Draw')),int(r['outcome']==0)-int(r['source']['outcome']==0),r['end_round']-r['source']['end_round'])
   totals=[a+b for a,b in zip(totals,vals)]
 a,b,c,f,d,t=totals
 return dict(zip(('incumbent','rounded','continuous','rounded_minus_incumbent','continuous_minus_incumbent','finish_difference','draw_difference','round_difference'),(a,b,c,b-a,c-a,f,d,t))),words

def population_audit():
 base=STUDY/'population-results';report=read(base/'report.json');verify_files(base,report['files']);replicas=read(base/'bootstrap-replicas.json');fit=read(OLD/'calibration/fit.json');continuous=[D(x) for x in fit['weights']]
 preparation={r['unit']:r for r in read(STUDY/'data/population-input-freeze.json')['unit_summaries']}
 check(report['status']=='completed' and report['incumbent_remains_authoritative'] is True,'population completion')
 oldids=set();oldinputs=set()
 for r in rows(OLD/'calibration/inputs.jsonl.gz'):oldinputs.add(r['input_id']);oldids.update(r['input'][k]['prospect_id'] for k in ('a','b'))
 for r in read(OLD/'bout_v1.json')['cases']:oldinputs.add(r['input_id']);oldids.update(r['input'][k]['prospect_id'] for k in ('a','b'))
 for r in read(OLD/'preview_transport_v1.json')['cases']:
  inp=json.loads(r['canonical_input']);oldinputs.add(r['input_id']);oldids.update(inp[k]['prospect_id'] for k in ('a','b'))
 grouped=collections.defaultdict(list);seen=set();ids=set();count=0;sentinel=object()
 for inp_row,full,brief in itertools.zip_longest(rows(STUDY/'data/population-inputs.jsonl.gz'),rows(base/'full-results.jsonl.gz'),rows(base/'outcomes.jsonl.gz'),fillvalue=sentinel):
  check(all(v is not sentinel for v in (inp_row,full,brief)),'row counts align')
  inp=inp_row['input'];iid=hashlib.sha256(b'SLUG fighter bout input\0v1\0'+canonical(inp)).hexdigest();check(iid==inp_row['input_id']==full['input_id']==brief['input_id'],'input identity')
  check(iid not in seen and iid not in oldinputs,'disjoint input');seen.add(iid)
  check(inp['a']['prospect_id']<inp['b']['prospect_id'],'corner order')
  for k in ('a','b'):
   pid=inp[k]['prospect_id'];check(pid not in ids and pid not in oldids,'disjoint fighter identity');ids.add(pid)
  result=full['result'];check(result['input_id']==iid,'result input binding');check(hashlib.sha256(b'SLUG fighter bout result\0v1\0'+canonical(result)).hexdigest()==brief['result_digest'],'result digest')
  y=label(result,inp);check(brief['outcome']==y and brief['method']==result['method'] and brief['end_round']==result['end_round'] and brief['end_tick']==result['end_tick'],'new outcome projection')
  check(full['source']==brief['source'] and brief['source']['outcome']==label(brief['source'],inp),'source outcome projection')
  cs=[condition(inp[k],inp['as_of_ms']) for k in ('a','b')];check(cs==inp_row['canonical_conditions'],'independent integer condition')
  ratings=[c['rating_hundredths'] for c in cs];check(ratings==inp_row['incumbent_ratings']==brief['incumbent_ratings'],'incumbent current ratings')
  x=[cs[0]['league_attributes_tenths'][k]-cs[1]['league_attributes_tenths'][k] for k in ATTR];check(x==brief['league_differences'],'league features')
  margins=[sum(a*b for a,b in zip(w,x)) for w in ((10,25,2,4,8),(9,25,2,6,8))]+[dot(continuous,x)]
  scores={k:score(v,y) for k,v in zip(('incumbent','rounded','continuous'),margins)};check(scores==brief['half_points'],'paired draw-first scoring')
  check([D(z) for z in brief['predictor_gaps']]==[D(margins[0])/10,D(margins[1])/10,margins[2]],'fixed predictor gaps')
  expectedages=[{'age_weeks':c['age_weeks'],'phase':c['phase'],'elapsed_decline_weeks':c['elapsed_decline_weeks'],'decline_weeks':inp[k]['manifest']['dna']['decline_weeks']} for k,c in zip(('a','b'),cs)];check(brief['actual_ages']==expectedages,'actual age projection')
  lo,hi=min(ratings),max(ratings);check((brief['ratio_numerator'],brief['ratio_denominator'])==(hi,lo),'rating ratio')
  panel=brief['panel'];check(panel=='natural' or (panel=='close' and 20*hi<=21*lo) or (brief['side']=='inside' and 9*lo<=5*hi and hi<=2*lo) or (brief['side']=='outside' and 2*lo<hi and 5*hi<=11*lo),'outcome-independent population filter')
  for key in ('population','panel','class','side','row_index','age_pair_code','xp_gap_code','original_xp'):check(brief[key]==inp_row[key],'population metadata '+key)
  positions=inp_row['original_age_positions'];check(inp_row['age_pair_code']==4*positions[0]+positions[1],'age cell identity')
  xp=brief['original_xp'];gap=abs(xp[0]-xp[1]);check(brief['xp_gap_code']==(0 if gap==0 else (1 if gap<=5 else (2 if gap<=10 else 3))),'XP gap cell identity')
  grouped[brief['population']+'/'+panel].append(brief);count+=1
 check(count==20000 and len(ids)==40000 and len(grouped)==12,'population totals')
 outcomes=[];checked_words=0;allkeys=('incumbent','rounded','continuous','rounded_minus_incumbent','continuous_minus_incumbent','finish_difference','draw_difference','round_difference')
 for pop in POPS:
  for panel in PANELS:
   name=pop+'/'+panel;group=grouped[name];n=1000 if panel=='d52-edge' else 2000;check(len(group)==n,'panel allocation');rec=report['panels'][name];metric_checks(group,rec)
   preparation_checks(group,rec['input_preparation'],preparation)
   for cl in CLASSES:
    sub=[r for r in group if r['class']==cl];check(len(sub)==n//5,'class allocation');metric_checks(sub,rec['classes'][cl])
    preparation_checks(sub,rec['classes'][cl]['input_preparation'],preparation)
   if panel=='d52-edge':
    check(set(rec['edge_sides'])=={'inside','outside'},'edge side completeness')
    for side in ('inside','outside'):
     sub=[r for r in group if r['side']==side];check(len(sub)==500,'edge side allocation');metric_checks(sub,rec['edge_sides'][side]);preparation_checks(sub,rec['edge_sides'][side]['input_preparation'],preparation)
     for cl in CLASSES:
      part=[r for r in sub if r['class']==cl];check(len(part)==100,'edge side/class allocation');metric_checks(part,rec['edge_sides'][side]['classes'][cl]);preparation_checks(part,rec['edge_sides'][side]['classes'][cl]['input_preparation'],preparation)
   cells=collections.defaultdict(list)
   for r in group:cells[resample_cell(r)].append(r)
   ordered=[sorted(v,key=lambda r:r['row_index']) for k,v in sorted(cells.items())]
   check(rec['bootstrap']['stratum_sizes']==[{'cell':list(k),'size':len(v)} for k,v in sorted(cells.items())],'stratum order/count')
   for cell,rs in cells.items():
    _,side,age,gap=cell;unit=100 if panel=='d52-edge' else 400
    if pop.startswith('mixed-age') and pop.endswith('xpvar'):expected=6+int(gap==age%4) if unit==400 else 1+int(gap==age%4)+int(gap==(age+1)%4)+int(age<4 and gap==(age+2)%4)
    elif pop.startswith('mixed-age'):expected=unit//16+int(age<unit%16)
    elif pop.endswith('xpvar'):expected=unit//4
    else:expected=unit
    check(len(rs)==expected,'fixed design quota')
   vals=replicas[name];check(set(vals)==set(allkeys) and all(len(v)==10000 and all(type(z)==int for z in v) for v in vals.values()),'replica shape')
   check(rec['bootstrap']['replicas']==10000 and rec['bootstrap']['ordinary_percentile_indices']==[249,9749] and rec['bootstrap']['prediction_half_point_denominator']==2*n and rec['bootstrap']['outcome_difference_denominator']==n,'bootstrap units/indices')
   for i in range(10000):
    check(vals['rounded_minus_incumbent'][i]==vals['rounded'][i]-vals['incumbent'][i] and vals['continuous_minus_incumbent'][i]==vals['continuous'][i]-vals['incumbent'][i],'paired replica identity')
    check(all(0<=vals[k][i]<=2*n for k in ('incumbent','rounded','continuous')),'replica score bounds')
    check(abs(vals['finish_difference'][i])<=n and abs(vals['draw_difference'][i])<=n and abs(vals['round_difference'][i])<=2*n,'replica outcome bounds')
   expectedintervals={k:[sorted(v)[249],sorted(v)[9749]] for k,v in vals.items()};check(rec['bootstrap']['ordinary_intervals']==expectedintervals,'all percentile intervals')
   for r in RESAMPLE_AUDIT:
    expected,words=selected_resample(name,ordered,r);checked_words+=words;check(all(vals[k][r]==v for k,v in expected.items()),'independent selected resampling '+name+'/'+str(r))
   if panel!='d52-edge':
    adjusted=[sorted(vals['rounded_minus_incumbent'])[i] for i in (31,9968)];check(rec['bootstrap']['simultaneous_challenger_interval']==adjusted,'multiplicity intervals')
    positive=expectedintervals['rounded_minus_incumbent'][0]>0;check(rec['ordinary_positive_challenger_evidence']==positive and report['eight_panel_positive_evidence'][name]==positive,'eight-panel evidence')
   delta=rec['half_points']['rounded']-rec['half_points']['incumbent'];ci=expectedintervals['rounded_minus_incumbent']
   outcomes.append({'panel':name,'n':n,'rounded_improvement_pp':str(D(100)*delta/(2*n)),'paired95_pp':[str(D(100)*v/(2*n)) for v in ci],'improved_worse_unchanged':rec['predictor_comparisons']['rounded_minus_incumbent'],'class_rounded_improvements_pp':{cl:str(D(100)*(v['half_points']['rounded']-v['half_points']['incumbent'])/(2*v['n'])) for cl,v in rec['classes'].items()},'new_minus_source_finish_pp':str(D(100)*(rec['finish_count']-rec['source_finish_count'])/n),'new_minus_source_draw_pp':str(D(100)*(rec['draw_count']-rec['source_draw_count'])/n),'new_minus_source_mean_end_round':str(D(rec['round_sum']-rec['source_round_sum'])/n),'policy_relevant':panel!='d52-edge'})
 check(report['all_eight_positive']==all(report['eight_panel_positive_evidence'].values()),'final conjunctive evidence')
 return {'families_checked':count,'fighters_checked':len(ids),'all_results_input_ids_digests_and_labels_checked':True,'condition_and_scoring_recomputed_for_all':True,'all120000_retained_bootstrap_rows_intervals_checked':True,'independently_regenerated_replicas_per_panel':list(RESAMPLE_AUDIT),'independently_regenerated_words':checked_words,'all_eight_positive':report['all_eight_positive'],'panel_results':outcomes}

def main():
 start=time.monotonic();self_before=h(pathlib.Path(__file__));freeze_path=STUDY/'source-input-freeze.json';freeze_hash=h(freeze_path);freeze=read(freeze_path);release=read(STUDY/'execution-release.json')
 check(release['status']=='approved' and release['source_input_freeze_sha256']==freeze_hash,'root execution release')
 verify_files(STUDY,freeze['files']);check(h(STUDY/'PROTOCOL.md')==PROTO and h(STUDY/'RESOURCE_AMENDMENT.md')==AMEND,'method pins')
 pins=read(STUDY/'original-input-pins.json');verify_files(OLD,pins);verify_files(OLD,read(OLD/'corpus-freeze.json')['files']);check(h(OLD/'calibration/report.json')==ORIGINAL_REPORT,'historical failed report')
 coeffhash=h(STUDY/'coefficient-results/report.json');pophash=h(STUDY/'population-results/report.json')
 coefficients=coefficient_audit();population=population_audit()
 for path in (STUDY/'coefficient-results/report.json',STUDY/'population-results/report.json'):
  rec=read(path);check(rec['source_input_freeze_sha256']==freeze_hash and rec['protocol_sha256']==PROTO,'report method/sourcefreeze identity')
 verify_files(STUDY,freeze['files']);verify_files(OLD,pins);verify_files(OLD,read(OLD/'corpus-freeze.json')['files'])
 check(h(freeze_path)==freeze_hash and h(pathlib.Path(__file__))==self_before,'audit/freeze unchanged')
 check(coeffhash==h(STUDY/'coefficient-results/report.json') and pophash==h(STUDY/'population-results/report.json'),'reports unchanged')
 required=[p for p in population['panel_results'] if p['policy_relevant']]
 conclusions={'challenger_met_all_eight_rule':population['all_eight_positive'],'required_panels_without_positive_interval':[p['panel'] for p in required if D(p['paired95_pp'][0])<=0],'required_panels_with_interval_wholly_below_zero':[p['panel'] for p in required if D(p['paired95_pp'][1])<0],'coefficient_stability_does_not_establish_predictive_improvement':True,'D65_remains_authoritative':True}
 report={'status':'passed','findings':[],'coefficient_report_sha256':coeffhash,'population_report_sha256':pophash,'protocol_sha256':PROTO,'resource_amendment_sha256':AMEND,'source_input_freeze_sha256':freeze_hash,'audit_source_sha256':self_before,'coefficient_audit':coefficients,'population_audit':population,'substantive_conclusions':conclusions,'old_report_sha256':ORIGINAL_REPORT,'original_pins_verified':True,'frozen_source_and_inputs_verified_before_after':True,'new_fits_or_fights_executed':False,'scope_limitations':['No refit or simulator was rerun; Decimal checks independently reconciled from retained reference outputs.','All retained bootstrap intervals were recomputed; ten preselected replicas per panel were independently regenerated, not all200million random selections.','Statistical conclusion applies to the declared class/age/XP-gap-balanced populations, not an unknown future player distribution.'],'wall_seconds':time.monotonic()-start}
 out=ROOT/'set42-d75-results-independent-review.json'
 with open(out,'x') as f:json.dump(report,f,indent=2,allow_nan=False);f.write('\n')
 text='# D75 results independent review\n\nStatus: **passed; no audit-integrity findings**. No new fits or fights were run.\n\nThe audit reconciled all1,000 retained fits, reference Decimal checks, normalized aggregates and joint rounded-tuple frequencies. It independently recomputed conditions and paired predictor scores for all20,000 unique families, matched full-result digests/labels, verified fixed quotas and coverage, and recomputed all reported bootstrap intervals. Ten preselected replicas per panel were separately regenerated from the frozen stream. Original evidence and frozen source/input hashes match before and after.\n\n'
 text+='The rounded challenger '+('met' if population['all_eight_positive'] else 'did not meet')+' the declared all-eight positive-interval evidence rule. D65 remains authoritative; this review grants no rating adoption or deployment authority.\n\n'
 text+='| Population/panel | Improvement pp | Paired95% interval pp |\n| --- | ---: | --- |\n'
 for p in population['panel_results']:text+='| '+p['panel']+' | '+p['rounded_improvement_pp']+' | '+', '.join(p['paired95_pp'])+' |\n'
 text+='\nThe detailed JSON records exact reviewed reports, source pins, numerical discrepancies, counts and limitations.\n'
 with open(ROOT/'set42-d75-results-independent-review.md','x') as f:f.write(text)
 print(json.dumps({'status':'passed','report_sha256':h(out),'all_eight_positive':population['all_eight_positive'],'seconds':report['wall_seconds']},indent=2))
if __name__=='__main__':main()
