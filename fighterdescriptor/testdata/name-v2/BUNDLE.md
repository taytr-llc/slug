# Frozen candidate names-v2 reference bundle

The root-reviewed contracts, exact names-only table and inherited v1 inputs are
bound by `approval.json`. The independent scripts and public inputs were locked
before oracle generation. `independent/reference-v2.json` and the separate
collision report are independently derived expectations, not Go output.

The top-level README and reviewed-proposals preserve earlier preparation/review
history; approval.json and the frozen contract status record the final review.
RESULTS.md describes completed independent verification. Current defaults and
production activation do not follow from this bundle.

Run `python3 independent/reference_v2.py --check` and, separately,
`python3 independent/reference_v2.py --sample --check` from any working directory.
No filesystem paths outside this bundle are needed for oracle regeneration.
