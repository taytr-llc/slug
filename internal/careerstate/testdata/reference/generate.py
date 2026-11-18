#!/usr/bin/env python3
"""Independent modeled career index/prefix equations. No new Go is an input."""
import argparse
import copy
import datetime
import hashlib
import json
from pathlib import Path
import re
import struct

ROOT = Path(__file__).resolve().parent
MAX_TIME = 8640000000000000
C = int(datetime.datetime(2026, 12, 12, 20, tzinfo=datetime.timezone.utc).timestamp()) * 1000
POLICY = '{"version":1,"scope":"two_fixture_standard_career","source":"result_v1_first_finality_after_activation","xp":"D42_standard_1_cap15","base_points":"D41_no_quality","effective_time":"first_finality_observed_at_ms","cutoff":"strict_before","later_conflict":"retain","c1":"zero_professional_record_only"}'
FIELDS = ("version", "award_id", "result_key", "card_id", "bout_id",
          "source_record_digest", "input_id", "result_digest", "finality_digest",
          "effective_at_ms", "saturday", "corner_a", "corner_b", "winner", "method")
TOTAL_FIELDS = ("wins", "losses", "draws", "xp", "base_points")


def j(value):
    text = json.dumps(value, ensure_ascii=False, separators=(",", ":"), allow_nan=False)
    for a, b in (("<", "\\u003c"), (">", "\\u003e"), ("&", "\\u0026"),
                 ("\u2028", "\\u2028"), ("\u2029", "\\u2029")):
        text = text.replace(a, b)
    return text.encode("utf-8")


def h(data):
    return hashlib.sha256(data).hexdigest()


def u32(n):
    return struct.pack("<I", n)


def u64(n):
    return struct.pack("<Q", n)


def lp(data):
    return u32(len(data)) + data


def raw(s):
    return bytes.fromhex(s)


def domain(s):
    return ("SLUG candidate career " + s + "\x00v1\x00").encode("ascii")


def ident(n):
    return "%064x" % n


def valid_id(s):
    return type(s) is str and re.fullmatch("[0-9a-f]{64}", s) is not None


def valid_time(n):
    return type(n) is int and 1 <= n <= MAX_TIME


POLICY_PREIMAGE = domain("policy") + lp(POLICY.encode("ascii"))
POLICY_ID = h(POLICY_PREIMAGE)


def activation(store, created, source_created, activated):
    preimage = domain("activation") + raw(POLICY_ID) + raw(store) + u64(created) + u64(source_created) + u64(activated)
    value = dict(version=1, policy_id=POLICY_ID, activation_id=h(preimage),
                 result_store_id=store, result_created_at_ms=created,
                 source_created_at_ms=source_created, activated_at_ms=activated)
    return value, preimage


def finality(t):
    # Structurally encoded synthetic bytes only: no signature, PoW, transaction,
    # canonicality, admission or finality authentication is asserted.
    checkpoint = dict(version=1, network=0x12141c16, genesis=ident(901),
                      height=100, hash=ident(902), signature="ab" * 64)
    anchor = dict(height=100, hash=ident(902), header="01" * 80,
                  txid=ident(903), raw_tx_digest=ident(904), raw_tx_size=260,
                  tx_count=2, tx_index=1, output_index=0,
                  payload=(b"SLUGRSLT\x01" + raw(ident(905)) + raw(ident(906))).hex(),
                  merkle_branch=[ident(907)])
    inclusion = dict(version=1, generation=1, anchor=anchor,
                     observed_at_ms=t - 1, observed_tip=dict(height=100, hash=ident(902)))
    return dict(checkpoint=checkpoint, inclusion=inclusion, observed_at_ms=t,
                observed_tip=dict(height=160, hash=ident(908)), descendants=60)


def finality_digest(f):
    return h(domain("finality") + lp(j(f)))


