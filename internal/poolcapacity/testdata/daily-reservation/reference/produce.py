#!/usr/bin/env python3
"""Independent conditional DAILY capacity oracle. Root executes after review.

Only frozen textual recipes and Python's standard library are inputs. This
program does not import a product encoder, inspect stores, invoke Go or use a
clock, entropy source or network. All expected arithmetic is computed here.
"""
import argparse
import hashlib
import json
import math
from pathlib import Path


BASE = Path(__file__).resolve().parent
SOURCE_READY = "625fc3cb19afe403bb4782051152e966dbbb077e13cc6d9ef38fb24534168e89"
INPUT_FILES = (
    "source-ready.json", "root-approval.json", "CONTRACT.md", "BYTES.md",
    "PLAN.md", "inputs/frames.json", "inputs/cases.json", "inputs/PROPOSAL.md",
    "../daily-capacity-sources.json", "../daily-capacity-peer-review.md",
)
INPUT_KEYS = ("source_max", "source_bytes", "rounds", "round_bytes",
              "credit_rounds", "batches", "gyms")
RESERVATION_KEYS = (
    "submissions", "blocks", "max_batches", "claim_leaves", "ledger_bytes",
    "round_bytes", "largest_round_bytes", "largest_credit_bytes",
    "largest_batch_bytes", "report_bytes", "receipt_bytes", "signing_bytes",
    "manager_copy_bytes",
)
U64 = (1 << 64) - 1
EXPECTED_CONSTANTS = {
    "max_money": 2100000000000000, "max_int64": (1 << 63) - 1,
    "max_rows": 16384, "max_batches": 32767, "window": 4096,
    "head_reserve": 131072, "report_reserve": 8388608,
    "delivery_reserve": 256, "logical_limit": 8589934592,
    "credit_limit": 131072, "batch_limit": 2097152, "round_limit": 2097152,
    "round_status_reserve": 4096, "winner_entry_limit": 65536,
    "submission_reserve": 4096, "worst_work_bytes": 16793600,
    "fraction_numerator_digits": 2490, "fraction_denominator_digits": 2468,
    "shape_inputs": 1464, "shape_payees": 256, "raw_limit": 400000,
    "signing_stream_limit": 671088640, "manager_copy_bytes": 335544320,
}


def require(condition, description):
    if not condition:
        raise ValueError(description)


def unique_object(pairs):
    result = {}
    for key, value in pairs:
        require(key not in result, "duplicate JSON field")
        result[key] = value
    return result


def parse(raw):
    return json.loads(raw, object_pairs_hook=unique_object,
                      parse_constant=lambda value: (_ for _ in ()).throw(
                          ValueError("nonfinite JSON number")))


def canonical(value):
    return json.dumps(value, ensure_ascii=True, allow_nan=False,
                      separators=(",", ":")).encode("ascii")


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def keys(value, expected):
    require(type(value) is dict and tuple(value) == tuple(expected),
            "missing, unknown or reordered fields")


def uint(value):
    return type(value) is int and 0 <= value <= U64


def decode_inputs(raw):
    value = parse(raw)
    keys(value, INPUT_KEYS)
    require(all(uint(x) for x in value.values()), "unsigned integer input")
    return value


def valid_inputs(value):
    if type(value) is not dict or tuple(value) != INPUT_KEYS:
        return False
    if not all(uint(x) for x in value.values()):
        return False
    return (1 <= value["source_max"] <= 16384
            and 4096 <= value["source_bytes"] <= 8589934592
            and 1 <= value["rounds"] <= 16384
            and 4096 <= value["round_bytes"] <= 8589934592
            and 1 <= value["credit_rounds"] <= 16384
            and 1 <= value["batches"] <= 32767
            and 1 <= value["gyms"] <= 256)


