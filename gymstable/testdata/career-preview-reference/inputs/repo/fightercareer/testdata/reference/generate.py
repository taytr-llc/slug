#!/usr/bin/env python3
"""Independent D41/D42 arithmetic model; Python standard library only.

Consumes only pinned prose, never Go or a Go result. JSON is a test artifact,
not a proposed production codec. Error outputs/precedence beyond the contract
are deliberately not asserted.
"""
import argparse
import hashlib
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parent
MAX = 2**32 - 1
FIELDS = ("wins", "losses", "draws", "xp", "base_points")
METHODS = ("decision", "ko", "tko_referee", "tko_three_knockdowns",
           "tko_corner", "decision_draw", "double_corner_stoppage")
KINDS = ("standard", "title")
BASE_SHAPES = ((1, 0, 0, 10), (1, 0, 0, 14), (0, 1, 0, 0), (0, 0, 1, 4))


class Invalid(ValueError):
    pass


class Overflow(ValueError):
    pass


def row(wins=0, losses=0, draws=0, xp=0, base_points=0):
    return dict(zip(FIELDS, (wins, losses, draws, xp, base_points)))


def award(winner, method, kind):
    if kind not in KINDS or method not in METHODS:
        raise Invalid()
    xp = 1 if kind == "standard" else 2
    if method in ("decision_draw", "double_corner_stoppage"):
        if winner != "draw":
            raise Invalid()
        return [row(draws=1, xp=xp, base_points=4),
                row(draws=1, xp=xp, base_points=4)]
    if winner not in ("a", "b"):
        raise Invalid()
    points = 10 if method == "decision" else 14
    result = [row(losses=1, xp=xp), row(losses=1, xp=xp)]
    result[0 if winner == "a" else 1] = row(wins=1, xp=xp, base_points=points)
    return result


def apply(prior, delta):
    # Corpus inputs are all representable by the API's uint32 value fields.
    if prior["xp"] > 15:
        raise Invalid()
    shape = tuple(delta[k] for k in ("wins", "losses", "draws", "base_points"))
    if delta["xp"] not in (1, 2) or shape not in BASE_SHAPES:
        raise Invalid()
    result = {k: prior[k] + delta[k] for k in FIELDS}
    if any(result[k] > MAX for k in FIELDS if k != "xp"):
        raise Overflow()
    result["xp"] = min(15, result["xp"])
    return result


def quality(rank):
    if rank == 0:
        raise Invalid()
    return 6 if rank <= 5 else 3 if rank <= 10 else 0


def expected(fn, key, *args):
    try:
        return {"error": "", key: fn(*args)}
    except Invalid:
        return {"error": "ErrInvalid"}
    except Overflow:
        return {"error": "ErrOverflow"}


def inputs():
    index = json.loads((ROOT / "input_index.json").read_text())
    result = {}
    for name, item in index["inputs"].items():
        data = (ROOT / name).read_bytes()
        digest = hashlib.sha256(data).hexdigest()
        assert digest == item["sha256"] and len(data) == item["bytes"]
        result[name] = digest
    return result


def corpus():
    awards, applies, qualities = [], [], []

    def add_award(name, winner, method, kind):
        awards.append({"id": name, "input": {"winner": winner, "method": method,
                       "kind": kind}, "expected": expected(award, "deltas", winner, method, kind)})

    # Complete supported method/winner/kind product, including invalid pairs.
    for kind in KINDS:
        for method in METHODS:
            for winner in ("a", "b", "draw"):
                add_award("%s/%s/%s" % (kind, method, winner), winner, method, kind)
    for i, winner in enumerate(("", "A", "B", "Draw", "none", " a", "a ", "a\x00")):
        add_award("bad-winner-%d" % i, winner, "decision", "standard")
    for i, method in enumerate(("", "Decision", "KO", "draw", "submission", "ko ", "ko\x00")):
        add_award("bad-method-%d" % i, "a", method, "standard")
    for i, kind in enumerate(("", "Standard", "TITLE", "exhibition", "title ", "title\x00")):
        add_award("bad-kind-%d" % i, "a", "ko", kind)

    def add_apply(name, prior, delta):
        applies.append({"id": name, "input": {"prior": prior, "delta": delta},
                        "expected": expected(apply, "totals", prior, delta)})

    shapes = []
    for w, l, d, p in BASE_SHAPES:
        for xp in (1, 2):
            shapes.append(row(w, l, d, xp, p))
    for n, delta in enumerate(shapes):
        for xp in (0, 13, 14, 15):
            add_apply("shape-%d/prior-xp-%d" % (n, xp), row(7, 11, 13, xp, 17), delta)
        add_apply("shape-%d/empty-history" % n, row(), delta)
        # Untouched fields at max are legal; the incremented counter must fit.
        prior = row(MAX, MAX, MAX, 15, MAX - delta["base_points"])
        incremented = next(k for k in ("wins", "losses", "draws") if delta[k])
        prior[incremented] -= 1
        add_apply("shape-%d/exact-max" % n, prior, delta)
        prior = dict(prior)
        prior[incremented] = MAX
        add_apply("shape-%d/counter-overflow" % n, prior, delta)
        if delta["base_points"]:
            prior = row(xp=14, base_points=MAX - delta["base_points"] + 1)
            add_apply("shape-%d/point-overflow-by-one" % n, prior, delta)
        add_apply("shape-%d/bad-prior-xp-16" % n, row(xp=16), delta)
    add_apply("bad-prior-xp-max", row(xp=MAX), shapes[0])

    # Explicit malformed shapes isolate validation from arithmetic overflow.
    bad_deltas = [row(), row(wins=1, xp=1), row(wins=1, xp=2, base_points=4),
                  row(losses=1, xp=1, base_points=10), row(draws=1, xp=1),
                  row(draws=1, xp=2, base_points=14), row(xp=1, base_points=4),
                  row(wins=1, losses=1, xp=1, base_points=10),
                  row(wins=1, draws=1, xp=2, base_points=14),
                  row(losses=1, draws=1, xp=1, base_points=4),
                  row(wins=2, xp=1, base_points=10), row(losses=2, xp=1),
                  row(draws=2, xp=2, base_points=4),
                  row(wins=MAX, xp=1, base_points=10),
                  row(wins=1, xp=1, base_points=MAX)]
    for xp in (0, 3, MAX):
        for delta in shapes[::2]:
            bad_deltas.append(dict(delta, xp=xp))
    for n, delta in enumerate(bad_deltas):
        add_apply("bad-delta-%d" % n, row(), delta)

    for rank in (0, 1, 2, 5, 6, 7, 10, 11, MAX - 1, MAX):
        qualities.append({"id": "rank-%d" % rank, "input": {"opponent_rank": rank},
                          "expected": expected(quality, "bonus", rank)})

    # Replay is intentionally ordinary arithmetic, not deduplication.
    sequences = []
    for name, initial, deltas in (
        ("repeat-standard-decision", row(), [shapes[0], shapes[0]]),
        ("repeat-title-finish-at-cap", row(xp=14), [shapes[3], shapes[3]]),
        ("draw-loss-finish", row(xp=13), [shapes[6], shapes[4], shapes[2]]),
    ):
        prior, steps = initial, []
        for delta in deltas:
            prior = apply(prior, delta)
            steps.append({"delta": delta, "totals": prior})
        sequences.append({"id": name, "initial": initial, "steps": steps})
    return {"version": 1, "input_sha256": inputs(), "award_cases": awards,
            "apply_cases": applies, "quality_cases": qualities,
            "apply_sequences": sequences}


