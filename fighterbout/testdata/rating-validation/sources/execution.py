"""Fail-closed study execution gate and numerical statistics helpers."""
import decimal,json,math,pathlib
import common as C
D=decimal.Decimal

def context():decimal.getcontext().prec=50;decimal.getcontext().rounding=decimal.ROUND_HALF_EVEN
context()
def gate():
 release=C.read_json(C.ROOT/'execution-release.json');f=C.ROOT/'source-input-freeze.json'
 assert release['status']=='approved' and release['source_input_freeze_sha256']==C.filehash(f),'execution not released'
 frozen=C.read_json(f)
 for p,h in frozen['files'].items():assert C.filehash(C.ROOT/p)==h,p
 C.check_pins();return frozen

def normalized(w):
 if not all(v.is_finite() for v in w):return None
 mean=sum(w,D(0))/D(5)
 return [v/mean for v in w] if mean>0 else None

def rounded(w):return [str(v.quantize(D('.1'))) for v in w]
def dot(w,x):
 n=D(0)
 for a,b in zip(w,x):n=n+a*D(b)
 return n

def halfscore(p,y):return 1 if y==0 or p==0 else (2 if (p>0)==(y>0) else 0)
def matrix_summary(vectors):
 if any(v is None or not all(x.is_finite() for x in v) for v in vectors):return None
 n=len(vectors);assert n==1000;means=[sum((v[j] for v in vectors),D(0))/D(n) for j in range(5)];cov=[]
 for j in range(5):
  row=[]
  for k in range(5):
   s=D(0)
   for v in vectors:s=s+(v[j]-means[j])*(v[k]-means[k])
   row.append(s/D(n-1))
  cov.append(row)
 corr=[[None if cov[j][j]==0 or cov[k][k]==0 else str(cov[j][k]/(cov[j][j]*cov[k][k]).sqrt()) for k in range(5)] for j in range(5)]
 return {'n':n,'means':[str(x) for x in means],'median':[str((sorted(v[j] for v in vectors)[499]+sorted(v[j] for v in vectors)[500])/D(2)) for j in range(5)],'percentile_indices':[24,974],'intervals':[[str(sorted(v[j] for v in vectors)[q]) for q in (24,974)] for j in range(5)],'covariance':[[str(v) for v in row] for row in cov],'correlation':corr}
