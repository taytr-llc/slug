"""Bounded public-row model; synthetic DB descriptions grant no capture handle.

Only the mixed-row length/framing and public cursor arithmetic are modeled.
Real public-key decryption, retained Manager identity, bucket Get prohibitions,
DB snapshot atomicity, context joins and key clearing require implementation
tests.  A plaintext public manifest here is separately pinned test input, never
a substitute for the concrete wallet's retained owner.
"""
from common import canonical, exact, integer, need, own, packed
from public_rows import cursors, IMPORTED, ROW_CAP

PURPOSES = (44, 49, 84, 86)
# Stored schema is INTERNAL byte first, EXTERNAL second (inherited db.go).
SCHEMAS = ('0000', '0403', '0404', '0606')
MANIFEST = ('version','profile','network','genesis','derivation','scan_from_height',
            'root_public_key','gym_path','gym_public_key','gym_id','accounts')
ACCOUNT = ('purpose','coin','account','external_type','internal_type','next_external','next_internal')
LAYOUT = ('manager_version','derivation_mode','recovery_version','watch_only',
          'network','genesis','public_manifest','public_root','scopes')
SCOPE = ('key_hex','is_bucket','schema_hex','accounts','imported_index')
ROW = ('key_hex','row_hex')


def raw_hex(value, maximum):
    # Fixture hex wrapper is bounded before decode; native storage is raw bytes.
    need(type(value) is str and len(value) <= maximum*2 and len(value)%2 == 0)
    need(all(c in '0123456789abcdef' for c in value))
    return bytes.fromhex(value)


def read_inventory(value, original):
    """Return public manifest plus modeled visit counts, never private spans."""
    v = exact(value, LAYOUT)
    for field, expected in (('manager_version',9),('derivation_mode',1),('recovery_version',1)):
        need(type(v[field]) is int and v[field] == expected, 'profile')
    need(v['watch_only'] is False, 'profile')
    need(type(v['network']) is int and v['network'] == original['network'], 'profile')
    need(v['genesis'] == original['genesis'], 'profile')
    public = exact(v['public_manifest'], MANIFEST)
    # Public retention is immutable. Current issuance comes only from rows.
    need(canonical(public) == canonical(original))
    need(v['public_root'] == original['root_public_key'])
    need(type(v['scopes']) is list)
    out = own(original)
    counts = dict(scope_entries=0, account_entries=0, imported_probes=0,
                  largest_mixed_row_bytes=0, private_span_bytes_copied=0)
    found = set()
    for s in v['scopes']:
        counts['scope_entries'] += 1
        need(counts['scope_entries'] <= 4)  # fifth observed entry is refusal witness
        s = exact(s, SCOPE)
        key = raw_hex(s['key_hex'], 8)
        need(len(key) == 8 and s['is_bucket'] is True)
        purpose, coin = int.from_bytes(key[:4],'little'), int.from_bytes(key[4:],'little')
        need(coin == 0 and purpose in PURPOSES and purpose not in found)
        # Real bucket cursor order is lexicographic key order, fixed by LE keys.
        i = counts['scope_entries']-1
        need(purpose == PURPOSES[i] and s['schema_hex'] == SCHEMAS[i])
        found.add(purpose)
        need(type(s['accounts']) is list)
        account_ids = []
        default = None
        for item in s['accounts']:
            counts['account_entries'] += 1
            need(len(account_ids) < 2)  # third entry is refusal witness
            item = exact(item, ROW)
            key = raw_hex(item['key_hex'],4)
            need(len(key) == 4)
            account = int.from_bytes(key,'little')
            need(account in (0,IMPORTED) and account not in account_ids)
            need(account == (0 if len(account_ids)==0 else IMPORTED))
            account_ids.append(account)
            row = raw_hex(item['row_hex'], ROW_CAP)
            counts['largest_mixed_row_bytes'] = max(counts['largest_mixed_row_bytes'],len(row))
            result = cursors(row,account)
            if account == 0:
                default = result
        need(account_ids == [0,IMPORTED] and default is not None)
        counts['imported_probes'] += 1
        # Missing/empty imported SUBBUCKET accepted. Parent account-index bucket
        # must exist; model uses a distinct sentinel for that structural failure.
        need(s['imported_index'] in ('absent','empty'))
        a = exact(out['accounts'][i], ACCOUNT)
        for name in ('next_external','next_internal'):
            a[name] = integer(default[name],0,4096)
        out['accounts'][i] = a
    need(found == set(PURPOSES))
    packed(out,4096)
    return out, counts
