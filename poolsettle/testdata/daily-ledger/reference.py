#!/usr/bin/env python3
"""Independent canonical daily-ledger fixtures and finite byte proof.

No product import, key access, network, filesystem journal, or implementation
output is used. Published signed transactions remain unchanged. Conditional
claim inputs are not mined/source-authority evidence. All integers are exact.
"""
import copy
import hashlib
import json
import pathlib
import struct
import sys

BASE = pathlib.Path(__file__).resolve().parent
MAX_MONEY = 21000000 * 100000000
MAX_TIME = 253402300799999
ZERO = "0" * 64
UNKNOWN = "f" * 64


def canon(value):
    return json.dumps(value, separators=(",", ":"), ensure_ascii=True).encode("ascii")


def digest(data):
    return hashlib.sha256(data).hexdigest()


def config_id(config):
    raw = canon(config)
    return digest(b"SLUG operator pool settlement config\x00v2\x00" + struct.pack("<I", len(raw)) + raw)


def source_id(source):
    return digest(b"SLUG pool share daily source\x00v2\x00" + canon(source))


def claims_id(claims):
    return digest(b"SLUG pool daily change claims\x00v2\x00" + canon(claims))


def number_key(prefix, number):
    return prefix.encode("ascii") + struct.pack(">Q", number)


def outpoint_key(prefix, txid, index):
    return prefix.encode("ascii") + txid.encode("ascii") + struct.pack(">I", index)


def frac(number):
    return {"numerator": str(number), "denominator": "1"}


def claim(root, height, gym, amount):
    return {"round_hash": root, "round_height": height, "gym_id": gym, "amount": frac(amount)}


def ref(kind, sequence):
    return {"kind": kind, "sequence": sequence}


def put(rows, key, value):
    assert key not in rows
    rows[key] = value if isinstance(value, bytes) else canon(value)


def logical(rows, bounds):
    result = bounds["head"] + bounds["report"]
    for key, value in rows.items():
        if key in (b"head", b"report"):
            continue
        result += len(key) + (bounds["state_ref"] if key.startswith(b"bs/") else len(value))
    return result


def marker(config, source):
    return {"application": "slug-pool-settlement-daily", "version": 2,
            "config": config, "source": source, "config_id": config_id(config), "source_id": source_id(source)}


def header(config, source, revision, credits, batches, pending, day, stamp):
    return {"application": "slug-pool-settlement-daily", "version": 2,
            "config_id": config_id(config), "source_id": source_id(source),
            "revision": revision, "credits": credits, "batches": batches,
            "events": credits + batches, "logical_bytes": 0, "pending_batch": pending,
            "last_day": day, "last_clock_ms": stamp}


