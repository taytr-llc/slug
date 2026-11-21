#!/usr/bin/env python3
"""Independent, conditional DAILY wallet wire model. Root alone executes it.

No product imports, Go execution, runtime owner, authentication, wallet, chain,
ledger or current-account authority is modeled. Public fixture identity comes
only from the two prior independent qualified corpora in the copied catalog.
"""
import argparse
import copy
import datetime
import hashlib
import json
import math
import os
from pathlib import Path
import re
import struct

HERE = Path(__file__).resolve().parent
APP = "SLUG-REMOTE-DAILY-WALLET-1"
U64 = (1 << 64) - 1
I64 = (1 << 63) - 1
SAFE = (1 << 53) - 1
MONEY = 2100000000000000
MAX_TIME = 253402300799999
REQUEST_CAP = 2048
RESPONSE_CAP = 65536
REQUEST_KEYS = "version application id command config_id view_id fixture sequence capture_sequence before_batch".split()
RESPONSE_KEYS = "version application id command config_id view_id ok snapshot job read error".split()
VIEW_KEYS = "version view_id account_authority_id account_id generation fixture root_public_key gym_public_key gym_id".split()
BINDING_KEYS = "view config_id source_binding_id settlement_config_id recipient_script issue_profile_id original_source_binding_id wallet_owner_id allocation_binding_id".split()
JOB_KEYS = "sequence capture_sequence before_batch state error".split()
CAPTURE_KEYS = "sequence binding captured_at_ms stale first current".split()
PAGE_KEYS = "version binding_id settlement_config_id revision high_batch before_batch next_before done recipient_script balance observed_at_ms items".split()
BALANCE_KEYS = "gym_id confirmed available reserved_atoms paid_atoms reason".split()
ITEM_KEYS = "day created_at_ms txid output_index atoms recorded_state".split()
ACCOUNT_KEYS = "state fixture id generation hold_until_ms held can_export last_error".split()
SNAP_KEYS = "version config_id state account view remote error".split()
REMOTE_KEYS = "version config_id view source_binding_id original_source_binding_id state worker_state last_error attempts hashes_per_second_milli records pending wallet_state wallet_height spendable_atoms immature_atoms unconfirmed_atoms payout_address".split()
CONTROLS = "snapshot login_fixture start stop logout".split()
READS = "daily_payout_begin daily_payout_read daily_payout_cancel".split()
ERRORS = "invalid_request stale_view busy unauthorized forbidden cancelled unavailable stale_capture not_found rate_limited outcome_unknown cleanup_incomplete".split()
JOB_ERRORS = "busy rate_limited stale_capture unauthorized unavailable cancelled".split()
REASONS = "payout-pending broadcast-unknown transaction-unavailable payout-conflict funding-unavailable funding-conflict funding-changed immature awaiting-confirmation below-minimum destination-unavailable daily-batch-complete fee-unavailable fee-budget change-dust signing-unavailable capacity transaction-capacity".split()


class Invalid(ValueError):
    pass


def need(value):
    if not value:
        raise Invalid("conditional wire predicate")


def canonical(value):
    return json.dumps(value, ensure_ascii=True, separators=(",", ":"), allow_nan=False).encode("ascii")


def clone(value):
    return copy.deepcopy(value)


def ordered(value, keys):
    need(type(value) is dict and list(value) == keys)


def number(value, low=0, high=SAFE):
    need(type(value) is int and low <= value <= high)
    return value


def decimal(value, low=0, high=U64, digits=20):
    need(type(value) is str and len(value) <= digits and re.fullmatch(r"0|[1-9][0-9]*", value) is not None)
    n = int(value)
    need(low <= n <= high)
    return n


def boolean(value):
    need(type(value) is bool)


def identifier(value):
    need(type(value) is str and re.fullmatch(r"[0-9a-f]{64}", value) is not None and value != "0" * 64)


def fraction(value):
    ordered(value, ["numerator", "denominator"])
    # Original fractions are reduced, with canonical positive denominators.
    # Bound each component before conversion; never coerce scalar types.
    n = decimal(value["numerator"], 0, 10 ** 4096 - 1, 4096)
    d = decimal(value["denominator"], 1, 10 ** 4096 - 1, 4096)
    need(math.gcd(n, d) == 1)
    return n, d


def checked_balance(value, binding, observed):
    ordered(value, BALANCE_KEYS)
    need(value["gym_id"] == binding["view"]["gym_id"])
    cn, cd = fraction(value["confirmed"])
    an, ad = fraction(value["available"])
    reserved = decimal(value["reserved_atoms"], 0, I64)
    paid = decimal(value["paid_atoms"], 0, I64)
    need(value["reason"] == "" or value["reason"] in REASONS)
    need(cn >= reserved * cd and an * cd <= (cn - reserved * cd) * ad)
    need(cn != 0 or value["reason"] == "")
    need(observed != 0 or (cn == an == reserved == paid == 0 and value["reason"] == ""))
    need(len(canonical(value)) <= 17408)


