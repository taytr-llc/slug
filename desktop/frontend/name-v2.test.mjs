// Browser-shape and original-source tests, not cryptographic proof acceptance.
import assert from 'node:assert/strict';
import test from 'node:test';
import {V1, V2, clone, names, portraits, conditions, stables, bouts, events, histories, harness, transport, installCycle} from './name-v2-fixture.mjs';

const hash = n => n.toString(16).padStart(64, '0');
const zero = names.cases.find(c => c.seed === hash(0));
function pageFixture(descriptorID = V2) {
  const old = stables.cases[0], pin = transport(stables.pins.find(p => p.name === 'zero256').pin, descriptorID);
  const row = descriptorID === V2 ? zero : portraits.cases.find(c => c.seed === hash(0));
  const c = clone(conditions.cases.find(c => c.name === 'canonical_0_elapsed_0_xp0').expected);
  const assignment = {...clone(old.assignment), sequence: 1, work_hash: row.seed, born_at_ms: c.born_at_ms, accepted_at_ms: c.born_at_ms, descriptor_id: descriptorID, manifest_digest: row.manifest_digest, portrait_digest: descriptorID === V2 ? row.svg_digest : old.assignment.portrait_digest};
  // The transport identity is deliberately opaque; no synthetic admission claim.
  const item = {assignment, manifest: clone(row.manifest), condition: c};
  const view = {version: 1, view_id: hash(60), account_authority_id: pin.account_authority_id, account_id: pin.account_id, generation: 1, authority_pin: clone(pin.source), authority_origin: 'http://127.0.0.1:32123', transport_trust_id: hash(90)};
  const status = {version: 1, store_id: hash(91), pin, checkpoint: 1, observed_source_count: 1, caught_up: true, roster_revision: 0, slots: Array(10).fill(''), operation_count: 0, pending_operation_id: '', last_operation_id: '', last_as_of_ms: c.as_of_ms};
  return {item, view, status, row};
}
function installPage(h, f = pageFixture()) {
  h.set('current', {account: {state: 'signed_in', fixture: 'zero256', held: false}, view: f.view, gym_id: f.status.pin.identity.gym_id});
  h.set('stable', {state: 'ready', view: f.view, status: f.status});
  h.set('pageItems', [f.item]);
  h.run('rememberedView=currentKey()');
  return f;
}
function eventFixture(h, descriptorID = V2) {
  const part = events.dtos.find(d => d.kind === 'ParticipantSnapshot' && d.value.members.length > 0);
  const fixture = part.fixture, rawFixture = events.fixtures.find(f => f.fixture === fixture);
  const f = transport({current_view: rawFixture.current_view, entry_pin: rawFixture.entry_pin, participant_id: rawFixture.participant_id}, descriptorID);
  const snap = transport(events.dtos.find(d => d.kind === 'EventSnapshot' && d.fixture === fixture && d.value.state === 'ready').value, descriptorID);
  h.set('current', {account: {state: 'signed_in', fixture}, view: f.current_view, gym_id: f.participant_id});
  h.set('stable', {state: 'ready', view: f.current_view, status: {pin: f.entry_pin.stable_pin, store_id: f.entry_pin.stable_store_id, caught_up: true, pending_operation_id: '', slots: Array(10).fill('')}});
  h.set('eventAuthority', {pin: snap.pin, storeID: snap.store_id, createdMS: snap.created_at_ms});
  const participant = transport(part.value, descriptorID);
  h.set('entryStore', participant.entry_store_id);
  return {participant, event: transport(events.dtos.find(d => d.kind === 'Event').value, descriptorID), pin: snap.pin};
}

test('the browser catalogs exactly the independently frozen v2 tuple alongside v1', () => {
  const h = harness();
  assert.deepEqual(clone(h.call('candidateDescriptor', V2)), names.descriptor.descriptor);
  assert.deepEqual(Object.keys(h.run('CANDIDATE_DESCRIPTORS')).sort(), [V1, V2].sort());
  for (const bad of names.descriptor_negative_cases) assert.throws(() => h.call('candidateDescriptor', bad.id), bad.name);
  assert.throws(() => h.call('candidateDescriptor', '__proto__'));
});