def load_inputs(lock_path):
    require(sha((BASE / "source-ready.json").read_bytes()) == SOURCE_READY,
            "source freeze changed")
    ready = parse((BASE / "source-ready.json").read_bytes())
    approval = parse((BASE / "root-approval.json").read_bytes())
    require(approval["approved_by"] == "root"
            and approval["source_ready_sha256"] == SOURCE_READY,
            "contract approval")
    for item in ready["files"]:
        raw = (BASE / item["path"]).read_bytes()
        require(len(raw) == item["bytes"] and sha(raw) == item["sha256"],
                "frozen input changed")
    for name in ("source_map", "premise_review"):
        item = ready[name]
        require(sha((BASE / item["path"]).read_bytes()) == item["sha256"],
                "frozen supporting input changed")
    lock = parse(lock_path.read_bytes())
    require(lock["version"] == 1 and lock["approved_by"] == "root",
            "producer execution is not approved")
    expected_paths = INPUT_FILES + ("produce.py", "PRODUCER.md")
    require(tuple(item["path"] for item in lock["files"]) == expected_paths,
            "lock inventory")
    for item in lock["files"]:
        raw = (BASE / item["path"]).read_bytes()
        require(len(raw) == item["bytes"] and sha(raw) == item["sha256"],
                "producer lock changed")
    return ready, lock


def frame_lengths(recipe):
    keys(recipe, ("version", "scope", "constants", "frames"))
    require(recipe["version"] == 1, "frame version")
    keys(recipe["constants"], EXPECTED_CONSTANTS)
    require(recipe["constants"] == EXPECTED_CONSTANTS, "constant recipe")
    frames = recipe["frames"]
    schemas = {
        "credit": ("round_hash", "round_digest", "coin"),
        "original_claim": ("round_hash", "round_height", "gym_id", "amount"),
        "input_reference": ("txid", "index"),
        "payment": ("gym_id", "script", "atoms"),
        "change": ("index", "atoms", "script", "claim_count", "claims_digest"),
        "batch": ("version", "sequence", "day", "created_at_ms", "inputs",
                  "payments", "fee_atoms", "change", "txid", "raw"),
        "reference": ("kind", "sequence"),
        "allocation": ("gym_id", "shares", "numerator", "denominator"),
        "round": ("version", "sequence", "block_hash", "block_height",
                  "first_accepted", "last_accepted", "share_keys", "pool_target",
                  "payout_address", "reward_atoms", "allocations", "winner"),
        "balance": ("gym_id", "confirmed", "available", "reserved_atoms",
                    "paid_atoms", "reason"),
        "report": ("day", "observed_at_ms", "txid", "state", "reason", "balances"),
        "residual_claim": ("round_hash", "round_height", "gym_id", "amount"),
    }
    keys(frames, schemas)
    for name, schema in schemas.items():
        keys(frames[name], schema)
    keys(frames["credit"]["coin"],
         ("txid", "index", "atoms", "script", "coinbase", "block_hash",
          "block_height", "claims"))
    for value in (frames["original_claim"]["amount"], frames["residual_claim"]["amount"],
                  frames["balance"]["confirmed"], frames["balance"]["available"]):
        keys(value, ("numerator", "denominator"))
    require(frames["batch"]["change"] == frames["change"], "change frame")
    require(int(frames["original_claim"]["amount"]["numerator"])
            == EXPECTED_CONSTANTS["max_money"] * 409600, "original claim numerator")
    require(int(frames["allocation"]["numerator"])
            == EXPECTED_CONSTANTS["max_money"] * 4096, "allocation numerator")
    require(len(frames["report"]["state"]) == 32
            and len(frames["report"]["reason"]) == 32
            and len(frames["balance"]["reason"]) == 32, "report literal bounds")
    lengths = {name: len(canonical(value)) for name, value in frames.items()}
    f = {
        "F_c": lengths["credit"], "F_q": lengths["original_claim"] + 1,
        "F_i": lengths["input_reference"] + 1, "F_p": lengths["payment"] + 1,
        "F_b": lengths["batch"], "F_x": lengths["reference"],
        "F_a": lengths["allocation"] + 1, "F_r": lengths["round"],
        "F_balance": lengths["balance"] + 2 * (2490 + 2468) + 1,
        "F_report": lengths["report"],
        "F_active": lengths["residual_claim"] + 2490 + 2468 + 1,
    }
    return lengths, f