def encoded(value):
    return (json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=True) + "\n").encode()


def verify(value):
    # Hand-readable anchors and algebraic invariants independent of enumeration.
    assert award("a", "decision", "standard") == [row(1, 0, 0, 1, 10), row(0, 1, 0, 1, 0)]
    assert award("b", "ko", "title") == [row(0, 1, 0, 2, 0), row(1, 0, 0, 2, 14)]
    assert award("draw", "double_corner_stoppage", "title") == [row(0, 0, 1, 2, 4)] * 2
    assert apply(row(7, 11, 13, 14, 17), row(1, 0, 0, 2, 14)) == row(8, 11, 13, 15, 31)
    assert apply(row(xp=15), row(draws=1, xp=1, base_points=4)) == row(0, 0, 1, 15, 4)
    assert [quality(n) for n in (1, 5, 6, 10, 11, MAX)] == [6, 6, 3, 3, 0, 0]
    valid_awards = [v for v in value["award_cases"] if not v["expected"]["error"]]
    assert len(valid_awards) == 24
    distinct = set()
    for item in valid_awards:
        a, b = item["expected"]["deltas"]
        assert all(d["wins"] + d["losses"] + d["draws"] == 1 for d in (a, b))
        assert a["wins"] == b["losses"] and b["wins"] == a["losses"]
        assert a["draws"] == b["draws"] and a["xp"] == b["xp"]
        distinct.update(tuple(d[k] for k in FIELDS) for d in (a, b))
    assert len(distinct) == 8
    for family in ("award_cases", "apply_cases", "quality_cases", "apply_sequences"):
        assert len({v["id"] for v in value[family]}) == len(value[family])
    for item in value["apply_cases"]:
        prior, delta = item["input"]["prior"], item["input"]["delta"]
        for values in (prior, delta):
            assert tuple(values) == FIELDS
            assert all(type(v) is int and 0 <= v <= MAX for v in values.values())
        exp = item["expected"]
        if not exp["error"]:
            result = exp["totals"]
            assert sum(result[k] - prior[k] for k in ("wins", "losses", "draws")) == 1
            assert 0 <= result["xp"] <= 15 and prior["xp"] <= result["xp"]
            assert all(v <= MAX for v in result.values())
        else:
            assert set(exp) == {"error"}
    assert value["apply_sequences"][0]["steps"][-1]["totals"] == row(2, 0, 0, 2, 20)
    assert value["apply_sequences"][1]["steps"][-1]["totals"] == row(2, 0, 0, 15, 28)


def main():
    p = argparse.ArgumentParser()
    mode = p.add_mutually_exclusive_group(required=True)
    mode.add_argument("--write", type=Path, help="exclusive fresh corpus destination")
    mode.add_argument("--check", type=Path, help="require exact regenerated bytes")
    args = p.parse_args()
    value = corpus()
    verify(value)
    data = encoded(value)
    if args.write:
        with args.write.open("xb") as f:
            f.write(data)
    else:
        assert args.check.read_bytes() == data, "corpus byte mismatch"
    counts = {k: len(value[k]) for k in ("award_cases", "apply_cases", "quality_cases", "apply_sequences")}
    print(json.dumps({"status": "passed", "corpus_sha256": hashlib.sha256(data).hexdigest(),
                      "bytes": len(data), "counts": counts}, sort_keys=True))


if __name__ == "__main__":
    main()
