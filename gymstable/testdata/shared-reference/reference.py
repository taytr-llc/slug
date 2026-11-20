#!/usr/bin/env python3
"""Independent Set59 public identity and source-cursor model; never invokes Go."""
import copy
import hashlib
import json
from pathlib import Path
import struct
import sys

ROOT = Path(__file__).resolve().parents[1]
SOURCE = ROOT / "reference/stable_v1.json"
SOURCE_SHA = "29f6a1302c2e04ade612fe2775c085c65b8bf33587c010060b727833182d2057"
PIN_KEYS = ["version", "account_authority_id", "account_id", "identity", "source", "condition_version", "projector_id"]
IDENTITY_KEYS = ["network", "genesis", "profile", "root_public_key", "gym_public_key", "gym_id"]
SOURCE_KEYS = ["version", "network", "genesis", "authority_id", "descriptor_id"]

def canonical(value):
    return json.dumps(value, ensure_ascii=False, separators=(",", ":")).encode()

def identity(pin, domain):
    raw = canonical(pin)
    preimage = domain + struct.pack("<I", len(raw)) + raw
    return raw.decode(), preimage.hex(), hashlib.sha256(preimage).hexdigest()

def page(owned, checkpoint, after, limit):
    candidates = [seq for seq in owned if seq > after]
    items = candidates[:limit]
    more = len(candidates) > limit
    next_after = items[-1] if more else checkpoint
    return {"after": after, "limit": limit, "items": items, "next_after": next_after, "has_more": more}

def generate():
    raw = SOURCE.read_bytes()
    if hashlib.sha256(raw).hexdigest() != SOURCE_SHA:
        raise RuntimeError("frozen independent Set41 input changed")
    original = json.loads(raw)
    pins = []
    common_source = original["pins"][0]["pin"]["source"]
    for entry in original["pins"]:
        p = copy.deepcopy(entry["pin"])
        assert list(p) == PIN_KEYS
        assert list(p["identity"]) == IDENTITY_KEYS and list(p["source"]) == SOURCE_KEYS
        _, _, old_id = identity(p, b"SLUG candidate stable\x00v1\x00")
        assert old_id == entry["expected_id"]
        p["version"] = 2
        p["source"] = copy.deepcopy(common_source)
        encoded, preimage, shared_id = identity(p, b"SLUG candidate shared stable\x00v1\x00")
        marker = {"application": "slug-candidate-shared-stable", "version": 2, "store_id": shared_id, "pin": p}
        pins.append({"fixture": entry["name"], "pin": p, "canonical": encoded,
                     "preimage": preimage, "id": shared_id,
                     "marker": canonical(marker).decode() + "\n", "legacy_id": old_id})

    # Owner labels are modeled input, not admissions or a source import.
    source = ["B", "A", "B", "B", "A", "B", "B", "B", "B", "A", "B", "B"]
    models = []
    for owner in ["A", "B"]:
        owned = [i + 1 for i, value in enumerate(source) if value == owner]
        sync = []
        count = 0
        for start in range(0, len(source), 8):
            end = min(start + 8, len(source))
            accepted = [seq for seq in owned if start < seq <= end]
            skipped = [seq for seq in range(start + 1, end + 1) if seq not in accepted]
            count += len(accepted)
            sync.append({"start": start, "checkpoint": end, "assignment_count": count,
                         "items": accepted, "skips": skipped})
        pages = [page(owned, len(source), after, limit) for after, limit in [(0, 2), (5, 2), (11, 2), (12, 8)]]
        recent = []
        for limit in [1, 2, 8]:
            count = min(limit, len(owned))
            offset = len(owned) - count
            recent.append({"after": owned[offset - 1] if offset else 0, "limit": limit,
                           "items": owned[offset:], "next_after": len(source), "has_more": False})
        models.append({"owner": owner, "owned": owned, "sync": sync, "pages": pages, "recent": recent})
    foreign_first = [{"checkpoint": 8, "assignment_count": 0, "items": []},
                     {"checkpoint": 9, "assignment_count": 1, "items": [9]}]
    return {"version": 1, "source_sha256": SOURCE_SHA,
            "generator_sha256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
            "pins": pins, "modeled_owners": source, "models": models,
            "foreign_only_first_page": foreign_first,
            "global_source_limit": 1000, "page_verification_limit": 8}

if __name__ == "__main__":
    output = json.dumps(generate(), ensure_ascii=False, indent=2) + "\n"
    if len(sys.argv) == 2:
        Path(sys.argv[1]).write_text(output)
    elif len(sys.argv) == 1:
        sys.stdout.write(output)
    else:
        raise SystemExit("usage: reference.py [output]")