def bounds(n, r, g, f):
    # Python integers remain exact even outside the successful API count domain.
    b = 0 if r == 0 else 2 * r - 1
    i, p = r + b, g * b
    c = 0 if r == 0 else n + r
    raw = 16 * b + 150 * i + 31 * (p + b)
    credits = r * (10 + f["F_c"]) + c * f["F_q"]
    batches = b * (10 + f["F_b"]) + i * f["F_i"] + p * f["F_p"] + 2 * raw
    indexes = (r * (67 + 8) + b * (67 + 8) + b * (13 + 8)
               + (r + b) * (10 + f["F_x"]) + (r + b) * (70 + f["F_x"])
               + i * (74 + 8))
    delivery = b * (11 + 256)
    w = min(n, 4096)
    round_total = (r * (f["F_r"] + 65536 + 4096) + 67 * n + f["F_a"] * n) if r else 0
    largest_round = (f["F_r"] + 65536 + 67 * w + f["F_a"] * min(g, w)) if r else 0
    largest_credit = (f["F_c"] + (min(g, 4096) + 1) * f["F_q"]) if r else 0
    largest_batch = (f["F_b"] + 1464 * f["F_i"] + 256 * f["F_p"] + 800000) if r else 0
    values = (n, r, b, c, 131072 + 8388608 + credits + batches + indexes + delivery,
              round_total, largest_round, largest_credit, largest_batch,
              f["F_report"] + g * f["F_balance"], 4096 * n,
              2 * c * f["F_active"] + 8388608,
              c * (64 + 64 + 2490 + 2468 + 256) + 1464 * 512 + 400000)
    reservation = dict(zip(RESERVATION_KEYS, values))
    components = {"historical_inputs": i, "payments": p, "raw_total": raw,
                  "credit_total": credits, "batch_total": batches,
                  "index_total": indexes, "delivery_total": delivery}
    return reservation, components


def zero_reservation():
    return dict.fromkeys(RESERVATION_KEYS, 0)


def recovery(in_, n, r, f):
    if not valid_inputs(in_) or not uint(n) or not uint(r) or r > n:
        return "invalid", zero_reservation(), ["invalid_input"]
    out, _ = bounds(n, r, in_["gyms"], f)
    tests = (
        (n <= in_["source_max"], "source_rows"),
        (r <= in_["rounds"], "round_rows"),
        (r <= in_["credit_rounds"], "credit_rows"),
        (out["max_batches"] <= in_["batches"], "batches"),
        (n + r <= 32768, "claims"),
        (r * EXPECTED_CONSTANTS["max_money"] <= EXPECTED_CONSTANTS["max_int64"], "money"),
        (all(uint(x) for x in out.values()), "uint64_conversion"),
        (out["receipt_bytes"] <= in_["source_bytes"], "receipt_bytes"),
        (out["ledger_bytes"] <= 8589934592, "ledger_bytes"),
        (out["round_bytes"] <= in_["round_bytes"], "round_bytes"),
        (out["largest_round_bytes"] <= 2097152, "largest_round_bytes"),
        (out["largest_credit_bytes"] <= 131072, "largest_credit_bytes"),
        (out["largest_batch_bytes"] <= 2097152, "largest_batch_bytes"),
        (out["report_bytes"] <= 8388608, "report_bytes"),
        (out["signing_bytes"] <= 671088640, "signing_bytes"),
        (out["manager_copy_bytes"] <= 335544320, "manager_copy_bytes"),
    )
    failures = [name for passed, name in tests if not passed]
    return ("capacity", zero_reservation(), failures) if failures else ("accept", out, [])


def vector(name, in_, n, r, f):
    classification, out, failed = recovery(in_, n, r, f)
    return {"name": name, "input": in_, "submissions": n, "blocks": r,
            "classification": classification, "reservation": out,
            "reservation_json": canonical(out).decode("ascii"),
            "failed_bounds": failed}


def ordinary(in_, n, f):
    if not valid_inputs(in_):
        return "invalid", zero_reservation(), ["invalid_input"]
    if not uint(n) or n > 4095:
        return "capacity", zero_reservation(), ["closing_window"]
    result, reservation, failed = recovery(in_, n + 1, n + 1, f)
    if result != "accept":
        return result, reservation, failed
    if 16793600 + 4096 * (n + 1) > in_["source_bytes"]:
        return "capacity", zero_reservation(), ["work_and_receipt_holdback"]
    return result, reservation, []


