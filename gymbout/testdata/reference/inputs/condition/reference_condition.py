#!/usr/bin/env python3
"""Independent exact-integer condition oracle; no Go implementation or output.

Only the pinned independent DNA oracle selects original fighters. Expected
condition values use integer ratios; the old simulation is compared afterward
to document binary-float differences, never to supply golden expectations.
"""
import argparse
from concurrent.futures import ThreadPoolExecutor
import copy
from decimal import Decimal, localcontext, ROUND_HALF_EVEN
import hashlib
import importlib.util
import json
from pathlib import Path
import re
import sys
from types import SimpleNamespace

sys.dont_write_bytecode=True
HERE=Path(__file__).resolve().parent
VERSION=1
PROJECTOR="slug-fighter-condition-candidate-v1"
WEEK_MS=604800000
INT64_MIN=-(1<<63)
INT64_MAX=(1<<63)-1
CONTRACT_HASH="8ee75162fcd631cae15b33804c787690e60bf4997fae6b7cd536bb771908e654"
ATTRIBUTES=("power","speed","chin","stamina","skill")
LOSS={"power":20,"speed":40,"chin":30,"stamina":30,"skill":10}
RATING={"power":10,"speed":25,"chin":2,"stamina":4,"skill":8}
ZERO={"version":0,"projector_id":"","dna_version":0,"seed":"","born_at_ms":0,"as_of_ms":0,"age_weeks":0,"phase":"","elapsed_decline_weeks":0,"prime_remaining_weeks":0,"prime_remaining_ms":0,"career_remaining_ms":0,"pro_xp":0,"aged_attributes_tenths":dict.fromkeys(ATTRIBUTES,0),"league_attributes_tenths":dict.fromkeys(ATTRIBUTES,0),"rating_hundredths":0}


def canonical(obj):
    return json.dumps(obj,ensure_ascii=True,separators=(",",":"),allow_nan=False).encode("ascii")


def pretty(obj):
    return (json.dumps(obj,ensure_ascii=True,indent=2,allow_nan=False)+"\n").encode("ascii")


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def module(name,path):
    spec=importlib.util.spec_from_file_location(name,path)
    value=importlib.util.module_from_spec(spec)
    sys.modules[name]=value
    spec.loader.exec_module(value)
    return value


def inputs():
    provenance=json.loads((HERE/"input_hashes.json").read_bytes())
    for path,item in provenance.items():
        if sha((HERE/path).read_bytes())!=item["sha256"]:
            raise ValueError("pinned input changed: "+path)
    if provenance["inputs/fighter-condition-contract.md"]["sha256"]!=CONTRACT_HASH:
        raise ValueError("unreviewed contract")
    dna=module("condition_independent_dna",HERE/"inputs/baseline/reference_dna.py")
    source=module("condition_prototype_comparison",HERE/"inputs/fight_sim.py")
    records=json.loads((HERE/"inputs/baseline/dna_v1.json").read_bytes())
    assert records==dna.golden_records()
    return provenance,dna,source,records


PROVENANCE,DNA,SOURCE,ORIGINAL_DNA=inputs()


def round_even(numerator,denominator):
    if type(numerator) is not int or type(denominator) is not int or denominator<=0:
        raise ValueError("invalid integer ratio")
    sign=-1 if numerator<0 else 1
    whole,remainder=divmod(abs(numerator),denominator)
    return sign*(whole+int(2*remainder>denominator or (2*remainder==denominator and whole%2==1)))


def aged_tenths(original,loss,decline,step):
    if any(type(n) is not int for n in (original,loss,decline,step)) or not 1<=original<=100 or loss not in (10,20,30,40) or not 13<=decline<=39 or not 0<=step<=decline:
        raise ValueError("invalid aging grid input")
    numerator=original*(100*decline-loss*step)
    denominator=10*decline
    return round_even(numerator,denominator)


