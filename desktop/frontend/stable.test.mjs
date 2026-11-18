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
  const window = new Element(), motion = new Element(), timerDelays = new Map(), calls = [], timers = new Map(), jobs = new Map(), history = new Map(); let counter = 0, id = 500, frame = 0, state = clone(initial), concurrent = 0, peak = 0;
  motion.matches = false; window.matchMedia = () => motion;
  const ok = (kind, value) => ({ id: ++frame, ok: true, [kind]: clone(value) }); const error = (value) => ({ id: ++frame, ok: false, error: value });
  const wrapOp = (o) => ({ version: 1, view: clone(state.manifest.view), store_id: state.stable.status.store_id, operation: o ? clone(o) : null });
  function publish(kind, result, op = '') { const jid = hash(++id), j = makeJob(state, jid, kind, result, op); jobs.set(jid, j); state.stable.last_job_id = jid; return ok('job', { ...j, state: 'queued', sync: null, page: null, roster: null, intent: null, mutation: null }); }
  function initialJob() { if (state.items.length) { const data = { version: 1, status: clone(state.stable.status), as_of_ms: state.stable.status.last_as_of_ms, items: clone(state.items) }; publish('sync', data); } }
  initialJob();
  const handlers = {
    Mode: () => mode, CareerMode: () => false, ManifestSnapshot: () => clone(state.manifest), StableSnapshot: () => ok('snapshot', state.stable),
    PreviewSnapshot: () => ({ id: ++frame, version: 1, ok: true, snapshot: { version: 1, state: state.manifest.view ? 'idle' : 'unavailable', error: '', view: clone(state.manifest.view), store_id: state.stable.status?.store_id ?? '', engine_version: state.manifest.view ? 1 : 0, engine_id: state.manifest.view ? 'slug-fighter-bout-candidate-v1' : '', active_job_id: '', queued_job_id: '', last_job_id: '' } }),
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
  const context = vm.createContext({ document, window, Call, Intl, Date, btoa: (s) => Buffer.from(s, 'ascii').toString('base64'), setTimeout(f, delay) { assert.ok([250, 1000].includes(delay)); const id = ++counter; timers.set(id, f); timerDelays.set(id, delay); return id; }, clearTimeout(id) { timers.delete(id); timerDelays.delete(id); } });
  return { elements, document, window, motion, timerDelays, calls, timers, jobs, history, handlers, context, ok, error, publish, get state() { return state; }, set state(s) { state = clone(s); }, get peak() { return peak; }, count(m) { return calls.filter((c) => c.method === m).length; }, get cards() { return elements.get('cards').children; }, buttons(t) { return descendants(elements.get('cards'), (e) => e.tagName === 'button' && e.textContent === t); }, async start() { vm.runInContext(source.slice(runtime.length), context, { filename: 'stable.js' }); await flush(); }, async tick() { assert.equal(timers.size, 1); const [id, f] = timers.entries().next().value; timers.delete(id); f(); await flush(); }, async click(id) { await elements.get(id).dispatch('click'); await flush(); }, async save(slot = 1) { const select = descendants(elements.get('cards'), (e) => e.tagName === 'select')[0]; select.value = String(slot); await this.buttons('Save to slot')[0].dispatch('click'); await flush(); } };
}

