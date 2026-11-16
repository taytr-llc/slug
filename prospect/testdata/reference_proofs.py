#!/usr/bin/env python3
"""Independent prospect-proof format 1 fixtures from PROSPECT_PROOFS.md.

Python constructs all protocol bytes, SHA256 commitments and IDs, SHA256d
transactions/branches, and expected DNA. The standalone argon_oracle.go imports
only the standard library and x/crypto/argon2, never a SLUG encoder or verifier.
It performs one production 256 MiB Argon2id calculation per fixed header.

No arguments (or --check) regenerates and checks proofs_v1.json. --generate
prints that reproducible array. Neither action mines, signs, publishes, reads
private keys, or contacts a node. Go dependency/toolchain downloads are disabled.

Both fixtures deliberately use a TEST-ONLY all-ones prospect target and an
artificial expected_bits of 0x01010000 (block target 1). These bits DO NOT match
the actual next-block bits at Phase A height 61. The known parent hash is only
a byte-binding example; it does not authenticate the supplied synthetic policy
or timestamps. These are not actual accepted pool shares. The second branch
contains opaque synthetic siblings, not a proven full block.
"""

import argparse
import hashlib
import json
import os
from pathlib import Path
import runpy
import struct
import subprocess


POLICY_DOMAIN = b"SLUG prospect policy\x00v1\x00"
GYM_DOMAIN = b"SLUG prospect gym\x00v1\x00"
ID_DOMAIN = b"SLUG prospect id\x00v1\x00"
GYM_ID = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
CONTEXT = {
    "version": 1,
    "network": 1397511495,
    "genesis": "7893676099cf9985bb198dfb6a347d9422a19f2422cf3326a999a7230ce246cd",
    "dna_version": 1,
    "prospect_target": "ff" * 32,
    "parent_hash": "1f08a4ac11d4c60ebecbb8dc85401fa2756576c3d1920c45cad5f7113b587ee0",
    "parent_height": 61,
    "expected_bits": 0x01010000,
    "min_timestamp": 1794780000,
    "max_timestamp": 1794794400,
}


def sha256(value):
    return hashlib.sha256(value).digest()


def sha256d(value):
    return sha256(sha256(value))


def le32(value):
    return struct.pack("<I", value)


def compact_size(value):
    if value < 0:
        raise ValueError("negative CompactSize")
    if value < 253:
        return bytes((value,))
    if value <= 0xFFFF:
        return b"\xfd" + struct.pack("<H", value)
    if value <= 0xFFFFFFFF:
        return b"\xfe" + le32(value)
    return b"\xff" + struct.pack("<Q", value)


def script_number_push(value):
    """Canonical positive BIP34/script-number encoding, without txscript."""
    if value < 1:
        raise ValueError("coinbase height must be positive")
    if value <= 16:
        return bytes((0x50 + value,))
    encoded = bytearray()
    while value:
        encoded.append(value & 255)
        value >>= 8
    if encoded[-1] & 0x80:
        encoded.append(0)
    return bytes((len(encoded),)) + encoded


def policy_id(context):
    return sha256(
        POLICY_DOMAIN
        + le32(context["network"])
        + bytes.fromhex(context["genesis"])
        + le32(context["dna_version"])
        + bytes.fromhex(context["prospect_target"])
    )


def commitment_script(context, gym_id):
    commitment = sha256(GYM_DOMAIN + policy_id(context) + bytes.fromhex(gym_id))
    return b"\x6a\x28SLUGPR01" + commitment


def coinbase_bytes(context, gym_id):
    script_sig = script_number_push(context["parent_height"] + 1) + b"\x00"
    script = commitment_script(context, gym_id)
    # Version 2, one null/max-index input, canonical height plus extranonce,
    # one 10-SLUG OP_TRUE output and one zero-value marked commitment output.
    return (
        le32(2)
        + compact_size(1)
        + bytes(32)
        + le32(0xFFFFFFFF)
        + compact_size(len(script_sig))
        + script_sig
        + le32(0xFFFFFFFF)
        + compact_size(2)
        + struct.pack("<q", 1_000_000_000)
        + compact_size(1)
        + b"\x51"
        + struct.pack("<q", 0)
        + compact_size(len(script))
        + script
        + le32(0)
    )


