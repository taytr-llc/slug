#!/usr/bin/env python3
"""Independent, bounded PUBLIC Set80 A/B checker. Root alone imports/runs it.

Expected policy/economics are from the pre-execution qualified 0f089 reference.
No product modules, subprocesses, credentials, wallet files or databases are read.
The source-bound Go audits supply consensus/signature/owner evidence; this reader
independently checks their public counts, byte identities and financial arithmetic.
"""
import copy
import datetime as dt
import hashlib
import json
import math
import os
from pathlib import Path
import re
import stat
import struct
from urllib.parse import urlsplit

REFERENCE = Path('/Users/taytrrip/slug/artifacts/set80/actual-reference-attempt1/python314.json')
REFERENCE_SHA = '0f0890e529b64c10ed000673aab7861b9bd526057cc3c2bbbe05b868028f3d10'
REFERENCE_BYTES = 173644
MAX_JSON = 16 << 20
HEX = re.compile(r'[0-9a-f]+\Z')
ZERO = {'numerator': '0', 'denominator': '1'}


def need(ok, why):
    if not ok:
        raise ValueError('Set80 public check: ' + why)


def keys(v, names, optional=(), ordered=False):
    required = names.split() if isinstance(names, str) else list(names)
    optional = optional.split() if isinstance(optional, str) else list(optional)
    need(type(v) is dict and set(required) <= set(v) <= set(required + optional), 'object fields: ' + ' '.join(required))
    if ordered:
        need(list(v) == required, 'canonical field order: ' + ' '.join(required))
    return v


def integer(v, low=0, high=(1 << 64)-1):
    need(type(v) is int and low <= v <= high, 'bounded integer')
    return v


def truth(v, expected=True):
    need(type(v) is bool and v is expected, 'exact boolean')


def text(v, cap=4096):
    need(type(v) is str and len(v) <= cap and v.isascii(), 'bounded ASCII string')
    return v


def hx(v, n=32, nonzero=True):
    text(v, n*2)
    need(len(v) == n*2 and HEX.fullmatch(v) and (not nonzero or v != '0'*(n*2)), 'canonical hex')
    return bytes.fromhex(v)


def rawhex(v, cap):
    text(v, cap*2)
    need(len(v) % 2 == 0 and 0 < len(v) <= cap*2 and HEX.fullmatch(v), 'bounded raw hex')
    return bytes.fromhex(v)


def sha(b):
    return hashlib.sha256(b).hexdigest()


def canonical(v):
    # All modeled fields are ASCII. Match Go JSON's HTML escapes as well.
    return json.dumps(v, ensure_ascii=True, allow_nan=False, separators=(',', ':')).replace('<', '\\u003c').replace('>', '\\u003e').replace('&', '\\u0026').encode('ascii')


def same(a, b, why):
    need(canonical(a) == canonical(b), why)


def identity(label, value, version='v1'):
    raw = canonical(value)
    return sha(label.encode('ascii') + b'\0' + version.encode('ascii') + b'\0' + struct.pack('<I', len(raw)) + raw)


def pairs(items):
    result = {}
    for k, v in items:
        need(k not in result, 'duplicate JSON key')
        result[k] = v
    return result


def bounded_tree(v, depth=0, budget=None):
    if budget is None:
        budget = [0]
    budget[0] += 1
    need(depth <= 48 and budget[0] <= 500000, 'JSON structural bound')
    if type(v) is dict:
        need(len(v) <= 20000, 'object bound')
        for k, x in v.items():
            text(k, 1024)
            bounded_tree(x, depth+1, budget)
    elif type(v) is list:
        need(len(v) <= 20000, 'array bound')
        for x in v:
            bounded_tree(x, depth+1, budget)
    elif type(v) is str:
        text(v, 8 << 20)
    elif type(v) is int:
        need(-(1 << 63) <= v <= (1 << 64)-1, 'integer JSON domain')
    elif type(v) is float:
        need(math.isfinite(v), 'finite JSON number')
    else:
        need(v is None or type(v) is bool, 'JSON type')


def parse(raw):
    value = json.loads(raw.decode('ascii'), object_pairs_hook=pairs,
                       parse_constant=lambda _: (_ for _ in ()).throw(ValueError('nonfinite JSON')))
    bounded_tree(value)
    return value


def read_regular(path, limit):
    need(path.is_absolute() and path.resolve(strict=True) == path, 'canonical public path')
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW)
    try:
        before = os.fstat(fd)
        need(stat.S_ISREG(before.st_mode) and 0 <= before.st_size <= limit, 'regular bounded public file')
        chunks, size = [], 0
        while True:
            chunk = os.read(fd, min(65536, limit+1-size))
            if not chunk:
                break
            chunks.append(chunk)
            size += len(chunk)
            need(size <= limit, 'public file grew over bound')
        after = os.fstat(fd)
        need((before.st_dev, before.st_ino, before.st_size, before.st_mtime_ns) ==
             (after.st_dev, after.st_ino, after.st_size, after.st_mtime_ns), 'public file changed during read')
        return b''.join(chunks)
    finally:
        os.close(fd)


class Public:
    def __init__(self, root):
        self.root = Path(root)
        need(self.root.is_absolute() and self.root.resolve(strict=True) == self.root and self.root.is_dir(), 'original public evidence root')
        self.identities = {}

    def raw(self, name, limit=MAX_JSON):
        need(re.fullmatch(r'[a-z0-9-]+\.(json|jsonl|log)', name), 'explicit flat public filename')
        path = self.root / name
        raw = read_regular(path, limit)
        ident = {'bytes': len(raw), 'sha256': sha(raw)}
        if str(path) in self.identities:
            need(self.identities[str(path)] == ident, 'public file changed across reads')
        self.identities[str(path)] = ident
        return raw

    def json(self, name):
        raw = self.raw(name)
        need(raw.endswith(b'\n') and raw, 'complete public JSON')
        return parse(raw)

    def lines(self, name, count, frame_bound):
        raw = self.raw(name, 1 << 20)
        need(not raw or raw.endswith(b'\n'), 'complete public stdout')
        need(b'\r' not in raw, 'LF stdout')
        lines = raw.splitlines()
        need(len(lines) <= count, 'stdout frame count')
        values = []
        for line in lines:
            need(0 < len(line)+1 <= frame_bound, 'stdout frame bytes')
            v = parse(line)
            need(canonical(v) == line, 'canonical stdout')
            values.append(v)
        return values

    def finish(self):
        for path, expected in self.identities.items():
            raw = read_regular(Path(path), MAX_JSON)
            need({'bytes': len(raw), 'sha256': sha(raw)} == expected, 'final immutable public evidence')
        return dict(self.identities)


def loopback_url(value, scheme):
    u = urlsplit(text(value, 512))
    need(u.scheme == scheme and u.hostname == '127.0.0.1' and u.username is None and u.password is None
         and not u.path and not u.query and not u.fragment and u.port is not None, 'explicit loopback transport')
    return integer(u.port, 1, 65535)