def event(a, n, t, pa, pb, winner, method):
    key = ident(1000 + n)
    award = h(domain("award") + raw(a["activation_id"]) + raw(key))
    corner = lambda p, gym: dict(prospect_id=ident(p), account_id=ident(300 + gym), gym_id=ident(400 + gym))
    return dict(version=1, award_id=award, result_key=key, card_id=ident(2000 + n),
                bout_id=ident(3000 + n), source_record_digest=ident(4000 + n),
                input_id=ident(5000 + n), result_digest=ident(6000 + n),
                finality_digest=finality_digest(finality(t)), effective_at_ms=t,
                saturday="2026-12-05", corner_a=corner(pa, 1), corner_b=corner(pb, 2),
                winner="" if winner == "draw" else ident(pa if winner == "a" else pb), method=method)


def delta(e, corner):
    # Independently repeat only the standard arithmetic in pinned prose.
    win = e["winner"] == e[corner]["prospect_id"]
    draw = e["winner"] == ""
    return dict(wins=int(win), losses=int(not draw and not win), draws=int(draw), xp=1,
                base_points=4 if draw else (10 if e["method"] == "decision" else 14) if win else 0)


def zero():
    return dict.fromkeys(TOTAL_FIELDS, 0)


def fold(total, d):
    result = {k: total[k] + d[k] for k in TOTAL_FIELDS}
    assert all(v <= 2**32 - 1 for v in result.values())
    result["xp"] = min(15, result["xp"])
    return result


def index_valid(a, events):
    if len(events) > 256:
        return False
    keys, awards = set(), set()
    for e in events:
        if tuple(e) != FIELDS or type(e["version"]) is not int or e["version"] != 1:
            return False
        if any(not valid_id(e[k]) for k in FIELDS[1:9]):
            return False
        if not valid_time(e["effective_at_ms"]) or e["effective_at_ms"] < a["activated_at_ms"]:
            return False
        if e["award_id"] != h(domain("award") + raw(a["activation_id"]) + raw(e["result_key"])):
            return False
        if e["result_key"] in keys or e["award_id"] in awards:
            return False
        keys.add(e["result_key"])
        awards.add(e["award_id"])
        try:
            date = datetime.date.fromisoformat(e["saturday"])
        except (ValueError, TypeError):
            return False
        if date.isoformat() != e["saturday"] or date.weekday() != 5:
            return False
        for c in (e["corner_a"], e["corner_b"]):
            if tuple(c) != ("prospect_id", "account_id", "gym_id") or not all(valid_id(v) for v in c.values()):
                return False
        if e["corner_a"]["prospect_id"] == e["corner_b"]["prospect_id"]:
            return False
        if e["method"] in ("decision_draw", "double_corner_stoppage"):
            if e["winner"] != "":
                return False
        elif e["method"] in ("decision", "ko", "tko_referee", "tko_three_knockdowns", "tko_corner"):
            if e["winner"] not in (e["corner_a"]["prospect_id"], e["corner_b"]["prospect_id"]):
                return False
        else:
            return False
        if len(j(e)) > 2048:
            return False
    return len(j(sorted(events, key=lambda e: (e["effective_at_ms"], e["award_id"])))) <= 544 * 1024


