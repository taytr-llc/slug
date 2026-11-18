#!/usr/bin/env python3
"""Finite independent commitment arithmetic from pinned consumer prose.

The author reviewed Go source, but no Go values/outputs or implementation calls
are inputs to this derivation. The two inherited snapshots were frozen before
careerstate Go comparisons. This is not an authenticated C1 admission fixture.
"""
import hashlib
import json
from pathlib import Path
import struct
import sys

HERE = Path(__file__).resolve().parent
PINS = {
    "CAREER_CONSUMERS.md": "1b30cc954d7f54c80a8ff3568edba6e81f9615fe7c288be808415d04dad3a56e",
    "selected_inputs.json": "38dfdfc62618335ae034fccd8d3d575225017275bc957f067f71b3dfacee6d44",
}
KEY_DOMAIN = b"SLUG candidate career consumer cutoff\x00v1\x00"
SET_DOMAIN = b"SLUG candidate career consumer receipts\x00v1\x00"


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def canonical(value):
    # All chosen scalars are decimal integers/lowercase hex; no string escaping
    # ambiguity is present. Dict insertion order is the declared field order.
    return json.dumps(value, ensure_ascii=True, separators=(",", ":")).encode("ascii")


def lp(raw):
    return struct.pack("<I", len(raw)) + raw


def main():
    for name, expected in PINS.items():
        assert sha((HERE / name).read_bytes()) == expected, name
    selected = json.loads((HERE / "selected_inputs.json").read_bytes())
    activation = json.loads(selected["activation"]["canonical_json"])
    assert canonical(activation).decode() == selected["activation"]["canonical_json"]
    receipts = []
    for pointer, case in zip(selected["snapshot_pointers"], selected["snapshots"]):
        inp, expected = case["input"], case["expected"]
        assert expected["error"] == "" and expected["mode"] == "cutoff"
        assert inp["clock_ms"] >= inp["as_of_ms"] >= activation["activated_at_ms"]
        ids = inp["ids"]
        assert 1 <= len(ids) <= 10 and sorted(set(ids)) == ids
        assert all(x["found"] and all(x["totals"][k] == 0 for k in ("wins", "losses", "draws", "xp")) for x in expected["totals"])
        ids_raw = canonical(ids)
        cutoff_raw = struct.pack("<Q", inp["as_of_ms"])
        key_input = KEY_DOMAIN + cutoff_raw + ids_raw
        key = "career_cutoff:" + sha(key_input)
        row = {
            "version": 1,
            "activation_id": activation["activation_id"],
            "cutoff_ms": inp["as_of_ms"],
            "prospect_ids": ids,
            "career_digest": expected["digest"],
        }
        raw = canonical(row)
        assert len(raw) <= 2048
        receipts.append({
            "id": case["id"], "source_pointer": pointer,
            "row": row, "canonical_json": raw.decode(), "row_bytes": len(raw),
            "key": key, "key_ascii_hex": key.encode().hex(),
            "cutoff_le64_hex": cutoff_raw.hex(), "ids_canonical_json": ids_raw.decode(),
            "key_preimage_hex": key_input.hex(),
        })
    cases = []
    for name, order in (("empty", []), ("one", [0]), ("two", [0, 1]), ("two-reversed", [1, 0])):
        ordered = sorted((receipts[i] for i in order), key=lambda r: r["key"])
        prefix = SET_DOMAIN + struct.pack("<I", len(order))
        preimage = prefix + b"".join(lp(r["key"].encode()) + lp(r["canonical_json"].encode()) for r in ordered)
        digest = sha(preimage)
        binding = {"version": 1, "activation": activation, "fence_count": len(order), "fence_digest": digest}
        encoded = canonical(binding)
        assert len(encoded) <= 2048
        cases.append({
            "id": name, "input_receipt_indices": order,
            "sorted_keys": [r["key"] for r in ordered],
            "fence_count": len(order), "preimage_hex": preimage.hex(),
            "fence_digest": digest, "binding_canonical_json": encoded.decode(),
            "binding_bytes": len(encoded),
        })
    assert cases[2]["fence_digest"] == cases[3]["fence_digest"]
    assert len({c["fence_digest"] for c in cases}) == 3
    out = {"version": 1, "input_sha256": PINS,
           "scope": "Prose-derived finite byte arithmetic; modeled inputs; no Go outputs, process, durability, admission or chain-validation claim.",
           "key_domain_hex": KEY_DOMAIN.hex(), "set_domain_hex": SET_DOMAIN.hex(),
           "receipts": receipts, "cases": cases}
    sys.stdout.buffer.write((json.dumps(out, indent=2, sort_keys=True) + "\n").encode("ascii"))


if __name__ == "__main__":
    main()