def build_case(guard, transaction, state, recipe):
    config, source = copy.deepcopy(guard["settlement"]), copy.deepcopy(guard["source"])
    config.update(recipe["settlement"])
    source.update(recipe["source"])
    rows, credits, inputs, remaining, payments = {}, [], [], [], []
    size = transaction["shape"]
    assert len(transaction["coins"]) == size["inputs"]
    fee = transaction["fee_atoms"]
    assert 0 <= fee < 10000000
    for i, original in enumerate(transaction["coins"]):
        assert original["atoms"] == 1000000000 and original["index"] == 0
        assert original["script"] == config["change_script"]
        root, height = format(i + 1, "064x"), i + 1
        txid = bytes.fromhex(original["hash_wire"])[::-1].hex()
        claims = [claim(root, height, "", 10000000)]
        carry = 990000000
        if i < size["payees"]:
            dest = config["destinations"][i]
            claims.append(claim(root, height, dest["gym_id"], 100000000))
            payments.append({"gym_id": dest["gym_id"], "script": dest["script"], "atoms": 100000000})
            carry -= 100000000
        claims.append(claim(root, height, UNKNOWN, carry))
        coin = {"txid": txid, "index": 0, "atoms": 1000000000, "script": original["script"],
                "coinbase": True, "block_hash": root, "block_height": height, "claims": claims}
        credit = {"round_hash": root, "round_digest": "01" * 32, "coin": coin}
        credits.append(credit)
        inputs.append({"txid": txid, "index": 0})
        remaining.extend([claim(root, height, "", 10000000 - (fee if i == 0 else 0)), claims[-1]])
        put(rows, number_key("c/", i + 1), credit)
        put(rows, b"cr/" + root.encode(), struct.pack(">Q", i + 1))
        put(rows, outpoint_key("o/", txid, 0), ref("credit", i + 1))
        put(rows, number_key("e/", i + 1), ref("credit", i + 1))
        put(rows, outpoint_key("spent/", txid, 0), struct.pack(">Q", 1))
    change = {"index": len(payments), "atoms": len(inputs) * 1000000000 - len(payments) * 100000000 - fee,
              "script": config["change_script"], "claim_count": len(remaining), "claims_digest": claims_id(remaining)}
    body = {"version": 2, "sequence": 1, "day": recipe["date"], "created_at_ms": recipe["stamp_ms"],
            "inputs": inputs, "payments": payments, "fee_atoms": fee, "change": change,
            "txid": transaction["txid"], "raw": transaction["signed_hex"]}
    put(rows, number_key("b/", 1), body)
    put(rows, number_key("bs/", 1), {"state": state})
    put(rows, b"bt/" + body["txid"].encode(), struct.pack(">Q", 1))
    put(rows, b"bd/" + recipe["date"].encode(), struct.pack(">Q", 1))
    put(rows, outpoint_key("o/", body["txid"], change["index"]), ref("change", 1))
    put(rows, number_key("e/", len(credits) + 1), ref("batch", 1))
    balances = []
    for i, dest in enumerate(config["destinations"]):
        atoms = 100000000 if i < size["payees"] else 0
        paid = atoms if state == "confirmed" else 0
        reserved = 0 if state == "confirmed" else atoms
        balances.append({"gym_id": dest["gym_id"], "confirmed": frac(atoms-paid), "available": frac(0),
                         "reserved_atoms": reserved, "paid_atoms": paid, "reason": "payout-pending" if reserved else ""})
    balances.append({"gym_id": UNKNOWN, "confirmed": frac(len(inputs)*990000000-len(payments)*100000000),
                     "available": frac(0), "reserved_atoms": 0, "paid_atoms": 0, "reason": "destination-unavailable"})
    report = {"day": recipe["date"], "observed_at_ms": recipe["stamp_ms"], "txid": body["txid"],
              "state": "complete" if state == "confirmed" else "pending",
              "reason": "daily-batch-complete" if state == "confirmed" else "payout-pending", "balances": balances}
    put(rows, b"report", report)
    head = header(config, source, 3 if state == "confirmed" else 2, len(credits), 1,
                  0 if state == "confirmed" else 1, recipe["date"], recipe["stamp_ms"])
    head["logical_bytes"] = logical(rows, recipe["bounds"])
    put(rows, b"head", head)
    assert validate_indices(rows, recipe["bounds"])
    assert all(len(canon(c)) <= recipe["bounds"]["credit"] for c in credits)
    assert len(canon(body)) <= recipe["bounds"]["batch"]
    return {"name": transaction["fixture"]+"-"+str(size["inputs"])+"-"+state,
            "marker": marker(config, source), "marker_hex": canon(marker(config, source)).hex(),
            "config_id": config_id(config), "source_id": source_id(source),
            "change_claims": remaining, "change_claims_digest": claims_id(remaining),
            "body": body, "body_hex": canon(body).hex(), "rows": encode_rows(rows)}


def encode_rows(rows):
    return [{"key_hex": k.hex(), "value_hex": v.hex()} for k, v in sorted(rows.items())]


def build_empty(guard, recipe):
    config, source = copy.deepcopy(guard["settlement"]), copy.deepcopy(guard["source"])
    config.update(recipe["settlement"])
    source.update(recipe["source"])
    rows = {b"report": canon({"day": "", "observed_at_ms": 0, "state": "", "balances": None})}
    head = header(config, source, 1, 0, 0, 0, "", 0)
    head["logical_bytes"] = logical(rows, recipe["bounds"])
    put(rows, b"head", head)
    assert validate_indices(rows, recipe["bounds"])
    return {"name": "empty-first-guard", "marker": marker(config, source),
            "marker_hex": canon(marker(config, source)).hex(), "config_id": config_id(config),
            "source_id": source_id(source), "rows": encode_rows(rows)}


