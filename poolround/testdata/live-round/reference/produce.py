"""Independent conditional round arithmetic/live-owner trace; NOT PoW authority.

Read only the frozen local recipe/oracle/helper/contract. No Go imports, product
execution, node, network, clock, entropy or private input. Invocation is not
authorized by this file: root reviews its hash, then supplies a fresh output.
"""
from collections import Counter
from fractions import Fraction
import hashlib
import json
import os
from pathlib import Path
import sys


BASE = Path(__file__).resolve().parent
PINS = {
    "trace-recipe.json": (16384, "81227908ba5cc4ccd01ece430f63c5bcf7016a652208058862da54ea92a624db"),
    "inputs/rounds_v1.json": (16384, "83697deac6c14bafc7051f603d232328fbda52dc46466a9db51cc7ce010ec17c"),
    "inputs/reference.py": (8192, "6955f98db2ba6265618790ea8394db877726b6953ef66883dec3b5b6cbd422d3"),
    "../CONTRACT.md": (16384, "ece2031c5d0619a2d573c0c39e646faa475a4a43af531e7f10cb5626baa3feb3"),
}
MAX_OUTPUT = 256 * 1024


def require(condition, label):
    if not condition:
        raise ValueError(label)


def read_inputs():
    data = {}
    for name, (limit, expected) in PINS.items():
        with (BASE / name).open("rb") as stream:
            raw = stream.read(limit + 1)
        require(0 < len(raw) <= limit, "input size")
        require(hashlib.sha256(raw).hexdigest() == expected, "input identity")
        data[name] = raw
    return data


def canonical(value):
    return json.dumps(value, sort_keys=True, separators=(",", ":"),
                      ensure_ascii=True, allow_nan=False).encode("ascii")


def projection(case):
    return {"through": case["through"], "rounds": len(case["rounds"]),
            "pending_shares": case["pending_shares"],
            "conditional_case_sha256": hashlib.sha256(canonical(case)).hexdigest()}


class Refusal(Exception):
    def __init__(self, cause, checked):
        self.cause = cause
        self.checked = checked


def derive(events, through, fault_at=None, cause=None):
    """Same Counter/Fraction arithmetic as the pinned independent helper.

    All state is local. A fault before the final event cannot update a caller's
    committed case; this is the modeled full-Sync atomicity contract.
    """
    require(type(through) is int and 0 <= through <= len(events), "through")
    rounds, pending = [], []
    for position, event in enumerate(events[:through], 1):
        if position == fault_at:
            raise Refusal(cause, {"through": position - 1,
                                  "pending_shares": len(pending), "rounds": rounds})
        require(event["sequence"] == position, "accepted order")
        pending.append(event)
        if not event["winner"]:
            continue
        counts = Counter(row["gym"] for row in pending)
        allocations = []
        for gym, count in sorted(counts.items()):
            amount = Fraction(event["reward_atoms"] * count, len(pending))
            allocations.append({"gym_id": gym, "shares": count,
                                "numerator": str(amount.numerator),
                                "denominator": str(amount.denominator)})
        total = sum(Fraction(int(a["numerator"]), int(a["denominator"]))
                    for a in allocations)
        require(total == event["reward_atoms"], "reward conservation")
        rounds.append({"sequence": len(rounds) + 1, "block_hash": event["work_hash"],
                       "first_accepted": pending[0]["sequence"],
                       "last_accepted": event["sequence"],
                       "share_keys": [row["share_key"] for row in pending],
                       "reward_atoms": event["reward_atoms"], "allocations": allocations})
        pending = []
    return {"through": through, "pending_shares": len(pending), "rounds": rounds}


def extend_events(recipe, original):
    spec = recipe["long_input"]
    require(spec["repeat_original_events"] == 3 and len(original) == 7, "fixed repetitions")
    require(spec["append_ordinary_gyms"] == ["A", "B"] and
            spec["appended_sequences"] == [22, 23] and
            spec["appended_reward_atoms"] == 0, "fixed tail")
    events = []
    for _ in range(3):
        for source in original:
            event = dict(source)
            sequence = len(events) + 1
            event.update(sequence=sequence, share_key=f"{sequence:064x}",
                         work_hash=f"{100 + sequence:064x}")
            events.append(event)
    for sequence, gym in ((22, "11" * 32), (23, "22" * 32)):
        events.append({"gym": gym, "winner": False, "reward_atoms": 0,
                       "sequence": sequence, "share_key": f"{sequence:064x}",
                       "work_hash": f"{100 + sequence:064x}"})
    require(len({e["share_key"] for e in events}) == 23 and
            len({e["work_hash"] for e in events}) == 23, "global identities")
    return events


