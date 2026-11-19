"""Independent Set66 conditional verifier, staged before input/outcome freeze.

Uses unchanged archived independent Python references for crypto/static/history
math. The sole work primitive is an unchanged standalone x/crypto Argon2id
executable importing no SLUG package. No new production implementation output
is used as an expected value. Import does not build/hash/run fixtures.
"""
import copy
import hashlib
import importlib.util
import json
from pathlib import Path
import re
import struct
import sys

from evidence_schema import (ARCHIVE, MAX_FILE, PIN, Refusal, Scanner,
                             canonical, need, parse_archive)

sys.dont_write_bytecode = True
HERE = Path(__file__).resolve().parent
MAX_TIME = 8640000000000000


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def le32(n):
    return struct.pack('<I', n)


def lp(raw):
    return le32(len(raw)) + raw


def domain_hash(domain, value):
    return sha(domain + lp(canonical(value)))


def hash_id(v):
    return type(v) is str and re.fullmatch('[0-9a-f]{64}', v) is not None


def integer(v, lo, hi):
    return type(v) is int and lo <= v <= hi


def load_module(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    sys.modules[name] = module
    spec.loader.exec_module(module)
    return module


def verify_inherited_inputs(repo_root=None):
    # Packaged reference locates its enclosing checkout. External preparation
    # passes --repo-root explicitly. No workspace/tmp path is an input contract.
    if repo_root is None:
        repo_root = next((parent for parent in HERE.parents
                          if (parent / 'go.mod').is_file()
                          and (parent / 'registry/testdata/manifest/reference').is_dir()), None)
    if repo_root is None:
        raise ValueError('explicit repository root required outside checkout')
    root = Path(repo_root).resolve()
    inventory = json.loads((HERE / 'dependency-manifest.json').read_bytes())
    for relative, row in inventory['files'].items():
        path = root / relative
        need(path.resolve().is_relative_to(root))
        raw = path.read_bytes()
        if len(raw) != row['bytes'] or sha(raw) != row['sha256']:
            raise ValueError('changed inherited input: ' + relative)
    return root, inventory


class ByteReader:
    """Canonical bounded transaction framing, independent of wire decoding."""
    def __init__(self, raw):
        self.raw, self.pos = raw, 0

    def take(self, n):
        need(integer(n, 0, len(self.raw) - self.pos))
        value = self.raw[self.pos:self.pos + n]
        self.pos += n
        return value

    def number(self, n):
        return int.from_bytes(self.take(n), 'little')

    def compact(self):
        tag = self.number(1)
        if tag < 253:
            return tag
        width, lower = {253: (2, 253), 254: (4, 65536), 255: (8, 1 << 32)}[tag]
        value = self.number(width)
        need(value >= lower)
        return value

    def varbytes(self, lo, hi):
        size = self.compact()
        need(lo <= size <= hi and size <= len(self.raw) - self.pos)
        return self.take(size)


def reserved_family(script):
    # Reserved OP_RETURN pushes keep their family status even if push length is
    # nonminimal or exceeds remaining bytes; such a script cannot match exactly.
    if len(script) < 2 or script[0] != 0x6a:
        return False
    op, start = script[1], 2
    if op <= 75:
        size = op
    elif op in (0x4c, 0x4d, 0x4e):
        width = {0x4c: 1, 0x4d: 2, 0x4e: 4}[op]
        if len(script) < 2 + width:
            return False
        size = int.from_bytes(script[2:2 + width], 'little')
        start += width
    else:
        return False
    return size >= 6 and script[start:].startswith(b'SLUGPR')


def coinbase(raw, commitment, height, height_prefix):
    need(type(raw) is bytes and 1 <= len(raw) <= 16384)
    reader = ByteReader(raw)
    reader.take(4)  # Existing transaction version, no new version policy.
    need(reader.compact() == 1)  # Stripped one-input coinbase; no witness marker.
    need(reader.take(32) == bytes(32) and reader.number(4) == 0xffffffff)
    script_sig = reader.varbytes(2, 100)
    need(script_sig.startswith(height_prefix(height)))
    reader.take(4)
    count = reader.compact()
    need(1 <= count <= 256)
    total, commitments = 0, 0
    for _ in range(count):
        value = reader.number(8)
        need(value <= 21000000 * 100000000 - total)
        total += value
        script = reader.varbytes(0, 16384)
        if reserved_family(script):
            commitments += 1
            need(value == 0 and script == commitment)
    reader.take(4)
    need(reader.pos == len(raw) and commitments == 1)


class Reference:
    def __init__(self, repo_root=None):
        root, inventory = verify_inherited_inputs(repo_root)
        roots = {name: root / relative for name, relative in inventory['roots'].items()}
        self.admission = load_module('evidence_archived_admission', roots['admission'] / 'reference_manifest_admission.py')
        self.names = load_module('evidence_archived_names', roots['name_v2'] / 'independent/reference_v2.py')
        history_root = roots['history']
        previous_common = sys.modules.get('common')
        common = load_module('common', history_root / 'common.py')
        try:
            self.history = load_module('evidence_archived_history', history_root / 'history_math.py')
        finally:
            if previous_common is None:
                sys.modules.pop('common', None)
            else:
                sys.modules['common'] = previous_common
        self.history_common = common
        self.jobs = self.admission.J
        self.descriptors = {}
        for descriptor in (self.jobs.CANDIDATE, self.names.APPROVAL['descriptor']):
            self.descriptors[self.jobs.descriptor_id(descriptor)] = copy.deepcopy(descriptor)
        # Explicit additive adapter for Pin2 and the exact approved descriptor2.
        # Old files stay byte-identical; arithmetic/event/row validators remain
        # the frozen history functions. No caller controls these overrides.
        self.history.pin = self.validate_pin
        self.history.authority_pin = self.validate_authority
        self.history.stable_id = self.stable_id

    def stable_id(self, pin):
        domain = b'SLUG candidate shared stable\0v1\0' if pin['version'] == 2 else b'SLUG candidate stable\0v1\0'
        return domain_hash(domain, pin)

    def descriptor(self, identifier):
        need(hash_id(identifier) and identifier in self.descriptors)
        return self.descriptors[identifier]

    def validate_authority(self, value):
        h = self.history
        v = h.fields(value, 'version network genesis authority_id descriptor_id')
        network, genesis = h.CANDIDATE_AUTH[:2]
        need(type(v['version']) is int and v['version'] == 5)
        need(type(v['network']) is int and v['network'] == network and v['genesis'] == genesis)
        need(hash_id(v['authority_id']))
        self.descriptor(v['descriptor_id'])
        return v

    def validate_pin(self, value):
        h = self.history
        v = h.fields(value, 'version account_authority_id account_id identity source condition_version projector_id')
        need(type(v['version']) is int and v['version'] in (1, 2))
        need(hash_id(v['account_authority_id']) and hash_id(v['account_id']))
        v['identity'] = h.fields(v['identity'], h.ID_FIELDS)
        need(canonical(v['identity']) in h.FIXTURE_IDENTITIES)
        need(type(v['condition_version']) is int and v['condition_version'] == 1)
        need(v['projector_id'] == 'slug-fighter-condition-candidate-v1')
        v['source'] = self.validate_authority(v['source'])
        need(v['source']['authority_id'] != v['account_authority_id'])
        need(v['source']['network'] == v['identity']['network'] and v['source']['genesis'] == v['identity']['genesis'])
        need(len(canonical(v)) <= 4096)
        return v

    def job(self, job):
        j = self.jobs
        descriptor = self.descriptor(job['descriptor_id'])
        need(job['version'] == 2 and job['rule_version'] == 1)
        need(descriptor['dna_version'] == job['context']['dna_version'])
        # The old reference's private no-support path is only framing math.
        # Exact support was enforced above against this frozen two-tuple map.
        expected = j.new_job(job['context'], job['authority_id'], job['descriptor_id'],
                             job['public_key'], job['challenge'], job['issued_at_ms'],
                             require_support=False)
        need(canonical(expected) == canonical(job))
        need(len(canonical(job)) <= 8192 and len(canonical(job['context'])) <= 4096)

    def proof_structure(self, job, proof):
        j = self.jobs
        self.admission.proof_binding(job, proof)
        need(len(canonical(proof)) <= 49152)
        # Unlike the old fixture-only helper's substring check, parse all
        # stripped transaction framing and reserved commitment outputs.
        coinbase(bytes.fromhex(proof['coinbase']), j.commitment(job),
                 job['context']['parent_height'] + 1, j.TPL.script_number_push)

    def signature(self, record):
        a, j = self.admission, self.jobs
        job, proof, auth = record['job'], record['proof'], record['authorization']
        need(auth['version'] == 2 and auth['public_key'] == job['public_key'])
        r, s = a.parse_signature(auth['signature'])
        _, point = j.public_key(auth['public_key'])
        digest = j.OLD.admission_digest(job, proof, j.PROOF.policy_id(job['context']))
        need(j.AUTH.verify(point, digest, r, s))
        return digest.hex()

    def appearance(self, work_hash, descriptor_id):
        descriptor = self.descriptor(descriptor_id)
        if descriptor['manifest_version'] == 1:
            manifest, _, _, binding, _ = self.admission.appearance(work_hash)
        else:
            manifest = self.names.generate(work_hash)
            raw = self.names.canonical(manifest)
            svg = self.names.render(raw)
            binding = dict(version=1, descriptor_id=descriptor_id,
                           manifest_digest=sha(self.names.MANIFEST_DOMAIN + raw),
                           portrait_digest=sha(self.names.SVG_DOMAIN + svg))
        need(binding['descriptor_id'] == descriptor_id)
        return manifest, binding

    def eligibility(self, record, verified):
        e, context = record['eligibility'], record['context']
        need(e['rule_version'] == 2)
        if e['source'] == 'current-parent':
            need(e['hash'] == context['parent_hash'] and e['height'] == context['parent_height'])
        elif e['source'] == 'canonical-winning-block':
            need(e['hash'] == verified['work_hash'] and e['height'] == context['parent_height'] + 1)
            need(int(verified['work_hash'], 16) <= self.admission.target(context['expected_bits']))
        else:
            raise Refusal()

    def supplied_history(self, capture, rows):
        h = self.history
        h.capture(copy.deepcopy(capture))
        need(len(rows) == capture['event_count'])
        awards, results = set(), set()
        last = None
        for index, row in enumerate(rows):
            h.history_row(copy.deepcopy(row), capture)
            need(row['ordinal'] == len(rows) - index)
            e = row['event']
            key = (e['effective_at_ms'], e['award_id'])
            need(last is None or key < last)
            last = key
            need(e['award_id'] not in awards and e['result_key'] not in results)
            awards.add(e['award_id'])
            results.add(e['result_key'])
        total = h.zero()
        for row in reversed(rows):
            total = h.apply(total, row['earned'])
            need(total == row['totals_after'])
        need(total == capture['totals'])
        # Deliberately do not regenerate CareerDigest from fighter-only rows.

    def validate_shape(self, expected, archive):
        try:
            expected = self.validate_pin(copy.deepcopy(expected))
            # Typed callers get the same shape/byte enforcement as raw callers.
            raw = canonical(archive)
            archive = parse_archive(raw)
            r, c = archive['admission'], archive['capture']
            need(c['status']['pin'] == expected, 'source_mismatch')
            need(c['status']['store_id'] == self.stable_id(expected), 'source_mismatch')
            source, identity = expected['source'], expected['identity']
            job, proof, verified = r['job'], r['proof'], r['verified']
            need(r['version'] == 5 and 1 <= r['sequence'] <= 1000)
            self.job(job)
            need(job['authority_id'] == source['authority_id'] and job['descriptor_id'] == source['descriptor_id'], 'source_mismatch')
            need(job['context']['network'] == source['network'] and job['context']['genesis'] == source['genesis'], 'source_mismatch')
            need(job['public_key'] == identity['gym_public_key'] and job['gym_id'] == identity['gym_id'], 'source_mismatch')
            need(r['context'] == job['context'])
            need(proof['version'] == 2 and proof['job_id'] == job['id'] and proof['gym_id'] == job['gym_id'])
            self.proof_structure(job, proof)
            self.signature(r)
            b = r['birth']
            need(b['rule_version'] == job['rule_version'] == 1 and b['born_at_ms'] == job['issued_at_ms'])
            need(job['issued_at_ms'] <= b['accepted_at_ms'] < job['expires_at_ms'])
            need(hash_id(verified['work_hash']))
            supplied_expected = self.admission.verified(job, proof, verified['work_hash'])
            need(verified == supplied_expected)
            self.eligibility(r, supplied_expected)
            manifest, binding = self.appearance(verified['work_hash'], source['descriptor_id'])
            need(manifest['dna'] == verified['dna'] and binding == r['static_binding'])
            assignment = dict(version=1, sequence=r['sequence'], prospect_id=verified['prospect_id'],
                              gym_id=verified['gym_id'], work_hash=verified['work_hash'],
                              born_at_ms=b['born_at_ms'], accepted_at_ms=b['accepted_at_ms'],
                              descriptor_id=binding['descriptor_id'], manifest_digest=binding['manifest_digest'],
                              portrait_digest=binding['portrait_digest'],
                              source_record_digest=domain_hash(b'SLUG stable source record\0v1\0', r))
            need(c['assignment'] == assignment)
            self.supplied_history(c, archive['rows'])
            return archive
        except Refusal:
            raise
        except (ValueError, AssertionError, TypeError, KeyError, OverflowError) as error:
            raise Refusal() from error

    def verify(self, expected, raw, work_oracle):
        """work_oracle is the independently frozen direct Argon primitive only.

        No supplied 'known hash' map can be used as a successful work report.
        The runner creates that primitive after its execution/input freeze.
        """
        archive = self.validate_shape(expected, parse_archive(raw))
        record = archive['admission']
        observed = work_oracle.hash(record['proof']['header'])
        actual = self.admission.verified(record['job'], record['proof'], observed['work_hash'])
        need(actual == record['verified'])
        self.eligibility(record, actual)
        c = archive['capture']
        return dict(version=1, status='consistent_supplied_evidence',
                    file_sha256=sha(raw), file_bytes=len(raw), source_store_id=self.stable_id(expected),
                    prospect_id=actual['prospect_id'], history_id=c['history_id'],
                    captured_as_of_ms=c['as_of_ms'], event_count=c['event_count'],
                    checks=dict(exact_source_match=True, supplied_work_consistent=True,
                                gym_authorization_valid=True, static_appearance_consistent=True,
                                receipt_assignment_consistent=True, supplied_history_consistent=True),
                    unverified=dict(issuance_and_admission_times=True, actual_awards_and_result_finality=True,
                                    canonical_chain=True, current_ownership=True,
                                    history_completeness_and_freshness=True))


if __name__ == '__main__':
    raise SystemExit('Staged reference only. Root must freeze actual inputs and the execution plan before any expected-output run.')
