// DOM-only entry tests. Frozen independent fixtures plus labelled transport state stubs.
// No browser rendering, real authority, proof execution, node or wallet writes.
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

const entryReference = JSON.parse(readFileSync(new URL('../../gymentry/testdata/reference/entry_v1.json', import.meta.url), 'utf8'));
const sharedReference = JSON.parse(readFileSync(new URL('../../gymstable/testdata/shared-reference/shared_v1.json', import.meta.url), 'utf8'));
// Pool cases adapt transport state using independent Pin2 inputs; they do not
// claim new mined records or new cryptographic entry/store goldens.
function entryHarness(fixture = 'zero256', five = false, pool = false) {
  const scenario = entryReference.scenarios.find((s) => s.name === `${fixture}/${five ? 'synthetic-five-class' : 'save-repeat-withdraw-close-historical'}`);
  const receipt = scenario.final_operations.find((o) => o.state === 'committed').receipt;
  const source = scenario.initial_source;
  const list = receipt.members.map((m) => ({ assignment: clone(m.assignment), manifest: clone(source.find((s) => s.assignment.prospect_id === m.assignment.prospect_id).manifest), condition: clone(m.condition_at_save) })).sort((a, b) => a.assignment.sequence - b.assignment.sequence);
  const e = environment(fixture, list), pin = entryReference.pins.find((p) => p.fixture === fixture), view = clone(entryReference.protocol_cases.find((p) => p.name === `${fixture}/nested-negative-jitter`).current_view);
  e.manifest.view = clone(view); e.stable.view = clone(view); e.stable.status.slots = Array(10).fill(''); for (const s of source) e.stable.status.slots[s.slot - 1] = s.assignment.prospect_id; e.stable.status.operation_count = e.stable.status.roster_revision = source.length; e.stable.status.last_operation_id = hash(700); e.stable.status.checkpoint = e.stable.status.observed_source_count = e.manifest.accepted_count = source.length;
  if (pool) {
    const shared = sharedReference.pins.find((p) => p.fixture === fixture), m = e.manifest;
    view.authority_pin = clone(shared.pin.source); m.view = clone(view); e.stable.view = clone(view);
    e.stable.status.pin = clone(shared.pin); e.stable.status.store_id = shared.id;
    delete m.accepted_count; delete m.projection;
    Object.assign(m, { mode: 'local-pool-pilot', assignment_count: source.length, source_binding_id: hash(1234), payout_address: fixture === 'zero256' ? 'slug1qzmtrqsfuaf6l6kkcsseumq26ukaphfj9s9ptme' : 'slug1qslk39wvggqa0vl8nd6jckaz54dw3vk45c8educ', backup_state: 'idle', backup_phase: '', backup_error: '' });
    m.balance = { state: 'ready', error: '', value: { version: 1, binding_id: m.source_binding_id, observed_at_ms: e.stable.status.last_as_of_ms, balance: { gym_id: m.gym_id, confirmed: { numerator: '0', denominator: '1' }, available: { numerator: '0', denominator: '1' }, reserved_atoms: 0, paid_atoms: 0, reason: '' } } };
  }
  const h = harness(e, pool ? 'local-pool-pilot' : 'local-manifest-pilot');
  if (pool) { h.handlers.PoolSnapshot = () => clone(h.state.manifest); h.handlers.RefreshPoolBalance = () => {}; }
  let counter = 800, frame = 900, pending = '', latestJob = '', now = receipt.committed_at_ms, revision = 0, latest = null, closed = false;
  const ops = new Map(), entryJobs = new Map(); const events = new Map(entryReference.events.filter((x) => x.fixture === fixture).map((x) => [x.event.saturday, clone(x)]));
  const getStatus = (saturday = receipt.event.saturday) => { const ev = events.get(saturday); assert.ok(ev, saturday); const r = latest?.event.saturday === saturday ? latest : null; const isPending = pending && ops.get(pending).intent.request.saturday === saturday; return { version: 1, store_id: pin.store_id, event: clone(ev.event), exists: !!r || !!isPending, phase: now >= ev.event.cutoff_ms ? 'closed_pending_lock' : 'accepting', entry_id: ev.entry_id, revision: r?.revision ?? 0, latest_operation_id: r?.operation_id ?? '', latest_receipt_digest: r ? hash(990) : '', saved_ids: r?.members.map((m) => m.assignment.prospect_id) ?? [], saved_at_ms: r?.committed_at_ms ?? 0, saved_roster_revision: r?.roster_revision ?? 0, current_roster_revision: h.state.stable.status.roster_revision, roster_changed: !!(r?.members.length && r.roster_revision !== h.state.stable.status.roster_revision), eligibility: 'not_checked', pending_operation_id: isPending ? pending : '', observed_at_ms: now, closed_observed_at_ms: (r || isPending) && now >= ev.event.cutoff_ms ? now : 0 }; };
  const reply = (field, value) => { const r = { id: ++frame, ok: true, view: clone(h.state.manifest.view), store_id: pin.store_id, snapshot: null, status: null, history: null, job: null, operation: null, error: '' }; r[field] = clone(value); return r; };
  const refusal = (error) => ({ id: ++frame, ok: false, view: null, store_id: '', snapshot: null, status: null, history: null, job: null, operation: null, error });
  const publish = (kind, request, opID, payload) => { const jid = hash(++counter), j = { version: 1, job_id: jid, kind, state: 'complete', view: clone(h.state.manifest.view), store_id: pin.store_id, event_id: request.event_id, saturday: request.saturday, operation_id: opID, error: '', intent: null, mutation: null, check: null }; j[{ prepare: 'intent', apply: 'mutation', check: 'check' }[kind]] = clone(payload); entryJobs.set(jid, j); latestJob = jid; const ack = clone(j); Object.assign(ack, { state: 'queued', intent: null, mutation: null, check: null }); if (kind === 'prepare') ack.operation_id = ''; return reply('job', ack); };
  Object.assign(h.handlers, {
    EntrySnapshot: () => reply('snapshot', { version: 1, state: 'idle', error: '', view: clone(h.state.manifest.view), store_id: pin.store_id, pending_operation_id: pending, active_job_id: '', queued_job_id: '', last_job_id: latestJob }),
    EntryUpcoming: () => reply('status', getStatus()), EntryStatus: (_, saturday) => reply('status', getStatus(saturday)),
    EntryPending: () => reply('operation', { version: 1, store_id: pin.store_id, operation: pending ? ops.get(pending) : null }),
    EntryOperation: (_, id) => ops.has(id) ? reply('operation', { version: 1, store_id: pin.store_id, operation: ops.get(id) }) : refusal('entry_not_found'),
    EntryJob: (_, id) => entryJobs.has(id) ? reply('job', entryJobs.get(id)) : refusal('entry_not_found'),
    BeginEntryPrepare: (_, q) => { if (pending) return refusal('entry_pending'); if (closed) return refusal('entry_closed'); const id = hash(++counter), i = { version: 1, sequence: ops.size + 1, operation_id: id, generation: h.state.manifest.view.generation, view_id: h.state.manifest.view.view_id, request: clone(q), prepared_at_ms: now }; const o = { version: 1, intent: i, state: 'prepared', receipt: null, abandoned_at_ms: 0 }; ops.set(id, o); pending = id; return publish('prepare', q, id, i); },
    BeginEntryApply: (_, id) => { const o = ops.get(id), q = o.intent.request; if (closed) return refusal('entry_closed'); assert.equal(o.intent.view_id, h.state.manifest.view.view_id); const previous = revision++; const r = { ...clone(receipt), event: clone(events.get(q.saturday).event), entry_id: events.get(q.saturday).entry_id, operation_id: id, operation_sequence: o.intent.sequence, generation: o.intent.generation, view_id: o.intent.view_id, action: q.action, previous_revision: previous, revision, roster_revision: q.expected_roster_revision, committed_at_ms: now, members: q.action === 'withdraw' ? [] : receipt.members.filter((m) => q.prospect_ids.includes(m.assignment.prospect_id)) }; Object.assign(o, { state: 'committed', receipt: r }); latest = r; pending = ''; return publish('apply', q, id, { version: 1, receipt: r, replayed: false }); },
    BeginEntryCheck: (_, saturday) => { const st = getStatus(saturday), check = { version: 1, status: st, as_of_ms: now, roster_revision: h.state.stable.status.roster_revision, members: st.saved_ids.map((id) => { const i = list.find((i) => i.assignment.prospect_id === id), slot = h.state.stable.status.slots.indexOf(id) + 1; return { slot, item: i, eligibility: slot ? 'eligible' : 'not_rostered' }; }) }; return publish('check', { event_id: st.event.event_id, saturday }, '', check); },
    CancelEntryJob: (_, id) => { const j = entryJobs.get(id); if (j.state !== 'complete') Object.assign(j, { state: 'cancelled', error: 'entry_cancelled', intent: null, mutation: null, check: null }); return reply('job', j); },
    AbandonEntryOperation: (_, id) => { const o = ops.get(id); if (o.state === 'prepared') { Object.assign(o, { state: 'abandoned', abandoned_at_ms: now }); pending = ''; } return reply('operation', { version: 1, store_id: pin.store_id, operation: o }); },
    EntryHistory: (_, after) => reply('history', { version: 1, store_id: pin.store_id, after_event_id: after, next_event_id: '', has_more: false, observed_at_ms: now, events: latest ? [getStatus(latest.event.saturday)] : [] }),
  });
  return Object.assign(h, { pin, source, list, ops, entryJobs, reply, refusal, getStatus, get latest() { return latest; }, setClosed(value = true) { closed = value; if (value) now = receipt.event.cutoff_ms; }, async select(n = list.length) { for (let i = 0; i < n; i++) { const b = this.buttons('Add to Saturday draft').find((b) => !b.disabled); assert.ok(b); await b.dispatch('click'); await flush(); } }, async date(value) { this.elements.get('entry-date').value = value; await this.elements.get('entry-date').dispatch('change'); await flush(); } });
}

