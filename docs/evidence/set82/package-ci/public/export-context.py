#!/usr/bin/env python3
"""Export committed public sources only; never copy either working tree."""
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import subprocess

CHAIN = "138553b1c6c3b5e9b1fd13d17861912f7ce1cd4c"
WALLET = "dfc5d9dd7e0f604486fbac812d0413fce1f45cd6"


def digest(path):
    result = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            result.update(block)
    return result.hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--chain-checkout", required=True, type=Path)
    parser.add_argument("--wallet-checkout", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    package = Path(__file__).resolve().parent
    output = args.output.resolve()
    output.mkdir(mode=0o700, parents=False, exist_ok=False)
    report = {"version": 1, "scope": "public-source-build-context", "sources": [], "package": {}}
    for name in ("Dockerfile", ".dockerignore"):
        shutil.copyfile(package / name, output / name)
        report["package"][name] = digest(output / name)
    for checkout, revision, prefix, name in (
        (args.chain_checkout, CHAIN, "slug/", "chain.tar"),
        (args.wallet_checkout, WALLET, "slugwallet/", "wallet.tar"),
    ):
        archive = output / name
        with archive.open("xb") as stream:
            subprocess.run(
                ["git", "-C", str(checkout.resolve()), "archive", "--format=tar",
                 "--prefix=" + prefix, revision], stdout=stream, check=True,
                timeout=120,
            )
        with archive.open("rb") as stream:
            actual = subprocess.check_output(
                ["git", "get-tar-commit-id"], stdin=stream, text=True, timeout=30,
            ).strip()
        if actual != revision:
            raise RuntimeError("Archive revision mismatch: " + name)
        report["sources"].append({"archive": name, "revision": revision,
                                  "sha256": digest(archive), "bytes": archive.stat().st_size})
    (output / "context.json").write_text(json.dumps(report, indent=2) + "\n")
    print(json.dumps(report))


if __name__ == "__main__":
    main()