def checked_page(value, binding):
    ordered(value, PAGE_KEYS)
    need(number(value["version"], 2, 2) == 2)
    need(value["binding_id"] == binding["source_binding_id"] and value["settlement_config_id"] == binding["settlement_config_id"] and value["recipient_script"] == binding["recipient_script"])
    decimal(value["revision"], 1)
    high = decimal(value["high_batch"], 0, 32767)
    before = decimal(value["before_batch"], 1, high + 1)
    low = max(1, before - 8)
    done = before <= 9
    boolean(value["done"])
    need(value["done"] == done and decimal(value["next_before"], 0, 32768) == (0 if done else low))
    observed = number(value["observed_at_ms"], 0, MAX_TIME)
    checked_balance(value["balance"], binding, observed)
    items = value["items"]
    need(type(items) is list and len(items) <= 8)
    need(observed != 0 or (high == 0 and not items))
    last, last_day, last_time = before, None, None
    seen, paid, reserved = set(), 0, 0
    epoch = datetime.datetime(1970, 1, 1, tzinfo=datetime.timezone.utc)
    for row in items:
        ordered(row, ["batch_sequence", "payment"])
        seq = decimal(row["batch_sequence"], low, last - 1)
        last = seq
        item = row["payment"]
        ordered(item, ITEM_KEYS)
        created = number(item["created_at_ms"], 1, observed)
        day = (epoch + datetime.timedelta(milliseconds=created)).strftime("%Y-%m-%d")
        need(item["day"] == day and (last_day is None or item["day"] < last_day) and (last_time is None or created < last_time))
        last_day, last_time = item["day"], created
        # Unchanged payout shape validates lowercase hexBytes(32), including
        # zero. This is not an assertion that a zero txid is attainable.
        need(type(item["txid"]) is str and re.fullmatch(r"[0-9a-f]{64}", item["txid"]) is not None)
        need(item["txid"] not in seen)
        seen.add(item["txid"])
        number(item["output_index"], 0, 255)
        atoms = number(item["atoms"], 100000000, MONEY)
        need(item["recorded_state"] in ["prepared", "broadcast", "confirmed"])
        if item["recorded_state"] == "confirmed":
            paid += atoms
        else:
            need(seq == high)
            reserved += atoms
        need(len(canonical(item)) <= 256)
    need(paid <= int(value["balance"]["paid_atoms"]) and reserved <= int(value["balance"]["reserved_atoms"]))


def checked_view(value, expected):
    ordered(value, VIEW_KEYS)
    number(value["version"], 1, 1)
    identifier(value["view_id"])
    number(value["generation"], 1, SAFE)
    for k in VIEW_KEYS:
        if k not in ["view_id", "generation"]:
            need(value[k] == expected[k])


def checked_binding(value, expected):
    ordered(value, BINDING_KEYS)
    checked_view(value["view"], expected["view"])
    # Ordered-byte equality also binds the original ViewID and generation.
    need(canonical(value) == canonical(expected))


def checked_job(value):
    ordered(value, JOB_KEYS)
    seq = decimal(value["sequence"], 1)
    cap = decimal(value["capture_sequence"])
    before = decimal(value["before_batch"], 0, 32768)
    need((cap == 0) == (before == 0) and (cap == 0 or cap < seq))
    need(value["state"] in ["working", "done", "failed", "cancelled"])
    need(value["error"] in JOB_ERRORS if value["state"] == "failed" else value["error"] == "")


def checked_capture(value, binding):
    ordered(value, CAPTURE_KEYS)
    decimal(value["sequence"], 1)
    checked_binding(value["binding"], binding)
    number(value["captured_at_ms"], 1, MAX_TIME)
    boolean(value["stale"])
    first, current = value["first"], value["current"]
    checked_page(first, binding)
    checked_page(current, binding)
    need(int(first["before_batch"]) == int(first["high_batch"]) + 1)
    for k in ["version", "binding_id", "settlement_config_id", "revision", "high_batch", "recipient_script", "balance", "observed_at_ms"]:
        need(canonical(first[k]) == canonical(current[k]))
    delta = int(first["before_batch"]) - int(current["before_batch"])
    need(delta >= 0 and delta % 8 == 0)
    if delta == 0:
        need(canonical(first) == canonical(current))


def checked_read(value, binding):
    ordered(value, ["job", "capture"])
    j, c = value["job"], value["capture"]
    checked_job(j)
    if c is None:
        need(j["state"] != "done")
        return
    checked_capture(c, binding)
    seq, cap, before = int(j["sequence"]), int(c["sequence"]), int(j["before_batch"])
    need(cap <= seq)
    if j["state"] == "done":
        if j["capture_sequence"] == "0":
            need(not c["stale"] and cap == seq and canonical(c["first"]) == canonical(c["current"]))
        else:
            need(int(j["capture_sequence"]) == cap and cap < seq and before in [int(c["first"]["before_batch"]), int(c["current"]["before_batch"])])
    else:
        # A failed/cancelled/working refresh may disclose an older retained
        # capture, never relabel it as the unfinished job's capture.
        need(cap < seq)
        if j["capture_sequence"] != "0":
            need(int(j["capture_sequence"]) == cap)


def checked_request(value, selected):
    ordered(value, REQUEST_KEYS)
    number(value["version"], 1, 1)
    need(value["application"] == APP and value["config_id"] == selected["config_id"])
    rid = decimal(value["id"], 1)
    command = value["command"]
    need(command in CONTROLS + READS)
    view = value["view_id"]
    need(view in ["", selected["view"]["view_id"]])
    seq = decimal(value["sequence"])
    cap = decimal(value["capture_sequence"])
    before = decimal(value["before_batch"], 0, 32768)
    if command in CONTROLS:
        need(seq == cap == before == 0)
        if command == "login_fixture":
            need(view == "" and value["fixture"] == selected["view"]["fixture"])
        else:
            need(value["fixture"] == "")
            if command != "snapshot":
                need(view != "")
    else:
        need(view != "" and value["fixture"] == "" and seq > 0)
        if command == "daily_payout_begin":
            need((cap == 0) == (before == 0) and (cap == 0 or cap < seq))
        else:
            need(cap == before == 0)
    if rid == 1:
        need(command == "snapshot" and view == "")