def validate_indices(rows, bounds):
    """Closed namespace/inverse-index model only; not proof/signature authority."""
    try:
        head = json.loads(rows[b"head"])
        expected = {b"head": rows[b"head"], b"report": rows[b"report"]}
        for seq in range(1, head["credits"]+1):
            key = number_key("c/", seq)
            credit = json.loads(rows[key])
            c = credit["coin"]
            put(expected, key, rows[key])
            put(expected, b"cr/"+credit["round_hash"].encode(), struct.pack(">Q", seq))
            if c["atoms"]:
                put(expected, outpoint_key("o/", c["txid"], c["index"]), ref("credit", seq))
        for seq in range(1, head["batches"]+1):
            key = number_key("b/", seq)
            body = json.loads(rows[key])
            assert body["sequence"] == seq
            put(expected, key, rows[key])
            put(expected, number_key("bs/", seq), rows[number_key("bs/", seq)])
            put(expected, b"bt/"+body["txid"].encode(), struct.pack(">Q", seq))
            put(expected, b"bd/"+body["day"].encode(), struct.pack(">Q", seq))
            for c in body["inputs"]:
                put(expected, outpoint_key("spent/", c["txid"], c["index"]), struct.pack(">Q", seq))
            if body["change"] is not None:
                put(expected, outpoint_key("o/", body["txid"], body["change"]["index"]), ref("change", seq))
        next_credit, next_batch = 1, 1
        for seq in range(1, head["events"]+1):
            key = number_key("e/", seq)
            event = json.loads(rows[key])
            if event["kind"] == "credit":
                assert event["sequence"] == next_credit
                next_credit += 1
            else:
                assert event["kind"] == "batch" and event["sequence"] == next_batch
                next_batch += 1
            put(expected, key, event)
        assert next_credit == head["credits"]+1 and next_batch == head["batches"]+1
        assert head["events"] == head["credits"]+head["batches"]
        return rows == expected and head["logical_bytes"] == logical(rows, bounds)
    except (KeyError, ValueError, TypeError, AssertionError, OverflowError):
        return False


def byte_proof(bounds):
    # Overestimates need not be valid fractions/transactions. They bound every
    # field in the closed grammar, not merely representative values.
    n, r, b, g = 16384, 16384, 32767, 256
    c = claim(UNKNOWN, 2147483647, UNKNOWN, MAX_MONEY*409600)
    c["amount"]["denominator"] = "409600"
    coin = {"txid": UNKNOWN,"index":4294967295,"atoms":MAX_MONEY,"script":"0014"+"f"*40,
            "coinbase":False,"block_hash":UNKNOWN,"block_height":2147483647,"claims":[]}
    credit_frame = len(canon({"round_hash":UNKNOWN,"round_digest":UNKNOWN,"coin":coin}))
    claim_bytes = len(canon(c))+1
    payment = {"gym_id":UNKNOWN,"script":coin["script"],"atoms":MAX_MONEY}
    input_ref = {"txid":UNKNOWN,"index":4294967295}
    change = {"index":256,"atoms":MAX_MONEY,"script":coin["script"],"claim_count":32768,"claims_digest":UNKNOWN}
    body = {"version":2,"sequence":b,"day":"9999-12-31","created_at_ms":MAX_TIME,"inputs":[],"payments":[],
            "fee_atoms":MAX_MONEY,"change":change,"txid":UNKNOWN,"raw":""}
    input_count = r+b
    payment_count = g*b
    raw_total = 16*b + 150*input_count + 31*(payment_count+b)
    credit_total = r*(len(number_key("c/",r))+credit_frame) + (n+r)*claim_bytes
    batch_total = b*(len(number_key("b/",b))+len(canon(body))) + input_count*(len(canon(input_ref))+1) + payment_count*(len(canon(payment))+1) + 2*raw_total
    index_total = (r*(3+64+8) + b*(3+64+8) + b*(3+10+8) +
                   (r+b)*(2+8+len(canon(ref("credit",r+b)))) +
                   (r+b)*(2+64+4+len(canon(ref("credit",r+b)))) + input_count*(6+64+4+8))
    delivery_total = b*(3+8+bounds["state_ref"])
    total = bounds["head"]+bounds["report"]+credit_total+batch_total+index_total+delivery_total
    # Explicit 1464-input/256-payee body bound, deliberately allowing the
    # larger raw envelope even though that particular shape is overweight.
    one_body = len(canon(body))+1464*(len(canon(input_ref))+1)+256*(len(canon(payment))+1)+2*400000
    one_credit = credit_frame+257*claim_bytes
    wide = claim(UNKNOWN,2147483647,UNKNOWN,0)
    wide["amount"] = {"numerator":"9"*4096,"denominator":"9"*4096}
    active_claim_bytes = len(canon(wide))+1
    request_bound = 65536*active_claim_bytes+(8<<20)
    assert one_credit <= bounds["credit"] and one_body <= bounds["batch"]
    assert total < bounds["logical"]
    assert active_claim_bytes < 9*1024 and request_bound < bounds["signing_stream"]
    return {"N":n,"R":r,"B":b,"gyms":g,"total_inputs_max":input_count,"total_payments_max":payment_count,
            "credit_claim_row_bytes_upper":claim_bytes,"credits_bytes_upper":credit_total,"batch_bytes_upper":batch_total,
            "inverse_index_bytes_upper":index_total,"delivery_reserved_bytes":delivery_total,
            "logical_bytes_upper":total,"logical_limit":bounds["logical"],"largest_credit_upper":one_credit,
            "largest_batch_upper":one_body,"active_claim_json_upper":active_claim_bytes,"signing_stream_upper":request_bound,
            "signing_stream_limit":bounds["signing_stream"]}


