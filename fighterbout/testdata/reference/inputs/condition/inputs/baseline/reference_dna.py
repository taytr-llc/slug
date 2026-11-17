#!/usr/bin/env python3
"""Independent SLUG fighter DNA v1 reference, using only Python's stdlib.

This implements the English v1 contract without importing or invoking the Go
generator. In particular, bounded attribute compositions are counted with
inclusion-exclusion and math.comb, rather than dynamic programming.

Run with no arguments (or --check) to validate dna_v1.json, --generate to print
the complete reproducible fixture array, or --seed HEX to print one DNA record.
This derives DNA only; it does not validate a block, chain membership, or PoW.
"""

import argparse
import hashlib
import itertools
import json
import math
from pathlib import Path
import re


DOMAIN = b"SLUG fighter DNA\x00v1\x00"
UINT64_RANGE = 1 << 64
ATTRIBUTE_NAMES = ("power", "speed", "chin", "stamina", "skill")
TIERS = (
    (55, "club", 250, 299),
    (82, "professional", 300, 349),
    (94, "contender", 350, 399),
    (99, "elite", 400, 449),
    (100, "generational", 450, 489),
)
FIXED_SEEDS = (
    "00" * 32,
    "ff" * 32,
    bytes(range(32)).hex(),
    "7893676099cf9985bb198dfb6a347d9422a19f2422cf3326a999a7230ce246cd",
    "729d92f2de29fa6415f4920df49164a8a9824971ff972d42037ee7c9d41f470f",
    "1f08a4ac11d4c60ebecbb8dc85401fa2756576c3d1920c45cad5f7113b587ee0",
)


def parse_seed(seed_hex):
    """Decode exactly 64 hex characters in RPC display order, without reversal."""
    if not isinstance(seed_hex, str) or re.fullmatch(r"[0-9a-fA-F]{64}", seed_hex) is None:
        raise ValueError("seed must contain exactly 64 hexadecimal characters")
    return bytes.fromhex(seed_hex)


class Stream:
    """A label-separated SHA-256 stream, one independent counter per label."""

    def __init__(self, seed, label):
        if len(seed) != 32:
            raise ValueError("seed must be 32 bytes")
        self.prefix = DOMAIN + label.encode("ascii") + b"\x00" + seed
        self.counter = 0

    def next_uint64(self):
        if self.counter >= UINT64_RANGE:
            raise OverflowError("stream counter exhausted")
        digest = hashlib.sha256(
            self.prefix + self.counter.to_bytes(8, "little")
        ).digest()
        self.counter += 1
        return int.from_bytes(digest[:8], "big")

    def uniform_below(self, n):
        if not 1 <= n <= UINT64_RANGE:
            raise ValueError("sample bound must be in [1, 2**64]")
        threshold = UINT64_RANGE % n
        while True:
            value = self.next_uint64()
            if value >= threshold:
                return value % n


def count_tuples(slots, total):
    """Count ordered tuples in [1,100] with the given length and total.

    Subtract one from each coordinate. Stars-and-bars counts nonnegative
    solutions, and inclusion-exclusion removes coordinates of 100 or greater:
      sum_j (-1)^j C(slots,j) C(total - 100*j - 1, slots - 1).
    """
    if slots == 0:
        return int(total == 0)
    if slots < 0 or total < slots or total > 100 * slots:
        return 0
    count = 0
    for excluded in range(slots + 1):
        top = total - 100 * excluded - 1
        if top >= slots - 1:
            count += (-1) ** excluded * math.comb(slots, excluded) * math.comb(top, slots - 1)
    return count


def unrank_attributes(total, rank):
    """Unrank one ordered five-tuple in ascending lexicographic order."""
    if not 0 <= rank < count_tuples(5, total):
        raise ValueError("attribute rank outside composition count")
    attributes = []
    remaining = total
    for position in range(5):
        suffix_slots = 4 - position
        for value in range(1, 101):
            suffixes = count_tuples(suffix_slots, remaining - value)
            if rank < suffixes:
                attributes.append(value)
                remaining -= value
                break
            rank -= suffixes
        else:
            raise AssertionError("unranking exhausted feasible attributes")
    if remaining != 0 or rank != 0:
        raise AssertionError("unranking left an unresolved total or rank")
    return attributes


