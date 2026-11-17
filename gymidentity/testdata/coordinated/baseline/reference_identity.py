#!/usr/bin/env python3
"""Independent PUBLIC-FIXTURE RS256 identity/reset-token reference.

Only the RSA example private exponent published in RFC 7515 Appendix A.2 is
used. It is public test material, never a provider/operator key. This script
uses Python's standard library, reads no private files, accesses no network,
and neither imports SLUG code nor derives expectations from Go outputs.

Raw RSA exponentiation and explicit EMSA-PKCS1-v1_5/SHA256 encoding independently
produce compact JWS values. Before generation, the exact RFC signature is
reproduced, preserving the published payload's CRLF/space bytes. Variable-time
Python arithmetic is for these public test fixtures only.
"""

import argparse
import base64
import hashlib
import json
from pathlib import Path


# https://www.rfc-editor.org/rfc/rfc7515.html#appendix-A.2.1
# Numbers below are PUBLIC TEST MATERIAL, transcribed from the RFC example.
RSA_N = (
    "ofgWCuLjybRlzo0tZWJjNiuSfb4p4fAkd_wWJcyQoTbji9k0l8W26mPddx"
    "HmfHQp-Vaw-4qPCJrcS2mJPMEzP1Pt0Bm4d4QlL-yRT-SFd2lZS-pCgNMs"
    "D1W_YpRPEwOWvG6b32690r2jZ47soMZo9wGzjb_7OMg0LOL-bSf63kpaSH"
    "SXndS5z5rexMdbBYUsLA9e-KXBdQOS-UTo7WTBEMa2R2CapHg665xsmtdV"
    "MTBQY4uDZlxvb3qCo5ZwKh9kG4LT6_I5IhlJH7aGhyxXFvUK-DWNmoudF8"
    "NAco9_h9iaGNj8q2ethFkMLs91kzk2PAcDTW9gb54h4FRWyuXpoQ"
)
RSA_E = "AQAB"
RSA_D = (
    "Eq5xpGnNCivDflJsRQBXHx1hdR1k6Ulwe2JZD50LpXyWPEAeP88vLNO97I"
    "jlA7_GQ5sLKMgvfTeXZx9SE-7YwVol2NXOoAJe46sui395IW_GO-pWJ1O0"
    "BkTGoVEn2bKVRUCgu-GjBVaYLU6f3l9kJfFNS3E0QbVdxzubSu3Mkqzjkn"
    "439X0M_V51gfpRLI9JYanrC4D4qAdGcopV_0ZHHzQlBjudU2QvXt4ehNYT"
    "CBr6XCLQUShb1juUO1ZdiYoFaFQT5Tw8bGUl_x_jTj3ccPDVZFD9pIuhLh"
    "BOneufuBiB4cS98l2SR_RQyGWSeWjnczT0QU91p1DhOVRuOopznQ"
)
RFC_SIGNATURE = (
    "cC4hiUPoj9Eetdgtv3hF80EGrhuB__dzERat0XF9g2VtQgr9PJbu3XOiZj5RZmh7"
    "AAuHIm4Bh-0Qc_lF5YKt_O8W2Fp5jujGbds9uJdbF9CUAr7t1dnZcAcQjbKBYNX4"
    "BAynRFdiuB--f_nZLgrnbyTyWzO75vRK5h6xBArLIARNPvkSjtQBMHlb1L07Qe7K"
    "0GarZRmB_eSN9383LcOLn6_dO--xi12jzDwusC-eOkHWEsqtFZESc6BfI7noOPqv"
    "hJ1phCnvWh6IeYI2w9QOYEUipUTI8np6LbgGY9Fs98rqVt5AXLIhWkWywlVmtVrB"
    "p0igcN_IoypGlUPQGe77Rw"
)
SHA256_DIGEST_INFO = bytes.fromhex("3031300d060960864801650304020105000420")


def b64(raw):
    return base64.urlsafe_b64encode(raw).decode("ascii").rstrip("=")


def unb64(text):
    return base64.urlsafe_b64decode(text + "=" * (-len(text) % 4))


