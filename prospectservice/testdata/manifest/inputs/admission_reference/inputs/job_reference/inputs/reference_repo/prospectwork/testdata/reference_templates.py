#!/usr/bin/env python3
"""Independent unmined, payable prospect-work block candidate fixtures.

Python implements transaction/block encodings, ordinary SHA256d Merkle trees,
SegWit commitment/weight, and SLUG Base58Check directly. Job byte serialization
is reused only from the independent set18 Python reference. No project Go
encoder/output is used, no header PoW is computed, and nothing is submitted.

The public job assertions and outpoints are synthetic TEST-ONLY inputs, not
issued jobs, spendable UTXOs, payable rewards, or actual node-accepted blocks.
The all-ones prospect target is never a production policy recommendation.
Run --check (default), or --generate to print the reproducible JSON fixtures.
"""

import argparse
import hashlib
import json
from pathlib import Path
import runpy
import struct


PAYOUT_HASH160 = bytes.fromhex("00112233445566778899aabbccddeeff00112233")
FLAGS = b"\x06/SLUG/"


def sha256(value):
    return hashlib.sha256(value).digest()


def sha256d(value):
    return sha256(sha256(value))


def le32(value):
    return struct.pack("<I", value)


def le64(value):
    return struct.pack("<Q", value)


def compact_size(value):
    if value < 253:
        return bytes((value,))
    if value <= 0xFFFF:
        return b"\xfd" + struct.pack("<H", value)
    if value <= 0xFFFFFFFF:
        return b"\xfe" + le32(value)
    return b"\xff" + le64(value)


def varbytes(value):
    return compact_size(len(value)) + value


def script_number_push(value):
    if 1 <= value <= 16:
        return bytes((0x50 + value,))
    result = bytearray()
    while value:
        result.append(value & 255)
        value >>= 8
    if result[-1] & 0x80:
        result.append(0)
    return bytes((len(result),)) + result


def base58check(version, payload):
    raw = bytes((version,)) + payload
    raw += sha256d(raw)[:4]
    leading = len(raw) - len(raw.lstrip(b"\x00"))
    value, encoded = int.from_bytes(raw, "big"), ""
    alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
    while value:
        value, remainder = divmod(value, 58)
        encoded = alphabet[remainder] + encoded
    return "1" * leading + encoded


def transaction(inputs, outputs, witnesses=None):
    """Return full and stripped version-2 encodings with zero lock time."""
    body = compact_size(len(inputs))
    for previous, index, script, sequence in inputs:
        body += previous + le32(index) + varbytes(script) + le32(sequence)
    body += compact_size(len(outputs))
    for value, script in outputs:
        body += struct.pack("<q", value) + varbytes(script)
    stripped = le32(2) + body + le32(0)
    if witnesses is None:
        return stripped, stripped
    if len(witnesses) != len(inputs) or not any(witnesses):
        raise AssertionError("witness serialization needs nonempty witness stacks")
    witness_bytes = b"".join(compact_size(len(stack)) + b"".join(varbytes(item) for item in stack) for stack in witnesses)
    return le32(2) + b"\x00\x01" + body + witness_bytes + le32(0), stripped


def merkle(leaves):
    level, branch = list(leaves), []
    while len(level) > 1:
        branch.append(level[1][::-1].hex())
        if len(level) % 2:
            level.append(level[-1])
        level = [sha256d(level[index] + level[index + 1]) for index in range(0, len(level), 2)]
    return level[0], branch


def transaction_fixtures():
    entries = []

    def add(previous, value, fee, depends, witness=None):
        full, stripped = transaction(
            [(previous, 0, b"\x01\x01" if witness is None else b"", 0xFFFFFFFE)],
            [(value, b"\x51")],
            None if witness is None else [witness],
        )
        entries.append({
            "data": full.hex(),
            "txid": sha256d(stripped)[::-1].hex(),
            "hash": sha256d(full)[::-1].hex(),
            "depends": depends,
            "fee": fee,
            "sigops": 0,
            "weight": 3 * len(stripped) + len(full),
        })
        return sha256d(stripped)

    first = add(sha256d(b"SLUG unspent synthetic template input one"), 1000000, 2000, [])
    add(first, 999000, 1000, [1])
    third = add(sha256d(b"SLUG unspent synthetic template input three"), 900000, 3000, [], [b"\x01", b"\x02"])
    add(third, 896000, 4000, [3], [b"", b"public fixture witness"])
    return entries