def validate_dna(dna):
    if not isinstance(dna,dict) or not isinstance(dna.get("seed"),str) or not re.fullmatch(r"[0-9a-f]{64}",dna["seed"]):
        raise ValueError("invalid canonical DNA")
    regenerated=DNA.derive_dna(dna["seed"])
    # JSON type spelling avoids Python's True==1 and 1.0==1 equivalence.
    if json.dumps(dna,sort_keys=True,allow_nan=False)!=json.dumps(regenerated,sort_keys=True,allow_nan=False):
        raise ValueError("DNA does not match independent regeneration")
    return regenerated


def project(version,dna,born_at_ms,as_of_ms,pro_xp):
    if type(version) is not int or version!=VERSION:
        raise ValueError("invalid projector version")
    original=validate_dna(dna)
    if type(born_at_ms) is not int or type(as_of_ms) is not int or not 1<=born_at_ms<=as_of_ms<=INT64_MAX:
        raise ValueError("invalid timestamp")
    if type(pro_xp) is not int or not 0<=pro_xp<=15:
        raise ValueError("invalid professional XP")
    elapsed=as_of_ms-born_at_ms
    age=elapsed//WEEK_MS
    P,D=original["prime_weeks"],original["decline_weeks"]
    step=max(0,min(D,age-P))
    phase="prime" if age<P else ("decline" if age<P+D else "retired")
    aged={name:aged_tenths(original["attributes"][name],LOSS[name],D,step) for name in ATTRIBUTES}
    league=aged.copy();league["skill"]+=10*pro_xp
    return {"version":VERSION,"projector_id":PROJECTOR,"dna_version":original["dna_version"],"seed":original["seed"],"born_at_ms":born_at_ms,"as_of_ms":as_of_ms,"age_weeks":age,"phase":phase,"elapsed_decline_weeks":step,"prime_remaining_weeks":max(0,P-age),"prime_remaining_ms":max(0,P*WEEK_MS-elapsed),"career_remaining_ms":max(0,(P+D)*WEEK_MS-elapsed),"pro_xp":pro_xp,"aged_attributes_tenths":aged,"league_attributes_tenths":league,"rating_hundredths":sum(RATING[k]*league[k] for k in ATTRIBUTES)}


def legacy_fighter(dna):
    return SimpleNamespace(prime_weeks=dna["prime_weeks"],decline_weeks=dna["decline_weeks"],career_weeks=dna["prime_weeks"]+dna["decline_weeks"],attrs={k.title():v for k,v in dna["attributes"].items()})


def grid():
    rows=[];ties=[];artifacts=[];comparisons=0
    losses=(10,20,30,40)
    selected={10:"Skill",20:"Power",30:"Chin",40:"Speed"}
    with localcontext() as ctx:
        ctx.prec=50
        for A in range(1,101):
            for D in range(13,40):
                outputs=[[] for _ in losses]
                fighter=SimpleNamespace(prime_weeks=26,decline_weeks=D,career_weeks=26+D,attrs={name.title():A for name in ATTRIBUTES})
                for step in range(D+1):
                    prototype=SOURCE.effective_attrs(fighter,26+step)
                    assert prototype["Chin"]==prototype["Stamina"]
                    for i,L in enumerate(losses):
                        numerator=A*(100*D-L*step);denominator=10*D
                        exact=aged_tenths(A,L,D,step)
                        independent=int((Decimal(numerator)/Decimal(denominator)).to_integral_value(rounding=ROUND_HALF_EVEN))
                        assert exact==independent
                        outputs[i].append(exact)
                        whole,rem=divmod(numerator,denominator)
                        tie=2*rem==denominator
                        if tie:
                            ties.append({"original":A,"decline_weeks":D,"elapsed_decline_weeks":step,"loss_percent":L,"numerator":numerator,"denominator":denominator,"floor":whole,"aged_tenths":exact})
                        old=prototype[selected[L]]
                        old_tenths=Decimal(str(old))*10
                        assert old_tenths==old_tenths.to_integral_value()
                        comparisons+=1
                        if int(old_tenths)!=exact:
                            assert tie,(A,D,step,L,exact,old)
                            artifacts.append({"original":A,"decline_weeks":D,"elapsed_decline_weeks":step,"loss_percent":L,"exact_tenths":exact,"prototype_tenths":int(old_tenths),"prototype_decimal":str(old),"reason":"binary-float representation changed an exact half-tie"})
                for i,L in enumerate(losses):
                    values=outputs[i]
                    assert values[0]==10*A and values[-1]==A*(100-L)//10
                    assert all(left>=right for left,right in zip(values,values[1:]))
                    assert all(0<value<=10*A for value in values)
                rows.append({"original":A,"decline_weeks":D,"aged_tenths_by_loss":outputs})
    assert comparisons==291600
    assert aged_tenths(1,40,13,13)==6
    return {"version":1,"loss_percentages":list(losses),"step_order":"index 0..decline_weeks inclusive","original_range":[1,100],"decline_range":[13,39],"cell_count":comparisons,"rows":rows},ties,artifacts


