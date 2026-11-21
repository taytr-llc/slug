"""Set80 section 6 conditional arithmetic; no product imports or I/O.

This models codec lengths and audited-count premises, not valid jobs, storage
sync, authentication, crypto, or concurrency. Full namespace validation must
precede these capacity decisions. Root alone executes the enclosing producer.
"""

import json


U64 = (1 << 64) - 1
TMAX = 16 << 20
JOB = 8192
ROW = 4096
KEY = len(b"issue/r/") + 32
JOB_KEY = len(b"job:") + 64
RECEIPT = 4096
PUBLIC_META = 8192
ADMISSION = 69632
SMALL = 8192
RESPONSE = TMAX + (64 << 10)
OUTCOME = 40 << 20


def _json(value):
    return json.dumps(value, ensure_ascii=True, separators=(",", ":")).encode("ascii")


def _u(value):
    if type(value) is not int or value < 0 or value > U64:
        raise ValueError("malformed_uint64")
    return value


def _add(*values):
    n = 0
    for value in values:
        value = _u(value)
        if value > U64 - n:
            raise ValueError("uint64_overflow")
        n += value
    return n


def _sub(a, b):
    a, b = _u(a), _u(b)
    if b > a:
        raise ValueError("uint64_underflow")
    return a - b


def _mul(a, b):
    a, b = _u(a), _u(b)
    if a and b > U64 // a:
        raise ValueError("uint64_overflow")
    return a * b


def _bounded(value, low, high):
    value = _u(value)
    if not low <= value <= high:
        raise ValueError("outside_selected_bound")
    return value


def _fit(required, maximum):
    return "fits" if _u(required) <= _u(maximum) else "capacity"


def _source(template_bytes, pending_row_bytes, base_bytes, maximum, submissions, ceiling):
    """One pending request; Base.Bytes already includes old receipts/works."""
    t = _bounded(template_bytes, 1, TMAX)
    row = _bounded(pending_row_bytes, 1, ROW)
    _bounded(ceiling, 1, 4095)
    _bounded(submissions, 0, 16384)
    _bounded(maximum, 1, 8 << 30)
    pending = _add(KEY, row, KEY, t)
    work = _add(t, JOB, len(b'{"job":,"template":}'))
    # Contract W(T) uses a conservative 20-byte frame, not actual Job bytes.
    work_bound = _add(t, JOB, 20)
    completed = _add(KEY, ROW, KEY, 32, work_bound)
    unused = _add(KEY, ROW, KEY, t, KEY, 32)
    sealed = _add(KEY, ROW, KEY, t)
    reserve = max(0, completed - pending)
    effective = _add(base_bytes, pending, reserve)
    receipt_slots = _add(1, max(0, ceiling - submissions))
    holdback = _mul(receipt_slots, RECEIPT)
    required = _add(effective, holdback)
    return {"pending_issue_bytes": pending, "work_bound": work_bound,
            "literal_work_frame_bound": work, "completed_issue_and_work_bytes": completed,
            "unused_issue_bytes": unused, "sealed_issue_bytes": sealed,
            "pending_reserved": reserve, "effective_bytes": effective,
            "receipt_slots": receipt_slots, "receipt_holdback": holdback,
            "required_bytes": required, "selected_maximum": maximum,
            "classification": _fit(required, maximum),
            "completion_growth_covered": completed <= pending + reserve,
            "unused_growth_covered": unused <= pending + reserve,
            "seal_growth_covered": sealed <= pending + reserve}


def _registry(max_requests, requests, actual_bytes, manifest_bytes,
              public_bytes, accepted, signed, maximum):
    p = _bounded(max_requests, 1, 8192)
    r = _bounded(requests, 0, p)
    m = _bounded(manifest_bytes, 1, PUBLIC_META)
    b = _bounded(public_bytes, 1, PUBLIC_META)
    _bounded(accepted, 0, 1000)
    _bounded(signed, 0, accepted)
    _bounded(maximum, 1, 256 << 20)
    slot = _add(KEY, ROW, KEY, 32, JOB_KEY, JOB)
    slots = _mul(_sub(p, r), slot)
    metadata = _add(_sub(PUBLIC_META, m), _sub(PUBLIC_META, b))
    unsigned = _mul(_sub(accepted, signed), _add(ADMISSION, SMALL))
    required = _add(actual_bytes, slots, metadata, unsigned)
    return {"slot_bound": slot, "remaining_request_slots": p-r,
            "request_holdback": slots, "metadata_growth": metadata,
            "unchanged_unsigned_admission_reserve": unsigned,
            "required_bytes": required, "selected_maximum": maximum,
            "classification": _fit(required, maximum)}


def _count_chain(works, jobs, issuer, source, maximum, matched, matched_issued):
    values = [_u(v) for v in (works, jobs, issuer, source, maximum, matched, matched_issued)]
    if not 1 <= maximum <= 8192:
        raise ValueError("outside_selected_bound")
    return {"counts": values, "classification": "fits" if
            works <= jobs <= issuer <= source <= maximum and
            matched == issuer and matched_issued == jobs else "corrupt"}


