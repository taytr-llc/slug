#!/usr/bin/env python3
"""Independent PUBLIC-FIXTURE account enrollment v1 byte/signature oracle.

This variable-time Python reference is not production cryptography. It accepts
no private-key input, reads no wallet/key files, imports no SLUG Go package, and
uses no network. Only the published zero256 and incrementing256 recovery
fixtures are derived. Their phrases and expected public identities come from
the independently generated slugwallet/recovery/testdata/recovery_v1.json.

BIP39 PBKDF2, standard BIP32, secp256k1 arithmetic, canonical Base58Check,
RFC6979/SHA256 ECDSA and low-S DER are computed here with Python's standard
library. The canonical enrollment bytes follow the frozen English contract,
not an implementation output. --check regenerates the adjacent JSON exactly;
--generate prints it. Neither operation hashes a block or creates a wallet.
"""

import argparse
import hashlib
import hmac
import json
from pathlib import Path


P = 0xFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFEFFFFFC2F
N = 0xFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFEBAAEDCE6AF48A03BBFD25E8CD0364141
G = (0x79BE667EF9DCBBAC55A06295CE870B07029BFCDB2DCE28D959F2815B16F81798,
     0x483ADA7726A3C4655DA4FBFC0E1108A8FD17B448A68554199C47D08FFB10D4B8)
BASE58 = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
NETWORK = 0x534C5547
GENESIS = "7893676099cf9985bb198dfb6a347d9422a19f2422cf3326a999a7230ce246cd"
PROFILE = "slug-phase-a-fixture-bip39-v1"
GYM_PATH = (1397511495, 1, 0)  # All three children are hardened.
DOMAIN = b"SLUG account enrollment\x00v1\x00"
FIXTURES = (
    {
        "name": "zero256",
        "mnemonic": " ".join(["abandon"] * 23 + ["art"]),
        "root_public_key": "JgduaQiYjRcnjcD9t1MfTP1qo8eWaAWPQtS9nChXwtAeyTsoEGVZ8SjVqa5qxxvvKqX8tqC77wMSjaMHXNY8JSYEASdzX6PT31ktXqKPJvkhyy31",
        "gym_public_key": "03ec7488667fd544b4a1de8fb0ae03730c9a2dc346c652cbaa59a077dbeddca519",
        "gym_id": "a227587d837507dbcb5f9d781967339d816070fb7bd574931c690ba6937b88e5",
    },
    {
        "name": "incrementing256",
        "mnemonic": "abandon amount liar amount expire adjust cage candy arch gather drum bullet absurd math era live bid rhythm alien crouch range attend journey unaware",
        "root_public_key": "JgduaQiYjRcnjcD9syNDf4x3txTZkg8HpvTKZtposTTw7bTLpRzvd7ibV3buRixMEqAwVAyreX36pAw5DxLXwG5gW6EGNWeRcJfJitkx2kCJ54pC",
        "gym_public_key": "033de5e2e3d5ad06f616572fcb3d61f089ce1105ef486debf308027b3b663d6d92",
        "gym_id": "28452fc8a478d28f284dfe671cfc57e7cabeca08ea953aef371f5674d3f4c510",
    },
)


def sha256(data):
    return hashlib.sha256(data).digest()


def le32(value):
    return value.to_bytes(4, "little")


def le64(value):
    return value.to_bytes(8, "little")


def framed_ascii(value):
    raw = value.encode("ascii")
    return le32(len(raw)) + raw


def add(a, b):
    if a is None:
        return b
    if b is None:
        return a
    ax, ay = a
    bx, by = b
    if ax == bx and (ay + by) % P == 0:
        return None
    if a == b:
        slope = 3 * ax * ax * pow(2 * ay, -1, P) % P
    else:
        slope = (by - ay) * pow(bx - ax, -1, P) % P
    x = (slope * slope - ax - bx) % P
    return x, (slope * (ax - x) - ay) % P


def multiply(scalar, point=G):
    if scalar < 0:
        raise ValueError("negative scalar")
    result = None
    while scalar:
        if scalar & 1:
            result = add(result, point)
        point = add(point, point)
        scalar >>= 1
    return result


