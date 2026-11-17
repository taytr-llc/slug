#!/usr/bin/env python3
"""Independent candidate-manifest oracle; Python standard library only.

No Go outputs, network, work validation, accounts, keys, or official issuance.
The contract is ../fighter-manifest-contract.md. --generate emits the complete
corpus; --check independently regenerates it and checks all negative inputs.
"""

import argparse
from collections import Counter
import copy
from decimal import Decimal, ROUND_HALF_EVEN
import hashlib
import importlib.util
import json
from pathlib import Path
import re
import sys

sys.dont_write_bytecode = True
HERE = Path(__file__).resolve().parent
DOMAIN = b"SLUG fighter manifest\x00v1\x00"
TABLE_DOMAIN = b"SLUG fighter manifest tables\x00v1\x00"
DIGEST_DOMAIN = b"SLUG fighter manifest digest\x00v1\x00"
TABLE_HASH = "72231fbe707cea9a65dd5b92f7b2719953795bceadbbf9e3b8e0b8f4aaef3f44"
GENERATOR = "slug-fighter-manifest-candidate-v1"
UINT64_RANGE = 1 << 64
MAX_DRAWS = 256
MAX_SIZE = 8192
MAX_DEPTH = 8
BASELINE_HASHES = {
    "reference_dna.py": "d0b2acb1d078cbe559d30d310297a934d5d133088bec3654b4fc0f2a75baf4b2",
    "dna_v1.json": "b88f16a29eaaf0cd0b190b7bbcf7a908edb39f9c4357567eabc3249cf40d6e71",
}
LABEL_BOUNDS = {
    "name.first": 32, "name.last": 32,
    "physical.weight": 329511, "physical.height_jitter": 11,
    "physical.reach": 7, "physical.stance": 4,
    "portrait.skin": 6, "portrait.hair_style": 8, "portrait.hair_color": 6,
    "portrait.facial_hair": 5, "portrait.head_shape": 4,
    "portrait.mouth": 4, "portrait.brow": 3, "portrait.background": 8,
    "portrait.scar_chance": 100, "portrait.scar_site": 2,
    "portrait.shoulder_jitter": 13,
}


def canonical(value):
    return json.dumps(value, ensure_ascii=True, separators=(",", ":"),
                      allow_nan=False).encode("ascii")


