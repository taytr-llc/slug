// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.
import test from 'node:test';
import assert from 'node:assert/strict';
import {mkdtempSync, openSync, writeFileSync, fsyncSync, closeSync, renameSync, readFileSync, rmSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {setTimeout as delay} from 'node:timers/promises';
import {createActionHarness, epochJSON, parseEpochToken, nextEpoch, serializeClaims, MAX_ACTION_MS, MAX_UINT64} from './harness.mjs';

const ID = n => n.toString(16).padStart(64, '0');
const binding = {version: 1, coordinator_id: ID(1), authority_id: ID(2), tenant_id: 'fixture-tenant', client_id: 'fixture-client', connection_id: 'fixture-connection'};
const NOW = 1_700_000_000;
const SUBJECT = 'auth0|public-fixture-a';
const event = (correlation = 'reset-a', extra = {}) => ({tenant: {id: binding.tenant_id}, client: {client_id: binding.client_id}, connection: {id: binding.connection_id, strategy: 'auth0'}, user: {user_id: SUBJECT}, transaction: {correlation_id: correlation}, ...extra});
const postEvent = correlation => { const e = event(correlation); delete e.client; return e; };
const record = (n = 10, correlation = 'reset-a') => ({...binding, subject: SUBJECT, correlation_id: correlation, event_id: ID(n), reset_nonce: ID(n + 1), provider_operation_id: ID(n + 2), account_id: ID(3), principal_id: ID(4), barrier_event_id: ID(n + 3), from_epoch: 1n, to_epoch: 2n, verified_at: NOW, provider_state: 'prepared', authorized: false, commits: 0});
const origin = () => ({...binding, subject: SUBJECT, correlation_id: 'login-a', login_flow_id: ID(40), nonce: ID(41), original_auth_id: ID(42), original_flow_id: ID(40), credential_epoch: 1n, authenticated_at: NOW, mode: 'fresh'});
const clone = structuredClone;
const denialAPI = () => { const denied = []; return {denied, access: {deny: reason => denied.push(reason)}}; };
const requireDenied = (result, api) => { assert.deepEqual(result, {ok: false}); assert.deepEqual(api.denied, ['SLUG account operation unavailable.']); };

// A tiny test-owned durable journal, not Action cache. It fsyncs each complete
// snapshot and directory rename; reopened instances cannot infer missing rows.
// There are deliberately no latestPending(), currentEpoch() or password APIs.
class FileResolver {
  constructor(dir) { this.dir = dir; this.file = join(dir, 'fixture.json'); this.load(); }
  static create(t) {
    const dir = mkdtempSync(join(tmpdir(), 'slug-action-fixture-'));
    t.after(() => rmSync(dir, {recursive: true, force: true}));
    const f = Object.create(FileResolver.prototype);
    f.dir = dir; f.file = join(dir, 'fixture.json'); f.data = {resets: [], logins: [], origins: []}; f.save();
    return new FileResolver(dir);
  }
  save() {
    const temp = this.file + '.tmp';
    const fd = openSync(temp, 'w', 0o600);
    try { writeFileSync(fd, JSON.stringify(this.data, (_, value) => typeof value === 'bigint' ? {$u64: epochJSON(value)} : value)); fsyncSync(fd); }
    finally { closeSync(fd); }
    renameSync(temp, this.file);
    const parent = openSync(this.dir, 'r');
    try { fsyncSync(parent); } finally { closeSync(parent); }
  }
  load() {
    this.data = JSON.parse(readFileSync(this.file, 'utf8'), (_, value) => value && typeof value === 'object' && Object.keys(value).length === 1 && Object.hasOwn(value, '$u64') ? parseEpochToken(value.$u64) : value);
  }
  addReset(r) { this.data.resets.push(clone(r)); this.save(); }
  async resolveReset(s) {
    const matches = this.data.resets.filter(r => ['tenant_id', 'connection_id', 'subject', 'correlation_id'].every(k => r[k] === s[k]) && (s.client_id === undefined || r.client_id === s.client_id));
    if (matches.length !== 1) throw new Error('fixture resolver missing or ambiguous');
    return clone(matches[0]);
  }
  async authorizeReset(r) {
    const original = this.data.resets.find(x => x.provider_operation_id === r.provider_operation_id);
    if (!original || original.event_id !== r.event_id || original.provider_state !== 'prepared') throw new Error('operation mismatch');
    original.authorized = true; this.save();
    return {authorized: true, event_id: original.event_id, provider_operation_id: original.provider_operation_id};
  }
  // Simulates the provider's own credential transaction, NOT an Action API.
  commitOperation(operationID) {
    const r = this.data.resets.find(x => x.provider_operation_id === operationID);
    if (!r || !r.authorized || r.provider_state !== 'prepared' || r.commits !== 0) throw new Error('no unused permit');
    r.provider_state = 'committed'; r.commits++; this.save();
  }
  addLogin(r, {transactionID, refreshID, sessionID} = {}) {
    this.data.logins.push({r: clone(r), transactionID, refreshID, sessionID});
    if (r.mode === 'fresh') this.data.origins.push(clone(r));
    this.save();
  }
  async resolveLogin(s) {
    const found = this.data.logins.filter(({r, transactionID, refreshID, sessionID}) => {
      if (!['tenant_id', 'client_id', 'connection_id', 'subject', 'correlation_id'].every(k => r[k] === s[k])) return false;
      if (s.protocol === 'oauth2-refresh-token') return typeof refreshID === 'string' && refreshID === s.refresh_token_id;
      return (typeof transactionID === 'string' && transactionID === s.transaction_id) || (typeof sessionID === 'string' && sessionID === s.session_id);
    });
    if (found.length !== 1) throw new Error('no original authentication mapping');
    const r = found[0].r;
    if (r.mode === 'reuse') {
      const original = this.data.origins.find(o => o.original_auth_id === r.original_auth_id);
      if (!original || ['original_flow_id', 'credential_epoch', 'authenticated_at', 'subject'].some(k => r[k] !== original[k])) throw new Error('changed original authentication');
    }
    return clone(r);
  }
}

function ack(r, phase) {
  return {version: 1, coordinator_id: r.coordinator_id, authority_id: r.authority_id, event_id: r.event_id, provider_operation_id: r.provider_operation_id, phase, can_commit: phase === 'pending', barrier: {version: 2, event_id: r.barrier_event_id, account_id: r.account_id, principal_id: r.principal_id, status: phase, from_epoch: r.from_epoch, to_epoch: r.to_epoch, verified_at_ms: r.verified_at * 1000}};
}

function coordinatorFixture() {
  const events = new Map();
  const calls = [];
  return {
    events, calls, epoch: 1n, now: NOW,
    async beginReset(r) {
      calls.push(['begin', r.provider_operation_id]);
      const existing = events.get(r.event_id);
      if (existing) return clone(existing);
      const result = ack(r, 'pending'); events.set(r.event_id, result); return clone(result);
    },
    async completeReset(r) {
      calls.push(['complete', r.provider_operation_id]);
      if (!events.has(r.event_id)) throw new Error('no durable begin');
      const result = ack(r, 'completed'); events.set(r.event_id, result); this.epoch = r.to_epoch; return clone(result);
    },
    async finishLogin(r) {
      calls.push(['login', r.credential_epoch, r.authenticated_at]);
      if (r.credential_epoch !== this.epoch || this.now - r.authenticated_at >= 300 || r.authenticated_at > this.now) throw new Error('stale original authentication');
      return {accepted: true, login_flow_id: r.login_flow_id, original_auth_id: r.original_auth_id, credential_epoch: r.credential_epoch};
    },
  };
}

function fixture(t, r = record()) {
  const resolver = FileResolver.create(t); resolver.addReset(r);
  const coordinator = coordinatorFixture();
  return {r, resolver, coordinator, actions: createActionHarness({binding, resolver, coordinator})};
}

test('exact durable begin permits only one bound provider commit; completion survives resolver restart', async t => {
  const f = fixture(t), api = denialAPI();
  assert.deepEqual(await f.actions.onExecutePostChallenge(event(), api), {ok: true});
  assert.equal(api.denied.length, 0);
  const reopened = new FileResolver(f.resolver.dir);
  assert.throws(() => reopened.commitOperation(ID(999)));
  reopened.commitOperation(f.r.provider_operation_id);
  assert.throws(() => reopened.commitOperation(f.r.provider_operation_id));
  const actions = createActionHarness({...f, binding, resolver: reopened});
  assert.deepEqual(await actions.onExecutePostChangePassword(postEvent('reset-a')), {ok: true});
  assert.deepEqual(await actions.onExecutePostChangePassword(postEvent('reset-a')), {ok: true});
  assert.equal(new FileResolver(reopened.dir).data.resets[0].commits, 1);
  requireDenied(await actions.onExecutePostChallenge(event(), api), api);
});

test('pre-challenge accepts no mismatched, historical or non-durable acknowledgement', async t => {
  const mutations = [
    ['missing', () => undefined], ['false permit', a => ({...a, can_commit: false})],
    ['truthy permit', a => ({...a, can_commit: 'true'})], ['historical completed', (_, r) => ack(r, 'completed')],
    ...['version', 'coordinator_id', 'authority_id', 'event_id', 'provider_operation_id', 'phase'].map(k => [k, a => ({...a, [k]: 'wrong'})]),
    ...['version', 'event_id', 'account_id', 'principal_id', 'status', 'from_epoch', 'to_epoch', 'verified_at_ms'].map(k => ['barrier ' + k, a => ({...a, barrier: {...a.barrier, [k]: 'wrong'}})]),
    ['Number epoch', a => ({...a, barrier: {...a.barrier, from_epoch: 1}})],
  ];
  for (const [name, mutate] of mutations) await t.test(name, async t => {
    const f = fixture(t); let authorized = false;
    f.resolver.authorizeReset = async () => { authorized = true; throw new Error('unexpected'); };
    f.coordinator.beginReset = async r => mutate(ack(r, 'pending'), r);
    const api = denialAPI(); requireDenied(await f.actions.onExecutePostChallenge(event(), api), api);
    assert.equal(authorized, false);
    assert.throws(() => f.resolver.commitOperation(f.r.provider_operation_id));
  });
});

test('a lost begin acknowledgement preserves pending state and retries the same operation', async t => {
  const f = fixture(t), begin = f.coordinator.beginReset.bind(f.coordinator);
  f.coordinator.beginReset = async r => { await begin(r); throw new Error('lost response with credentials'); };
  const api = denialAPI(); requireDenied(await f.actions.onExecutePostChallenge(event(), api), api);
  assert.equal(f.coordinator.events.get(f.r.event_id).phase, 'pending');
  assert.throws(() => f.resolver.commitOperation(f.r.provider_operation_id));
  f.coordinator.beginReset = begin;
  assert.deepEqual(await f.actions.onExecutePostChallenge(event(), denialAPI()), {ok: true});
  assert.equal(f.coordinator.events.size, 1);
});

test('unknown durable permit write outcome denies; restart keeps the original bound permit', async t => {
  const f = fixture(t), save = f.resolver.authorizeReset.bind(f.resolver);
  f.resolver.authorizeReset = async (...args) => { await save(...args); throw new Error('uncertain acknowledgement'); };
  const api = denialAPI(); requireDenied(await f.actions.onExecutePostChallenge(event(), api), api);
  const r = new FileResolver(f.resolver.dir);
  assert.equal(r.data.resets[0].authorized, true);
  const a = createActionHarness({...f, binding, resolver: r});
  assert.deepEqual(await a.onExecutePostChallenge(event(), denialAPI()), {ok: true});
  r.commitOperation(f.r.provider_operation_id);
  assert.equal(r.data.resets[0].commits, 1);
});

test('completion uses the same committed operation, never the newer pending subject flow', async t => {
  const f = fixture(t), second = record(20, 'reset-b'); f.resolver.addReset(second);
  assert.deepEqual(await f.actions.onExecutePostChallenge(event(), denialAPI()), {ok: true});
  f.resolver.commitOperation(f.r.provider_operation_id);
  assert.deepEqual(await f.actions.onExecutePostChallenge(event('reset-b'), denialAPI()), {ok: true});
  assert.deepEqual(await f.actions.onExecutePostChangePassword(postEvent('reset-b')), {ok: false, outcome: 'reconciliation-required'});
  assert.deepEqual(await f.actions.onExecutePostChangePassword(postEvent('unknown')), {ok: false, outcome: 'reconciliation-required'});
  assert.deepEqual(await f.actions.onExecutePostChangePassword(postEvent('reset-a')), {ok: true});
  assert.equal(f.coordinator.events.get(second.event_id).phase, 'pending');
  assert.deepEqual(f.coordinator.calls.filter(c => c[0] === 'complete'), [['complete', f.r.provider_operation_id]]);
});

test('lost asynchronous completion cannot roll back password change or clear pending; same event resumes', async t => {
  const f = fixture(t);
  await f.actions.onExecutePostChallenge(event(), denialAPI()); f.resolver.commitOperation(f.r.provider_operation_id);
  const complete = f.coordinator.completeReset.bind(f.coordinator);
  f.coordinator.completeReset = async () => { throw new Error('remote unavailable'); };
  assert.deepEqual(await f.actions.onExecutePostChangePassword(postEvent('reset-a')), {ok: false, outcome: 'reconciliation-required'});
  assert.equal(f.resolver.data.resets[0].provider_state, 'committed');
  assert.equal(f.coordinator.events.get(f.r.event_id).phase, 'pending');
  f.coordinator.completeReset = complete;
  assert.deepEqual(await f.actions.onExecutePostChangePassword(postEvent('reset-a')), {ok: true});
});

test('correlation is only a lookup key: missing, ambiguous or foreign mappings fail', async t => {
  const cases = [
    ['missing correlation', e => { delete e.transaction.correlation_id; }],
    ['request body is not a mapping', e => { delete e.transaction; e.request = {body: {correlation_id: 'reset-a', provider_operation_id: ID(12)}}; }],
    ['foreign subject', e => { e.user.user_id = 'auth0|public-fixture-b'; }],
    ['foreign tenant', e => { e.tenant.id = 'elsewhere'; }],
    ['foreign client', e => { e.client.client_id = 'elsewhere'; }],
    ['foreign connection', e => { e.connection.id = 'elsewhere'; }],
    ['pre-hook missing client', e => { delete e.client; }],
  ];
  for (const [name, mutate] of cases) await t.test(name, async t => {
    const f = fixture(t), e = event(), api = denialAPI(); mutate(e);
    requireDenied(await f.actions.onExecutePostChallenge(e, api), api);
    assert.equal(f.coordinator.calls.length, 0);
  });
  await t.test('ambiguous mapping', async t => {
    const f = fixture(t); f.resolver.addReset(record(20));
    const api = denialAPI(); requireDenied(await f.actions.onExecutePostChallenge(event(), api), api);
    assert.equal(f.coordinator.calls.length, 0);
  });
});

test('resolver is mandatory and cannot be replaced by Action cache or metadata', () => {
  assert.throws(() => createActionHarness({binding, resolver: {}, coordinator: coordinatorFixture()}));
});

function loginEvent(refresh = false, correlation = 'login-a') {
  const e = event(correlation);
  e.transaction.protocol = refresh ? 'oauth2-refresh-token' : 'oidc-basic-profile';
  if (refresh) e.refresh_token = {id: 'fixture-refresh-1'};
  else e.transaction.id = 'fixture-transaction-1';
  return e;
}

test('old refresh preserves original epoch/auth time after provider reset and resolver restart', async t => {
  const f = fixture(t), first = origin();
  f.resolver.addLogin(first, {transactionID: 'fixture-transaction-1'});
  assert.deepEqual(await f.actions.onExecutePostLogin(loginEvent(), denialAPI()), {ok: true});
  const reuse = {...first, mode: 'reuse', login_flow_id: ID(50), nonce: ID(51), correlation_id: 'login-refresh'};
  f.resolver.addLogin(reuse, {refreshID: 'fixture-refresh-1'});
  f.coordinator.epoch = 2n; // The reset advances the authority, not this origin.
  const reopened = new FileResolver(f.resolver.dir);
  const actions = createActionHarness({...f, binding, resolver: reopened});
  const e = loginEvent(true, 'login-refresh');
  e.user.app_metadata = {credential_epoch: '2', auth_time: NOW + 1};
  e.request = {body: {credential_epoch: 2, original_auth_id: ID(99)}};
  const api = denialAPI(); requireDenied(await actions.onExecutePostLogin(e, api), api);
  assert.deepEqual(f.coordinator.calls.at(-1), ['login', 1n, NOW]);
  assert.equal(reopened.data.origins[0].credential_epoch, 1n);
});

test('fresh/reuse mapping rejects unknown origins, epoch laundering and refresh masquerading as fresh', async t => {
  for (const mode of ['missing', 'unknown', 'epoch', 'time', 'fresh']) await t.test(mode, async t => {
    const f = fixture(t), r = origin();
    f.resolver.addLogin(r, {transactionID: 'fixture-transaction-1'});
    const reuse = {...r, mode: 'reuse', correlation_id: 'login-refresh', login_flow_id: ID(50), nonce: ID(51)};
    if (mode === 'unknown') reuse.original_auth_id = ID(99);
    if (mode === 'epoch') reuse.credential_epoch = 2n;
    if (mode === 'time') reuse.authenticated_at++;
    if (mode === 'fresh') { reuse.mode = 'fresh'; reuse.original_flow_id = reuse.login_flow_id; }
    if (mode !== 'missing') f.resolver.addLogin(reuse, {refreshID: 'fixture-refresh-1'});
    const api = denialAPI(); requireDenied(await f.actions.onExecutePostLogin(loginEvent(true, 'login-refresh'), api), api);
    assert.equal(f.coordinator.calls.length, 0);
  });
});

test('a newly issued login transport cannot refresh the original five-minute authentication', async t => {
  const f = fixture(t), r = origin();
  f.resolver.addLogin(r, {transactionID: 'fixture-transaction-1'});
  const reuse = {...r, mode: 'reuse', correlation_id: 'login-refresh', login_flow_id: ID(50), nonce: ID(51)};
  f.resolver.addLogin(reuse, {refreshID: 'fixture-refresh-1'});
  assert.deepEqual(await f.actions.onExecutePostLogin(loginEvent(true, 'login-refresh'), denialAPI()), {ok: true});
  f.coordinator.now += 300;
  const api = denialAPI(); requireDenied(await f.actions.onExecutePostLogin(loginEvent(true, 'login-refresh'), api), api);
});

function claims(kind, epoch) {
  const common = {iss: 'https://identity.slug.invalid', aud: 'slug-provider-fixture-' + (kind === 'login' ? 'login' : 'reset'), sub: SUBJECT, iat: NOW, exp: NOW + 300, provider_version: 1, coordinator_id: ID(1), authority_id: ID(2), tenant_id: binding.tenant_id, client_id: binding.client_id, connection_id: binding.connection_id};
  return kind === 'login' ? {...common, auth_time: NOW, nonce: ID(41), slug_auth_epoch: epoch, login_flow_id: ID(40), original_auth_id: ID(42), original_flow_id: ID(40), auth_mode: 'fresh'} : {...common, event_id: ID(10), stage: 'begin', from_epoch: epoch, to_epoch: nextEpoch(epoch), verified_at: NOW, reset_nonce: ID(11), provider_operation_id: ID(12)};
}

test('epochs remain canonical raw uint64 integers above 2^53 and at MaxUint64', () => {
  for (const value of [1n, 9007199254740991n, 9007199254740992n, 9007199254740993n, MAX_UINT64]) {
    assert.equal(parseEpochToken(epochJSON(value)), value);
    const json = serializeClaims('login', claims('login', value));
    assert.match(json, new RegExp('"slug_auth_epoch":' + value.toString() + '(,|})'));
    assert.equal(parseEpochToken(/"slug_auth_epoch":([0-9]+)/.exec(json)[1]), value);
  }
  const reset = serializeClaims('reset', claims('reset', MAX_UINT64 - 1n));
  assert.match(reset, /"from_epoch":18446744073709551614,"to_epoch":18446744073709551615/);
  assert.throws(() => nextEpoch(MAX_UINT64));
  assert.notEqual(BigInt(JSON.parse(serializeClaims('login', claims('login', 9007199254740993n))).slug_auth_epoch), 9007199254740993n, 'demonstrates why a Number-first decoder is forbidden');
});

test('epoch codec rejects Number coercion, quoted/fractional/exponent/overflow tokens and aliases', () => {
  for (const value of [1, Number.MAX_SAFE_INTEGER, Number(MAX_UINT64), '1', null, true, 0n, -1n, MAX_UINT64 + 1n]) assert.throws(() => epochJSON(value));
  for (const raw of ['', '0', '-1', '+1', '01', '1.0', '1e0', ' 1', '1 ', '"1"', '18446744073709551616', '١', 1, 1n, null]) assert.throws(() => parseEpochToken(raw));
  for (const value of [1, '1', Number(9007199254740993n)]) assert.throws(() => serializeClaims('login', claims('login', value)));
  const withAlias = claims('login', 1n); withAlias.Slug_Auth_Epoch = 1n;
  assert.throws(() => serializeClaims('login', withAlias));
});

test('each hook has one budget capped at 18 seconds, and aborts hanging dependencies', async t => {
  for (const budgetMS of [0, -1, 18_001, NaN, Infinity, 1.5]) {
    const f = fixture(t); assert.throws(() => createActionHarness({...f, binding, budgetMS}));
  }
  assert.equal(MAX_ACTION_MS, 18_000);
  for (const phase of ['resolve', 'begin', 'authorize', 'complete', 'login']) await t.test(phase, async t => {
    const f = fixture(t); let signal;
    const hang = async (...args) => { signal = args.at(-1).signal; return new Promise(() => {}); };
    if (phase === 'resolve') f.resolver.resolveReset = hang;
    if (phase === 'begin') f.coordinator.beginReset = hang;
    if (phase === 'authorize') f.resolver.authorizeReset = hang;
    if (phase === 'complete') { await f.actions.onExecutePostChallenge(event(), denialAPI()); f.resolver.commitOperation(f.r.provider_operation_id); f.coordinator.completeReset = hang; }
    if (phase === 'login') { f.resolver.addLogin(origin(), {transactionID: 'fixture-transaction-1'}); f.coordinator.finishLogin = hang; }
    const actions = createActionHarness({...f, binding, budgetMS: 10}), api = denialAPI();
    if (phase === 'complete') assert.deepEqual(await actions.onExecutePostChangePassword(postEvent('reset-a')), {ok: false, outcome: 'reconciliation-required'});
    else if (phase === 'login') requireDenied(await actions.onExecutePostLogin(loginEvent(), api), api);
    else requireDenied(await actions.onExecutePostChallenge(event(), api), api);
    assert.equal(signal.aborted, true);
  });
});

test('late begin success after the deadline cannot reach permit storage or permit a second flow', async t => {
  const f = fixture(t); let release, authorized = false;
  f.coordinator.beginReset = async r => new Promise(resolve => { release = () => resolve(ack(r, 'pending')); });
  f.resolver.authorizeReset = async () => { authorized = true; };
  const actions = createActionHarness({...f, binding, budgetMS: 10}), api = denialAPI();
  requireDenied(await actions.onExecutePostChallenge(event(), api), api);
  release(); await delay(0);
  assert.equal(authorized, false);
});

test('serial calls consume one overall budget instead of receiving fresh deadlines', async t => {
  const f = fixture(t), resolve = f.resolver.resolveReset.bind(f.resolver);
  let authorized = false;
  f.resolver.resolveReset = async (s, {signal}) => { await delay(15, undefined, {signal}); return resolve(s); };
  f.coordinator.beginReset = async (r, {signal}) => { await delay(15, undefined, {signal}); return ack(r, 'pending'); };
  f.resolver.authorizeReset = async () => { authorized = true; };
  const actions = createActionHarness({...f, binding, budgetMS: 25}), api = denialAPI();
  requireDenied(await actions.onExecutePostChallenge(event(), api), api);
  assert.equal(authorized, false);
});

test('post-change resolver cannot substitute another client and completed ACK must bind its operation', async t => {
  for (const kind of ['client', 'operation', 'permit']) await t.test(kind, async t => {
    const f = fixture(t);
    await f.actions.onExecutePostChallenge(event(), denialAPI()); f.resolver.commitOperation(f.r.provider_operation_id);
    if (kind === 'client') f.resolver.data.resets[0].client_id = 'another-client';
    else f.coordinator.completeReset = async r => {
      const a = ack(r, 'completed');
      if (kind === 'operation') a.provider_operation_id = ID(99);
      else a.can_commit = true;
      return a;
    };
    assert.deepEqual(await f.actions.onExecutePostChangePassword(postEvent('reset-a')), {ok: false, outcome: 'reconciliation-required'});
  });
});

test('dependency errors and event credentials are never logged or reflected in public output', async t => {
  const f = fixture(t), logs = [];
  const original = {log: console.log, error: console.error, warn: console.warn};
  for (const key of Object.keys(original)) console[key] = (...args) => logs.push(args);
  try {
    f.coordinator.beginReset = async () => { throw new Error('password=fixture-password token=fixture-token secret=fixture-key'); };
    const e = event(); e.secrets = {key: 'fixture-key'}; e.request = {body: {password: 'fixture-password', token: 'fixture-token'}};
    const api = denialAPI(); const result = await f.actions.onExecutePostChallenge(e, api);
    requireDenied(result, api); assert.deepEqual(logs, []);
    assert.doesNotMatch(JSON.stringify({result, denied: api.denied}), /fixture-password|fixture-token|fixture-key/);
  } finally { Object.assign(console, original); }
});