N = int.from_bytes(unb64(RSA_N), "big")
E = int.from_bytes(unb64(RSA_E), "big")
D = int.from_bytes(unb64(RSA_D), "big")
KEY_BYTES = (N.bit_length() + 7) // 8


def digest_info(message):
    return SHA256_DIGEST_INFO + hashlib.sha256(message).digest()


def encoded_message(message):
    tail = digest_info(message)
    return b"\x00\x01" + b"\xff" * (KEY_BYTES - len(tail) - 3) + b"\x00" + tail


def signature(message):
    encoded = encoded_message(message)
    raw = pow(int.from_bytes(encoded, "big"), D, N).to_bytes(KEY_BYTES, "big")
    assert pow(int.from_bytes(raw, "big"), E, N).to_bytes(KEY_BYTES, "big") == encoded
    return raw


def compact(header, payload):
    """Sign caller-provided bytes, including deliberately invalid JSON shapes."""
    message = (b64(header) + "." + b64(payload)).encode("ascii")
    return message.decode("ascii") + "." + b64(signature(message))


def raw_json(value):
    return json.dumps(value, ensure_ascii=True, separators=(",", ":")).encode("ascii")


def rfc_self_check():
    assert N.bit_length() == 2048 and E == 65537
    header = b'{"alg":"RS256"}'
    payload = b'{"iss":"joe",\r\n "exp":1300819380,\r\n "http://example.com/is_root":true}'
    token = compact(header, payload)
    assert token.split(".")[2] == RFC_SIGNATURE, "does not reproduce RFC 7515 A.2"
    return token


NOW = 1700000000
ISSUER = "https://identity.slug.invalid"
AUDIENCE = "slug-desktop-fixture"
RESET_AUDIENCE = "slug-reset-fixture"
EPOCH_CLAIM = "slug_auth_epoch"
KID = "rfc7515-a2"
NONCE = "0123456789abcdef" * 4
EVENT = "fedcba9876543210" * 4


def fixture_key():
    return {
        "fixture_only": True,
        "source": "https://www.rfc-editor.org/rfc/rfc7515.html#appendix-A.2.1",
        "kty": "RSA", "kid": KID, "n": RSA_N, "e": RSA_E, "d": RSA_D,
    }


def login_claims():
    return {
        "iss": ISSUER, "aud": AUDIENCE, "sub": "public-fixture-a",
        "nonce": NONCE, "iat": NOW - 5, "exp": NOW + 295,
        "auth_time": NOW - 20, EPOCH_CLAIM: 1,
    }


def reset_claims():
    return {
        "iss": ISSUER, "aud": RESET_AUDIENCE, "sub": "public-fixture-a",
        "iat": NOW - 5, "exp": NOW + 295,
        "event_id": EVENT, "stage": "begin", "from_epoch": 1,
        "to_epoch": 2, "verified_at": NOW - 20,
    }


def changed(base, **changes):
    return dict(base, **changes)


def expected(kind, claims):
    base = {"issuer": claims["iss"], "subject": claims["sub"],
            "issued_at_ms": claims["iat"] * 1000,
            "expires_at_ms": claims["exp"] * 1000}
    if kind == "login":
        return dict(base, nonce=claims["nonce"],
                    authenticated_at_ms=claims["auth_time"] * 1000,
                    credential_epoch=claims[EPOCH_CLAIM])
    return dict(base, event_id=claims["event_id"], stage=claims["stage"],
                from_epoch=claims["from_epoch"], to_epoch=claims["to_epoch"],
                verified_at_ms=claims["verified_at"] * 1000)


