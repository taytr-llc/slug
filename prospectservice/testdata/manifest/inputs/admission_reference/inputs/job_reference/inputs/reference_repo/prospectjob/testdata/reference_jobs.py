#!/usr/bin/env python3
"""Independent D68 mining-job, proof-v2 and authorization-v2 fixtures.

All inputs are public TEST-ONLY values: private scalars 1 and 2, sequential
challenge bytes, fixed asserted timestamps and an all-ones prospect target.
They are never real issuer records, wallets, pool shares or target policy.

Only independent Python references from prior sets are reused for secp256k1,
RFC6979, transaction serialization and DNA. Production Argon2id comes from
the standalone oracle importing only x/crypto/argon2. No project Go output
or encoder is used to generate expected values; no private files are read.

Run --check (the default) to regenerate/check jobs_v1.json, or --generate to
print the reproducible array. Dependency downloads and network access remain
disabled by the existing standalone oracle wrapper.
"""

import argparse
import hashlib
import json
from pathlib import Path
import runpy
import struct


JOB_DOMAIN = b"SLUG mining job\x00v1\x00"
COMMITMENT_DOMAIN = b"SLUG prospect job gym\x00v2\x00"
AUTH_DOMAIN = b"SLUG prospect admission\x00v2\x00"
LIFETIME_MS = 300000


def sha256(value):
    return hashlib.sha256(value).digest()


def le32(value):
    return struct.pack("<I", value)


def le64(value):
    return struct.pack("<Q", value)


def job_preimage(job):
    context = job["context"]
    return (
        JOB_DOMAIN
        + le32(job["version"])
        + le32(job["rule_version"])
        + le32(context["version"])
        + le32(context["network"])
        + bytes.fromhex(context["genesis"])
        + le32(context["dna_version"])
        + bytes.fromhex(context["prospect_target"])
        + bytes.fromhex(context["parent_hash"])
        + le32(context["parent_height"])
        + le32(context["expected_bits"])
        + le32(context["min_timestamp"])
        + le32(context["max_timestamp"])
        + bytes.fromhex(job["public_key"])
        + bytes.fromhex(job["gym_id"])
        + bytes.fromhex(job["challenge"])
        + le64(job["issued_at_ms"])
        + le64(job["expires_at_ms"])
    )


def commitment_script(job, policy):
    commitment = sha256(COMMITMENT_DOMAIN + policy + bytes.fromhex(job["gym_id"]) + bytes.fromhex(job["id"]))
    return b"\x6a\x28SLUGPR02" + commitment


