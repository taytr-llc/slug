import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import { loadUI, deferred } from './harness.mjs';

const lockPath = process.argv[2], artifact = process.argv[3];
if (!lockPath || !artifact) throw new Error('Usage: node test.mjs input-lock.json exclusive-artifact-directory');
const lock = JSON.parse(fs.readFileSync(lockPath));
const sha = p => crypto.createHash('sha256').update(fs.readFileSync(p)).digest('hex');
const verify = () => { for (const f of lock.files) assert.equal(sha(f.path), f.sha256, `Changed source ${f.path}`); };
verify(); fs.mkdirSync(artifact, { recursive: false });
fs.copyFileSync(lockPath, path.join(artifact, 'input-lock.json'));
const corpus = JSON.parse(fs.readFileSync(lock.corpus));
const fixture = (family, name = 'v2/zero256-awarded/initial') => structuredClone(corpus.positives.find(c => c.family === family && c.name === name).value);
const id = n => n.toString(16).padStart(64, '0');
const basePage = fixture('page');
const gym = basePage.items[0].ownership.owner_public_key;
function snapshot(view = id(1), held = false) {
  return { version: 1, mode: 'local-owned-gym', state: view ? 'signed_in' : 'signed_out', pending_reset_id: '',
    account: { state: view ? 'signed_in' : 'signed_out', id: view ? id(2) : '', held, can_export: !held && !!view, hold_until_ms: held ? 1795300000000 : 0 },
    view: view ? { view_id: view, account_id: id(2), gym_public_key: gym } : null,
    child: view ? { state: 'stopped', wallet_state: 'ready', attempts: 0, height: 10, pending: 0, spendable_atoms: 100000000, immature_atoms: 0, backup: { state: 'idle', phase: '', error: '' } } : null };
}
const ui = s => loadUI(lock.js, lock.html, s ?? snapshot());
const details = { recipient_gym: `03${'a'.repeat(64)}`, total_fee_atoms: 20000, return_address: 'public-test-return' };
const status = (state = 'review', request = id(3)) => ({ version: 1, view_id: id(1), request_id: request, prospect_id: basePage.items[0].ownership.prospect_id, recipient_gym: details.recipient_gym, state, error: '', review: { details }, delivery: null, operation: null });
const tests = [];
const test = (name, fn) => tests.push({ name, fn });

test('every dispatched method exists in actual native source', async () => {
  const js = fs.readFileSync(lock.js, 'utf8');
  const native = lock.native.map(p => fs.readFileSync(p, 'utf8')).join('\n');
  const methods = [...new Set(js.match(/\bOwned[A-Z][A-Za-z]+\b/g))];
  assert(methods.length > 25);
  for (const m of methods) assert.match(native, new RegExp(`func \\(g \\*Gym\\) ${m}\\(`), m);
});

test('inherited owned page and history render without DTO substitution', async () => {
  const u = await ui(); u.api.renderStable(fixture('page')); u.api.set({ history: fixture('history_page') }); u.api.renderHistory();
  assert.match(u.$('fighters').textContent, new RegExp(basePage.items[0].manifest.name.display));
  assert.match(u.$('history').textContent, /2026-11-21.*Loss.*decision.*\+1 XP/);
  assert(!/undefined|NaN/.test(u.$('fighters').textContent + u.$('history').textContent));
});

test('late old-view preparation response cannot repopulate new view', async () => {
  const u = await ui(), d = deferred();
  u.$('transfer-fighter').value = basePage.items[0].ownership.prospect_id; u.$('recipient').value = details.recipient_gym; u.api.controls();
  u.handlers.set('OwnedPrepareTransfer', () => d.promise);
  const running = u.$('transfer-prepare').fire(); await Promise.resolve();
  u.snapshot(snapshot(id(9))); await u.api.refresh();
  d.resolve(status()); await running;
  assert.equal(u.api.state().request, ''); assert.equal(u.api.state().transfer, null);
  assert.equal(u.$('transfer-details').textContent, '');
});

