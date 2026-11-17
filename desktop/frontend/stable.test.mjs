// DOM-only stable tests. Public fixtures; no browser, wallet, node or authority writes.
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { setImmediate } from 'node:timers/promises';
import test from 'node:test';
import vm from 'node:vm';
const source = readFileSync(new URL('./stable.js', import.meta.url), 'utf8');
const html = readFileSync(new URL('./stable.html', import.meta.url), 'utf8');
const runtime = "import { Call } from '/wails/runtime.js';";
assert.ok(source.startsWith(runtime));
const reference = JSON.parse(readFileSync(new URL('../../gymstable/testdata/reference/stable_v1.json', import.meta.url), 'utf8'));
const portraits = JSON.parse(readFileSync(new URL('../../fighterportrait/testdata/reference/portraits_v1.json', import.meta.url), 'utf8'));
const clone = (v) => JSON.parse(JSON.stringify(v));
const hash = (n) => n.toString(16).padStart(64, '0');
class Element {
  constructor(tag = 'div') { this.tagName = tag; this.children = []; this.parent = null; this.attributes = new Map(); this.listeners = new Map(); this.style = {}; this.hidden = false; this.disabled = false; this.className = ''; this._text = ''; }
  set textContent(v) { this._text = String(v); for (const c of this.children) c.parent = null; this.children = []; }
  get textContent() { return this._text + this.children.map((c) => c.textContent).join(''); }
  set innerHTML(_) { throw new Error('Unsafe HTML interpolation'); }
  setAttribute(k, v) { this.attributes.set(k, String(v)); }
  removeAttribute(k) { this.attributes.delete(k); }
  addEventListener(k, f) { const events = this.listeners.get(k) ?? []; events.push(f); this.listeners.set(k, events); }
  async dispatch(k) { if (this.disabled && k === 'click') return; await Promise.all((this.listeners.get(k) ?? []).map((f) => f())); }
  append(...children) { for (const c of children) this.insertBefore(c, null); }
  insertBefore(c, next) { if (c === next) return; c.remove(); const i = next === null ? this.children.length : this.children.indexOf(next); assert.ok(i >= 0); this.children.splice(i, 0, c); c.parent = this; }
  remove() { if (this.parent) { this.parent.children.splice(this.parent.children.indexOf(this), 1); this.parent = null; } }
}
function descendants(n, predicate) { return n.children.flatMap((c) => [...(predicate(c) ? [c] : []), ...descendants(c, predicate)]); }
function deferred() { let resolve, reject; const promise = new Promise((y, n) => { resolve = y; reject = n; }); return { promise, resolve, reject }; }
const caseA = reference.cases[0];
function item(c = caseA, index = 0) { return clone({ assignment: c.assignment, manifest: c.manifest, condition: c.conditions[index].expected }); }
function environment(fixture = 'zero256', list = [item()]) {
  const pin = reference.pins.find((p) => p.name === fixture); const count = Math.max(0, ...list.map((i) => i.assignment.sequence)); const asOf = Math.max(0, ...list.map((i) => i.condition.as_of_ms));
  const view = { version: 1, view_id: hash(fixture === 'zero256' ? 60 : 61), account_authority_id: pin.pin.account_authority_id, account_id: pin.pin.account_id, generation: 1, authority_pin: clone(pin.pin.source), authority_origin: 'http://127.0.0.1:32123', transport_trust_id: hash(90) };
  const manifest = { version: 1, mode: 'local-manifest-pilot', state: 'stopped', attempts: 0, hashes_per_second: 0, height: 0, pending: 0, last_error: '', recovery_error: '', wallet_state: 'closed', wallet_height: 0, payout_address: 'a'.repeat(34), gym_id: pin.pin.identity.gym_id, gym_public_key: pin.pin.identity.gym_public_key, spendable_atoms: 0, immature_atoms: 0, backup_state: 'idle', backup_phase: '', backup_error: '', account: { state: 'signed_in', fixture, id: view.account_id, generation: 1, hold_until_ms: 0, held: false, can_export: true, last_error: '' }, view, accepted_count: count, projection: { state: 'idle', error: '', revision: 0, through_sequence: 0, has_older: count > 0, complete_recent: false, items: [] } };
  const status = { version: 1, store_id: pin.expected_id, pin: clone(pin.pin), checkpoint: count, observed_source_count: count, caught_up: true, roster_revision: 0, slots: Array(10).fill(''), operation_count: 0, pending_operation_id: '', last_operation_id: '', last_as_of_ms: asOf };
  const stable = { version: 1, state: 'ready', error: '', view: clone(view), status, active_job_id: '', queued_job_id: '', last_job_id: '', pending_operation_id: '' };
  return { manifest, stable, items: clone(list) };
}
function blank() {
  const e = environment('zero256', []); Object.assign(e.manifest, { payout_address: '', gym_id: '', gym_public_key: '', view: null }); e.manifest.account = { state: 'signed_out', fixture: '', id: '', generation: 0, hold_until_ms: 0, held: false, can_export: false, last_error: '' };
  e.stable = { version: 1, state: 'unavailable', error: '', view: null, status: null, active_job_id: '', queued_job_id: '', last_job_id: '', pending_operation_id: '' }; return e;
}
function makeJob(e, id, kind, result = null, op = '') {
  const j = { version: 1, job_id: id, kind, state: result ? 'done' : 'queued', view: clone(e.manifest.view), store_id: e.stable.status.store_id, operation_id: op, error: '', sync: null, page: null, roster: null, intent: null, mutation: null };
  if (result) j[{ sync: 'sync', inventory: 'page', roster: 'roster', prepare: 'intent', apply: 'mutation' }[kind]] = clone(result); return j;
}
async function flush() { for (let i = 0; i < 25; i++) await setImmediate(); }
function harness(initial = environment(), mode = 'local-manifest-pilot') {
  const elements = new Map([...html.matchAll(/\bid="([^"]+)"/g)].map((m) => [m[1], new Element()]));
  const document = new Element(); document.hidden = false; document.getElementById = (id) => elements.get(id); document.createElement = (tag) => new Element(tag);
  const window = new Element(), calls = [], timers = new Map(), jobs = new Map(), history = new Map(); let counter = 0, id = 500, frame = 0, state = clone(initial), concurrent = 0, peak = 0;
  const ok = (kind, value) => ({ id: ++frame, ok: true, [kind]: clone(value) }); const error = (value) => ({ id: ++frame, ok: false, error: value });
  const wrapOp = (o) => ({ version: 1, view: clone(state.manifest.view), store_id: state.stable.status.store_id, operation: o ? clone(o) : null });
  function publish(kind, result, op = '') { const jid = hash(++id), j = makeJob(state, jid, kind, result, op); jobs.set(jid, j); state.stable.last_job_id = jid; return ok('job', { ...j, state: 'queued', sync: null, page: null, roster: null, intent: null, mutation: null }); }
  function initialJob() { if (state.items.length) { const data = { version: 1, status: clone(state.stable.status), as_of_ms: state.stable.status.last_as_of_ms, items: clone(state.items) }; publish('sync', data); } }
  initialJob();
  const handlers = {
    Mode: () => mode, ManifestSnapshot: () => clone(state.manifest), StableSnapshot: () => ok('snapshot', state.stable),
    ManifestPortrait: () => { throw new Error('fixture portrait is not loaded'); },
    StableJob: (view, jid) => jobs.has(jid) ? ok('job', jobs.get(jid)) : error('stable_not_found'),
    BeginStableSync: () => publish('sync', { version: 1, status: clone(state.stable.status), as_of_ms: state.stable.status.last_as_of_ms || 1794893590614, items: [] }),
    BeginStableRecent: () => { const after=Math.max(0,state.stable.status.checkpoint-8), list=state.items.filter(i=>i.assignment.sequence>after).sort((a,b)=>a.assignment.sequence-b.assignment.sequence), next=list.at(-1)?.assignment.sequence??after; return publish('inventory',{version:1,status:clone(state.stable.status),as_of_ms:state.stable.status.last_as_of_ms,after,next_after:next,has_more:next<state.stable.status.checkpoint,items:clone(list)}); },
    BeginStableInventory: (_, after, limit) => { const list = state.items.filter((i) => i.assignment.sequence > after).slice(0, limit); const next = list.at(-1)?.assignment.sequence ?? after; return publish('inventory', { version: 1, status: clone(state.stable.status), as_of_ms: state.stable.status.last_as_of_ms, after, next_after: next, has_more: next < state.stable.status.checkpoint, items: clone(list) }); },
    BeginStableRoster: (_, first, limit) => publish('roster', { version: 1, status: clone(state.stable.status), as_of_ms: state.stable.status.last_as_of_ms, first_slot: first, next_slot: first + limit > 10 ? 0 : first + limit, items: state.stable.status.slots.map((id, n) => ({ slot: n + 1, item: state.items.find((i) => i.assignment.prospect_id === id) })).filter((x) => x.slot >= first && x.slot < first + limit && x.item) }),
    BeginStablePrepare: (view, revision, slot, prospectID) => {
      const s = state.stable.status, op = hash(++id); const intent = { version: 1, sequence: ++s.operation_count, operation_id: op, generation: state.manifest.view.generation, view_id: view, request: { expected_revision: revision, slot, prospect_id: prospectID }, prepared_at_ms: s.last_as_of_ms };
      const record = { version: 1, intent, state: 'prepared', receipt: null, abandoned_at_ms: 0 }; history.set(op, record); s.pending_operation_id = state.stable.pending_operation_id = s.last_operation_id = op; return publish('prepare', intent);
    },
    BeginStableApply: (view, op) => {
      const record = history.get(op), s = state.stable.status, r = record.intent.request, previous = s.slots[r.slot - 1], before = s.roster_revision;
      s.slots[r.slot - 1] = r.prospect_id; const changed = previous !== r.prospect_id; if (changed) s.roster_revision++; s.pending_operation_id = state.stable.pending_operation_id = '';
      const receipt = { version: 1, operation_id: op, generation: state.manifest.view.generation, view_id: view, request: clone(r), previous_prospect_id: previous, changed, previous_revision: before, revision: s.roster_revision, slots: clone(s.slots), committed_at_ms: s.last_as_of_ms };
      Object.assign(record, { state: 'committed', receipt }); return publish('apply', { version: 1, receipt, replayed: false }, op);
    },
    StableOperation: (_, op) => history.has(op) ? ok('operation', wrapOp(history.get(op))) : error('stable_not_found'),
    StablePending: () => ok('operation', wrapOp(history.get(state.stable.status.pending_operation_id))),
    AbandonStableOperation: (_, op) => { const r = history.get(op); if (r.state === 'prepared') { r.state = 'abandoned'; r.abandoned_at_ms = state.stable.status.last_as_of_ms; state.stable.status.pending_operation_id = state.stable.pending_operation_id = ''; } return ok('operation', wrapOp(r)); },
    CancelStableJob: (_, jid) => { const j = jobs.get(jid); if (['queued', 'running'].includes(j.state)) Object.assign(j, { state: 'cancelled', error: 'stable_cancelled' }); return ok('job', j); },
    Start: () => { state.manifest.state = 'running'; }, Stop: () => { state.manifest.state = 'stopped'; }, LoginFixture: (fixture) => { state = environment(fixture, []); }, Logout: () => { state = blank(); }, ResetFixtureAccount: () => { state = blank(); }, ExportManifestReceipt: () => '', ExportRecovery: () => '', VerifyBackup: () => '',
  };
  const Call = { async ByName(name, ...args) { const method = name.slice(9); assert.match(name, /^main\.Gym\./); assert.ok(Object.hasOwn(handlers, method), `Unexpected API ${method}`); calls.push({ method, args: clone(args) }); concurrent++; peak = Math.max(peak, concurrent); try { return await handlers[method](...args); } finally { concurrent--; } } };
  const context = vm.createContext({ document, window, Call, Intl, Date, btoa: (s) => Buffer.from(s, 'ascii').toString('base64'), setTimeout(f, delay) { assert.equal(delay, 1000); const id = ++counter; timers.set(id, f); return id; }, clearTimeout(id) { timers.delete(id); } });
  return { elements, document, window, calls, timers, jobs, history, handlers, context, ok, error, publish, get state() { return state; }, set state(s) { state = clone(s); }, get peak() { return peak; }, count(m) { return calls.filter((c) => c.method === m).length; }, get cards() { return elements.get('cards').children; }, buttons(t) { return descendants(elements.get('cards'), (e) => e.tagName === 'button' && e.textContent === t); }, async start() { vm.runInContext(source.slice(runtime.length), context, { filename: 'stable.js' }); await flush(); }, async tick() { assert.equal(timers.size, 1); const [id, f] = timers.entries().next().value; timers.delete(id); f(); await flush(); }, async click(id) { await elements.get(id).dispatch('click'); await flush(); }, async save(slot = 1) { const select = descendants(elements.get('cards'), (e) => e.tagName === 'select')[0]; select.value = String(slot); await this.buttons('Save to slot')[0].dispatch('click'); await flush(); } };
}

test('explicit stable entry, exact approved asset and no mining on login', async () => {
  const h = harness(blank()); await h.start(); assert.deepEqual(h.calls.map((c) => c.method), ['Mode', 'ManifestSnapshot', 'StableSnapshot']); assert.equal(h.count('Start'), 0); assert.equal(h.elements.get('login-a').disabled, false);
  const wrong = harness(blank(), 'local-account-pilot'); await wrong.start(); assert.equal(wrong.count('ManifestSnapshot'), 0); assert.equal(wrong.timers.size, 0);
  assert.match(html, /src="\/stable.js"/); assert.match(html, /href="\/stable.css"/); assert.match(html, /src="\/slug-transparent.png"/); assert.doesNotMatch(html, /onclick=|https?:|<style/); assert.doesNotMatch(source, /\.innerHTML\s*=|crypto\.subtle|createElement\(['"]svg/);
  assert.equal(createHash('sha256').update(readFileSync(new URL('./slug-transparent.png', import.meta.url))).digest('hex'), '50ceada462548c677fb6e68e454625ca359aced2954daad5fb13abb9db85f693');
});
test('automatic typed Sync produces full dated condition and ten independent roster slots', async () => {
  const h = harness(); await h.start(); assert.equal(h.cards.length, 1); assert.match(h.cards[0].textContent, /Current|Rating|Recorded professional XP: 0/); assert.equal(h.elements.get('roster-slots').children.length, 10); assert.equal(h.count('Start'), 0); assert.equal(h.count('BeginStableSync'), 0); assert.equal(h.peak, 1); assert.equal(h.buttons('Save to slot')[0].disabled, false);
});
test('one choice durably prepares and applies without optimistic slot mutation', async () => {
  const h = harness(); await h.start(); const gate = deferred(), original = h.handlers.BeginStableApply; h.handlers.BeginStableApply = async (...args) => { await gate.promise; return original(...args); };
  const saving = h.save(10); await flush(); assert.equal(h.count('BeginStablePrepare'), 1); assert.equal(h.count('BeginStableApply'), 1); assert.equal(h.state.stable.status.slots[9], ''); assert.match(h.elements.get('roster-count').textContent, /0 \/ 10/);
  gate.resolve(); await saving; await flush(); assert.equal(h.state.stable.status.slots[9], caseA.assignment.prospect_id); assert.match(h.elements.get('roster-count').textContent, /1 \/ 10/); assert.equal(h.peak, 1);
});
test('committed Apply lost acknowledgment reconciles the same operation without reapplying', async () => {
  const h = harness(); await h.start(); const original = h.handlers.BeginStableApply; h.handlers.BeginStableApply = (...args) => { original(...args); throw new Error('private backend path'); }; await h.save(2);
  assert.equal(h.count('BeginStablePrepare'), 1); assert.equal(h.count('BeginStableApply'), 1); assert.equal(h.count('StableOperation'), 1); assert.equal(h.state.stable.status.roster_revision, 1); assert.match(h.elements.get('mutation-state').textContent, /confirmed|Roster saved/); assert.doesNotMatch(h.elements.get('mutation-state').textContent, /private backend/);
});
test('all 48 independent condition outputs and both fixture pins validate without a Go-derived golden', async () => {
  const h = harness(); await h.start(); let count = 0;
  for (const c of reference.cases) for (let n = 0; n < c.conditions.length; n++) { const i = item(c, n), e = environment(c.fixture, [i]); h.context.fixtureItem = clone(i); h.context.fixtureStatus = clone(e.stable.status); h.context.fixtureAsOf = i.condition.as_of_ms; assert.doesNotThrow(() => vm.runInContext('fullItem(fixtureItem, fixtureStatus, fixtureAsOf)', h.context)); count++; }
  assert.equal(count, 48); assert.ok(reference.cases.some((c) => c.manifest.portrait.shoulder_jitter < 0));
});
test('same-generation relogin can inspect but cannot execute an old prepared intent', async () => {
  const h = harness(); await h.start(); h.handlers.BeginStableApply = () => h.error('outcome_unknown'); await h.save(); const op = [...h.history.values()][0]; assert.equal(op.state, 'prepared');
  h.state.manifest.view.view_id = hash(999); h.state.stable.view.view_id = hash(999); h.state.stable.last_job_id = ''; await h.tick(); await h.click('reconcile');
  assert.equal(h.elements.get('apply-saved').disabled, true); assert.equal(h.elements.get('abandon').disabled, false); assert.match(h.elements.get('mutation-state').textContent, /earlier sign-in/); const count = h.count('BeginStableApply'); await h.click('apply-saved'); assert.equal(h.count('BeginStableApply'), count); await h.click('abandon'); assert.equal(op.state, 'abandoned'); assert.equal(h.state.stable.status.roster_revision, 0);
});

test('typed unknown Prepare is recovered but never auto-applied without its acknowledgment', async () => {
  const h = harness(); await h.start(); const original = h.handlers.BeginStablePrepare; h.handlers.BeginStablePrepare = (...args) => { original(...args); return h.error('outcome_unknown'); }; await h.save(7);
  assert.equal(h.count('BeginStablePrepare'), 1); assert.equal(h.count('BeginStableApply'), 0); assert.equal(h.count('StablePending'), 1); assert.equal(h.elements.get('apply-saved').disabled, false); assert.match(h.elements.get('mutation-state').textContent, /slot 7/);
  await h.click('apply-saved'); assert.equal(h.count('BeginStableApply'), 1); assert.equal(h.count('BeginStablePrepare'), 1); assert.equal(h.state.stable.status.slots[6], caseA.assignment.prospect_id);
});
test('definite stale revision rejects without optimistic roster changes or a new ID', async () => {
  const h = harness(); await h.start(); h.handlers.BeginStablePrepare = () => h.error('stable_conflict'); await h.save(); assert.equal(h.count('BeginStableApply'), 0); assert.equal(h.state.stable.status.roster_revision, 0); assert.match(h.elements.get('mutation-state').textContent, /roster changed/); assert.match(h.elements.get('roster-count').textContent, /0 \/ 10/);
});
test('held current sessions keep reversible roster controls and refuse recovery export', async () => {
  const e = environment(); e.manifest.account.held = true; e.manifest.account.can_export = false; e.manifest.account.hold_until_ms = e.stable.status.last_as_of_ms + 86400000; const h = harness(e); await h.start();
  assert.equal(h.buttons('Save to slot')[0].disabled, false); assert.equal(h.elements.get('export-recovery').disabled, true); await h.save(4); assert.equal(h.count('BeginStableApply'), 1); assert.equal(h.count('ExportRecovery'), 0);
});
test('logout invalidates an in-flight job result before an automatic Apply can occur', async () => {
  const h = harness(); await h.start(); const gate = deferred(), original = h.handlers.StableJob; let delayed = false;
  h.handlers.StableJob = (...args) => { const j = h.jobs.get(args[1]); if (j.kind === 'prepare' && !delayed) { delayed = true; return gate.promise; } return original(...args); };
  await h.save(); assert.equal(h.count('BeginStableApply'), 0); const jid = h.calls.filter((x) => x.method === 'StableJob').at(-1).args[1]; const old = h.ok('job', h.jobs.get(jid));
  const logout = h.elements.get('logout').dispatch('click'); await flush(); assert.equal(h.cards.length, 0); gate.resolve(old); await logout; await flush(); assert.equal(h.count('BeginStableApply'), 0); assert.equal(h.state.manifest.account.state, 'signed_out'); assert.equal(h.peak, 1);
});
test('fixed inventory page is protected while a new live arrival updates counts', async () => {
  const c = reference.cases.find((c) => c.fixture === 'zero256' && c.assignment.sequence === 1); const h = harness(environment('zero256', [item(c)])); await h.start(); await h.click('inventory'); assert.equal(h.cards.length, 1); const card = h.cards[0];
  // Transport-shape copy: new sequence/ID, no assertion of source provenance.
  const added = clone(h.state.items[0]); added.assignment.sequence = 2; added.assignment.prospect_id = hash(777); h.state.items.push(added); h.state.stable.status.checkpoint = h.state.stable.status.observed_source_count = h.state.manifest.accepted_count = 2; h.state.manifest.projection.has_older = true;
  h.publish('sync', { version: 1, status: clone(h.state.stable.status), as_of_ms: h.state.stable.status.last_as_of_ms, items: [added] }); await h.tick(); assert.equal(h.cards.length, 1); assert.equal(h.cards[0], card); assert.match(h.elements.get('stable-count').textContent, /2 assigned/); assert.match(h.elements.get('alert').textContent, /selected page/);
});
test('all ten roster slots remain accessible as bounded 8 plus 2 detail pages', async () => {
  const c = reference.cases.find((c) => c.fixture === 'zero256' && c.assignment.sequence === 1), first = item(c);
  // These are structural UI rows, not additional mined-proof goldens.
  const list = Array.from({ length: 10 }, (_, n) => { const i = clone(first); i.assignment.sequence = n + 1; i.assignment.prospect_id = hash(200 + n); return i; }); const e = environment('zero256', list);
  e.stable.status.slots = list.map((i) => i.assignment.prospect_id); e.stable.status.operation_count = e.stable.status.roster_revision = 10; e.stable.status.last_operation_id = hash(900);
  const h = harness(e); h.state.stable.last_job_id = ''; await h.start(); await h.click('roster-details'); assert.equal(h.cards.length, 8); assert.match(h.elements.get('page-label').textContent, /1–8/); await h.click('next'); assert.equal(h.cards.length, 2); assert.match(h.elements.get('page-label').textContent, /9–10/); assert.equal(h.elements.get('next').disabled, true); assert.equal(h.elements.get('roster-slots').children.length, 10);
  assert.deepEqual(h.calls.filter((c) => c.method === 'BeginStableRoster').map((c) => c.args.slice(1)), [[1, 8], [9, 2]]);
});
test('strict current-view, pin, revision and result-shape refusals never render foreign data', async (t) => {
  const mutations = {
    'wrong account': (e) => { e.stable.view.account_id = hash(12); },
    'missing field': (e) => { delete e.stable.last_job_id; },
    'foreign profile': (e) => { e.stable.status.pin.identity.profile = 'production'; },
    'lying caught-up': (e) => { e.stable.status.observed_source_count++; },
    'eleven slots': (e) => { e.stable.status.slots.push(''); },
    'duplicate slots': (e) => { e.stable.status.slots[0] = e.stable.status.slots[1] = caseA.assignment.prospect_id; },
    'unsafe generation': (e) => { e.stable.view.generation = Number.MAX_SAFE_INTEGER + 1; },
    'signed-out leak': (e) => { e.stable.state = 'unavailable'; e.stable.view = e.stable.status = null; e.stable.pending_operation_id = hash(2); },
  };
  for (const [name, change] of Object.entries(mutations)) await t.test(name, async () => { const h = harness(); h.state.stable.last_job_id = ''; change(h.state); await h.start(); assert.equal(h.cards.length, 0); assert.equal(h.elements.get('mining').disabled, true); assert.doesNotMatch(h.elements.get('alert').textContent, /production|5277ed/); });
  const h = harness(); await h.start(); h.state.stable.status.store_id = hash(88); await h.tick(); assert.equal(h.cards.length, 0); assert.equal(h.elements.get('mining').disabled, true);
});
test('invalid condition, negative fields and arbitrary markup fail before display', async (t) => {
  for (const [name, change] of Object.entries({ xp: (i) => { i.condition.pro_xp = 1; }, rating: (i) => { i.condition.rating_hundredths++; }, time: (i) => { i.condition.as_of_ms--; }, jitter: (i) => { i.manifest.portrait.shoulder_jitter = -7; }, zero: (i) => { i.manifest.dna.attributes.power = 0; }, markup: (i) => { i.manifest.name.first = '<script>'; }, receipt: (i) => { i.record = {}; } })) await t.test(name, async () => { const e = environment(); change(e.items[0]); const h = harness(e); await h.start(); assert.equal(h.cards.length, 0); assert.equal(h.count('ManifestPortrait'), 0); });
});
test('Pending null is allowed only for the correlated Pending request', async () => {
  const h = harness(); await h.start(); h.context.testFrame = h.ok('operation', { version: 1, view: clone(h.state.manifest.view), store_id: h.state.stable.status.store_id, operation: null });
  assert.doesNotThrow(() => vm.runInContext("response(testFrame, 'operation', current, stable, true)", h.context)); assert.throws(() => vm.runInContext("response(testFrame, 'operation', current, stable, false)", h.context));
});
test('native modal pauses polling and stale dialog success is discarded', async () => {
  const h = harness(); await h.start(); const gate = deferred(); h.handlers.ExportManifestReceipt = () => gate.promise;
  const exporting = h.buttons('Export public receipt')[0].dispatch('click'); await flush(); const polls = h.count('ManifestSnapshot'); await h.tick(); assert.equal(h.count('ManifestSnapshot'), polls);
  h.state = blank(); gate.resolve('Receipt saved.'); await exporting; await flush(); assert.equal(h.cards.length, 0); assert.doesNotMatch(h.elements.get('alert').textContent, /Receipt saved/); assert.equal(h.peak, 1);
});
test('independent image-only SVG consumer accepts all50 frozen bytes and refuses active content', async () => {
  const h = harness(blank()); await h.start(); for (const p of portraits.cases) { h.context.svg = p.svg; assert.equal(vm.runInContext('imageOnlySVG(svg)', h.context), true); }
  h.context.svg = portraits.cases[0].svg.replace('</svg>', '<script>bad()</script></svg>'); assert.equal(vm.runInContext('imageOnlySVG(svg)', h.context), false);
});
test('pagehide joins no new UI calls and drops images/results from a late reply', async () => {
  const h = harness(); await h.start(); const gate = deferred(); h.handlers.ManifestSnapshot = () => gate.promise; const ticking = h.tick(); await flush(); await h.window.dispatch('pagehide'); const count = h.calls.length; gate.resolve(clone(h.state.manifest)); await ticking; await flush(); assert.equal(h.calls.length, count); assert.equal(h.cards.length, 0); assert.equal(h.timers.size, 0);
});

test('new navigation cancels an obsolete queued page before requesting its replacement', async () => {
  const h = harness(); await h.start(); const original = h.handlers.BeginStableInventory;
  h.handlers.BeginStableInventory = (...args) => { const r = original(...args); h.jobs.set(r.job.job_id, clone(r.job)); return r; };
  await h.click('inventory'); assert.equal(h.elements.get('roster-details').disabled, false); const prior = h.calls.filter(c => c.method === 'StableJob').at(-1).args[1];
  await h.click('roster-details'); const actions = h.calls.filter(c => ['CancelStableJob', 'BeginStableRoster'].includes(c.method)); assert.deepEqual(actions.map(c => c.method), ['CancelStableJob', 'BeginStableRoster']); assert.equal(actions[0].args[1], prior); assert.equal(h.peak, 1);
});
test('roster page cannot omit an occupied slot or smuggle a slot beyond its range', async () => {
  const h = harness(); await h.start(); const i = item(); const st = clone(h.state.stable.status); st.slots[0] = i.assignment.prospect_id;
  h.context.rosterPage = {version:1,status:st,as_of_ms:st.last_as_of_ms,first_slot:1,next_slot:9,items:[]};
  assert.throws(() => vm.runInContext("pageValue(rosterPage, 'roster', current.view, current.account.fixture)",h.context));
  h.context.rosterPage.items=[{slot:1,item:i}]; assert.doesNotThrow(() => vm.runInContext("pageValue(rosterPage, 'roster', current.view, current.account.fixture)",h.context));
  h.context.rosterPage.first_slot=2; assert.throws(() => vm.runInContext("pageValue(rosterPage, 'roster', current.view, current.account.fixture)",h.context));
});

test('unavailable stable data preserves authenticated account and Stop controls', async () => {
  const h = harness(); h.state.manifest.state = 'running'; h.state.stable = blank().stable; h.state.stable.error = 'stable_unavailable'; await h.start();
  assert.equal(h.elements.get('logout').disabled, false); assert.equal(h.elements.get('reset').disabled, false); assert.equal(h.elements.get('mining').disabled, false); assert.equal(h.elements.get('mining').textContent, 'Stop mining'); assert.equal(h.cards.length, 0); assert.equal(h.elements.get('inventory').disabled, true);
  await h.click('mining'); assert.equal(h.count('Stop'), 1); assert.equal(h.count('Start'), 0); assert.equal(h.elements.get('export-recovery').disabled, false); await h.click('logout'); assert.equal(h.state.manifest.account.state, 'signed_out');
});
test('unavailable cannot carry a retained view and active/queued jobs cannot share identity', async (t) => {
  for (const mutate of [e=>{e.stable.state='unavailable';},e=>{e.stable.active_job_id=e.stable.queued_job_id=hash(501);}]) {
    const h=harness(); mutate(h.state); await h.start(); assert.equal(h.cards.length,0); assert.equal(h.elements.get('mining').disabled,true);
  }
});

test('fixed latest inventory stays fixed until explicit Recent follows new arrivals', async () => {
  const c=reference.cases.find(c=>c.fixture==='zero256'&&c.assignment.sequence===1),h=harness(environment('zero256',[item(c)])); await h.start(); await h.click('inventory');
  // Labelled structural transport copies; these do not assert mined provenance.
  function add(sequence) { const i=clone(h.state.items[0]);i.assignment.sequence=sequence;i.assignment.prospect_id=hash(810+sequence);h.state.items.push(i);h.state.stable.status.checkpoint=h.state.stable.status.observed_source_count=h.state.manifest.accepted_count=sequence;return i; }
  const two=add(2);h.publish('sync',{version:1,status:clone(h.state.stable.status),as_of_ms:h.state.stable.status.last_as_of_ms,items:[two]});await h.tick();assert.equal(h.cards.length,1);
  // A fresh arrival between snapshot and native Recent must not be rejected
  // against a locally calculated old checkpoint.
  const recent=h.handlers.BeginStableRecent;h.handlers.BeginStableRecent=(...args)=>{add(3);return recent(...args);};await h.click('recent');assert.equal(h.cards.length,3);assert.equal(h.count('BeginStableRecent'),1);assert.deepEqual(h.calls.filter(c=>c.method==='BeginStableRecent')[0].args,[h.state.manifest.view.view_id]);
  const four=add(4);h.publish('sync',{version:1,status:clone(h.state.stable.status),as_of_ms:h.state.stable.status.last_as_of_ms,items:[four]});await h.tick();assert.equal(h.cards.length,4);assert.equal(h.count('BeginStableInventory'),1);
});

test('Recent terminal page must end at its own checkpoint', async () => {
  const c=reference.cases.find(c=>c.fixture==='zero256'&&c.assignment.sequence===1), h=harness(environment('zero256',[item(c)]));await h.start();const native=h.handlers.BeginStableRecent;
  h.handlers.BeginStableRecent=(...args)=>{const r=native(...args), j=h.jobs.get(r.job.job_id);j.page.status.checkpoint=j.page.status.observed_source_count=2;j.page.next_after=1;j.page.has_more=true;return r;};await h.click('recent');assert.equal(h.cards.length,0);assert.match(h.elements.get('alert').textContent,/could not be validated/);
});
