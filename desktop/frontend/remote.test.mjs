// Runs only the shipped page against a public DOM/protocol fixture. No native
// window, helper, wallet, network or mining acceptance is claimed.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { createHash } from 'node:crypto';
import test from 'node:test';
import vm from 'node:vm';
const source = readFileSync(new URL('./remote.js', import.meta.url), 'utf8');
const html = readFileSync(new URL('./remote.html', import.meta.url), 'utf8');
const raw = readFileSync(new URL('../pilot/testdata/remote-pool-protocol/reference-v1.json', import.meta.url));
assert.equal(createHash('sha256').update(raw).digest('hex'), '001430345f09c1fc8e1c924db18564250df6b5f32e21a2cddcfbc8d6be83a865');
const corpus = JSON.parse(raw);
const fixture = name => structuredClone(corpus.positives.find(v => v.name === `zero256-default-${name}`).response.result);
class Element {
  constructor(id) { this.id = id; this.disabled = true; this.hidden = false; this.textContent = ''; this.listeners = new Map(); }
  set innerHTML(_) { throw new Error('HTML interpolation forbidden'); }
  addEventListener(name, fn) { this.listeners.set(name, fn); }
  async click() { if (!this.disabled) await this.listeners.get('click')?.(); }
}
async function harness(initial = fixture('snapshot')) {
  const elements = new Map([...html.matchAll(/\bid="([^"]+)"/g)].map(v => [v[1], new Element(v[1])]));
  const calls = [], handlers = new Map(), timers = [];
  const document = { getElementById(id) { assert.ok(elements.has(id), id); return elements.get(id); } };
  const context = vm.createContext({ document, setTimeout(fn) { timers.push(fn); }, Call: { async ByName(name, ...args) {
    assert.ok(name.startsWith('main.RemoteGym.')); const method = name.slice('main.RemoteGym.'.length);
    assert.ok(['Snapshot', 'LoginFixture', 'Start', 'Stop', 'Logout'].includes(method));
    calls.push({ method, args: structuredClone(args) });
    if (handlers.has(method)) return handlers.get(method)(...args);
    if (method === 'Snapshot') return structuredClone(initial);
    throw new Error('unexpected native call');
  } } });
  const prefix = "import { Call } from '/wails/runtime.js';\n"; assert.ok(source.startsWith(prefix));
  vm.runInContext(source.slice(prefix.length) + '\nglobalThis.uiState=()=>({snapshot,busy,retired,configID,fixture});', context, { timeout: 1000 });
  for (let i = 0; i < 12; i++) await Promise.resolve();
  assert.equal(context.uiState().busy, false);
  calls.length = 0;
  return { $: id => elements.get(id), calls, handlers, timers, state: context.uiState };
}
test('configured identity only, existing upper layout and five methods', async () => {
  const h = await harness(); assert.equal(h.$('login-a').disabled, false); assert.equal(h.$('login-b').disabled, true); assert.equal(h.$('login-b').hidden, true);
  for (const removed of ['reset', 'export-recovery', 'transfer-prepare', 'entry-save', 'refresh-stable']) assert.equal(h.$(removed), undefined);
  assert.match(html, /class="masthead"/); assert.match(html, /class="mining-bar"/); assert.match(html, /href="\/stable.css"/);
  h.handlers.set('LoginFixture', () => fixture('login_fixture')); await h.$('login-a').click();
  assert.equal(h.calls[0].method, 'LoginFixture'); assert.deepEqual(h.calls[0].args, ['zero256']);
  const original = h.state().snapshot.view.view_id;
  h.handlers.set('Start', () => fixture('start')); await h.$('mining').click();
  assert.deepEqual(h.calls.at(-1), { method: 'Start', args: [original] }); assert.equal(h.$('mining').textContent, 'Stop mining');
  h.handlers.set('Stop', () => fixture('stop')); await h.$('mining').click();
  assert.deepEqual(h.calls.at(-1), { method: 'Stop', args: [original] });
  h.handlers.set('Logout', () => fixture('logout')); await h.$('logout').click();
  assert.deepEqual(h.calls.at(-1), { method: 'Logout', args: [original] }); assert.equal(h.$('payout').textContent, '—'); assert.equal(h.$('spendable').textContent, '—');
});
test('one pending action, unknown outcome stays visible with no automatic retry', async () => {
  const h = await harness(fixture('login_fixture'));
  let reject; h.handlers.set('Start', () => new Promise((_, no) => { reject = no; }));
  const pending = h.$('mining').click(); await Promise.resolve();
  assert.equal(h.$('mining').disabled, true); assert.equal(h.$('logout').disabled, true);
  await h.$('mining').click(); assert.equal(h.calls.length, 1);
  reject(new Error('The remote request outcome is unknown.')); await pending;
  assert.equal(h.state().retired, true); assert.match(h.$('alert').textContent, /outcome unknown/); assert.equal(h.$('payout').textContent, '—');
  for (const timer of h.timers.splice(0)) await timer();
  assert.equal(h.calls.length, 1); assert.match(h.$('alert').textContent, /outcome unknown/);
});
test('expiry and wrong config clear stale wallet and view controls', async () => {
  const h = await harness(fixture('snapshot-original'));
  h.handlers.set('Snapshot', () => fixture('expired')); await h.timers.shift()();
  assert.equal(h.$('mining').disabled, true); assert.equal(h.$('payout').textContent, '—'); assert.equal(h.$('gym-key').textContent, '—');
  const wrong = fixture('snapshot-original'); wrong.config_id = 'f'.repeat(64);
  h.handlers.set('Snapshot', () => wrong); await h.timers.shift()();
  assert.equal(h.state().snapshot, null); assert.equal(h.$('mining').disabled, true); assert.equal(h.$('spendable').textContent, '—');
});
test('mutation reply cannot replace its original view', async () => {
  const h = await harness(fixture('login_fixture'));
  const wrong = fixture('start'); wrong.view.view_id = 'e'.repeat(64);
  h.handlers.set('Start', () => wrong); await h.$('mining').click();
  assert.equal(h.state().snapshot, null); assert.equal(h.$('mining').disabled, true); assert.equal(h.calls.length, 1);
});