def binary_envelope(in_, f):
    probes = []

    def fits(n):
        classification, out, failed = ordinary(in_, n, f)
        probes.append({"n": n, "classification": classification})
        return classification, out, failed

    classification, out, failed = fits(0)
    if classification != "accept":
        return classification, 0, zero_reservation(), probes
    lo, hi = 0, 4095
    while lo < hi:
        mid = (lo + hi + 1) // 2
        classification, candidate, failed = fits(mid)
        if classification == "accept":
            lo, out = mid, candidate
        else:
            require(classification == "capacity", "search input unexpectedly invalid")
            hi = mid - 1
    require(len(probes) <= 13, "search probe bound")
    return "accept", lo, out, probes


def envelope_vector(case, profile, f):
    # Independent exhaustive oracle checks the selected binary algorithm.
    admitted, seen_refusal, digest = [], False, hashlib.sha256()
    for n in range(4096):
        classification, out, failed = ordinary(profile, n, f)
        require(classification in ("accept", "capacity"), "profile classification")
        if classification == "accept":
            require(not seen_refusal, "nonmonotone ordinary predicate")
            admitted.append(n)
        else:
            seen_refusal = True
        digest.update(canonical([n, classification, out]) + b"\n")
    classification, maximum, out, probes = binary_envelope(profile, f)
    require(len(probes) <= 13, "all search endpoints bounded")
    if admitted:
        require(classification == "accept" and maximum == admitted[-1], "binary/exhaustive mismatch")
        require(case.get("expected_maximum") == maximum, "frozen expected maximum")
        next_class, next_out, next_failed = ordinary(profile, maximum + 1, f)
        require(next_class == "capacity", "next ordinary candidate must refuse")
        next_case = {"n": maximum + 1, "classification": next_class,
                     "reservation": next_out, "failed_bounds": next_failed}
    else:
        require(classification == case.get("expected_error") == "capacity"
                and len(probes) == 1, "none-fit endpoint")
        next_case = {"n": 0, "classification": "capacity", "reservation": zero_reservation(),
                     "failed_bounds": ordinary(profile, 0, f)[2]}
    return {"name": case["name"], "input": profile, "classification": classification,
            "maximum": maximum, "reservation": out,
            "reservation_json": canonical(out).decode("ascii"),
            "next_failing_ordinary": next_case, "probes": probes,
            "probe_count": len(probes), "exhaustive_predicates": 4096,
            "exhaustive_accepts": len(admitted), "exhaustive_sha256": digest.hexdigest()}


