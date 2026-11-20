# Actual v2 bout transport regression

`record.json` is the unchanged public saved bout from Set65
`actual-v2-attempt1`, the first genuine v2 shared-pool event cycle. That run
reached real seed finality and saved the bout, then failed desktop publication
because `boutRecordShape` still hashed its v2 manifests with the v1 domain.
The original failed run is retained outside the repository. Extraction opened
only a copy of its public bout database read-only; no account or wallet files
were opened. `pin.json` is the unchanged pin extracted from its public marker.

These are regression **inputs**, not an independently generated fight oracle
and not proof of current authority. The test reconstructs explicitly labelled
transport observation/job metadata, checks the full saved record and completed
job shape, and rejects mixed descriptor/digest cases. It does not assert a new
engine winner or treat these files as trusted live stores.

`verify_domains.py` independently recomputes the relevant SHA256 commitments
using Python's standard library, without Go or the production implementation.
It confirms both original assignment digests use v2 and both v1 substitutions
fail; engine input/result commitments retain their unchanged v1 domains.
`domain-report.json` records the initial independent check. The separate frozen
Set65 name-v2 corpus remains the deterministic manifest/name oracle.