def checked_snapshot(value, selected):
    ordered(value, SNAP_KEYS)
    number(value["version"], 2, 2)
    need(value["config_id"] == selected["config_id"])
    state, account = value["state"], value["account"]
    ordered(account, ACCOUNT_KEYS)
    need(state in ["signed_out", "signed_in", "failed", "expired"] and account["state"] == state and account["fixture"] == selected["view"]["fixture"] and account["last_error"] == value["error"])
    for key in ["held", "can_export"]:
        boolean(account[key])
    need(not account["can_export"])
    number(account["generation"])
    number(account["hold_until_ms"])
    if state != "signed_in":
        need(value["view"] is None and value["remote"] is None and account["id"] == "" and account["generation"] == account["hold_until_ms"] == 0 and not account["held"])
        need(value["error"] == ("remote_unavailable" if state == "failed" else "") or (state == "expired" and value["error"] == "remote_unavailable"))
        return
    need(value["error"] == "")
    checked_view(value["view"], selected["view"])
    need(canonical(value["view"]) == canonical(selected["view"]) and account["id"] == selected["view"]["account_id"] and account["generation"] == selected["view"]["generation"])
    need(not account["held"] or account["hold_until_ms"] > 0)
    remote = value["remote"]
    ordered(remote, REMOTE_KEYS)
    number(remote["version"], 2, 2)
    need(remote["config_id"] == selected["config_id"] and canonical(remote["view"]) == canonical(selected["view"]))
    for a, b in [("source_binding_id", "source_binding_id"), ("original_source_binding_id", "original_source_binding_id")]:
        need(remote[a] == selected["binding"][b])
    need(remote["payout_address"] == selected["remote"]["base"]["expected_local"]["recipient"]["recipient"]["address"])
    need(remote["state"] in ["stopped", "starting", "running", "failed"])
    need(remote["worker_state"] in ["ready", "stopped", "failed", "paused", "preparing", "retrying", "hashing", "delivering", "unavailable"])
    need(remote["last_error"] in ["", "unavailable", "outcome_unknown", "busy", "capacity", "rate_limited", "expired", "rejected", "not_found", "unauthorized", "forbidden", "invalid_request", "too_large", "stale", "clock", "cycle_unavailable", "signer_unavailable", "worker_unavailable", "cleanup_incomplete"])
    number(remote["attempts"])
    number(remote["hashes_per_second_milli"])
    need(remote["state"] == "running" or remote["hashes_per_second_milli"] == 0)
    records = number(remote["records"], 0, selected["remote"]["base"]["max_records"])
    number(remote["pending"], 0, records)
    need(remote["wallet_state"] in ["unavailable", "syncing", "ready"])
    number(remote["wallet_height"], 0, 2147483647)
    for k in ["spendable_atoms", "immature_atoms", "unconfirmed_atoms"]:
        number(remote[k], 0, MONEY)
    if remote["wallet_state"] == "unavailable":
        need(all(remote[k] == 0 for k in ["wallet_height", "spendable_atoms", "immature_atoms", "unconfirmed_atoms"]))


def checked_response(value, request, selected):
    checked_request(request, selected)
    ordered(value, RESPONSE_KEYS)
    number(value["version"], 1, 1)
    for k in ["version", "application", "id", "command", "config_id", "view_id"]:
        need(type(value[k]) is type(request[k]) and value[k] == request[k])
    boolean(value["ok"])
    if not value["ok"]:
        need(all(value[k] is None for k in ["snapshot", "job", "read"]) and value["error"] in ERRORS)
        return
    need(value["error"] == "")
    command = request["command"]
    if command in CONTROLS:
        need(value["job"] is None and value["read"] is None and value["snapshot"] is not None)
        snap = value["snapshot"]
        checked_snapshot(snap, selected)
        if command == "login_fixture":
            need(snap["state"] == "signed_in" and snap["remote"]["state"] == "stopped")
        elif command == "logout":
            need(snap["state"] == "signed_out")
        elif command in ["start", "stop"]:
            need(snap["state"] == "signed_in")
            if command == "stop":
                need(snap["remote"]["state"] in ["stopped", "failed"] and snap["remote"]["hashes_per_second_milli"] == 0)
        elif request["view_id"] == "":
            need(snap["state"] != "signed_in" and (request["id"] != "1" or snap["state"] == "signed_out"))
    elif command == "daily_payout_begin":
        need(value["snapshot"] is None and value["read"] is None and value["job"] is not None)
        checked_job(value["job"])
        need(all(value["job"][k] == request[k] for k in ["sequence", "capture_sequence", "before_batch"]))
    else:
        need(value["snapshot"] is None and value["job"] is None and value["read"] is not None)
        checked_read(value["read"], selected["binding"])
        need(value["read"]["job"]["sequence"] == request["sequence"])


def unique_object(pairs):
    out = {}
    for k, v in pairs:
        need(k not in out)
        out[k] = v
    return out


