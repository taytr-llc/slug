# Independent Set65 reference

This is an external, standard-library-only Python oracle for the approved v2
contracts in the parent bundle. It uses no Go code/output, node, network,
private key, mining or production account. The approved contracts, exact
names-only table, inherited contracts and independent v1 inputs are archived
and SHA256-pinned before any new expectations are emitted.

`public-inputs.json` fixes the seed/coverage rules, name-stream counters,
primitive rejection cases, collision scenarios and 100,000-input sample domain
before results. `input-lock.json` pins those rules, the oracle script and all
semantic inputs. The oracle verifies the lock both before and after a run.
Changing a script/input requires a new retained attempt; never change an
expectation to match an implementation.

The old independent manifest and portrait references under `inputs/portrait-v1`
are copied byte-for-byte from their existing archived Python reference bundle.
The entire v1 manifest and SVG corpora are regenerated and compared before v2
corpus generation/checking. V2 manifest values inherit the independently
derived old DNA/physical/style/portrait fields. V2 names are computed here from
the new domains and exact table. V2 SVG uses the archived rational paint routine
after full v2 validation, replacing only its explicit renderer attribute; that
routine already derives the labels from the supplied validated manifest. The
graphics body is independently compared against the original v1 rendering for
every v2 case. This is reuse of an archived independent reference, not capture
of production rendering output.

Commands:

```
python3 reference_v2.py --generate
python3 reference_v2.py --check
python3 reference_v2.py --sample --generate
python3 reference_v2.py --sample --check
```

Generation uses exclusive output creation. Check completely regenerates the
corpus/report and compares the bytes; it does not merely verify an embedded
checksum. The reference corpus contains the complete canonical manifest and
SVG bytes/digests, the exact schema-1 descriptor and ID, name-index coverage,
literal stream preimages/counters/digests, independent-stream ordering and
counter exhaustion, scripted rejection/exhaustion primitives, actual apostrophe
names, wrong tuples and malformed canonical input cases. The number of selected
cases follows the predeclared first-coverage rule, not a desired output size.

Collision measurements are a separate report. The theoretical model explicitly
assumes independent uniform names; the observed experiment uses fixed public
SHA256-derived seeds, with no proof-of-work qualification. Neither is a forecast
or numeric release gate. Probability rounded to one is distinguished from
pigeonhole certainty. The report includes distinct names, excess entries,
equal-name pairs, affected fighters, component frequencies and all requested
population prefixes. No cryptographic Monte Carlo, real accounts or keys are
needed for this bounded preparation.

Future repository integration must archive the complete approved contract
bundle with the same relative paths. Explanatory repository documentation can
link to the frozen files; do not edit their bytes to update prose or paths.