def snapshot(a, events, ids, as_of, mode, clock, state="live"):
    # Every fixture models a realizable audited owner, even when the request is
    # invalid/unfenced. Querying Current beyond ClockMS does not permit the
    # retained index itself to contain a committed event beyond ClockMS.
    assert valid_time(clock) and clock >= a["activated_at_ms"]
    assert all(e["effective_at_ms"] <= clock for e in events)
    # Refusal cases isolate one cause; no unspecified cross-error priority.
    if state != "live":
        return dict(error="career_" + state)
    if (not valid_time(as_of) or as_of < a["activated_at_ms"] or mode not in ("current", "cutoff")
        or not 1 <= len(ids) <= 10 or not all(valid_id(i) for i in ids)
        or ids != sorted(set(ids))):
        return dict(error="career_invalid")
    if mode == "cutoff" and clock < as_of:
        return dict(error="career_unfenced")
    assert index_valid(a, events)
    ordered = sorted(events, key=lambda e: (e["effective_at_ms"], e["award_id"]))
    included = [e for e in ordered if e["effective_at_ms"] < as_of or
                (mode == "current" and e["effective_at_ms"] == as_of)]
    preimage = (domain("prefix") + raw(a["activation_id"]) + u64(as_of) + lp(mode.encode("ascii"))
                + u32(len(ids)) + b"".join(raw(i) for i in ids) + u32(len(included))
                + b"".join(lp(j(e)) for e in included))
    totals = {i: zero() for i in ids}
    for e in included:
        for corner in ("corner_a", "corner_b"):
            pid = e[corner]["prospect_id"]
            if pid in totals:
                totals[pid] = fold(totals[pid], delta(e, corner))
    return dict(error="", digest=h(preimage), as_of_ms=as_of, mode=mode,
                prefix_award_ids=[e["award_id"] for e in included],
                totals=[dict(prospect_id=i, found=True, totals=totals[i]) for i in ids],
                unrequested_lookup=dict(prospect_id=ident(999), found=False, totals=zero()))


