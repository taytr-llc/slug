#!/usr/bin/env python3
"""Independent frozen-contract v2 oracle; standard library, no Go or network."""
import argparse
from collections import Counter
import copy
import hashlib
import importlib.util
import json
import math
from pathlib import Path
import re
import sys

sys.dont_write_bytecode = True
HERE = Path(__file__).resolve().parent
BUNDLE = HERE.parent
NAME_DOMAIN = b"SLUG fighter manifest\x00v2\x00"
MANIFEST_DOMAIN = b"SLUG fighter manifest digest\x00v2\x00"
SVG_DOMAIN = b"SLUG fighter portrait SVG\x00v2\x00"
DESCRIPTOR_DOMAIN = b"SLUG fighter issuance descriptor\x00v1\x00"
GENERATOR = "slug-fighter-manifest-candidate-v2"
RENDERER = "slug-fighter-portrait-candidate-v2"
U64 = 1 << 64


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def canonical(value):
    return json.dumps(value, ensure_ascii=True, separators=(",", ":"), allow_nan=False).encode("ascii")


def pretty(value):
    return (json.dumps(value, ensure_ascii=True, indent=2, allow_nan=False) + "\n").encode("ascii")


def load_module(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    value = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(value)
    return value


def verify_inputs():
    locked = json.loads((HERE / "input-lock.json").read_bytes())
    for relative, expected in locked["files"].items():
        if sha((BUNDLE / relative).read_bytes()) != expected:
            raise ValueError("frozen input changed: " + relative)
    return locked


LOCK = verify_inputs()
APPROVAL = json.loads((BUNDLE / "approval.json").read_bytes())
PLAN = json.loads((HERE / "public-inputs.json").read_bytes())
TABLE_BYTES = (BUNDLE / "tables_v2.json").read_bytes()
TABLE = json.loads(TABLE_BYTES)
TABLE_HASH = sha(b"SLUG fighter manifest tables\x00v2\x00" + TABLE_BYTES)
assert canonical(TABLE) == TABLE_BYTES
assert all(len(TABLE[key]) == len(set(TABLE[key])) == 512 for key in TABLE)
assert TABLE_HASH == APPROVAL["descriptor"]["manifest_table_hash"]
PORTRAIT = load_module("archived_independent_portrait", HERE / "inputs/portrait-v1/reference_portraits.py")
V1 = PORTRAIT.MANIFEST


def message(seed, label, counter):
    if not isinstance(seed, bytes) or len(seed) != 32 or label not in ("name.first", "name.last"):
        raise ValueError("name stream input")
    if type(counter) is not int or not 0 <= counter < U64:
        raise OverflowError("counter_exhausted")
    return NAME_DOMAIN + label.encode("ascii") + b"\x00" + seed + counter.to_bytes(8, "little")


class Stream:
    def __init__(self, seed, label, counter=0):
        self.seed, self.label, self.counter = seed, label, counter

    def next(self):
        raw = hashlib.sha256(message(self.seed, self.label, self.counter)).digest()
        self.counter += 1
        return int.from_bytes(raw[:8], "big")


def uniform(n, draw):
    if type(n) is not int or not 0 < n < U64:
        raise ValueError("invalid_bound")
    threshold = U64 % n
    for attempt in range(256):
        value = draw()
        if type(value) is not int or not 0 <= value < U64:
            raise ValueError("invalid_draw")
        if value >= threshold:
            return value % n, attempt + 1
    raise ValueError("sampling_exhausted")


def indices(seed):
    return tuple(uniform(512, Stream(seed, label).next)[0] for label in ("name.first", "name.last"))


def generate(seed_hex):
    if not isinstance(seed_hex, str) or re.fullmatch(r"[0-9a-fA-F]{64}", seed_hex, re.ASCII) is None:
        raise ValueError("seed")
    seed = bytes.fromhex(seed_hex)
    base = V1.generate(seed.hex())
    first, last = indices(seed)
    result = copy.deepcopy(base)
    result["manifest_version"] = 2
    result["generator_id"] = GENERATOR
    result["table_hash"] = TABLE_HASH
    a, b = TABLE["first_names"][first], TABLE["last_names"][last]
    result["name"] = {"first": a, "last": b, "display": a + " " + b}
    for key in ("dna", "physical", "style", "portrait"):
        assert result[key] == base[key]
    return result


def parse(raw):
    V1.preflight_json(raw)

    def pairs(items):
        value = {}
        for key, item in items:
            if key in value:
                raise ValueError("duplicate")
            value[key] = item
        return value

    def integer(text):
        if len(text) > 20 or re.fullmatch(r"-?(0|[1-9][0-9]*)", text, re.ASCII) is None:
            raise ValueError("integer")
        return int(text)

    def invalid(_):
        raise ValueError("noninteger")

    value = json.loads(raw, object_pairs_hook=pairs, parse_int=integer, parse_float=invalid, parse_constant=invalid)
    if not isinstance(value, dict) or type(value.get("manifest_version")) is not int or value["manifest_version"] != 2:
        raise ValueError("version")
    if not isinstance(value.get("dna"), dict) or not isinstance(value["dna"].get("seed"), str):
        raise ValueError("seed")
    expected = generate(value["dna"]["seed"])
    if raw != canonical(expected):
        raise ValueError("noncanonical_or_nonderived")
    return expected


def render(raw, version=2):
    if type(version) is not int or version != 2:
        raise ValueError("renderer")
    value = parse(raw)
    # The archived independently authored rational paint routine is unchanged.
    # Full v2 validation precedes it; only its explicit renderer attribute differs.
    svg = PORTRAIT._draw(value)
    old = b'data-renderer="slug-fighter-portrait-candidate-v1"'
    new = b'data-renderer="slug-fighter-portrait-candidate-v2"'
    assert svg.count(old) == 1
    svg = svg.replace(old, new, 1)
    assert len(svg) <= 32768 and svg.isascii() and not svg.endswith(b"\n")
    return svg


def body(svg):
    return svg.split(b"</desc>", 1)[1]


def rejected(fn):
    try:
        fn()
    except (ValueError, TypeError, KeyError, OverflowError, UnicodeError):
        return True
    raise AssertionError("negative case unexpectedly accepted")


def baseline_check():
    old_manifest = json.loads((HERE / "inputs/portrait-v1/inputs/manifest/manifests_v1.json").read_bytes())
    old_portrait = json.loads((HERE / "inputs/portrait-v1/portraits_v1.json").read_bytes())
    assert V1.corpus() == old_manifest
    assert PORTRAIT.corpus() == old_portrait
    return {"manifest_cases": len(old_manifest["cases"]), "portrait_cases": len(old_portrait["cases"]), "complete_archived_corpora_regenerated": True}


def selected_seeds():
    selected = {item["seed_hex"]: [item["name"]] for item in PLAN["fixed_seeds"]}
    firsts, lasts = set(), set()
    final = 0
    for number in range(1, PLAN["name_index_coverage"]["limit"] + 1):
        seed = number.to_bytes(32, "big")
        a, b = indices(seed)
        if a not in firsts or b not in lasts:
            selected.setdefault(seed.hex(), []).append("first_missing_name_index")
        firsts.add(a)
        lasts.add(b)
        final = number
        if len(firsts) == len(lasts) == 512:
            break
    assert len(firsts) == len(lasts) == 512
    return selected, {"last_integer_checked": final, "first_indices": sorted(firsts), "last_indices": sorted(lasts)}


def streams():
    values = []
    for seed_spec in PLAN["fixed_seeds"][:3]:
        for label in ("name.first", "name.last"):
            for text in PLAN["stream_vectors"]["counters_decimal"]:
                counter = int(text)
                preimage = message(bytes.fromhex(seed_spec["seed_hex"]), label, counter)
                digest = hashlib.sha256(preimage).digest()
                value = int.from_bytes(digest[:8], "big")
                values.append({"seed": seed_spec["seed_hex"], "label": label, "counter_decimal": text, "preimage_hex": preimage.hex(), "sha256": digest.hex(), "prefix_uint64_decimal": str(value), "index512": value % 512})
    maximum = Stream(bytes(32), "name.first", U64 - 1)
    maximum.next()
    assert rejected(maximum.next)
    # Draw order cannot move another label's independent counter.
    a, b = Stream(bytes(32), "name.first"), Stream(bytes(32), "name.last")
    a0, a1, b0 = a.next(), a.next(), b.next()
    x, y = Stream(bytes(32), "name.first"), Stream(bytes(32), "name.last")
    assert (x.next(), y.next(), x.next()) == (a0, b0, a1)
    return values, {"first_then_first_then_last": list(map(str, (a0, a1, b0))), "interleaved_first_then_last_then_first": list(map(str, (a0, b0, a1))), "max_counter_next": "counter_exhausted"}


def sampling():
    tests = [
        ("bound10_reject0and5_accept6", 10, [0, 5, 6], 6, 3),
        ("bound10_last_permitted_draw", 10, [0] * 255 + [6], 6, 256),
        ("bound512_zero", 512, [0], 0, 1),
        ("bound512_max", 512, [U64 - 1], 511, 1),
    ]
    rows = []
    for name, bound, draws, want, attempts in tests:
        actual = uniform(bound, iter(draws).__next__)
        assert actual == (want, attempts)
        rows.append({"name": name, "bound": bound, "draws_decimal": list(map(str, draws)), "index": want, "attempts": attempts})
    assert rejected(lambda: uniform(10, iter([0] * 256).__next__))
    rows.append({"name": "bound10_exhaustion", "bound": 10, "repeated_draw": "0", "attempts": 256, "error": "sampling_exhausted"})
    return rows


def negative_cases(base):
    rows = []
    raw = canonical(base)
    for path, replacement in [
        (("manifest_version",), 1), (("manifest_version",), 3),
        (("generator_id",), "slug-fighter-manifest-candidate-v1"),
        (("table_hash",), V1.TABLE_HASH), (("table_hash",), "0" * 64),
        (("dna", "dna_version"), 2), (("portrait", "portrait_version"), 2),
        (("physical", "weight_lb"), 128 if base["physical"]["weight_lb"] != 128 else 129),
        (("name", "display"), base["name"]["display"] + "X"),
    ]:
        mutant = copy.deepcopy(base)
        target = mutant
        for key in path[:-1]:
            target = target[key]
        target[path[-1]] = replacement
        bad = canonical(mutant)
        assert rejected(lambda: parse(bad)) and rejected(lambda: render(bad))
        rows.append({"name": "wrong_" + "_".join(path) + "_" + str(replacement), "input_hex": bad.hex(), "accepted": False})
    # Even a perfectly formed different table name is not derived for this seed.
    mutant = copy.deepcopy(base)
    other = next(x for x in TABLE["first_names"] if x != base["name"]["first"])
    mutant["name"] = {"first": other, "last": base["name"]["last"], "display": other + " " + base["name"]["last"]}
    malformed = [
        ("nonderived_valid_table_name", canonical(mutant)),
        ("newline", raw + b"\n"), ("duplicate_version", raw.replace(b'{"manifest_version":2', b'{"manifest_version":2,"manifest_version":2', 1)),
        ("escaped_key", raw.replace(b'"name":', b'"na\\u006de":', 1)),
        ("fraction_version", raw.replace(b'"manifest_version":2', b'"manifest_version":2.0', 1)),
        ("unknown_field", raw[:-1] + b',"unknown":0}'),
        ("reordered_fields", canonical(dict(reversed(list(base.items()))))),
        ("over_size", b" " * 8193), ("deep_object", b'{"x":' * 9 + b'0' + b'}' * 9),
    ]
    for name, bad in malformed:
        assert rejected(lambda: parse(bad)) and rejected(lambda: render(bad))
        rows.append({"name": name, "input_hex": bad.hex(), "accepted": False})
    old = canonical(V1.generate(base["dna"]["seed"]))
    assert rejected(lambda: render(old, 2))
    assert rejected(lambda: PORTRAIT.render(raw, 1))
    assert rejected(lambda: render(raw, 1))
    renderer_cases = [{"renderer_version": version, "manifest_hex": data.hex(), "accepted": False} for version, data in [(2, old), (1, raw), (0, raw), (3, raw)]]
    return rows, renderer_cases


def descriptor_cases():
    d = APPROVAL["descriptor"]
    raw = canonical(d)
    expected_id = sha(DESCRIPTOR_DOMAIN + raw)
    cases = []
    for field, value in [("manifest_version", 1), ("renderer_version", 1), ("generator_id", V1.GENERATOR), ("manifest_table_hash", V1.TABLE_HASH), ("manifest_contract_hash", "ba8d33dbad3722eed6b4406ef120f946d7e06e1ea04305cbe8648889ba169e80"), ("renderer_id", PORTRAIT.RENDERER), ("renderer_contract_hash", PORTRAIT.CONTRACT_HASH)]:
        wrong = dict(d)
        wrong[field] = value
        wrong_bytes = canonical(wrong)
        wrong_id = sha(DESCRIPTOR_DOMAIN + wrong_bytes)
        assert wrong_id != expected_id and wrong_id != "c1a8650a39f695814db67fddf79815ef249a13a5c0cc5af0b906b061c4c3a303"
        cases.append({"name": "mixed_" + field, "descriptor": wrong, "canonical": wrong_bytes.decode(), "id": wrong_id, "supported_candidate": False})
    return {"descriptor": d, "canonical": raw.decode(), "preimage_hex": (DESCRIPTOR_DOMAIN + raw).hex(), "id": expected_id, "supported_candidate": True, "production_available": False}, cases


def corpus():
    baseline = baseline_check()
    seeds, coverage = selected_seeds()
    records, punctuation = [], {}
    for seed, reasons in seeds.items():
        value = generate(seed)
        raw = canonical(value)
        assert parse(raw) == value and generate(seed.upper()) == value
        svg = render(raw)
        old = V1.generate(seed)
        old_raw = canonical(old)
        old_svg = PORTRAIT.render(old_raw)
        assert body(svg) == body(old_svg)
        label = PORTRAIT.escape("Portrait preview of " + value["name"]["display"]).encode()
        assert b'aria-label="' + label + b'"' in svg and b'<title>' + label + b'</title>' in svg
        if "'" in value["name"]["last"]:
            punctuation.setdefault(value["name"]["last"], seed)
            assert b"&apos;" in svg
        first, last = indices(bytes.fromhex(seed))
        records.append({"seed": seed, "selection_reasons": reasons, "first_index": first, "last_index": last, "manifest": value, "canonical": raw.decode(), "manifest_digest": sha(MANIFEST_DOMAIN + raw), "svg": svg.decode(), "svg_sha256": sha(svg), "svg_digest": sha(SVG_DOMAIN + svg), "v1_manifest_digest": V1.digest_manifest(old_raw), "v1_svg_digest": sha(PORTRAIT.DOMAIN + old_svg), "non_name_values_equal_v1": True, "graphics_body_equal_v1": True, "graphics_body_sha256": sha(body(svg))})
    assert set(punctuation) == set(PLAN["apostrophe_cases"]["actual_surnames"])
    stream_values, order = streams()
    negatives, renderer_negatives = negative_cases(records[0]["manifest"])
    descriptor, descriptor_negatives = descriptor_cases()
    return {"version": 1, "scope": "independent public candidate byte expectations; no Go, PoW, keys or activation", "approval_sha256": sha((BUNDLE / "approval.json").read_bytes()), "input_lock_sha256": sha((HERE / "input-lock.json").read_bytes()), "baseline": baseline, "descriptor": descriptor, "descriptor_negative_cases": descriptor_negatives, "selection": coverage, "apostrophe_first_seeds": punctuation, "cases": records, "streams": stream_values, "stream_order_and_exhaustion": order, "sampling": sampling(), "invalid_manifest_inputs": negatives, "invalid_renderer_pairs": renderer_negatives}


def model(size, population):
    log_no = math.fsum(math.log1p(-k / size) for k in range(population)) if population <= size else None
    probability = -math.expm1(log_no) if log_no is not None else 1.0
    distinct = -size * math.expm1(population * math.log1p(-1 / size))
    return {"population": population, "pool_size": size, "any_collision_probability": probability, "collision_mathematically_guaranteed": population > size, "log_probability_no_collision": log_no, "equal_name_pairs_expected": population * (population - 1) / (2 * size), "distinct_names_expected": distinct, "extra_entries_expected": population - distinct, "fraction_with_namesake_expected": -math.expm1((population - 1) * math.log1p(-1 / size))}


def observed(counts, n):
    return {"population": n, "distinct_names": len(counts), "extra_entries": n - len(counts), "equal_name_pairs": sum(c * (c - 1) // 2 for c in counts.values()), "fighters_with_namesake": sum(c for c in counts.values() if c > 1), "fraction_with_namesake": sum(c for c in counts.values() if c > 1) / n, "any_collision": any(c > 1 for c in counts.values())}


def sample():
    plan = PLAN["sample"]
    domain = bytes.fromhex(plan["domain_hex"])
    counts = {1: Counter(), 2: Counter()}
    components = {1: [Counter(), Counter()], 2: [Counter(), Counter()]}
    observations = {1: [], 2: []}
    seen, first_seeds = set(), []
    input_digest = hashlib.sha256()
    for i in range(plan["count"]):
        seed = hashlib.sha256(domain + i.to_bytes(8, "little")).digest()
        assert seed not in seen
        seen.add(seed)
        input_digest.update(seed)
        if i < 10:
            first_seeds.append(seed.hex())
        old = tuple(V1.uniform_below(32, V1.Stream(seed, label).next) for label in ("name.first", "name.last"))
        new = indices(seed)
        for version, pair in [(1, old), (2, new)]:
            counts[version][pair] += 1
            for column, index in enumerate(pair):
                components[version][column][index] += 1
            if i + 1 in plan["scenarios"]:
                observations[version].append(observed(counts[version], i + 1))
    return {"version": 1, "input_lock_sha256": sha((HERE / "input-lock.json").read_bytes()), "plan": plan, "sample_seed_stream_sha256": input_digest.hexdigest(), "first10_public_seeds": first_seeds, "distinct_seed_count": len(seen), "interpretation": "Theoretical rows assume independent uniform names. Observed rows describe only these fixed public SHA256-derived inputs, not qualified/admitted work or a production forecast. A floating probability rounded to1 is not mathematical certainty. No release collision threshold is applied.", "theory": {str(v): [model(size, n) for n in plan["scenarios"]] for v, size in [(1, 1024), (2, 262144)]}, "observed": {str(v): observations[v] for v in (1, 2)}, "component_frequencies": {str(v): [[counter[i] for i in range(32 if v == 1 else 512)] for counter in components[v]] for v in (1, 2)}}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--sample", action="store_true")
    modes = parser.add_mutually_exclusive_group(required=True)
    modes.add_argument("--generate", action="store_true")
    modes.add_argument("--check", action="store_true")
    args = parser.parse_args()
    name = "collision-report.json" if args.sample else "reference-v2.json"
    result = sample() if args.sample else corpus()
    encoded = pretty(result)
    output = HERE / name
    if args.generate:
        with output.open("xb") as file:
            file.write(encoded)
    elif output.read_bytes() != encoded:
        raise SystemExit("FAIL: full independent regeneration differs")
    verify_inputs()
    summary = {"mode": "generate" if args.generate else "check", "file": name, "bytes": len(encoded), "sha256": sha(encoded), "result": "pass"}
    if not args.sample:
        summary.update(cases=len(result["cases"]), descriptor_id=result["descriptor"]["id"], coverage_last_integer=result["selection"]["last_integer_checked"])
    print(json.dumps(summary, sort_keys=True))


if __name__ == "__main__":
    main()
