// Frozen independent compositions; the DOM harness grants no source/GUI authority.
import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';
import {readFileSync} from 'node:fs';
import test from 'node:test';
import vm from 'node:vm';
const bytes = readFileSync(new URL('../../gymbout/testdata/reference/bout_v1.json', import.meta.url));
assert.equal(createHash('sha256').update(bytes).digest('hex'), '09f0430789328b9226fc4a24aae3cdb07beeb8b41d61b42cd33e4ba3121fa2ec');
const ref = JSON.parse(bytes), clone = v => JSON.parse(JSON.stringify(v));
const source = readFileSync(new URL('./stable.js', import.meta.url), 'utf8'), html = readFileSync(new URL('./stable.html', import.meta.url), 'utf8');
const script = source.slice("import { Call } from '/wails/runtime.js';".length, source.indexOf("ui['event-refresh'].addEventListener"));
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
function dto(kind, name) { const v = ref.dtos.find(d => d.kind === kind && d.name === name); assert.ok(v, kind + '/' + name); return clone(v.value); }
function harness(composition = ref.compositions[0], viewIndex = 0) {
  const c = clone(composition), view = c.views[viewIndex], pin = c.pin, eventPin = pin.source_pin.source_pin.source_pin;
  const ownPin = eventPin.participants.find(p => p.stable_pin.account_id === view.account_id), gymID = ownPin.stable_pin.identity.gym_id;
  const fixture = gymID === 'a227587d837507dbcb5f9d781967339d816070fb7bd574931c690ba6937b88e5' ? 'zero256' : 'incrementing256';
  const elements = new Map([...html.matchAll(/\bid="([^"]+)"/g)].map(m => [m[1], new Element()]));
  const handlers = {}, calls = [], timers = new Map(); let id = 2000, active = 0, peak = 0, timerID = 0;
  const motion = {matches: false, addEventListener(_, fn) { this.change = fn; }};
  const document = {getElementById: id => elements.get(id), createElement: tag => new Element(tag), hidden: false};
  const context = vm.createContext({document, window: {matchMedia: () => motion}, Call: {async ByName(name, ...args) { const method = name.slice(9); calls.push({method, args: clone(args)}); assert.ok(handlers[method], `Unexpected ${method}`); active++; peak = Math.max(peak, active); try { return clone(await handlers[method](...args)); } finally { active--; } }}, Intl, Date, btoa: s => Buffer.from(s, 'ascii').toString('base64'), setTimeout(fn) { timers.set(++timerID, fn); return timerID; }, clearTimeout(id) { timers.delete(id); }});
  vm.runInContext(script, context, {filename: 'stable.js'});
  const h = {context, elements, handlers, calls, timers, motion, document, c, view, get peak() { return peak; }, run: s => vm.runInContext(s, context), set(n, v) { context.inputValue = clone(v); vm.runInContext(`${n}=inputValue`, context); }, count: m => calls.filter(c => c.method === m).length};
  h.set('current', {account: {state: 'signed_in', fixture, held: false, can_export: true}, view, gym_id: gymID});
  h.set('stable', {state: 'ready', view, status: {pin: ownPin.stable_pin, store_id: ownPin.stable_store_id, caught_up: true, pending_operation_id: '', slots: Array(10).fill('')}});
  h.set('eventAuthority', {pin: eventPin, storeID: pin.source_pin.source_pin.source_store_id, createdMS: 1});
  h.set('seedAuthority', {pin: pin.source_pin.source_pin, storeID: pin.source_pin.source_store_id, createdMS: 1});
  h.set('cardAuthority', {pin: pin.source_pin, storeID: pin.source_store_id, createdMS: c.source_store_created_at_ms});
  h.set('bookedAuthority', {pin, storeID: c.record.store_id, sourceCreatedMS: c.source_store_created_at_ms, createdMS: c.store_created_at_ms});
  h.set('bookedSnapshot', {version: 1, state: 'ready', error: '', view, store_id: c.record.store_id, pin, source_created_at_ms: c.source_store_created_at_ms, created_at_ms: c.store_created_at_ms, active_job_id: '', queued_job_id: '', last_job_id: ''});
  h.set('entryDate', c.source.event.saturday); h.run('enabled=true;rememberedView=currentKey()');
  h.reply = (field, value) => ({id: ++id, ok: true, view: clone(view), store_id: c.record.store_id, snapshot: null, card_status: null, status: null, record: null, job: null, error: '', [field]: clone(value)});
  h.refusal = error => ({id: ++id, ok: false, view: null, store_id: '', snapshot: null, card_status: null, status: null, record: null, job: null, error});
  h.unsaved = () => dto('Status', 'unsaved/' + c.name); h.saved = () => dto('Status', 'saved/' + c.name);
  h.select = (saved = false) => { const s = saved ? h.saved() : h.unsaved(); h.set('bookedPage', {...s, bouts: [s.saved]}); h.run('delete bookedPage.saved'); h.run(`bookedSelect('${c.source.bout.bout_id}')`); };
  return h;
}
function watch(h, j) { h.set('bookedWatch', {id: j.job_id, state: 'queued', date: j.saturday, boutID: j.bout_id, cardID: j.card_id}); }
const deferred = () => { let resolve; const promise = new Promise(r => { resolve = r; }); return {promise, resolve}; };

test('all 11 frozen independent saved compositions reach the UI without engine or condition projection', () => {
  for (const c of ref.compositions) { const h = harness(c); h.context.input = dto('ReadResult', 'saved/' + c.name); h.run("condition=()=>{throw Error('projection')};fullItem=()=>{throw Error('projection')}"); assert.doesNotThrow(() => h.run('bookedResultValue(input)'), c.name); }
});
test('every accepted wire response passes presentation validation with its original authority and view', () => {
  let count = 0;
  for (const c of ref.protocol_cases.filter(c => c.accept && c.response.ok)) {
    const composition = ref.compositions.find(v => JSON.stringify(v.pin) === JSON.stringify(c.authority_pin)), h = harness(composition, c.fixture_index);
    h.set('current.view', c.current_view); h.set('stable.view', c.current_view); h.set('bookedSnapshot.view', c.current_view);
    if (c.response.snapshot?.state === 'setup_incomplete') h.run('bookedAuthority=null');
    const field = ['snapshot', 'card_status', 'status', 'record', 'job'].find(k => c.response[k] !== null); h.context.input = clone(c.response);
    assert.doesNotThrow(() => h.run(`bookedEnvelope(input,'${field}')`), c.name); count++;
  } assert.ok(count >= 20);
});
test('snapshot polling and selection never run or refresh a chain', async () => {
  const h = harness(); h.handlers.BoutSnapshot = () => h.reply('snapshot', h.run('bookedSnapshot'));
  await h.run('fetchBookedSnapshot()'); h.select(); h.run('renderBooked()');
  assert.deepEqual(h.calls.map(v => v.method), ['BoutSnapshot']); assert.equal(h.run('bookedRecord'), null); assert.equal(h.peak, 1);
});
test('both held original fixture views can explicitly queue a run', async () => {
  for (const i of [0, 1]) { const h = harness(undefined, i); h.select(); h.run('current.account.held=true'); h.handlers.BeginBoutRun = () => h.reply('job', dto('BoutJob', 'queued-' + i)); await h.run('bookedBegin()'); assert.equal(h.count('BeginBoutRun'), 1); assert.equal(h.run('bookedWatch.state'), 'queued'); assert.equal(h.run('bookedRetainedBytes()'), 393216); }
});
test('unknown Run is query-first and never automatically repeated', async () => {
  const h = harness(); h.select(); h.handlers.BeginBoutRun = () => h.refusal('outcome_unknown'); await h.run('bookedBegin()'); await h.run('bookedBegin()');
  assert.equal(h.count('BeginBoutRun'), 1); assert.ok(h.run('bookedUnknown')); assert.match(h.run('bookedMessage'), /Check saved result/);
  h.handlers.BoutStatus = () => h.reply('status', h.unsaved()); await h.run("bookedRead('status')"); assert.equal(h.run('bookedUnknown'), null); assert.equal(h.count('BeginBoutRun'), 1);
});
test('saved records open paused and replay only local retained ticks and scorecard', async () => {
  const h = harness(); h.select(true); h.handlers.BoutRecord = () => h.reply('record', dto('ReadResult', 'saved/genuine_set46_middle')); await h.run("bookedRead('record')");
  assert.equal(h.run('bookedTimer'), null); assert.equal(h.run('bookedStep'), 0); assert.equal(h.run('bookedRecord.result_digest'), h.c.result_digest);
  h.run('stepBooked(bookedTicks());stepBooked(0);playBooked()'); assert.ok(h.timers.size > 0); const tick = [...h.timers.values()].at(-1); tick(); assert.equal(h.run('bookedStep'), 1);
  h.motion.matches = true; h.motion.change(); assert.equal(h.run('bookedTimer'), null); h.run('playBooked()'); assert.equal(h.run('bookedTimer'), null);
  h.run('stepBooked(bookedTicks())'); assert.equal(h.elements.get('bout-outcome').hidden, false); assert.match(h.elements.get('bout-scores').textContent, /Whole-fight scorecard/); assert.equal(h.calls.length, 1);
});
test('old-view and old-selection Record replies cannot repopulate the replay', async () => {
  for (const logout of [true, false]) { const h = harness(); h.select(true); const gate = deferred(); h.handlers.BoutRecord = () => gate.promise; const p = h.run("bookedRead('record')"); h.run(logout ? 'epoch++;clearBooked();current=null;stable=null' : "entryDate='2026-11-28';bookedSelectDate()"); gate.resolve(h.reply('record', dto('ReadResult', 'saved/genuine_set46_middle'))); await p; assert.equal(h.run('bookedRecord'), null); assert.equal(h.run('bookedStatus'), null); }
});
test('a late Run ACK is tracked without replacing another selected date', async () => {
  const h = harness(); h.select(); const gate = deferred(); h.handlers.BeginBoutRun = () => gate.promise; const p = h.run('bookedBegin()'); h.run("entryDate='2026-11-28';bookedSelectDate()"); gate.resolve(h.reply('job', dto('BoutJob', 'queued-0'))); await p; assert.equal(h.run('bookedWatch.date'), '2026-11-21'); assert.equal(h.run('bookedRecord'), null); assert.equal(h.run('bookedSelected'), '');
});
test('new explicit conflict and observation uncertainty win over an older terminal overlay', async () => {
  for (const uncertainty of [false, true]) { const h = harness(); h.select(); const s = dto('Status', 'historical_conflict'); h.context.input = s; h.run('bookedAdoptStatus(input)'); if (uncertainty) h.run("bookedRefusal('outcome_unknown')");
    const j = dto('BoutJob', 'completed-0'); watch(h, j); h.handlers.BoutJob = () => h.reply('job', j); await h.run(`bookedFetchJob('${j.job_id}')`);
    assert.equal(h.run('bookedRecord.result_digest'), h.c.result_digest); assert.equal(h.run('bookedStatus.saved.usable'), false); assert.equal(h.run('bookedOverlayKnown'), !uncertainty);
  }
});
test('terminal tuple and records stay immutable across repeated polls', async () => {
  const h = harness(); h.select(); const j = dto('BoutJob', 'completed-0'); watch(h, j); h.handlers.BoutJob = () => h.reply('job', j); await h.run(`bookedFetchJob('${j.job_id}')`); const before = h.run('JSON.stringify(bookedRecord)');
  j.result.replayed = !j.result.replayed; await h.run(`bookedFetchJob('${j.job_id}')`); assert.equal(h.run('JSON.stringify(bookedRecord)'), before); assert.ok(h.run('bookedUnknown'));
});
test('shared capacity refuses before Run and preserves old evidence and unknowns', async () => {
  const h = harness(); h.select(); h.set('pageItems', [{padding: 'x'.repeat(800000)}]); await h.run('bookedBegin()'); assert.equal(h.calls.length, 0); assert.equal(h.run('bookedWatch'), null); assert.match(h.run('bookedMessage'), /full/);
  h.set('pageItems', []); h.set('bookedWatch', {id: '', state: 'queued'}); const left = h.run('CACHE_MAX-(previewRetainedBytes()+entryRetainedBytes()+eventRetainedBytes()+seedRetainedBytes()+cardRetainedBytes()+bookedRetainedBytes()+JSON.stringify(pageItems).length)'); assert.equal(h.run(`previewCapacity(${left})`), true); assert.equal(h.run(`previewCapacity(${left + 1})`), false);
});
test('transactional terminal capacity rejection retains the prior accepted state', async () => {
  const h = harness(); h.select(); const j = dto('BoutJob', 'completed-0'); watch(h, j); const before = h.run('JSON.stringify([bookedWatch,bookedLastJob,bookedTerminal,bookedRecord,bookedStatus,bookedOverlayKnown,bookedOverlayRead,bookedUnknown])');
  h.handlers.BoutJob = () => h.reply('job', j); h.run('bookedCapacity=()=>false'); await h.run(`bookedFetchJob('${j.job_id}')`); assert.equal(h.run('JSON.stringify([bookedWatch,bookedLastJob,bookedTerminal,bookedRecord,bookedStatus,bookedOverlayKnown,bookedOverlayRead,bookedUnknown])'), before);
});
test('missing card and empty saved card are distinct; unmatched fighters never become bouts', async () => {
  for (const name of ['absent', 'completed_empty']) { const h = harness(); h.handlers.BoutCardStatus = () => h.reply('card_status', dto('CardStatus', name)); await h.run("bookedRead('card_status')"); assert.equal(h.run('bookedPage.bouts.length'), 0); assert.match(h.elements.get('bout-empty').textContent, name === 'absent' ? /No card/ : /no booked bouts/); assert.equal(h.elements.get('bout-run').disabled, true); }
});
test('body identity, scalar versions, zero timestamps and wrong source bindings refuse', () => {
  const h = harness(); const r = dto('ReadResult', 'saved/genuine_set46_middle'); for (const mutate of [v => v.record.result.version = true, v => v.record.input.seed = 'f'.repeat(64), v => v.status.saved.input_id = 'e'.repeat(64), v => v.record.input.a.manifest.name.first = 'A'.repeat(65)]) { const v = clone(r); mutate(v); h.context.input = v; assert.throws(() => h.run('bookedResultValue(input)')); }
  for (const name of ['unavailable', 'setup-0']) { const v = dto('BoutSnapshot', name); v.created_at_ms = false; const response = h.reply('snapshot', v); response.view = v.view; response.store_id = v.store_id; h.context.input = response; assert.throws(() => h.run("bookedEnvelope(input,'snapshot')")); }
});
test('unknown and malformed native portrait replies clear current usability and cached images', async () => {
  const h = harness(); h.select(true); h.context.input = dto('ReadResult', 'saved/genuine_set46_middle'); h.run('bookedAdoptResult(input)'); h.handlers.BoutPortrait = () => ({version: 1, ok: false, portrait: null, status: null, error: 'outcome_unknown'});
  await h.run('loadBookedPortrait(bookedRecord.source.corners[0])'); assert.equal(h.run('bookedOverlayKnown'), false); assert.equal(h.run('bookedImages.size'), 0); assert.equal(h.count('BeginBoutRun'), 0);
});
test('build script selects the additive mode and preserves normal-profile and icon guards', () => {
  const b = readFileSync(new URL('../scripts/build-manifest-macos.sh', import.meta.url), 'utf8'); assert.match(b, /-X main.boutBuild=1/); assert.match(b, /<string>0.4.11<\/string>/); assert.match(b, /<string>51<\/string>/); assert.match(b, /slugtestclock\|slugtestpow/); assert.match(b, /ForTestClock/); assert.match(b, /build-icons-macos.sh/); assert.doesNotMatch(b, /-tags=[^\n]*slugtest/);
});

test('48 independent portrait envelope controls require exact tuple and fresh status even on cache hits', async () => {
  // The frozen portrait cases deliberately contain a non-rendered placeholder.
  // Supply a preexisting safe independent SVG only to exercise DOM image admission;
  // native, not this UI test, checks renderer output against the expected digest.
  const svg = JSON.parse(readFileSync(new URL('../../fighterportrait/testdata/reference/portraits_v1.json', import.meta.url))).cases[0].svg;
  let count = 0;
  for (const c of ref.portrait_cases) {
    const h = harness(ref.compositions[0], c.fixture_index), reply = clone(c.response);
    h.set('current.view', c.current_view); h.set('stable.view', c.current_view); h.set('bookedSnapshot.view', c.current_view); h.select(true);
    if (c.selected_record_fixture !== null) { h.context.input = dto('ReadResult', 'saved/genuine_set46_middle'); h.run('bookedAdoptResult(input)'); }
    if (reply.portrait) reply.portrait.svg = svg;
    h.handlers.BoutPortrait = () => reply;
    const id = reply.portrait?.prospect_id ?? h.c.source.bout.corner_a;
    h.context.corner = clone(h.c.source.corners.find(v => v.member.assignment.prospect_id === id) ?? h.c.source.corners[0]);
    if (c.cache_hit && c.accept && reply.ok) { await h.run('loadBookedPortrait(corner)'); assert.equal(h.run('bookedImages.size'), 1, c.name); }
    await h.run('loadBookedPortrait(corner)');
    assert.equal(h.run('bookedImages.size') > 0, c.accept && reply.ok, c.name);
    if (c.accept) assert.equal(h.count('BoutPortrait'), c.cache_hit && reply.ok ? 2 : 1, c.name);
    if (c.name.startsWith('historical_conflict')) { assert.equal(h.run('bookedStatus.saved.usable'), false); assert.match(h.elements.get('bout-overlay').textContent, /conflict/); }
    count++;
  } assert.equal(count, 48);
});
test('late portrait completion cannot populate a new view or selected Saturday', async () => {
  for (const close of [false, true]) {
    const h = harness(); h.select(true); h.context.input = dto('ReadResult', 'saved/genuine_set46_middle'); h.run('bookedAdoptResult(input)');
    const gate = deferred(); h.handlers.BoutPortrait = () => gate.promise; const p = h.run('loadBookedPortrait(bookedRecord.source.corners[0])');
    h.run(close ? 'epoch++;clearBooked();current=null;stable=null' : "entryDate='2026-11-28';bookedSelectDate()"); gate.resolve(clone(ref.portrait_cases[0].response)); await p; assert.equal(h.run('bookedImages.size'), 0); assert.equal(h.run('bookedRecord'), null);
  }
});
test('saved evidence disables Run even when a newer metadata observation was unresolved', async () => {
  const h = harness(); h.select(); h.run("bookedRefusal('outcome_unknown')"); const j = dto('BoutJob', 'completed-0'); watch(h, j); h.handlers.BoutJob = () => h.reply('job', j); await h.run(`bookedFetchJob('${j.job_id}')`);
  assert.ok(h.run('bookedRecord')); assert.equal(h.elements.get('bout-run').disabled, true); await h.run('bookedBegin()'); assert.equal(h.count('BeginBoutRun'), 0);
});
test('an unknown original bout stays recoverable after date navigation without Run', async () => {
  const h = harness(); h.select(); h.handlers.BeginBoutRun = () => h.refusal('outcome_unknown'); await h.run('bookedBegin()'); h.run("entryDate='2026-11-28';bookedSelectDate();renderBooked()");
  assert.equal(h.elements.get('bout-check').disabled, false); assert.match(h.elements.get('bout-message').textContent, /2026-11-21/);
  // The event handler uses the serialized scheduler; inspect queued work without
  // starting unrelated periodic polling in this focused harness.
  h.run('pump=()=>undefined;render=renderBooked'); h.handlers.BoutStatus = () => h.reply('status', h.saved()); h.run('bookedCheck()'); await h.run('tasks.shift()()');
  assert.equal(h.run('entryDate'), '2026-11-21'); assert.equal(h.run('bookedSelected'), h.c.source.bout.bout_id); assert.equal(h.run('bookedUnknown'), null); assert.equal(h.count('BeginBoutRun'), 1); assert.equal(h.count('BoutStatus'), 1);
});
test('no second transcript or full render-key copy is retained for replay', async () => {
  const h = harness(); h.select(); const j = dto('BoutJob', 'completed-0'); watch(h, j); h.handlers.BoutJob = () => h.reply('job', j); await h.run(`bookedFetchJob('${j.job_id}')`);
  assert.equal(h.run('bookedWatch'), null); assert.ok(h.run('bookedRecord')); assert.equal(h.run('bookedTerminal.key.includes("damage_taken_q")'), false); assert.equal(h.run('bookedRendered.includes("damage_taken_q")'), false); assert.equal(h.run('bookedMetadataBytes()<73728'), true); assert.equal(h.run('bookedRetainedBytes()<393216'), true);
  h.run('bookedClearRecord()'); assert.equal(h.run('bookedRecord'), null); assert.equal(h.run('bookedImages.size'), 0); assert.equal(h.run('bookedRendered'), '');
});
test('normal typed busy preserves accepted result while current-view failure clears it', async () => {
  const h = harness(); h.select(true); h.context.input = dto('ReadResult', 'saved/genuine_set46_middle'); h.run('bookedAdoptResult(input)'); const before = h.run('JSON.stringify(bookedRecord)');
  h.handlers.BoutStatus = () => h.refusal('busy'); await h.run("bookedRead('status')"); assert.equal(h.run('JSON.stringify(bookedRecord)'), before);
  h.handlers.BoutStatus = () => h.refusal('stale_view'); await h.run("bookedRead('status')"); assert.equal(h.run('bookedRecord'), null); assert.equal(h.run('bookedImages.size'), 0); assert.equal(h.run('bookedTimer'), null);
});
test('stale common metadata page cannot overwrite a newer selected status', async () => {
  const h = harness(); h.select(true); const fresh = h.saved(); fresh.observed_at_ms += 100; fresh.source.observed_at_ms = fresh.observed_at_ms; h.set('bookedStatus', fresh); const before = h.run('JSON.stringify(bookedPage)');
  const s = h.saved(), page = {...s, bouts: [s.saved]}; delete page.saved; h.handlers.BoutCardStatus = () => h.reply('card_status', page); await h.run("bookedRead('card_status')");
  assert.equal(h.run('JSON.stringify(bookedPage)'), before); assert.equal(h.run('bookedStatus.observed_at_ms'), fresh.observed_at_ms);
});

test('every retained independent bout labels replay provisional until its own result finality is loaded', () => {
  for (const c of ref.compositions) {
    const h = harness(c); h.select(true); h.context.input = dto('ReadResult', 'saved/' + c.name);
    h.run('bookedResultValue(input);bookedAdoptResult(input);renderBooked()');
    assert.match(h.elements.get('bout-state').textContent, /Provisional result/, c.name);
    assert.match(h.elements.get('bout-asof').textContent, /^Provisional result/, c.name);
    assert.equal(h.elements.get('bout-outcome').hidden, true);
    h.run('stepBooked(bookedTicks())'); assert.match(h.elements.get('bout-finish').textContent, /^Provisional result:/, c.name);
    assert.match(h.elements.get('bout-scores').textContent, /^Provisional result/, c.name);
    assert.equal(h.calls.length, 0);
  }
});

test('display-only finish controls qualify every win and both draw labels', () => {
  // These are renderer branch controls, not new accepted transcripts or oracle outcomes.
  for (const method of ['decision', 'ko', 'tko_referee', 'tko_three_knockdowns', 'tko_corner', 'decision_draw', 'double_corner_stoppage']) {
    const h = harness(); h.select(true); h.context.input = dto('ReadResult', 'saved/genuine_set46_middle'); h.run('bookedAdoptResult(input)');
    h.context.method = method; h.run("bookedRecord.result.method=method;if(method==='decision_draw'||method==='double_corner_stoppage')bookedRecord.result.winner='';stepBooked(bookedTicks())");
    const text = h.elements.get('bout-finish').textContent; assert.match(text, /^Provisional result:/, method);
    if (method === 'decision_draw' || method === 'double_corner_stoppage') assert.match(text, /Candidate draw/); else assert.match(text, /wins the candidate bout/);
    assert.equal(h.calls.length, 0);
  }
});