def derive_dna(seed_hex):
    seed = parse_seed(seed_hex)
    ticket = Stream(seed, "rarity").uniform_below(100)
    for cutoff, rarity, low, high in TIERS:
        if ticket < cutoff:
            break
    budget = low + Stream(seed, "budget").uniform_below(high - low + 1)
    rank = Stream(seed, "attributes").uniform_below(count_tuples(5, budget))
    return {
        "dna_version": 1,
        "seed": seed.hex(),
        "rarity": rarity,
        "talent_budget": budget,
        "attributes": dict(zip(ATTRIBUTE_NAMES, unrank_attributes(budget, rank))),
        "prime_weeks": 26 + Stream(seed, "prime").uniform_below(27),
        "decline_weeks": 13 + Stream(seed, "decline").uniform_below(27),
    }


def golden_records():
    """Fixed public examples, then the first integer seeds covering missing tiers.

    Additional seeds are positive integers encoded as 32-byte big-endian seed
    bytes. Their selection depends solely on this reference, not Go output.
    """
    records = [derive_dna(seed) for seed in FIXED_SEEDS]
    missing = {tier[1] for tier in TIERS} - {record["rarity"] for record in records}
    candidate = 1
    while missing:
        seed = candidate.to_bytes(32, "big").hex()
        candidate += 1
        record = derive_dna(seed)
        if record["rarity"] in missing:
            records.append(record)
            missing.remove(record["rarity"])
    return records


def self_check():
    """Check combinatorics and rejection behavior without consulting Go."""
    if sum(count_tuples(5, total) for total in range(5, 501)) != 100**5:
        raise AssertionError("bounded composition counts do not partition all five-tuples")
    for total in range(0, 203):
        enumerated = sum(1 for a in range(1, 101) if 1 <= total - a <= 100)
        if count_tuples(2, total) != enumerated:
            raise AssertionError("inclusion-exclusion disagrees with enumeration")
    for total in range(5, 10):
        tuples = [values for values in itertools.product(range(1, 6), repeat=5) if sum(values) == total]
        if count_tuples(5, total) != len(tuples):
            raise AssertionError("five-tuple count disagrees with enumeration")
        for rank, values in enumerate(tuples):
            if unrank_attributes(total, rank) != list(values):
                raise AssertionError("unranking disagrees with lexicographic enumeration")

    class ScriptedStream(Stream):
        def __init__(self):
            self.values = iter((0, 5, 6, 19))
            self.attempts = 0

        def next_uint64(self):
            self.attempts += 1
            return next(self.values)

    stream = ScriptedStream()
    if stream.uniform_below(10) != 6 or stream.attempts != 3:
        raise AssertionError("rejection sampling did not reject values below threshold 6")
    if stream.uniform_below(10) != 9 or stream.attempts != 4:
        raise AssertionError("sampling did not advance after a successful attempt")
    if derive_dna(FIXED_SEEDS[3].upper()) != derive_dna(FIXED_SEEDS[3]):
        raise AssertionError("hex case changed DNA")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    actions = parser.add_mutually_exclusive_group()
    actions.add_argument("--check", action="store_true", help="check the adjacent dna_v1.json (default)")
    actions.add_argument("--generate", action="store_true", help="print the reproducible fixture array")
    actions.add_argument("--seed", help="print the DNA for exactly 64 hex characters")
    args = parser.parse_args()
    self_check()
    if args.seed is not None:
        try:
            result = derive_dna(args.seed)
        except ValueError as err:
            parser.error(str(err))
        print(json.dumps(result, indent=2))
        return
    records = golden_records()
    if args.generate:
        print(json.dumps(records, indent=2))
        return
    fixture = Path(__file__).with_name("dna_v1.json")
    with fixture.open(encoding="utf-8") as handle:
        committed = json.load(handle)
    if records != committed:
        raise SystemExit("dna_v1.json differs from independently regenerated DNA")
    print(f"verified {len(records)} independent DNA v1 records covering all {len(TIERS)} tiers")


if __name__ == "__main__":
    main()