def parse_frame(raw, kind, selected, request=None):
    cap = REQUEST_CAP if kind == "request" else RESPONSE_CAP
    need(type(raw) is bytes and 1 < len(raw) <= cap and raw[-1:] == b"\n" and b"\n" not in raw[:-1] and b"\r" not in raw)
    try:
        value = json.loads(raw[:-1].decode("ascii"), object_pairs_hook=unique_object)
    except (ValueError, UnicodeError, RecursionError):
        raise Invalid("strict single JSON frame")
    need(canonical(value) + b"\n" == raw)
    if kind == "request":
        checked_request(value, selected)
    else:
        checked_response(value, request, selected)
    return value


def load_inputs():
    catalog = json.loads((HERE / "input-catalog.json").read_bytes())
    loaded = {}
    identities = []
    for row in catalog["inputs"]:
        p = HERE / "inputs" / row["name"]
        need(not p.is_symlink() and p.is_file())
        raw = p.read_bytes()
        need(len(raw) == row["bytes"] and hashlib.sha256(raw).hexdigest() == row["sha256"])
        identities.append({"name": row["name"], "bytes": len(raw), "sha256": row["sha256"]})
        if row["name"].endswith(".json"):
            loaded[row["name"]] = json.loads(raw)
    return loaded, identities


def selected_fixture(prior):
    remote = clone(prior["remote"])
    c = remote["base"]
    local = c["expected_local"]
    rc = local["recipient"]["config"]
    identity = rc["identity"]
    # Only the new original View correlation/generation are modeled literals.
    # All account/config/recipient/profile bytes are prior independent inputs.
    view = dict(zip(VIEW_KEYS, [1, "3" * 64, rc["account_authority_id"], rc["account_id"], 1, rc["fixture"], identity["root_public_key"], identity["gym_public_key"], identity["gym_id"]]))
    raw = canonical(c["settlement"])
    sid = hashlib.sha256(b"SLUG operator pool settlement config\x00v2\x00" + struct.pack("<I", len(raw)) + raw).hexdigest()
    binding = dict(zip(BINDING_KEYS, [view, prior["remote_id"], c["miners"]["source_id"], sid, local["recipient"]["recipient"]["script"], prior["profile_id"], local["original_source_binding_id"], local["wallet_owner_id"], local["allocation_binding_id"]]))
    return {"remote": remote, "config_id": prior["remote_id"], "view": view, "binding": binding}


def request(selected, command, rid=2, sequence=0, capture=0, before=0, signed=True):
    view = selected["view"]["view_id"] if signed else ""
    fixture = selected["view"]["fixture"] if command == "login_fixture" else ""
    if command == "login_fixture":
        view = ""
    return dict(zip(REQUEST_KEYS, [1, APP, str(rid), command, selected["config_id"], view, fixture, str(sequence), str(capture), str(before)]))


def job(sequence=1, capture=0, before=0, state="working", error=""):
    return dict(zip(JOB_KEYS, [str(sequence), str(capture), str(before), state, error]))


def response(q, arm=None, value=None, error=""):
    r = {k: clone(q[k]) for k in RESPONSE_KEYS[:6]}
    r.update({"ok": error == "", "snapshot": None, "job": None, "read": None, "error": error})
    if arm is not None:
        r[arm] = clone(value)
    return r


def snapshot(selected, signed):
    view = selected["view"]
    state = "signed_in" if signed else "signed_out"
    account = dict(zip(ACCOUNT_KEYS, [state, view["fixture"], view["account_id"] if signed else "", view["generation"] if signed else 0, 0, False, False, ""]))
    remote = None
    if signed:
        base = selected["remote"]["base"]
        remote = dict(zip(REMOTE_KEYS, [2, selected["config_id"], clone(view), selected["binding"]["source_binding_id"], selected["binding"]["original_source_binding_id"], "stopped", "stopped", "", 0, 0, 0, 0, "unavailable", 0, 0, 0, 0, base["expected_local"]["recipient"]["recipient"]["address"]]))
    return dict(zip(SNAP_KEYS, [2, selected["config_id"], state, account, clone(view) if signed else None, remote, ""]))


def modeled_page(selected, high=18, before=19, revision=7, balance=None, items=None):
    b = selected["binding"]
    if balance is None:
        balance = dict(zip(BALANCE_KEYS, [b["view"]["gym_id"], {"numerator": "0", "denominator": "1"}, {"numerator": "0", "denominator": "1"}, "0", "0", ""]))
    done = before <= 9
    return dict(zip(PAGE_KEYS, [2, b["source_binding_id"], b["settlement_config_id"], str(revision), str(high), str(before), "0" if done else str(before - 8), done, b["recipient_script"], clone(balance), 1736294401000, [] if items is None else clone(items)]))


def capture(selected, first, current=None, sequence=1, stale=False):
    return dict(zip(CAPTURE_KEYS, [str(sequence), clone(selected["binding"]), 1736294401001, stale, clone(first), clone(first if current is None else current)]))


def next_window(c, sequence, before):
    if c is None or sequence == 0 or str(sequence) != c["sequence"] or before == 0:
        return "not_found"
    for which in ["first", "current"]:
        if int(c[which]["before_batch"]) == before:
            return "cached_" + which
    if c["stale"]:
        return "stale_capture"
    if c["current"]["done"] or int(c["current"]["next_before"]) != before:
        return "invalid_request"
    return "remote_exact_next"


def replace(value, path, item):
    out = clone(value)
    node = out
    for key in path[:-1]:
        node = node[key]
    node[path[-1]] = item
    return out