def build():
    a, ap = activation(ident(90), C - 300000, C - 400000, C - 200000)
    a2, ap2 = activation(ident(90), C - 300000, C - 400000, C - 199999)
    a3, ap3 = activation(ident(91), C - 300000, C - 400000, C - 200000)
    events = {}
    events["unrelated"] = event(a, 1, C - 2, 3, 4, "a", "ko")
    events["before"] = event(a, 2, C - 1, 1, 2, "a", "decision")
    events["equal"] = event(a, 3, C, 1, 2, "draw", "double_corner_stoppage")
    events["after"] = event(a, 4, C + 1, 1, 2, "b", "tko_corner")
    events["equal_later_commit"] = event(a, 5, C, 1, 2, "b", "ko")
    methods = ("decision", "ko", "tko_referee", "tko_three_knockdowns", "tko_corner",
               "decision_draw", "double_corner_stoppage")
    n = 10
    for method in methods:
        for winner in (("draw",) if method.endswith("draw") or method == "double_corner_stoppage" else ("a", "b")):
            name = method + "/" + winner
            events[name] = event(a, n, C - 1, 1, 2, winner, method)
            n += 1
    for k in range(16):
        events["cap-%02d" % k] = event(a, 100 + k, C - 1, 1, 2, "a", "ko")

    cases = []
    def case(name, labels, ids=(1, 2), as_of=C, mode="cutoff", clock=None, state="live", note="modeled captured audited prefix"):
        selected = [ident(i) if type(i) is int else i for i in ids]
        if clock is None:
            clock = max([C] + [events[label]["effective_at_ms"] for label in labels])
        inp = dict(activation="primary", events=labels, ids=selected, as_of_ms=as_of,
                   mode=mode, clock_ms=clock, owner_state=state)
        exp = snapshot(a, [events[v] for v in labels], selected, as_of, mode, clock, state)
        cases.append(dict(id=name, input=inp, expected=exp, provenance=note))
    all_events = ["after", "equal", "unrelated", "before"]
    case("empty-live-prefix", [])
    case("empty-current-prefix", [], mode="current")
    case("empty-other-asof", [], as_of=C + 1, clock=C + 1)
    for offset in (-1, 0, 1):
        for mode in ("current", "cutoff"):
            case("boundary/%s/%+d" % (mode, offset), all_events, as_of=C + offset, mode=mode, clock=C + 1)
    case("repeated-cutoff-capture", all_events)
    case("later-postcutoff-commit", all_events + ["equal_later_commit"], clock=C + 1000)
    case("post-conflict-retained-first-finality", all_events, note="Modeled mutable conflict overlay changed; the exact original Event/Finality bytes stay unchanged.")
    case("current-before-same-ms-commit", all_events, mode="current")
    case("current-after-same-ms-commit", all_events + ["equal_later_commit"], mode="current")
    case("unrelated-event-bound", ["before", "unrelated"])
    case("unrelated-event-absent", ["before"])
    case("selection-subset", all_events, ids=(1,))
    case("requested-known-zero", all_events, ids=(0,))
    case("requested-ten", all_events, ids=tuple(range(10)))
    case("current-does-not-require-cutoff-fence", ["before", "unrelated"], clock=C - 1, mode="current")
    case("unfenced-cutoff", ["before", "unrelated"], clock=C - 1)
    case("zero-asof", [], as_of=0)
    case("before-activation", [], as_of=a["activated_at_ms"] - 1)
    case("exact-activation", [], as_of=a["activated_at_ms"])
    case("max-safe-asof", [], as_of=MAX_TIME, clock=MAX_TIME)
    case("above-max-safe-asof", [], as_of=MAX_TIME + 1, clock=MAX_TIME)
    case("bad-mode", [], mode="Cutoff")
    for label, ids in (("empty", ()), ("unsorted", (2, 1)), ("duplicate", (1, 1)),
                       ("eleven", tuple(range(11))), ("short", ("0",)), ("uppercase", ("A" * 64,))):
        case("bad-ids/" + label, [], ids=ids)
    for state in ("inactive", "closed", "poisoned", "cancelled", "corrupt"):
        case("owner/" + state, [], state=state)
    for label in list(events)[5:17]:
        case("mapping/" + label, [label], mode="current")
    case("sixteen-results-xp-cap", ["cap-%02d" % i for i in range(16)], mode="current")
    case("same-prefix-different-input-order", list(reversed(all_events)))

    bad = []
    def refusal(name, values):
        bad.append(dict(id=name, events=values, expected_valid=index_valid(a, values),
                        note="Index invariant refusal, not an invented public API error code."))
    refusal("duplicate-result-and-award", [events["before"], copy.deepcopy(events["before"])])
    for field, value in (("award_id", ident(555)), ("effective_at_ms", a["activated_at_ms"] - 1),
                         ("winner", ident(999)), ("method", "submission"), ("version", 2)):
        v = copy.deepcopy(events["before"])
        v[field] = value
        refusal("bad-" + field, [v])
    v = copy.deepcopy(events["equal"])
    v["winner"] = ident(1)
    refusal("draw-with-winner", [v])

    def transition_valid(previous, proposed):
        if not index_valid(a, previous) or not index_valid(a, proposed):
            return False
        by_key = {v["result_key"]: j(v) for v in proposed}
        return all(by_key.get(v["result_key"]) == j(v) for v in previous)

    transitions = []
    def transition(name, previous, proposed):
        transitions.append(dict(id=name, previous=previous, proposed=proposed,
                                expected_valid=transition_valid(previous, proposed)))
    transition("first-finality", [], [events["before"]])
    transition("replay-identical-first-finality", [events["before"]], [events["before"]])
    transition("add-another-result", [events["before"]], [events["before"], events["after"]])
    transition("cannot-remove-after-conflict", [events["before"]], [])
    for field, value in (("result_digest", ident(777)), ("finality_digest", ident(778)),
                         ("effective_at_ms", C), ("winner", ident(2))):
        v = copy.deepcopy(events["before"])
        v[field] = value
        transition("immutable-" + field, [events["before"]], [v])
    v = copy.deepcopy(events["before"])
    v["corner_a"]["gym_id"] = ident(779)
    transition("immutable-original-gym", [events["before"]], [v])

    finalities = []
    for t in (C - 2, C - 1, C, C + 1):
        f = finality(t)
        finalities.append(dict(observed_at_ms=t, value=f, canonical_json=j(f).decode(),
                               digest=finality_digest(f), provenance="synthetic original-v1 encoding; not authenticated evidence"))
    return dict(version=1, policy=dict(json=POLICY, preimage_hex=POLICY_PREIMAGE.hex(), policy_id=POLICY_ID),
                cutoff=dict(saturday="2026-12-12", local="12:00 America/Los_Angeles", utc="2026-12-12T20:00:00Z", cutoff_ms=C),
                activations=[dict(id=name, value=v, canonical_json=j(v).decode(), preimage_hex=pre.hex())
                             for name, v, pre in (("primary", a, ap), ("different-time", a2, ap2), ("different-store", a3, ap3))],
                finalities=finalities,
                events=[dict(id=name, value=e, canonical_json=j(e).decode(),
                             award_preimage_hex=(domain("award") + raw(a["activation_id"]) + raw(e["result_key"])).hex())
                        for name, e in events.items()], snapshot_cases=cases,
                index_refusals=bad, index_transitions=transitions)