test('view change clears all old captured identity text and controls', async () => {
  const u = await ui(); u.api.renderStable(fixture('page')); u.api.set({ history: fixture('history_page') }); u.api.renderHistory();
  u.$('entry-status').textContent = '2026-11-21 old gym entry';
  u.snapshot(snapshot('')); await u.api.refresh(); u.api.controls();
  assert.equal(u.$('fighters').textContent, ''); assert.equal(u.$('history').textContent, '');
  assert(!u.$('history-status').textContent.includes(fixture('history').ownership.prospect_id), 'old fighter remains in history label');
  assert(!u.$('entry-status').textContent.includes('old gym'), 'old entry remains in label');
  assert(u.$('transfer-confirm').disabled);
});

test('failed refresh clears stale signed-in action controls', async () => {
  const u = await ui(); u.api.set({ request: id(3), transfer: status() }); u.api.controls();
  u.handlers.set('OwnedSnapshot', () => { throw new Error('Original helper unavailable'); });
  await u.timers.shift().fn();
  assert(u.$('transfer-confirm').disabled, 'stale review still actionable after helper failure');
  assert(u.$('mining').disabled, 'stale mining control still actionable after helper failure');
  assert.equal(u.$('gym-key').textContent, '—', 'unavailable account retained previous gym identity');
  assert.equal(u.$('spendable').textContent, '—', 'unavailable account retained undated spendable balance');
  assert.equal(u.$('wallet-state').textContent, 'Unavailable');
  assert.equal(u.$('state').textContent, 'Unavailable');
});

test('saved request selection dispatches exact selected ID, not latest', async () => {
  const u = await ui();
  u.handlers.set('OwnedTransferRequests', () => [ { request_id: id(5), created_at_ms: 2000, prospect_id: id(15) }, { request_id: id(4), created_at_ms: 1000, prospect_id: id(14) } ]);
  u.handlers.set('OwnedTransferOperation', (_, r) => ({ version: 1, request_id: r, stage: 'intent', request: { recipient_gym: details.recipient_gym } }));
  await u.$('requests-refresh').fire(); assert.equal(u.api.state().request, '');
  u.$('transfer-requests').value = id(4); await u.$('transfer-requests').fire('change');
  await u.$('transfer-lookup').fire();
  assert.deepEqual(u.calls.at(-1), { method: 'OwnedTransferOperation', args: [id(1), id(4)] });
});

test('review and sign delegates consent to native and never auto-delivers', async () => {
  const u = await ui(); u.api.set({ request: id(3) }); u.api.renderTransfer(status()); u.api.controls();
  u.handlers.set('OwnedConfirmTransfer', () => status('dialog'));
  await u.$('transfer-confirm').fire();
  assert.deepEqual(u.calls, [{ method: 'OwnedConfirmTransfer', args: [id(1), id(3)] }]);
  assert.equal(u.api.state().dialogPending, true); assert(u.$('send-commit').disabled);
  u.handlers.set('OwnedTransferStatus', () => status('cancelled')); await u.api.refresh();
  assert.equal(u.api.state().transfer.state, 'cancelled');
  assert(!u.calls.some(c => c.method === 'OwnedDeliverTransfer'));
});

test('delivery completion alone never claims finalized ownership', async () => {
  const u = await ui(); u.api.set({ request: id(3) });
  u.api.renderOperation({ version: 1, request_id: id(3), stage: 'complete', request: { recipient_gym: details.recipient_gym } });
  assert.match(u.$('transfer-status').textContent, /Transactions delivered.*Check ownership finality/);
  assert(!/Ownership finalized/.test(u.$('finality').textContent));
});

