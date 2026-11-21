"""Small public-payload helpers for the unexecuted selected WIRE2 reference.

This module does not create transactions, ownership, a wallet or a payout.
The three inputs are separately qualified public wire corpora.  Original
canonical payload values are carried forward only as conditional codec cases.
New envelope/correlation rules belong to the selected contract, not this file.
"""

import copy
import hashlib
import json
import re


MAX_MONEY = 2100000000000000
MAX_U64 = 18446744073709551615
MAX_TIME = 253402300799999
MAX_HEIGHT = 2147483647
MAX_ACTIVITY = 256
MAX_PAGE = 8


def require(ok, message):
    if not ok:
        raise ValueError(message)


def canonical(value):
    # All inherited protocol strings are ASCII. Match the existing Go JSON
    # string escaping too, without calling its encoder or reading its output.
    text = json.dumps(value, ensure_ascii=False, separators=(",", ":"),
                      allow_nan=False)
    for old, new in (("&", "\\u0026"), ("<", "\\u003c"), (">", "\\u003e"),
                     ("\u2028", "\\u2028"), ("\u2029", "\\u2029")):
        text = text.replace(old, new)
    return text.encode("utf-8")


def exact_fields(value, fields):
    return type(value) is dict and list(value) == list(fields)


def integer(value, low, high):
    # bool is an int subclass; JSON booleans are never protocol integers.
    return type(value) is int and low <= value <= high


def lower_hex(value, size=64, nonzero=False):
    return (type(value) is str and len(value) == size and
            re.fullmatch(r"[0-9a-f]+", value) is not None and
            (not nonzero or value != "0" * size))


def unsigned_decimal(value, maximum=MAX_U64):
    return (type(value) is str and len(value) <= len(str(maximum)) and
            re.fullmatch(r"0|[1-9][0-9]*", value) is not None and
            int(value) <= maximum)


def signed_decimal(value, maximum=MAX_MONEY):
    return (type(value) is str and len(value) <= len(str(maximum)) + 1 and
            re.fullmatch(r"0|-?[1-9][0-9]*", value) is not None and
            -maximum <= int(value) <= maximum)


def slug_text(atoms):
    require(integer(atoms, -MAX_MONEY, MAX_MONEY), "bounded integer atoms")
    whole, fraction = divmod(abs(atoms), 100000000)
    return ("-" if atoms < 0 else "") + str(whole) + "." + str(fraction).zfill(8)


def inherited_response(corpus, name, arm):
    rows = [row for row in corpus["positives"] if row["name"] == name]
    require(len(rows) == 1, "one inherited case: " + name)
    row = rows[0]
    value = row["value"]
    frame = row["frame"].encode("utf-8")
    require(row["kind"] == "response" and value["ok"] is True,
            "inherited successful response: " + name)
    require(canonical(value) + b"\n" == frame and
            hashlib.sha256(frame).hexdigest() == row["sha256"],
            "original canonical frame: " + name)
    require(value.get(arm) is not None, "inherited result arm: " + name)
    return copy.deepcopy(value[arm])


def inherited_daily_response(corpus, name):
    rows = [row for row in corpus["cases"] if row["name"] == name]
    require(len(rows) == 1, "one inherited DAILY case: " + name)
    row = rows[0]
    require(row["valid"] is True and row["kind"] == "response",
            "valid inherited DAILY response: " + name)
    frame = bytes.fromhex(row["frame_hex"])
    require(len(frame) == row["frame_bytes"] and
            hashlib.sha256(frame).hexdigest() == row["sha256"] and
            frame.endswith(b"\n"), "inherited DAILY frame: " + name)
    value = json.loads(frame)
    require(canonical(value) + b"\n" == frame,
            "inherited DAILY canonical frame: " + name)
    return value


def window(count, after, limit):
    """Set70 local captured positions, unrelated to global pool batch slots."""
    require(integer(count, 0, MAX_ACTIVITY) and integer(after, 0, count) and
            integer(limit, 1, MAX_PAGE), "bounded captured activity position")
    length = min(limit, count - after)
    next_after = after + length
    return {"length": length, "next": next_after, "has_more": next_after < count}


def retained_payment(capture, capture_sequence, batch_sequence, txid, output_index):
    """Conditional exact retained DAILY membership, without any wallet lookup.

    Initial First and Current are normally the same window. Equal copies of a
    batch are one row, not two candidates; a conflicting copy fails closed.
    No caller-provided amount or recipient is accepted. The caller supplies an
    already validated immutable capture as a premise, not an authority getter.
    """
    require(unsigned_decimal(capture_sequence) and capture_sequence != "0" and
            capture["sequence"] == capture_sequence,
            "exact retained capture sequence")
    require(unsigned_decimal(batch_sequence) and batch_sequence != "0" and
            lower_hex(txid) and integer(output_index, 0, 255),
            "exact payout selector shape")
    by_batch = {}
    by_output = {}
    for page in (capture["first"], capture["current"]):
        for item in page["items"]:
            batch = item["batch_sequence"]
            payment = item["payment"]
            encoded = canonical(item)
            if batch in by_batch:
                require(by_batch[batch][0] == encoded,
                        "conflicting retained copies of one batch")
            else:
                outpoint = (payment["txid"], payment["output_index"])
                require(outpoint not in by_output,
                        "one retained batch per payment output")
                by_batch[batch] = (encoded, item)
                by_output[outpoint] = batch
    require(batch_sequence in by_batch, "selected batch is retained")
    item = by_batch[batch_sequence][1]
    payment = item["payment"]
    require(payment["txid"] == txid and payment["output_index"] == output_index,
            "selected retained transaction and output")
    return copy.deepcopy(item)


def inherited_amounts(activity, payout):
    rows = {}
    for family, values, key in (("Set70", activity["atom_formats"], "slug"),
                                ("Set67", payout["atom_formats"], "text")):
        for value in values:
            atoms = value["atoms"]
            text = slug_text(atoms)
            require(text == value[key], "qualified amount spelling: " + family)
            if atoms not in rows:
                rows[atoms] = {"atoms": atoms, "text": text, "families": []}
            rows[atoms]["families"].append(family)
    return [rows[atoms] for atoms in sorted(rows)]