test('every independent v2 manifest and portrait passes only its source tuple', () => {
  const h = harness();
  for (const c of names.cases) {
    assert.doesNotThrow(() => h.call('manifestValue', c.manifest, c.seed, V2), c.seed);
    assert.equal(h.call('imageOnlySVG', c.svg, V2), true, c.seed);
    assert.throws(() => h.call('manifestValue', c.manifest, c.seed, V1), c.seed);
    assert.equal(h.call('imageOnlySVG', c.svg, V1), false, c.seed);
  }
  for (const c of portraits.cases) {
    assert.doesNotThrow(() => h.call('manifestValue', c.manifest, c.seed, V1));
    assert.equal(h.call('imageOnlySVG', c.svg, V1), true);
    assert.throws(() => h.call('manifestValue', c.manifest, c.seed, V2));
    assert.equal(h.call('imageOnlySVG', c.svg, V2), false);
  }
});

test('v2 names preserve real apostrophes and reject invalid components even for booked inputs', () => {
  const h = harness();
  for (const [surname, seed] of Object.entries(names.apostrophe_first_seeds)) {
    const c = names.cases.find(c => c.seed === seed);
    assert.equal(c.manifest.name.last, surname);
    assert.ok(c.svg.includes(surname.replaceAll("'", '&apos;')));
    assert.equal(h.call('imageOnlySVG', c.svg, V2), true);
    assert.doesNotThrow(() => h.call('manifestValue', c.manifest, c.seed, V2, true));
  }
  for (const bad of ['', "'Alan", 'Alan-', "O''Brien", 'Anne Marie', 'A'.repeat(33), 'A\0B', '<script>', 'José']) {
    const m = clone(zero.manifest); m.name.first = bad; m.name.display = `${bad} ${m.name.last}`;
    for (const booked of [false, true]) assert.throws(() => h.call('manifestValue', m, zero.seed, V2, booked), JSON.stringify(bad));
  }
  for (const change of [s => s.replace('</svg>', '<script/></svg>'), s => s.replace('role="img"', 'onload="alert(1)"'), s => s.replace('<title>', '<title>&evil;'), s => s.replace('data-renderer="', 'data-renderer="unknown-')]) {
    assert.equal(h.call('imageOnlySVG', change(zero.svg), V2), false);
  }
  assert.equal(h.call('imageOnlySVG', zero.svg, hash(999)), false);
});

test('manifest component substitutions fail despite globally supported versions', () => {
  const h = harness(), v1 = portraits.cases.find(c => c.seed === zero.seed).manifest;
  for (const field of ['manifest_version', 'generator_id', 'table_hash']) {
    const m = clone(zero.manifest); m[field] = v1[field]; assert.throws(() => h.call('manifestValue', m, zero.seed, V2), field);
  }
  const m = clone(zero.manifest); m.dna.dna_version = 2; assert.throws(() => h.call('manifestValue', m, zero.seed, V2));
  m.dna.dna_version = 1; m.portrait.portrait_version = 2; assert.throws(() => h.call('manifestValue', m, zero.seed, V2));
});

test('pool view and sparse page stay bound to their original source; solo remains v1', () => {
  const h = harness();
  for (const id of [V1, V2]) {
    const f = installPage(h, pageFixture(id));
    assert.doesNotThrow(() => h.call('checkedView', f.view));
    const page = {version: 1, status: f.status, as_of_ms: f.item.condition.as_of_ms, after: 0, next_after: 1, has_more: false, items: [f.item]};
    assert.doesNotThrow(() => h.call('pageValue', page, 'inventory', f.view, 'zero256'));
    const bad = clone(page); bad.items[0].assignment.descriptor_id = id === V1 ? V2 : V1;
    assert.throws(() => h.call('pageValue', bad, 'inventory', f.view, 'zero256'));
    const foreign = clone(f.view); foreign.authority_pin.descriptor_id = id === V1 ? V2 : V1;
    assert.throws(() => h.call('pageValue', page, 'inventory', foreign, 'zero256'));
  }
  h.run('poolMode=false');
  assert.throws(() => h.call('checkedView', pageFixture(V2).view));
  assert.doesNotThrow(() => h.call('checkedView', pageFixture(V1).view));
});

