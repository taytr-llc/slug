#!/usr/bin/env python3
"""Append frozen dependency evidence; no runtime import or engine execution."""
import hashlib
import json
import pathlib

W = pathlib.Path('/Users/taytrrip/slug')
OUT = pathlib.Path(__file__).resolve().parent
FREEZE = '0fa378f221a8901ba8bdc97f9d86680aa260b274ca5d972a9fc6796d61669067'
V1_INDEX = 'ea71df113574b1a7500f0ceac0560b08cd18fa5350cd65c17b24198299d98d51'
sha = lambda b: hashlib.sha256(b).hexdigest()


def immutable(path, raw):
    path.parent.mkdir(parents=True, exist_ok=True)
    if path.exists():
        assert path.read_bytes() == raw, ('existing evidence changed', str(path))
    else:
        path.write_bytes(raw)


raw = (W / 'set47-contract-dependency-freeze.json').read_bytes()
assert sha(raw) == FREEZE
freeze = json.loads(raw)
assert freeze['runtime_implementation_authorized_now'] is False
assert freeze['bout_metadata_ms'] == 250
old = OUT / 'inputs/provenance/input_index-v1.json'
if not old.exists():
    raw_index = (OUT / 'input_index.json').read_bytes()
    assert sha(raw_index) == V1_INDEX
    immutable(old, raw_index)
assert sha(old.read_bytes()) == V1_INDEX
index = json.loads(old.read_bytes())
for rel, item in index.items():
    assert sha((OUT / rel).read_bytes()) == item['sha256'], rel


def copy(name, expected, rel=None):
    origin = W / name
    data = origin.read_bytes()
    assert sha(data) == expected, name
    rel = rel or ('inputs/dependency46/' + name)
    immutable(OUT / rel, data)
    index[rel] = dict(origin=str(origin), origin_sha256=expected,
                      json_pointer=None, extraction='byte_exact',
                      sha256=expected, bytes=len(data))


for name, expected in freeze['contract_sha256'].items():
    assert sha((OUT / 'inputs/contracts' / name).read_bytes()) == expected
copy('set47-contract-dependency-freeze.json', FREEZE,
     'inputs/contracts/set47-contract-dependency-freeze.json')
for name, expected in freeze['evidence_sha256'].items():
    existing = 'inputs/contracts/' + name
    if existing in index:
        assert index[existing]['sha256'] == expected
        assert sha((W / name).read_bytes()) == expected
    else:
        prefix = 'inputs/contracts/' if name.startswith('set47-card-dependency') else 'inputs/dependency46/'
        copy(name, expected, prefix + name)
copy('set46-card-read-liveness-independent-review.json',
     '60d4b0f934ac7843ad67000060b47dbf4620b30d09b8abff7c01d57dd0e64e47')
copy('set46-card-liveness-engineering-amendment.json',
     'b48a45ce117d226dce351b629c8a20794c19433ed9f6d69410372158690cc2eb')
copy('set46-card-read-liveness-index-addendum.md',
     'e2ac5b50b605906536e0aede1874ca28a58dbc05bbcf958ea3e05f991d65307e')
copy('set46-card-read-liveness-index-addendum-pre-budget.md',
     '9179e4a5f2414cb40ebfa818c088aa88823f2812962ae6f951b341057b8e4861')
index['inputs/provenance/input_index-v1.json'] = dict(
    origin=str(W / 'set47-reference-draft-before-dependency-addendum.json'),
    origin_sha256='64e9a55490e234427777b76911de872ccaaf2a3fc214606cde6f40d435129455',
    json_pointer='/files/input_index.json',
    extraction='byte_exact_preserved_inventory_named_by_draft_capture',
    sha256=V1_INDEX, bytes=len(old.read_bytes()))
new = (json.dumps(index, indent=2) + '\n').encode()
current = OUT / 'input_index.json'
assert sha(current.read_bytes()) == V1_INDEX or current.read_bytes() == new
current.write_bytes(new)
print(json.dumps(dict(status='passed', files=len(index),
                      original_inputs_preserved=46,
                      input_index_sha256=sha(new),
                      previous_input_index_sha256=V1_INDEX,
                      dependency_freeze_sha256=FREEZE,
                      engine_executed=False, corpus_generated=False,
                      new47_runtime_read=False)))