def corpus():
    rfc_token = rfc_self_check()
    cases = []
    headers = {
        "login": {"alg": "RS256", "kid": KID, "typ": "JWT"},
        "reset": {"alg": "RS256", "kid": KID, "typ": "slug-reset+jwt"},
    }
    bases = {"login": login_claims(), "reset": reset_claims()}

    def add(name, kind="login", claims=None, header=None, payload=None,
            header_raw=None, accept=False, nonce=NONCE, now=NOW, token=None,
            signed=True, reason=""):
        claims = bases[kind] if claims is None else claims
        header = headers[kind] if header is None else header
        payload = raw_json(claims) if payload is None else payload
        header_raw = raw_json(header) if header_raw is None else header_raw
        if token is None:
            token = compact(header_raw, payload)
        item = {"name": name, "kind": kind, "now_unix_seconds": now,
                "expected_nonce": nonce if kind == "login" else "",
                "token": token, "mathematically_signed": signed,
                "accepted": accept}
        if accept:
            item["expected"] = expected(kind, claims)
        else:
            assert reason, "negative vectors must document the rejected contract"
            item["rejection_reason"] = reason
        cases.append(item)
        return token

    login = bases["login"]
    reset = bases["reset"]
    login_token = add("valid-login", accept=True)
    add("valid-login-auth-at-issuance", claims=changed(login, auth_time=NOW-5), accept=True)
    add("valid-login-without-typ", header={"alg": "RS256", "kid": KID}, accept=True)
    add("valid-login-epoch-two", claims=changed(login, slug_auth_epoch=2), accept=True)
    add("valid-login-epoch-max-uint64", claims=changed(login, slug_auth_epoch=2**64-1), accept=True)
    add("valid-login-zero-nonce", claims=changed(login, nonce="0"*64), nonce="0"*64, accept=True)
    add("valid-single-audience-array", claims=changed(login, aud=[AUDIENCE]), accept=True)
    add("valid-multiple-audience-authorized-party",
        claims=changed(login, aud=["other-resource", AUDIENCE], azp=AUDIENCE), accept=True)
    add("valid-optional-not-before", claims=changed(login, nbf=NOW-5), accept=True)
    add("valid-auth-freshness-exact-boundary", claims=changed(login, auth_time=NOW-300), accept=True)
    add("valid-ignored-email-amr-acr", claims=changed(login, email="same@example.invalid",
        email_verified=False, amr=["pwd"], acr="provider-specific", token_use="id"), accept=True)
    add("valid-same-email-different-subject", claims=changed(login,
        sub="public-fixture-b", email="same@example.invalid"), accept=True)
    add("valid-header-whitespace-at-size-limit",
        header_raw=raw_json(headers["login"]).ljust(2048, b" "), accept=True)
    add("valid-payload-whitespace-at-size-limit", payload=raw_json(login).ljust(8192, b" "), accept=True)
    # Unknown ordinary claims are not authorization. Even a field with the
    # output DTO's name cannot replace the trust configuration's own digest.
    add("valid-untrusted-output-name-ignored", claims=changed(login,
        verifier_config_hash="00"*32, credential_epoch=999), accept=True)

    mutations = [
        ("issuer", {"iss": "https://other.identity.invalid"}, "issuer must match exactly"),
        ("issuer-case", {"iss": ISSUER.upper()}, "issuer is not case-normalized"),
        ("issuer-slash", {"iss": ISSUER+"/"}, "issuer is not URL-normalized"),
        ("audience", {"aud": "other-resource"}, "pinned audience absent"),
        ("empty-audience-array", {"aud": []}, "audience cannot be empty"),
        ("duplicate-audience", {"aud": [AUDIENCE, AUDIENCE]}, "audience entries must be unique"),
        ("nonstring-audience-entry", {"aud": [AUDIENCE, 1]}, "audience entries must be strings"),
        ("multiple-audience-missing-azp", {"aud": [AUDIENCE, "other-resource"]}, "multiple audiences require azp"),
        ("wrong-azp", {"azp": "other-resource"}, "any azp must match pinned audience"),
        ("null-azp", {"azp": None}, "present azp must be a string"),
        ("empty-subject", {"sub": ""}, "principal subject cannot be empty"),
        ("wrong-nonce", {"nonce": "10"*32}, "token nonce must match authority attempt"),
        ("uppercase-nonce", {"nonce": NONCE.upper()}, "nonce must be canonical lowerhex"),
        ("short-nonce", {"nonce": NONCE[:-1]}, "nonce must decode to exactly32 bytes"),
        ("expired", {"exp": NOW}, "expiry is exclusive"),
        ("future-issued", {"iat": NOW+1, "exp": NOW+10}, "iat cannot be in the future"),
        ("long-token-lifetime", {"exp": NOW+296}, "token lifetime cannot exceed300 seconds"),
        ("expiry-before-issue", {"exp": NOW-6}, "expiry must follow issuance"),
        ("future-authentication", {"auth_time": NOW+1}, "authentication cannot be in the future"),
        ("auth-after-issuance", {"auth_time": NOW-4}, "authentication cannot follow token issuance"),
        ("refresh-does-not-freshen-auth", {"iat": NOW, "exp": NOW+300, "auth_time": NOW-301}, "fresh iat does not replace fresh auth_time"),
        ("future-not-before", {"nbf": NOW+1}, "nbf cannot be in the future"),
        ("not-before-after-expiry", {"nbf": NOW+296}, "nbf cannot exceed expiry"),
        ("zero-epoch", {EPOCH_CLAIM: 0}, "epochs start at1"),
        ("overflow-epoch", {EPOCH_CLAIM: 2**64}, "epoch must fit uint64"),
        ("negative-epoch", {EPOCH_CLAIM: -1}, "epoch unsigned integer required"),
        ("string-epoch", {EPOCH_CLAIM: "1"}, "epoch is not a string"),
        ("fractional-epoch", {EPOCH_CLAIM: 1.0}, "canonical integer decimal required"),
        ("null-epoch", {EPOCH_CLAIM: None}, "required epoch cannot be null"),
        ("millisecond-overflow", {"exp": (2**63-1)//1000+1}, "seconds must convert to int64 milliseconds"),
        ("security-claim-case-alias", {"ISS": ISSUER}, "registered security field aliases forbidden"),
        ("epoch-case-alias", {EPOCH_CLAIM.upper(): 1}, "configured security field aliases forbidden"),
    ]
    for name, updates, reason in mutations:
        add("reject-login-"+name, claims=dict(login, **updates), reason=reason)
    for field in login:
        c = dict(login)
        del c[field]
        add("reject-login-missing-"+field, claims=c, reason="required login claim missing")
    for field in ("iat", "exp", "auth_time"):
        for label, value in (("null", None), ("zero", 0), ("string", str(NOW)),
                             ("fractional", float(NOW)), ("boolean", True)):
            add("reject-login-"+field+"-"+label, claims=dict(login, **{field: value}),
                reason="security NumericDate must be positive canonical integer seconds")
    add("reject-login-exponent-numericdate", payload=raw_json(login).replace(
        b'"iat":1699999995', b'"iat":1.699999995e9'), reason="numeric exponent is not canonical integer decimal")
    add("reject-login-wrong-expected-nonce", nonce="10"*32, reason="expected nonce comes from authority, not token")
    add("reject-login-malformed-expected-nonce", nonce="not-a-nonce", reason="expected nonce has fixed canonical shape")

    for field, value, name in (
        ("alg", "none", "none-algorithm"), ("alg", "HS256", "hmac-algorithm-confusion"),
        ("alg", "PS256", "rsa-pss-not-rs256"), ("alg", "rs256", "algorithm-case"),
        ("kid", "unknown-key", "unknown-key-id"), ("kid", "", "empty-key-id"),
        ("typ", "slug-reset+jwt", "reset-type-on-login"),
        ("jku", "https://attacker.invalid/jwks", "remote-key-url"),
        ("jwk", {"kty": "RSA", "n": RSA_N, "e": RSA_E}, "embedded-key"),
        ("crit", [], "critical-header"), ("ALG", "RS256", "header-case-alias"),
    ):
        add("reject-header-"+name, header=dict(headers["login"], **{field: value}),
            reason="header must match fixed RS256/kid/typ profile")
    add("reject-header-missing-kid", header={"alg": "RS256", "typ": "JWT"}, reason="kid required")
    add("reject-header-null-typ", header=changed(headers["login"], typ=None), reason="present typ cannot be null")
    for kind in ("login", "reset"):
        h, p = raw_json(headers[kind]), raw_json(bases[kind])
        add("reject-"+kind+"-duplicate-header", kind, header_raw=h[:-1]+b',"alg":"RS256"}',
            reason="duplicate decoded header keys forbidden")
        add("reject-"+kind+"-duplicate-claim", kind, payload=p[:-1]+b',"iss":"'+ISSUER.encode()+b'"}',
            reason="duplicate decoded payload keys forbidden")
        add("reject-"+kind+"-escaped-duplicate-claim", kind,
            payload=p[:-1]+b',"\\u0069ss":"'+ISSUER.encode()+b'"}',
            reason="escaped keys cannot hide duplicates")
        add("reject-"+kind+"-nested-duplicate", kind,
            payload=p[:-1]+b',"metadata":{"x":1,"x":2}}',
            reason="unknown metadata also requires unique JSON object keys")
        add("reject-"+kind+"-deep-metadata", kind,
            payload=p[:-1]+b',"metadata":'+b'['*80+b'0'+b']'*80+b'}',
            reason="recursive JSON depth is bounded")
        add("reject-"+kind+"-invalid-utf8", kind,
            payload=p[:-1]+b',"metadata":"\xff"}', reason="invalid UTF8 must not be repaired")
        add("reject-"+kind+"-trailing-json", kind, payload=p+b'{}', reason="exactly one JSON object required")
        add("reject-"+kind+"-payload-over-limit", kind, payload=p.ljust(8193, b" "),
            reason="decoded payload maximum8192 bytes")
    add("reject-header-over-limit", header_raw=raw_json(headers["login"]).ljust(2049, b" "),
        reason="decoded header maximum2048 bytes")
    add("reject-compact-token-over-limit", token="A"*16385, signed=False,
        reason="compact token maximum16384 bytes")
    parts = login_token.split(".")
    for index, label in ((0, "header"), (1, "payload"), (2, "signature")):
        padded = list(parts)
        padded[index] += "="
        add("reject-padded-base64-"+label, token=".".join(padded), signed=False,
            reason="compact segments must use canonical unpadded base64url")
    malformed = bytearray(unb64(parts[2]))
    malformed[-1] ^= 1
    add("reject-invalid-signature", token=".".join(parts[:2]+[b64(malformed)]), signed=False,
        reason="RSA signature must verify")
    add("reject-short-signature", token=".".join(parts[:2]+[b64(unb64(parts[2])[:-1])]), signed=False,
        reason="RSA signature must match modulus size")
    add("reject-extra-compact-segment", token=login_token+".ignored", signed=False,
        reason="exactly three compact JWS segments required")
    add("reject-signature-over-limit", token=".".join(parts[:2]+[b64(b'\x00'*513)]), signed=False,
        reason="decoded signature maximum512 bytes")

    reset_token = add("valid-reset-begin", "reset", accept=True)
    complete = changed(reset, stage="complete")
    add("valid-reset-complete", "reset", claims=complete, accept=True)
    add("valid-reset-complete-old-verification", "reset", claims=changed(complete,
        verified_at=NOW-86400), accept=True)
    add("valid-reset-zero-event-id", "reset", claims=changed(reset, event_id="0"*64), accept=True)
    add("valid-reset-begin-freshness-exact-boundary", "reset",
        claims=changed(reset, verified_at=NOW-300), accept=True)
    add("valid-reset-begin-old-verification-retry", "reset",
        claims=changed(reset, verified_at=NOW-301), accept=True)
    add("valid-reset-next-epoch-max-uint64", "reset",
        claims=changed(reset, from_epoch=2**64-2, to_epoch=2**64-1), accept=True)
    add("valid-reset-other-subject", "reset", claims=changed(reset, sub="public-fixture-b"), accept=True)
    reset_mutations = [
        ("wrong-issuer", {"iss": "https://other.identity.invalid"}, "issuer pinned"),
        ("login-audience", {"aud": AUDIENCE}, "reset audience distinct from login"),
        ("array-audience", {"aud": [RESET_AUDIENCE]}, "reset audience must be a single string"),
        ("wrong-stage", {"stage": "reset"}, "only begin/complete stages exist"),
        ("uppercase-stage", {"stage": "BEGIN"}, "stage is exact case-sensitive string"),
        ("short-event-id", {"event_id": EVENT[:-1]}, "fixed32-byte event ID required"),
        ("uppercase-event-id", {"event_id": EVENT.upper()}, "event ID canonical lowerhex"),
        ("verification-after-issue", {"verified_at": NOW-4}, "verified_at cannot follow issuance"),
        ("future-verification", {"verified_at": NOW+1}, "verified_at cannot be in future"),
        ("expired", {"exp": NOW}, "expiry exclusive"),
        ("future-issued", {"iat": NOW+1, "exp": NOW+100}, "iat cannot be future"),
        ("long-lifetime", {"exp": NOW+296}, "reset TTL at most300 seconds"),
        ("equal-epochs", {"from_epoch": 2}, "to_epoch must exactly increment from_epoch"),
        ("skipped-epoch", {"to_epoch": 3}, "epochs cannot skip"),
        ("zero-from-epoch", {"from_epoch": 0, "to_epoch": 1}, "epochs start at1"),
        ("overflow-next-epoch", {"from_epoch": 2**64-1, "to_epoch": 2**64}, "checked uint64 epoch advance"),
        ("fractional-from-epoch", {"from_epoch": 1.0}, "canonical integer epoch required"),
        ("null-to-epoch", {"to_epoch": None}, "required epoch cannot be null"),
        ("security-claim-case-alias", {"STAGE": "begin"}, "security field aliases forbidden"),
    ]
    for name, updates, reason in reset_mutations:
        add("reject-reset-"+name, "reset", claims=dict(reset, **updates), reason=reason)
    for field in reset:
        c = dict(reset)
        del c[field]
        add("reject-reset-missing-"+field, "reset", claims=c, reason="required reset claim missing")
    add("reject-reset-missing-type", "reset", header={"alg": "RS256", "kid": KID},
        reason="reset JWS requires explicit slug-reset+jwt typ")
    add("reject-login-as-reset", "reset", token=login_token, reason="login token cannot authorize reset")
    add("reject-reset-as-login", token=reset_token, reason="reset token cannot authorize login")
    # A correctly signed complete token may carry old verified_at; the durable
    # store must still enforce a matching already-verified pending begin. This
    # oracle does not claim signature validity proves event order or completion.
    add("reject-reset-complete-old-issued", "reset", claims=changed(complete,
        iat=NOW-301, exp=NOW-1, verified_at=NOW-600), reason="complete assertion itself must be fresh/unexpired")
    assert len({c["name"] for c in cases}) == len(cases)
    return {
        "fixture_only": True, "version": 1,
        "source": fixture_key()["source"],
        "config": {"version": 1, "issuer": ISSUER, "audience": AUDIENCE,
                   "reset_audience": RESET_AUDIENCE, "epoch_claim": EPOCH_CLAIM,
                   "public_jwks": {"keys": [{"kty": "RSA", "kid": KID,
                       "n": RSA_N, "e": RSA_E, "alg": "RS256", "use": "sig"}]}},
        "rfc7515_example": rfc_token,
        "cases": cases,
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    mode = parser.add_mutually_exclusive_group(required=True)
    mode.add_argument("--check", action="store_true", help="compare regenerated public corpus with committed files")
    mode.add_argument("--generate", action="store_true", help="print public identity corpus JSON")
    args = parser.parse_args()
    result = corpus()
    encoded = json.dumps(result, indent=2, ensure_ascii=True) + "\n"
    if args.generate:
        print(encoded, end="")
        return
    directory = Path(__file__).resolve().parent
    assert (directory / "identity_v1.json").read_text() == encoded, "identity corpus differs"
    key = json.dumps(fixture_key(), indent=2) + "\n"
    assert (directory / "rfc7515_rsa.json").read_text() == key, "public RFC key differs"
    accepted = sum(c["accepted"] for c in result["cases"])
    print(f"RFC signature and {len(result['cases'])} independent cases match ({accepted} accepted).")


if __name__ == "__main__":
    main()
