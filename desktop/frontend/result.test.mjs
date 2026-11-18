// UI tests consume the separately committed Python corpus. They do not grant
// cryptographic/source authority or claim native chooser/GUI acceptance.
import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';
import {readFileSync} from 'node:fs';
import test from 'node:test';
import vm from 'node:vm';
const bytes = readFileSync(new URL('../../gymresult/testdata/reference/corpus.json', import.meta.url));
assert.equal(createHash('sha256').update(bytes).digest('hex'), '07c00eef8d87ae9ae5dcd2d120a5dfd1f7b1ba33a1447b773cfd4d5194fd1a94');
const ref = JSON.parse(bytes), clone = v => JSON.parse(JSON.stringify(v));
const savedBoutBytes = readFileSync(new URL('../../gymresult/testdata/reference/inputs/actual47/bout_record.json', import.meta.url));
assert.equal(createHash('sha256').update(savedBoutBytes).digest('hex'), '2675c392cb7ce82e779898d03170daba76eed6155c947221d6e5ecbe55733692');
const savedBout = JSON.parse(savedBoutBytes);
const source = readFileSync(new URL('./stable.js', import.meta.url), 'utf8'), html = readFileSync(new URL('./stable.html', import.meta.url), 'utf8');
const script = source.slice("import { Call } from '/wails/runtime.js';".length, source.indexOf("ui['event-refresh'].addEventListener"));
const core = (kind, name) => clone(ref.core.positive.find(v => v.kind === kind && v.name === name).value);
const wire = (kind, name) => clone(ref.wire.cases.find(v => v.category === 'dto' && v.kind === kind && v.name === name).value);
class Element {
  constructor(tag = 'div') { this.tagName = tag; this.children = []; this.attributes = new Map(); this.listeners = new Map(); this._text = ''; this.disabled = false; }
  set textContent(v) { this._text = String(v); this.children = []; }
  get textContent() { return this._text + this.children.map(v => v.textContent).join(''); }
  set innerHTML(_) { throw Error('Unsafe HTML'); }
  append(...v) { this.children.push(...v); }
  setAttribute(k, v) { this.attributes.set(k, String(v)); }
  removeAttribute(k) { this.attributes.delete(k); }
  addEventListener(k, v) { this.listeners.set(k, v); }
}
function harness(index = 0) {
  const snap = wire('ResultSnapshot', `fixture${index}/snapshot/none-offline`), p = snap.pin, view = snap.view;
  const src = core('ResultSource', 'saved-source'), ss = core('Status', 'status-prepared').source;
  const ep = p.source_pin.source_pin.source_pin.source_pin, sp = ep.participants.find(v => v.stable_pin.account_id === view.account_id);
  const elements = new Map([...html.matchAll(/\bid="([^"]+)"/g)].map(m => [m[1], new Element()]));
  const handlers = {}, calls = [], timers = new Map(); let id = 200, active = 0, peak = 0, timerID = 0;
  const document = {getElementById: id => elements.get(id), createElement: tag => new Element(tag), hidden: false};
  const motion = {matches: false, addEventListener() {}};
  const context = vm.createContext({document, window: {matchMedia: () => motion}, Call: {async ByName(name, ...args) { const method = name.slice(9); calls.push({method, args: clone(args)}); assert.ok(handlers[method], `Unexpected ${method}`); active++; peak = Math.max(peak, active); try { const v = await handlers[method](...args); return v === undefined ? null : clone(v); } finally { active--; } }}, Intl, Date, btoa: s => Buffer.from(s, 'ascii').toString('base64'), setTimeout(fn) { timers.set(++timerID, fn); return timerID; }, clearTimeout(id) { timers.delete(id); }});
  vm.runInContext(script, context, {filename: 'stable.js'});
  const h = {context, elements, handlers, calls, timers, document, snap, p, view, src, ss, get peak() { return peak; }, run: s => vm.runInContext(s, context), set(n, v) { context.input = clone(v); vm.runInContext(`${n}=input`, context); }, count: m => calls.filter(v => v.method === m).length};
  h.set('current', {account: {state: 'signed_in', fixture: index ? 'incrementing256' : 'zero256', held: false, can_export: true, id: view.account_id}, view, gym_id: sp.stable_pin.identity.gym_id});
  h.set('stable', {state: 'ready', view, status: {pin: sp.stable_pin, store_id: sp.stable_store_id, caught_up: true, pending_operation_id: '', slots: Array(10).fill('')}});
  h.set('eventAuthority', {pin: ep, storeID: p.source_pin.source_pin.source_pin.source_store_id, createdMS: 1});
  h.set('seedAuthority', {pin: p.source_pin.source_pin.source_pin, storeID: p.source_pin.source_pin.source_store_id, createdMS: 1});
  h.set('cardAuthority', {pin: p.source_pin.source_pin, storeID: p.source_pin.source_store_id, createdMS: 1});
  h.set('bookedAuthority', {pin: p.source_pin, storeID: p.source_store_id, sourceCreatedMS: 1, createdMS: src.store_created_at_ms});
  h.set('bookedSnapshot', {version: 1, state: 'ready', error: '', view, store_id: p.source_store_id, pin: p.source_pin, source_created_at_ms: 1, created_at_ms: src.store_created_at_ms, active_job_id: '', queued_job_id: '', last_job_id: ''});
  h.set('bookedStatus', ss); h.set('bookedSelected', src.source.bout.bout_id); h.set('entryDate', src.source.event.saturday);
  h.set('publicationAuthority', {pin: p, storeID: snap.store_id, sourceCreatedMS: snap.source_created_at_ms, createdMS: snap.created_at_ms}); h.set('publicationSnapshot', snap);
  h.run('enabled=true;rememberedView=currentKey();bookedOverlayKnown=bookedOverlayRead=true');
  h.reply = (field, value) => ({id: ++id, ok: true, view: clone(view), store_id: snap.store_id, snapshot: null, status: null, record: null, job: null, error: '', [field]: clone(value)});
  h.refusal = error => ({id: ++id, ok: false, view: null, store_id: '', snapshot: null, status: null, record: null, job: null, error});
  h.adopt = name => { h.context.input = core('ActionResult', 'action-' + name); h.run('publicationAdoptResult(input)'); };
  h.watch = j => h.set('publicationWatch', {id: j.job_id, kind: j.kind, state: 'queued', date: j.saturday, boutID: j.bout_id, cardID: j.card_id, resultKey: j.result_key});
  return h;
}
const deferred = () => { let resolve; const promise = new Promise(r => { resolve = r; }); return {promise, resolve}; };

test('all independent compact result positives pass presentation checks without generating source', () => {
  const h = harness(); h.run("condition=()=>{throw Error('projection')};fullItem=()=>{throw Error('projection')}"); let count = 0;
  const methods = {Record: 'publicationRecordValue', Status: 'publicationStatusValue', ActionResult: 'publicationResult', ReadResult: 'publicationResult', ResultSource: 'publicationSource', ProofSummary: 'publicationProofSummary'};
  for (const v of ref.core.positive.filter(v => methods[v.kind])) { h.context.input = clone(v.value); assert.doesNotThrow(() => h.run(`${methods[v.kind]}(input)`), v.kind + '/' + v.name); count++; }
  assert.ok(count > 40);
});
test('accepted native protocol payloads retain original view and authority in presentation', () => {
  let count = 0;
  for (const c of ref.wire.cases.filter(v => v.category === 'protocol' && v.accept && v.response.ok)) {
    const index = c.current_view?.account_id === ref.core.pin.publisher_account_id ? 0 : 1, h = harness(index);
    if (c.current_view) { h.set('current.view', c.current_view); h.set('stable.view', c.current_view); h.set('bookedSnapshot.view', c.current_view); h.set('publicationSnapshot.view', c.current_view); }
    const r = clone(c.response), field = ['snapshot', 'status', 'record', 'job'].find(k => r[k] !== null); h.context.input = r;
    assert.doesNotThrow(() => h.run(`publicationEnvelope(input,'${field}')`), c.name); count++;
  }
  assert.ok(count > 20);
});
test('passive polling and saved publication reads never connect, publish, attest or export', async () => {
  const h = harness(); h.handlers.ResultSnapshot = () => h.reply('snapshot', h.snap); h.handlers.ResultStatus = () => h.reply('status', core('Status', 'status-prepared')); h.handlers.ResultRecord = () => h.reply('record', core('ActionResult', 'action-prepared'));
  await h.run('fetchPublicationSnapshot();'); await h.run('publicationRead()'); assert.deepEqual(h.calls.map(v => v.method), ['ResultSnapshot', 'ResultStatus', 'ResultRecord']); assert.equal(h.peak, 1);
});
test('held Demo A can check and export but cannot publish or finalize; Demo B cannot connect the publisher', () => {
  const h = harness(); h.adopt('finalized60'); h.run('current.account.held=true'); assert.equal(h.run("publicationCanAct('publish')"), false); assert.equal(h.run("publicationCanAct('attest')"), false); assert.equal(h.run("publicationCanAct('reconcile')"), true); assert.equal(h.run("publicationCanAct('export')"), true);
  const other = harness(1); other.adopt('finalized60'); for (const k of ['publish', 'reconcile', 'attest', 'export']) assert.equal(other.run(`publicationCanAct('${k}')`), false);
});
test('59 descendants cannot finalize and historical finality is not confused with current connection', () => {
  const h = harness(); h.adopt('included59'); h.run('renderPublication()'); assert.equal(h.elements.get('result-attest').disabled, true); assert.match(h.elements.get('result-finality').textContent, /59 descendant/);
  h.adopt('finalized60'); h.run('renderPublication()'); assert.match(h.elements.get('result-state').textContent, /Historical test finality/); assert.match(h.elements.get('result-connection').textContent, /disconnected/);
});
test('fresh source conflict preserves the earlier result-chain observation but blocks strong actions', () => {
  const h = harness(); h.context.input = core('ReadResult', 'fresh-source-conflict-retained-result-chain-observation'); h.run('publicationAdoptResult(input);renderPublication()');
  assert.equal(h.run('publicationStatus.current_finality'), true); assert.equal(h.run("publicationCanAct('publish')"), false); assert.equal(h.run("publicationCanAct('attest')"), false); assert.match(h.elements.get('result-conflict').textContent, /earlier chain check/);
});
test('older successful terminal cannot remove a newer explicit conflict or finality', async () => {
  const h = harness(); h.adopt('checkpoint_conflict'); const before = createHash('sha256').update(h.run('JSON.stringify([publicationRecord,publicationStatus,publicationOverlayKnown])')).digest('hex');
  const j = wire('ResultJob', 'fixture0/job/publish/completed'); h.watch(j); h.handlers.ResultJob = () => h.reply('job', j); await h.run(`publicationFetchJob('${j.job_id}')`);
  assert.equal(createHash('sha256').update(h.run('JSON.stringify([publicationRecord,publicationStatus,publicationOverlayKnown])')).digest('hex'), before); assert.equal(h.run('publicationUnknown'), null);
  assert.match(h.elements.get('result-state').textContent, /^Finalized result · Conflicting evidence/);
});
test('late publication ACK follows its original bout without replacing a new selection', async () => {
  const h = harness(), gate = deferred(), j = wire('ResultJob', 'fixture0/job/publish/queued'); h.handlers.BeginResultPublish = () => gate.promise;
  const pending = h.run("publicationBegin('publish')"); h.run("entryDate='2026-12-05';bookedSelectDate()"); gate.resolve(h.reply('job', j)); await pending;
  assert.equal(h.run('publicationWatch.date'), '2026-11-28'); assert.equal(h.run('publicationStatus'), null); assert.equal(h.run('bookedSelected'), '');
});
test('old View responses cannot repopulate result evidence', async () => {
  const h = harness(), gate = deferred(); h.handlers.ResultStatus = () => gate.promise; const pending = h.run('publicationRead()'); h.run('epoch++;clearBooked();current=null;stable=null'); gate.resolve(h.reply('status', core('Status', 'status-prepared'))); await pending;
  assert.equal(h.run('publicationStatus'), null); assert.equal(h.run('publicationRecord'), null);
});
test('unknown publish is query-first and never silently repeated', async () => {
  const h = harness(); h.handlers.BeginResultPublish = () => h.refusal('outcome_unknown'); await h.run("publicationBegin('publish')"); await h.run("publicationBegin('publish')");
  assert.equal(h.count('BeginResultPublish'), 1); assert.ok(h.run('publicationUnknown')); assert.match(h.run('publicationMessage'), /may have been saved or submitted/);
});
test('an old cached terminal cannot resolve an unacknowledged publish for the same bout', async () => {
  const h = harness(), j = wire('ResultJob', 'fixture0/job/publish/completed');
  h.handlers.BeginResultPublish = () => h.refusal('outcome_unknown'); h.handlers.ResultJob = () => h.reply('job', j);
  await h.run("publicationBegin('publish')"); await h.run(`publicationFetchJob('${j.job_id}')`);
  assert.ok(h.run('publicationUnknown')); assert.equal(h.run('publicationStatus'), null); assert.equal(h.run("publicationCanAct('publish')"), false);
});
test('a complete explicit absent read resolves uncertainty without a chain action', async () => {
  const h = harness(); h.handlers.BeginResultPublish = () => h.refusal('outcome_unknown'); h.handlers.ResultStatus = () => h.reply('status', core('Status', 'absent'));
  await h.run("publicationBegin('publish')"); await h.run('publicationRead()');
  assert.equal(h.run('publicationUnknown'), null); assert.equal(h.run('publicationStatus.phase'), 'absent'); assert.equal(h.run("publicationCanAct('publish')"), true);
  assert.deepEqual(h.calls.map(v => v.method), ['BeginResultPublish', 'ResultStatus']);
});
test('uncertainty survives an incomplete explicit read and clears after its complete retry', async () => {
  const h = harness(); h.handlers.BeginResultPublish = () => h.refusal('outcome_unknown'); h.handlers.ResultStatus = () => h.reply('status', core('Status', 'status-prepared')); h.handlers.ResultRecord = () => h.refusal('busy');
  await h.run("publicationBegin('publish')"); await h.run('publicationRead()'); assert.ok(h.run('publicationUnknown'));
  h.handlers.ResultRecord = () => h.reply('record', core('ActionResult', 'action-prepared')); await h.run('publicationRead()'); assert.equal(h.run('publicationUnknown'), null);
});
test('a known uncertain job is resolved only by its own successful terminal', async () => {
  const h = harness(), j = wire('ResultJob', 'fixture0/job/publish/completed'); h.watch(j); h.handlers.ResultJob = () => h.refusal('outcome_unknown');
  await h.run(`publicationFetchJob('${j.job_id}')`); assert.equal(h.run('publicationUnknown.jobID'), j.job_id);
  h.handlers.ResultJob = () => h.reply('job', j); await h.run(`publicationFetchJob('${j.job_id}')`); assert.equal(h.run('publicationUnknown'), null);
});
test('failed job error grammar and structural anchor range match the frozen contract', () => {
  const h = harness(), j = wire('ResultJob', 'fixture0/job/publish/completed'); j.state = 'failed'; j.result = null; j.error = 'result_setup_incomplete'; h.context.input = j; assert.doesNotThrow(() => h.run('publicationJobValue(input)'));
  j.error = 'invalid_frame'; h.context.input = j; assert.throws(() => h.run('publicationJobValue(input)'));
  const a = core('Record', 'record-included59').inclusion.anchor; a.height = 1; h.context.input = a; assert.doesNotThrow(() => h.run('publicationAnchor(input)')); a.height = 0; h.context.input = a; assert.throws(() => h.run('publicationAnchor(input)'));
});
test('public export uses only the native selected tuple and returns its bounded summary', async () => {
  const h = harness(); h.adopt('finalized60'); const queued = wire('ResultJob', 'fixture0/job/export/queued'), done = wire('ResultJob', 'fixture0/job/export/completed');
  h.handlers.ExportResultProof = () => h.reply('job', queued); h.handlers.ResultJob = () => h.reply('job', done); await h.run("publicationBegin('export')"); await h.run(`publicationFetchJob('${queued.job_id}')`);
  assert.equal(h.calls[0].args.length, 3); assert.deepEqual(h.calls[0].args, [h.view.view_id, h.src.source.event.saturday, h.src.source.bout.bout_id]); assert.ok(h.run('publicationExport')); assert.match(h.elements.get('result-exported').textContent, /Public proof saved/); assert.equal(h.run('publicationExport.bytes'), done.export.bytes);
});
test('native chooser cancellation restores retained evidence without uncertainty', async () => {
  const h = harness(); h.adopt('finalized60'); const prior = h.run('JSON.stringify(publicationSavedState())'); h.handlers.ExportResultProof = () => null; await h.run("publicationBegin('export')"); assert.equal(h.run('JSON.stringify(publicationSavedState())'), prior); assert.equal(h.run('publicationUnknown'), null);
});
test('capacity is reciprocal and refuses before beginning without evicting evidence', async () => {
  const h = harness(); h.adopt('prepared'); const before = h.run('JSON.stringify(publicationRecord)'); h.set('pageItems', [{padding: 'x'.repeat(900000)}]); await h.run("publicationBegin('publish')"); assert.equal(h.calls.length, 0); assert.equal(h.run('JSON.stringify(publicationRecord)'), before); assert.equal(h.run('publicationWatch'), null);
  h.set('pageItems', []); const remaining = h.run('CACHE_MAX-(previewRetainedBytes()+entryRetainedBytes()+eventRetainedBytes()+seedRetainedBytes()+cardRetainedBytes()+bookedRetainedBytes()+publicationRetainedBytes()+JSON.stringify(pageItems).length)'); assert.equal(h.run(`previewCapacity(${remaining})`), true); assert.equal(h.run(`previewCapacity(${remaining+1})`), false);
});
test('terminal capacity refusal restores previous record, observation and job tracking', async () => {
  const h = harness(); h.adopt('prepared'); const j = wire('ResultJob', 'fixture0/job/publish/completed'); h.watch(j); const before = h.run('JSON.stringify(publicationSavedState())'); h.handlers.ResultJob = () => h.reply('job', j); h.run('publicationCapacity=()=>false'); await h.run(`publicationFetchJob('${j.job_id}')`); assert.equal(h.run('JSON.stringify(publicationSavedState())'), before);
});
test('own result conflict cannot advertise current finality and malformed sizes/types fail', () => {
  const h = harness(); for (const mutate of [v => v.current_finality = true, v => v.version = true, v => v.source_record_digest = 'e'.repeat(64)]) { const v = core('Status', 'status-checkpoint_conflict'); mutate(v); h.context.input = v; assert.throws(() => h.run('publicationStatusValue(input)')); }
  const p = core('ProofSummary', 'proof-summary'); p.bytes = 25165825; h.context.input = p; assert.throws(() => h.run('publicationProofSummary(input)'));
});
test('result build extends prior mode and preserves diagnostic profile and icon guards', () => {
  const b = readFileSync(new URL('../scripts/build-manifest-macos.sh', import.meta.url), 'utf8'); for (const k of ['candidate', 'stable', 'entry', 'event', 'seed', 'card', 'bout', 'result']) assert.ok(b.includes(`-X main.${k}Build=1`)); assert.match(b, /<string>0.4.10<\/string>/); assert.match(b, /<string>50<\/string>/); assert.match(b, /slugtestclock\|slugtestpow/); assert.match(b, /ForTestClock/); assert.match(b, /build-icons-macos.sh/);
});

test('late refusal or chooser cancellation cannot restore a previous selected publication', async () => {
  for (const kind of ['publish', 'export']) {
    const h = harness(), gate = deferred(); if (kind === 'export') h.adopt('finalized60');
    h.handlers[kind === 'export' ? 'ExportResultProof' : 'BeginResultPublish'] = () => gate.promise;
    const pending = h.run(`publicationBegin('${kind}')`); h.run("entryDate='2026-12-05';bookedSelectDate()"); gate.resolve(kind === 'export' ? null : h.refusal('busy')); await pending;
    assert.equal(h.run('publicationRecord'), null); assert.equal(h.run('publicationStatus'), null); assert.equal(h.run('publicationWatch'), null);
  }
});

test('rejected result cache replacement retains protected bout portrait evidence', async () => {
  const h = harness(); h.adopt('prepared'); h.run("bookedImages.set('retained', 'data:image/svg+xml;base64,retained');publicationCapacity=()=>false");
  h.handlers.ResultStatus = () => h.reply('status', core('ReadResult', 'fresh-source-conflict-retained-result-chain-observation').status);
  await h.run('publicationRead()'); assert.equal(h.run('bookedImages.size'), 1); assert.equal(h.run('bookedOverlayKnown'), true); assert.match(h.run('publicationMessage'), /full/);
});

function showSavedReplay(h) {
  h.set('bookedRecord', savedBout);
  h.run('bookedRecordValue(bookedRecord);bookedMatchRecord(bookedRecord,bookedStatus);renderBooked()');
}
test('all pre-final publication states label the saved outcome provisional, including no loaded publication', () => {
  for (const name of [null, 'absent', 'status-prepared', 'status-built', 'status-dispatch_unknown', 'status-submitted', 'status-included59', 'status-checkpoint59']) {
    const h = harness();
    if (name) { h.context.input = core('Status', name); h.run('publicationStatusValue(input);publicationAdoptStatus(input)'); }
    showSavedReplay(h);
    assert.match(h.elements.get('result-state').textContent, /^Provisional result/, name);
    assert.match(h.elements.get('bout-asof').textContent, /^Provisional result/, name);
    h.run('stepBooked(bookedTicks())');
    assert.match(h.elements.get('bout-finish').textContent, /^Provisional result:/, name);
    assert.match(h.elements.get('bout-scores').textContent, /^Provisional result/, name);
    assert.equal(h.calls.length, 0);
  }
  const h = harness(), s = core('Status', 'status-included59'); s.descendants = 60; s.scan_tip_height++;
  h.context.input = s; h.run('publicationStatusValue(input);publicationAdoptStatus(input)'); showSavedReplay(h);
  assert.match(h.elements.get('bout-state').textContent, /Provisional result/);
});
test('matching historical result finality labels replay final through offline, shallow and conflicting observations', () => {
  for (const name of ['finalized60', 'offline_historical', 'shallow_historical', 'checkpoint_conflict', 'source-conflict']) {
    const h = harness();
    if (name === 'source-conflict') { h.context.input = core('ReadResult', 'fresh-source-conflict-retained-result-chain-observation'); h.run('publicationResult(input);publicationAdoptResult(input)'); }
    else h.adopt(name);
    showSavedReplay(h); h.run('stepBooked(3)');
    const record = h.run('JSON.stringify(bookedRecord)'), step = h.run('bookedStep');
    assert.match(h.elements.get('result-state').textContent, /^Finalized result/, name);
    assert.match(h.elements.get('bout-asof').textContent, /^Finalized result/, name);
    assert.match(h.elements.get('bout-finish').textContent, /^Finalized result:/, name);
    assert.match(h.elements.get('bout-scores').textContent, /^Finalized result/, name);
    if (name.endsWith('conflict')) assert.match(h.elements.get('bout-asof').textContent, /Conflict reported/);
    h.run("publicationRefusal('outcome_unknown');renderBooked()");
    assert.match(h.elements.get('bout-finish').textContent, /^Finalized result:/);
    assert.equal(h.run('JSON.stringify(bookedRecord)'), record); assert.equal(h.run('bookedStep'), step); assert.equal(h.calls.length, 0);
  }
});
test('finality metadata is bound to the selected original view and exact saved outcome', () => {
  const mismatches = [
    "publicationSnapshot.view.view_id='f'.repeat(64)", "bookedSnapshot.view.view_id='f'.repeat(64)",
    "entryDate='2026-12-05'", "bookedSelected='f'.repeat(64)", "bookedStatus.card_id='f'.repeat(64)",
    "publicationStatus.source_record_digest='f'.repeat(64)", "publicationStatus.input_id='f'.repeat(64)", "publicationStatus.result_digest='f'.repeat(64)",
    "publicationStatus.source.saved.record_digest='f'.repeat(64)", "publicationStatus.source.saved.source_digest='f'.repeat(64)",
    "bookedRecord.input_id='f'.repeat(64)", "bookedRecord.result_digest='f'.repeat(64)", "bookedRecord.bout_id='f'.repeat(64)",
  ];
  for (const mismatch of mismatches) {
    const h = harness(); h.adopt('finalized60'); showSavedReplay(h);
    h.run(mismatch); assert.equal(h.run('publicationForSelectedBout()'), null, mismatch);
    h.run('renderBooked()'); assert.match(h.elements.get('bout-finish').textContent, /^Provisional result:/, mismatch);
  }
});
test('a validated status-only read updates replay finality without replaying or mutating its transcript', async () => {
  const h = harness(); showSavedReplay(h); h.run('stepBooked(2)'); const before = h.run('JSON.stringify(bookedRecord)');
  h.handlers.ResultStatus = () => h.reply('status', core('Status', 'status-finalized60'));
  h.handlers.ResultRecord = () => h.refusal('busy'); await h.run('publicationRead()');
  assert.equal(h.run('publicationRecord'), null); assert.match(h.elements.get('bout-finish').textContent, /^Finalized result:/);
  assert.equal(h.run('bookedStep'), 2); assert.equal(h.run('JSON.stringify(bookedRecord)'), before);
  assert.deepEqual(h.calls.map(v => v.method), ['ResultStatus', 'ResultRecord']);
  h.run('publicationSelect();renderBooked()'); assert.match(h.elements.get('bout-finish').textContent, /^Provisional result:/); assert.match(h.elements.get('bout-asof').textContent, /Finality has not been read/);
});

test('rejected metadata capacity replacement preserves the prior finality label and replay', async () => {
  const h = harness(); h.adopt('finalized60'); showSavedReplay(h); h.run('stepBooked(2)');
  const before = h.run('JSON.stringify([publicationStatus,publicationRecord,bookedRecord])'); h.run('publicationCapacity=()=>false');
  h.handlers.ResultStatus = () => h.reply('status', core('Status', 'status-checkpoint_conflict')); await h.run('publicationRead()');
  assert.equal(h.run('JSON.stringify([publicationStatus,publicationRecord,bookedRecord])'), before);
  assert.match(h.elements.get('bout-finish').textContent, /^Finalized result:/); assert.doesNotMatch(h.elements.get('result-state').textContent, /Conflicting/);
  assert.equal(h.run('bookedStep'), 2); assert.deepEqual(h.calls.map(v => v.method), ['ResultStatus']);
});
