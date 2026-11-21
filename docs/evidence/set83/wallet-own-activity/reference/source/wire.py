"""Selected WIRE2 frames; never imported or executed before root approval.

Old WIRE1 result payloads are exact qualified premises. New activity and
observation envelopes use the selected original binding as a conditional
codec fixture, not proof of wallet ownership or retained native membership.
"""

import copy
import datetime
import hashlib
import json

from inherited import (MAX_MONEY, MAX_TIME, MAX_U64, canonical, exact_fields,
                       inherited_amounts, inherited_daily_response,
                       inherited_response, integer, lower_hex, require,
                       retained_payment, unsigned_decimal)
from payloads import (capture_valid, detail_payload_valid,
                      observation_payload_valid, page_payload_valid)
from tables import scalar_tables


APP = "SLUG-REMOTE-DAILY-WALLET-2"
REQUEST = ("version", "application", "id", "command", "config_id", "view_id",
           "fixture", "sequence", "capture_sequence", "before_batch", "after",
           "limit", "txid", "output_index", "batch_sequence")
ARMS = ("snapshot", "job", "read", "activity_job", "activity_read",
        "activity_page", "activity_detail", "payout_observation")
RESPONSE = ("version", "application", "id", "command", "config_id", "view_id",
            "ok") + ARMS + ("error",)
OLD = {"snapshot": "snapshot", "login_fixture": "snapshot", "start": "snapshot",
       "stop": "snapshot", "logout": "snapshot", "daily_payout_begin": "job",
       "daily_payout_read": "read", "daily_payout_cancel": "read"}
NEW = {"wallet_activity_begin": "activity_job", "wallet_activity_read": "activity_read",
       "wallet_activity_cancel": "activity_read", "wallet_activity_page": "activity_page",
       "wallet_activity_detail": "activity_detail", "daily_payout_observe": "payout_observation"}
ERRORS = ("invalid_request", "stale_view", "busy", "unauthorized", "forbidden",
          "cancelled", "unavailable", "stale_capture", "not_found", "rate_limited",
          "outcome_unknown", "cleanup_incomplete")
OBSERVATION = ("capture_sequence", "binding", "revision", "high_batch", "captured_at_ms",
               "settlement_observed_at_ms", "stale", "batch_sequence", "payment",
               "observed_at_ms", "state", "error", "wallet_synced", "wallet_height",
               "wallet_tip", "confirmations", "block_hash")


def request(selection, command, **changes):
    value = dict(zip(REQUEST, (2, APP, "2", command, selection["config_id"],
                              selection["view"]["view_id"], "", "0", "0", "0",
                              0, 0, "", 0, "0")))
    value.update(changes)
    return value


def response(q, payload=None, error=""):
    out = {key: q[key] for key in RESPONSE[:6]}
    out["ok"] = not error
    out.update({arm: None for arm in ARMS})
    if not error:
        out[(OLD | NEW)[q["command"]]] = copy.deepcopy(payload)
    out["error"] = error
    return out


def frame_cap(q, kind):
    if kind == "request":
        return 2048
    if q["command"] in OLD:
        return 65536
    if q["command"] in ("wallet_activity_page", "wallet_activity_detail"):
        return 98304
    return 16384