def make_proof(context, gym_id, siblings):
    coinbase = coinbase_bytes(context, gym_id)
    merkle_root = sha256d(coinbase)
    for sibling in siblings:
        if sibling == merkle_root:
            raise AssertionError("fixture contains a forbidden equal sibling")
        merkle_root = sha256d(merkle_root + sibling)
    header = (
        struct.pack("<i", 4)
        + bytes.fromhex(context["parent_hash"])[::-1]
        + merkle_root
        + le32(1794787200)
        + le32(context["expected_bits"])
        + le32(0)
    )
    if len(header) != 80 or len(coinbase) > 16384:
        raise AssertionError("fixture byte lengths violate the contract")
    return {
        "version": 1,
        "gym_id": gym_id,
        "header": header.hex(),
        "coinbase": coinbase.hex(),
        "merkle_branch": [sibling[::-1].hex() for sibling in siblings],
    }


def direct_argon_hashes(headers):
    repo = Path(__file__).resolve().parents[2]
    oracle = Path(__file__).with_name("argon_oracle.go").resolve()
    environment = os.environ.copy()
    # The repository already pins x/crypto. An unavailable local dependency or
    # toolchain must fail, rather than turn fixture checking into a download.
    environment.update(GOPROXY="off", GOSUMDB="off", GOTOOLCHAIN="local")
    result = subprocess.run(
        ["go", "run", str(oracle), *headers],
        cwd=repo,
        env=environment,
        check=True,
        capture_output=True,
        text=True,
        timeout=120,
    )
    records = json.loads(result.stdout)
    if len(records) != len(headers):
        raise AssertionError("oracle returned an unexpected result count")
    hashes = []
    for header, record in zip(headers, records):
        digest = bytes.fromhex(record["digest"])
        if record["header"] != header or len(digest) != 32:
            raise AssertionError("oracle returned the wrong header or digest size")
        if record["work_hash"] != digest[::-1].hex():
            raise AssertionError("oracle changed digest/display byte order")
        hashes.append(record["work_hash"])
    return hashes


def golden_records():
    context = dict(CONTEXT)
    # These bytes are deterministic placeholders for the hidden right-hand
    # transaction/subtree hashes. Their contents are deliberately not validated.
    branches = (
        ("synthetic-context-coinbase-only", []),
        (
            "synthetic-context-two-level-misses-block-target",
            [
                sha256d(b"SLUG prospect fixture synthetic sibling 1"),
                sha256d(b"SLUG prospect fixture synthetic sibling subtree 2"),
            ],
        ),
    )
    proofs = [make_proof(context, GYM_ID, siblings) for _, siblings in branches]
    work_hashes = direct_argon_hashes([proof["header"] for proof in proofs])
    repo = Path(__file__).resolve().parents[2]
    # Reuse only the independently authored Python DNA reference, never Go.
    dna_reference = runpy.run_path(str(repo / "fighter/testdata/reference_dna.py"))
    dna_reference["self_check"]()
    records = []
    for (name, _), proof, work_hash in zip(branches, proofs, work_hashes):
        work_value = int(work_hash, 16)
        if not 1 < work_value <= int(context["prospect_target"], 16):
            raise AssertionError("fixture must pass prospect target and miss block target 1")
        identifier = sha256(
            ID_DOMAIN
            + le32(context["network"])
            + bytes.fromhex(context["genesis"])
            + bytes.fromhex(work_hash)
        ).hex()
        records.append({
            "name": name,
            "context": dict(context),
            "proof": proof,
            "commitment_script": commitment_script(context, GYM_ID).hex(),
            "expected": {
                "version": 1,
                "verification": "supplied-context",
                "network": context["network"],
                "genesis": context["genesis"],
                "prospect_id": identifier,
                "policy_id": policy_id(context).hex(),
                "gym_id": GYM_ID,
                "work_hash": work_hash,
                "parent_hash": context["parent_hash"],
                "parent_height": context["parent_height"],
                "dna": dna_reference["derive_dna"](work_hash),
            },
        })
    return records


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    actions = parser.add_mutually_exclusive_group()
    actions.add_argument("--check", action="store_true", help="regenerate and check adjacent proofs_v1.json (default)")
    actions.add_argument("--generate", action="store_true", help="print the independently generated fixture array")
    args = parser.parse_args()
    records = golden_records()
    if args.generate:
        print(json.dumps(records, indent=2))
        return
    with Path(__file__).with_name("proofs_v1.json").open(encoding="utf-8") as handle:
        committed = json.load(handle)
    if records != committed:
        raise SystemExit("proofs_v1.json differs from independently regenerated records")
    print(f"verified {len(records)} independent prospect v1 records with production Argon2id")


if __name__ == "__main__":
    main()