def _source_many(count, template_bytes, row_bytes, maximum):
    _bounded(count, 1, 8192)
    one = _source(template_bytes, row_bytes, 0, 8 << 30, 4095, 4095)
    effective = _mul(count, one["effective_bytes"])
    # Receipt holdback belongs to the source, not each pending request.
    required = _add(effective, RECEIPT)
    return {"pending_requests": count, "effective_bytes": effective,
            "one_shared_canonical_holdback": RECEIPT, "required_bytes": required,
            "classification": _fit(required, _bounded(maximum, 1, 8 << 30))}


def _outbox(records, requests, max_records, max_requests, old_bytes, old_reserved, maximum):
    _bounded(max_records, 1, 4096)
    _bounded(max_requests, 1, 8192)
    _bounded(maximum, 64 << 20, 1 << 30)
    _u(records)
    _u(requests)
    total = _add(records, requests)
    if total > max_records or requests > max_requests:
        raise ValueError("corrupt_retained_counts")
    request_id = "1" * 64  # only canonical string width, not an issued ID
    q = len(_json({"version": 1, "request": {"request_id": request_id}, "result": None}))
    scan = len(_json({"version": 1, "request_id": request_id})) + 8
    q_bound = _add(_sub(q, len(b"null")), RESPONSE)
    active_reserve = _add(_sub(q_bound, q), scan, OUTCOME)
    required = _add(old_bytes, old_reserved, q, active_reserve)
    required_slots = _add(total, 2)
    can_start = requests < max_requests and required_slots <= max_records
    return {"null_request_payload": q, "max_request_payload": q_bound,
            "scan_and_cursor_payload": scan, "active_extra_reserve": active_reserve,
            "required_bytes": required, "required_slots": required_slots,
            "selected_maximum": maximum,
            "classification": "fits" if can_start and required <= maximum else "capacity"}


def _case(name, function, *args):
    result = {"name": name, "inputs": list(args)}
    try:
        result["expected"] = function(*args)
    except ValueError as error:
        result["expected"] = {"classification": str(error)}
    return result