def request_valid(q, selection):
    if not exact_fields(q, REQUEST):
        return False
    if not (type(q["version"]) is int and q["version"] == 2 and q["application"] == APP and
            q["config_id"] == selection["config_id"] and
            all(unsigned_decimal(q[key]) for key in
                ("id", "sequence", "capture_sequence", "before_batch", "batch_sequence")) and
            q["id"] != "0" and type(q["fixture"]) is str and type(q["txid"]) is str and
            all(integer(q[key], 0, 4294967295) for key in ("after", "limit", "output_index")) and
            (q["view_id"] == "" or lower_hex(q["view_id"], nonzero=True))):
        return False
    command = q["command"]
    if q["id"] == "1" and (command != "snapshot" or q["view_id"] != ""):
        return False
    if command in OLD:
        if (q["after"], q["limit"], q["txid"], q["output_index"], q["batch_sequence"]) != (0, 0, "", 0, "0"):
            return False
        if command.startswith("daily_payout_"):
            if not q["view_id"] or q["fixture"] or q["sequence"] == "0":
                return False
            if command != "daily_payout_begin":
                return q["capture_sequence"] == q["before_batch"] == "0"
            return ((q["capture_sequence"] == q["before_batch"] == "0") or
                    (0 < int(q["capture_sequence"]) < int(q["sequence"]) and
                     1 <= int(q["before_batch"]) <= 32768))
        if q["sequence"] != "0" or q["capture_sequence"] != "0" or q["before_batch"] != "0":
            return False
        if command == "login_fixture":
            return q["view_id"] == "" and q["fixture"] == selection["view"]["fixture"]
        if q["fixture"]:
            return False
        return command == "snapshot" or bool(q["view_id"])
    if command not in NEW or not q["view_id"] or q["fixture"] or q["before_batch"] != "0":
        return False
    if command in ("wallet_activity_begin", "wallet_activity_read", "wallet_activity_cancel"):
        return (q["sequence"] != "0" and q["capture_sequence"] == "0" and
                (q["after"], q["limit"], q["txid"], q["output_index"], q["batch_sequence"]) == (0, 0, "", 0, "0"))
    if q["sequence"] != "0" or q["capture_sequence"] == "0":
        return False
    if command == "wallet_activity_page":
        return q["after"] <= 256 and 1 <= q["limit"] <= 8 and q["txid"] == "" and q["output_index"] == 0 and q["batch_sequence"] == "0"
    if q["after"] != 0 or q["limit"] != 0 or not lower_hex(q["txid"]):
        return False
    if command == "wallet_activity_detail":
        return q["output_index"] == 0 and q["batch_sequence"] == "0"
    return q["output_index"] <= 255 and 1 <= int(q["batch_sequence"]) <= 32767


def activity_job_valid(j):
    if not (exact_fields(j, ("sequence", "state", "error")) and
            unsigned_decimal(j["sequence"]) and j["sequence"] != "0"):
        return False
    if j["state"] == "failed":
        return j["error"] in ("busy", "capacity", "activity_unavailable")
    return j["state"] in ("working", "done", "cancelled") and j["error"] == ""


def activity_capture_valid(c, selection):
    return (exact_fields(c, ("sequence", "binding", "metadata")) and
            unsigned_decimal(c["sequence"]) and c["sequence"] != "0" and
            canonical(c["binding"]) == canonical(selection["binding"]) and capture_valid(c["metadata"]))