def verify(v):
    a = v["activations"][0]["value"]
    assert len({p["value"]["activation_id"] for p in v["activations"]}) == 3
    ev = {e["id"]: e["value"] for e in v["events"]}
    assert index_valid(a, list(ev.values()))
    assert all(len(j(e)) <= 2048 for e in ev.values())
    cases = {c["id"]: c["expected"] for c in v["snapshot_cases"]}
    base = cases["boundary/cutoff/+0"]
    for label in ("repeated-cutoff-capture", "later-postcutoff-commit", "post-conflict-retained-first-finality", "same-prefix-different-input-order"):
        assert cases[label] == base
    assert cases["unrelated-event-bound"]["totals"] == cases["unrelated-event-absent"]["totals"]
    assert cases["unrelated-event-bound"]["digest"] != cases["unrelated-event-absent"]["digest"]
    assert cases["current-before-same-ms-commit"]["digest"] != cases["current-after-same-ms-commit"]["digest"]
    assert len({cases[n]["digest"] for n in ("empty-live-prefix", "empty-current-prefix", "empty-other-asof")}) == 3
    assert base["totals"][0]["totals"] == dict(wins=1, losses=0, draws=0, xp=1, base_points=10)
    cap = cases["sixteen-results-xp-cap"]["totals"]
    assert cap[0]["totals"] == dict(wins=16, losses=0, draws=0, xp=15, base_points=224)
    assert cap[1]["totals"] == dict(wins=0, losses=16, draws=0, xp=15, base_points=0)
    assert cases["requested-known-zero"]["totals"][0] == dict(prospect_id=ident(0), found=True, totals=zero())
    assert all(not c["expected_valid"] for c in v["index_refusals"])
    assert [c["expected_valid"] for c in v["index_transitions"]] == [True] * 3 + [False] * 6
    for family in ("events", "activations", "snapshot_cases", "index_refusals", "index_transitions"):
        assert len({x["id"] for x in v[family]}) == len(v[family])


def main():
    p = argparse.ArgumentParser()
    g = p.add_mutually_exclusive_group(required=True)
    g.add_argument("--write", type=Path)
    g.add_argument("--check", type=Path)
    args = p.parse_args()
    index = json.loads((ROOT / "input_index.json").read_text())
    for path, entry in index["inputs"].items():
        if path.startswith("inputs/"):
            b = (ROOT / path).read_bytes()
            assert h(b) == entry["sha256"] and len(b) == entry["bytes"]
    contract = (ROOT / "inputs/CAREER_ACCOUNTING.md").read_text()
    assert contract.split("```json\n", 1)[1].split("\n```", 1)[0] == POLICY
    value = build()
    verify(value)
    value["input_sha256"] = {k: x["sha256"] for k, x in index["inputs"].items() if k.startswith("inputs/")}
    data = (json.dumps(value, ensure_ascii=True, sort_keys=True, separators=(",", ":")) + "\n").encode()
    if args.write:
        with args.write.open("xb") as f:
            f.write(data)
    else:
        assert args.check.read_bytes() == data
    print(json.dumps(dict(status="passed", sha256=h(data), bytes=len(data),
                         counts={k: len(value[k]) for k in ("activations", "finalities", "events", "snapshot_cases", "index_refusals", "index_transitions")})))


if __name__ == "__main__":
    main()