def produce_bounds(fixtures):
    """Return finite arithmetic cases only; fixtures deliberately unused."""
    source = []
    for name, t, row, base, n, ceiling in (
            ("small-pending", 1024, 512, 0, 0, 2),
            ("maximum-template-and-receipts", TMAX, ROW, 0, 0, 4095),
            ("full-ordinary-keeps-one-canonical", TMAX, ROW, 4096, 4095, 4095),
            ("past-ordinary-keeps-one-canonical", TMAX, ROW, 8192, 4096, 4095)):
        inputs = (t, row, base, 8 << 30, n, ceiling)
        required = _source(*inputs)["required_bytes"]
        for suffix, maximum in (("exact", required), ("one-byte-below", required-1)):
            source.append(_case(name+"-"+suffix, _source, t, row, base, maximum, n, ceiling))
    source.extend([
        _case("maximum-request-count-does-not-imply-bytes-fit", _source_many, 8192, TMAX, ROW, 8 << 30),
        _case("shared-not-per-request-receipt-reserve", _source_many, 2, 1024, 512, 64 << 20),
        _case("malformed-row-too-large", _source, 1024, ROW+1, 0, 64 << 20, 0, 2),
        _case("malformed-template-too-large", _source, TMAX+1, 512, 0, 64 << 20, 0, 2),
        _case("overflow-before-capacity", _source, 1024, 512, U64, 64 << 20, 0, 2),
    ])

    registry = []
    for name, p, r, actual, m, b, accepted, signed in (
            ("fresh-maximum", 8192, 0, 16384, 1024, 1024, 0, 0),
            ("with-unsigned-admissions", 8192, 1, 32768, 8191, 8191, 1000, 0),
            ("all-slots-consumed", 8192, 8192, 200 << 20, 8192, 8192, 1000, 999)):
        required = _registry(p, r, actual, m, b, accepted, signed, 256 << 20)["required_bytes"]
        for suffix, maximum in (("exact", required), ("one-byte-below", required-1)):
            registry.append(_case(name+"-"+suffix, _registry, p, r, actual, m, b, accepted, signed, maximum))
    registry.extend([
        _case("requests-exceed-profile", _registry, 8, 9, 0, 512, 512, 0, 0, 256 << 20),
        _case("metadata-too-large", _registry, 8, 0, 0, 8193, 512, 0, 0, 256 << 20),
        _case("signed-exceeds-accepted", _registry, 8, 0, 0, 512, 512, 1, 2, 256 << 20),
        _case("overflow-before-capacity", _registry, 8, 0, U64, 512, 512, 0, 0, 256 << 20),
    ])

    # The exact slot maximum is released once; a seal charges only its row.
    slot = _add(KEY, ROW, KEY, 32, JOB_KEY, JOB)
    conversions = []
    for state, actual in (("issued", slot), ("sealed", KEY+ROW)):
        before = _mul(8, slot)
        after = _add(actual, _mul(7, slot))
        conversions.append({"state": state, "before_slots_bytes": before,
                            "after_actual_plus_slots_bytes": after,
                            "released_bytes": _sub(before, after),
                            "replay_delta_bytes": 0, "replay_delta_requests": 0})

    outbox = []
    first = _outbox(0, 0, 2, 8, 0, 0, 1 << 30)
    # Keep the independently selected minimum 64MiB even at a byte boundary.
    old = (64 << 20) - first["required_bytes"] + 1
    for name, maximum in (("exact-bytes", (64 << 20)+1), ("one-byte-below", 64 << 20)):
        outbox.append(_case(name, _outbox, 0, 0, 2, 8, old, 0, maximum))
    for name, r, q, mr, mq in (
            ("exact-two-slots", 0, 0, 2, 8),
            ("one-slot-cannot-start", 0, 0, 1, 8),
            ("last-two-shared-slots", 2047, 2047, 4096, 8192),
            ("one-shared-slot-left", 2048, 2047, 4096, 8192),
            ("profile-requests-full", 0, 8, 16, 8),
            ("corrupt-counts", 2048, 2049, 4096, 8192)):
        outbox.append(_case(name, _outbox, r, q, mr, mq, 0, 0, 1 << 30))
    outbox.append(_case("overflow-before-capacity", _outbox, 0, 0, 2, 8, U64, 0, 1 << 30))

    # Conditional small lengths exercise conversion, not fabricated Work/entry.
    q = first["null_request_payload"]
    scan = first["scan_and_cursor_payload"]
    response_bytes, exact_first_budget, entry_bytes, pending_growth = 1024, 131072, 4096, 98304
    completed_q = _add(q-4, response_bytes)
    live_total = _add(completed_q, scan, exact_first_budget)
    found_total = _add(completed_q, scan, entry_bytes, pending_growth)
    transitions = [
        {"state": "active", "requests": 1, "records": 0, "reserved_slots": 1,
         "bytes": q, "reserved": first["active_extra_reserve"]},
        {"state": "work-live-scan-no-entry", "requests": 1, "records": 0,
         "reserved_slots": 1, "bytes": completed_q+scan, "reserved": exact_first_budget},
        {"state": "first-found", "requests": 1, "records": 1,
         "reserved_slots": 0, "bytes": completed_q+scan+entry_bytes, "reserved": pending_growth},
        {"state": "scan-cleared-pending-retained", "requests": 1, "records": 1,
         "reserved_slots": 0, "bytes": completed_q+entry_bytes, "reserved": pending_growth},
        {"state": "terminal-no-scan", "requests": 1, "records": 0,
         "reserved_slots": 0, "bytes": completed_q, "reserved": 0},
    ]
    arithmetic = [
        _case("uint64-exact-add", _add, U64-1, 1),
        _case("uint64-add-overflow", _add, U64, 1),
        _case("uint64-multiply-overflow", _mul, U64, 2),
        _case("uint64-underflow", _sub, 0, 1),
        _case("boolean-is-not-counter", _u, True),
        _case("negative-counter", _u, -1),
        _case("decimal-string-is-not-counter", _u, "1"),
        _case("null-is-not-counter", _u, None),
        _case("fractional-counter", _u, 1.5),
    ]
    counts = [
        _case("legal-pending-gap", _count_chain, 0, 1, 1, 2, 8, 1, 1),
        _case("terminal-counts-not-released", _count_chain, 1, 2, 3, 3, 8, 3, 2),
        _case("inequality-alone-does-not-prove-match", _count_chain, 1, 2, 3, 3, 8, 2, 2),
        _case("issued-match-required", _count_chain, 1, 2, 3, 3, 8, 3, 1),
        _case("works-exceed-jobs", _count_chain, 2, 1, 3, 3, 8, 3, 1),
    ]
    return {"version": 1,
            "scope": "Conditional arithmetic over completely audited rows; no durable, crypto, session, PoW or concurrency authority.",
            "units": "Source issue and registry namespaces charge keys+values; source Base.Bytes and outbox bytes retain payload semantics.",
            "constants": {"uint64_max": U64, "template_max": TMAX, "job_max": JOB,
                          "request_row_max": ROW, "issue_key": KEY, "old_job_key": JOB_KEY,
                          "receipt": RECEIPT, "public_meta": PUBLIC_META,
                          "public_admission": ADMISSION, "public_small": SMALL,
                          "work_response": RESPONSE, "outcome": OUTCOME},
            "source": source, "registry": registry, "registry_slot_conversion": conversions,
            "counts": counts, "outbox": outbox,
            "outbox_conversion_assumptions": {"response_bytes": response_bytes,
                    "precise_first_entry_budget": exact_first_budget, "entry_bytes": entry_bytes,
                    "pending_entry_growth": pending_growth,
                    "all_lengths_conditional_not_encoded_rows": True},
            "outbox_transitions": transitions,
            "outbox_conversion_checks": {"live_within_initial": live_total <= first["required_bytes"],
                    "first_found_within_live": found_total <= live_total,
                    "duplicate_replay_delta_bytes": 0, "duplicate_replay_delta_slots": 0,
                    "reopen_recomputes_same_reserve": True,
                    "terminal_or_scan_clear_never_deletes_request": True},
            "checked_arithmetic": arithmetic}