def main():
    if len(sys.argv) != 2:
        raise SystemExit("usage: reference.py output.json")
    output = pathlib.Path(sys.argv[1])
    if output.exists():
        raise SystemExit("output already exists")
    original = (BASE/"inputs/operator-wallet-v1.json").read_bytes()
    if digest(original) != "c1f2d725fcdbc3af7ba1b0a30f4dd4dfc6983352ffb0b4f72d83f7c09ac7b087":
        raise SystemExit("input digest")
    corpus = json.loads(original)
    recipe = json.loads((BASE/"inputs/recipe.json").read_bytes())
    guards = {g["fixture"]:g for g in corpus["guards"]}
    positives = [build_case(guards[t["fixture"]],t,s,recipe) for t in corpus["transactions"] for s in ("prepared","confirmed")]
    positives.append(build_empty(corpus["guards"][0], recipe))
    first = positives[0]
    base_rows = {bytes.fromhex(x["key_hex"]):bytes.fromhex(x["value_hex"]) for x in first["rows"]}
    negatives = []
    for name in recipe["mutations"]:
        rows = copy.deepcopy(base_rows)
        choose = lambda p:next(k for k in rows if k.startswith(p))
        if name == "missing-credit-index": del rows[choose(b"cr/")]
        elif name == "wrong-credit-index": rows[choose(b"cr/")]=struct.pack(">Q",2)
        elif name == "wrong-tx-index": rows[choose(b"bt/")]=struct.pack(">Q",2)
        elif name == "wrong-day-index": rows[choose(b"bd/")]=struct.pack(">Q",2)
        elif name == "missing-spent": del rows[choose(b"spent/")]
        elif name == "wrong-spent": rows[choose(b"spent/")]=struct.pack(">Q",2)
        elif name == "wrong-object-reference":
            key=next(k for k,v in rows.items() if k.startswith(b"o/") and json.loads(v)["kind"] == "credit")
            rows[key]=canon(ref("change",1))
        elif name == "missing-event": del rows[number_key("e/",1)]
        elif name == "duplicate-event-target": rows[number_key("e/",2)]=canon(ref("credit",1))
        elif name == "unknown-key": rows[b"foreign"]=b"{}"
        elif name == "wrong-claims-digest":
            body=json.loads(rows[number_key("b/",1)]);body["change"]["claims_digest"]=ZERO;rows[number_key("b/",1)]=canon(body)
        elif name == "wrong-logical-bytes":
            head=json.loads(rows[b"head"]);head["logical_bytes"]+=1;rows[b"head"]=canon(head)
        else: raise AssertionError(name)
        if name != "wrong-logical-bytes":
            head=json.loads(rows[b"head"]);head["logical_bytes"]=logical(rows,recipe["bounds"]);rows[b"head"]=canon(head)
        if name == "wrong-claims-digest":
            assert json.loads(rows[number_key("b/",1)])["change"]["claims_digest"] != claims_id(first["change_claims"])
        else:
            assert not validate_indices(rows,recipe["bounds"]), name
        negatives.append({"name":name,"rows":encode_rows(rows),"expected":"refuse"})
    result={"version":2,"scope":"canonical conditional ledger bytes/inverse indices and finite arithmetic, not source/funding authority",
            "budget":byte_proof(recipe["bounds"]),"positives":positives,"refusals":negatives}
    raw=canon(result)+b"\n"
    if len(raw)>8<<20: raise SystemExit("output bound")
    with output.open("xb") as stream: stream.write(raw)
    print(json.dumps({"bytes":len(raw),"sha256":digest(raw),"positives":len(positives),"refusals":len(negatives)}))


if __name__ == "__main__":
    main()
