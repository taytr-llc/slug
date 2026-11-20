#!/usr/bin/env python3
"""Independently check the retained actual input's manifest digest domains."""
import hashlib
import json
from pathlib import Path

HERE = Path(__file__).resolve().parent
raw = (HERE / "record.json").read_bytes()
assert hashlib.sha256(raw).hexdigest() == "849c5566b32135a5d89c5defcd36fc429ac5e624d6afb7640de6417d24692996"
record = json.loads(raw)

def canonical(value):
    return (json.dumps(value, separators=(",", ":"), ensure_ascii=False)
            .replace("&", "\\u0026").replace("<", "\\u003c")
            .replace(">", "\\u003e").encode())

for key, corner in zip(("a", "b"), record["source"]["corners"]):
    manifest = record["input"][key]["manifest"]
    assignment = corner["member"]["assignment"]
    assert manifest["manifest_version"] == 2
    assert assignment["descriptor_id"] == "cd02ae57b5918548244be2cabbe6c0c433feb5a1c165745e2e5c6bf235128e75"
    expected = assignment["manifest_digest"]
    assert hashlib.sha256(b"SLUG fighter manifest digest\0v2\0" + canonical(manifest)).hexdigest() == expected
    assert hashlib.sha256(b"SLUG fighter manifest digest\0v1\0" + canonical(manifest)).hexdigest() != expected

for name, identity, domain in (
    ("input", "input_id", b"SLUG fighter bout input\0v1\0"),
    ("result", "result_digest", b"SLUG fighter bout result\0v1\0"),
):
    assert hashlib.sha256(domain + canonical(record[name])).hexdigest() == record[identity]
assert record["result"]["input_id"] == record["input_id"]
print("PASS: exact retained input, both v2 manifest domains, unchanged engine-v1 input/result domains")
