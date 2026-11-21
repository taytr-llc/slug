from pathlib import Path
import importlib.util, os, subprocess, sys, hashlib
sys.dont_write_bytecode=True
helper=Path("/Users/taytrrip/slug/set80-prep/slug-github.py")
spec=importlib.util.spec_from_file_location("slug_ci",helper)
m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
job=sys.argv[1]
assert job.isdigit()
out=Path("/Users/taytrrip/slug/artifacts/set80/ci-attempt1")/("job-"+job+".log")
env=dict(os.environ,GH_TOKEN=m.credential());env.pop("GH_DEBUG",None)
with out.open("xb") as f:
 r=subprocess.run(["gh","api","repos/taytr-llc/slug/actions/jobs/"+job+"/logs"],stdout=f,stderr=subprocess.PIPE,env=env,timeout=120)
print("exit",r.returncode,"bytes",out.stat().st_size,"sha256",hashlib.sha256(out.read_bytes()).hexdigest())
if r.returncode: print(r.stderr.decode("utf-8","replace")[:1000])
raise SystemExit(r.returncode)
