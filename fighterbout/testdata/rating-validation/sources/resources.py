"""Bounded owned-directory artifact accounting; preserves failures, never deletes."""
import pathlib
ROOT=pathlib.Path(__file__).resolve().parent
LIMIT=1<<30

def enforce(extra_bytes=0):
 assert type(extra_bytes)==int and extra_bytes>=0
 total=sum(p.stat().st_size for p in ROOT.rglob('*') if p.is_file())
 if total+extra_bytes>LIMIT:raise RuntimeError('D75 retained artifact budget exceeded; partial artifacts preserved')
 return total