def dna_cases():
    records=[{"name":"canonical_"+str(i),"seed":d["seed"],"dna":d} for i,d in enumerate(ORIGINAL_DNA)]
    seen={x["seed"] for x in records}
    def coverage():
        return {"prime_min":any(x["dna"]["prime_weeks"]==26 for x in records),"prime_max":any(x["dna"]["prime_weeks"]==52 for x in records),"decline_min":any(x["dna"]["decline_weeks"]==13 for x in records),"decline_max":any(x["dna"]["decline_weeks"]==39 for x in records),"attribute_min":any(1 in x["dna"]["attributes"].values() for x in records),"attribute_max":any(100 in x["dna"]["attributes"].values() for x in records)}
    n=0
    while not all(coverage().values()):
        n+=1
        if n>10000: raise AssertionError("bounded fixture coverage search")
        seed=f"{n:064x}"
        if seed in seen:continue
        d=DNA.derive_dna(seed);old=coverage()
        records.append({"name":"coverage_integer_"+str(n),"seed":seed,"dna":d})
        if old==coverage():records.pop()
        else:seen.add(seed)
    return records,n,coverage()


def negatives(dna):
    result=[]
    base={"version":1,"dna":copy.deepcopy(dna),"born_at_ms":1700000000000,"as_of_ms":1700000000000,"pro_xp":0}
    def add(name,changes):
        value=copy.deepcopy(base);value.update(changes)
        try: project(**value)
        except (ValueError,TypeError,KeyError,OverflowError): pass
        else: raise AssertionError("invalid projection accepted: "+name)
        result.append({"name":name,**value,"expected_error":True,"expected_zero":copy.deepcopy(ZERO)})
    for v in (0,2,(1<<32)-1,-1):add("version_"+str(v),{"version":v})
    for b in (0,-1,INT64_MIN):add("birth_"+str(b),{"born_at_ms":b})
    for t in (INT64_MIN,-1,0,base["born_at_ms"]-1):add("asof_"+str(t),{"as_of_ms":t})
    add("maximum_future_birth",{"born_at_ms":INT64_MAX,"as_of_ms":INT64_MAX-1})
    for xp in (-1,16,-(1<<31),(1<<31)-1):add("xp_"+str(xp),{"pro_xp":xp})
    fields=[("dna_version",2),("rarity","clubx"),("talent_budget",dna["talent_budget"]+1),("prime_weeks",25),("decline_weeks",40)]
    for key,value in fields:
        mutant=copy.deepcopy(dna);mutant[key]=value;add("dna_"+key,{"dna":mutant})
    for attr in ATTRIBUTES:
        for bad in (-1,0,101,dna["attributes"][attr]+1):
            mutant=copy.deepcopy(dna);mutant["attributes"][attr]=bad;add("dna_"+attr+"_"+str(bad),{"dna":mutant})
    for seed in ("",dna["seed"][:-1]," "+dna["seed"],"0x"+dna["seed"],"g"*64,"FF"*32,"ff"*32):
        mutant=copy.deepcopy(dna);mutant["seed"]=seed;add("seed_"+str(len(result)),{"dna":mutant})
    return result