def public_key(scalar):
    if not 0 < scalar < N:
        raise ValueError("invalid public fixture scalar")
    x, y = multiply(scalar)
    return bytes([2 | (y & 1)]) + x.to_bytes(32, "big")


def base58check(payload):
    raw = payload + sha256(sha256(payload))[:4]
    value, text = int.from_bytes(raw, "big"), ""
    while value:
        value, remainder = divmod(value, 58)
        text = BASE58[remainder] + text
    return "1" * (len(raw) - len(raw.lstrip(b"\x00"))) + text


def fixture_key(fixture):
    # Every phrase is canonical English ASCII; the BIP39 extra passphrase is
    # empty, so NFKD is byte-identical and the salt is exactly "mnemonic".
    seed = hashlib.pbkdf2_hmac("sha512", fixture["mnemonic"].encode("ascii"),
                               b"mnemonic", 2048, 64)
    master = hmac.digest(b"Bitcoin seed", seed, "sha512")
    scalar, chain = int.from_bytes(master[:32], "big"), master[32:]
    if not 0 < scalar < N:
        raise AssertionError("invalid public fixture root")
    root_public = base58check(NETWORK.to_bytes(4, "big") + bytes(9) + chain + public_key(scalar))
    for child in GYM_PATH:
        # Standard BIP32 retains the full 32-byte private scalar, including
        # any leading zero, for hardened child derivation.
        data = b"\x00" + scalar.to_bytes(32, "big") + (child | 0x80000000).to_bytes(4, "big")
        child_hash = hmac.digest(chain, data, "sha512")
        tweak = int.from_bytes(child_hash[:32], "big")
        if tweak >= N or (scalar + tweak) % N == 0:
            raise AssertionError("fixture hits an invalid BIP32 child")
        scalar, chain = (scalar + tweak) % N, child_hash[32:]
    pub = public_key(scalar).hex()
    gym = sha256(b"SLUG gym identity\x00v1\x00" + le32(NETWORK) + bytes.fromhex(GENESIS) + bytes.fromhex(pub)).hex()
    actual = (root_public, pub, gym)
    expected = tuple(fixture[key] for key in ("root_public_key", "gym_public_key", "gym_id"))
    if actual != expected:
        raise AssertionError("independent recovery identity does not match published fixture")
    return scalar


def payload(challenge):
    return (DOMAIN + le32(challenge["version"])
            + b"".join(bytes.fromhex(challenge[key]) for key in (
                "authority_id", "account_id", "principal_id", "challenge_id"))
            + le64(challenge["generation"]) + framed_ascii(challenge["action"])
            + le32(challenge["network"]) + bytes.fromhex(challenge["genesis"])
            + framed_ascii(challenge["profile"]) + framed_ascii(challenge["root_public_key"])
            + bytes.fromhex(challenge["gym_public_key"]) + bytes.fromhex(challenge["gym_id"])
            + le64(challenge["issued_at_ms"]) + le64(challenge["expires_at_ms"]))


def rfc6979_nonces(scalar, digest):
    seed = scalar.to_bytes(32, "big") + (int.from_bytes(digest, "big") % N).to_bytes(32, "big")
    key, value = bytes(32), b"\x01" * 32
    key = hmac.digest(key, value + b"\x00" + seed, "sha256")
    value = hmac.digest(key, value, "sha256")
    key = hmac.digest(key, value + b"\x01" + seed, "sha256")
    value = hmac.digest(key, value, "sha256")
    while True:
        value = hmac.digest(key, value, "sha256")
        candidate = int.from_bytes(value, "big")
        if 0 < candidate < N:
            yield candidate
        key = hmac.digest(key, value + b"\x00", "sha256")
        value = hmac.digest(key, value, "sha256")


def sign(scalar, digest):
    z = int.from_bytes(digest, "big")
    for nonce in rfc6979_nonces(scalar, digest):
        r = multiply(nonce)[0] % N
        if not r:
            continue
        s = pow(nonce, -1, N) * (z + r * scalar) % N
        if s:
            return r, min(s, N - s)
    raise AssertionError("unreachable nonce exhaustion")


