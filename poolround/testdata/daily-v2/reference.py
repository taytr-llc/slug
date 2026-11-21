"""Independent canonical round selection bytes; no Go or accepted-work oracle."""
import copy, hashlib, json, pathlib, struct, sys
root = pathlib.Path(__file__).resolve().parent
raw = (root / 'inputs/identities_v1.json').read_bytes()
assert hashlib.sha256(raw).hexdigest() == '361a08179c56d024836c45646de215024096d3a234fab93fe9566dbcd3c37a1c'
def canonical(v): return json.dumps(v, separators=(',', ':'), ensure_ascii=True)
old = json.loads(raw)
assert canonical(old['client']['source']) == old['source_canonical']
assert hashlib.sha256(b'SLUG pool service source\x00v1\x00' + old['source_canonical'].encode()).hexdigest() == old['source_id']
cases=[]
for name, ver, works, submissions, rounds, size in [('legacy',1,8,32,32,1<<20),('daily-721',2,721,4097,721,8<<30),('daily-max',2,8192,16384,16384,8<<30),('daily-lower',2,1,1,1,4096)]:
    source=copy.deepcopy(old['client']['source'])
    source.update(version=ver,max_works=works,max_submissions=submissions,max_bytes=size)
    limits=dict(version=ver,max_rounds=rounds,max_bytes=size)
    prefix=hashlib.sha256(b'SLUG pool accepted prefix\x00v1\x00'+canonical(source).encode()).hexdigest()
    marker=dict(application='slug-pool-round-journal',version=ver,limits=limits,source=source)
    meta=dict(marker,through=0,prefix=prefix,rounds=0,bytes=0)
    cases.append(dict(name=name,limits=limits,source=source,marker=canonical(marker),meta=canonical(meta),initial_prefix=prefix))
out=dict(scope='Canonical configuration/key bytes only, not proof or round authority',cases=cases,index_keys=[dict(sequence=n,hex=(b'i'+struct.pack('>Q',n)).hex()) for n in [1,255,256,257,720,721,4096,4097,16384]])
sys.stdout.write(json.dumps(out,indent=2)+'\n')
