"""Finite journal/init models, not filesystem execution or runtime outcomes."""
from common import *
import crypto_result as R
import schema_core as S
from reference_results import examples

def dbkey(prefix,txid,index=None):return (prefix.encode()+raw(txid)+(b''if index is None else u32(index))).hex()
def audit(p,rows):
 assert type(rows)is list and len(rows)<=641
 db={}
 for k,v in rows:
  assert k not in db;db[k]=v
 assert'meta'.encode().hex()in db
 m=S.parse('Metadata',db.pop('meta'.encode().hex()).encode(),p);records={};inventory={};reserved={}
 for key,value in db.items():
  b=raw(key);assert b[:1]in [b'r',b'c',b'u'];assert len(b)==(33 if b[:1]==b'r'else37)
  if b[:1]==b'r':
   r=S.parse('Record',value.encode(),p);assert b[1:].hex()==r['result_key'];assert max(r['prepared_at_ms'],r['observation']['observed_at_ms'])<=m['clock_ms'];records[r['result_key']]=r
  elif b[:1]==b'c':
   c=S.parse('FundingCoin',value.encode(),p);assert b[1:33].hex()==c['outpoint']['txid']and int.from_bytes(b[33:],'little')==c['outpoint']['index'];inventory[key]=c
  else:reserved[key]=S.parse('Reservation',value.encode(),p)
 assert len(records)==m['result_count']<=256 and len(inventory)<=128 and len(reserved)<=256
 expected={}
 for key,r in records.items():
  pub=r['publication']
  if pub:
   op=pub['coin']['outpoint'];uk=dbkey('u',op['txid'],op['index']);assert uk not in expected;expected[uk]=dict(version=1,result_key=key,txid=pub['txid'])
 assert reserved==expected
 index=[dict(result_key=k,saturday=r['statement']['statement']['source']['source']['event']['saturday'],bout_id=r['statement']['statement']['source']['source']['bout']['bout_id'],record_digest=R.record_digest(p,r))for k,r in sorted(records.items())]
 assert len(J(index))<=96*1024
 return dict(result_count=len(records),inventory_count=len(inventory),reservation_count=len(reserved),index=index)

def store_fragment():
 x=examples();p=x['pin'];r=x['records']['finalized60'];m=dict(version=1,store_id=R.store_id(p),pin=p,source_created_at_ms=x['info']['source_created_at_ms'],created_at_ms=x['info']['created_at_ms'],clock_ms=x['now']+10000,result_count=1,funding_cursor=R.scan_initial(p,funding=True),funding_restarts=[],funding_complete=False,last_funding_tip=None)
 op=r['publication']['coin']['outpoint'];rk=dbkey('r',r['result_key']);uk=dbkey('u',op['txid'],op['index']);rows=[['meta'.encode().hex(),J(m).decode()],[rk,J(r).decode()],[uk,J(dict(version=1,result_key=r['result_key'],txid=r['publication']['txid'])).decode()]];cases=[]
 def test(name,v,accept):
  try:o=audit(p,v);actual=True
  except(Exception):o=None;actual=False
  assert actual==accept,(name,actual,accept);cases.append(dict(name=name,rows=v,accept=accept,expected=o))
 test('one-publication-reservation',rows,True)
 for name,mut in [('missing-reservation',rows[:2]),('duplicate-physical-key',rows+[rows[1]]),('unexpected-init-key',rows+[['init'.encode().hex(),'{}']]),('unknown-prefix',rows+[['78'+'00'*32,'{}']]),('unindexed-reservation',rows+[[dbkey('u',label('other coin'),0),rows[2][1]]])]:test(name,mut,False)
 bad=clone(rows);bad[0][1]=J(dict(m,result_count=0)).decode();test('count-mismatch',bad,False)
 bad=clone(rows);bad[0][1]=J(dict(m,clock_ms=x['now']-1)).decode();test('clock-before-row',bad,False)
 bad=clone(rows);bad[1][0]=dbkey('r',label('wrong result'));test('wrong-result-key',bad,False)
 bad=clone(rows);bad[1][1]=' '+bad[1][1]
 # Open enforces canonical storage in addition to the public key-reordering parser.
 def canonical_audit(v):
  for key,value in v:
   kind='Metadata'if key=='meta'.encode().hex()else'Record'if raw(key)[:1]==b'r'else'Reservation'if raw(key)[:1]==b'u'else'FundingCoin'
   assert J(S.parse(kind,value.encode(),p)).decode()==value
  return audit(p,v)
 try:canonical_audit(bad);raise AssertionError('noncanonical accepted')
 except AssertionError:pass
 cases.append(dict(name='noncanonical-storage',rows=bad,accept=False,expected=None,additional_rule='canonical byte equality required'))
 bad=clone(rows);v=json.loads(bad[2][1]);v['txid']=label('different publication');bad[2][1]=J(v).decode();test('reservation-wrong-publication',bad,False)
 # Index bound uses four metadata fields only; modeled IDs grant no card authority.
 index=[dict(result_key=f'{i+1:064x}',saturday='2026-11-28',bout_id=label('modeled index bout'+str(i)),record_digest=label('modeled index record'+str(i)))for i in range(256)]
 assert len(J(index))<=96*1024
 init=[]
 for name,existing,parent,accept,preserve in [('absent','absent','prepared',True,False),('real-empty','empty','prepared',True,False),('matching-marker','marker','prepared',True,False),('matching-init','init','prepared',True,False),('initialized-empty','empty-meta','prepared',True,True),('ready-existing','populated','ready',True,True),('ready-missing','absent','ready',False,False),('ready-partial','init','ready',False,False),('prepared-populated','populated','prepared',False,False),('wrong-pin','wrong-pin','prepared',False,False),('source-incarnation-changed','wrong-source-time','prepared',False,False),('symlink','symlink','prepared',False,False),('unknown-file','unknown','prepared',False,False)]:
  init.append(dict(name=name,existing=existing,parent_phase=parent,accept=accept,preserve_created_at=preserve,scope='abstract frozen lifecycle expectation, no filesystem crash execution'))
 return dict(version=1,kind='store',scope='Canonical raw row/reservation graph and initialization models, no locks/fsync/session authority execution',cases=cases,initialization=init,index_capacity=dict(entries=256,canonical_bytes=len(J(index)),cap=96*1024,fields=S.FIELDS['IndexEntry'],rows=index))