def fraction_proof():
    lcms = [1]
    for value in range(1, 4097):
        lcms.append(lcms[-1] // math.gcd(lcms[-1], value) * value)
    terms, m, exponent = [], 1, 0
    while m < 4096:
        quotient = lcms[2 * m] // lcms[m]
        central = math.comb(2 * m, m)
        require(central % quotient == 0 and central <= 1 << (2 * m), "LCM binomial bound")
        exponent += 2 * m
        terms.append({"m": m, "quotient_divides_binomial": True, "power_exponent": 2 * m})
        m *= 2
    require(exponent == 8190 and lcms[-1] <= 1 << exponent, "LCM telescoping bound")
    d = 100 * lcms[-1]
    upper_d = 100 * (1 << exponent)
    upper_n = 16384 * EXPECTED_CONSTANTS["max_money"] * upper_d
    require(len(str(upper_d)) <= 2468 and len(str(upper_n)) <= 2490, "fraction width")
    require(all(d % (100 * w) == 0 for w in range(1, 4097)), "all original denominators")
    return {"window": 4096, "lcm": str(lcms[-1]), "common_denominator": str(d),
            "actual_denominator_digits": len(str(d)), "power_bound_exponent": exponent,
            "bound_denominator_digits": len(str(upper_d)),
            "bound_numerator_digits": len(str(upper_n)), "steps": terms,
            "denominator_preservation": "Only sums, differences and integer debits; no residual division."}


def malformed_vectors(profile):
    raw = canonical(profile).decode("ascii")
    variants = [
        ("duplicate", raw[:-1] + ',"source_max":1}'),
        ("unknown", raw[:-1] + ',"unknown":1}'),
        ("reordered", canonical(dict(reversed(list(profile.items())))).decode("ascii")),
        ("missing", canonical({k: v for k, v in profile.items() if k != "gyms"}).decode("ascii")),
    ]
    for name, replacement in (("true", "true"), ("false", "false"),
                              ("negative", "-1"), ("nonintegral", "1.5"),
                              ("uint64-overflow", str(U64 + 1))):
        variants.append((name, raw.replace('"source_max":16384', '"source_max":' + replacement, 1)))
    result = []
    for name, wire in variants:
        refused = False
        try:
            decoded = decode_inputs(wire)
            refused = not valid_inputs(decoded)
        except (ValueError, TypeError):
            refused = True
        require(refused, "malformed recipe control failed")
        result.append({"name": name, "input_json": wire, "classification": "invalid"})
    return result


def byte_edges(accepted, f):
    edges = []
    for case in accepted:
        for field, required in (("source_bytes", "receipt_bytes"), ("round_bytes", "round_bytes")):
            amount = case["reservation"][required]
            if amount < 4096:
                continue  # Exact zero is outside the selected input grammar.
            exact = dict(case["input"])
            exact[field] = amount
            at = vector(case["name"] + "/" + field + "/exact", exact,
                        case["submissions"], case["blocks"], f)
            require(at["classification"] == "accept", "exact configurable byte edge")
            edges.append(at)
            if amount > 4096:
                below = dict(exact)
                below[field] -= 1
                at = vector(case["name"] + "/" + field + "/one-below", below,
                            case["submissions"], case["blocks"], f)
                require(at["classification"] == "capacity", "one-byte-below edge")
                edges.append(at)
    return edges


def monotone_samples(f):
    samples = (0, 1, 2, 3, 7, 64, 256, 721, 4095, 4096, 4097, 4392, 4393, 8192, 16384)
    count, digest = 0, hashlib.sha256()
    for g in (2, 64, 256):
        for fixed in samples:
            for axis in ("n", "r"):
                previous = None
                for value in samples:
                    n, r = (value, fixed) if axis == "n" else (fixed, value)
                    if r > n:
                        continue
                    current, _ = bounds(n, r, g, f)
                    if previous is not None:
                        require(all(current[k] >= previous[k] for k in RESERVATION_KEYS),
                                "nonmonotone reservation field")
                        count += 1
                    digest.update(canonical([axis, g, n, r, current]) + b"\n")
                    previous = current
    return {"sample_counts": list(samples), "gyms": [2, 64, 256],
            "adjacent_comparisons": count, "sha256": digest.hexdigest(),
            "analytic_premise": "Nonnegative linear terms and nondecreasing min bounds; R=0 jumps upward."}


def source_holdback(envelopes):
    rows = []
    for case in envelopes:
        if case["classification"] != "accept":
            continue
        maximum = case["maximum"]
        for current in sorted({0, maximum // 2, maximum, maximum + 1}):
            slots = max(maximum - current, 0) + 1
            required = 16793600 + 4096 * slots
            for free in (required - 1, required):
                rows.append({"profile": case["name"], "maximum": maximum,
                             "retained_submissions": current, "reserved_receipt_slots": slots,
                             "required_free_bytes": required, "current_free_bytes": free,
                             "byte_predicate_fits": free >= required})
    return {"conditional_only": "Original current free bytes must come from audited metadata; this arithmetic does not permit issuance after drain or prove a pre-issuer no-effect fence.",
            "cases": rows}


def derive(ready, lock):
    recipe = parse((BASE / "inputs/frames.json").read_bytes())
    cases = parse((BASE / "inputs/cases.json").read_bytes())
    keys(cases, ("version", "scope", "profiles", "envelopes", "recoveries", "invalid_inputs",
                 "byte_edges", "ordering_cases", "monotonicity", "proof_edges"))
    require(cases["version"] == 1, "case version")
    lengths, f = frame_lengths(recipe)
    for profile in cases["profiles"].values():
        require(valid_inputs(decode_inputs(canonical(profile))), "selected profile")
    recoveries = []
    for case in cases["recoveries"]:
        expected_keys = ("name", "profile", "n", "r", "expected")
        if "conditional_only" in case:
            expected_keys += ("conditional_only",)
        keys(case, expected_keys)
        row = vector(case["name"], cases["profiles"][case["profile"]], case["n"], case["r"], f)
        require(row["classification"] == case["expected"], "frozen recovery classification")
        if "conditional_only" in case:
            row["conditional_only"] = case["conditional_only"]
        recoveries.append(row)
    envelopes = []
    for case in cases["envelopes"]:
        keys(case, ("name", "profile", "expected_maximum" if "expected_maximum" in case else "expected_error"))
        envelopes.append(envelope_vector(case, cases["profiles"][case["profile"]], f))
    require(envelopes[0]["probe_count"] == 13 and envelopes[-1]["probe_count"] == 1,
            "all-fit and none-fit probe controls")
    invalid = []
    for index, case in enumerate(cases["invalid_inputs"]):
        keys(case, ("field", "value"))
        require(case["field"] in INPUT_KEYS, "invalid-field recipe")
        profile = dict(cases["profiles"]["daily64"])
        profile[case["field"]] = case["value"]
        row = vector("invalid-%02d-%s" % (index + 1, case["field"]), profile, 1, 1, f)
        require(row["classification"] == "invalid", "invalid selected limit")
        invalid.append(row)
    accepted = [row for row in recoveries if row["classification"] == "accept"]
    fixed, components = bounds(16384, 16384, 256, f)
    require(fixed["manager_copy_bytes"] <= 335544320
            and fixed["signing_bytes"] <= 671088640, "separate request copy bounds")
    require(fixed["largest_credit_bytes"] <= 131072
            and fixed["largest_batch_bytes"] <= 2097152
            and fixed["largest_round_bytes"] <= 2097152
            and fixed["report_bytes"] <= 8388608
            and fixed["ledger_bytes"] <= 8589934592, "fixed codec maxima")
    overflow = EXPECTED_CONSTANTS["max_money"] * 16384
    require(overflow > U64, "uint64 overflow control")
    proof = fraction_proof()
    return {
        "version": 1,
        "scope": "Conditional independent arithmetic and codec bounds only; no source topology, PoW, canonicality, custody, funding, signing, performance or transport authority.",
        "source_ready_sha256": SOURCE_READY,
        "inputs": lock["files"],
        "constants": recipe["constants"], "frame_json_bytes": lengths, "formula_frames": f,
        "fraction_proof": proof,
        "fixed_grammar_maxima": {"reservation": fixed, "components": components,
                                 "conditional_only": "This maximal count tuple is monetarily refused; these are grammar diagnostics, not permission to admit it."},
        "exact_overflow_control": {"max_money_times_rows": str(overflow), "exceeds_uint64": True},
        "recoveries": recoveries, "envelopes": envelopes, "invalid_inputs": invalid,
        "malformed_inputs": malformed_vectors(cases["profiles"]["daily64"]),
        "byte_edges": byte_edges(accepted, f), "monotonicity": monotone_samples(f),
        "source_holdback": source_holdback(envelopes),
        "kernel_requirements_only": [
            {"case": "nonmember-request", "error": "unauthorized", "poison_or_drain": False},
            {"case": "retained-valid-nonmember", "error": "config"},
            {"case": "retained-malformed-gym", "error": "corrupt"},
        ],
        "kernel_scope": "These last three entries repeat frozen operation requirements; this pure producer has no owner/session/DB and does not test them.",
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--lock", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    require(args.output.is_absolute() and not args.output.exists(), "fresh absolute output required")
    ready, lock = load_inputs(args.lock)
    output = derive(ready, lock)
    # Repeat all source checks before publishing any output. The outer runner
    # independently binds source, elapsed time and final bytes as well.
    load_inputs(args.lock)
    raw = canonical(output) + b"\n"
    require(len(raw) <= 2 << 20, "finite output bound")
    with args.output.open("xb") as file:
        file.write(raw)
    print(json.dumps({"bytes": len(raw), "sha256": sha(raw),
                      "recoveries": len(output["recoveries"]),
                      "envelopes": len(output["envelopes"]),
                      "invalid_inputs": len(output["invalid_inputs"]),
                      "malformed_inputs": len(output["malformed_inputs"]),
                      "byte_edges": len(output["byte_edges"])}, separators=(",", ":")))


if __name__ == "__main__":
    main()
