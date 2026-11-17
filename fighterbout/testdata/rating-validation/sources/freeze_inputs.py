#!/usr/bin/env python3
"""Seal completed preparation and reviewed sources. No fit or fight invocation."""
import ast,pathlib,platform,sys
import common as C
import resources

def main():
 assert platform.python_version()=='3.14.3'
 assert not any((C.ROOT/p).exists() for p in ('source-input-freeze.json','execution-release.json','coefficient-results','population-results'))
 pins=C.check_pins();resources.enforce()
 copies={
  'set42-rating-validation-source-review.json':'source-review.json',
  'set42-rating-validation-source-review.md':'source-review.md',
  'set42-rating-validation-method-review.json':'method-review.json',
  'set42-rating-validation-method-review.md':'method-review.md',
  'set42-rating-accelerator-root-review.json':'accelerator-root-review.json',
 }
 for src,dest in copies.items():
  raw=(C.ROOT.parent/src).read_bytes();resources.enforce(len(raw))
  with (C.ROOT/dest).open('xb') as f:f.write(raw)
 source_review=C.read_json(C.ROOT/'source-review.json');assert source_review['status']=='passed' and not source_review['findings']
 for p,h in source_review['source_sha256'].items():assert C.filehash(C.ROOT/p)==h,p
 assert C.read_json(C.ROOT/'protocol-approval.json')['protocol_sha256']==C.PROTOCOL
 build=C.read_json(C.ROOT/'accelerator-build.json');assert build['status']=='passed'
 assert C.filehash(C.ROOT/'fit_accelerator.cpp')==build['source_sha256'] and C.filehash(C.ROOT/'fit_accelerator')==build['binary_sha256']
 assert C.filehash(pathlib.Path(build['compiler']))==build['compiler_sha256']
 for p in C.ROOT.glob('*.py'):ast.parse(p.read_text(),filename=str(p))
 cf=C.read_json(C.DATA/'coefficient-input-freeze.json');pf=C.read_json(C.DATA/'population-input-freeze.json');audit=C.read_json(C.DATA/'input-audit.json')
 assert cf['fits_run']==0 and pf['fights_run']==0 and audit['status']=='passed' and audit['no_fits_or_fights']
 assert pf['families']==audit['families']==20000 and pf['unique_fighter_ids']==audit['unique_fighter_ids']==40000
 assert audit['exact_failed_attempt_prefix_rows']==14151 and audit['disjoint_original_inputs'] and audit['disjoint_original_fighter_ids']
 for rec in (cf,pf,audit):
  for p,h in rec['files'].items():assert C.filehash(C.DATA/p)==h,p
 assert pf['own_source_before']==pf['own_source_after']
 prior=C.read_json(C.ROOT/'preparation-attempts/attempt-002/before.json')
 assert prior['own_source_before']==pf['own_source_before']
 for p,h in prior['own_source_before'].items():assert C.filehash(C.ROOT/p)==h,p
 failure=C.read_json(C.ROOT/'preparation-attempts/attempt-001/failure.json')
 for p,h in failure['files'].items():assert C.filehash(C.ROOT/'preparation-attempts/attempt-001'/p)==h,p
 files={str(p.relative_to(C.ROOT)):C.filehash(p) for p in sorted(C.ROOT.rglob('*')) if p.is_file() and '__pycache__' not in p.parts}
 assert C.check_pins()==pins
 for p,h in files.items():assert C.filehash(C.ROOT/p)==h,p
 result={'status':'frozen_before_study_fits_and_outcomes','at':C.now(),'decision':'D75','protocol_sha256':C.PROTOCOL,'resource_amendment_sha256':C.RESOURCE_AMENDMENT,'original_input_pin_map_sha256':C.ORIGINAL_PINS,'source_review_sha256':C.filehash(C.ROOT/'source-review.json'),'input_audit_sha256':C.filehash(C.DATA/'input-audit.json'),'files':files,'external_original_pins':pins,'interpreter':{'executable':sys.executable,'version':platform.python_version(),'sha256':C.filehash(pathlib.Path(sys.executable))},'retained_bytes_before_freeze':resources.enforce(),'coefficient_replicas':1000,'population_families':20000,'new_study_fits':0,'new_study_fights':0,'source_changes':[],'next_gate':'Root execution-release.json must approve this exact file hash; no weight adoption authorized.'}
 C.write_json(C.ROOT/'source-input-freeze.json',result)
 print('Frozen source/input SHA256',C.filehash(C.ROOT/'source-input-freeze.json'),flush=True)

if __name__=='__main__':main()