def generate():
    inputs, identities = load_inputs()
    selected = selected_fixture(inputs["selection-reference.json"])
    daily = inputs["daily-reference.json"]
    cases, targets = [], []

    def case(name, kind, valid, value=None, q=None, raw=None, selection=None, scope="conditional ordered codec and selected binding only"):
        chosen = selected if selection is None else selection
        frame = canonical(value) + b"\n" if raw is None else raw
        try:
            parse_frame(frame, kind, chosen, q)
            accepted = True
        except Invalid:
            accepted = False
        if accepted != valid:
            raise Invalid("independent-case-classification:" + name)
        row = {"name": name, "kind": kind, "valid": valid, "scope": scope, "frame_hex": frame.hex(), "frame_bytes": len(frame), "sha256": hashlib.sha256(frame).hexdigest()}
        if q is not None:
            row["request"] = clone(q)
        if selection is not None:
            row["selection_override"] = clone(selection)
        cases.append(row)
        return frame

    def positive(name, kind, value, q=None, selection=None):
        raw = case(name, kind, True, value, q, selection=selection)
        targets.append((name, kind, raw, q, selection))

    first_request = request(selected, "snapshot", rid=1, signed=False)
    positive("initial-snapshot", "request", first_request)
    for command in CONTROLS:
        q = request(selected, command)
        positive("control-" + command, "request", q)
        snap = snapshot(selected, command != "logout")
        positive("control-" + command + "-result", "response", response(q, "snapshot", snap), q)
    positive("initial-signed-out-result", "response", response(first_request, "snapshot", snapshot(selected, False)), first_request)
    max_selected = clone(selected)
    max_selected["view"]["generation"] = SAFE
    max_selected["binding"]["view"]["generation"] = SAFE
    max_snapshot = snapshot(max_selected, True)
    max_snapshot["account"]["hold_until_ms"] = SAFE
    max_snapshot["account"]["held"] = True
    max_snapshot["remote"].update({"state": "running", "worker_state": "delivering", "last_error": "cleanup_incomplete", "attempts": SAFE, "hashes_per_second_milli": SAFE, "records": selected["remote"]["base"]["max_records"], "pending": selected["remote"]["base"]["max_records"], "wallet_state": "ready", "wallet_height": 2147483647, "spendable_atoms": MONEY, "immature_atoms": MONEY, "unconfirmed_atoms": MONEY})
    max_snapshot_q = request(max_selected, "snapshot", rid=U64)
    max_snapshot_frame = case("safe-numeric-snapshot-widths", "response", True, response(max_snapshot_q, "snapshot", max_snapshot), max_snapshot_q, selection=max_selected, scope="conditional scalar maxima; not a reachable account/wallet balance assertion")
    for command in READS:
        q = request(selected, command, rid=U64, sequence=U64)
        positive("max-correlation-" + command, "request", q)
    q_begin = request(selected, "daily_payout_begin", sequence=1)
    positive("fresh-begin", "request", q_begin)
    positive("begin-working", "response", response(q_begin, "job", job()), q_begin)
    q_continue = request(selected, "daily_payout_begin", sequence=2, capture=1, before=11)
    positive("exact-continuation", "request", q_continue)
    positive("continuation-working", "response", response(q_continue, "job", job(2, 1, 11)), q_continue)
    for error in ERRORS:
        case("closed-refusal-" + error, "response", True, response(q_begin, error=error), q_begin)

    # Sparse global windows intentionally match no payments: empty is not EOF.
    first = modeled_page(selected)
    middle = modeled_page(selected, before=11)
    terminal = modeled_page(selected, before=3)
    cap = capture(selected, first)
    q_read = request(selected, "daily_payout_read", sequence=1)
    read = {"job": job(1, state="done"), "capture": cap}
    base_response = response(q_read, "read", read)
    positive("fresh-empty-nonfinal", "response", base_response, q_read)
    for name, current, before in [("middle-empty-nonfinal", middle, 11), ("terminal-empty-done", terminal, 3), ("cached-first-keeps-current", terminal, 19)]:
        q = request(selected, "daily_payout_read", sequence=2)
        value = {"job": job(2, 1, before, "done"), "capture": capture(selected, first, current)}
        positive(name, "response", response(q, "read", value), q)
    for state, error in [("working", ""), ("cancelled", "")] + [("failed", x) for x in JOB_ERRORS]:
        q = request(selected, "daily_payout_read", sequence=2)
        c = capture(selected, first, stale=error == "stale_capture")
        value = {"job": job(2, 1, 11, state, error), "capture": c}
        case("retained-old-capture-" + state + "-" + (error or "none"), "response", True, response(q, "read", value), q)
    q_cancel = request(selected, "daily_payout_cancel", sequence=1)
    positive("completed-cancel-keeps-capture", "response", response(q_cancel, "read", read), q_cancel)
    positive("working-no-capture", "response", response(q_read, "read", {"job": job(), "capture": None}), q_read)
    stale_cached_q = request(selected, "daily_payout_read", sequence=3)
    stale_cached = {"job": job(3, 1, 19, "done"), "capture": capture(selected, first, middle, stale=True)}
    positive("completed-cached-stale-first-window", "response", response(stale_cached_q, "read", stale_cached), stale_cached_q)

    # Inherited independently modeled money values are rebound to the selected
    # gym only. This is conditional shape, not evidence these states exist.
    money_names = ["paid-9007199254740991", "paid-9007199254740992", "paid-9223372036854775807", "reserved-9223372036854775807", "four-4096-digit-components"]
    for name in money_names:
        inherited = next(x for x in daily["payments"]["balances"] if x["name"] == name and x["valid"])
        balance = clone(inherited["value"]["balance"])
        balance["gym_id"] = selected["view"]["gym_id"]
        page = modeled_page(selected, high=0, before=1, revision=U64, balance=balance)
        value = {"job": job(U64, state="done"), "capture": capture(selected, page, sequence=U64)}
        q = request(selected, "daily_payout_read", rid=U64, sequence=U64)
        positive("lossless-" + name, "response", response(q, "read", value), q)

    inherited_eight = next(x for x in daily["payments"]["pages"] if x["name"] == "slots-8" and x["valid"])
    eight_balance = clone(inherited_eight["value"]["balance"])
    eight_balance["gym_id"] = selected["view"]["gym_id"]
    eight = modeled_page(selected, high=8, before=9, balance=eight_balance, items=inherited_eight["value"]["items"])
    positive("eight-inherited-payment-items", "response", response(q_read, "read", {"job": job(1, state="done"), "capture": capture(selected, eight)}), q_read)
    zero_txid_page = clone(eight)
    zero_txid_page["items"][0]["payment"]["txid"] = "0" * 64
    case("conditional-original-zero-txid-shape", "response", True, response(q_read, "read", {"job": job(1, state="done"), "capture": capture(selected, zero_txid_page)}), q_read, scope="Original hexBytes(32) shape permits zero; not a reachable or authenticated transaction claim")

    for field in ["id", "sequence", "capture_sequence", "before_batch"]:
        for label, invalid in [("numeric", 1), ("boolean", True), ("null", None), ("leading-zero", "01"), ("negative", "-1"), ("plus", "+1"), ("exponent", "1e0"), ("decimal", "1.0"), ("overflow", str(U64 + 1))]:
            case("request-" + field + "-" + label, "request", False, replace(q_continue, [field], invalid))
    for name, path, invalid in [
        ("wrong-family", ["application"], "SLUG-REMOTE-POOL-1"),
        ("wrong-version", ["version"], 2),
        ("boolean-version", ["version"], True),
        ("wrong-config", ["config_id"], "4" * 64),
        ("wrong-view", ["view_id"], "5" * 64),
        ("zero-sequence", ["sequence"], "0"),
        ("future-capture", ["capture_sequence"], "2"),
        ("mixed-zero-window", ["before_batch"], "0"),
        ("unknown-command", ["command"], "call_arbitrary"),
        ("unexpected-fixture", ["fixture"], selected["view"]["fixture"]),
    ]:
        case("request-" + name, "request", False, replace(q_continue, path, invalid))
    case("initial-must-snapshot", "request", False, replace(q_begin, ["id"], "1"))
    missing = clone(q_begin)
    del missing["fixture"]
    case("request-missing-required-field", "request", False, missing)
    case("request-float-version", "request", False, replace(q_begin, ["version"], 1.0))

    for name, path, invalid in [
        ("wrong-id", ["id"], "3"),
        ("numeric-id", ["id"], 2),
        ("wrong-command", ["command"], "daily_payout_cancel"),
        ("wrong-view", ["view_id"], "7" * 64),
        ("boolean-ok", ["ok"], 1),
        ("nonnull-wrong-arm", ["job"], job()),
        ("success-error", ["error"], "busy"),
        ("zero-capture", ["read", "capture", "sequence"], "0"),
        ("adopt-future-capture", ["read", "capture", "sequence"], "2"),
        ("numeric-capture", ["read", "capture", "sequence"], 1),
        ("wrong-binding", ["read", "capture", "binding", "wallet_owner_id"], "9" * 64),
        ("different-view", ["read", "capture", "binding", "view", "view_id"], "8" * 64),
        ("numeric-paid", ["read", "capture", "current", "balance", "paid_atoms"], 0),
        ("overflow-paid", ["read", "capture", "current", "balance", "paid_atoms"], str(I64 + 1)),
        ("leading-zero-fraction", ["read", "capture", "current", "balance", "confirmed", "numerator"], "00"),
        ("too-long-fraction", ["read", "capture", "current", "balance", "confirmed", "numerator"], "1" * 4097),
        ("zero-denominator", ["read", "capture", "current", "balance", "confirmed", "denominator"], "0"),
        ("unreduced-fraction", ["read", "capture", "current", "balance", "confirmed"], {"numerator": "0", "denominator": "2"}),
        ("header-revision", ["read", "capture", "current", "revision"], "8"),
        ("header-observation", ["read", "capture", "current", "observed_at_ms"], 1736294401001),
        ("premature-empty-eof", ["read", "capture", "first", "done"], True),
        ("null-items", ["read", "capture", "first", "items"], None),
        ("completed-stale", ["read", "capture", "stale"], True),
    ]:
        case("response-" + name, "response", False, replace(base_response, path, invalid), q_read)
    # A standalone valid remote cursor still cannot form an unreachable native
    # first/current pair. Both pages individually pass their shape checks.
    off_grid = clone(base_response)
    off_grid["read"]["capture"]["current"] = modeled_page(selected, before=10)
    case("response-current-off-native-grid", "response", False, off_grid, q_read)
    same_cursor_changed = clone(base_response)
    same_cursor_changed["read"]["capture"]["current"]["balance"]["paid_atoms"] = "1"
    case("response-same-cursor-different-page", "response", False, same_cursor_changed, q_read)
    failed = response(request(selected, "daily_payout_read", sequence=2), "read", {"job": job(2, 1, 11, "failed", "busy"), "capture": capture(selected, first, sequence=2)})
    case("response-failed-job-cannot-adopt-capture", "response", False, failed, request(selected, "daily_payout_read", sequence=2))
    case("response-required-read-null", "response", False, replace(base_response, ["read"], None), q_read)
    case("response-view-generation-unsafe", "response", False, replace(base_response, ["read", "capture", "binding", "view", "generation"], SAFE + 1), q_read)
    case("response-float-observation", "response", False, replace(base_response, ["read", "capture", "current", "observed_at_ms"], 1736294401000.0), q_read)

    # Strict framing controls are applied to representative complete requests,
    # control responses and capture responses. Raw duplicates are never erased
    # by a map before expected validity is checked.
    framing_targets = [targets[0], next(x for x in targets if x[0] == "initial-signed-out-result"), next(x for x in targets if x[0] == "fresh-empty-nonfinal")]
    for name, kind, raw, q, override in framing_targets:
        body = raw[:-1]
        value = json.loads(body)
        keys = list(value)
        reorder = {k: value[k] for k in [keys[1], keys[0]] + keys[2:]}
        variants = {
            "no-lf": body, "double-lf": raw + b"\n", "crlf": body + b"\r\n",
            "leading-space": b" " + raw, "trailing-space": body + b" \n",
            "second-json": body + b"{}\n", "duplicate": body[:1] + b'"version":1,' + body[1:] + b"\n",
            "field-order": canonical(reorder) + b"\n",
            "unknown-field": body[:-1] + b',"extra":null}\n',
            "escaped-key": raw.replace(b'"version"', b'"\\u0076ersion"', 1),
            "non-ascii": body[:-1] + ',"extra":"é"}\n'.encode("utf-8"),
        }
        for label, candidate in variants.items():
            case(name + "-framing-" + label, kind, False, q=q, raw=candidate, selection=override)
    reordered_binding = clone(base_response)
    b = reordered_binding["read"]["capture"]["binding"]
    reordered_binding["read"]["capture"]["binding"] = {k: b[k] for k in list(b)[1:] + list(b)[:1]}
    case("response-nested-binding-order", "response", False, reordered_binding, q_read)
    reordered_fraction = clone(base_response)
    reordered_fraction["read"]["capture"]["current"]["balance"]["confirmed"] = {"denominator": "1", "numerator": "0"}
    case("response-nested-fraction-order", "response", False, reordered_fraction, q_read)

    # Exact arithmetic proof uses disjoint validated maximum component caps;
    # literal wide frames illustrate valid widths, not attainable ledger state.
    large = clone(next(x for x in daily["payments"]["balances"] if x["name"] == "four-4096-digit-components")["value"]["balance"])
    large["gym_id"] = selected["view"]["gym_id"]
    large["paid_atoms"] = str(I64)
    wide_first = modeled_page(selected, high=8, before=9, revision=U64, balance=large, items=inherited_eight["value"]["items"])
    wide_read = {"job": job(U64, state="done"), "capture": capture(selected, wide_first, sequence=U64)}
    wide_q = request(selected, "daily_payout_read", rid=U64, sequence=U64)
    wide_r = response(wide_q, "read", wide_read)
    wide_raw = case("wide-valid-two-pages-four-4096-components", "response", True, wide_r, wide_q, scope="conditional codec maxima only; repeated first/current page, no ledger attainability claim")
    scalar = clone(wide_first)
    scalar["balance"], scalar["items"] = None, []
    scalar.update({"high_batch": "32767", "before_batch": "32768", "next_before": "32767", "done": False, "observed_at_ms": MAX_TIME})
    scalar_bytes = len(canonical(scalar)) - len(b"null")
    wrapper = {"batch_sequence": "32767", "payment": None}
    wrapper_bytes = len(canonical(wrapper)) - len(b"null") + 1  # include separating comma
    page_bound = scalar_bytes + 17408 + 8 * (256 + wrapper_bytes)
    envelope = clone(wide_r)
    envelope["read"]["capture"]["first"] = None
    envelope["read"]["capture"]["current"] = None
    envelope["read"]["capture"]["binding"]["view"]["generation"] = SAFE
    envelope["read"]["capture"]["captured_at_ms"] = MAX_TIME
    envelope["read"]["job"].update({"capture_sequence": str(U64), "before_batch": "32768", "state": "cancelled", "error": "stale_capture"})
    envelope_bytes = len(canonical(envelope)) + 1 - 2 * len(b"null")
    # The artificial simultaneous maxima here are disjoint byte components,
    # not an accepted typed state (e.g. cancelled with a nonempty error).
    # 16KiB covers either the literal safe-numeric snapshot or the read envelope.
    total_bound = 2 * page_bound + 16384
    need(scalar_bytes < 1024 and wrapper_bytes < 64 and envelope_bytes < 16384 and len(max_snapshot_frame) < 16384 and total_bound < RESPONSE_CAP)
    # Bounds are full encoded frames including LF, before any parse/allocation.
    case("request-over-cap-before-parse", "request", False, q=None, raw=b" " * REQUEST_CAP + b"\n")
    case("response-over-cap-before-parse", "response", False, q=q_read, raw=b" " * RESPONSE_CAP + b"\n")

    continuation_cases = []
    current_middle = capture(selected, first, middle)
    current_end = capture(selected, first, terminal)
    stale_middle = clone(current_middle)
    stale_middle["stale"] = True
    for name, value, seq, before, expected in [
        ("fresh-cached-first", cap, 1, 19, "cached_first"),
        ("empty-first-is-not-eof", cap, 1, 11, "remote_exact_next"),
        ("cached-current", current_middle, 1, 11, "cached_current"),
        ("cached-first-after-current", current_middle, 1, 19, "cached_first"),
        ("next-after-empty-current", current_middle, 1, 3, "remote_exact_next"),
        ("skip-window", cap, 1, 3, "invalid_request"),
        ("stale-keeps-first", stale_middle, 1, 19, "cached_first"),
        ("stale-forbids-next", stale_middle, 1, 3, "stale_capture"),
        ("old-capture-sequence", current_middle, 2, 3, "not_found"),
        ("zero-capture-sequence", current_middle, 0, 3, "not_found"),
        ("done-has-no-next", current_end, 1, 1, "invalid_request"),
    ]:
        need(next_window(value, seq, before) == expected)
        continuation_cases.append({"name": name, "scope": "conditional retained-two-window correlation only", "capture": clone(value), "capture_sequence": str(seq), "before_batch": str(before), "expected": expected})

    # These declarations are hand-tallied from the fixed loops above. A source
    # successor that changes coverage must deliberately update them before use.
    need(len(cases) == 170 and sum(x["valid"] for x in cases) == 55 and len(continuation_cases) == 11)

    model = {
        "version": 1,
        "scope": "Independent conditional strict codec/correlation model; no live authority, owner lifecycle, cryptography, node or financial state claim.",
        "inputs": identities,
        "selection": selected,
        "cases": cases,
        "continuations": continuation_cases,
        "counts": {"cases": len(cases), "valid": sum(x["valid"] for x in cases), "invalid": sum(not x["valid"] for x in cases), "continuations": len(continuation_cases), "sequence_model": 5},
        "bounds": {"scope": "Disjoint original validated component byte caps, not a claimed maximum attainable ledger state", "request_cap_including_lf": REQUEST_CAP, "response_cap_including_lf": RESPONSE_CAP, "fraction_component_digits": 4096, "balance_cap": 17408, "payment_item_cap": 256, "page_scalar_bytes": scalar_bytes, "item_wrapper_and_comma_bytes": wrapper_bytes, "page_conservative_bytes": page_bound, "envelope_binding_job_capture_bytes": envelope_bytes, "snapshot_literal_bytes_including_lf": len(max_snapshot_frame), "envelope_or_snapshot_allowance": 16384, "response_conservative_bytes_including_lf": total_bound, "wide_literal_bytes_including_lf": len(wide_raw)},
        "sequence_model": [
            {"name": "first-request", "last_id": "0", "request_id": "1", "command": "snapshot", "valid": True},
            {"name": "duplicate-request", "last_id": "1", "request_id": "1", "command": "snapshot", "valid": False},
            {"name": "increasing-gap", "last_id": "1", "request_id": "3", "command": "snapshot", "valid": True},
            {"name": "max-id", "last_id": str(U64 - 1), "request_id": str(U64), "command": "snapshot", "valid": True},
            {"name": "no-wrap", "last_id": str(U64), "request_id": "1", "command": "snapshot", "valid": False},
        ],
        "exclusions": ["No arbitrary remote or profile input is accepted as runtime authority.", "No live account/View/credential lease, helper dispatch, cancellation or close is simulated as proof.", "No expected bytes are learned from Go output or active implementation.", "No renderer, UI, blocked-response, nonterminal live history or real wallet readiness qualification."]
    }
    need(len(canonical(model)) <= 16 * 1024 * 1024)
    return model


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--lock", required=True)
    parser.add_argument("--output", required=True)
    args = parser.parse_args()
    lock = json.loads(Path(args.lock).read_bytes())
    need(lock.get("execution_authorized") is True and lock.get("source_root") == str(HERE))
    rows = lock["files"]
    need(type(rows) is list and len(rows) >= 15)
    observed = {}
    for row in rows:
        p = Path(row["path"])
        need(p.is_absolute() and str(p) == str(p.resolve()) and p.is_file() and not p.is_symlink())
        raw = p.read_bytes()
        need(len(raw) == row["bytes"] and hashlib.sha256(raw).hexdigest() == row["sha256"])
        observed[str(p)] = (len(raw), row["sha256"])
    local = {str(p) for p in HERE.rglob("*") if p.is_file() and p.name not in ["source-ready.json", "producer-lock.proposed.json"]}
    need(local == set(observed))
    out = Path(args.output)
    need(out.is_absolute() and str(out.parent.resolve()) == str(out.parent) and HERE not in out.parents and not out.exists())
    result = canonical(generate()) + b"\n"
    for name, expected in observed.items():
        raw = Path(name).read_bytes()
        need((len(raw), hashlib.sha256(raw).hexdigest()) == expected)
    fd = os.open(str(out), os.O_WRONLY | os.O_CREAT | os.O_EXCL | getattr(os, "O_NOFOLLOW", 0), 0o600)
    with os.fdopen(fd, "wb") as stream:
        stream.write(result)
        stream.flush()
        os.fsync(stream.fileno())


if __name__ == "__main__":
    main()