test('entry members and both noon participant copies require the exact source descriptor', () => {
  for (const id of [V1, V2]) {
    const h = harness(), f = eventFixture(h, id), foreign = id === V1 ? V2 : V1;
    assert.doesNotThrow(() => h.call('eventPinValue', f.pin));
    assert.doesNotThrow(() => h.call('eventParticipant', f.participant, f.event));
    assert.doesNotThrow(() => h.call('entryReceipt', f.participant.entry.latest_receipt));
    const receipt = clone(f.participant.entry.latest_receipt); receipt.members[0].assignment.descriptor_id = foreign;
    assert.throws(() => h.call('entryReceipt', receipt));
    const bad = clone(f.participant); bad.members[0].assignment.descriptor_id = foreign; bad.entry.latest_receipt.members[0].assignment.descriptor_id = foreign;
    assert.throws(() => h.call('eventParticipant', bad, f.event));
    const pin = clone(f.pin); pin.participants[0].stable_pin.source.descriptor_id = foreign;
    assert.throws(() => h.call('eventPinValue', pin));
  }
});

test('booked inputs and result publication cannot substitute another supported descriptor', () => {
  const original = bouts.compositions.find(c => c.name === 'modeled_welter_attribution_0');
  for (const id of [V1, V2]) {
    const h = harness(), c = installCycle(h, original, id), foreign = id === V1 ? V2 : V1;
    assert.doesNotThrow(() => h.call('eventPinValue', c.pin.source_pin.source_pin.source_pin));
    assert.doesNotThrow(() => h.call('bookedRecordValue', c.record));
    const bad = clone(c.record); bad.source.corners[0].member.assignment.descriptor_id = foreign;
    assert.throws(() => h.call('bookedRecordValue', bad));
    const wrongManifest = clone(c.record); wrongManifest.input.a.manifest = id === V1 ? clone(zero.manifest) : clone(original.record.input.a.manifest);
    assert.throws(() => h.call('bookedRecordValue', wrongManifest));
    const r = c.record, source = {version: 1, store_id: r.store_id, store_created_at_ms: c.store_created_at_ms, record_digest: hash(99), source_digest: r.source_digest, input_id: r.input_id, result_digest: r.result_digest, created_at_ms: r.created_at_ms, source: r.source, summary: {winner: r.result.winner, method: r.result.method, end_round: r.result.end_round, end_tick: r.result.end_tick}};
    assert.doesNotThrow(() => h.call('publicationSource', source));
    const mixed = clone(source); mixed.source.corners[1].member.assignment.descriptor_id = foreign;
    assert.throws(() => h.call('publicationSource', mixed));
  }
});

test('finalized history keeps its own source descriptor through both supported versions', () => {
  for (const id of [V1, V2]) {
    const h = harness(), c = transport(histories.history.histories.find(x => x.name === 'genuine_win').expected.capture, id);
    const view = transport(histories.wire.dtos.find(x => x.id === 'job/done').value.view, id);
    Object.assign(view, {account_id: c.status.pin.account_id, account_authority_id: c.status.pin.account_authority_id, authority_pin: clone(c.status.pin.source)});
    const fixture = c.status.pin.identity.gym_id === 'a227587d837507dbcb5f9d781967339d816070fb7bd574931c690ba6937b88e5' ? 'zero256' : 'incrementing256';
    h.set('current', {account: {state: 'signed_in', fixture}, view});
    h.set('stable', {state: 'ready', view, status: c.status});
    h.set('careerActivation', c.activation);
    assert.doesNotThrow(() => h.call('historyCapture', c));
    const bad = clone(c); bad.assignment.descriptor_id = id === V1 ? V2 : V1;
    assert.throws(() => h.call('historyCapture', bad));
  }
});

test('portrait loading checks the selected source renderer and drops old-view responses', async () => {
  for (const scenario of ['valid', 'foreign-renderer', 'old-view']) {
    const h = harness(), f = installPage(h), a = f.item.assignment;
    h.context.fighter = clone(f.item);
    h.run("cards.set(fighter.assignment.prospect_id,{fingerprint:JSON.stringify(fighter),image:document.createElement('img'),placeholder:document.createElement('span')})");
    let release;
    h.handlers.ManifestPortrait = () => new Promise(resolve => { release = resolve; });
    const pending = h.call('loadPortrait', f.item);
    if (scenario === 'old-view') h.run('current.view={...current.view,generation:current.view.generation+1}');
    release({version: 1, view: clone(f.view), sequence: a.sequence, id: a.prospect_id, manifest_digest: a.manifest_digest, portrait_digest: a.portrait_digest, svg: scenario === 'foreign-renderer' ? portraits.cases.find(c => c.seed === zero.seed).svg : zero.svg});
    await pending;
    assert.equal(h.run('images.size'), scenario === 'valid' ? 1 : 0, scenario);
    assert.equal(h.calls.length, 1);
  }
});