def verify(point, digest, r, s):
    if not 0 < r < N or not 0 < s < N:
        return False
    inverse = pow(s, -1, N)
    candidate = add(multiply(int.from_bytes(digest, "big") * inverse % N), multiply(r * inverse % N, point))
    return candidate is not None and candidate[0] % N == r


def der(r, s):
    def integer(value):
        raw = value.to_bytes((value.bit_length() + 7) // 8, "big")
        if raw[0] & 0x80:
            raw = b"\x00" + raw
        return b"\x02" + bytes([len(raw)]) + raw
    body = integer(r) + integer(s)
    return b"\x30" + bytes([len(body)]) + body


def self_check():
    if multiply(N) is not None or multiply(N + 1) != G:
        raise AssertionError("group order check failed")
    if public_key(1).hex() != "0279be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798":
        raise AssertionError("public scalar one mismatch")
    if public_key(2).hex() != "02c6047f9441ed7d6d3045406e95c07cd85c778e4b8cef3ca7abac09b95c709ee5":
        raise AssertionError("public scalar two mismatch")
    # Field-wise checks pin framing independently of the complete fixtures.
    if le32(0x01020304).hex() != "04030201" or le64(0x0102030405060708).hex() != "0807060504030201":
        raise AssertionError("little-endian encoding mismatch")
    if framed_ascii("enroll").hex() != "06000000656e726f6c6c":
        raise AssertionError("ASCII length framing mismatch")


def generate():
    self_check()
    records = []
    for index, fixture in enumerate(FIXTURES):
        scalar = fixture_key(fixture)
        for action_index, action in enumerate(("enroll", "recover")):
            label = fixture["name"] + "/" + action
            challenge = {
                "version": 1,
                "authority_id": sha256(b"SLUG public fixture account authority").hex(),
                "account_id": sha256(("SLUG public fixture account/" + fixture["name"]).encode()).hex(),
                "principal_id": sha256(("SLUG public fixture principal/" + fixture["name"]).encode()).hex(),
                "challenge_id": sha256(("SLUG public fixture challenge/" + label).encode()).hex(),
                "generation": 0x0102030405060708 + index * 16 + action_index,
                "action": action,
                "network": NETWORK,
                "genesis": GENESIS,
                "profile": PROFILE,
                "root_public_key": fixture["root_public_key"],
                "gym_public_key": fixture["gym_public_key"],
                "gym_id": fixture["gym_id"],
                "issued_at_ms": 1794873600123 + index * 100000 + action_index * 10000,
                "expires_at_ms": 1794873660123 + index * 100000 + action_index * 10000,
            }
            raw = payload(challenge)
            digest = sha256(raw)
            r, s = sign(scalar, digest)
            point = multiply(scalar)
            if not verify(point, digest, r, s) or verify(point, sha256(digest), r, s):
                raise AssertionError("independent ECDSA verification failed")
            if s > N // 2 or not verify(point, digest, r, N - s):
                raise AssertionError("high-S negative fixture is not a mathematical twin")
            records.append({
                "name": label,
                "fixture_name": fixture["name"],
                "test_only_private_key": scalar.to_bytes(32, "big").hex(),
                "challenge": challenge,
                "authorization": {"version": 1, "public_key": fixture["gym_public_key"], "signature": der(r, s).hex()},
                "expected_payload": raw.hex(),
                "expected_digest": digest.hex(),
                "invalid_high_s_signature": der(r, N - s).hex(),
            })
    return {"fixture_only": True, "cases": records}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    group = parser.add_mutually_exclusive_group()
    group.add_argument("--check", action="store_true")
    group.add_argument("--generate", action="store_true")
    args = parser.parse_args()
    encoded = json.dumps(generate(), indent=2) + "\n"
    if args.generate:
        print(encoded, end="")
    else:
        path = Path(__file__).with_name("enrollment_v1.json")
        if path.read_text() != encoded:
            raise SystemExit("independent enrollment fixtures differ")
        print("Independent recovery identities, four enrollment payloads and ECDSA fixtures passed.")


if __name__ == "__main__":
    main()