def prototype_comparison(dna,snapshot):
    fighter=legacy_fighter(dna);age=snapshot["age_weeks"]
    assert SOURCE.phase_at(fighter,age).lower()==snapshot["phase"]
    assert SOURCE.prime_remaining(fighter,age)==snapshot["prime_remaining_weeks"]
    old=SOURCE.effective_attrs(fighter,age)
    # Freeze the prototype's legacy binary64 evaluation order explicitly.
    # Python 3.12 changed built-in sum(float); host interpreter behavior must
    # not decide diagnostic artifacts, and never decides golden condition.
    effective=old.copy()
    effective["Skill"]+=min(15,snapshot["pro_xp"])
    old_rating=0.0
    for name in SOURCE.ATTRS:
        old_rating=old_rating+SOURCE.RATING_WEIGHTS[name]*effective[name]
    if sys.version_info < (3,12):
        lf=SimpleNamespace(f=fighter,xp=snapshot["pro_xp"],age=lambda week:week)
        assert SOURCE.league_rating(lf,age)==old_rating
    aged_equal=all(Decimal(str(old[k.title()]))*10==snapshot["aged_attributes_tenths"][k] for k in ATTRIBUTES)
    precise_from_old=sum(Decimal(str(old[k.title()]))*Decimal(RATING[k])/10 for k in ATTRIBUTES)+Decimal(snapshot["pro_xp"])*Decimal("0.8")
    exact=Decimal(snapshot["rating_hundredths"])/100
    if Decimal(str(old_rating))!=exact:
        return {"age_weeks":age,"pro_xp":snapshot["pro_xp"],"exact_rating_hundredths":snapshot["rating_hundredths"],"prototype_rating_decimal":repr(old_rating),"exact_rating_from_prototype_aged_values":str(precise_from_old),"reason":"rating binary arithmetic residue" if aged_equal else "aging half-tie difference, possibly plus rating binary residue"}
    return None


