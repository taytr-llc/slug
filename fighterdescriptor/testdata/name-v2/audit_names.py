"""Preparation audit only: no fighter generation or implementation oracle."""
from collections import Counter
import hashlib
import json
from pathlib import Path
import re

ROOT = Path(__file__).resolve().parent
data = json.loads((ROOT / "candidate_names.json").read_text())
assert list(data) == ["first_names", "last_names"]
grammar = re.compile(r"[A-Za-z]+(?:['-][A-Za-z]+)*\Z", re.ASCII)
report = {"status": "preparatory candidate; pending root review; not frozen", "components": {}}
for key, values in data.items():
    assert len(values) == 512
    assert values == sorted(values, key=lambda value: (value.casefold(), value))
    assert all(1 <= len(value.encode("ascii")) <= 32 and grammar.fullmatch(value) for value in values)
    assert len(set(values)) == len(values)
    assert len({value.casefold() for value in values}) == len(values)
    report["components"][key] = {
        "count": len(values),
        "exact_unique": len(set(values)),
        "ascii_case_fold_unique": len({value.casefold() for value in values}),
        "maximum_ascii_bytes": max(len(value.encode("ascii")) for value in values),
        "initial_counts": dict(sorted(Counter(value[0] for value in values).items())),
        "punctuated_entries": [value for value in values if "'" in value or "-" in value],
    }

# Enumerate the entire product, not a sample, to check exact and folded
# collisions between different component-index pairs. Runtime output reuse
# across different seeds is a different phenomenon and remains permitted.
names = [first + " " + last for first in data["first_names"] for last in data["last_names"]]
assert len(names) == len(set(names)) == len({name.casefold() for name in names}) == 262144
report["cartesian_product"] = {
    "pairs": len(names),
    "exact_display_strings": len(set(names)),
    "ascii_case_fold_display_strings": len({name.casefold() for name in names}),
    "ambiguous_different_pairs": 0,
    "maximum_display_ascii_bytes": max(len(name.encode("ascii")) for name in names),
    "separator": "one ASCII space; no component contains a space",
}
report["shared_first_surname_components"] = sorted(set(data["first_names"]) & set(data["last_names"]))
report["real_name_coincidence_examples"] = [name for name in ("Hugh Grant", "Ray Charles", "William Wallace") if name in set(names)]
report["real_name_coincidence_note"] = "Illustrative ordinary-name coincidences noticed in review; not an exhaustive public-figure search, identity attribution or blacklist. No generated-name rerolls are proposed."

v1 = Path("/Users/taytrrip/slug/slug/fightermanifest/tables_v1.json").read_bytes()
expected = "9f9380b708cae73b8d2641eaf04ecefb3a3b7fa9dd1b25b1f234f6511780b8c7"
assert hashlib.sha256(v1).hexdigest() == expected
report["existing_v1_table"] = {"expected_preexisting_sha256": expected, "matches_preexisting_record": True, "written_by_this_audit": False}
report["limits"] = [
    "No claim of trademark clearance or global linguistic safety.",
    "No name frequencies, demographic representation or production population are inferred.",
    "No generator, descriptor, digest domain or expected fighter vectors are defined here.",
    "Candidate ordering and JSON formatting remain reviewable, not a frozen byte contract.",
    "Different seeds can reuse these names; the no-ambiguity result is not a uniqueness promise.",
]
(ROOT / "audit-report.json").write_text(json.dumps(report, indent=2) + "\n")
print(json.dumps({"first_names": 512, "last_names": 512, "unambiguous_display_strings": 262144, "v1_unchanged": True, "status": report["status"]}))