def golden_records():
    repo = Path(__file__).resolve().parents[2]
    job_reference = runpy.run_path(str(repo / "prospectjob/testdata/reference_jobs.py"))
    public_jobs = json.loads((repo / "prospectjob/testdata/jobs_v1.json").read_text())
    all_transactions = transaction_fixtures()
    payout_address = base58check(0x53, PAYOUT_HASH160)
    payout_script = b"\x76\xa9\x14" + PAYOUT_HASH160 + b"\x88\xac"
    records = []
    for name, count, extra_nonce in (
        ("coinbase-only", 0, 0),
        ("legacy-odd-leaf-dependent", 2, 0x0102030405060708),
        ("witness-odd-two-level-dependent", 4, 0xFFFFFFFFFFFFFFFF),
    ):
        # Copy public independent job bytes, change bits to SLUG's easy normal
        # profile bits, and rederive the ID independently. This remains a
        # synthetic assertion: no issuer or node issued it.
        job = json.loads(json.dumps(public_jobs[0]["job"]))
        context = job["context"]
        context["expected_bits"] = 0x207FFFFF
        job["id"] = sha256(job_reference["job_preimage"](job)).hex()
        policy = sha256(b"SLUG prospect policy\x00v1\x00" + le32(context["network"]) + bytes.fromhex(context["genesis"]) + le32(context["dna_version"]) + bytes.fromhex(context["prospect_target"]))
        job_script = job_reference["commitment_script"](job, policy)
        transactions = all_transactions[:count]
        coinbase_value = 1000000000 + sum(tx["fee"] for tx in transactions)
        witness_commitment = None
        if count == 4:
            witness_root, _ = merkle([bytes(32)] + [bytes.fromhex(tx["hash"])[::-1] for tx in transactions])
            witness_commitment = sha256d(witness_root + bytes(32))
        script_sig = script_number_push(context["parent_height"] + 1) + b"\x08" + le64(extra_nonce) + FLAGS
        outputs = [(coinbase_value, payout_script), (0, job_script)]
        if witness_commitment is not None:
            outputs.append((0, b"\x6a\x24\xaa\x21\xa9\xed" + witness_commitment))
        coinbase_full, coinbase_stripped = transaction(
            [(bytes(32), 0xFFFFFFFF, script_sig, 0xFFFFFFFF)],
            outputs,
            [[bytes(32)]] if witness_commitment is not None else None,
        )
        root, branch = merkle([sha256d(coinbase_stripped)] + [bytes.fromhex(tx["txid"])[::-1] for tx in transactions])
        timestamp = job["issued_at_ms"] // 1000
        header = le32(0x20000000) + bytes.fromhex(context["parent_hash"])[::-1] + root + le32(timestamp) + le32(context["expected_bits"]) + le32(0)
        full_block = header + compact_size(1 + count) + coinbase_full + b"".join(bytes.fromhex(tx["data"]) for tx in transactions)
        base_size = len(header) + len(compact_size(1 + count)) + len(coinbase_stripped) + sum((tx["weight"] - len(bytes.fromhex(tx["data"]))) // 3 for tx in transactions)
        block = {
            "bits": "207fffff",
            "curtime": timestamp,
            "height": context["parent_height"] + 1,
            "previousblockhash": context["parent_hash"],
            "sigoplimit": 80000,
            "sizelimit": 4000000,
            "weightlimit": 4000000,
            "transactions": transactions,
            "version": 0x20000000,
            "coinbaseaux": {"flags": FLAGS.hex()},
            "coinbasevalue": coinbase_value,
            "target": f"{0x7FFFFF << (8 * 29):064x}",
            "mintime": context["min_timestamp"],
            "maxtime": context["max_timestamp"],
            "mutable": ["time", "transactions", "prevblock"],
            "noncerange": "00000000ffffffff",
            "capabilities": ["proposal"],
        }
        if witness_commitment is not None:
            block["default_witness_commitment"] = witness_commitment.hex()
        records.append({
            "name": name,
            "job": job,
            "template": {"context": dict(context), "block": block},
            "payout_address": payout_address,
            "extra_nonce": extra_nonce,
            "expected": {
                "version": 1,
                "job_id": job["id"],
                "block_hex": full_block.hex(),
                "proof": {
                    "version": 2,
                    "gym_id": job["gym_id"],
                    "header": header.hex(),
                    "coinbase": coinbase_stripped.hex(),
                    "merkle_branch": branch,
                    "job_id": job["id"],
                },
                "payout_address": payout_address,
                "coinbase_value": coinbase_value,
                "transaction_count": 1 + count,
                "weight": base_size * 3 + len(full_block),
                "sigop_cost": 4,
            },
            "expected_merkle_root": root[::-1].hex(),
            "expected_coinbase_txid": sha256d(coinbase_stripped)[::-1].hex(),
            "expected_serialized_size": len(full_block),
            "expected_base_size": base_size,
            "expected_payout_script": payout_script.hex(),
        })
    return records


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    actions = parser.add_mutually_exclusive_group()
    actions.add_argument("--check", action="store_true")
    actions.add_argument("--generate", action="store_true")
    args = parser.parse_args()
    records = golden_records()
    if args.generate:
        print(json.dumps(records, indent=2))
        return
    path = Path(__file__).with_name("templates_v1.json")
    if json.loads(path.read_text()) != records:
        raise SystemExit("templates_v1.json differs from independent byte derivation")
    print(f"verified {len(records)} independent unmined full-block candidates, including witness and odd Merkle levels")


if __name__ == "__main__":
    main()