def run_trace(recipe, events):
    committed = derive(events, 0)
    source_through, live = 0, False
    observations, confirmations, trace = {}, {}, []
    for step in recipe["transcript"]:
        op = step["op"]
        before = projection(committed)
        result = {"op": op, "before": before}
        if op == "original_full_sync":
            source_through = step["source_through"]
            candidate = derive(events, source_through)
            committed = candidate  # Single publication after complete derivation.
            require(committed["through"] == step["expect_committed_through"], "sync target")
            result["atomic_commit"] = True
        elif op == "begin_live":
            require(step["expect_full_audit"] is True, "begin audit")
            live = True
        elif op == "source_append":
            source_through = step["source_through"]
            require(committed["through"] == step["expect_committed_through"], "append is not sync")
        elif op == "original_full_sync_error":
            require(step["at"] == source_through == 23, "final-row fault")
            try:
                derive(events, source_through, step["at"], step["cause"])
            except Refusal as failure:
                result.update(refused=True, cause=failure.cause,
                              checked_candidate_prefix=projection(failure.checked),
                              atomic_commit=False)
            else:
                raise ValueError("missing modeled refusal")
            if step["cause"] == "last-row-corrupt":
                live = False
            require(projection(committed) == before and
                    committed["through"] == step["expect_committed_through"], "no partial commit")
        elif op == "restore_exact_fixture_and_reanchor":
            # Test-model restoration only; no production repair operation exists.
            require(step["expect_full_audit"] is True, "reanchor audit")
            live = True
        elif op == "live_list":
            require(live, "list original live handle")
            after = step["after"]
            require(0 <= after <= len(committed["rounds"]), "list cursor")
            page = committed["rounds"][after:after + 8]
            sequences = [row["sequence"] for row in page]
            require(sequences == step["expected_round_sequences"], "list page")
            result.update(cursor=after, round_sequences=sequences, records=page,
                          index_record_pairs=len(page), namespace_iterators=0)
        elif op == "live_refresh":
            require(live and 1 <= step["round"] <= len(committed["rounds"]), "refresh handle")
            number = step["round"]
            observation = {"canonical": step["canonical"], "descendants": step["descendants"]}
            observations[number] = observation
            if number not in confirmations and observation["canonical"] and observation["descendants"] >= 60:
                confirmations[number] = dict(observation)
            confirmation = confirmations.get(number)
            require((confirmation is not None) == step["expect_confirmation"], "first confirmation")
            if confirmation is not None:
                status = "confirmed" if observation["canonical"] else "funding-conflict"
            else:
                status = "provisional" if observation["canonical"] else "orphaned"
            result.update(round=number, observation=observation, confirmation=confirmation,
                          status=status, immutable_round=committed["rounds"][number - 1],
                          targeted_source_checks=2, node_observations=1, namespace_iterators=0)
        elif op == "full_walk":
            require(committed == derive(events, committed["through"]), "unchanged allocations")
            require(confirmations.get(1) == {"canonical": True, "descendants": 60}, "locked confirmation")
            require(observations.get(1) == {"canonical": False, "descendants": 0}, "later conflict")
            result.update(full_audit_required=True, locked_confirmation=confirmations[1])
        elif op == "close_reopen_full_audit":
            live = False
            require(committed == derive(events, step["expect_committed_through"]), "reopen exact")
            require(step["expect_old_handle"] == "invalid", "reopen handle")
            result.update(full_audit_required=True, old_handle_valid=False,
                          retained_confirmation=confirmations[1],
                          retained_observation=observations[1])
        else:
            raise ValueError("unknown trace operation")
        result.update(after=projection(committed), source_through=source_through, live_handle_valid=live)
        trace.append(result)
    return trace


def produce(data):
    recipe = json.loads(data["trace-recipe.json"])
    original = json.loads(data["inputs/rounds_v1.json"])
    require(recipe["version"] == 2 and original["version"] == 1, "input versions")
    require(recipe["old_oracle"]["sha256"] == PINS["inputs/rounds_v1.json"][1], "oracle pin")
    require(len(original["cases"]) == 5, "original case count")
    for case in original["cases"]:
        require(derive(original["events"], case["through"]) == case, "original arithmetic unchanged")
    events = extend_events(recipe, original["events"])
    cases = []
    for literal in recipe["literal_count_checks"]:
        case = derive(events, literal["through"])
        require(len(case["rounds"]) == literal["rounds"] and
                case["pending_shares"] == literal["pending"], "literal count check")
        cases.append(case)
    bounds = {"list_index_record_pairs": 8, "list_source_gate_calls": 1,
              "list_source_entries_nonzero_prefix": 0, "list_source_entries_zero_prefix": 1,
              "refresh_index_record_pairs": 1, "refresh_targeted_source_entries": 2,
              "refresh_node_observations": 1, "namespace_iterators": 0}
    require(recipe["warm_read_bounds"] == bounds, "fixed bounded-work contract")
    trace = run_trace(recipe, events)
    require(len(trace) == 15, "trace count")
    return {"version": 1,
            "scope": "Conditional projections only: arithmetic, full-Sync atomicity and required live-operation work bounds. Not PoW, issuer, node, storage or financial authority; not measured operation counts.",
            "input_sha256": {name: pin[1] for name, pin in PINS.items()},
            "original_arithmetic": original,
            "extended_arithmetic": {"events": events, "cases": cases},
            "trace": trace, "required_warm_work_bounds": bounds,
            "counts": {"original_cases": 5, "extended_cases": 2, "events": 23,
                       "trace_steps": 15, "modeled_refusals": 2, "list_pages": 3,
                       "refresh_states": 3},
            "invariants": recipe["invariants"]}


def main():
    require(len(sys.argv) == 3 and sys.argv[1] == "--output", "usage: produce.py --output FRESH_ABSOLUTE_FILE")
    output = Path(sys.argv[2])
    require(output.is_absolute(), "absolute output required")
    data = read_inputs()
    result = produce(data)
    raw = json.dumps(result, sort_keys=True, indent=2, ensure_ascii=True,
                     allow_nan=False).encode("ascii") + b"\n"
    require(len(raw) <= MAX_OUTPUT, "output bound")
    require(read_inputs() == data, "inputs changed during derivation")
    with output.open("xb") as stream:
        stream.write(raw)
        stream.flush()
        os.fsync(stream.fileno())
    print(json.dumps({"bytes": len(raw), "sha256": hashlib.sha256(raw).hexdigest(),
                      "counts": result["counts"]}, sort_keys=True))


if __name__ == "__main__":
    main()