test('explicit finality preserves void, pending, pending evidence and conflict labels', async () => {
  const u = await ui(); u.api.set({ request: id(3) }); u.api.controls();
  for (const [state, expected] of [['pending', /pending signed checkpoint/], ['void', /competing transfer is void/], ['finalized', /Ownership finalized/]]) {
    u.handlers.set('OwnedTransferFinality', () => ({ version: 1, request_id: id(3), state, head: { chain_height: 164 }, public_evidence_pending: state === 'finalized', conflict_id: state === 'finalized' ? id(7) : '' }));
    await u.$('transfer-finality').fire(); assert.match(u.$('finality').textContent, expected);
    if (state === 'finalized') assert.match(u.$('finality').textContent, /evidence publication is still pending.*later chain conflict/);
  }
});

test('held controls block transfer/sign/send/export but preserve reads and roster', async () => {
  const u = await ui(snapshot(id(1), true)); u.api.set({ request: id(3), transfer: status() });
  u.$('transfer-fighter').value = id(4); u.$('recipient').value = details.recipient_gym; u.api.controls();
  for (const id of ['transfer-prepare','transfer-resume','transfer-confirm','send-commit','send-spend','export-recovery']) assert(u.$(id).disabled, id);
  for (const id of ['transfer-lookup','transfer-finality','observe-commit','requests-refresh','refresh-stable','slot-lookup','verify-backup']) assert(!u.$(id).disabled, id);
});

test('roster selection sends exact captured head/state and bounded slot', async () => {
  const u = await ui(), p = fixture('page'); u.api.renderStable(p); u.api.controls();
  const card = u.$('fighters').children[0], pick = card.children.find(c => c.tagName === 'SELECT'), apply = card.children.find(c => c.textContent === 'Prepare roster selection');
  assert.equal(pick.children.length, 10); pick.value = '10';
  u.handlers.set('OwnedPrepareSlot', (_, request) => ({ request, operation_id: id(8) }));
  await apply.fire(); const call = u.calls.at(-1);
  assert.deepEqual(call.args, [id(1), { expected_revision: p.status.roster_revision, slot: 10, prospect_id: p.items[0].ownership.prospect_id, expected_state_id: p.items[0].ownership.state_id, head: p.head }]);
});

test('entry replace is bounded/sorted and withdrawal works without stable capture', async () => {
  const u = await ui(), p = fixture('page'); p.status.slots = [id(8), id(4), '', '', '', '', '', '', '', ''];
  const event = { version: 1, event_id: id(7), saturday: '2026-11-21', cutoff_ms: 1795291200000 };
  u.api.set({ page: p, entry: { version: 1, event, revision: 5, saved_ids: [id(4)], phase: 'open' } });
  u.handlers.set('OwnedPrepareEntry', (_, request) => ({ request, operation_id: id(9) }));
  await u.api.prepareEntry('replace');
  assert.deepEqual(u.calls.at(-1).args[1], { version: 1, event_id: id(7), saturday: '2026-11-21', action: 'replace', expected_revision: 5, expected_roster_revision: p.status.roster_revision, prospect_ids: [id(4), id(8)] });
  u.api.set({ page: null }); await u.api.prepareEntry('withdraw');
  assert.equal(u.calls.at(-1).args[1].expected_roster_revision, 0); assert.deepEqual(u.calls.at(-1).args[1].prospect_ids, []);
});

const results = [];
for (const t of tests) {
  try { await t.fn(); results.push({ name: t.name, pass: true }); console.log(`PASS ${t.name}`); }
  catch (e) { results.push({ name: t.name, pass: false, error: e.message, stack: e.stack }); console.log(`FAIL ${t.name}: ${e.message}`); }
}
let unchanged = false;
try { verify(); unchanged = true; } catch (e) { results.push({ name: 'source binding', pass: false, error: e.message }); }
const report = { version: 1, node: process.version, input_lock_sha256: sha(lockPath), sources_unchanged: unchanged, tests: results, passed: results.every(t => t.pass) && unchanged };
fs.writeFileSync(path.join(artifact, 'report.json'), JSON.stringify(report, null, 2) + '\n');
process.exitCode = report.passed ? 0 : 1;