test('explicit stable entry, exact approved asset and no mining on login', async () => {
  const h = harness(blank()); await h.start(); assert.deepEqual(h.calls.map((c) => c.method), ['Mode', 'CareerMode', 'ManifestSnapshot', 'StableSnapshot']); assert.equal(h.count('Start'), 0); assert.equal(h.elements.get('login-a').disabled, false);
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

// These selected-pair fixtures test UI transport/state only. They are not new
// proof/manifest/bout goldens or a claim that these edited IDs were admitted.
function previewHarness() {
  const a=item(), b=item(); b.assignment.prospect_id=hash(777); b.assignment.sequence=4;
  const h=harness(environment('zero256',[a,b])); let pending=null;
  const wrap=(j)=>({id:71,version:1,ok:true,job:clone(j)});
  h.handlers.BeginFightPreview=(view,aID,bID)=>{pending={version:1,job_id:hash(888),state:'queued',view:clone(h.state.manifest.view),store_id:h.state.stable.status.store_id,engine_version:1,engine_id:'slug-fighter-bout-candidate-v1',fighter_a_id:aID,fighter_b_id:bID,error:'',result:null};return wrap(pending);};
  h.handlers.FightPreviewJob=()=>wrap(pending);
  h.handlers.CancelFightPreview=()=>{if(pending)Object.assign(pending,{state:'cancelled',error:'preview_cancelled'});return wrap(pending);};
  h.select=async()=>{for(const b of h.buttons('Select for preview'))await b.dispatch('click');await flush();};
  return h;
}
test('preview selection is explicit, bounded and survives inventory navigation',async()=>{
  const h=previewHarness();await h.start();await h.select();assert.equal(h.count('BeginFightPreview'),0);assert.equal(h.elements.get('preview-selection').children.length,2);assert.equal(h.elements.get('preview-begin').disabled,false);
  await h.click('inventory');assert.equal(h.elements.get('preview-selection').children.length,2);assert.equal(h.count('BeginFightPreview'),0);assert.equal(h.count('Start'),0);
  await h.click('preview-begin');assert.equal(h.count('BeginFightPreview'),1);const call=h.calls.find(c=>c.method==='BeginFightPreview');assert.ok(call.args[1]<call.args[2]);assert.equal(h.peak,1);
  await h.click('preview-clear');assert.equal(h.elements.get('preview-selection').children.length,0);assert.equal(h.count('CancelFightPreview'),1);assert.equal(h.count('BeginFightPreview'),1);
});
test('changing selection during Begin cancels the late job without adopting it',async()=>{
  const h=previewHarness();await h.start();await h.select();const gate=deferred(),begin=h.handlers.BeginFightPreview;h.handlers.BeginFightPreview=async(...args)=>{await gate.promise;return begin(...args);};
  const work=h.click('preview-begin');await flush();await h.click('preview-clear');gate.resolve();await work;await flush();
  assert.equal(h.elements.get('preview-selection').children.length,0);assert.equal(h.elements.get('preview-replay').hidden,true);assert.equal(h.count('CancelFightPreview'),1);assert.equal(h.count('BeginFightPreview'),1);assert.equal(h.peak,1);
});
test('preview rejects mismatched class and duplicate selection before dispatch',async()=>{
  const a=item(),b=item();b.assignment.prospect_id=hash(777);b.assignment.sequence=4;b.manifest.physical.weight_lb=195;b.manifest.physical.weight_class='middle';const e=environment('zero256',[a,b]);const h=harness(e);await h.start();for(const b of h.buttons('Select for preview'))await b.dispatch('click');await flush();
  assert.equal(h.elements.get('preview-selection').children.length,1);assert.equal(h.elements.get('preview-begin').disabled,true);assert.match(h.elements.get('preview-message').textContent,/same weight class/);assert.equal(h.count('BeginFightPreview'),0);
  for(const b of h.buttons('Select for preview'))await b.dispatch('click');assert.equal(h.elements.get('preview-selection').children.length,1);
});
test('a later sign-in with unchanged StoreID discards preview selection and job',async()=>{
  const h=previewHarness();await h.start();await h.select();await h.click('preview-begin');h.state.manifest.view.view_id=hash(989);h.state.stable.view.view_id=hash(989);h.state.stable.last_job_id='';await h.tick();
  assert.equal(h.elements.get('preview-selection').children.length,0);assert.equal(h.elements.get('preview-replay').hidden,true);assert.equal(h.elements.get('preview-begin').disabled,true);
});
test('old terminal discovery cannot populate a newly selected pair automatically',async()=>{
  const h=previewHarness();await h.start();await h.select();await h.click('preview-begin');await h.click('preview-clear');await h.select();const before=h.count('FightPreviewJob');await h.tick();
  assert.equal(h.count('FightPreviewJob'),before);assert.equal(h.count('BeginFightPreview'),1);assert.equal(h.elements.get('preview-replay').hidden,true);
});

test('all55 frozen independent complete bout transcripts pass the text consumer',async()=>{
  const h=harness();await h.start();const corpus=JSON.parse(readFileSync(new URL('../../fighterbout/testdata/reference/bout_v1.json',import.meta.url),'utf8'));
  assert.equal(corpus.cases.length,55);const methods=new Set();
  for(const c of corpus.cases){h.context.boutFixture=JSON.parse(c.canonical_result);h.context.boutIDs=[c.input.a.prospect_id,c.input.b.prospect_id];assert.doesNotThrow(()=>vm.runInContext('boutValue(boutFixture,boutIDs)',h.context),c.name);methods.add(h.context.boutFixture.method);}
  assert.ok(methods.has('decision_draw'));assert.ok(methods.has('tko_corner'));assert.ok(methods.has('ko'));
});
test('bout text consumer rejects altered score, extra steps and missing final state',async()=>{
  const h=harness();await h.start();const corpus=JSON.parse(readFileSync(new URL('../../fighterbout/testdata/reference/bout_v1.json',import.meta.url),'utf8'));const c=corpus.cases[0];
  for(const change of [v=>v.scores[0].total_tenths++,v=>v.rounds[0].ticks.push(clone(v.rounds[0].ticks[0])),v=>v.final.pop(),v=>v.rounds[0].ticks[0].states.push(clone(v.rounds[0].ticks[0].states[0])),v=>v.winner=hash(4)]){
    const v=JSON.parse(c.canonical_result);change(v);h.context.boutFixture=v;h.context.boutIDs=[c.input.a.prospect_id,c.input.b.prospect_id];assert.throws(()=>vm.runInContext('boutValue(boutFixture,boutIDs)',h.context));
  }
});

function completedPreviewHarness(index=0){
  const corpus=JSON.parse(readFileSync(new URL('../../fighterbout/testdata/reference/preview_transport_v1.json',import.meta.url),'utf8')),c=corpus.cases[index],fixture=c.name.split('-')[0];
  const e=environment(fixture,[...c.response.job.result.pair.fighters].sort((a,b)=>a.assignment.sequence-b.assignment.sequence));e.manifest.view=clone(c.view);e.manifest.account.id=c.view.account_id;e.manifest.account.generation=c.view.generation;e.stable.view=clone(c.view);e.stable.status=clone(c.status);
  const h=harness(e);let accepted=false,done=false;
  h.handlers.PreviewSnapshot=()=>({id:72,version:1,ok:true,snapshot:accepted?{...clone(c.snapshot_response.snapshot),state:done?'ready':'busy',last_job_id:done?c.response.job.job_id:'',active_job_id:done?'':c.response.job.job_id}: {version:1,state:'idle',error:'',view:clone(c.view),store_id:c.status.store_id,engine_version:1,engine_id:'slug-fighter-bout-candidate-v1',active_job_id:'',queued_job_id:'',last_job_id:''}});
  h.handlers.BeginFightPreview=()=>{accepted=true;return {id:73,version:1,ok:true,job:{...clone(c.response.job),state:'queued',result:null}};};
  h.handlers.FightPreviewJob=()=>{done=true;return clone(c.response);};
  h.handlers.CancelFightPreview=()=>clone(c.response);
  h.handlers.FightPreviewPortrait=(_,job,id)=>{assert.equal(job,c.response.job.job_id);const p=c.portrait_evidence.find(p=>p.prospect_id===id),a=c.response.job.result.pair.fighters.find(f=>f.assignment.prospect_id===id).assignment;return {version:1,ok:true,portrait:{version:1,view:clone(c.view),store_id:c.status.store_id,job_id:job,prospect_id:id,result_digest:c.engine_result_digest,descriptor_id:a.descriptor_id,manifest_digest:a.manifest_digest,portrait_digest:p.portrait_digest,svg:p.svg}};};
  h.select=async()=>{for(const b of h.buttons('Select for preview'))await b.dispatch('click');await flush();};h.fixture=c;return h;
}
test('busy snapshot followed by completed job retains the newer independent preview',async()=>{
  for(let n=0;n<4;n++){const h=completedPreviewHarness(n);await h.start();await h.select();await h.click('preview-begin');assert.equal(h.elements.get('preview-replay').hidden,false,h.fixture.name);assert.match(h.elements.get('preview-message').textContent,/ready/i);assert.equal(h.count('FightPreviewJob'),1);assert.equal(h.count('FightPreviewPortrait'),2);assert.equal(h.elements.get('preview-fighters').children.length,2);assert.equal(h.peak,1);}
});
test('local replay and stepping issue no native request and respect dynamic reduced motion',async()=>{
  const h=completedPreviewHarness();await h.start();await h.select();await h.click('preview-begin');const calls=h.calls.length;
  await h.click('preview-next');assert.match(h.elements.get('preview-step').textContent,/Step 1 of/);await h.click('preview-previous');assert.match(h.elements.get('preview-step').textContent,/opening bell/);
  await h.click('preview-play');const play=[...h.timers].find(([id])=>h.timerDelays.get(id)===250);assert.ok(play);h.timers.delete(play[0]);play[1]();await flush();assert.match(h.elements.get('preview-step').textContent,/Step 1 of/);
  h.motion.matches=true;await h.motion.dispatch('change');assert.equal([...h.timers.keys()].filter(id=>h.timerDelays.get(id)===250).length,0);assert.equal(h.elements.get('preview-play').disabled,true);assert.match(h.elements.get('preview-motion').textContent,/Reduced motion/);
  await h.click('preview-result');assert.equal(h.elements.get('preview-outcome').hidden,false);assert.match(h.elements.get('preview-scores').textContent,/Whole-fight scorecard/);await h.click('preview-restart');assert.match(h.elements.get('preview-step').textContent,/opening bell/);assert.equal(h.calls.length,calls);
  await h.click('preview-clear');assert.equal(h.elements.get('preview-fighters').children.length,0);assert.equal(h.elements.get('preview-replay').hidden,true);
});

test('preview outcome and scorecard are explicitly unrecorded and cannot inherit result finality',async()=>{
  const h=completedPreviewHarness();await h.start();await h.select();await h.click('preview-begin');const calls=h.calls.length;
  // An unrelated retained publication is not an authority for this practice preview.
  vm.runInContext('publicationStatus={finalized_at_ms:1}',h.context);
  await h.click('preview-result');
  assert.match(h.elements.get('preview-state').textContent,/^Unrecorded preview/);
  assert.match(h.elements.get('preview-finish').textContent,/^Unrecorded preview:/);
  assert.match(h.elements.get('preview-scores').textContent,/^Unrecorded preview/);
  assert.doesNotMatch(h.elements.get('preview-finish').textContent,/Finalized result|Provisional result/);
  assert.equal(h.calls.length,calls);
});
test('portrait completion for an old selection is discarded before creating an image',async()=>{
  const h=completedPreviewHarness();await h.start();await h.select();const gate=deferred(),portrait=h.handlers.FightPreviewPortrait;h.handlers.FightPreviewPortrait=async(...args)=>{await gate.promise;return portrait(...args);};
  await h.click('preview-begin');assert.equal(h.count('FightPreviewPortrait'),1);await h.click('preview-clear');gate.resolve();await flush();assert.equal(h.elements.get('preview-fighters').children.length,0);assert.equal(h.elements.get('preview-replay').hidden,true);assert.equal(h.count('FightPreviewPortrait'),1);assert.equal(h.peak,1);
});
test('hidden or closed page stops local replay and does not advance it in the background',async()=>{
  const h=completedPreviewHarness();await h.start();await h.select();await h.click('preview-begin');await h.click('preview-play');h.document.hidden=true;await h.document.dispatch('visibilitychange');assert.equal(h.timers.size,0);const before=h.calls.length;await h.window.dispatch('pagehide');await flush();assert.equal(h.calls.length,before);assert.equal(h.elements.get('preview-fighters').children.length,0);
});

test('fresh portrait authorization refusal immediately clears replay and selection',async()=>{
  const h=completedPreviewHarness();await h.start();await h.select();const gate=deferred(),portrait=h.handlers.FightPreviewPortrait;let n=0;
  h.handlers.FightPreviewPortrait=async(...args)=>{if(++n===1)return portrait(...args);await gate.promise;return {version:1,ok:false,error:'stale_view'};};
  await h.click('preview-begin');assert.equal(h.elements.get('preview-replay').hidden,false);await h.click('preview-play');gate.resolve();await flush();
  assert.equal(h.elements.get('preview-replay').hidden,true);assert.equal(h.elements.get('preview-selection').children.length,0);assert.equal(h.elements.get('preview-fighters').children.length,0);assert.equal([...h.timers.keys()].filter(id=>h.timerDelays.get(id)===250).length,0);
});
test('typed helper failure clears replay immediately while busy retains it',async()=>{
  const h=completedPreviewHarness();await h.start();await h.select();await h.click('preview-begin');assert.equal(h.elements.get('preview-replay').hidden,false);
  h.handlers.PreviewSnapshot=()=>({id:74,version:1,ok:false,error:'busy'});await h.tick();assert.equal(h.elements.get('preview-replay').hidden,false);
  h.handlers.PreviewSnapshot=()=>({id:75,version:1,ok:false,error:'preview_unavailable'});await h.tick();assert.equal(h.elements.get('preview-replay').hidden,true);assert.equal(h.elements.get('preview-fighters').children.length,0);assert.equal(h.elements.get('preview-selection').children.length,2);assert.equal(h.elements.get('preview-begin').disabled,true);
});

// Modeled typed Career replies test UI correlation; native separately binds the
// activation digest/incarnation. Expected XP effects use the unchanged D42 rule.
function careerReply(e, wins = 1) {
  const original=clone(e.items[0]), asOf=original.condition.as_of_ms;
  const c=clone(original.condition); c.pro_xp=Math.min(15,wins); c.league_attributes_tenths.skill+=10*c.pro_xp; c.rating_hundredths+=80*c.pro_xp;
  const status=clone(e.stable.status); status.last_as_of_ms=asOf;
  return {id:900,job_id:"",ok:true,view:clone(e.manifest.view),store_id:status.store_id,page:{version:1,activation:{version:1,policy_id:'2bdf6089dc4bc641dc6e6e87dab21a15196e6b31708c9f84ab8dd728049e50d2',activation_id:hash(991),result_store_id:hash(992),result_created_at_ms:original.assignment.born_at_ms,source_created_at_ms:original.assignment.born_at_ms,activated_at_ms:original.assignment.born_at_ms},career_digest:hash(993),as_of_ms:asOf,source:{version:1,status,as_of_ms:asOf,after:0,next_after:1,has_more:false,items:[original]},items:[{prospect_id:original.assignment.prospect_id,totals:{wins,losses:0,draws:0,xp:c.pro_xp,base_points:10*wins},condition:c}]},roster:null,error:''};
}
test('career mode renders finalized totals and XP condition without changing the legacy source', async()=>{
  const e=environment("zero256",[item(reference.cases[2])]),h=harness(e),r=careerReply(e); h.handlers.CareerMode=()=>true;h.handlers.CareerInventory=(_,jobID)=>({...clone(r),job_id:jobID});await h.start();
  assert.equal(h.cards.length,1);assert.match(h.cards[0].textContent,/Finalized professional record: 1–0–0/);assert.match(h.cards[0].textContent,/professional XP: 1/);assert.match(h.cards[0].textContent,/Base-point history: 10 \(not a ranking\)/);
  assert.equal(h.buttons('Select for preview')[0].disabled,true);assert.equal(vm.runInContext('pageItems[0].condition.pro_xp',h.context),0);assert.equal(h.count('Start'),0);assert.equal(h.count('BeginStableInventory'),0);
});
test('career page malformed identity, clock, XP and incarnation refuse without replacing retained page',async()=>{
 const e=environment("zero256",[item(reference.cases[2])]),h=harness(e),r=careerReply(e);h.handlers.CareerMode=()=>true;h.handlers.CareerInventory=(_,jobID)=>({...clone(r),job_id:jobID});await h.start();const before=h.cards[0].textContent;
 for(const change of [v=>v.view.view_id=hash(999),v=>v.page.items[0].prospect_id=hash(999),v=>v.page.as_of_ms++,v=>v.page.items[0].totals.xp=2,v=>v.page.items[0].condition.league_attributes_tenths.skill++,v=>v.page.activation.activation_id=hash(998)]){const bad=clone(r);change(bad);h.handlers.CareerInventory=(_,jobID)=>({...bad,job_id:jobID});await h.click('inventory');assert.equal(h.cards[0].textContent,before);assert.match(h.elements.get('stable-progress').textContent+h.elements.get('announcement').textContent+h.elements.get('mutation-state').textContent+vm.runInContext('message',h.context),/validated/);}
});
test('late career response cannot disclose the prior login page',async()=>{
 const e=environment("zero256",[item(reference.cases[2])]),h=harness(e),r=careerReply(e),gate=deferred();h.handlers.CareerMode=()=>true;h.handlers.CareerInventory=(_,jobID)=>({...clone(r),job_id:jobID});await h.start();h.handlers.CareerInventory=async(_,jobID)=>{await gate.promise;return {...r,job_id:jobID}};const pending=h.click('inventory');await flush();h.state=blank();vm.runInContext('forgetView();current=stable=null;',h.context);gate.resolve();await pending;await flush();assert.equal(h.cards.length,0);
});
test('newly read finalized award clears selected zero-XP practice participant',async()=>{
 const e=environment("zero256",[item(reference.cases[2])]),h=harness(e);h.handlers.CareerMode=()=>true;h.handlers.CareerInventory=(_,jobID)=>({...careerReply(e,0),job_id:jobID});await h.start();vm.runInContext('selectPreview(pageItems[0])',h.context);assert.equal(vm.runInContext('previewSelected.length',h.context),1);
 h.handlers.CareerInventory=(_,jobID)=>({...careerReply(e,1),job_id:jobID});await h.click('inventory');assert.equal(vm.runInContext('previewSelected.length',h.context),0);assert.match(h.elements.get('preview-message').textContent,/zero-XP profile/);
});
test('career cache refusal rolls back activation, overlay and source page together',async()=>{
 const e=environment("zero256",[item(reference.cases[2])]),h=harness(e);h.handlers.CareerMode=()=>true;h.handlers.CareerInventory=(_,jobID)=>({...careerReply(e,0),job_id:jobID});await h.start();const before=vm.runInContext('JSON.stringify([pageItems,careerPage,careerActivation])',h.context);
 vm.runInContext('globalThis.capacityOriginal=previewCapacity;previewCapacity=()=>careerPage?.items[0]?.totals.xp===1?false:capacityOriginal();',h.context);h.handlers.CareerInventory=(_,jobID)=>({...careerReply(e,1),job_id:jobID});await h.click('inventory');assert.equal(vm.runInContext('JSON.stringify([pageItems,careerPage,careerActivation])',h.context),before);vm.runInContext('previewCapacity=capacityOriginal;',h.context);
});

test('career overlay requires the selected completed job and identical legacy source', async () => {
  const e=environment('zero256',[item(reference.cases[2])]),h=harness(e);
  h.handlers.CareerMode=()=>true;
  h.handlers.CareerInventory=(_,jobID)=>({...careerReply(e,0),job_id:jobID});
  await h.start();
  assert.equal(h.count('BeginStableRecent'),1);
  assert.equal(h.count('CareerInventory'),1);
  const before=vm.runInContext('JSON.stringify([pageItems,careerPage,careerActivation])',h.context);
  for (const mutate of [r=>r.job_id=hash(12345),r=>r.page.source.status.operation_count++]) {
    h.handlers.CareerInventory=(_,jobID)=>{const r={...careerReply(e,1),job_id:jobID};mutate(r);return r;};
    await h.click('inventory');
    assert.equal(vm.runInContext('JSON.stringify([pageItems,careerPage,careerActivation])',h.context),before);
    assert.match(vm.runInContext('message',h.context),/could not be validated/);
  }
});
test('same-view superseded career overlay cannot replace the captured page', async () => {
  const e=environment('zero256',[item(reference.cases[2])]),h=harness(e),gate=deferred();
  h.handlers.CareerMode=()=>true;
  h.handlers.CareerInventory=(_,jobID)=>({...careerReply(e,0),job_id:jobID});
  await h.start();
  const before=vm.runInContext('JSON.stringify([pageItems,careerPage,careerActivation])',h.context);
  h.handlers.CareerInventory=async(_,jobID)=>{await gate.promise;return {...careerReply(e,1),job_id:jobID};};
  const pending=h.click('inventory');await flush();
  vm.runInContext(`careerSelectedJob='${hash(12346)}'`,h.context);
  gate.resolve();await pending;await flush();
  assert.equal(vm.runInContext('JSON.stringify([pageItems,careerPage,careerActivation])',h.context),before);
});
