#!/usr/bin/env python3
"""Independent pool identity contract oracle; Python standard library only.

The input is the previously frozen Set 52 public job/template corpus. No Go
program is executed and no SLUG implementation output is imported. The source
field order and hash domains below are the explicit pool service v1 contract.
Run from any directory and redirect stdout to a fresh output file.
"""

import hashlib
import json
import pathlib
import struct


def sha256(value):
    return hashlib.sha256(value).hexdigest()


root = pathlib.Path(__file__).resolve().parents[2]
input_path = root / "prospectwork/testdata/pool_share_reference/pool_share_v1.json"
input_bytes = input_path.read_bytes()
input_digest = "2210d992e618b7a0952736dede1263b678f1a7a2d55d14ccde1cc6b29317de87"
assert sha256(input_bytes) == input_digest, "frozen independent input changed"
fixture = json.loads(input_bytes)["sources"][0]
assert fixture["name"] == "coinbase-only"
job = fixture["job"]
context = job["context"]

# Ordered maps express the exact published JSON field ordering, including all
# fields. These ASCII-only inputs need no JSON HTML or Unicode escape policy.
source = {
    "version": 1,
    "authority_pin": {
        "version": 5,
        "network": context["network"],
        "genesis": context["genesis"],
        "authority_id": job["authority_id"],
        "descriptor_id": job["descriptor_id"],
    },
    "policy": {
        "version": context["version"],
        "network": context["network"],
        "genesis": context["genesis"],
        "dna_version": context["dna_version"],
        "prospect_target": context["prospect_target"],
    },
    "payout_address": fixture["payout_address"],
    "pool_target": "f" * 64,
    "max_works": 8,
    "max_submissions": 32,
    "max_bytes": 1 << 20,
}
source_canonical = json.dumps(source, ensure_ascii=True, separators=(",", ":"))
source_id = sha256(b"SLUG pool service source\x00v1\x00" + source_canonical.encode("ascii"))
origin = "http://127.0.0.1:19056"
origin_bytes = origin.encode("ascii")
manifest_preimage = (
    b"SLUG manifest transport trust\x00v1\x00"
    + struct.pack("<I", len(origin_bytes))
    + origin_bytes
    + b"\x01"  # Literal-loopback HTTP trust mode.
    + struct.pack("<I", 0)  # No CA certificate fingerprints.
)
manifest_trust = sha256(manifest_preimage)
pool_preimage = (
    b"SLUG pool transport trust\x00v1\x00"
    + manifest_trust.encode("ascii")
    + source_id.encode("ascii")
    + job["public_key"].encode("ascii")
)
result = {
    "version": 1,
    "scope": "Public pool source and literal-loopback transport identities; no work or receipt authority",
    "provenance": {
        "input_path": str(input_path.relative_to(root)),
        "input_sha256": input_digest,
        "oracle": "Python standard-library hashlib, json and struct; no Go execution or harvested output",
        "oracle_sha256": sha256(pathlib.Path(__file__).read_bytes()),
    },
    "client": {
        "version": 1,
        "url": origin,
        "token": "ab" * 32,  # Public nonsecret credential fixture, excluded from identities.
        "ca_certificate": "",
        "public_key": job["public_key"],
        "source": source,
    },
    "source_canonical": source_canonical,
    "source_id": source_id,
    "manifest_http_preimage_hex": manifest_preimage.hex(),
    "manifest_http_trust_id": manifest_trust,
    "pool_transport_preimage_hex": pool_preimage.hex(),
    "pool_transport_trust_id": sha256(pool_preimage),
}
print(json.dumps(result, indent=2, ensure_ascii=True) + "\n", end="")
