// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license that can be found in
// the LICENSE file.

// These tests execute the shipped frontend against a minimal DOM and the public
// bridge contract. They do not start a node, miner, native window, or network.
// Run with: node --test desktop/frontend/app.test.mjs
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { setImmediate } from 'node:timers/promises';
import test from 'node:test';
import vm from 'node:vm';

const source = readFileSync(new URL('./app.js', import.meta.url), 'utf8');
const html = readFileSync(new URL('./index.html', import.meta.url), 'utf8');
const runtimeImport = "import { Call } from '/wails/runtime.js';";
assert.ok(source.startsWith(runtimeImport), 'the harness replaces only the native runtime import');

class Element {
  constructor(tagName = 'div') {
    this.tagName = tagName;
    this.children = [];
    this.parent = null;
    this.attributes = new Map();
    this.listeners = new Map();
    this.style = {};
    this.hidden = false;
    this.disabled = false;
    this.className = '';
    this._text = '';
    this.classList = { toggle() {} };
  }
  set textContent(value) {
    this._text = String(value);
    for (const child of this.children) child.parent = null;
    this.children = [];
  }
  get textContent() {
    return this._text + this.children.map((child) => child.textContent).join('');
  }
  set innerHTML(_) { throw new Error('Untrusted HTML interpolation is forbidden'); }
  setAttribute(name, value) { this.attributes.set(name, String(value)); }
  removeAttribute(name) { this.attributes.delete(name); }
  addEventListener(name, callback) {
    const callbacks = this.listeners.get(name) ?? [];
    callbacks.push(callback);
    this.listeners.set(name, callbacks);
  }
  async dispatch(name) {
    await Promise.all((this.listeners.get(name) ?? []).map((callback) => callback()));
  }
  append(...children) { for (const child of children) this.insertBefore(child, null); }
  insertBefore(child, next) {
    if (child === next) return;
    child.remove();
    const index = next === null ? this.children.length : this.children.indexOf(next);
    assert.ok(index >= 0, 'insertBefore reference belongs to this parent');
    this.children.splice(index, 0, child);
    child.parent = this;
  }
  remove() {
    if (!this.parent) return;
    this.parent.children.splice(this.parent.children.indexOf(this), 1);
    this.parent = null;
  }
}