def selected(selection, report, ref):
    keys(selection, 'setup remote setup_id remote_id due_ms start_ms initial_height')
    setup, remote = selection['setup'], selection['remote']
    keys(setup, 'version base issue_profile', ordered=True)
    keys(remote, 'version base issue_profile', ordered=True)
    need(setup['version'] == remote['version'] == 1, 'explicit wrapper version')
    s, c = setup['base'], remote['base']
    normalized = copy.deepcopy(s)
    normalized['ingress'] = ref['setup']['base']['ingress']
    normalized['schedule']['daily_utc_minute'] = ref['setup']['base']['schedule']['daily_utc_minute']
    same(normalized, ref['setup']['base'], 'only original transport and due may vary')
    normalized = copy.deepcopy(c)
    normalized['origin'] = ref['remote']['base']['origin']
    normalized['ca_certificate'] = ref['remote']['base']['ca_certificate']
    same(normalized, ref['remote']['base'], 'original remote/local-wallet selection unchanged')
    ingress = keys(s['ingress'], 'version listen origin certificate_spki pool_url admission_url ownership_url', ordered=True)
    need(ingress['version'] == 2 and c['origin'] == ingress['origin'], 'same explicit ingress')
    ports = [loopback_url(ingress['origin'], 'https')]
    ports.append(loopback_url('http://' + ingress['listen'], 'http'))
    ports += [loopback_url(ingress[k], 'http') for k in ('pool_url', 'admission_url', 'ownership_url')]
    need(len(set(ports)) == 5, 'distinct selected listeners')
    hx(ingress['certificate_spki'])
    pem = text(c['ca_certificate'], 65536)
    need(pem.startswith('-----BEGIN CERTIFICATE-----\n') and pem.endswith('-----END CERTIFICATE-----\n'), 'public CA framing')
    start, due = integer(selection['start_ms'], 1), integer(selection['due_ms'], 1)
    need(due == ((start + 420000)//60000)*60000, 'original seven-minute UTC target')
    date = lambda ms: dt.datetime.fromtimestamp(ms/1000, dt.timezone.utc)
    need(date(start).date() == date(start+900000).date(), 'same finite UTC day')
    need(s['schedule']['daily_utc_minute'] == date(due).hour*60+date(due).minute, 'same schedule minute')
    base_id = identity('SLUG pool operator setup', s, 'v2')
    source_id = sha(b'SLUG pool share daily source\0v2\0'+canonical(s['pool']))
    profile = dict(version=1, base_setup_id=base_id, source_config_id=source_id, max_requests=s['pool']['max_works'])
    same(setup['issue_profile'], profile, 'derived original profile')
    same(remote['issue_profile'], profile, 'same remote profile')
    pid = identity('SLUG pool issue recovery profile', profile)
    sid = identity('SLUG pool issue recovery setup', setup)
    rid = identity('SLUG pool issue recovery attachment', remote)
    need(selection['setup_id'] == report['setup_id'] == sid and selection['remote_id'] == report['remote_id'] == rid
         and report['issue_profile_id'] == pid, 'independently derived wrapper identities')
    need(report['start_ms'] == start and report['due_ms'] == due and report['initial_height'] == selection['initial_height'] == 2, 'same scenario')
    need(source_id == s['miners']['source_id'] == profile['source_config_id'] and ref['ordinary_maximum'] == 2, 'exact fixed source/cap')
    return s, c, pid


def row_key(store, prefix, ident, binary=True):
    return store + '/' + (prefix.encode('ascii') + (hx(ident) if binary else ident.encode('ascii'))).hex()


def hash_rows(rows):
    need(type(rows) is dict and 1 <= len(rows) <= 16384, 'full bounded public namespace')
    for key, value in rows.items():
        name, sep, raw = key.partition('/')
        need(name in ('issuer', 'pool', 'rounds', 'settlement') and sep and len(raw) <= 2048, 'accounting namespace key')
        rawhex(raw, 1024)
        hx(value)


def financial_shape(f, s, root):
    keys(f, 'submissions accepted works authority_jobs admissions births entries rounds credits header body body_sha256 body_bytes delivery report read_only_unchanged source_requests issuer_requests public_rows public_files_before public_files_after')
    truth(f['read_only_unchanged'])
    for k in ('submissions', 'accepted', 'works', 'authority_jobs', 'births', 'source_requests', 'issuer_requests'):
        integer(f[k], 0, 1000)
    need(f['accepted'] <= f['submissions'] <= 2 and f['works'] <= f['authority_jobs'] <= 8, 'real finite physical source')
    need(type(f['admissions']) is list and len(f['admissions']) == f['births'] <= 64, 'complete bounded births')
    for key in ('entries', 'rounds', 'credits'):
        need(type(f[key]) is list and len(f[key]) <= 2, 'finite financial lists')
    hash_rows(f['public_rows'])
    before, after = f['public_files_before'], f['public_files_after']
    need(type(before) is dict and 1 <= len(before) <= 4096, 'public file inventory')
    same(before, after, 'closed public files unchanged')
    for name, digest in before.items():
        need(type(name) is str and '..' not in name.split('/') and any(name.startswith(str(root / 'operator' / store) + '/') for store in ('issuer', 'pool', 'rounds', 'settlement')), 'original public-only file inventory')
        hx(digest)
    h = keys(f['header'], 'application version config_id source_id revision credits batches events logical_bytes pending_batch last_day last_clock_ms', ordered=True)
    need(h['application'] == 'slug-pool-settlement-daily' and h['version'] == 2 and h['config_id'] == s['miners']['settlement_config_id']
         and h['source_id'] == s['miners']['source_id'], 'original daily ledger binding')
    for k in ('revision', 'credits', 'batches', 'events', 'logical_bytes', 'pending_batch', 'last_clock_ms'):
        integer(h[k])
    need(h['credits'] == len(f['credits']) <= 2 and h['batches'] <= 1 and h['pending_batch'] <= h['batches'], 'ledger exact counts')
    need(f['public_rows'].get('settlement/'+b'head'.hex()) == sha(canonical(h)) and f['public_rows'].get('settlement/'+b'report'.hex()) == sha(canonical(f['report'])), 'original header/report exact audited bytes')
    body = keys(f['body'], 'version sequence day created_at_ms inputs payments fee_atoms change txid raw', ordered=True)
    if h['batches']:
        raw = canonical(body)
        need(len(raw) == f['body_bytes'] and sha(raw) == f['body_sha256'], 'immutable signed body bytes')
        key = 'settlement/' + (b'b/'+struct.pack('>Q', 1)).hex()
        need(f['public_rows'].get(key) == sha(raw), 'body is original audited row')
    else:
        need(f['body_bytes'] == 0 and f['body_sha256'] == f['delivery'] == '' and body['version'] == body['sequence'] == 0
             and body['raw'] == body['txid'] == '' and body['inputs'] is None and body['payments'] is None, 'no hidden batch')
    return f


def audit(v, s, pid, request_id, root):
    keys(v, 'financial source_request issuer_request job template template_sha256 source_requests issuer_requests source_templates source_inverses issuer_inverses requests issuer_rows jobs work row_hashes read_only_unchanged')
    truth(v['read_only_unchanged'])
    f = financial_shape(v['financial'], s, root)
    same(v['row_hashes'], f['public_rows'], 'same fully audited public namespace')
    rows = v['row_hashes']
    stored = {store: [bytes.fromhex(k.split('/', 1)[1]) for k in rows if k.startswith(store+'/')] for store in ('issuer', 'pool', 'settlement')}
    for prefix, width, expected in ((b'w', 65, f['works']), (b's', 65, f['submissions']), (b'q', 9, f['submissions']), (b'a', 9, f['accepted'])):
        need(sum(len(k) == width and k.startswith(prefix) for k in stored['pool']) == expected, 'physical complete source index count')
    for store, prefix, field in (('pool', b'issue/r/', 'source_requests'), ('pool', b'issue/t/', 'source_templates'), ('pool', b'issue/j/', 'source_inverses'), ('issuer', b'issue/r/', 'issuer_requests'), ('issuer', b'issue/j/', 'issuer_inverses')):
        need(sum(len(k) == 40 and k.startswith(prefix) for k in stored[store]) == v[field], 'physical complete request namespace count')
    requests, issuers, jobs = v['requests'], v['issuer_rows'], v['jobs']
    need(type(requests) is list and type(issuers) is list and type(jobs) is list, 'request census arrays')
    need(1 <= len(requests) == v['source_requests'] == f['source_requests'] <= 8 and len(issuers) == v['issuer_requests'] == f['issuer_requests'] <= len(requests)
         and len(jobs) == f['authority_jobs'] == v['issuer_inverses'], 'all original requests/jobs counted')
    enrollments = {x['gym_id']: x['public_key'] for x in s['miners']['enrollments']}
    rm, im, jm = {}, {}, {}
    for r in requests:
        keys(r, 'version intent intent_id state job_id resolved_at_ms reason', ordered=True)
        i = keys(r['intent'], 'version profile_id request_id gym_id public_key template_sha256', ordered=True)
        rid = i['request_id']; hx(rid); hx(i['template_sha256']); hx(i['public_key'], 33)
        need(i['version'] == r['version'] == 1 and i['profile_id'] == pid and enrollments.get(i['gym_id']) == i['public_key'], 'selected original member intent')
        need(rid not in rm and r['intent_id'] == identity('SLUG pool issue recovery intent', i), 'unique independent intent identity')
        rm[rid] = r
        need(rows.get(row_key('pool', 'issue/r/', rid)) == sha(canonical(r)), 'source request exact audited row')
        need(r['state'] in ('pending', 'work', 'issued_unused', 'not_issued'), 'closed request state')
        if r['state'] == 'pending':
            need(r['job_id'] == r['reason'] == '' and r['resolved_at_ms'] == 0, 'pending has no invented result')
        elif r['state'] == 'work':
            hx(r['job_id']); integer(r['resolved_at_ms'], 1); need(r['reason'] == '', 'work reason')
        elif r['state'] == 'issued_unused':
            hx(r['job_id']); integer(r['resolved_at_ms'], 1); need(r['reason'] == 'expired_before_work', 'unused reason')
        else:
            need(r['job_id'] == '' and r['reason'] in ('parent_changed', 'intake_drained'), 'sealed request reason')
    for r in issuers:
        keys(r, 'version profile_id request_id intent_id gym_id public_key state job_id sealed_at_ms reason', ordered=True)
        rid = r['request_id']; need(rid in rm and rid not in im, 'unique source/issuer pairing'); im[rid] = r
        i = rm[rid]['intent']
        need(r['version'] == 1 and r['profile_id'] == pid and r['intent_id'] == rm[rid]['intent_id'] and r['gym_id'] == i['gym_id'] and r['public_key'] == i['public_key'], 'exact paired issuer identity')
        need(rows.get(row_key('issuer', 'issue/r/', rid)) == sha(canonical(r)), 'issuer request exact audited row')
        need(r['state'] in ('issued', 'sealed'), 'issuer state')
        if r['state'] == 'issued':
            hx(r['job_id']); need(r['sealed_at_ms'] == 0 and r['reason'] == '', 'issued immutable job')
        else:
            need(r['job_id'] == '' and r['sealed_at_ms'] > 0, 'sealed no job')
    for j in jobs:
        keys(j, 'version rule_version id authority_id descriptor_id context public_key gym_id challenge issued_at_ms expires_at_ms', ordered=True)
        jid = j['id']; hx(jid); hx(j['challenge']); need(jid not in jm, 'unique original job'); jm[jid] = j
        need(j['version'] == 2 and j['gym_id'] in enrollments and j['public_key'] == enrollments[j['gym_id']], 'original job gym/key')
        need(j['authority_id'] == s['pool']['authority_pin']['authority_id'] and j['descriptor_id'] == s['pool']['authority_pin']['descriptor_id'], 'original job authority/descriptor')
        integer(j['issued_at_ms'], 1); need(j['expires_at_ms']-j['issued_at_ms'] == 300000, 'original five-minute lifetime')
        need(rows.get(row_key('issuer', 'job:', jid, False)) == sha(canonical(j)), 'exact original stored job')
    for rid, ir in im.items():
        if ir['state'] == 'issued':
            need(ir['job_id'] in jm, 'issuer retains original job')
            j = jm[ir['job_id']]
            need(j['gym_id'] == ir['gym_id'] and j['public_key'] == ir['public_key'], 'job issuer correlation')
            need(rows.get(row_key('issuer', 'issue/j/', j['id'])) == sha(hx(rid)), 'issuer reverse exact')
    works = sum(r['state'] == 'work' for r in requests)
    inverses = sum(r['state'] in ('work', 'issued_unused') for r in requests)
    need(works == f['works'] and v['source_templates'] == len(requests)-works and v['source_inverses'] == inverses, 'complete retained source state census')
    for rid, r in rm.items():
        need(rid in im or r['state'] == 'pending', 'only pending may precede issuer')
        if r['state'] in ('work', 'issued_unused'):
            need(im[rid]['state'] == 'issued' and im[rid]['job_id'] == r['job_id'], 'same completed issuer job')
            need(rows.get(row_key('pool', 'issue/j/', r['job_id'])) == sha(hx(rid)), 'source reverse exact')
            if r['state'] == 'issued_unused':
                need(r['resolved_at_ms'] >= jm[r['job_id']]['expires_at_ms'], 'terminal never renews expiry')
    same(v['source_request'], rm[request_id], 'selected request in complete census')
    same(v['issuer_request'], im[request_id], 'selected issuer in complete census')
    same(v['job'], jm[im[request_id]['job_id']], 'selected original job')
    template = v['template']
    need(sha(canonical(template)) == v['template_sha256'] == rm[request_id]['intent']['template_sha256'], 'exact original template digest')
    same(v['job']['context'], template['context'], 'original context retained')
    if rm[request_id]['state'] == 'work':
        same(v['work'], dict(job=v['job'], template=template), 'complete same work')
        need(rows.get(row_key('pool', 'w', v['job']['id'], False)) == sha(canonical(v['work'])), 'completed work exact stored bytes')
        need(row_key('pool', 'issue/t/', request_id) not in rows, 'completed template moves into work')
    else:
        need(v['work'] is None and row_key('pool', 'w', v['job']['id'], False) not in rows, 'no hidden original work')
        need(rows.get(row_key('pool', 'issue/t/', request_id)) == v['template_sha256'], 'retained exact unpublished template')
    return v


def empty_financial(f):
    need(all(f[k] == 0 for k in ('submissions', 'accepted', 'births', 'works')) and all(f[k] == [] for k in ('admissions', 'entries', 'rounds', 'credits'))
         and f['header']['credits'] == f['header']['batches'] == 0, 'zero financial/source effects')


def outbox(v, request_id=None, pending=False):
    keys(v, 'records pending requests active_request_id logical_bytes reserved meta_sha256 meta_bytes marker_sha256 scan_sha256 scan_request_id cursor_sha256 next_nonce requests_sha256 rows_sha256 namespace_rows read_only_unchanged', ordered=True)
    truth(v['read_only_unchanged'])
    for k in ('records', 'pending', 'requests', 'logical_bytes', 'reserved', 'meta_bytes', 'next_nonce', 'namespace_rows'):
        integer(v[k])
    need(v['pending'] <= v['records'] and 1 <= v['requests'] <= 64 and v['records']+v['requests'] <= 64
         and v['logical_bytes']+v['reserved'] <= 67108864 and 0 < v['meta_bytes'] <= 262144, 'outbox exact bounded controls')
    for k in ('meta_sha256', 'marker_sha256', 'requests_sha256', 'rows_sha256'):
        hx(v[k])
    for k in ('active_request_id', 'scan_request_id', 'scan_sha256', 'cursor_sha256'):
        if v[k]: hx(v[k])
    if v['active_request_id']:
        need(v['scan_sha256'] == v['scan_request_id'] == v['cursor_sha256'] == '' and v['next_nonce'] == 0, 'active null excludes scan')
    if v['scan_sha256']:
        need(v['scan_request_id'] and v['cursor_sha256'], 'scan and current request/cursor')
    if pending:
        need(v['requests'] == 1 and v['records'] == v['pending'] == 0 and v['active_request_id'] == request_id and v['namespace_rows'] == 2, 'one null original request before dispatch completion')
        q = canonical(dict(version=1, request=dict(request_id=request_id), result=None))
        k = b'q'+hx(request_id)
        framed = struct.pack('>Q', len(k))+k+struct.pack('>Q', len(q))+q
        need(v['requests_sha256'] == sha(b'SLUG actual recoverable outbox requests\0v1\0'+framed), 'independent null q bytes')
    return v


def response(v, s, pid, request_id, gap, state):
    keys(v, 'version binding_id settlement_config_id recipient_script issue_profile_id request_id gym_id public_key result', ordered=True)
    en = s['miners']['enrollments'][0]
    need(v['version'] == 3 and v['binding_id'] == s['miners']['source_id'] and v['settlement_config_id'] == s['miners']['settlement_config_id']
         and v['recipient_script'] == en['payee_script'] and v['issue_profile_id'] == pid and v['request_id'] == request_id
         and v['gym_id'] == en['gym_id'] and v['public_key'] == en['public_key'], 'complete successful response selection/money/gym binding')
    r = keys(v['result'], 'version state intent_id job_id resolved_at_ms reason work', ordered=True)
    need(r['version'] == 1 and r['state'] == state and r['intent_id'] == gap['source_request']['intent_id'] and r['job_id'] == gap['job']['id'], 'same exact result identity')
    integer(r['resolved_at_ms'], 1)
    if state == 'work':
        need(gap['job']['issued_at_ms'] <= r['resolved_at_ms'] < gap['job']['expires_at_ms'] and r['reason'] == '', 'original live resolution time')
        same(r['work'], dict(job=gap['job'], template=gap['template']), 'same original job/template/expiry work')
    else:
        need(state == 'issued_unused' and r['work'] is None and r['reason'] == 'expired_before_work' and r['resolved_at_ms'] >= gap['job']['expires_at_ms'], 'original expired unpublished terminal')
    return v


def processes(public, report, case, probe_pid, selection):
    integer(probe_pid, 1)
    used = {probe_pid}
    def pid(v):
        p = integer(v['pid'], 1)
        need(p not in used and v['parent_pid'] == v['pgid'] == probe_pid, 'original distinct child PID/inherited PGID')
        used.add(p)
        truth(v['joined'])
    ops = report['operator_generations']
    names = ['operator-init', 'operator-rotate-pool-0', 'operator-rotate-pool-1', 'operator-rotate-admission']
    names += ['operator-serve-'+str(i) for i in range(1, 4 if case == 'A' else 3)]
    need(type(ops) is list and len(ops) == len(names), 'exact original operator generations')
    for i, (name, op) in enumerate(zip(names, ops)):
        keys(op, 'generation command pid parent_pid pgid joined intentional_crash unexpected_exit signal signal_count exit_code cleanup_error io_error frames')
        need(op['generation'] == name and op['command'] == ('init' if i == 0 else 'rotate' if i < 4 else 'serve'), 'same original command')
        pid(op)
        for k in ('unexpected_exit', 'cleanup_error', 'io_error'): truth(op[k], False)
        crash = name == 'operator-serve-1'
        truth(op['intentional_crash'], crash)
        need(op['exit_code'] == (-1 if crash else 0), 'exact command exit')
        need(op['signal'] == ('' if i < 4 else 'SIGKILL' if crash else 'SIGTERM') and op['signal_count'] == (0 if i < 4 else 1), 'one intentional crash; other original clean shutdowns')
        frames = public.lines(name+'-stdout.jsonl', 256, 1 << 20)
        need(1 <= len(frames) <= 256, 'original command output')
        same(frames, op['frames'], 'exact captured operator frames')
        need(public.raw(name+'-stderr.log', 1 << 20) == b'', 'empty operator stderr')
        for frame in frames:
            keys(frame, 'version command observed_at_ms code', 'result')
            need(frame['version'] == 2 and frame['command'] == op['command'] and frame['code'] in ('complete', 'listening', 'status', 'busy', 'unavailable'), 'closed operator status')
            integer(frame['observed_at_ms'], report['start_ms'])
            if frame['code'] == 'listening':
                need(frame['result']['origin'] == selection['setup']['base']['ingress']['origin'], 'original selected public listener')
            if frame['code'] == 'status':
                need(frame['result']['setup_id'] == report['setup_id'], 'same original runtime status')
        if i < 4:
            need(len(frames) == 1 and frames[0]['code'] == 'complete', 'one-shot original command success')
        else:
            need(any(f['code'] == 'listening' for f in frames), 'actual original listener startup')
    clis = report['cli_generations']
    need(type(clis) is list and len(clis) == (3 if case == 'A' else 1), 'exact original CLI generations')
    status_names = 'version observed_at_ms config_id state worker_state code records pending'
    zero_status = dict(version=0, observed_at_ms=0, config_id='', state='', worker_state='', code='', records=0, pending=0)
    for i, c in enumerate(clis, 1):
        keys(c, 'pid parent_pid pgid joined exit_code signal signal_count forced_kill unexpected_exit wait_error stdout_joined stderr_joined io_error status_count config_id last_status')
        pid(c)
        for k in ('forced_kill', 'unexpected_exit', 'wait_error', 'io_error'): truth(c[k], False)
        for k in ('stdout_joined', 'stderr_joined'): truth(c[k])
        need(c['exit_code'] == 0 and c['signal'] == 'SIGTERM' and c['signal_count'] == 1 and c['config_id'] == report['remote_id'], 'clean original CLI stop')
        frames = public.lines('cli-'+str(i)+'-stdout.jsonl', 1024, 512)
        need(len(frames) == c['status_count'] and (i != 3 or len(frames) > 0), 'exact CLI output count')
        same(c['last_status'], frames[-1] if frames else zero_status, 'same final emitted status')
        need(public.raw('cli-'+str(i)+'-stderr.log', 1 << 20) == b'', 'empty CLI stderr')
        last = 0
        for frame in frames:
            keys(frame, status_names, ordered=True)
            need(frame['version'] == 2 and frame['config_id'] == report['remote_id'] and frame['state'] == 'signed_in', 'original signed-in selected CLI')
            need(frame['worker_state'] in ('ready', 'stopped', 'failed', 'paused', 'preparing', 'retrying', 'hashing', 'delivering', 'unavailable')
                 and frame['code'] in ('', 'unavailable', 'outcome_unknown', 'busy', 'capacity', 'rate_limited'), 'closed public worker status')
            integer(frame['records'], last, 64); last = frame['records']
            integer(frame['pending'], 0, last); integer(frame['observed_at_ms'], report['start_ms'])
    nodes = report['node_cleanup']
    need(type(nodes) is list and len(nodes) == 2, 'two original normal nodes')
    for n in nodes:
        keys(n, 'pid parent_pid pgid group_error joined forced_kill unexpected_exit wait_error stop_rpc_error log_close_error clean')
        pid(n)
        for k in ('group_error', 'forced_kill', 'unexpected_exit', 'stop_rpc_error', 'log_close_error'): truth(n[k], False)
        need(n['wait_error'] == '', 'original node exit')
        truth(n['clean'])
    need(report['intentional_crash_count'] == 1, 'exactly one intentional original process crash')
    return ops, clis


def relay(v, request_id, start, original_response=None, lost=False, held=False):
    keys(v, 'request_id lost held response_sha256 response events error')
    need(v['request_id'] == request_id, 'original first request at relay')
    truth(v['error'], False); truth(v['lost'], lost); truth(v['held'], held)
    events = v['events']
    need(type(events) is list and 1 <= len(events) <= 128, 'bounded actual request events')
    losses = []
    for e in events:
        keys(e, 'request_id path at_ms forwarded status response_sha256 lost', ordered=True)
        hx(e['request_id']); integer(e['at_ms'], start); integer(e['status'], 0, 599)
        need(e['path'] in ('/pool/v3/issue', '/pool/v3/issue-lookup'), 'only selected exact request route')
        truth(e['forwarded'], e['forwarded']); truth(e['lost'], e['lost'])
        if e['response_sha256']: hx(e['response_sha256'])
        if e['lost']:
            losses.append(e)
            need(e['request_id'] == request_id and e['forwarded'] and e['status'] == 200, 'loss after real successful original server reply')
    if lost:
        need(len(losses) == 1, 'one exact lost successful reply')
        same(v['response'], original_response, 'exact withheld response retained')
        expected = sha(canonical(original_response)+b'\n')
        need(v['response_sha256'] == losses[0]['response_sha256'] == expected, 'original HTTP response bytes')
    else:
        need(not losses and v['response_sha256'] == sha(b''), 'no hidden withheld B reply')
    return events


def parse_transaction(raw_hex):
    """Strict canonical native SegWit parser; does not pretend to verify ECDSA."""
    raw = rawhex(raw_hex, 400000)
    pos = 0
    def take(n):
        nonlocal pos
        need(0 <= n <= len(raw)-pos, 'transaction truncation')
        b = raw[pos:pos+n]; pos += n
        return b
    def varint():
        first = take(1)[0]
        if first < 253: return first
        width = {253: 2, 254: 4, 255: 8}[first]
        n = int.from_bytes(take(width), 'little')
        need(n >= {253: 253, 254: 65536, 255: 4294967296}[first], 'minimal compact integer')
        return n
    version = take(4)
    need(int.from_bytes(version, 'little', signed=True) == 2 and take(2) == b'\x00\x01', 'native signed transaction version/witness')
    input_start = pos
    n = varint(); need(n in (1, 2), 'independent funding count')
    inputs = []
    for _ in range(n):
        txid = take(32)[::-1].hex(); index = int.from_bytes(take(4), 'little')
        need(varint() == 0, 'native empty scriptSig')
        sequence = int.from_bytes(take(4), 'little')
        inputs.append(dict(txid=txid, index=index))
        need(sequence == 0xffffffff, 'original final input sequence')
    need(len({(x['txid'], x['index']) for x in inputs}) == n, 'distinct funding outpoints')
    count = varint(); need(count == 2, 'recipient plus change only')
    outputs = []
    for _ in range(count):
        atoms = int.from_bytes(take(8), 'little', signed=True); integer(atoms, 1, (1 << 63)-1)
        length = varint(); need(length == 22, 'native output script')
        script = take(length); need(script[:2] == b'\x00\x14', 'native P2WPKH output')
        outputs.append(dict(atoms=atoms, script=script.hex()))
    witness_start = pos
    for _ in range(n):
        need(varint() == 2, 'native signature/public-key witness')
        size = varint(); need(9 <= size <= 73, 'bounded signature')
        signature = take(size); need(signature[0] == 0x30 and signature[-1] == 1, 'DER and SIGHASH_ALL')
        need(varint() == 33, 'compressed public key witness')
        need(take(33)[0] in (2, 3), 'compressed public key')
    locktime = take(4); need(locktime == b'\0'*4 and pos == len(raw), 'exact locktime and no trailing bytes')
    stripped = version+raw[input_start:witness_start]+locktime
    txid = hashlib.sha256(hashlib.sha256(stripped).digest()).digest()[::-1].hex()
    return dict(txid=txid, inputs=inputs, outputs=outputs, stripped_bytes=len(stripped),
                raw_bytes=len(raw), weight=len(stripped)*3+len(raw), raw_sha256=sha(raw))


def snapshot(v, c, remote_id, count, pending):
    keys(v, 'version config_id state account view remote', 'error')
    need(v['version'] == 2 and v['config_id'] == remote_id and v['state'] == 'signed_in' and v.get('error', '') == '', 'original stopped signed-in controller')
    view = keys(v['view'], 'version view_id account_authority_id account_id generation fixture root_public_key gym_public_key gym_id', ordered=True)
    expected = c['expected_local']['recipient']['config']; ident = expected['identity']
    need(view['version'] == 1 and view['account_authority_id'] == expected['account_authority_id'] and view['account_id'] == expected['account_id']
         and view['fixture'] == expected['fixture'] and view['root_public_key'] == ident['root_public_key'] and view['gym_public_key'] == ident['gym_public_key'] and view['gym_id'] == ident['gym_id'], 'original schema14 account/vault/recipient correlation')
    text(view['view_id'], 256); need(view['view_id'] != '', 'original opaque view correlation'); integer(view['generation'], 1)
    r = keys(v['remote'], 'version config_id view source_binding_id original_source_binding_id state worker_state last_error attempts hashes_per_second_milli records pending wallet_state wallet_height spendable_atoms immature_atoms unconfirmed_atoms payout_address', ordered=True)
    same(r['view'], view, 'same original view in attachment')
    need(r['version'] == 2 and r['config_id'] == remote_id and r['source_binding_id'] == c['miners']['source_id'] and r['original_source_binding_id'] == c['expected_local']['original_source_binding_id']
         and r['payout_address'] == c['expected_local']['recipient']['recipient']['address'], 'selected remote versus preserved local source')
    need(r['state'] == 'stopped' and r['attempts'] == r['hashes_per_second_milli'] == 0 and r['records'] == count and r['pending'] == pending, 'no Start on stopped reopen')
    need(r['last_error'] in ('', 'busy', 'rate_limited', 'capacity', 'outcome_unknown', 'unavailable') and r['wallet_state'] in ('unavailable', 'syncing', 'ready'), 'closed snapshot scalar state')
    for k in ('wallet_height', 'spendable_atoms', 'immature_atoms', 'unconfirmed_atoms'): integer(r[k], 0, (1 << 63)-1)
    return r


def receipt_base(v):
    r = copy.deepcopy(v)
    r['block_state'], r['block_observed_at_ms'] = '', 0
    return r


def worker_rows(v, gap, s):
    keys(v, 'outbox rows accepted authorizations accepted_fighters pending_fighters lookup_missing', ordered=True)
    box = outbox(v['outbox'])
    rows, accepted = v['rows'], v['accepted']
    need(type(rows) is list and len(rows) == box['records'] and 1 <= len(rows) <= 63 and type(accepted) is list and len(accepted) in (1, 2), 'finite complete worker records and rewards')
    signatures, births, pending = 0, 0, 0
    row_map = {}
    for index, r in enumerate(rows, 1):
        keys(r, 'version sequence work nonce work_hash duplicate_key pool_win pool_state fighter next_attempt_ms retry_count', 'pool_receipt last_error')
        need(r['version'] == 2 and r['sequence'] == index, 'contiguous daily saved entry')
        integer(r['nonce'], 0, 0xffffffff); hx(r['work_hash']); hx(r['duplicate_key'])
        work = keys(r['work'], 'job template', ordered=True)
        if work['job']['id'] == gap['job']['id']:
            same(work, dict(job=gap['job'], template=gap['template']), 'retained original recovered work unchanged')
        f = r['fighter']; need(type(f) is dict, 'original prospect candidate evidence')
        need(f['job'] == work['job'] and f['work_hash'] == r['work_hash'] and f['prospect_win'] is True, 'same saved fighter work')
        if 'authorization' in f:
            auth = f['authorization']; signatures += 1
            need(auth['public_key'] == s['miners']['enrollments'][0]['public_key'], 'original vault signing key')
        state = f['admission_state']
        if state == 'accepted':
            need('receipt' in f and 'authorization' in f, 'actual acknowledged fighter')
            rec = f['receipt']['record']
            same(rec['authorization'], f['authorization'], 'saved authorization receipt')
            same(rec['job'], f['job'], 'saved fighter original job')
            same(rec['proof'], f['candidate']['proof'], 'saved fighter proof')
            births += 1
        elif state in ('pending', 'needs-signature'):
            need('receipt' not in f and f.get('last_error', '') in ('', 'capacity', 'busy', 'rate_limited', 'outcome_unknown', 'unavailable'), 'retained interrupted admission')
            pending += 1
        else:
            need(state == 'expired' and f.get('last_error') == 'expired' and 'receipt' not in f, 'exact observed expiry only')
        if not r['pool_win']:
            need('pool_receipt' not in r, 'prospect-only never credit')
        row_map[(work['job']['id'], r['nonce'])] = r
    need(signatures == v['authorizations'] > 0 and births == v['accepted_fighters'] and pending == v['pending_fighters'], 'complete original signature/admission counts')
    need(len(row_map) == len(rows), 'unique saved job/nonce')
    missing = v['lookup_missing']; need(type(missing) is list and len(missing) == len(set(missing)), 'exact missing lookup list')
    for seq in missing:
        r = rows[integer(seq, 1, len(rows))-1]
        need(r['pool_win'] and r['pool_state'] == 'pending', 'only retained pending may be not found')
    found_original = False
    for i, r in enumerate(accepted, 1):
        need(r['version'] == 1 and r['accepted_sequence'] == i and r['pool'] is True and r['block'] is True and r['gym_id'] == s['miners']['enrollments'][0]['gym_id'], 'complete canonical accepted original prefix')
        need(r['pool_target'] == s['pool']['pool_target'], 'original fixed real block target')
        integer(r['block_height'], 3, 4)
        key = (r['job_id'], r['nonce']); need(key in row_map, 'source receipt belongs to retained original entry')
        saved = row_map[key]
        need(saved['pool_win'] and saved['work_hash'] == r['work_hash'] and saved['duplicate_key'] == r['duplicate_key'], 'same original winning proof')
        if 'pool_receipt' in saved: same(receipt_base(saved['pool_receipt']), receipt_base(r), 'exact durable receipt')
        found_original |= r['job_id'] == gap['job']['id']
    need(found_original, 'original recovered job consumed through canonical receipt')
    return len(accepted)


def settled_balance(b, gym, paid, strings=False):
    keys(b, 'gym_id confirmed available reserved_atoms paid_atoms reason', ordered=True)
    need(b['gym_id'] == gym and b['confirmed'] == ZERO and b['available'] == ZERO and b['reason'] == '', 'complete zero remaining miner balance')
    need(b['reserved_atoms'] == ('0' if strings else 0) and b['paid_atoms'] == (str(paid) if strings else paid), 'exact paid/reserved money')


def financial_payment(f, evidence, s, econ, payment, tx, due, final_height):
    n = econ['accepted_rewards']; en = s['miners']['enrollments'][0]
    accepted = evidence['accepted']
    need(f['accepted'] == n and len(f['entries']) == len(f['rounds']) == len(f['credits']) == n, 'one credit per complete canonical winner')
    for original, entry in zip(accepted, f['entries']):
        same(receipt_base(original), receipt_base(entry['share']), 'complete authenticated accepted prefix in full audit')
    h, body = f['header'], f['body']
    need(h['credits'] == n and h['batches'] == 1 and h['events'] == n+1 and h['pending_batch'] == 0 and f['delivery'] == 'confirmed', 'one original confirmed batch')
    need(body['version'] == 2 and body['sequence'] == 1 and body['txid'] == payment['txid'] and body['raw'] == payment['raw'] and body['fee_atoms'] == econ['fee_atoms'], 'same exact journaled payment body')
    same(body['inputs'], tx['inputs'], 'signed exact original funding order')
    same(body['payments'], [dict(gym_id=en['gym_id'], script=en['payee_script'], atoms=econ['miner_atoms'])], 'one exact original recipient output')
    change = keys(body['change'], 'index atoms script claim_count claims_digest', ordered=True)
    need(change['index'] == 1 and change['atoms'] == econ['change_atoms'] and change['script'] == s['settlement']['change_script'] and change['claim_count'] == n, 'same complete operator change claims')
    hx(change['claims_digest'])
    day = dt.datetime.fromtimestamp(due/1000, dt.timezone.utc).strftime('%Y-%m-%d')
    need(body['day'] == day and due <= body['created_at_ms'] <= payment['broadcast_at_ms'], 'same real scheduler day/due')
    credit_points, round_hashes = set(), set()
    for record, credit in zip(f['rounds'], f['credits']):
        keys(record, 'round observation confirmation', ordered=True)
        r = record['round']; block_hash = r['block_hash']; hx(block_hash)
        need(block_hash not in round_hashes, 'one immutable round per winner'); round_hashes.add(block_hash)
        need(r['sequence'] in range(1, n+1) and r['reward_atoms'] == 1000000000 and r['pool_target'] == s['pool']['pool_target'] and r['payout_address'] == s['pool']['payout_address'], 'original round economic source')
        matching = [a for a in accepted if a['work_hash'] == block_hash]
        need(len(matching) == 1, 'round from exact actual accepted block')
        a = matching[0]
        need(r['block_height'] == a['block_height'] and r['first_accepted'] == r['last_accepted'] == a['accepted_sequence'] and r['share_keys'] == [a['duplicate_key']], 'immutable single-winner membership')
        need(len(r['allocations']) == 1, 'one original gym allocation')
        allocation = r['allocations'][0]
        need(allocation == dict(gym_id=en['gym_id'], shares=1, numerator='1000000000', denominator='1'), 'full gross original allocation')
        same(receipt_base(r['winner']['share']), receipt_base(a), 'same round winner')
        confirmation = record['confirmation']; need(type(confirmation) is dict, 'original locked confirmation')
        need(confirmation['canonical'] is True and confirmation['block_hash'] == block_hash and confirmation['block_height'] == a['block_height']
             and confirmation['descendants'] >= 60 and confirmation['tip_height']-confirmation['block_height'] == confirmation['descendants']
             and confirmation['tip_height'] <= final_height, 'original actual 60-descendant lock')
        hx(confirmation['tip_hash'])
        keys(credit, 'round_hash round_digest coin', ordered=True)
        need(credit['round_hash'] == block_hash, 'credit same locked round')
        hx(credit['round_digest'])
        coin = keys(credit['coin'], 'txid index atoms script coinbase block_hash block_height claims', ordered=True)
        hx(coin['txid']); integer(coin['index'], 0, 0xffffffff)
        point = (coin['txid'], coin['index']); need(point not in credit_points, 'distinct original funding'); credit_points.add(point)
        need(coin['coinbase'] is True and coin['atoms'] == 1000000000 and coin['script'] == s['settlement']['change_script']
             and coin['block_hash'] == block_hash and coin['block_height'] == a['block_height'], 'actual original reward provenance')
        claims = coin['claims']; need(type(claims) is list and len(claims) == 2, 'complete original 99/1 claims')
        wanted = {en['gym_id']: '990000000', '': '10000000'}
        for claim in claims:
            keys(claim, 'round_hash round_height gym_id amount', ordered=True)
            need(claim['round_hash'] == block_hash and claim['round_height'] == a['block_height'] and claim['gym_id'] in wanted
                 and claim['amount'] == dict(numerator=wanted.pop(claim['gym_id']), denominator='1'), 'exact independent claim')
        need(not wanted, 'no omitted original claim')
    need(credit_points == {(x['txid'], x['index']) for x in tx['inputs']}, 'only all original accepted funding used')
    # Every issuer birth is explained by an exact retained original authorization.
    acknowledged = 0
    for saved in f['admissions']:
        matches = []
        for row in evidence['rows']:
            fighter = row['fighter']
            if fighter.get('authorization') == saved['authorization'] and fighter['job'] == saved['job'] and fighter['candidate']['proof'] == saved['proof']:
                matches.append(fighter)
        need(len(matches) == 1, 'no unexplained issuer admission')
        fighter = matches[0]
        if 'receipt' in fighter:
            same(fighter['receipt']['record'], saved, 'same acknowledged original birth'); acknowledged += 1
        else:
            need(fighter['admission_state'] == 'pending', 'uncertain admission remains pending')
    need(acknowledged == evidence['accepted_fighters'], 'all acknowledged original births accounted')
    report = keys(f['report'], 'day observed_at_ms state balances', 'txid reason')
    need(report['day'] == day and report['observed_at_ms'] >= body['created_at_ms'] and len(report['balances']) == 2, 'same full original saved report')
    for i, balance in enumerate(report['balances']):
        settled_balance(balance, s['miners']['enrollments'][i]['gym_id'], econ['miner_atoms'] if i == 0 else 0)


def money_pages(phase, s, econ, body, confirmed):
    balances, pages = phase['balances'], phase['pages']
    need(type(balances) is list and type(pages) is list and len(balances) == len(pages) == 2, 'all selected gyms and full pages')
    for i, (v, p) in enumerate(zip(balances, pages)):
        keys(v, 'version binding_id settlement_config_id recipient_script balance observed_at_ms', ordered=True)
        keys(p, 'version binding_id settlement_config_id revision high_batch before_batch next_before done recipient_script balance observed_at_ms items', ordered=True)
        en = s['miners']['enrollments'][i]
        for x in (v, p):
            need(x['version'] == 2 and x['binding_id'] == s['miners']['source_id'] and x['settlement_config_id'] == s['miners']['settlement_config_id'] and x['recipient_script'] == en['payee_script'], 'full authenticated public money binding')
            integer(x['observed_at_ms'], confirmed)
            settled_balance(x['balance'], en['gym_id'], econ['miner_atoms'] if i == 0 else 0, strings=True)
        need(type(p['revision']) is str and re.fullmatch('[1-9][0-9]*', p['revision']) and int(p['revision']) <= (1 << 64)-1, 'captured exact revision')
        need(p['high_batch'] == '1' and p['before_batch'] == '2' and p['next_before'] == '0' and p['done'] is True, 'complete one-batch captured page')
        if i == 0:
            expected = dict(batch_sequence='1', payment=dict(day=body['day'], created_at_ms=body['created_at_ms'], txid=body['txid'], output_index=0, atoms=econ['miner_atoms'], recorded_state='confirmed'))
            same(p['items'], [expected], 'exact original dated payment output')
        else:
            need(p['items'] == [], 'other gym complete empty payment history')


def money_refusals(v, start, scenario_end):
    keys(v, 'complete deadline_ms refusal_count refusals last_refusal', ordered=True)
    truth(v['complete']); integer(v['deadline_ms'], start, scenario_end)
    n = integer(v['refusal_count'], 0, 2400)
    need(type(v['refusals']) is list and len(v['refusals']) == min(n, 32), 'bounded retained public refusal prefix')
    if n == 0:
        need(v['last_refusal'] is None, 'no invented last refusal')
    else:
        values = v['refusals']+[v['last_refusal']]
        for r in values:
            keys(r, 'read gym_index at_ms code outcome_unknown', ordered=True)
            need(r['read'] in ('balance', 'payouts') and r['gym_index'] in (0, 1) and r['code'] in ('busy', 'rate_limited', 'unavailable') and r['outcome_unknown'] is False, 'only definite pure-read retries')
            integer(r['at_ms'], start, v['deadline_ms'])
        if n <= 32: same(v['last_refusal'], v['refusals'][-1], 'same actual last refusal')
    # The bound helper fixes the child at 120 seconds. The public schema has
    # no separate call-start timestamp, so do not invent a maximum phase-write
    # delay. Every observed refusal must still fit that one retained window.
    if n:
        need(v['deadline_ms']-v['refusals'][0]['at_ms'] <= 120000, 'one retained money-read window')


def check(owned: Path, case: str, probe_pid: int):
    """Return exact absolute PUBLIC artifact path -> {bytes,sha256}; raise on refusal."""
    need(case in ('A', 'B'), 'explicit case')
    reference_raw = read_regular(REFERENCE, 1 << 20)
    need(len(reference_raw) == REFERENCE_BYTES and sha(reference_raw) == REFERENCE_SHA, 'pre-execution qualified reference')
    ref = parse(reference_raw)
    need(ref['version'] == 1 and ref['ordinary_maximum'] == 2 and [e['accepted_rewards'] for e in ref['economics']] == [1, 2], 'frozen finite reference')
    public = Public(owned)
    report = public.json('actual-report.json')
    common = 'version case passed completed start_ms scope public_seed_mining due_ms setup_id remote_id issue_profile_id initial_height initial_tip request_id intent_id job_id issued_at_ms expires_at_ms tls_relay send_error sends send_attempts operator_generations cli_generations node_cleanup intentional_crash_count cleanup_errors clean_join seconds'
    extra_a = 'accepted_rewards accepted_fighters pending_fighters original_signatures stopped_reopen paid_atoms fee_atoms send_count attempts final_audit_unchanged final_height miner_atoms scheduled_payment original_wallet_received'
    extra_b = 'expiry_ms resumed_at_ms state works submissions accepted births credits batches send_count final_height read_only_unchanged'
    keys(report, common+' '+(extra_a if case == 'A' else extra_b))
    incompatible = ['04-expired-terminal.json'] if case == 'A' else [
        '04-response-loss.json', '05-completed-source.json', '06-replayed-work.json',
        '07-worker-closed.json', '08-stopped-reopen.json', '09-mature.json',
        '10-confirmed-payment.json', '11-payment-wallet.json', 'outbox-after.json',
        'historical-replay.json', 'money-read-evidence.json']
    for name in incompatible:
        need(not os.path.lexists(public.root / name), 'foreign-case public success artifact')
    need(report['version'] == 1 and report['case'] == case and report['scope'] == 'finite original request recovery on standalone operator and headless CLI', 'exact actual scope')
    for k in ('passed', 'completed', 'clean_join'): truth(report[k])
    for k in ('public_seed_mining', 'send_error'): truth(report[k], False)
    need(report['cleanup_errors'] == [] and type(report['seconds']) in (int, float) and 0 < report['seconds'] <= 1080, 'all original cleanup and outer bound')
    for k in ('initial_tip', 'request_id', 'intent_id', 'job_id'): hx(report[k])
    selection = public.json('01-selection.json')
    s, c, pid = selected(selection, report, ref)
    request_id = report['request_id']
    ops, clis = processes(public, report, case, probe_pid, selection)
    phase2 = keys(public.json('02-gap.json'), 'barrier audit operator')
    gap = audit(phase2['audit'], s, pid, request_id, public.root)
    same(phase2['operator'], ops[4], 'same exact intentionally killed original operator')
    need(gap['source_requests'] == gap['issuer_requests'] == gap['source_templates'] == gap['issuer_inverses'] == gap['financial']['authority_jobs'] == 1
         and gap['source_inverses'] == 0 and gap['source_request']['state'] == 'pending' and gap['issuer_request']['state'] == 'issued' and gap['work'] is None, 'actual issuer-committed/source-unretained gap')
    empty_financial(gap['financial'])
    job = gap['job']
    need(job['id'] == report['job_id'] and gap['source_request']['intent_id'] == report['intent_id'] and job['issued_at_ms'] == report['issued_at_ms'] and job['expires_at_ms'] == report['expires_at_ms'], 'exact reported original job/intent/lifetime')
    barrier = keys(phase2['barrier'], 'block sha256 at_ms', ordered=True)
    proposal = rawhex(barrier['block'], 4 << 20)
    need(len(proposal) >= 81 and sha(proposal) == barrier['sha256'] and job['issued_at_ms'] <= barrier['at_ms'] < job['expires_at_ms'], 'real original successful proposal barrier bytes/time')
    need(int.from_bytes(proposal[72:76], 'little') == 0x207fffff, 'unchanged normal target')
    local = outbox(public.json('03-local-gap.json'), request_id, pending=True)
    done = public.json('13-complete.json')
    for k, v in done.items():
        need(k in report and report[k] == v, 'same final public assertion: '+k)
    if case == 'B':
        keys(done, 'completed case request_id job_id expiry_ms resumed_at_ms state works submissions accepted births credits batches send_count final_height read_only_unchanged')
        terminal = keys(public.json('04-expired-terminal.json'), 'expired_observed_at_ms original_expires_at_ms terminal replays')
        need(terminal['original_expires_at_ms'] == job['expires_at_ms'] and terminal['expired_observed_at_ms'] >= job['expires_at_ms'], 'actual original wall expiry')
        r = response(terminal['terminal'], s, pid, request_id, gap, 'issued_unused')
        need(type(terminal['replays']) is list and len(terminal['replays']) == 2, 'lookup and resume terminal replay')
        for replay in terminal['replays']: same(replay, r, 'first terminal exact and permanent')
        final_phase = keys(public.json('12-final-audit.json'), 'audit outbox local_terminal_consumption')
        final = audit(final_phase['audit'], s, pid, request_id, public.root)
        empty_financial(final['financial'])
        for k in ('job', 'template', 'issuer_request'): same(final[k], gap[k], 'original never-published '+k)
        need(final['source_requests'] == final['issuer_requests'] == final['source_templates'] == final['source_inverses'] == final['issuer_inverses'] == 1, 'one original retained unused request/template/reverse')
        need(final['source_request']['state'] == 'issued_unused' and final['source_request']['resolved_at_ms'] == r['result']['resolved_at_ms'] and final['source_request']['reason'] == r['result']['reason'], 'same durable terminal result')
        same(final_phase['outbox'], local, 'stopped original null request remains exact; no fabricated local terminal consumption')
        need(final_phase['local_terminal_consumption'] == 'separate focused original worker control', 'explicitly scoped separate local control')
        need(all(done[k] == 0 for k in ('works', 'submissions', 'accepted', 'births', 'credits', 'batches', 'send_count')) and done['final_height'] == 2
             and done['expiry_ms'] == job['expires_at_ms'] and done['resumed_at_ms'] == r['result']['resolved_at_ms'] and done['state'] == 'issued_unused', 'terminal cannot mint work/money')
        truth(done['read_only_unchanged'])
        need(report['sends'] == [] and report['send_attempts'] == [], 'zero B broadcast attempts')
        relay(report['tls_relay'], request_id, report['start_ms'])
    else:
        keys(done, 'completed case request_id job_id accepted_rewards accepted_fighters pending_fighters original_signatures stopped_reopen paid_atoms fee_atoms send_count attempts final_audit_unchanged final_height')
        loss = public.json('04-response-loss.json')
        withheld = response(loss['response'], s, pid, request_id, gap, 'work')
        loss_events = relay(loss, request_id, report['start_ms'], withheld, lost=True, held=True)
        need(all(e['request_id'] == request_id for e in loss_events), 'no replacement request across gap/reply loss')
        completed_phase = keys(public.json('05-completed-source.json'), 'audit local')
        completed = audit(completed_phase['audit'], s, pid, request_id, public.root)
        need(completed['source_requests'] == completed['issuer_requests'] == completed['source_inverses'] == completed['issuer_inverses'] == completed['financial']['works'] == completed['financial']['authority_jobs'] == 1
             and completed['source_templates'] == 0 and completed['financial']['submissions'] == completed['financial']['accepted'] == completed['financial']['header']['credits'] == 0, 'exact one source completion; no hidden work consumption on lost reply')
        for k in ('job', 'template', 'issuer_request'): same(completed[k], gap[k], 'original completed '+k)
        same(completed['work'], withheld['result']['work'], 'withheld same durably completed work')
        need(completed['source_request']['state'] == 'work' and completed['source_request']['resolved_at_ms'] == withheld['result']['resolved_at_ms'], 'same original durable completion')
        same(completed_phase['local'], local, 'lost reply retains exact original null q')
        same(public.json('06-replayed-work.json'), withheld, 'exact real server replay')
        closed = keys(public.json('07-worker-closed.json'), 'rows request control cli')
        same(closed['request'], withheld, 'exact validated work persisted in original q')
        n = worker_rows(closed['rows'], gap, s)
        same(closed['control'], closed['rows']['outbox'], 'complete selected outbox controls')
        need(closed['control']['active_request_id'] != request_id, 'only validated completion clears original active request')
        same(closed['cli'], clis[2], 'same actual mining CLI joined before audit')
        econ = next(e for e in ref['economics'] if e['accepted_rewards'] == n)
        reopened = public.json('08-stopped-reopen.json')
        box = closed['control']; snapshot(reopened, c, report['remote_id'], box['records'], box['pending'])
        mature = keys(public.json('09-mature.json'), 'snapshot due_ms observed_at_ms height economics')
        same(mature['snapshot']['view'], reopened['view'], 'same original reopened view')
        mature_wallet = snapshot(mature['snapshot'], c, report['remote_id'], box['records'], box['pending'])
        need(mature_wallet['wallet_state'] == 'ready' and mature_wallet['wallet_height'] >= mature['height']
             and mature['height'] >= max(x['block_height'] for x in closed['rows']['accepted'])+100
             and mature['due_ms'] == report['due_ms'] and mature['observed_at_ms'] < report['due_ms'], 'actual maturity before unchanged original due')
        expected_econ = {k: econ[k] for k in ('accepted_rewards', 'credits', 'batches', 'input_count', 'output_count', 'gross_atoms', 'miner_atoms', 'fee_atoms', 'change_atoms')}
        same(mature['economics'], expected_econ, 'frozen independent reward branch')
        confirm = keys(public.json('10-confirmed-payment.json'), 'payment confirmation_block confirmed_at_ms')
        payment = keys(confirm['payment'], 'txid raw raw_sha256 input_count miner_atoms fee_atoms change_atoms broadcast_at_ms')
        tx = parse_transaction(payment['raw']); hx(confirm['confirmation_block'])
        need(payment['txid'] == tx['txid'] and payment['raw_sha256'] == tx['raw_sha256'] and payment['input_count'] == len(tx['inputs']) == n, 'independent actual signed bytes/txid/input count')
        need(payment['broadcast_at_ms'] >= report['due_ms'] and confirm['confirmed_at_ms'] >= payment['broadcast_at_ms'], 'actual due and subsequent confirmation')
        for k in ('miner_atoms', 'fee_atoms', 'change_atoms'): need(payment[k] == econ[k], 'independent exact '+k)
        same(tx['outputs'], [dict(atoms=econ['miner_atoms'], script=s['settlement']['destinations'][0]['script']), dict(atoms=econ['change_atoms'], script=s['settlement']['change_script'])], 'exact original payment and operator change')
        need(n*1000000000-sum(x['atoms'] for x in tx['outputs']) == econ['fee_atoms'] and tx['weight'] <= econ['shape']['weight'] and tx['raw_bytes'] <= econ['shape']['raw_bytes'], 'independent fee and conservative native shape')
        wallet = keys(public.json('11-payment-wallet.json'), 'payment balances pages wallet_before wallet_after confirmation_block')
        same(wallet['payment'], payment, 'same validated payment'); same(wallet['wallet_before'], mature['snapshot'], 'same before-payment original wallet')
        need(wallet['confirmation_block'] == confirm['confirmation_block'], 'same canonical confirmation block')
        after_wallet = snapshot(wallet['wallet_after'], c, report['remote_id'], box['records'], box['pending'])
        same(wallet['wallet_after']['view'], reopened['view'], 'same original wallet/view through payment')
        need(after_wallet['wallet_state'] == 'ready' and after_wallet['wallet_height'] >= mature['height']+1 and after_wallet['spendable_atoms']-mature_wallet['spendable_atoms'] == econ['miner_atoms'], 'original stopped wallet exact payment delta')
        same(public.json('outbox-after.json'), box, 'all q/e/h/p/meta/scan/cursor retained on stopped reopen')
        history = keys(public.json('historical-replay.json'), 'lookup resume observed_at_ms')
        need(history['observed_at_ms'] >= job['expires_at_ms'], 'actual original expiry passed')
        same(history['lookup'], withheld, 'completed lookup unchanged after expiry'); same(history['resume'], withheld, 'same ID resume unchanged after expiry')
        final = audit(public.json('12-final-audit.json'), s, pid, request_id, public.root)
        for k in ('job', 'template', 'issuer_request', 'source_request'): same(final[k], completed[k], 'final original retained '+k)
        financial_payment(final['financial'], closed['rows'], s, econ, payment, tx, report['due_ms'], done['final_height'])
        money_pages(wallet, s, econ, final['financial']['body'], confirm['confirmed_at_ms'])
        money_refusals(public.json('money-read-evidence.json'), confirm['confirmed_at_ms'], report['start_ms']+900000)
        sends, attempts = report['sends'], report['send_attempts']
        need(type(sends) is list and len(sends) == 1 and type(attempts) is list and 1 <= len(attempts) <= 16, 'exactly one actual successful broadcast')
        for call in sends+attempts:
            keys(call, 'raw txid at_ms', ordered=True)
            need(call['raw'] == payment['raw'] and call['at_ms'] >= report['due_ms'], 'only original signed broadcast bytes')
            # Failed pre-forward attempts may have no txid; successful send must.
            need(call['txid'] in ('', payment['txid']), 'no replacement transaction ID')
        need(sends[0]['txid'] == payment['txid'] and sends[0]['at_ms'] == payment['broadcast_at_ms'], 'exact successful original send')
        same(done['attempts'], attempts, 'same exact bounded attempts')
        need(done['accepted_rewards'] == n and done['accepted_fighters'] == closed['rows']['accepted_fighters'] and done['pending_fighters'] == closed['rows']['pending_fighters'] and done['original_signatures'] == closed['rows']['authorizations'], 'complete observed stop-race accounting')
        need(done['paid_atoms'] == report['miner_atoms'] == econ['miner_atoms'] and done['fee_atoms'] == econ['fee_atoms'] and done['send_count'] == 1 and 103 <= done['final_height'] <= 105, 'exact final payment/bounded chain')
        for k in ('stopped_reopen', 'final_audit_unchanged'): truth(done[k])
        truth(report['scheduled_payment']); truth(report['original_wallet_received'])
        events = relay(report['tls_relay'], request_id, report['start_ms'], withheld, lost=True)
        need(events[:len(loss_events)] == loss_events, 'retained original relay event prefix')
        need(sum(e['request_id'] == request_id and e['status'] == 200 and e['forwarded'] and not e['lost'] and e['response_sha256'] == loss['response_sha256'] for e in events) >= 3, 'actual identical initial and historical successful responses')
    need(done['completed'] is True and done['case'] == case and done['request_id'] == request_id and done['job_id'] == job['id'], 'exact completed original scenario')
    return public.finish()
