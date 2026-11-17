# Candidate stable reference

`reference_stable.py` derives the set41 expectations from approved contract
`f977e8a7cef5ab470e5019f4f1b9d88cdb4fd03d5ee1feb6acc32321389d84d2`.
Run it with `--check` to independently rederive and compare the corpus. It uses
Python's standard library and the already frozen standalone x/crypto Argon
primitive; it imports no SLUG Go implementation.

The input contains six **public fixture** candidate receipts produced by set40's
packaged acceptance. Their original report SHA256 is recorded in the input.
Actual-runtime records are inputs, not expected outputs: the frozen set37 oracle
independently verifies normal 256-MiB Argon, signature, job, DNA and static binding
before new namespace and full-record digests are calculated. The exact published
zero256 and incrementing256 identities come from set27's independent reference.
Historical authority timestamps and canonical-node observations are trusted
inputs; this oracle does not authenticate them or assert transferable ownership.

The corpus freezes two canonical stable pins, six complete assignment bindings,
and 48 condition boundaries calculated by the independent set35 integer oracle.
The 2,790 existing XP0 condition cases are referenced by hash and exercised from
the original corpus, rather than copied. Nonzero-XP tests remain in set35.

No operator/private wallet files, real keys, network service, issuance or public
mining are involved. The exact reference scripts and public inputs are pinned.