function deferred() {
  let resolve;
  let reject;
  const promise = new Promise((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
}

function validSnapshot(overrides = {}) {
  return {
    state: 'stopped', mode: 'local-wallet-pilot', attempts: 12, hashes_per_second: 1.25,
    wallet_state: 'closed', wallet_height: 61, spendable_atoms: 123456789, immature_atoms: 1000000000,
    payout_address: 'a'.repeat(34), gym_id: 'a'.repeat(64), gym_public_key: '02' + 'b'.repeat(64),
    backup_state: 'idle', backup_phase: '', backup_error: '',
    height: 61, pending: 0, last_error: '', recovery_error: '', prospect_count: 1,
    prospects: [{
      sequence: 1, id: 'a'.repeat(64), work_hash: 'b'.repeat(64),
      born_at_ms: Date.now() - 61_000, rarity: 'club', talent_budget: 320,
      attributes: { power: 60, speed: 70, chin: 80, stamina: 50, skill: 60 },
    }],
    ...overrides,
  };
}

async function settle() { await setImmediate(); }

function harness(initial = validSnapshot()) {
  const elements = new Map([...html.matchAll(/\bid="([^"]+)"/g)]
    .map((match) => [match[1], new Element()]));
  const document = new Element();
  document.hidden = false;
  document.getElementById = (id) => elements.get(id) ?? null;
  document.createElement = (tag) => new Element(tag);
  const window = new Element();
  const calls = [];
  const timers = new Map();
  let nextTimer = 0;
  let value = initial;
  let activeSnapshots = 0;
  let peakSnapshots = 0;
  let activeBridgeCalls = 0;
  let peakBridgeCalls = 0;
  const handlers = {
    Snapshot: () => value,
    Start: () => { value = { ...value, state: 'starting' }; },
    Stop: () => { value = { ...value, state: 'stopped' }; },
    ExportReceipt: () => '', ExportRecovery: () => '', VerifyBackup: () => '',
    LoginFixture: () => {}, Logout: () => {}, ResetFixtureAccount: () => {},
  };
  const Call = {
    async ByName(name, ...args) {
      assert.match(name, /^main\.Gym\.(Snapshot|Start|Stop|ExportReceipt|ExportRecovery|VerifyBackup|LoginFixture|Logout|ResetFixtureAccount)$/);
      const method = name.slice('main.Gym.'.length);
      calls.push({ method, args });
      activeBridgeCalls++;
      peakBridgeCalls = Math.max(peakBridgeCalls, activeBridgeCalls);
      if (method === 'Snapshot') {
        activeSnapshots++;
        peakSnapshots = Math.max(peakSnapshots, activeSnapshots);
      }
      try { return await handlers[method](...args); }
      finally {
        activeBridgeCalls--;
        if (method === 'Snapshot') activeSnapshots--;
      }
    },
  };
  const context = vm.createContext({
    document, window, Call, Intl, Date,
    requestAnimationFrame: (callback) => callback(),
    setTimeout(callback, delay) {
      assert.equal(delay, 1000, 'background polling remains bounded to once per second');
      const id = ++nextTimer;
      timers.set(id, callback);
      return id;
    },
    clearTimeout(id) { timers.delete(id); },
  });
  const api = {
    elements, document, window, calls, timers, handlers,
    set value(next) { value = next; },
    get value() { return value; },
    get peakSnapshots() { return peakSnapshots; },
    get peakBridgeCalls() { return peakBridgeCalls; },
    count(method) { return calls.filter((call) => call.method === method).length; },
    async refresh() { await document.dispatch('visibilitychange'); await settle(); },
    async tick() {
      assert.equal(timers.size, 1, 'exactly one polling timer is scheduled');
      const [id, callback] = timers.entries().next().value;
      timers.delete(id);
      await callback();
      await settle();
    },
    start() { vm.runInContext(source.slice(runtimeImport.length), context, { filename: 'app.js' }); },
  };
  return api;
}

function descendants(root, predicate) {
  return root.children.flatMap((child) => [
    ...(predicate(child) ? [child] : []), ...descendants(child, predicate),
  ]);
}
function receiptButtons(h) {
  return descendants(h.elements.get('roster'), (node) => node.className === 'receipt-button');
}

test('opening and restoring the page never starts mining or overlaps snapshot requests', async () => {
  const h = harness();
  const first = deferred();
  h.handlers.Snapshot = () => first.promise;
  h.start();
  await h.document.dispatch('visibilitychange');
  await h.document.dispatch('visibilitychange');
  assert.equal(h.count('Snapshot'), 1);
  assert.equal(h.count('Start'), 0);
  assert.equal(h.elements.get('mining-toggle').disabled, true);
  await h.window.dispatch('pagehide');
  await h.window.dispatch('pageshow');
  first.resolve(h.value);
  await settle();
  assert.equal(h.peakSnapshots, 1);
  assert.equal(h.timers.size, 1, 'restoring during an in-flight poll has one successor');
  h.handlers.Snapshot = () => h.value;
  await h.tick();
  await h.window.dispatch('pagehide');
  assert.equal(h.timers.size, 0);
  await h.window.dispatch('pageshow');
  await settle();
  assert.equal(h.timers.size, 1);
  assert.equal(h.count('Start'), 0);
});

test('saved-work warning permits explicit retry and starting can be stopped', async () => {
  const h = harness(validSnapshot({ pending: 1, recovery_error: 'Saved delivery needs a retry.' }));
  h.start();
  await settle();
  const button = h.elements.get('mining-toggle');
  assert.equal(button.disabled, false, 'recovery cannot disable the action that retries it');
  assert.match(h.elements.get('alert').textContent, /Saved delivery/);
  const starting = deferred();
  h.handlers.Start = () => starting.promise;
  const click = button.dispatch('click');
  await button.dispatch('click');
  assert.equal(h.count('Start'), 1, 'repeated clicks do not overlap control operations');
  assert.equal(button.disabled, true);
  h.value = { ...h.value, state: 'starting' };
  starting.resolve();
  await click;
  assert.equal(button.disabled, false);
  assert.equal(h.elements.get('action-label').textContent, 'Stop mining');
  await button.dispatch('click');
  assert.equal(h.count('Stop'), 1);
  assert.equal(h.elements.get('action-label').textContent, 'Start mining');
  assert.equal(button.disabled, false);
});

test('native export is serialized and cancellation never claims a saved receipt', async () => {
  const first = validSnapshot().prospects[0];
  const h = harness(validSnapshot({ prospect_count: 2, prospects: [first,
    { ...first, sequence: 2, id: 'c'.repeat(64), work_hash: 'd'.repeat(64) }],
  }));
  h.start();
  await settle();
  const buttons = receiptButtons(h);
  const dialog = deferred();
  h.handlers.ExportReceipt = () => dialog.promise;
  const exporting = buttons[0].dispatch('click');
  await buttons[1].dispatch('click');
  assert.equal(h.count('ExportReceipt'), 1);
  assert.ok(buttons.every((button) => button.disabled));
  assert.deepEqual(h.calls.find((call) => call.method === 'ExportReceipt').args, [2]);
  dialog.resolve('');
  await exporting;
  assert.equal(h.elements.get('alert').textContent, 'Export canceled.');
  assert.ok(buttons.every((button) => !button.disabled));
  h.handlers.ExportReceipt = () => 'Receipt saved.';
  await buttons[1].dispatch('click');
  assert.equal(h.elements.get('alert').textContent, 'Receipt saved.');
  assert.equal(h.count('ExportReceipt'), 2);
});

test('controls and native export wait for polling without competing for the single bridge slot', async () => {
  const h = harness();
  h.start();
  await settle();
  const button = h.elements.get('mining-toggle');
  for (const operation of ['Start', 'ExportReceipt']) {
    const snapshot = deferred();
    const action = deferred();
    h.handlers.Snapshot = () => snapshot.promise;
    h.handlers[operation] = () => action.promise;
    const refreshing = h.refresh();
    await settle();
    const before = h.count(operation);
    const clicked = operation === 'Start'
      ? button.dispatch('click') : receiptButtons(h)[0].dispatch('click');
    await settle();
    assert.equal(h.count(operation), before, 'user action waits for the active snapshot');
    snapshot.resolve(h.value);
    await refreshing;
    await settle();
    assert.equal(h.count(operation), before + 1);
    const snapshotsBefore = h.count('Snapshot');
    await h.tick();
    await h.refresh();
    await button.dispatch('click');
    await receiptButtons(h)[0].dispatch('click');
    assert.equal(h.count('Snapshot'), snapshotsBefore, 'polling yields while a user operation owns the slot');
    assert.equal(h.count(operation), before + 1, 'no additional user operation queues behind the first');
    h.handlers.Snapshot = () => h.value;
    action.resolve(operation === 'ExportReceipt' ? '' : undefined);
    await clicked;
    assert.equal(button.disabled, false);
    assert.equal(h.peakBridgeCalls, 1, 'the real bridge rejects simultaneous operations');
  }
});

test('malformed snapshots preserve the last coherent roster and recover on a valid update', async () => {
  const h = harness();
  h.start();
  await settle();
  const roster = h.elements.get('roster');
  const original = roster.children[0];
  const malformed = [
    { mode: 'live' }, { state: 'imaginary' }, { attempts: Number.MAX_SAFE_INTEGER + 1 },
    { hashes_per_second: Infinity }, { prospect_count: 0 }, { prospects: null },
    { prospects: [{ ...h.value.prospects[0], born_at_ms: 8640000000000001 }] },
    { prospects: [h.value.prospects[0], h.value.prospects[0]], prospect_count: 2 },
  ];
  for (const mutation of malformed) {
    h.value = { ...validSnapshot(), ...mutation };
    await h.refresh();
    assert.equal(roster.children.length, 1);
    assert.equal(roster.children[0], original);
    assert.equal(h.elements.get('attempts').textContent, '12');
    assert.equal(h.elements.get('mining-toggle').disabled, true);
    assert.match(h.elements.get('alert').textContent, /unreadable/);
  }
  h.value = validSnapshot({ attempts: 13 });
  await h.refresh();
  assert.equal(roster.children[0], original, 'valid refresh reuses the same prospect card');
  assert.equal(h.elements.get('attempts').textContent, '13');
  assert.equal(h.elements.get('mining-toggle').disabled, false);
  assert.equal(h.elements.get('alert').hidden, true);
});

test('unknown display fields stay text-only and cannot create invalid attribute bars', async () => {
  const value = validSnapshot({ last_error: '<img src=x onerror=alert(1)>' });
  value.prospects[0].rarity = '<script>attack()</script>';
  value.prospects[0].talent_budget = -1;
  value.prospects[0].attributes = { power: 101, speed: -2, chin: NaN, stamina: '70', skill: 100 };
  const h = harness(value);
  h.start();
  await settle();
  const roster = h.elements.get('roster');
  assert.match(roster.textContent, /Rarity unavailable/);
  assert.doesNotMatch(roster.textContent, /script/);
  assert.equal(h.elements.get('alert').textContent, value.last_error);
  const bars = descendants(roster, (node) => node.tagName === 'i');
  assert.deepEqual(bars.map((bar) => bar.style.width), ['0%', '0%', '0%', '0%', '100%']);
  assert.equal(roster.attributes.get('aria-live'), 'polite');
  const original = roster.children[0];
  await h.tick();
  assert.equal(roster.children[0], original);
});

test('wallet values are public and native recovery actions take no webview paths', async () => {
  const h = harness();
  h.start(); await settle();
  assert.match(h.elements.get('spendable').textContent, /1\.23456789/);
  assert.equal(h.elements.get('immature').textContent, '10');
  assert.equal(h.elements.get('payout').textContent, h.value.payout_address);
  const dialog = deferred();
  h.handlers.ExportRecovery = () => dialog.promise;
  const clicked = h.elements.get('export-recovery').dispatch('click');
  await h.elements.get('verify-backup').dispatch('click');
  await h.elements.get('mining-toggle').dispatch('click');
  await h.tick();
  assert.equal(h.count('ExportRecovery'), 1);
  assert.equal(h.count('VerifyBackup'), 0);
  assert.equal(h.count('Start'), 0);
  assert.deepEqual(h.calls.find((call) => call.method === 'ExportRecovery').args, []);
  dialog.resolve(''); await clicked;
  assert.equal(h.elements.get('alert').textContent, 'Selection canceled.');
  assert.equal(h.peakBridgeCalls, 1);
});

test('asynchronous backup verification keeps polling and excludes mining until completion', async () => {
  const h = harness();
  h.start(); await settle();
  h.handlers.VerifyBackup = () => {
    h.value = { ...h.value, backup_state: 'running', backup_phase: 'restoring' };
    return 'Backup verification started.';
  };
  await h.elements.get('verify-backup').dispatch('click');
  assert.deepEqual(h.calls.find((call) => call.method === 'VerifyBackup').args, []);
  assert.equal(h.elements.get('mining-toggle').disabled, true);
  assert.equal(h.elements.get('export-recovery').disabled, true);
  assert.match(h.elements.get('backup-status').textContent, /fresh restore/);
  const before = h.count('Snapshot');
  await h.tick();
  assert.equal(h.count('Snapshot'), before + 1);
  await h.elements.get('mining-toggle').dispatch('click');
  assert.equal(h.count('Start'), 0);
  h.value = { ...h.value, backup_state: 'succeeded', backup_phase: 'complete' };
  await h.tick();
  assert.equal(h.elements.get('mining-toggle').disabled, false);
  assert.match(h.elements.get('backup-status').textContent, /Backup verified/);
  h.value = { ...h.value, state: 'running' };
  await h.tick();
  assert.equal(h.elements.get('export-recovery').disabled, true);
  assert.equal(h.elements.get('verify-backup').disabled, true);
});

function accountSnapshot(state, overrides = {}) {
  const signedIn = state === 'signed_in';
  return validSnapshot({ mode: 'local-account-pilot',
    gym_id: signedIn ? 'a227587d837507dbcb5f9d781967339d816070fb7bd574931c690ba6937b88e5' : '',
    gym_public_key: signedIn ? '03ec7488667fd544b4a1de8fb0ae03730c9a2dc346c652cbaa59a077dbeddca519' : '',
    ...(signedIn ? {} : { payout_address: '', spendable_atoms: 0, immature_atoms: 0, prospect_count: 0, prospects: [] }),
    account: {
      state, fixture: state === 'signed_out' ? '' : 'zero256', id: signedIn ? 'c'.repeat(64) : '',
      generation: signedIn ? 1 : 0, hold_until_ms: 0, held: false, can_export: signedIn, last_error: '', ...overrides,
    },
  });
}

test('fixture login selects only a demo label and never starts mining', async () => {
  const h = harness(accountSnapshot('signed_out'));
  h.start(); await settle();
  assert.equal(h.elements.get('account-panel').hidden, false);
  assert.equal(h.elements.get('mining-toggle').disabled, true);
  assert.equal(h.elements.get('export-recovery').disabled, true);
  h.handlers.LoginFixture = (fixture) => {
    assert.equal(fixture, 'zero256');
    h.value = accountSnapshot('signing_in');
  };
  await h.elements.get('login-a').dispatch('click');
  assert.deepEqual(h.calls.find((call) => call.method === 'LoginFixture').args, ['zero256']);
  assert.equal(h.count('Start'), 0);
  assert.equal(h.elements.get('login-b').disabled, true);
  h.value = accountSnapshot('signed_in'); await h.tick();
  assert.equal(h.elements.get('mining-toggle').disabled, false);
  assert.equal(h.elements.get('export-recovery').disabled, false);
  assert.equal(h.elements.get('login-b').disabled, true);
  h.handlers.Logout = () => { h.value = accountSnapshot('signing_out'); };
  await h.elements.get('account-logout').dispatch('click');
  assert.deepEqual(h.calls.find((call) => call.method === 'Logout').args, []);
  assert.equal(h.elements.get('mining-toggle').disabled, true);
  assert.equal(h.peakBridgeCalls, 1);
});

test('reset hold blocks recovery export while keeping existing-wallet mining available', async () => {
  const h = harness(accountSnapshot('signed_in'));
  h.start(); await settle();
  h.handlers.ResetFixtureAccount = () => { h.value = accountSnapshot('resetting'); };
  await h.elements.get('account-reset').dispatch('click');
  assert.deepEqual(h.calls.find((call) => call.method === 'ResetFixtureAccount').args, []);
  assert.equal(h.elements.get('mining-toggle').disabled, true);
  h.value = accountSnapshot('signed_in', { generation: 2, hold_until_ms: Date.now()+86400000, held: true, can_export: false });
  await h.tick();
  assert.equal(h.elements.get('mining-toggle').disabled, false);
  assert.equal(h.elements.get('export-recovery').disabled, true);
  await h.elements.get('export-recovery').dispatch('click');
  assert.equal(h.count('ExportRecovery'), 0);
  assert.match(h.elements.get('account-hold').textContent, /marketplace purchases and listings/);
  assert.match(h.elements.get('account-hold').textContent, /Viewing and mining remain available/);
  h.value = accountSnapshot('signed_in', {generation:2}); await h.tick();
  assert.equal(h.elements.get('export-recovery').disabled, false);
  assert.equal(h.elements.get('account-hold').textContent, '');
});


test('account projection rejects mixed identity, contradictory hold and out-of-range dates', async () => {
  const h = harness(accountSnapshot('signed_in'));
  h.start(); await settle();
  const badValues = [
    accountSnapshot('signed_in', { fixture: '' }),
    accountSnapshot('signed_in', { id: '' }),
    accountSnapshot('signed_in', { generation: 0 }),
    accountSnapshot('signed_in', { held: true, can_export: true, hold_until_ms: Date.now() + 86400000 }),
    accountSnapshot('signed_in', { held: true, can_export: false, hold_until_ms: 0 }),
    accountSnapshot('signed_in', { held: true, can_export: false, hold_until_ms: 8640000000000001 }),
    accountSnapshot('signed_in', { fixture: 'incrementing256' }),
    { ...accountSnapshot('signed_in'), gym_id: 'f'.repeat(64) },
    { ...accountSnapshot('signed_out'), spendable_atoms: 1 },
    accountSnapshot('signing_in', { generation: 1 }),
  ];
  for (const value of badValues) {
    h.value = value; await h.refresh();
    assert.equal(h.elements.get('mining-toggle').disabled, true);
    assert.equal(h.elements.get('export-recovery').disabled, true);
    assert.match(h.elements.get('alert').textContent, /account|identity|signed-out/);
  }
  h.value = accountSnapshot('signed_in'); await h.refresh();
  assert.equal(h.elements.get('mining-toggle').disabled, false);
  assert.equal(h.elements.get('export-recovery').disabled, false);
});
