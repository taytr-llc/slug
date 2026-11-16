#!/usr/bin/env python3
"""Independent public TEST-ONLY D69 block-target winner fixture.

The Python set20 builder and prior job/signature/DNA references supply all
bytes. The standalone x/crypto Argon2id oracle hashes at most sixteen fixed
headers sequentially. No SLUG Go encoder/verifier supplies expected outputs,
no private files are read, and no node is contacted or block submitted.
Scalar 1 is deliberately public fixture material, never a real wallet key.
"""

import argparse
import hashlib
import json
from pathlib import Path
import runpy
import struct


def golden_record():
    repo = Path(__file__).resolve().parents[2]
    templates = runpy.run_path(str(repo / "prospectwork/testdata/reference_templates.py"))
    jobs = runpy.run_path(str(repo / "prospectjob/testdata/reference_jobs.py"))
    proofs = runpy.run_path(str(repo / "prospect/testdata/reference_proofs.py"))
    auth = runpy.run_path(str(repo / "gymauth/testdata/reference_authorizations.py"))
    dna = runpy.run_path(str(repo / "fighter/testdata/reference_dna.py"))
    fixture = templates["golden_records"]()[0]
    job, context = fixture["job"], fixture["job"]["context"]
    candidate = fixture["expected"]
    proof = candidate["proof"]
    prefix = bytes.fromhex(proof["header"])[:76]
    target = int(fixture["template"]["block"]["target"], 16)
    for nonce in range(16):
        header = (prefix + struct.pack("<I", nonce)).hex()
        work_hash = proofs["direct_argon_hashes"]([header])[0]
        if int(work_hash, 16) <= target:
            break
    else:
        raise AssertionError("bounded fixture search found no block-target winner")
    candidate["block_hex"] = header + candidate["block_hex"][160:]
    proof["header"] = header
    policy = proofs["policy_id"](context)
    digest = jobs["admission_digest"](job, proof, policy)
    r, s = auth["sign"](1, digest)
    if not auth["verify"](auth["multiply"](1), digest, r, s):
        raise AssertionError("independent authorization failed verification")
    return {
        "name": "public-test-canonical-winning-block",
        "test_only_private_key": (1).to_bytes(32, "big").hex(),
        "nonce": nonce,
        "attempts_until_block_win": nonce + 1,
        "job": job,
        "context": context,
        "template": fixture["template"],
        "candidate": candidate,
        "block_hex": candidate["block_hex"],
        "proof": proof,
        "authorization": {"version": 2, "public_key": job["public_key"], "signature": auth["der_signature"](r, s).hex()},
        "expected_digest": digest.hex(),
        "expected_verified": {
            "version": 2,
            "verification": "supplied-context",
            "network": context["network"],
            "genesis": context["genesis"],
            "prospect_id": hashlib.sha256(proofs["ID_DOMAIN"] + struct.pack("<I", context["network"]) + bytes.fromhex(context["genesis"]) + bytes.fromhex(work_hash)).hexdigest(),
            "policy_id": policy.hex(),
            "gym_id": job["gym_id"],
            "work_hash": work_hash,
            "parent_hash": context["parent_hash"],
            "parent_height": context["parent_height"],
            "dna": dna["derive_dna"](work_hash),
        },
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    group = parser.add_mutually_exclusive_group()
    group.add_argument("--check", action="store_true")
    group.add_argument("--generate", action="store_true")
    args = parser.parse_args()
    record = golden_record()
    if args.generate:
        print(json.dumps(record, indent=2))
    elif json.loads(Path(__file__).with_name("mining_winner_v1.json").read_text()) != record:
        raise SystemExit("mining_winner_v1.json differs from independent derivation")
    else:
        print(f"verified independent production D69 winner at nonce {record['nonce']}, {record['attempts_until_block_win']} bounded attempt(s)")


if __name__ == "__main__":
    main()
