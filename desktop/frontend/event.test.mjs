// DOM/transport state tests using the independently frozen Python corpus.
// No browser rendering, normal-PoW execution or real-clock capture claim.
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import vm from 'node:vm';

const source = readFileSync(new URL('./stable.js', import.meta.url), 'utf8');
const html = readFileSync(new URL('./stable.html', import.meta.url), 'utf8');
const raw = readFileSync(new URL('../../gymevent/testdata/reference/event_v1.json', import.meta.url));
assert.equal(createHash('sha256').update(raw).digest('hex'), '9287f92c0cb46c585b44274c70ae57c5d6c9a4234e7078fd1d69a200644f996a');
const ref = JSON.parse(raw), clone = (v) => JSON.parse(JSON.stringify(v));
const runtime = "import { Call } from '/wails/runtime.js';";
const script = source.slice(runtime.length, source.indexOf("ui['event-refresh'].addEventListener"));
class Element {
  constructor(tag = 'div') { this.tagName = tag; this.children = []; this.attributes = new Map(); this.listeners = new Map(); this.hidden = false; this.disabled = false; this._text = ''; }
  set textContent(v) { this._text = String(v); this.children = []; }
  get textContent() { return this._text + this.children.map((c) => c.textContent).join(''); }
  set innerHTML(_) { throw new Error('Unsafe HTML interpolation'); }
  setAttribute(k, v) { this.attributes.set(k, String(v)); }
  removeAttribute(k) { this.attributes.delete(k); }
  addEventListener(k, f) { this.listeners.set(k, f); }
  append(...nodes) { this.children.push(...nodes); }
}
function dto(kind, fixture, predicate = () => true) {
  const row = ref.dtos.find((d) => d.kind === kind && (d.fixture === fixture || ['SnapshotReceipt', 'HistoryPage'].includes(kind) && d.fixture === '') && predicate(d.value));
  assert.ok(row, `Missing independent ${kind}/${fixture}`); return clone(row.value);
}
function harness(fixture = 'zero256') {
  const f = ref.fixtures.find((v) => v.fixture === fixture), snap = dto('EventSnapshot', fixture, (s) => s.state === 'ready');
  const elements = new Map([...html.matchAll(/\bid="([^"]+)"/g)].map((m) => [m[1], new Element()]));
  const document = { getElementById: (id) => elements.get(id), createElement: (tag) => new Element(tag), hidden: false };
  const window = { matchMedia: () => ({ matches: false, addEventListener() {} }) }, calls = [], handlers = {}; let frame = 1000, peak = 0, active = 0;
  const Call = { async ByName(name, ...args) { const method = name.slice(9); calls.push({ method, args: clone(args) }); assert.ok(handlers[method], `Unexpected ${method}`); active++; peak = Math.max(active, peak); try { return clone(await handlers[method](...args)); } finally { active--; } } };
  const context = vm.createContext({ document, window, Call, Intl, Date, btoa: (s) => Buffer.from(s, 'ascii').toString('base64'), setTimeout() { return 1; }, clearTimeout() {} });
  vm.runInContext(script, context, { filename: 'stable.js' });
  context.fixtureState = { account: { state: 'signed_in', fixture, held: false, can_export: true }, view: clone(f.current_view), gym_id: f.participant_id };
  context.stableState = { state: 'ready', view: clone(f.current_view), status: { pin: clone(f.entry_pin.stable_pin), store_id: f.entry_pin.stable_store_id, caught_up: true, pending_operation_id: '', slots: Array(10).fill('') } };
  context.initial = snap;
  vm.runInContext("current = fixtureState; stable = stableState; enabled = true; rememberedView = currentKey();", context);
  const reply = (kind, v) => ({ id: ++frame, ok: true, view: clone(f.current_view), store_id: ref.pin.store_id, snapshot: null, status: null, record: null, history: null, job: null, error: '', [kind]: clone(v) });
  const refusal = (error) => ({ id: ++frame, ok: false, view: null, store_id: '', snapshot: null, status: null, record: null, history: null, job: null, error });
  context.envelope = reply('snapshot', snap);
  vm.runInContext("eventSnapshot = eventEnvelope(envelope, 'snapshot').value", context);
  return { context, elements, handlers, reply, refusal, calls, fixture: f, snapshot: snap, get peak() { return peak; }, set(name, value) { context.inputValue = clone(value); vm.runInContext(`${name} = inputValue`, context); }, run(code) { return vm.runInContext(code, context); }, count(method) { return calls.filter((c) => c.method === method).length; } };
}
function select(h, s) { h.set('entryDate', s.event.saturday); h.set('eventDate', s.event.saturday); h.set('eventStatus', s); }
function deferred() { let resolve; const promise = new Promise((r) => { resolve = r; }); return { promise, resolve }; }

test('all frozen successful event frames reach the bounded candidate consumer', () => {
  let checked = 0;
  for (const c of ref.protocol_cases.filter((c) => c.accepted && c.response.ok)) {
    const h = harness(c.fixture), r = clone(c.response), field = ['snapshot', 'status', 'record', 'history', 'job'].find((k) => r[k] !== null);
    if (field === 'snapshot' && r.snapshot.state === 'setup_incomplete') h.run('eventAuthority = null');
    h.context.currentFrameView = clone(c.current_view);
    if (c.current_view) h.run('current.view = currentFrameView; stable.view = currentFrameView; eventSnapshot.view = currentFrameView');
    h.context.input = r;
    assert.doesNotThrow(() => h.run(`eventEnvelope(input, '${field}')`), c.name); checked++;
  }
  assert.ok(checked >= 20);
});

test('retained noon snapshot ignores a later live-roster edit and uses capture time honestly', () => {
  for (const fixture of ['zero256', 'incrementing256']) {
    const h = harness(fixture), record = dto('SnapshotReceipt', fixture);
    h.context.input = h.reply('record', record);
    h.run("eventDetail = { kind: 'record', value: eventEnvelope(input, 'record').value }; entryDate = eventDetail.value.input.event.saturday; stable.status.slots = Array(10).fill(''); renderEvent()");
    const text = h.elements.get('event-members').textContent;
    assert.match(text, /Candidate assignment at noon.*Captured/); assert.ok(record.input.participants.every((p) => p.members.every((m) => text.includes(m.assignment.prospect_id))));
    assert.equal(h.calls.length, 0); assert.equal(h.run('eventDetail.value.snapshot_digest'), record.snapshot_digest);
    assert.doesNotMatch(text, /official|finalized|ownership transferred/i);
  }
});

test('closed status captures automatically once for the original view and selected event', async () => {
  const h = harness(), status = dto('EventStatus', 'zero256', (s) => s.phase === 'awaiting_capture' && s.observed_at_ms >= s.event.cutoff_ms && s.coordinator_created_at_ms < s.event.cutoff_ms);
  const queued = dto('EventJob', 'zero256', (j) => j.state === 'queued'); select(h, status);
  h.handlers.EventStatus = () => h.reply('status', status); h.handlers.BeginEventCapture = () => h.reply('job', queued);
  await h.run('eventReadStatus(true)'); assert.equal(h.count('BeginEventCapture'), 1); assert.equal(h.run('eventWatch.id'), queued.job_id);
  h.run('eventWatch = null'); await h.run('eventReadStatus(true)'); assert.equal(h.count('BeginEventCapture'), 1); assert.equal(h.peak, 1);
});

test('before-noon authority observation never uses browser time to start capture', async () => {
  const h = harness(), status = dto('EventStatus', 'zero256', (s) => s.phase === 'before_cutoff'); select(h, status);
  h.handlers.EventStatus = () => h.reply('status', status); await h.run('eventReadStatus(true)'); assert.equal(h.count('BeginEventCapture'), 0); assert.equal(h.elements.get('event-capture').disabled, true);
});

test('lost capture acknowledgement remains unknown when status is merely incomplete', async () => {
  const h = harness(), status = dto('EventStatus', 'zero256', (s) => s.phase === 'awaiting_capture' && s.observed_at_ms >= s.event.cutoff_ms); select(h, status);
  h.handlers.BeginEventCapture = () => h.refusal('outcome_unknown'); h.handlers.EventStatus = () => h.reply('status', status);
  await h.run('eventBegin()'); assert.equal(h.run('eventUnknown'), true); await h.run('eventReadStatus()'); assert.equal(h.run('eventUnknown'), true); await h.run('eventBegin()'); assert.equal(h.count('BeginEventCapture'), 1); assert.match(h.elements.get('event-message').textContent, /may be saved/);
});

test('late job result cannot repopulate presentation after logout or reset', async () => {
  const h = harness(), j = dto('EventJob', 'zero256', (v) => v.state === 'completed'), gate = deferred();
  h.set('entryDate', j.saturday); h.set('eventWatch', { id: j.job_id, eventID: j.event_id, saturday: j.saturday, participantID: j.participant_id });
  h.handlers.EventJob = () => gate.promise;
  const pending = h.run(`eventFetchJob('${j.job_id}')`); h.run('epoch++; clearEvent(); current = null; stable = null'); gate.resolve(h.reply('job', j)); await pending;
  assert.equal(h.run('eventDetail'), null); assert.equal(h.elements.get('event-members').textContent, '');
});

test('shared cache refuses new capture before dispatch, preserving the saved detail', async () => {
  const h = harness(), status = dto('EventStatus', 'zero256', (s) => s.phase === 'awaiting_capture' && s.observed_at_ms >= s.event.cutoff_ms); select(h, status);
  h.run("previewImages.set('public-cache-stub', 'x'.repeat(500000))"); await h.run('eventBegin()'); assert.equal(h.count('BeginEventCapture'), 0); assert.equal(h.run('eventWatch'), null); assert.match(h.elements.get('event-message').textContent, /capacity/);
});

test('one-fixture setup remains usable and never fabricates the missing gym', () => {
  const h = harness(), setup = dto('EventSnapshot', 'zero256', (s) => s.state === 'setup_incomplete'); h.run('eventAuthority = null');
  const r = h.reply('snapshot', setup); r.store_id = ''; h.context.input = r;
  h.run("eventSnapshot = eventEnvelope(input, 'snapshot').value; eventMessage = eventError('event_setup_incomplete'); renderEvent()");
  assert.equal(h.run('accountReady()'), true); assert.equal(h.run('canEdit()'), true); assert.equal(h.run('eventDetail'), null); assert.match(h.elements.get('event-state').textContent, /Setup incomplete/); assert.match(h.elements.get('event-message').textContent, /both demo gyms/); assert.equal(h.calls.length, 0);
});

test('complete record replaces capture detail without retaining a second full result', async () => {
  const h = harness(), j = dto('EventJob', 'zero256', (v) => v.state === 'completed'), record = dto('SnapshotReceipt', 'zero256');
  h.set('entryDate', record.input.event.saturday); h.set('eventDetail', { kind: 'capture', value: j.result }); h.handlers.EventRecord = () => h.reply('record', record); await h.run('eventReadRecord()');
  assert.equal(h.run('eventDetail.kind'), 'record'); assert.equal(h.run('eventDetail.value.snapshot_digest'), record.snapshot_digest); assert.equal(h.run('eventCapacity()'), true); assert.equal(h.count('EventRecord'), 1);
});

test('foreign custody, mutable clock and seed fields fail before display', () => {
  const original = dto('SnapshotReceipt', 'zero256');
  for (const [name, mutate] of Object.entries({
    owner: (r) => { r.input.participants[0].owner.ownership_basis = 'official'; },
    cutoff: (r) => { r.input.participants[0].roster.cutoff_ms++; },
    clock: (r) => { r.captures[0].captured_at_ms = r.input.event.cutoff_ms - 1; },
    seed: (r) => { r.input.event.finalized = true; },
    members: (r) => { r.input.participants[0].members.push(r.input.participants[0].members[0]); },
  })) { const h = harness(), record = clone(original); mutate(record); h.context.input = h.reply('record', record); assert.throws(() => h.run("eventEnvelope(input, 'record')"), name); assert.equal(h.elements.get('event-members').textContent, ''); }
});


test('date navigation invalidates a capture queued behind a delayed native call', async () => {
  const h = harness(), status = dto('EventStatus', 'zero256', (s) => s.phase === 'awaiting_capture' && s.observed_at_ms >= s.event.cutoff_ms), gate = deferred(); select(h, status);
  h.handlers.EventStatus = () => gate.promise;
  h.run('render = renderEvent; background = Promise.resolve()');
  const pending = h.run('eventReadStatus()');
  h.run("queueEventSelection(eventBegin, 'event-capture'); entryNavigate('2026-11-28')");
  assert.equal(h.run('eventStatus'), null); assert.equal(h.run('tasks.length'), 1);
  gate.resolve(h.reply('status', status)); await pending;
  assert.equal(h.run('eventStatus'), null); await h.run('tasks.shift()()');
  assert.equal(h.count('BeginEventCapture'), 0); assert.equal(h.run('eventWatch'), null);
});

test('a complete record arriving after date navigation cannot repopulate that selection', async () => {
  const h = harness(), record = dto('SnapshotReceipt', 'zero256'), gate = deferred(); h.set('entryDate', record.input.event.saturday); h.set('eventDate', record.input.event.saturday);
  h.handlers.EventRecord = () => gate.promise; h.run("render = renderEvent; command = 'record'; background = Promise.resolve()");
  const pending = h.run('eventReadRecord()'); h.run("entryNavigate('2026-11-28')");
  gate.resolve(h.reply('record', record)); await pending;
  assert.equal(h.run('eventDetail'), null); assert.equal(h.elements.get('event-members').textContent, '');
});


test('render identities are bounded and clearing the view retains no full result string', () => {
  const h = harness(), record = dto('SnapshotReceipt', 'zero256'); h.set('entryDate', record.input.event.saturday); h.set('eventDetail', { kind: 'record', value: record }); h.set('eventHistory', dto('HistoryPage', 'zero256'));
  h.run('renderEvent()'); const membersKey = h.elements.get('event-members')._key, historyKey = h.elements.get('event-history-list')._key;
  assert.ok(membersKey.length + historyKey.length <= 4096); assert.doesNotMatch(membersKey, /condition_at_cutoff|latest_receipt|born_at_ms/);
  assert.ok(h.run('eventRetainedBytes()') >= h.run('entryBytes([eventDetail, eventHistory])') + 4096);
  h.run('clearEvent()'); assert.equal(h.elements.get('event-members')._key, ''); assert.equal(h.elements.get('event-history-list')._key, ''); assert.equal(h.run('eventDetail'), null);
});