def corpus():
    records,search_end,edge_coverage=dna_cases();cases=[];rating_artifacts=[];unique_artifacts=set()
    born=1700000000123
    for index,item in enumerate(records):
        dna=item["dna"];P,D=dna["prime_weeks"],dna["decline_weeks"]
        elapsed={0,1,50000*WEEK_MS,(P+D+100)*WEEK_MS+123}
        for week in range(P+D+3):
            for offset in (-1,0,1):
                e=week*WEEK_MS+offset
                if e>=0:elapsed.add(e)
        inputs_=[("elapsed_"+str(e),born,born+e) for e in sorted(elapsed)]
        inputs_ += [("max_equal",INT64_MAX,INT64_MAX),("max_minus_one",INT64_MAX-1,INT64_MAX),("maximum_elapsed",1,INT64_MAX),("positive_min_equal",1,1)]
        for e in (WEEK_MS-1,WEEK_MS,WEEK_MS+1,P*WEEK_MS-1,P*WEEK_MS,P*WEEK_MS+1,(P+D)*WEEK_MS-1,(P+D)*WEEK_MS,(P+D)*WEEK_MS+1):
            inputs_.append(("near_max_elapsed_"+str(e),INT64_MAX-e,INT64_MAX))
        before=canonical(dna)
        for suffix,birth,asof in inputs_:
            for xp in (0,1,7,14,15):
                expected=project(1,dna,birth,asof,xp)
                assert canonical(dna)==before
                assert expected["rating_hundredths"]<=50200
                assert expected["age_weeks"]==(asof-birth)//WEEK_MS
                assert expected["prime_remaining_weeks"]==(expected["prime_remaining_ms"]+WEEK_MS-1)//WEEK_MS
                assert expected["league_attributes_tenths"]["skill"]-expected["aged_attributes_tenths"]["skill"]==10*xp
                for k in ATTRIBUTES[:-1]:assert expected["league_attributes_tenths"][k]==expected["aged_attributes_tenths"][k]
                cases.append({"name":item["name"]+"_"+suffix+"_xp"+str(xp),"dna_index":index,"version":1,"born_at_ms":birth,"as_of_ms":asof,"pro_xp":xp,"expected":expected})
                artifact=prototype_comparison(dna,expected)
                if artifact:
                    artifact={"dna_index":index,**artifact};key=canonical(artifact)
                    if key not in unique_artifacts:rating_artifacts.append(artifact);unique_artifacts.add(key)
        # XP only affects the D65 rating by exactly 80 hundredths per point.
        for age in (0,P,P+1,P+D,P+D+100):
            base=project(1,dna,born,born+age*WEEK_MS,0)
            for xp in range(16):
                value=project(1,dna,born,born+age*WEEK_MS,xp)
                assert value["aged_attributes_tenths"]==base["aged_attributes_tenths"]
                assert value["rating_hundredths"]-base["rating_hundredths"]==80*xp
    sample=cases[::max(1,len(cases)//128)][:128]
    def repeat(case):
        return project(case["version"],records[case["dna_index"]]["dna"],case["born_at_ms"],case["as_of_ms"],case["pro_xp"])
    with ThreadPoolExecutor(max_workers=8) as pool:
        assert list(pool.map(repeat,sample))==[c["expected"] for c in sample]
    # Prove the output contains no eligibility/ownership or fight-only inputs.
    assert set(cases[0]["expected"])==set(ZERO)
    invalid=negatives(records[0]["dna"])
    out={"preview_only":True,"version":VERSION,"projector_id":PROJECTOR,"week_ms":WEEK_MS,"approved_contract_sha256":CONTRACT_HASH,"provenance":PROVENANCE,"dna_cases":records,"cases":cases,"invalid_cases":invalid,"coverage":{"canonical_dna_cases":len(ORIGINAL_DNA),"supplementary_seed_search_last_integer":search_end,"edge_coverage":edge_coverage,"xp_values":[0,1,7,14,15],"all_xp_delta_checks":list(range(16)),"week_edges":"-1/0/+1ms at every whole week from birth through career_end+2","int64_extremes":True,"concurrent_projection_checks":len(sample)},"validation":{"valid_cases":len(cases),"invalid_cases":len(invalid),"originals_unchanged":True,"float_expected_values":False,"eligibility_or_ownership_claims":False}}
    return out,rating_artifacts


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    group=parser.add_mutually_exclusive_group(required=True)
    group.add_argument("--generate",action="store_true");group.add_argument("--check",action="store_true")
    args=parser.parse_args()
    condition,rating_artifacts=corpus();values,ties,aging_artifacts=grid()
    artifacts={"version":1,"normative":False,"description":"These are prototype binary-float differences, not golden condition values.","rating_float_model":"sequential-binary64-left-fold-v1","rating_float_model_detail":"Multiply and add in SOURCE.ATTRS order, starting at 0.0; do not use host built-in sum(float).","aging_grid_comparisons":values["cell_count"],"aging_mismatch_count":len(aging_artifacts),"aging_mismatches":aging_artifacts,"rating_mismatch_count":len(rating_artifacts),"rating_mismatches":rating_artifacts}
    documents={"condition_v1.json":pretty(condition),"aging_grid_v1.json":canonical(values)+b"\n","ties_v1.json":pretty({"version":1,"tie_count":len(ties),"ties":ties}),"prototype_artifacts_v1.json":pretty(artifacts)}
    for name,raw in documents.items():
        file=HERE/name
        if args.generate:file.write_bytes(raw)
        elif file.read_bytes()!=raw:
            old=file.read_bytes()
            at=next((i for i,(a,b) in enumerate(zip(raw,old)) if a!=b),min(len(raw),len(old)))
            print("DIAGNOSTIC",name,"first_byte",at,"lengths",len(raw),len(old),"equal_objects",json.loads(raw)==json.loads(old),file=sys.stderr)
            print("GENERATED",repr(raw[max(0,at-100):at+250]),file=sys.stderr)
            print("STORED",repr(old[max(0,at-100):at+250]),file=sys.stderr)
            raise SystemExit("FAIL: independent regeneration differs: "+name)
    print(json.dumps({"result":"pass","mode":"generate" if args.generate else "check","valid_cases":len(condition["cases"]),"invalid_cases":len(condition["invalid_cases"]),"dna_cases":len(condition["dna_cases"]),"grid_cells":values["cell_count"],"ties":len(ties),"prototype_aging_differences":len(aging_artifacts),"prototype_rating_differences":len(rating_artifacts),"hashes":{name:sha(raw) for name,raw in documents.items()}},sort_keys=True))


if __name__=="__main__":
    main()