def make_proof(job, policy, siblings, proof_reference):
    context = job["context"]
    old_script = proof_reference["commitment_script"](context, job["gym_id"])
    coinbase = proof_reference["coinbase_bytes"](context, job["gym_id"])
    if coinbase.count(old_script) != 1:
        raise AssertionError("independent coinbase must contain exactly one original commitment")
    coinbase = coinbase.replace(old_script, commitment_script(job, policy))
    merkle_root = proof_reference["sha256d"](coinbase)
    for sibling in siblings:
        if sibling == merkle_root:
            raise AssertionError("fixture has an equal right sibling")
        merkle_root = proof_reference["sha256d"](merkle_root + sibling)
    header = (
        le32(4)
        + bytes.fromhex(context["parent_hash"])[::-1]
        + merkle_root
        + le32(job["issued_at_ms"] // 1000)
        + le32(context["expected_bits"])
        + le32(0)
    )
    if len(header) != 80:
        raise AssertionError("incorrect independently serialized header size")
    return {
        "version": 2,
        "gym_id": job["gym_id"],
        "job_id": job["id"],
        "header": header.hex(),
        "coinbase": coinbase.hex(),
        "merkle_branch": [sibling[::-1].hex() for sibling in siblings],
    }


def admission_digest(job, proof, policy):
    coinbase = bytes.fromhex(proof["coinbase"])
    return sha256(
        AUTH_DOMAIN
        + policy
        + bytes.fromhex(job["gym_id"])
        + bytes.fromhex(job["public_key"])
        + le32(proof["version"])
        + bytes.fromhex(proof["job_id"])
        + bytes.fromhex(proof["header"])
        + le32(len(coinbase))
        + coinbase
        + le32(len(proof["merkle_branch"]))
        + b"".join(bytes.fromhex(sibling) for sibling in proof["merkle_branch"])
    )


def golden_records():
    repo = Path(__file__).resolve().parents[2]
    proofs_reference = runpy.run_path(str(repo / "prospect/testdata/reference_proofs.py"))
    auth_reference = runpy.run_path(str(repo / "gymauth/testdata/reference_authorizations.py"))
    dna_reference = runpy.run_path(str(repo / "fighter/testdata/reference_dna.py"))
    auth_reference["self_check"]()
    dna_reference["self_check"]()
    context = dict(proofs_reference["CONTEXT"])
    policy = proofs_reference["policy_id"](context)
    jobs, proofs = [], []
    for index, scalar in enumerate((1, 2)):
        public_key = auth_reference["public_key"](scalar)
        issued = 1794787200000 + index * 60000
        job = {
            "version": 1,
            "rule_version": 1,
            "id": "",
            "context": dict(context),
            "public_key": public_key.hex(),
            "gym_id": auth_reference["gym_id"](context, public_key),
            "challenge": bytes(range(index * 32, (index + 1) * 32)).hex(),
            "issued_at_ms": issued,
            "expires_at_ms": issued + LIFETIME_MS,
        }
        job["id"] = sha256(job_preimage(job)).hex()
        siblings = [] if index == 0 else [
            proofs_reference["sha256d"](b"SLUG job proof synthetic sibling 1"),
            proofs_reference["sha256d"](b"SLUG job proof synthetic sibling subtree 2"),
        ]
        jobs.append(job)
        proofs.append(make_proof(job, policy, siblings, proofs_reference))
    hashes = proofs_reference["direct_argon_hashes"]([proof["header"] for proof in proofs])
    records = []
    for index, (job, proof, work_hash) in enumerate(zip(jobs, proofs, hashes)):
        scalar = index + 1
        digest = admission_digest(job, proof, policy)
        r, s = auth_reference["sign"](scalar, digest)
        if not auth_reference["verify"](auth_reference["multiply"](scalar), digest, r, s):
            raise AssertionError("independently generated job authorization is invalid")
        if not 1 < int(work_hash, 16) <= int(context["prospect_target"], 16):
            raise AssertionError("fixture must pass prospect target and miss block target one")
        records.append({
            "name": f"public-test-job-{index + 1}-" + ("coinbase-only" if index == 0 else "two-level"),
            "test_only_private_key": scalar.to_bytes(32, "big").hex(),
            "job": job,
            "context": dict(context),
            "proof": proof,
            "authorization": {
                "version": 2,
                "public_key": job["public_key"],
                "signature": auth_reference["der_signature"](r, s).hex(),
            },
            "expected_digest": digest.hex(),
            "job_id_preimage": job_preimage(job).hex(),
            "commitment_script": commitment_script(job, policy).hex(),
            "expected_verified": {
                "version": 2,
                "verification": "supplied-context",
                "network": context["network"],
                "genesis": context["genesis"],
                "prospect_id": sha256(proofs_reference["ID_DOMAIN"] + le32(context["network"]) + bytes.fromhex(context["genesis"]) + bytes.fromhex(work_hash)).hex(),
                "policy_id": policy.hex(),
                "gym_id": job["gym_id"],
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
    actions.add_argument("--check", action="store_true", help="regenerate and check adjacent fixtures (default)")
    actions.add_argument("--generate", action="store_true", help="print the independent fixture array")
    args = parser.parse_args()
    records = golden_records()
    if args.generate:
        print(json.dumps(records, indent=2))
        return
    with Path(__file__).with_name("jobs_v1.json").open(encoding="utf-8") as handle:
        if json.load(handle) != records:
            raise SystemExit("jobs_v1.json differs from independently regenerated job/proof/auth records")
    print(f"verified {len(records)} independent job assertions with v2 signatures and production Argon2id")


if __name__ == "__main__":
    main()
