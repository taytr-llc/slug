// Candidate UI behavioral tests. No browser, native window, wallet or mining.
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { setImmediate } from 'node:timers/promises';
import test from 'node:test';
import vm from 'node:vm';

const source = readFileSync(new URL('./manifest.js', import.meta.url), 'utf8');
const html = readFileSync(new URL('./manifest.html', import.meta.url), 'utf8');
const runtime = "import { Call } from '/wails/runtime.js';";
assert.ok(source.startsWith(runtime));
const reference = JSON.parse(readFileSync(new URL('../../fighterportrait/testdata/reference/portraits_v1.json', import.meta.url), 'utf8'));
const DESCRIPTOR = 'c1a8650a39f695814db67fddf79815ef249a13a5c0cc5af0b906b061c4c3a303';
const GENESIS = '7893676099cf9985bb198dfb6a347d9422a19f2422cf3326a999a7230ce246cd';
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
function item(sequence = 1, index = sequence - 1) {
  const v = reference.cases[index]; const m = v.manifest;
  return { sequence, id: hash(sequence + 100), work_hash: v.seed, born_at_ms: 1700000000000 + sequence, accepted_at_ms: 1700000000500 + sequence, name: clone(m.name), physical: clone(m.physical), style: m.style, rarity: m.dna.rarity, talent_budget: m.dna.talent_budget, attributes: clone(m.dna.attributes), descriptor_id: DESCRIPTOR, manifest_digest: v.manifest_digest, portrait_digest: v.svg_digest, eligibility_mode: 'current-parent' };
}
function blank() {
  return { version: 1, mode: 'local-manifest-pilot', state: 'stopped', attempts: 0, hashes_per_second: 0, height: 0, pending: 0, last_error: '', recovery_error: '', wallet_state: 'closed', wallet_height: 0, payout_address: '', gym_id: '', gym_public_key: '', spendable_atoms: 0, immature_atoms: 0, backup_state: 'idle', backup_phase: '', backup_error: '', account: { state: 'signed_out', fixture: '', id: '', generation: 0, hold_until_ms: 0, held: false, can_export: false, last_error: '' }, view: null, accepted_count: 0, projection: { state: 'idle', error: '', revision: 0, through_sequence: 0, has_older: false, complete_recent: false, items: [] } };
}
function snapshot(items = [item()], fixture = 'zero256') {
  const s = blank(); const b = fixture === 'incrementing256';
  Object.assign(s, { attempts: 12, hashes_per_second: 1.5, height: 61, wallet_state: 'ready', wallet_height: 61, payout_address: 'a'.repeat(34), gym_id: b ? '28452fc8a478d28f284dfe671cfc57e7cabeca08ea953aef371f5674d3f4c510' : 'a227587d837507dbcb5f9d781967339d816070fb7bd574931c690ba6937b88e5', gym_public_key: b ? '033de5e2e3d5ad06f616572fcb3d61f089ce1105ef486debf308027b3b663d6d92' : '03ec7488667fd544b4a1de8fb0ae03730c9a2dc346c652cbaa59a077dbeddca519', spendable_atoms: 100000000, immature_atoms: 2000000000 });
  s.account = { state: 'signed_in', fixture, id: hash(b ? 51 : 50), generation: 1, hold_until_ms: 0, held: false, can_export: true, last_error: '' };
  s.view = { version: 1, view_id: hash(b ? 61 : 60), account_authority_id: hash(70), account_id: s.account.id, generation: 1, authority_pin: { version: 5, network: 0x534c5547, genesis: GENESIS, authority_id: hash(b ? 81 : 80), descriptor_id: DESCRIPTOR }, authority_origin: 'http://127.0.0.1:32123', transport_trust_id: hash(90) };
  s.accepted_count = items[0]?.sequence ?? 0;
  s.projection = { state: 'ready', error: '', revision: 1, through_sequence: s.accepted_count, has_older: s.accepted_count > items.length, complete_recent: items.length === Math.min(8, s.accepted_count) && items.every((p, i) => p.sequence === s.accepted_count - i), items: clone(items) };
  return s;
}
function portrait(s, sequence) {
  const p = s.projection.items.find((p) => p.sequence === sequence); const v = reference.cases.find((v) => v.svg_digest === p.portrait_digest);
  return { version: 1, view: clone(s.view), sequence, id: p.id, manifest_digest: p.manifest_digest, portrait_digest: p.portrait_digest, svg: v.svg };
}
async function flush() { for (let i = 0; i < 12; i++) await setImmediate(); }
function harness(initial = snapshot(), mode = 'local-manifest-pilot') {
  const elements = new Map([...html.matchAll(/\bid="([^"]+)"/g)].map((m) => [m[1], new Element()]));
  const document = new Element(); document.hidden = false; document.getElementById = (id) => elements.get(id); document.createElement = (tag) => new Element(tag);
  const window = new Element(); const calls = []; const timers = new Map(); let counter = 0; let value = clone(initial); let concurrent = 0, peak = 0;
  const handlers = { Mode: () => mode, ManifestSnapshot: () => clone(value), ManifestPortrait: (seq) => portrait(value, seq), Start: () => { value.state = 'running'; }, Stop: () => { value.state = 'stopped'; }, LoginFixture: (name) => { value = snapshot([], name); }, Logout: () => { value = blank(); }, ResetFixtureAccount: () => { value = blank(); }, RefreshManifestProspects: () => {}, ExportManifestReceipt: () => '', ExportRecovery: () => '', VerifyBackup: () => '' };
  const Call = { async ByName(name, ...args) { assert.match(name, /^main\.Gym\./); const method = name.slice(9); assert.ok(Object.hasOwn(handlers, method), `unexpected API ${method}`); calls.push({ method, args }); concurrent++; peak = Math.max(peak, concurrent); try { return await handlers[method](...args); } finally { concurrent--; } } };
  const context = vm.createContext({ document, window, Call, Intl, Date, btoa: (s) => Buffer.from(s, 'ascii').toString('base64'), setTimeout(f, delay) { assert.equal(delay, 1000); const id = ++counter; timers.set(id, f); return id; }, clearTimeout(id) { timers.delete(id); } });
  return { elements, document, window, calls, timers, handlers, context, get value() { return value; }, set value(v) { value = clone(v); }, get peak() { return peak; }, count(method) { return calls.filter((c) => c.method === method).length; }, get cards() { return elements.get('cards').children; }, get imgs() { return descendants(elements.get('cards'), (e) => e.tagName === 'img'); }, buttons(name) { return descendants(elements.get('cards'), (e) => e.tagName === 'button' && e.textContent === name); }, async start() { vm.runInContext(source.slice(runtime.length), context, { filename: 'manifest.js' }); await flush(); }, async tick() { assert.equal(timers.size, 1); const [id, f] = timers.entries().next().value; timers.delete(id); f(); await flush(); }, async refresh() { await document.dispatch('visibilitychange'); await flush(); }, async click(id) { await elements.get(id).dispatch('click'); await flush(); } };
}

test('explicit mode, approved asset, isolated entry and no mining on launch', async () => {
  const h = harness(blank()); await h.start(); assert.deepEqual(h.calls.map((c) => c.method), ['Mode', 'ManifestSnapshot']); assert.equal(h.count('Start'), 0); assert.equal(h.elements.get('login-a').disabled, false);
  const wrong = harness(blank(), 'local-account-pilot'); await wrong.start(); assert.equal(wrong.count('ManifestSnapshot'), 0); assert.equal(wrong.timers.size, 0); assert.match(wrong.elements.get('alert').textContent, /correct mode/);
  assert.equal(createHash('sha256').update(readFileSync(new URL('./slug-transparent.png', import.meta.url))).digest('hex'), '50ceada462548c677fb6e68e454625ca359aced2954daad5fb13abb9db85f693');
  assert.match(html, /src="\/slug-transparent.png" alt="SLUG"/); assert.match(html, /src="\/manifest.js"/); assert.doesNotMatch(html, /app\.js|style\.css|https?:|<style|onclick=/); assert.doesNotMatch(source, /\.innerHTML\s*=|createElement\(['"]svg/);
});

test('automatic live cards and independent SVG bytes without Refresh or second proof work', async () => {
  const h = harness(snapshot([])); h.value.state = 'running'; await h.start();
  h.value = snapshot([item(1)]); h.value.state = 'running'; h.value.projection.revision = 2; await h.tick();
  assert.equal(h.cards.length, 1); assert.match(h.cards[0].textContent, new RegExp(reference.cases[0].manifest.name.display)); assert.equal(h.count('RefreshManifestProspects'), 0); assert.equal(h.count('Start'), 0); assert.equal(h.count('ManifestPortrait'), 1);
  assert.equal(Buffer.from(h.imgs[0].attributes.get('src').split(',')[1], 'base64').toString(), reference.cases[0].svg);
  const card = h.cards[0]; await h.tick(); assert.equal(h.cards[0], card); assert.equal(h.count('ManifestPortrait'), 1); assert.equal(h.peak, 1);
});

test('portrait pipeline is serial and queued user Stop takes priority over next image', async () => {
  const s = snapshot([item(2), item(1)]); s.state = 'running'; const h = harness(s); const gate = deferred(); const oldPortrait = h.handlers.ManifestPortrait;
  h.handlers.ManifestPortrait = (seq) => seq === 2 ? gate.promise : oldPortrait(seq); await h.start(); assert.equal(h.count('ManifestPortrait'), 1);
  const stopping = h.elements.get('mining').dispatch('click'); await flush(); assert.equal(h.count('Stop'), 0);
  gate.resolve(portrait(s, 2)); await stopping; await flush();
  const methods = h.calls.map((c) => c.method); assert.equal(methods[3], 'Stop'); assert.ok(methods.lastIndexOf('ManifestPortrait') > methods.indexOf('Stop')); assert.equal(h.peak, 1); assert.equal(h.value.state, 'stopped');
});

test('polls coalesce and never overlap portraits or other native calls', async () => {
  const h = harness(snapshot([])); await h.start(); const gate = deferred(); h.handlers.ManifestSnapshot = () => gate.promise;
  const tick = h.tick(); await flush(); await h.document.dispatch('visibilitychange'); await h.document.dispatch('visibilitychange'); await flush(); assert.equal(h.count('ManifestSnapshot'), 2); assert.equal(h.peak, 1);
  h.handlers.ManifestSnapshot = () => clone(h.value); gate.resolve(clone(h.value)); await tick; await flush(); assert.equal(h.peak, 1); assert.ok(h.count('ManifestSnapshot') <= 3);
});

test('logout invalidates delayed portrait before another fixture can appear', async () => {
  const a = snapshot(); const h = harness(a); const gate = deferred(); h.handlers.ManifestPortrait = () => gate.promise; await h.start();
  const logout = h.elements.get('logout').dispatch('click'); await flush(); assert.equal(h.cards.length, 0);
  gate.resolve(portrait(a, 1)); await logout; await flush(); assert.equal(h.imgs.length, 0);
  h.handlers.ManifestPortrait = (seq) => portrait(h.value, seq); await h.click('login-b'); assert.equal(h.value.account.fixture, 'incrementing256'); assert.equal(h.cards.length, 0); assert.equal(h.peak, 1);
});

test('late native dialog response is reconciled with current view before reporting success', async () => {
  const h = harness(); await h.start(); const gate = deferred(); h.handlers.ExportManifestReceipt = () => gate.promise;
  const exporting = h.buttons('Export public receipt')[0].dispatch('click'); await flush(); const polls = h.count('ManifestSnapshot'); await h.tick(); assert.equal(h.count('ManifestSnapshot'), polls);
  h.value = blank(); gate.resolve('Receipt saved.'); await exporting; await flush(); assert.equal(h.cards.length, 0); assert.doesNotMatch(h.elements.get('alert').textContent, /Receipt saved/); assert.equal(h.peak, 1);
  const calls = h.calls.map((c) => c.method); assert.equal(calls[calls.indexOf('ExportManifestReceipt') + 1], 'ManifestSnapshot');
});

test('cancelled export is quiet; held viewing/mining/public receipt remains available', async () => {
  const s = snapshot(); s.account.held = true; s.account.can_export = false; s.account.hold_until_ms = Date.now() + 86400000;
  const h = harness(s); await h.start(); assert.equal(h.elements.get('export-recovery').disabled, true); assert.equal(h.elements.get('mining').disabled, false); assert.equal(h.buttons('Export public receipt')[0].disabled, false);
  await h.buttons('Export public receipt')[0].dispatch('click'); await flush(); assert.doesNotMatch(h.elements.get('alert').textContent, /saved/i); assert.equal(h.count('ExportManifestReceipt'), 1); assert.equal(h.count('ExportRecovery'), 0);
});

test('strict new DTO rejects malformed and cross-mode projections without displaying raw data', async (t) => {
  const mutations = {
    'legacy shape': (s) => { s.prospects = []; }, 'wrong mode': (s) => { s.mode = 'local-account-pilot'; }, 'missing field': (s) => { delete s.projection; }, 'unsafe counter': (s) => { s.attempts = Number.MAX_SAFE_INTEGER + 1; }, 'NaN rate': (s) => { s.hashes_per_second = NaN; }, 'wrong fixture key': (s) => { s.gym_id = hash(5); }, 'unknown status error': (s) => { s.last_error = '/private/secret/location'; }, 'unknown account field': (s) => { s.account.token = 'secret'; }, 'null items': (s) => { s.projection.items = null; }, 'nine records': (s) => { s.projection.items = Array.from({ length: 9 }, (_, i) => item(9 - i)); }, 'duplicate IDs': (s) => { s.projection.items = [item(1), item(1)]; s.accepted_count = 2; }, 'noncanonical hash': (s) => { s.projection.items[0].id = 'A'.repeat(64); }, 'zero base attribute': (s) => { s.projection.items[0].attributes.power = 0; }, 'mismatched display name': (s) => { s.projection.items[0].name.display = '<script>'; }, 'foreign descriptor': (s) => { s.view.authority_pin.descriptor_id = hash(3); }, 'bad authority origin': (s) => { s.view.authority_origin = 'https://example.com'; }, 'null signed-in view': (s) => { s.view = null; }, 'signed-out disclosure': (s) => { s.account.state = 'expired'; }, 'false complete page': (s) => { s.accepted_count = 4; }, 'failed projection retains records': (s) => { s.projection.state = 'failed'; s.projection.error = 'projection_corrupt'; },
  };
  for (const [name, mutate] of Object.entries(mutations)) await t.test(name, async () => { const s = snapshot(); mutate(s); const h = harness(s); await h.start(); assert.equal(h.cards.length, 0); assert.equal(h.count('ManifestPortrait'), 0); assert.equal(h.elements.get('mining').disabled, true); assert.doesNotMatch(h.elements.get('alert').textContent, /secret|script|private\/secret/); });
});

test('same view cannot silently replace its pin or rewind/change a published revision', async (t) => {
  for (const [name, mutate] of Object.entries({ pin: (s) => { s.view.transport_trust_id = hash(33); }, rewind: (s) => { s.projection.revision = 0; }, record: (s) => { s.projection.items[0].name = { first: 'Same', last: 'Name', display: 'Same Name' }; } })) await t.test(name, async () => { const h = harness(); await h.start(); const s = clone(h.value); mutate(s); h.value = s; await h.tick(); assert.equal(h.cards.length, 0); assert.equal(h.elements.get('mining').disabled, true); });
});

test('eight-card cap, sequence gaps and duplicate names use distinct stable identities', async () => {
  const list = Array.from({ length: 8 }, (_, i) => item(12 - i, 0)); const h = harness(snapshot(list)); await h.start();
  assert.equal(h.cards.length, 8); assert.equal(new Set(h.cards).size, 8); assert.equal(h.imgs.filter((i) => i.attributes.has('src')).length, 8); assert.equal(h.count('ManifestPortrait'), 8); assert.equal(h.peak, 1); assert.match(h.elements.get('coverage').textContent, /latest 8 of 12/);
  const s = snapshot([item(4), item(2), item(1)]); s.projection.revision = 2; s.view.view_id = hash(600); h.value = s; await h.tick(); assert.equal(h.cards.length, 3); assert.match(h.elements.get('coverage').textContent, /still syncing/); assert.equal(h.count('RefreshManifestProspects'), 0);
});

test('wrong-view, arbitrary markup, digest mismatch and oversized SVG never become images', async (t) => {
  for (const [name, change] of Object.entries({ 'other view': (p) => { p.view.view_id = hash(999); }, 'script': (p) => { p.svg = p.svg.replace('</svg>', '<script>evil()</script></svg>'); }, 'remote asset': (p) => { p.svg = p.svg.replace('</svg>', '<image href="https://example.test/x"/></svg>'); }, 'digest mismatch': (p) => { p.portrait_digest = hash(999); }, 'oversize': (p) => { p.svg = 'a'.repeat(32769); }, 'extra field': (p) => { p.path = '/tmp/a'; } })) await t.test(name, async () => { const h = harness(); h.handlers.ManifestPortrait = (seq) => { const p = portrait(h.value, seq); change(p); return p; }; await h.start(); assert.equal(h.imgs[0].attributes.has('src'), false); assert.match(h.cards[0].textContent, /Portrait unavailable/); });
});

test('connection failure clears old cards and a stopped recovery warning does not block Start', async () => {
  const s = snapshot(); s.recovery_error = 'outcome_unknown'; const h = harness(s); await h.start(); assert.equal(h.elements.get('mining').disabled, false);
  h.handlers.ManifestSnapshot = () => { throw new Error('secret database path'); }; await h.tick(); assert.equal(h.cards.length, 0); assert.doesNotMatch(h.elements.get('alert').textContent, /secret|database/);
  h.handlers.ManifestSnapshot = () => clone(s); await h.tick(); assert.equal(h.cards.length, 1); assert.equal(h.elements.get('mining').disabled, false);
});

test('pagehide ignores late responses and clears image sources without new calls', async () => {
  const h = harness(); const gate = deferred(); h.handlers.ManifestPortrait = () => gate.promise; await h.start(); const old = portrait(h.value, 1); await h.window.dispatch('pagehide'); const count = h.calls.length; gate.resolve(old); await flush(); assert.equal(h.cards.length, 0); assert.equal(h.timers.size, 0); assert.equal(h.calls.length, count);
});

test('all 50 frozen independent portraits pass the bounded image-only consumer unchanged', async () => {
  for (let i = 0; i < reference.cases.length; i++) {
    const h = harness(snapshot([item(1, i)])); await h.start();
    assert.equal(h.imgs[0].hidden, false, reference.cases[i].name);
    assert.equal(Buffer.from(h.imgs[0].attributes.get('src').split(',')[1], 'base64').toString('ascii'), reference.cases[i].svg);
    await h.window.dispatch('pagehide');
  }
  assert.equal(reference.cases.length, 50);
});

test('cold loading is stoppable and failure retry stays explicit', async () => {
  const s = snapshot([]); s.projection.state = 'loading'; s.projection.complete_recent = false;
  const h = harness(s); await h.start(); assert.equal(h.count('RefreshManifestProspects'), 0); assert.equal(h.elements.get('refresh').disabled, true); assert.equal(h.elements.get('mining').textContent, 'Stop loading');
  await h.click('mining'); assert.equal(h.count('Stop'), 1); assert.equal(h.count('Start'), 0);
  s.projection.state = 'failed'; s.projection.error = 'projection_corrupt'; h.value = s; await h.tick();
  assert.equal(h.elements.get('refresh').disabled, false); assert.equal(h.count('RefreshManifestProspects'), 0);
  await h.click('refresh'); assert.equal(h.count('RefreshManifestProspects'), 1);
});

test('malformed dialog result cannot claim success or disclose raw errors', async () => {
  const h = harness(); await h.start(); h.handlers.ExportManifestReceipt = () => 'secret arbitrary output';
  await h.buttons('Export public receipt')[0].dispatch('click'); await flush();
  assert.match(h.elements.get('alert').textContent, /Could not save/); assert.doesNotMatch(h.elements.get('alert').textContent, /secret|Receipt saved/);
});

test('unchanged cards preserve expanded details across polls and evictions clear sources', async () => {
  const h = harness(snapshot([item(2), item(1)])); await h.start(); const card = h.cards[0]; const details = descendants(card, (e) => e.tagName === 'details')[0]; details.open = true; await h.tick(); assert.equal(h.cards[0], card); assert.equal(details.open, true);
  const oldImage = h.imgs[1]; const s = snapshot([item(3), item(2)]); s.projection.revision = 2; h.value = s; await h.tick(); assert.equal(oldImage.attributes.has('src'), false); assert.equal(h.cards[1], card);
});

test('Snapshot reply arriving after logout click cannot republish the old account', async () => {
  const h = harness(); await h.start(); const old = clone(h.value); const gate = deferred(); h.handlers.ManifestSnapshot = () => gate.promise;
  const pendingPoll = h.tick(); await flush(); const logout = h.elements.get('logout').dispatch('click'); await flush(); assert.equal(h.cards.length, 0);
  h.handlers.ManifestSnapshot = () => clone(h.value); gate.resolve(old); await pendingPoll; await logout; await flush(); assert.equal(h.cards.length, 0); assert.equal(h.elements.get('gym-id').textContent, '—'); assert.equal(h.peak, 1);
});


test('initial automatic preparation exposes Stop without an authenticated view', async () => {
  for (const state of ['signing_in', 'resetting']) {
    const s = blank(); s.account.state = state; s.account.fixture = 'zero256';
    const h = harness(s); h.handlers.Stop = () => { h.value = blank(); };
    await h.start(); assert.equal(h.cards.length, 0); assert.equal(h.count('Start'), 0);
    assert.equal(h.elements.get('mining').disabled, false); assert.equal(h.elements.get('mining').textContent, 'Stop preparing');
    assert.equal(h.elements.get('login-a').disabled, true); assert.equal(h.elements.get('logout').disabled, true);
    assert.equal(h.elements.get('export-recovery').disabled, true); assert.equal(h.elements.get('refresh').disabled, true);
    await h.click('mining'); assert.equal(h.count('Stop'), 1); assert.equal(h.count('Start'), 0);
    assert.equal(h.elements.get('login-a').disabled, false);
  }
});
