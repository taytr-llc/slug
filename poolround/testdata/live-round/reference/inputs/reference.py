"""Independent modeled accounting windows; these are not PoW/issuer proofs."""
from collections import Counter
from fractions import Fraction
import json
from pathlib import Path

gym_a, gym_b = "11" * 32, "22" * 32
events = [
    {"gym": gym_a, "winner": False, "reward_atoms": 0},
    {"gym": gym_a, "winner": False, "reward_atoms": 0},
    {"gym": gym_b, "winner": True, "reward_atoms": 1000000123},
    {"gym": gym_a, "winner": False, "reward_atoms": 0},
    {"gym": gym_b, "winner": True, "reward_atoms": 1000000123},
    {"gym": gym_a, "winner": False, "reward_atoms": 0},
    {"gym": gym_b, "winner": True, "reward_atoms": 1000000001, "late": True},
]
for sequence, event in enumerate(events, 1):
    event.update(sequence=sequence, share_key=f"{sequence:064x}", work_hash=f"{100 + sequence:064x}")

cases = []
for through in (0, 2, 3, 6, 7):
    rounds, pending = [], []
    for event in events[:through]:
        pending.append(event)
        if not event["winner"]:
            continue
        counts = Counter(row["gym"] for row in pending)
        allocations = []
        for gym, count in sorted(counts.items()):
            amount = Fraction(event["reward_atoms"] * count, len(pending))
            allocations.append({"gym_id": gym, "shares": count,
                                "numerator": str(amount.numerator), "denominator": str(amount.denominator)})
        assert sum(Fraction(int(a["numerator"]), int(a["denominator"])) for a in allocations) == event["reward_atoms"]
        rounds.append({"sequence": len(rounds) + 1, "block_hash": event["work_hash"],
                       "first_accepted": pending[0]["sequence"], "last_accepted": event["sequence"],
                       "share_keys": [row["share_key"] for row in pending],
                       "reward_atoms": event["reward_atoms"], "allocations": allocations})
        pending = []
    cases.append({"through": through, "pending_shares": len(pending), "rounds": rounds})

result = {"version": 1, "scope": "Modeled accepted-sequence arithmetic only; no PoW, issuance or canonicality claims.",
          "events": events, "cases": cases}
Path(__file__).with_name("rounds_v1.json").write_text(json.dumps(result, indent=2) + "\n")
