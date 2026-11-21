#!/usr/bin/env python3
"""Root-only independent WIRE2 generator; an approved frozen lock is required."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import stat
import sys


ROOT = Path(__file__).resolve().parent
MAX_OUTPUT = 16 * 1024 * 1024
MAX_INPUT_TOTAL = 32 * 1024 * 1024
SOURCES = ("generate.py", "inherited.py", "payloads.py", "tables.py", "wire.py")


def need(ok, message):
    if not ok:
        raise ValueError(message)


def digest(raw):
    return hashlib.sha256(raw).hexdigest()


def read_file(path, maximum):
    need(path.is_absolute() and path == path.resolve(strict=True), "absolute nonlinked input")
    before = path.lstat()
    need(stat.S_ISREG(before.st_mode) and before.st_nlink == 1 and
         0 < before.st_size <= maximum, "bounded regular input")
    fields = lambda s: (s.st_dev, s.st_ino, s.st_mode, s.st_nlink, s.st_uid, s.st_gid,
                        s.st_size, s.st_mtime_ns, s.st_ctime_ns)
    with os.fdopen(os.open(path, os.O_RDONLY | os.O_NOFOLLOW), "rb") as handle:
        need(fields(os.fstat(handle.fileno())) == fields(before), "original opened input")
        raw = handle.read(maximum + 1)
        need(fields(os.fstat(handle.fileno())) == fields(before), "unchanged opened input")
    need(fields(path.lstat()) == fields(before) and len(raw) == before.st_size,
         "unchanged named input")
    return raw


def verify(lock):
    need(lock.get("version") == 1 and lock.get("execution_authorized") is True and
         lock.get("approved_by") == "root", "root-approved execution required")
    need(lock.get("source_root") == str(ROOT) and lock.get("output_limit_bytes") == MAX_OUTPUT,
         "original source/output bounds")
    entries = lock.get("files")
    need(type(entries) is list and 1 <= len(entries) <= 64, "bounded complete inventory")
    found, total = {}, 0
    for entry in entries:
        need(list(entry) == ["path", "bytes", "sha256"], "ordered lock row")
        path = Path(entry["path"])
        need(path.is_absolute() and path.is_relative_to(ROOT) and path.name not in
             ("producer-lock.proposed.json", "source-ready.json"), "bound source path")
        relative = str(path.relative_to(ROOT))
        need(relative not in found, "duplicate lock path")
        raw = read_file(path, MAX_INPUT_TOTAL)
        need(type(entry["bytes"]) is int and len(raw) == entry["bytes"] and
             digest(raw) == entry["sha256"], "exact input identity: " + relative)
        found[relative] = raw
        total += len(raw)
    need(total <= MAX_INPUT_TOTAL and all(name in found for name in SOURCES), "complete bounded source set")
    catalog = json.loads(found["input-catalog.json"])
    for row in catalog["inputs"]:
        rel = "inputs/" + row["name"]
        need(rel in found and len(found[rel]) == row["bytes"] and
             digest(found[rel]) == row["sha256"], "catalog exact public input: " + rel)
    expected = set(SOURCES) | {"input-catalog.json", "README.md", "REFERENCE_PLAN.md"} | {
        "inputs/" + row["name"] for row in catalog["inputs"]}
    need(set(found) == expected, "exact selected source inventory")
    return found


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--lock", required=True)
    parser.add_argument("--output", required=True)
    args = parser.parse_args()
    lock_path, output = Path(args.lock), Path(args.output)
    raw_lock = read_file(lock_path, 65536)
    lock = json.loads(raw_lock)
    before = verify(lock)
    # Imports are only these frozen sibling pure-Python leaves plus stdlib.
    # This path must never point into product code or a generated output tree.
    sys.dont_write_bytecode = True
    from wire import produce
    inputs = {name: json.loads(before["inputs/" + name]) for name in
              ("activity-wire-v1.json", "payout-wire-v1.json", "daily-wire-v1.json")}
    result = produce(inputs)
    result["input_catalog_sha256"] = digest(before["input-catalog.json"])
    result["producer_files"] = [{"path": name, "bytes": len(before[name]), "sha256": digest(before[name])}
                                for name in SOURCES]
    raw = (json.dumps(result, ensure_ascii=True, indent=2, allow_nan=False) + "\n").encode("ascii")
    need(len(raw) <= MAX_OUTPUT, "bounded output")
    need(verify(lock) == before and read_file(lock_path, 65536) == raw_lock,
         "inputs and approved lock unchanged before output")
    need(output.is_absolute() and output.parent == output.parent.resolve(strict=True) and
         not output.is_relative_to(ROOT), "fresh external output")
    with os.fdopen(os.open(output, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600), "wb") as handle:
        need(handle.write(raw) == len(raw), "complete output write")
        handle.flush()
        os.fsync(handle.fileno())
    need(verify(lock) == before and read_file(lock_path, 65536) == raw_lock,
         "inputs and lock unchanged after output")
    print(json.dumps({"version": 1, "bytes": len(raw), "sha256": digest(raw),
                      "counts": result["counts"], "product_calls": 0}, separators=(",", ":")))


if __name__ == "__main__":
    main()
