#!/usr/bin/env python3
"""Parser-only inherited-width diagnostics; never evidence or work positives."""
import argparse
import copy
import json
from pathlib import Path
import sys

sys.dont_write_bytecode=True
from evidence_schema import canonical, parse_archive, Refusal, need
from reference_evidence import Reference
from generate_reference import actual_inputs, verify_lock, pretty, sha

HERE=Path(__file__).resolve().parent
FIELDS=('talent_budget','attributes.power','attributes.speed','attributes.chin',
        'attributes.stamina','attributes.skill','prime_weeks','decline_weeks')
VALUES=(1<<32,(1<<63)-1,1<<63)


def plan():
    return dict(version=1,scope='Parser-only signed64 inherited int boundary; no value is valid DNA or work evidence',
                source='v1/zero256-unawarded',fields=list(FIELDS),
                values_decimal=[str(n) for n in VALUES],
                expected_parse_accept=[True,True,False],expected_semantic_accept=False,
                normal_work_calls=0)


def run(reference):
    expected,base=actual_inputs()['v1/zero256-unawarded'];results=[]
    for field in FIELDS:
        for value in VALUES:
            archive=copy.deepcopy(base);target=archive['admission']['verified']['dna']
            parts=field.split('.')
            for part in parts[:-1]:target=target[part]
            target[parts[-1]]=value
            raw=canonical(archive)
            try:parse_archive(raw);parsed=True
            except Refusal:parsed=False
            need(parsed==(value<(1<<63)))
            try:reference.validate_shape(expected,archive);semantic=True
            except Refusal:semantic=False
            need(not semantic)
            results.append(dict(field=field,value_decimal=str(value),parse_accepted=parsed,
                                semantic_accepted=semantic,normal_work_calls=0))
    return dict(version=1,scope=plan()['scope'],cases=results,normal_work_calls=0)


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--repo-root',type=Path)
    parser.add_argument('--lock',type=Path,required=True)
    parser.add_argument('--output',type=Path,required=True)
    args=parser.parse_args()
    need(not args.output.exists())
    _,lock_hash=verify_lock(args.lock)
    need(json.loads((HERE/'parser-boundary-plan.json').read_bytes())==plan())
    result=run(Reference(args.repo_root));result['execution_lock_sha256']=lock_hash
    raw=pretty(result);args.output.parent.mkdir(parents=True,exist_ok=True);args.output.write_bytes(raw)
    print(json.dumps(dict(passed=True,cases=len(result['cases']),normal_work_calls=0,sha256=sha(raw))))


if __name__=='__main__':main()