def payment_valid(p, observed_ms):
    if not exact_fields(p, ("day", "created_at_ms", "txid", "output_index", "atoms", "recorded_state")):
        return False
    if not (type(p["day"]) is str and len(p["day"]) == 10 and
            integer(p["created_at_ms"], 1, observed_ms) and lower_hex(p["txid"]) and
            integer(p["output_index"], 0, 255) and integer(p["atoms"], 100000000, MAX_MONEY) and
            p["recorded_state"] in ("prepared", "broadcast", "confirmed")):
        return False
    day = datetime.date(1970, 1, 1) + datetime.timedelta(days=p["created_at_ms"] // 86400000)
    return day.isoformat() == p["day"]


def response_valid(r, q, selection, addresses, old_payloads):
    if not request_valid(q, selection) or not exact_fields(r, RESPONSE):
        return False
    if (any(type(r[k]) is not type(q[k]) or r[k] != q[k] for k in RESPONSE[:6]) or
            type(r["ok"]) is not bool or q["view_id"] not in ("", selection["view"]["view_id"])):
        return False
    if not r["ok"]:
        return r["error"] in ERRORS and all(r[arm] is None for arm in ARMS)
    arm = (OLD | NEW)[q["command"]]
    if r["error"] != "" or r[arm] is None or any(r[x] is not None for x in ARMS if x != arm):
        return False
    v = r[arm]
    if q["command"] in OLD:
        return canonical(v) in old_payloads.get(q["command"], set())
    if arm == "activity_job":
        return activity_job_valid(v) and v["sequence"] == q["sequence"]
    if arm == "activity_read":
        if not exact_fields(v, ("job", "capture")) or not activity_job_valid(v["job"]) or v["job"]["sequence"] != q["sequence"]:
            return False
        j, c = v["job"], v["capture"]
        if c is not None and not activity_capture_valid(c, selection):
            return False
        if j["state"] == "done":
            return c is not None and c["sequence"] == j["sequence"]
        return c is None or int(c["sequence"]) < int(j["sequence"])
    if arm in ("activity_page", "activity_detail"):
        keys = ("capture", "after", "limit", "next", "has_more", "items") if arm == "activity_page" else ("capture", "item", "outputs")
        if not exact_fields(v, keys) or not activity_capture_valid(v["capture"], selection) or v["capture"]["sequence"] != q["capture_sequence"]:
            return False
        if arm == "activity_page":
            return v["after"] == q["after"] and v["limit"] == q["limit"] and page_payload_valid(v["capture"]["metadata"], v["after"], v["limit"], v["next"], v["has_more"], v["items"])
        return (v["item"]["txid"] == q["txid"] and
                detail_payload_valid(v["capture"]["metadata"], v["item"], v["outputs"], addresses))
    if not exact_fields(v, OBSERVATION):
        return False
    if not (v["capture_sequence"] == q["capture_sequence"] and v["batch_sequence"] == q["batch_sequence"] and
            canonical(v["binding"]) == canonical(selection["binding"]) and
            unsigned_decimal(v["revision"]) and v["revision"] != "0" and
            unsigned_decimal(v["high_batch"], 32767) and 1 <= int(v["batch_sequence"]) <= int(v["high_batch"]) and
            integer(v["captured_at_ms"], 1, MAX_TIME) and
            integer(v["settlement_observed_at_ms"], 1, MAX_TIME) and
            type(v["stale"]) is bool and
            payment_valid(v["payment"], v["settlement_observed_at_ms"]) and
            v["payment"]["txid"] == q["txid"] and v["payment"]["output_index"] == q["output_index"]):
        return False
    return observation_payload_valid({k: v[k] for k in OBSERVATION[9:]})


def produce(inputs):
    old_activity = inputs["activity-wire-v1.json"]
    old_payout = inputs["payout-wire-v1.json"]
    daily = inputs["daily-wire-v1.json"]
    selection = copy.deepcopy(daily["selection"])
    # Carry only original addresses already independently qualified. This is
    # an explicit conditional grammar premise, not a new address decoder.
    addresses = {""}
    for row in old_activity["positives"]:
        detail = row["value"].get("detail")
        if detail:
            addresses.update(x["address"] for x in detail["outputs"])
    cases, old_payloads = [], {}

    def add(name, q, payload=None, valid=True, error="", raw=None, kind="response", scope="conditional codec"):
        obj = q if kind == "request" else response(q, payload, error)
        frame = canonical(obj) + b"\n" if raw is None else raw
        cases.append({"name": name, "kind": kind, "valid": valid, "scope": scope,
                      "request": copy.deepcopy(q), "frame_hex": frame.hex(),
                      "frame_bytes": len(frame), "sha256": hashlib.sha256(frame).hexdigest()})
        return obj

    old_names = ("initial-signed-out-result", "control-snapshot-result", "control-login_fixture-result",
                 "control-start-result", "control-stop-result", "control-logout-result", "begin-working",
                 "fresh-empty-nonfinal", "completed-cancel-keeps-capture",
                 "lossless-paid-9007199254740991", "lossless-paid-9007199254740992",
                 "lossless-paid-9223372036854775807", "lossless-reserved-9223372036854775807",
                 "lossless-four-4096-digit-components")
    for name in old_names:
        old = inherited_daily_response(daily, name)
        source = next(x for x in daily["cases"] if x["name"] == name)
        oq = source["request"]
        q = request(selection, oq["command"], **{k: oq[k] for k in oq if k not in ("version", "application", "command")})
        arm = OLD[q["command"]]
        payload = old[arm]
        old_payloads.setdefault(q["command"], set()).add(canonical(payload))
        add("wire2-" + name + "-request", q, kind="request")
        add("wire2-" + name, q, payload)

    detail_names = ("zero256-detail-response", "zero256-incoming-detail", "modeled-self-payment",
                    "modeled-partial-input", "modeled-zero-fee", "modeled-zero-atoms", "modeled-coinbase",
                    "modeled-no-owned-output", "modeled-maximum-money", "modeled-maximum-outputs",
                    "height-and-confirmations-boundary")
    detail_examples = {}
    for name in detail_names:
        old = inherited_response(old_activity, name, "detail")
        c = {"sequence": "1", "binding": copy.deepcopy(selection["binding"]), "metadata": old["capture"]}
        v = {"capture": c, "item": old["item"], "outputs": old["outputs"]}
        q = request(selection, "wallet_activity_detail", capture_sequence="1", txid=old["item"]["txid"])
        add("activity-detail-" + name, q, v)
        detail_examples[name] = (q, v)
    base_q, base_detail = detail_examples["zero256-detail-response"]
    base_capture = copy.deepcopy(base_detail["capture"])
    for state, error in (("working", ""), ("done", ""), ("cancelled", ""), ("failed", "busy"),
                         ("failed", "capacity"), ("failed", "activity_unavailable")):
        seq = "1" if state == "done" else "2"
        job = {"sequence": seq, "state": state, "error": error}
        q = request(selection, "wallet_activity_begin", sequence=seq)
        add("activity-begin-" + state + "-" + (error or "empty"), q, job)
        q = request(selection, "wallet_activity_read", sequence=seq)
        add("activity-read-" + state + "-retained-" + (error or "empty"), q,
            {"job": job, "capture": copy.deepcopy(base_capture)})
        if state != "done":
            add("activity-read-" + state + "-null-" + (error or "empty"), q, {"job": job, "capture": None})
    q = request(selection, "wallet_activity_cancel", sequence="2")
    add("activity-cancel-retains-original", q,
        {"job": {"sequence": "2", "state": "cancelled", "error": ""}, "capture": copy.deepcopy(base_capture)})
    page_names = ("page-empty-response", "page-end-response", "page-interior-response",
                  "page-tail-response", "page-full-end-response", "mixed-order-page")
    pages = {}
    for name in page_names:
        old = inherited_response(old_activity, name, "page")
        c = {"sequence": "1", "binding": copy.deepcopy(selection["binding"]), "metadata": old["capture"]}
        p = {"capture": c}
        p.update({key: old[key] for key in ("after", "limit", "next", "has_more", "items")})
        q = request(selection, "wallet_activity_page", capture_sequence="1", after=p["after"], limit=p["limit"])
        add("activity-" + name, q, p)
        pages[name] = (q, p)

    daily_read = inherited_daily_response(daily, "eight-inherited-payment-items")["read"]
    daily_capture = copy.deepcopy(daily_read["capture"])
    chosen = daily_capture["first"]["items"][0]
    payment = chosen["payment"]
    observations = {}
    for name in ("wallet_not_observed", "wallet_unconfirmed", "wallet_confirmed", "wallet_unavailable",
                 "wallet_max_confirmations", "wallet_max_timestamp"):
        old = inherited_response(old_payout, name, "observation")
        first = daily_capture["first"]
        v = {"capture_sequence": daily_capture["sequence"], "binding": copy.deepcopy(selection["binding"]),
             "revision": first["revision"], "high_batch": first["high_batch"],
             "captured_at_ms": daily_capture["captured_at_ms"], "settlement_observed_at_ms": first["observed_at_ms"],
             "stale": daily_capture["stale"], "batch_sequence": chosen["batch_sequence"], "payment": copy.deepcopy(payment)}
        v.update({key: old[key] for key in OBSERVATION[9:]})
        q = request(selection, "daily_payout_observe", capture_sequence=v["capture_sequence"],
                    batch_sequence=v["batch_sequence"], txid=payment["txid"], output_index=payment["output_index"])
        add("selected-" + name, q, v, scope="Original payload shape under conditional retained payment and wallet-output premise")
        observations[name] = (q, v)
    q, v = observations["wallet_confirmed"]
    stale = copy.deepcopy(v); stale["stale"] = True
    add("selected-dated-stale-payment", q, stale)
    genesis = copy.deepcopy(observations["wallet_not_observed"][1]); genesis["wallet_height"] = 0
    add("selected-height-zero-tip-present", observations["wallet_not_observed"][0], genesis)

    # One canonical request per new operation and immutable maximum sequence.
    new_qs = [request(selection, command, sequence="1") for command in
              ("wallet_activity_begin", "wallet_activity_read", "wallet_activity_cancel")]
    new_qs += [pages["page-tail-response"][0], base_q, q]
    for nq in new_qs:
        add("request-" + nq["command"], nq, kind="request")
    mq = request(selection, "wallet_activity_begin", id=str(MAX_U64), sequence=str(MAX_U64))
    add("request-max-sequence", mq, kind="request")
    for error in ERRORS:
        add("activity-closed-error-" + error, new_qs[0], error=error)

    # Scalar/selector mutations each retain the exact other fields.
    for field, values in (("id", (0, True, None, "0", "01", "-1", "+1", "1e0", "18446744073709551616")),
                          ("sequence", (1, True, "0", "01", "-1", "18446744073709551616"))):
        for i, val in enumerate(values):
            bad = copy.deepcopy(new_qs[0]); bad[field] = val
            add("invalid-request-" + field + "-" + str(i), bad, valid=False, kind="request")
    for field, val in (("version", 1), ("application", "SLUG-REMOTE-DAILY-WALLET-1"), ("config_id", "f" * 64),
                       ("view_id", ""), ("fixture", "zero256"), ("before_batch", "1"), ("after", 1),
                       ("limit", 1), ("txid", "0" * 64), ("output_index", 1), ("batch_sequence", "1")):
        bad = copy.deepcopy(new_qs[0]); bad[field] = val
        add("invalid-request-arm-" + field, bad, valid=False, kind="request")
    for name, template, field, val in (
            ("page-limit-zero", new_qs[3], "limit", 0), ("page-limit-nine", new_qs[3], "limit", 9),
            ("page-after257", new_qs[3], "after", 257), ("page-after-bool", new_qs[3], "after", True),
            ("page-capture-numeric", new_qs[3], "capture_sequence", 1),
            ("detail-tx-upper", base_q, "txid", "F" * 64),
            ("observation-output256", q, "output_index", 256), ("observation-batch0", q, "batch_sequence", "0"),
            ("observation-batch32768", q, "batch_sequence", "32768")):
        bad = copy.deepcopy(template); bad[field] = val
        add("invalid-request-" + name, bad, valid=False, kind="request")

    # Public payload shape refusals, separate from native membership decisions.
    mutations = (("amount-string", ("item", "received_atoms"), "1"),
                 ("amount-bool", ("item", "received_atoms"), True),
                 ("negative-received", ("item", "received_atoms"), -1),
                 ("above-money", ("item", "received_atoms"), MAX_MONEY + 1),
                 ("net-conflict", ("item", "net_atoms"), MAX_MONEY + 1),
                 ("fee-bool", ("item", "fee_atoms"), True),
                 ("tx-foreign", ("item", "txid"), "e" * 64),
                 ("capture-foreign", ("capture", "sequence"), "2"),
                 ("binding-foreign", ("capture", "binding", "wallet_owner_id"), "e" * 64),
                 ("capture-node-tip", ("capture", "metadata", "node_tip"), "e" * 64),
                 ("outputs-null", ("outputs",), None))
    for name, path, val in mutations:
        bad = copy.deepcopy(base_detail); target = bad
        for part in path[:-1]: target = target[part]
        target[path[-1]] = val
        add("invalid-detail-" + name, base_q, bad, valid=False)
    pq, pv = pages["page-tail-response"]
    for field, val in (("next", 255), ("has_more", True), ("items", None), ("after", 247), ("limit", 7)):
        bad = copy.deepcopy(pv); bad[field] = val
        add("invalid-page-" + field, pq, bad, valid=False)
    bad = copy.deepcopy(pv); bad["items"][1] = copy.deepcopy(bad["items"][0])
    add("invalid-page-duplicate-tx", pq, bad, valid=False)
    for state, capture in (("done", None), ("done", dict(base_capture, sequence="2")),
                           ("working", base_capture), ("cancelled", base_capture), ("failed", base_capture)):
        j = {"sequence": "1", "state": state, "error": "busy" if state == "failed" else ""}
        rq = request(selection, "wallet_activity_read", sequence="1")
        add("invalid-read-" + state + "-" + ("null" if capture is None else "sequence"), rq,
            {"job": j, "capture": copy.deepcopy(capture)}, valid=False)
    oq, ov = observations["wallet_confirmed"]
    for field, val in (("capture_sequence", "999"), ("batch_sequence", "0"), ("high_batch", "0"),
                       ("revision", "0"), ("stale", "true"), ("confirmations", 0),
                       ("block_hash", "0" * 64), ("wallet_tip", "0" * 64), ("error", "wallet_unavailable")):
        bad = copy.deepcopy(ov); bad[field] = val
        add("invalid-observation-" + field, oq, bad, valid=False)
    bad = copy.deepcopy(ov); bad["payment"]["atoms"] = MAX_MONEY + 1
    add("invalid-observation-payment-over-bound", oq, bad, valid=False)

    # Canonical framing mutations use a small, fully valid new response.
    rq = new_qs[0]; rv = response(rq, {"sequence": "1", "state": "working", "error": ""})
    rf = canonical(rv) + b"\n"
    for name, raw in (("missing-lf", rf[:-1]), ("double-lf", rf+b"\n"), ("crlf", rf[:-1]+b"\r\n"),
                      ("space", b" "+rf), ("duplicate-version", rf.replace(b'{"version":2,', b'{"version":2,"version":2,', 1)),
                      ("unknown-key", rf[:-2]+b',"unknown":0}\n'),
                      ("reordered", canonical({key: rv[key] for key in reversed(rv)})+b"\n")):
        add("invalid-frame-" + name, rq, valid=False, raw=raw)
    wrong = copy.deepcopy(rv); wrong["activity_read"] = {"job": rv["activity_job"], "capture": None}
    add("invalid-frame-two-arms", rq, valid=False, raw=canonical(wrong)+b"\n")

    def decode(frame):
        def unique(pairs):
            out = {}
            for key, val in pairs:
                require(key not in out, "duplicate key")
                out[key] = val
            return out
        require(frame.endswith(b"\n") and b"\n" not in frame[:-1] and b"\r" not in frame,
                "one trailing LF")
        obj = json.loads(frame[:-1], object_pairs_hook=unique)
        require(canonical(obj)+b"\n" == frame, "canonical bytes")
        return obj
    for case in cases:
        actual = False
        try:
            frame = bytes.fromhex(case["frame_hex"])
            require(len(frame) <= frame_cap(case["request"], case["kind"]), "command frame cap")
            val = decode(frame)
            actual = request_valid(val, selection) if case["kind"] == "request" else response_valid(val, case["request"], selection, addresses, old_payloads)
        except (ValueError, KeyError, TypeError, IndexError, OverflowError):
            actual = False
        require(actual == case["valid"], "authored frame decision mismatch: " + case["name"])
    require(len({c["name"] for c in cases}) == len(cases), "unique authored cases")
    require((len(cases), sum(c["valid"] for c in cases), sum(not c["valid"] for c in cases)) ==
            (165, 90, 75), "hand-tallied frame inventory")

    # The inherited atom examples are cross-checked without projecting them
    # into a new live wallet or re-deriving any historical transaction.
    inherited = inherited_amounts(old_activity, old_payout)
    return {"version": 1, "scope": "Conditional selected WIRE2 transport; no account, wallet, membership, financial or runtime authority",
            "selection": selection, "cases": cases, "scalar_tables": scalar_tables(),
            "inherited_amounts": inherited,
            "context_cases": context_cases(selection, daily_capture, chosen),
            "operation_cases": operation_cases(),
            "bounds": frame_bounds(selection, daily),
            "counts": {"frames": len(cases), "valid": sum(c["valid"] for c in cases),
                       "invalid": sum(not c["valid"] for c in cases)}}


def context_cases(selection, capture, chosen):
    """Authored native predicates; these are not stateless frame refusals."""
    payment = chosen["payment"]
    args = (capture["sequence"], chosen["batch_sequence"], payment["txid"], payment["output_index"])
    rows = []
    def add(name, value, selectors, valid):
        actual = False
        try:
            actual = canonical(retained_payment(value, *selectors)) == canonical(chosen)
        except (ValueError, KeyError, TypeError): pass
        require(actual == valid, "authored retained membership: " + name)
        rows.append({"name": name, "capture": value, "selectors": list(selectors), "valid": valid,
                     "scope": "Already validated capture premise; native immutable membership required"})
    same = copy.deepcopy(capture); same["current"] = copy.deepcopy(same["first"])
    add("identical-first-current-one-selection", same, args, True)
    add("selected-original-window", copy.deepcopy(capture), args, True)
    stale = copy.deepcopy(same); stale["stale"] = True
    add("stale-dated-selection", stale, args, True)
    conflict = copy.deepcopy(same); conflict["current"]["items"][0]["payment"]["atoms"] += 1
    add("conflicting-duplicate-refused", conflict, args, False)
    absent = copy.deepcopy(same)
    for page in (absent["first"], absent["current"]): page["items"] = page["items"][1:]
    add("evicted-row-refused", absent, args, False)
    add("replaced-capture-refused", same, (str(int(args[0])+1),)+args[1:], False)
    add("wrong-output-refused", same, args[:3]+((args[3]+1)%256,), False)
    add("wrong-tx-refused", same, args[:2]+("f"*64,)+args[3:], False)
    return rows


def operation_cases():
    # Declarative original-owner cases, never claimed as executed lifecycle
    # evidence by this producer. Native tests must supply their real owners.
    return [
        {"name": "first-admission", "before": ["0", None], "request": ["begin", "1"], "expected": "working1"},
        {"name": "exact-admission-repeat", "before": ["1", "working"], "request": ["begin", "1"], "expected": "same1-no-new-task"},
        {"name": "skip-sequence", "before": ["1", "working"], "request": ["begin", "3"], "expected": "refused-no-new-task"},
        {"name": "caller-cancel-after-admission", "before": ["1", "working"], "request": ["caller-cancel", "1"], "expected": "same1-remains-owned"},
        {"name": "failed-refresh-retains-capture", "before": ["2", "capture1"], "request": ["fail", "2"], "expected": "job2-failed-capture1"},
        {"name": "cancel-refresh-joins", "before": ["2", "capture1"], "request": ["cancel", "2"], "expected": "job2-cancelled-capture1-after-join"},
        {"name": "successful-refresh-invalidates", "before": ["2", "capture1"], "request": ["complete", "2"], "expected": "capture2-old-selector-refused"},
        {"name": "old-view-disclosure", "before": ["1", "old-view"], "request": ["read", "1"], "expected": "stale_view-no-disclosure"},
        {"name": "stop-retains-dated", "before": ["1", "capture1"], "request": ["stop", "0"], "expected": "joined-capture1-dated"},
        {"name": "close-clears-access", "before": ["1", "capture1"], "request": ["close", "0"], "expected": "joined-no-view-access"},
    ]


def frame_bounds(selection, daily):
    # Conservative independent JSON-width arithmetic, not attainable monetary
    # states. Count the complete key punctuation and maximum encoded scalars.
    def obj(fields): return 2 + max(0, len(fields)-1) + sum(len(k)+3+v for k,v in fields)
    def array(n, width): return 2 + max(0,n-1) + n*width
    binding = len(canonical(selection["binding"])) - len(str(selection["view"]["generation"])) + 20
    metadata = obj((("version",1),("captured_at_ms",15),("wallet_height",10),("wallet_tip",66),
                    ("node_height",10),("node_tip",66),("count",3),("has_older",5)))
    block = obj((("height",10),("hash",66),("time_ms",15)))
    item = obj((("txid",66),("raw_sha256",66),("recorded_at_ms",15),("coinbase",5),("block",block),
                ("confirmations",10),("received_atoms",16),("spent_atoms",16),("change_atoms",16),
                ("net_atoms",17),("fee_atoms",16),("input_count",3),("output_count",3),
                ("owned_input_count",3),("owned_output_count",3)))
    # 90-byte permitted address is ASCII; use 180 even if every byte needed
    # escaping. Real address grammars are narrower, so this stays conservative.
    output = obj((("index",3),("atoms",16),("owned",5),("change",5),("address",182)))
    cap = obj((("sequence",22),("binding",binding),("metadata",metadata)))
    page = obj((("capture",cap),("after",3),("limit",1),("next",3),("has_more",5),("items",array(8,item))))
    detail = obj((("capture",cap),("item",item),("outputs",array(256,output))))
    command_width = max(len(command) for command in (OLD | NEW)) + 2
    envelope = obj((("version",1),("application",len(APP)+2),("id",22),
                    ("command",command_width),("config_id",66),("view_id",66),("ok",5)) +
                   tuple((key,4) for key in ARMS) + (("error",max(map(len,ERRORS))+2),)) + 1
    # This treats every unused arm as null and then replaces the selected one.
    page_frame, detail_frame = envelope - 4 + page, envelope - 4 + detail
    job = obj((("sequence",22),("state",11),("error",22)))
    read = obj((("job",job),("capture",cap)))
    payment = obj((("day",12),("created_at_ms",15),("txid",66),("output_index",3),
                   ("atoms",16),("recorded_state",11)))
    observation = obj((("capture_sequence",22),("binding",binding),("revision",22),("high_batch",7),
                       ("captured_at_ms",15),("settlement_observed_at_ms",15),("stale",5),
                       ("batch_sequence",7),("payment",payment),("observed_at_ms",15),
                       ("state",14),("error",20),("wallet_synced",5),("wallet_height",10),
                       ("wallet_tip",66),("confirmations",10),("block_hash",66)))
    small_frame = envelope - 4 + max(job, read, observation)
    request_frame = obj((("version",1),("application",len(APP)+2),("id",22),
                        ("command",command_width),("config_id",66),("view_id",66),
                        ("fixture",len("incrementing256")+2),("sequence",22),("capture_sequence",22),
                        ("before_batch",22),("after",3),("limit",1),("txid",66),
                        ("output_index",3),("batch_sequence",22))) + 1
    inherited_bound = daily["bounds"]["response_conservative_bytes_including_lf"]
    require(inherited_bound == 56738, "qualified WIRE1 component bound")
    old_frame = inherited_bound + sum(len(key)+8 for key in ARMS[3:])
    require(request_frame <= 2048 and old_frame <= 65536 and small_frame <= 16384 and
            page_frame <= 98304 and detail_frame <= 98304, "command frame conservative bounds")
    return {"scope": "Conservative codec widths, not jointly attainable financial states; original binding length is selected input",
            "binding_bytes":binding,"metadata_bytes":metadata,"item_bytes":item,"output_bytes":output,
            "capture_bytes":cap,"page_payload_bytes":page,"detail_payload_bytes":detail,
            "page_frame_bytes_at_most":page_frame,"detail_frame_bytes_at_most":detail_frame,
            "request_frame_bytes_at_most":request_frame,"old_command_frame_bytes_at_most":old_frame,
            "new_small_frame_bytes_at_most":small_frame,
            "request_cap":2048,"old_response_cap":65536,"new_small_response_cap":16384,
            "new_page_detail_response_cap":98304}