def load_inputs():
    for name, expected in BASELINE_HASHES.items():
        if hashlib.sha256((HERE / "baseline" / name).read_bytes()).hexdigest() != expected:
            raise ValueError("independent DNA baseline bytes changed")
    raw = (HERE / "tables_v1.json").read_bytes()
    if hashlib.sha256(TABLE_DOMAIN + raw).hexdigest() != TABLE_HASH:
        raise ValueError("candidate table bytes changed")
    tables = json.loads(raw)
    if canonical(tables) != raw:
        raise ValueError("noncanonical candidate tables")
    spec = importlib.util.spec_from_file_location(
        "independent_dna_baseline", HERE / "baseline" / "reference_dna.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return tables, module


TABLES, DNA = load_inputs()


def stream_message(seed, label, counter):
    if not isinstance(seed, bytes) or len(seed) != 32:
        raise ValueError("seed must be exactly 32 bytes")
    if label not in LABEL_BOUNDS:
        raise ValueError("unknown manifest stream")
    if type(counter) is not int or not 0 <= counter < UINT64_RANGE:
        raise OverflowError("stream counter exhausted")
    return DOMAIN + label.encode("ascii") + b"\x00" + seed + counter.to_bytes(8, "little")


class Stream:
    def __init__(self, seed, label):
        self.seed, self.label, self.counter = seed, label, 0

    def next(self):
        message = stream_message(self.seed, self.label, self.counter)
        self.counter += 1
        return int.from_bytes(hashlib.sha256(message).digest()[:8], "big")


def uniform_below(n, next_value):
    if type(n) is not int or not 1 <= n < UINT64_RANGE:
        raise ValueError("invalid uniform bound")
    threshold = UINT64_RANGE % n
    for _ in range(MAX_DRAWS):
        r = next_value()
        if type(r) is not int or not 0 <= r < UINT64_RANGE:
            raise ValueError("invalid stream value")
        if r >= threshold:
            return r % n
    raise ValueError("manifest field sampling exhausted")


def weight_units(weight):
    if type(weight) is not int or not 128 <= weight <= 268:
        raise ValueError("weight outside candidate range")
    return 91 * (weight - 127) if weight <= 178 else 51 * (269 - weight)


def weight_from_rank(rank):
    if type(rank) is not int or not 0 <= rank < 329511:
        raise ValueError("invalid weight rank")
    for weight in range(128, 269):
        units = weight_units(weight)
        if rank < units:
            return weight
        rank -= units
    raise AssertionError("weight rank not exhausted")


def round_tenths_even(value):
    if type(value) is not int or value < 0:
        raise ValueError("rounding input must be nonnegative integer tenths")
    whole, remainder = divmod(value, 10)
    return whole + int(remainder > 5 or (remainder == 5 and whole % 2 == 1))


def height_from(weight, jitter):
    weight_units(weight)  # Same strict range; no float or hidden clipping.
    if type(jitter) is not int or not -5 <= jitter <= 5:
        raise ValueError("invalid half-inch jitter")
    return max(62, min(80, round_tenths_even(640 + weight - 128 + 5 * jitter)))


def weight_class(weight):
    weight_units(weight)
    for limit, name in zip(TABLES["weight_classes"]["upper_bounds_lb"],
                           TABLES["weight_classes"]["names"]):
        if weight <= limit:
            return name
    return "heavy"


def derive_style(attrs, reach_offset):
    if set(attrs) != set(DNA.ATTRIBUTE_NAMES) or any(
            type(v) is not int or not 1 <= v <= 100 for v in attrs.values()):
        raise ValueError("invalid original attributes")
    if type(reach_offset) is not int or not -2 <= reach_offset <= 4:
        raise ValueError("invalid physical reach offset")
    if attrs["power"] >= 70 and attrs["chin"] >= 65 and attrs["speed"] < 60:
        return "slugger"
    if attrs["speed"] >= 70 and attrs["skill"] >= 65 and reach_offset >= 2:
        return "outfighter"
    if attrs["speed"] >= 65 and attrs["power"] >= 65:
        return "boxer_puncher"
    if (attrs["speed"] >= 65 and attrs["stamina"] >= 70
            and attrs["skill"] >= 55 and reach_offset <= 1):
        return "pressure"
    return "boxer"


def generate(seed_hex):
    seed = DNA.parse_seed(seed_hex)
    dna = DNA.derive_dna(seed.hex())
    def draw(label):
        return uniform_below(LABEL_BOUNDS[label], Stream(seed, label).next)
    first = TABLES["first_names"][draw("name.first")]
    last = TABLES["last_names"][draw("name.last")]
    weight = weight_from_rank(draw("physical.weight"))
    height = height_from(weight, draw("physical.height_jitter") - 5)
    reach_offset = draw("physical.reach") - 2
    scar = "none"
    if draw("portrait.scar_chance") < 35:
        scar = TABLES["portrait"]["scar_sites"][draw("portrait.scar_site")]
    return {
        "manifest_version": 1,
        "generator_id": GENERATOR,
        "table_hash": TABLE_HASH,
        "dna": dna,
        "name": {"first": first, "last": last, "display": first + " " + last},
        "physical": {
            "height_in": height, "reach_in": height + reach_offset,
            "stance": "southpaw" if draw("physical.stance") == 0 else "orthodox",
            "weight_lb": weight, "weight_class": weight_class(weight),
        },
        "style": derive_style(dna["attributes"], reach_offset),
        "portrait": {
            "portrait_version": 1, "skin": draw("portrait.skin"),
            "hair_style": draw("portrait.hair_style"),
            "hair_color": draw("portrait.hair_color"),
            "facial_hair": draw("portrait.facial_hair"),
            "head_shape": draw("portrait.head_shape"),
            "mouth": draw("portrait.mouth"), "brow": draw("portrait.brow"),
            "background": draw("portrait.background"), "scar": scar,
            "shoulder_jitter": draw("portrait.shoulder_jitter") - 6,
        },
    }


def preflight_json(raw):
    if not isinstance(raw, bytes) or not 1 <= len(raw) <= MAX_SIZE:
        raise ValueError("manifest byte limit")
    if not raw.isascii() or b"\\" in raw:
        raise ValueError("manifest must be literal ASCII without escapes")
    depth, quoted = 0, False
    for byte in raw:
        if byte == 34:
            quoted = not quoted
        elif not quoted:
            if byte == 91 or byte == 93:
                raise ValueError("arrays are not manifest fields")
            if byte == 123:
                depth += 1
                if depth > MAX_DEPTH:
                    raise ValueError("manifest nesting limit")
            elif byte == 125:
                depth -= 1
                if depth < 0:
                    raise ValueError("invalid JSON nesting")
    if quoted or depth:
        raise ValueError("incomplete JSON")


def parse_manifest(raw):
    preflight_json(raw)
    def pairs(items):
        out = {}
        for key, value in items:
            if key in out:
                raise ValueError("duplicate field")
            out[key] = value
        return out
    def integer(text):
        if len(text) > 20 or not re.fullmatch(r"-?(0|[1-9][0-9]*)", text):
            raise ValueError("invalid bounded integer")
        return int(text)
    def forbidden(_):
        raise ValueError("noninteger number")
    obj = json.loads(raw, object_pairs_hook=pairs, parse_int=integer,
                     parse_float=forbidden, parse_constant=forbidden)
    if not isinstance(obj, dict) or type(obj.get("manifest_version")) is not int:
        raise ValueError("manifest must be a typed object")
    if obj["manifest_version"] != 1 or obj.get("generator_id") != GENERATOR:
        raise ValueError("unknown manifest version or generator")
    if not isinstance(obj.get("dna"), dict) or not isinstance(obj["dna"].get("seed"), str):
        raise ValueError("missing DNA seed")
    regenerated = generate(obj["dna"]["seed"])
    if raw != canonical(regenerated):
        raise ValueError("noncanonical or non-derived manifest")
    return regenerated


def digest_manifest(raw):
    parse_manifest(raw)
    return hashlib.sha256(DIGEST_DOMAIN + raw).hexdigest()


def features(value):
    out = {"rarity:" + value["dna"]["rarity"],
           "class:" + value["physical"]["weight_class"],
           "style:" + value["style"], "stance:" + value["physical"]["stance"],
           "first:" + value["name"]["first"], "last:" + value["name"]["last"]}
    out.update("portrait:" + key + ":" + str(val) for key, val in value["portrait"].items()
               if key != "portrait_version")
    return out


def required_features():
    out = {"rarity:" + x[1] for x in DNA.TIERS}
    out.update("class:" + x for x in TABLES["weight_classes"]["names"])
    out.update("style:" + x for x in TABLES["style"]["priority"])
    out.update("stance:" + x for x in ("orthodox", "southpaw"))
    out.update("first:" + x for x in TABLES["first_names"])
    out.update("last:" + x for x in TABLES["last_names"])
    for key in ("skin", "hair_style", "hair_color", "facial_hair", "head_shape", "mouth", "brow", "background"):
        out.update("portrait:" + key + ":" + str(i) for i in range(LABEL_BOUNDS["portrait." + key]))
    out.update("portrait:scar:" + x for x in ("none", "left_brow", "right_cheek"))
    out.update("portrait:shoulder_jitter:" + str(i) for i in range(-6, 7))
    return out


def manifest_cases():
    fixed = [x["seed"] for x in DNA.golden_records()]
    fixed.extend(f"{n:064x}" for n in (0x7158, 0x1f235))
    records, covered, seen = [], set(), set()
    def add(name, value):
        raw = canonical(value)
        records.append({"name": name, "seed": value["dna"]["seed"],
                        "manifest": value, "canonical": raw.decode("ascii"),
                        "digest": hashlib.sha256(DIGEST_DOMAIN + raw).hexdigest()})
        covered.update(features(value)); seen.add(value["dna"]["seed"])
    for i, seed in enumerate(fixed):
        add("fixed_" + str(i), generate(seed))
    needed = required_features()
    for n in range(1, 100001):
        if needed <= covered:
            return records, n - 1, len(needed)
        seed = f"{n:064x}"
        if seed not in seen:
            value = generate(seed)
            if features(value) - covered:
                add("coverage_integer_" + str(n), value)
    raise AssertionError("bounded independent selection did not cover options")


def stream_vectors():
    seed = bytes(range(32))
    out = []
    for label in LABEL_BOUNDS:
        for counter in (0, 1, 256, UINT64_RANGE - 1):
            message = stream_message(seed, label, counter)
            digest = hashlib.sha256(message).digest()
            out.append({"seed": seed.hex(), "label": label, "counter_decimal": str(counter),
                        "message_hex": message.hex(), "sha256": digest.hex(),
                        "uint64_decimal": str(int.from_bytes(digest[:8], "big"))})
    return out


def style_vectors():
    defaults = {k: 50 for k in DNA.ATTRIBUTE_NAMES}
    definitions = [
        ("default", {}, 0, "boxer"),
        ("slugger_threshold", {"power":70,"chin":65,"speed":59}, 4, "slugger"),
        ("slugger_power_below", {"power":69,"chin":65,"speed":59}, 4, "boxer"),
        ("slugger_chin_below", {"power":70,"chin":64,"speed":59}, 4, "boxer"),
        ("slugger_speed_equal", {"power":70,"chin":65,"speed":60}, 4, "boxer"),
        ("outfighter_threshold", {"speed":70,"skill":65}, 2, "outfighter"),
        ("outfighter_short_reach", {"speed":70,"skill":65}, 1, "boxer"),
        ("outfighter_speed_below", {"speed":69,"skill":65}, 2, "boxer"),
        ("outfighter_skill_below", {"speed":70,"skill":64}, 2, "boxer"),
        ("outfighter_before_puncher", {"speed":70,"skill":65,"power":65}, 2, "outfighter"),
        ("puncher_threshold", {"speed":65,"power":65}, 4, "boxer_puncher"),
        ("puncher_speed_below", {"speed":64,"power":65}, 4, "boxer"),
        ("puncher_power_below", {"speed":65,"power":64}, 4, "boxer"),
        ("pressure_threshold", {"speed":65,"stamina":70,"skill":55}, 1, "pressure"),
        ("pressure_speed_below", {"speed":64,"stamina":70,"skill":55}, 1, "boxer"),
        ("pressure_stamina_below", {"speed":65,"stamina":69,"skill":55}, 1, "boxer"),
        ("pressure_skill_below", {"speed":65,"stamina":70,"skill":54}, 1, "boxer"),
        ("pressure_reach_above", {"speed":65,"stamina":70,"skill":55}, 2, "boxer"),
        ("puncher_before_pressure", {"speed":65,"power":65,"stamina":70,"skill":55}, 1, "boxer_puncher"),
        ("physical_style_changes_short", {"speed":70,"stamina":70,"skill":65}, 1, "pressure"),
        ("physical_style_changes_long", {"speed":70,"stamina":70,"skill":65}, 2, "outfighter"),
    ]
    out = []
    for name, updates, reach, expected in definitions:
        attrs = dict(defaults); attrs.update(updates)
        if derive_style(attrs, reach) != expected:
            raise AssertionError("hand-written style boundary expectation failed")
        out.append({"name":name,"attributes":attrs,"reach_offset_in":reach,"expected":expected})
    return out


def codec_negatives(base):
    raw = canonical(base)
    out = []
    def add(name, data):
        try:
            parse_manifest(data)
        except (ValueError, TypeError, OverflowError):
            out.append({"name":name,"input_hex":data.hex(),"accepted":False})
            return
        raise AssertionError("negative manifest parsed: " + name)
    for name, data in [
        ("empty",b""), ("leading_whitespace",b" "+raw), ("trailing_newline",raw+b"\n"),
        ("second_object",raw+b"{}"), ("trailing_nul",raw+b"\x00"),
        ("duplicate_top",raw.replace(b'"manifest_version":1',b'"manifest_version":1,"manifest_version":1',1)),
        ("duplicate_nested",raw.replace(b'"dna_version":1',b'"dna_version":1,"dna_version":1',1)),
        ("escaped_alias",raw.replace(b'"manifest_version"',b'"manifest_versi\\u006fn"',1)),
        ("case_alias",raw.replace(b'"manifest_version"',b'"MANIFEST_VERSION"',1)),
        ("bool_as_one",raw.replace(b'"manifest_version":1',b'"manifest_version":true',1)),
        ("float_as_one",raw.replace(b'"manifest_version":1',b'"manifest_version":1.0',1)),
        ("exponent_as_one",raw.replace(b'"manifest_version":1',b'"manifest_version":1e0',1)),
        ("leading_zero_integer",raw.replace(b'"manifest_version":1',b'"manifest_version":01',1)),
        ("integer_overflow",raw.replace(b'"manifest_version":1',b'"manifest_version":'+b'9'*1000,1)),
        ("nonfinite",raw.replace(b'"manifest_version":1',b'"manifest_version":NaN',1)),
        ("invalid_utf8",b'{"x":"\xff"}'), ("oversize",b' '*8193),
        ("deep_object",b'{"x":'*9+b'0'+b'}'*9), ("array",b'[]'),
        ("negative_zero",raw.replace(b'"manifest_version":1',b'"manifest_version":-0',1)),
    ]:
        add(name,data)
    for key in base:
        value=copy.deepcopy(base); del value[key]; add("missing_"+key,canonical(value))
        value=copy.deepcopy(base); value[key]=None; add("null_"+key,canonical(value))
    for path, value in [
        (("manifest_version",),2), (("generator_id",),"latest"), (("table_hash",),"00"*32),
        (("extra",),1), (("dna","seed"),"0"), (("dna","dna_version"),2),
        (("dna","attributes","power"),101), (("dna","prime_weeks"),0),
        (("name","first"),"Custom"), (("name","display"),"Nickname"),
        (("physical","height_in"),0), (("physical","weight_lb"),269),
        (("physical","weight_class"),"Heavy"), (("physical","stance"),"custom"),
        (("style",),"unknown"), (("portrait","portrait_version"),2),
        (("portrait","skin"),6), (("portrait","shoulder_jitter"),7),
        (("portrait","scar"),"custom"), (("portrait","svg"),"<svg/>"),
    ]:
        obj=copy.deepcopy(base); here=obj
        for key in path[:-1]: here=here[key]
        here[path[-1]]=value
        add("mutated_"+"_".join(path),canonical(obj))
    add("reordered_fields",canonical(dict(reversed(list(base.items())))))
    uppercase=generate("ff"*32); uppercase["dna"]["seed"]=uppercase["dna"]["seed"].upper()
    add("uppercase_manifest_seed",canonical(uppercase))
    for key in ("fighter_id", "prospect_id", "gym_id", "born_at_ms", "owner"):
        value=copy.deepcopy(base);value[key]="untrusted-claim"
        add("no_admission_field_"+key,canonical(value))
    for n in range(1,1001):
        zero=generate(f"{n:064x}")
        if zero["portrait"]["shoulder_jitter"]==0:
            add("semantically_equal_negative_zero",canonical(zero).replace(
                b'"shoulder_jitter":0',b'"shoulder_jitter":-0'))
            break
    else: raise AssertionError("no bounded zero-jitter example found")
    return out


def self_check():
    DNA.self_check()
    baseline = json.loads((HERE / "baseline" / "dna_v1.json").read_bytes())
    if DNA.golden_records() != baseline:
        raise AssertionError("independent DNA baseline does not reproduce")
    if sum(weight_units(w) for w in range(128,269)) != 91*51*52//2+51*90*91//2:
        raise AssertionError("weight sum disagrees with arithmetic-series derivation")
    if sum(weight_units(w) for w in range(128,269)) != 329511:
        raise AssertionError("weight total mismatch")
    if max(range(128,269), key=weight_units) != 178:
        raise AssertionError("candidate weight mode changed")
    for value, expected in [(625,62),(635,64),(645,64),(655,66),(795,80),(805,80)]:
        if round_tenths_even(value) != expected:
            raise AssertionError("integer tie-to-even rounding mismatch")
    for value in range(615,806):
        decimal_expected=int((Decimal(value)/Decimal(10)).quantize(Decimal(1),rounding=ROUND_HALF_EVEN))
        if round_tenths_even(value)!=decimal_expected:
            raise AssertionError("integer rounding differs from independent Decimal rounding")
    scripted=iter((0,5,6,19)); calls=[]
    def next_value():
        x=next(scripted);calls.append(x);return x
    if uniform_below(10,next_value)!=6 or calls!=[0,5,6]:
        raise AssertionError("rejection threshold differs")
    if uniform_below(10,next_value)!=9:
        raise AssertionError("draws did not advance")
    calls=[]
    def zero(): calls.append(0);return 0
    try: uniform_below(10,zero)
    except ValueError: pass
    else: raise AssertionError("sampling budget not enforced")
    if len(calls)!=256: raise AssertionError("wrong rejection budget")
    last=iter([0]*255+[6])
    if uniform_below(10,lambda:next(last))!=6:
        raise AssertionError("last permitted draw was not accepted")
    stream=Stream(bytes(32),"name.first");stream.counter=UINT64_RANGE-1
    stream.next()
    try: stream.next()
    except OverflowError: pass
    else: raise AssertionError("stream counter wrapped")
    if generate("ff"*32)!=generate("FF"*32):
        raise AssertionError("input hex case changed manifest")
    for record in baseline:
        if generate(record["seed"])["dna"]!=record:
            raise AssertionError("manifest changed canonical DNA")
    for bad in ("", "0"*63, "0"*65, "  "+"0"*64, "0x"+"0"*64, "é"*64):
        try: generate(bad)
        except ValueError: pass
        else: raise AssertionError("invalid seed accepted")


def corpus():
    self_check()
    cases,last,coverage=manifest_cases()
    for case in cases:
        parsed=parse_manifest(case["canonical"].encode("ascii"))
        if parsed!=case["manifest"] or digest_manifest(case["canonical"].encode())!=case["digest"]:
            raise AssertionError("canonical roundtrip failed")
    weight_vectors=[]; rank=0
    for w in range(128,269):
        units=weight_units(w)
        if weight_from_rank(rank)!=w or weight_from_rank(rank+units-1)!=w:
            raise AssertionError("weight interval boundary mismatch")
        weight_vectors.append({"weight_lb":w,"units":units,"first_rank":rank,"last_rank":rank+units-1})
        rank+=units
    counts={key:Counter() for key in ("rarity","weight_class","style","stance","scar")}
    for n in range(1,4097):
        value=generate(f"{n:064x}")
        for key,item in [("rarity",value["dna"]["rarity"]),("weight_class",value["physical"]["weight_class"]),
                         ("style",value["style"]),("stance",value["physical"]["stance"]),
                         ("scar",value["portrait"]["scar"])]: counts[key][item]+=1
    return {
        "preview_only":True,"corpus_version":1,"generator_id":GENERATOR,
        "table_hash":TABLE_HASH,"baseline_sha256":BASELINE_HASHES,
        "selection":{"fixed":"all original DNA vectors plus two observed short-ID collision seeds",
                     "coverage":"first positive big-endian integer seeds adding an uncovered option",
                     "last_integer_checked":last,"covered_options":coverage},
        "cases":cases,"stream_vectors":stream_vectors(),
        "sampling_vectors":[{"bound":10,"draws_decimal":["0","5","6"],"expected":6,"attempts":3},
                            {"bound":10,"draws_decimal":["19"],"expected":9,"attempts":1},
                            {"bound":10,"prefix_repeated_draw":"0","prefix_count":255,
                             "last_draw":"6","expected":6,"attempts":256},
                            {"bound":10,"repeated_draw":"0","attempts":256,"error":"sampling_exhausted"},
                            {"counter_decimal":str(UINT64_RANGE-1),"valid_draws":1,"next_error":"counter_exhausted"}],
        "weight_rank_intervals":weight_vectors,
        "height_vectors":[{"weight_lb":w,"jitter_steps":j,"height_in":height_from(w,j)}
                          for w in (128,145,146,170,171,178,195,196,225,226,268) for j in (-5,0,5)],
        "class_vectors":[{"weight_lb":w,"expected":weight_class(w)}
                         for w in (128,145,146,170,171,195,196,225,226,268)],
        "style_vectors":style_vectors(),"invalid_manifest_inputs":codec_negatives(cases[0]["manifest"]),
        "sample_mix":{"seed_encoding":"integers 1..4096, 32-byte big-endian",
                      "count":4096,"counts":{k:dict(sorted(v.items())) for k,v in counts.items()},
                      "interpretation":"deterministic candidate diagnostics, not fight or pricing calibration"},
    }


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    action=parser.add_mutually_exclusive_group()
    action.add_argument("--check",action="store_true")
    action.add_argument("--generate",action="store_true")
    action.add_argument("--seed")
    args=parser.parse_args()
    if args.seed is not None:
        self_check()
        print(canonical(generate(args.seed)).decode("ascii"))
        return
    expected=corpus()
    if args.generate:
        print(json.dumps(expected,ensure_ascii=True,indent=2,allow_nan=False))
        return
    saved=json.loads((HERE/"manifests_v1.json").read_bytes())
    if saved!=expected:
        raise SystemExit("manifest corpus differs from independent regeneration")
    print(json.dumps({"verified_manifest_cases":len(expected["cases"]),
                      "stream_vectors":len(expected["stream_vectors"]),
                      "invalid_inputs":len(expected["invalid_manifest_inputs"]),
                      "sample_mix":expected["sample_mix"]},indent=2))


if __name__=="__main__": main()
