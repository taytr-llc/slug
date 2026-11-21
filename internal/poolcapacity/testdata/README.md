# Independent capacity reference

`two_axis.py` extends the independently frozen Set74 `bounds-v2.py` arithmetic
preserved as `inherited-bounds-v2.py`. It imports no product code or output.
The source was frozen before the recovery implementation. Python 3.14.3 and
Python 3.9.6 produced identical `two-axis-v1.json` bytes:
`699cf788958f4f11e64582a2d31b37102db0afc76fdcf19403337e226405a03c`.

The corpus contains 22 recovery cases, seven old N=R equivalence cases, twelve
invalid-limit mutations and the genuine ordinary intake ceilings for the
combined tests. Regenerate by running `python3 two_axis.py`; the Go test pins
the original corpus digest. See `POOL_LATE_WINNER_RECOVERY.md` at the repository
root for the accounting premises and finite scope.