test('same app obtains authority Upcoming and makes no entry or mining during attach', async () => {
  const h = entryHarness(); await h.start(); assert.equal(h.elements.get('entry-date').value, '2026-11-21'); assert.match(h.elements.get('entry-cutoff').textContent, /Noon Pacific.*2026-11-21T20:00:00.000Z/); assert.equal(h.count('BeginEntryPrepare'), 0); assert.equal(h.count('Start'), 0); assert.equal(h.peak, 1); assert.match(html, /Saturday entry/); assert.doesNotMatch(html, /onclick=|<script[^>]*src="https:/);
});
test('explicit save acknowledges exact Prepare then Apply; repeated save is a new revision', async () => {
  const h = entryHarness(); await h.start(); await h.select(); await h.click('entry-save'); assert.equal(h.count('BeginEntryPrepare'), 1); assert.equal(h.count('BeginEntryApply'), 1); assert.equal(h.ops.size, 1); assert.equal([...h.ops.values()][0].state, 'committed'); assert.match(h.elements.get('entry-saved').textContent, /revision 1/); assert.match(h.elements.get('entry-save').textContent, /Replace/); await h.click('entry-save'); assert.equal(h.count('BeginEntryPrepare'), 2); assert.match(h.elements.get('entry-saved').textContent, /revision 2/); assert.equal(h.peak, 1);
});
test('five distinct class selection is bounded and duplicate class is refused', async () => {
  const h = entryHarness('zero256', true); await h.start(); await h.select(5); assert.equal(h.elements.get('entry-draft').children.length, 5); assert.match(h.elements.get('entry-count').textContent, /5 \/ 5/); assert.equal(h.buttons('Add to Saturday draft').filter((b) => !b.disabled).length, 0); await h.click('entry-save'); assert.equal([...h.ops.values()][0].receipt.members.length, 5);
  const g = entryHarness(); await g.start(); await g.select(1); const first = g.list.find((i) => i.assignment.prospect_id === vm.runInContext('entryDraft[0].id', g.context)), other = clone(first); other.assignment.prospect_id = hash(111); g.state.stable.status.slots[9] = other.assignment.prospect_id; g.context.extra = other; vm.runInContext('stable.status.slots[9] = extra.assignment.prospect_id; selectEntry(extra)', g.context); assert.match(g.elements.get('entry-message').textContent, /at most one/); assert.equal(g.elements.get('entry-draft').children.length, 1);
});
test('date change while Prepare result is in flight never automatically applies old choice', async () => {
  const h = entryHarness(); await h.start(); await h.select(); const gate = deferred(), orig = h.handlers.EntryJob; h.handlers.EntryJob = (...args) => gate.promise; await h.click('entry-save'); assert.equal(h.count('BeginEntryApply'), 0); const id = h.calls.find((c) => c.method === 'EntryJob').args[1]; const old = orig(h.state.manifest.view.view_id, id); await h.date('2028-02-26'); gate.resolve(old); await flush(); assert.equal(h.count('BeginEntryApply'), 0); assert.equal([...h.ops.values()][0].state, 'prepared'); assert.match(h.elements.get('entry-pending').textContent, /2026-11-21/); assert.equal(h.elements.get('entry-apply').disabled, true);
});
test('draft change during Prepare prevents automatic Apply even for same date/view', async () => {
  const h = entryHarness(); await h.start(); await h.select(); const gate = deferred(), orig = h.handlers.EntryJob; h.handlers.EntryJob = (...args) => gate.promise; await h.click('entry-save'); const id = h.calls.find((c) => c.method === 'EntryJob').args[1]; await h.click('entry-clear'); gate.resolve(orig(h.state.manifest.view.view_id, id)); await flush(); assert.equal(h.count('BeginEntryApply'), 0); assert.equal(h.elements.get('entry-draft').children.length, 0); assert.equal([...h.ops.values()][0].state, 'prepared');
});
test('lost Prepare ACK queries Pending and requires a separate explicit Apply', async () => {
  const h = entryHarness(); await h.start(); await h.select(); const orig = h.handlers.BeginEntryPrepare; h.handlers.BeginEntryPrepare = (...args) => { orig(...args); return h.refusal('outcome_unknown'); }; await h.click('entry-save'); assert.equal(h.count('BeginEntryApply'), 0); assert.equal(h.count('EntryPending'), 1); assert.equal(h.elements.get('entry-apply').disabled, false); await h.click('entry-apply'); assert.equal(h.count('BeginEntryApply'), 1); assert.equal(h.count('BeginEntryPrepare'), 1);
});
test('lost Apply ACK after cutoff reconciles original receipt without retrying mutation', async () => {
  const h = entryHarness(); await h.start(); await h.select(); const orig = h.handlers.BeginEntryApply; h.handlers.BeginEntryApply = (...args) => { orig(...args); h.setClosed(); throw new Error('private failure text'); }; await h.click('entry-save'); assert.equal(h.count('BeginEntryApply'), 1); assert.equal(h.count('EntryOperation'), 1); assert.equal(h.ops.size, 1); assert.match(h.elements.get('entry-message').textContent, /confirmed/); assert.match(h.elements.get('entry-warning').textContent, /intake is closed/); assert.doesNotMatch(h.elements.get('entry-message').textContent, /private failure/); assert.equal(h.elements.get('entry-save').disabled, true);
});
test('held current session can enter and withdraw while recovery export stays denied', async () => {
  const h = entryHarness('incrementing256'); h.state.manifest.account.held = true; h.state.manifest.account.can_export = false; h.state.manifest.account.hold_until_ms = h.state.stable.status.last_as_of_ms + 86400000; await h.start(); await h.select(); await h.click('entry-save'); assert.equal(h.count('BeginEntryApply'), 1); assert.equal(h.elements.get('export-recovery').disabled, true); await h.click('entry-withdraw'); assert.equal(h.count('BeginEntryApply'), 2); assert.deepEqual([...h.ops.values()].at(-1).receipt.members, []); assert.equal(h.count('ExportRecovery'), 0);
});
test('new same-generation sign-in can query and abandon an old intent but cannot Apply it', async () => {
  const h = entryHarness(); await h.start(); await h.select(); const orig = h.handlers.BeginEntryPrepare; h.handlers.BeginEntryPrepare = (...args) => { orig(...args); return h.refusal('outcome_unknown'); }; await h.click('entry-save'); h.state.manifest.view.view_id = hash(444); h.state.stable.view.view_id = hash(444); h.state.stable.last_job_id = ''; await h.tick(); assert.equal(h.elements.get('entry-apply').disabled, true); assert.match(h.elements.get('entry-pending').textContent, /earlier sign-in/); await h.click('entry-abandon'); assert.equal([...h.ops.values()][0].state, 'abandoned'); assert.equal(h.count('BeginEntryApply'), 0);
});
test('logout invalidates in-flight Prepare before automatic Apply and clears draft/history', async () => {
  const h = entryHarness(); await h.start(); await h.select(); const gate = deferred(), orig = h.handlers.EntryJob; h.handlers.EntryJob = (...args) => gate.promise; await h.click('entry-save'); const id = h.calls.find((c) => c.method === 'EntryJob').args[1], old = orig(h.state.manifest.view.view_id, id); const logout = h.elements.get('logout').dispatch('click'); await flush(); assert.equal(h.elements.get('entry-draft').children.length, 0); gate.resolve(old); await logout; await flush(); assert.equal(h.count('BeginEntryApply'), 0); assert.equal(h.elements.get('entry-save').disabled, true);
});
test('Check and history do not mutate saved choice; navigation keeps fixed inventory', async () => {
  const h = entryHarness(); await h.start(); await h.select(); await h.click('entry-save'); const before = JSON.stringify([...h.ops.values()]); await h.click('entry-check'); assert.match(h.elements.get('entry-eligibility').textContent, /Checked.*Eligible/); await h.click('entry-history'); assert.equal(h.elements.get('entry-history-list').children.length, 1); assert.equal(JSON.stringify([...h.ops.values()]), before); await h.click('inventory'); const card = h.cards[0]; await h.click('entry-status'); assert.equal(h.cards[0], card); assert.equal(h.count('Start'), 0);
});
test('strict completed correlation rejects changed operation, author, and terminal event', async (t) => {
  for (const [name, change] of Object.entries({ author: (j) => { j.intent.view_id = hash(666); }, selection: (j) => { j.intent.request.prospect_ids = [hash(777)]; }, event: (j) => { j.event_id = hash(888); } })) await t.test(name, async () => { const h = entryHarness(); await h.start(); await h.select(); const orig = h.handlers.EntryJob; h.handlers.EntryJob = (...args) => { const r = orig(...args); if (r.job.kind === 'prepare') change(r.job); return r; }; await h.click('entry-save'); assert.equal(h.count('BeginEntryApply'), 0); assert.match(h.elements.get('entry-message').textContent, /may have been recorded/); assert.equal(h.elements.get('entry-save').disabled, true); });
});
test('native modal and hidden page pause entry polls; old route failure is contained', async () => {
  const h = entryHarness(); await h.start(); const gate = deferred(); h.handlers.ExportManifestReceipt = () => gate.promise; const exporting = h.buttons('Export public receipt')[0].dispatch('click'); await flush(); const polls = h.count('EntrySnapshot'); await h.tick(); assert.equal(h.count('EntrySnapshot'), polls); gate.resolve('Receipt saved.'); await exporting; await flush(); h.document.hidden = true; await h.document.dispatch('visibilitychange'); assert.equal(h.timers.size, 0);
  const old = harness(); await old.start(); assert.equal(old.cards.length, 1); assert.equal(old.elements.get('entry-save').disabled, true); assert.match(old.elements.get('entry-state').textContent, /Unavailable/); assert.equal(old.peak, 1);
});
test('entry reservation participates in shared cache and a refusal does not dispatch', async () => {
  const h = entryHarness(); await h.start(); await h.select(); h.context.large = 'x'.repeat(500000); vm.runInContext("previewImages.set('public-cache-stub', large)", h.context); await h.click('entry-save'); assert.equal(h.count('BeginEntryPrepare'), 0); assert.match(h.elements.get('entry-message').textContent, /capacity/); assert.equal(vm.runInContext('entryWatch === null', h.context), true);
});
test('independent complete five-member and negative-jitter envelopes validate without new expectations', async () => {
  const h = entryHarness(); await h.start(); let tested = 0;
  for (const c of entryReference.protocol_cases.filter((c) => c.name.startsWith('zero256/') && c.accepted)) { if (c.response.job?.kind !== 'check' || c.response.job.state !== 'complete') continue; h.context.input = clone(c.response); assert.doesNotThrow(() => vm.runInContext("entryEnvelope(input, 'job')", h.context)); tested++; }
  assert.equal(tested, 1); assert.ok(entryReference.protocol_cases.find((c) => c.name === 'zero256/nested-negative-jitter').response.job.check.members.some((m) => m.item.manifest.portrait.shoulder_jitter < 0));
});
test('Stop disables automatic Apply even when its saved Prepare completes before cancellation', async () => {
  const h = entryHarness(); await h.start(); await h.select(); const original = h.handlers.EntryJob; let running = true;
  h.handlers.EntryJob = (...args) => { const r = original(...args); if (running) Object.assign(r.job, { state: 'running', operation_id: '', intent: null }); return r; };
  await h.click('entry-save'); assert.equal(h.count('BeginEntryApply'), 0); h.state.manifest.state = 'running'; const stop = h.handlers.Stop; h.handlers.Stop = () => { running = false; stop(); }; await h.tick(); await h.click('mining'); assert.equal(h.count('Stop'), 1); assert.equal(h.count('BeginEntryApply'), 0); assert.equal([...h.ops.values()][0].state, 'prepared');
});
test('entry typed authorization refusal clears saved presentation and old correlation', async () => {
  const h = entryHarness(); await h.start(); await h.select(); await h.click('entry-save'); await h.click('entry-history'); h.handlers.EntryStatus = () => h.refusal('stale_view'); await h.click('entry-status'); assert.equal(h.elements.get('entry-draft').children.length, 0); assert.equal(h.elements.get('entry-history-list').children.length, 0); assert.equal(h.elements.get('entry-save').disabled, true); assert.equal(vm.runInContext('entryStore', h.context), '');
});
test('eligibility check terminal is not shown on a changed selected Saturday', async () => {
  const h = entryHarness(); await h.start(); await h.select(); await h.click('entry-save'); const gate = deferred(), orig = h.handlers.EntryJob; h.handlers.EntryJob = (...args) => { const r = orig(...args); return r.job.kind === 'check' ? gate.promise : r; }; await h.click('entry-check'); const id = h.calls.filter((c) => c.method === 'EntryJob').at(-1).args[1]; await h.date('2028-02-26'); gate.resolve(orig(h.state.manifest.view.view_id, id)); await flush(); assert.equal(h.elements.get('entry-eligibility').children.length, 0); assert.equal(h.count('BeginEntryPrepare'), 1);
});
test('Pending cannot smuggle a committed historical row, while Abandon may return committed history', async () => {
  const h = entryHarness(); await h.start(); await h.select(); await h.click('entry-save'); const committed = clone([...h.ops.values()][0]); h.handlers.EntryPending = () => h.reply('operation', { version: 1, store_id: h.pin.store_id, operation: committed }); vm.runInContext("entryOperation = entryFlow = null; entryUnknown = true; entrySnapshot.pending_operation_id = ''; render()", h.context); await h.click('entry-reconcile'); assert.equal(vm.runInContext('entryOperation', h.context), null); assert.equal(vm.runInContext('entryUnknown', h.context), true); assert.doesNotMatch(h.elements.get('entry-message').textContent, /confirmed/);
  const g = entryHarness(); await g.start(); await g.select(); const original = g.handlers.BeginEntryPrepare; g.handlers.BeginEntryPrepare = (...args) => { original(...args); return g.refusal('outcome_unknown'); }; await g.click('entry-save'); const id = [...g.ops.keys()][0]; g.handlers.BeginEntryApply(g.state.manifest.view.view_id, id); await g.click('entry-abandon'); assert.equal(g.ops.get(id).state, 'committed'); assert.match(g.elements.get('entry-message').textContent, /already committed/); assert.equal(g.count('BeginEntryApply'), 0);
});


test('pool fixtures enter and withdraw through original-view entry controls without legacy mining reads', async () => {
  for (const fixture of ['zero256', 'incrementing256']) {
    const h = entryHarness(fixture, false, true); await h.start(); await h.select(); await h.click('entry-save');
    assert.equal(h.count('BeginEntryPrepare'), 1); assert.equal(h.count('BeginEntryApply'), 1);
    const saved = clone([...h.ops.values()][0]); assert.equal(saved.state, 'committed');
    assert.deepEqual(saved.receipt.members.map((m) => [m.assignment.prospect_id, m.assignment.sequence]).sort(), h.list.map((i) => [i.assignment.prospect_id, i.assignment.sequence]).sort());
    assert.equal(saved.receipt.view_id, h.state.manifest.view.view_id);
    await h.click('entry-withdraw'); assert.equal(h.count('BeginEntryApply'), 2);
    assert.deepEqual([...h.ops.values()].at(-1).receipt.members, []);
    for (const method of ['Start', 'ManifestSnapshot', 'PreviewSnapshot', 'CardSnapshot']) assert.equal(h.count(method), 0);
    assert.equal(h.peak, 1);
  }
});
test('held pool session can save entry while recovery export remains denied', async () => {
  const h = entryHarness('incrementing256', false, true);
  Object.assign(h.state.manifest.account, { held: true, can_export: false, hold_until_ms: h.state.stable.status.last_as_of_ms + 86400000 });
  await h.start(); await h.select(); await h.click('entry-save');
  assert.equal(h.count('BeginEntryApply'), 1); assert.equal(h.elements.get('export-recovery').disabled, true);
  assert.equal(h.count('ExportRecovery'), 0);
});
test('pool backup pauses new entry work and preserves the local draft', async () => {
  const h = entryHarness('zero256', false, true); await h.start(); await h.select();
  const draft = vm.runInContext('JSON.stringify(entryDraft)', h.context);
  Object.assign(h.state.manifest, { backup_state: 'running', backup_phase: 'restoring' }); await h.tick();
  assert.equal(h.elements.get('entry-save').disabled, true); assert.equal(h.elements.get('entry-check').disabled, true);
  await h.click('entry-save'); assert.equal(h.count('BeginEntryPrepare'), 0);
  assert.equal(vm.runInContext('JSON.stringify(entryDraft)', h.context), draft);
  Object.assign(h.state.manifest, { backup_state: 'succeeded', backup_phase: 'complete' }); await h.tick();
  await h.click('entry-save'); assert.equal(h.count('BeginEntryApply'), 1); assert.equal(h.count('Start'), 0);
});
