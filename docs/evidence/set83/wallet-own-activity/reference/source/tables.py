"""Hand-written bounded positions and scalar spelling decisions.

These cases are independent of an executing wallet. `expected` values below
are authored from the existing 256-record/eight-row and 10^8-atoms rules.
The selected wire assembler may consume them without querying product code.
"""

from inherited import MAX_MONEY, MAX_U64, require, signed_decimal, slug_text, unsigned_decimal, window


CURSORS = (
    ("empty", 0, 0, 8, True, 0, 0, False),
    ("single", 1, 0, 8, True, 1, 1, False),
    ("eight", 8, 0, 8, True, 8, 8, False),
    ("nine-first", 9, 0, 8, True, 8, 8, True),
    ("nine-tail", 9, 8, 8, True, 1, 9, False),
    ("nine-end", 9, 9, 1, True, 0, 9, False),
    ("maximum-first", 256, 0, 8, True, 8, 8, True),
    ("maximum-tail", 256, 248, 8, True, 8, 256, False),
    ("maximum-last-one", 256, 255, 8, True, 1, 256, False),
    ("maximum-end", 256, 256, 8, True, 0, 256, False),
    ("zero-limit", 1, 0, 0, False, None, None, None),
    ("nine-limit", 9, 0, 9, False, None, None, None),
    ("past-empty", 0, 1, 8, False, None, None, None),
    ("past-end", 1, 2, 8, False, None, None, None),
    ("negative-after", 1, -1, 8, False, None, None, None),
    ("over-count", 257, 0, 8, False, None, None, None),
    ("boolean-count", True, 0, 8, False, None, None, None),
    ("boolean-after", 1, False, 8, False, None, None, None),
    ("boolean-limit", 1, 0, True, False, None, None, None),
    ("float-after", 1, 0.0, 8, False, None, None, None),
)


DECIMALS = (
    ("zero", "0", True, True),
    ("one", "1", True, True),
    ("negative-one", "-1", False, True),
    ("money-max", "2100000000000000", True, True),
    ("money-min", "-2100000000000000", False, True),
    ("above-money", "2100000000000001", True, False),
    ("below-money", "-2100000000000001", False, False),
    ("max-u64", "18446744073709551615", True, False),
    ("above-u64", "18446744073709551616", False, False),
    ("leading-zero", "01", False, False),
    ("negative-zero", "-0", False, False),
    ("plus", "+1", False, False),
    ("exponent", "1e0", False, False),
    ("fraction", "1.0", False, False),
    ("space", " 1", False, False),
    ("empty", "", False, False),
    ("number", 1, False, False),
    ("boolean", True, False, False),
    ("null", None, False, False),
)


AMOUNTS = (
    (-2100000000000000, "-21000000.00000000"),
    (-100000141, "-1.00000141"),
    (-141, "-0.00000141"),
    (-1, "-0.00000001"),
    (0, "0.00000000"),
    (1, "0.00000001"),
    (141, "0.00000141"),
    (99999999, "0.99999999"),
    (100000000, "1.00000000"),
    (100000001, "1.00000001"),
    (1980000000, "19.80000000"),
    (2970000000, "29.70000000"),
    (4950000000, "49.50000000"),
    (2100000000000000, "21000000.00000000"),
)


def scalar_tables():
    cursors = []
    for name, count, after, limit, valid, size, next_after, has_more in CURSORS:
        actual = None
        try:
            actual = window(count, after, limit)
        except ValueError:
            pass
        expected = None if not valid else {"length": size, "next": next_after, "has_more": has_more}
        require(actual == expected, "authored cursor decision: " + name)
        cursors.append({"name": name, "count": count, "after": after, "limit": limit,
                        "valid": valid, "expected": expected})
    decimals = []
    for name, value, u64, signed_money in DECIMALS:
        require(unsigned_decimal(value) == u64 and signed_decimal(value) == signed_money,
                "authored scalar decision: " + name)
        decimals.append({"name": name, "value": value, "uint64_valid": u64,
                         "signed_money_valid": signed_money})
    amounts = []
    for atoms, text in AMOUNTS:
        require(slug_text(atoms) == text, "authored amount spelling")
        amounts.append({"atoms": atoms, "text": text})
    return {"cursor_cases": cursors, "decimal_cases": decimals, "amount_formats": amounts,
            "limits": {"maximum_activity_records": 256, "maximum_page_rows": 8,
                       "maximum_money": MAX_MONEY, "maximum_uint64": str(MAX_U64)},
            "scope": "Local captured positions and integer spelling only; no live cursor, balance reconstruction, transaction or financial authority."}
