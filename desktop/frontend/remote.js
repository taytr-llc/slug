import { Call } from '/wails/runtime.js';
// Five fixed native methods. No config, path, source, token or arbitrary action
// is supplied by this page; the native owner independently pins its input.
const $ = id => document.getElementById(id);
const text = (id, value) => { $(id).textContent = String(value ?? ''); };
const isID = value => typeof value === 'string' && /^[0-9a-f]{64}$/.test(value);
const safeInt = (value, max = Number.MAX_SAFE_INTEGER) => Number.isSafeInteger(value) && value >= 0 && value <= max;
const amount = value => safeInt(value, 2100000000000000) ? (value / 1e8).toFixed(8) : '—';
const human = value => String(value ?? '').replaceAll('_', ' ');
const requireValue = value => { if (!value) throw new Error('Unverified remote status'); };
let snapshot = null, configID = '', fixture = '', busy = false, retired = false;
const view = () => snapshot?.view?.view_id ?? '';
const call = (method, ...args) => Call.ByName(`main.RemoteGym.${method}`, ...args);
function message(value) { text('alert', value); $('alert').hidden = !value; }
function clearStatus() {
  snapshot = null;
  for (const id of ['state', 'account-state', 'wallet-state']) text(id, 'Unavailable');
  for (const id of ['attempts', 'rate', 'pending', 'height', 'spendable', 'immature', 'unconfirmed', 'payout', 'gym-key']) text(id, '—');
  text('hold', ''); text('mining', 'Start mining'); text('mining-copy', 'Waiting for the original remote gym status.');
}
function controls() {
  const signed = snapshot?.state === 'signed_in' && isID(view());
  for (const [id, expected] of [['login-a', 'zero256'], ['login-b', 'incrementing256']]) {
    $(id).disabled = busy || retired || !snapshot || signed || snapshot.state !== 'signed_out' || fixture !== expected;
    $(id).hidden = !!fixture && fixture !== expected;
  }
  $('logout').disabled = busy || retired || !signed;
  $('mining').disabled = busy || retired || !signed || !['running', 'stopped', 'failed'].includes(snapshot?.remote?.state);
}
function validateSnapshot(value) {
  requireValue(value?.version === 1 && isID(value.config_id) && (!configID || value.config_id === configID));
  requireValue(value.account?.state === value.state && ['zero256', 'incrementing256'].includes(value.account.fixture));
  requireValue(!fixture || value.account.fixture === fixture);
  if (value.state === 'signed_in') {
    requireValue(isID(value.view?.view_id) && value.view.account_id === value.account.id && value.view.fixture === value.account.fixture);
    const r = value.remote;
    requireValue(r?.version === 1 && r.config_id === value.config_id && r.view?.view_id === value.view.view_id);
    for (const name of ['attempts', 'hashes_per_second_milli', 'pending', 'wallet_height']) requireValue(safeInt(r[name]));
    for (const name of ['spendable_atoms', 'immature_atoms', 'unconfirmed_atoms']) requireValue(safeInt(r[name], 2100000000000000));
  } else {
    requireValue(['signed_out', 'expired', 'failed'].includes(value.state) && value.view === null && value.remote === null);
  }
  return value;
}
function render(value) {
  const s = validateSnapshot(value); configID = s.config_id; fixture = s.account.fixture; snapshot = s;
  const r = s.remote;
  text('state', human(r?.state ?? s.state)); text('account-state', human(s.state));
  text('hold', s.account.held ? 'This account is on a security hold.' : '');
  text('mining-copy', view() ? 'Mining uses this gym’s existing wallet and selected remote pool.' : 'Sign in to attach your existing wallet.');
  text('mining', r?.state === 'running' ? 'Stop mining' : 'Start mining');
  text('attempts', r?.attempts ?? 0); text('rate', `${((r?.hashes_per_second_milli ?? 0) / 1000).toFixed(2)} hashes / second`);
  text('pending', r?.pending ?? 0); text('height', r?.wallet_height ?? 0); text('wallet-state', human(r?.wallet_state ?? 'closed'));
  text('spendable', amount(r?.spendable_atoms)); text('immature', amount(r?.immature_atoms)); text('unconfirmed', amount(r?.unconfirmed_atoms));
  text('payout', r?.payout_address || '—'); text('gym-key', s.view?.gym_public_key || '—');
  if (r?.last_error) message(`Remote mining status: ${human(r.last_error)}. Pending delivery remains retained.`);
  else if (s.error) message(`Remote gym status: ${human(s.error)}.`);
}
function failed(error) {
  // Native errors contain only static public messages. Never echo an arbitrary
  // bridge payload into the page, and never repeat a mutation after uncertainty.
  const value = String(error?.message ?? error ?? '');
  clearStatus();
  if (value.includes('outcome is unknown')) {
    retired = true;
    message('Request outcome unknown. No request was repeated. Close this connection and inspect the original attachment’s saved state before another action.');
  } else if (value.includes('did not close cleanly') || value.includes('cleanup')) {
    retired = true; message('Cleanup is incomplete. The original helper and selected configuration remain retained.');
  } else {
    message('The original remote gym is unavailable. No action was repeated.');
  }
}
async function refresh() { render(await call('Snapshot')); }
async function task(action) {
  if (busy || retired) return;
  busy = true; controls(); message('');
  try { await action(); } catch (error) { failed(error); }
  finally { busy = false; controls(); }
}
function bind(id, action) { $(id).addEventListener('click', () => task(action)); }
for (const [id, expected] of [['login-a', 'zero256'], ['login-b', 'incrementing256']]) bind(id, async () => {
  requireValue(fixture === expected && !view());
  render(await call('LoginFixture', expected));
});
bind('mining', async () => {
  const original = view(); requireValue(isID(original));
  const method = snapshot.remote.state === 'running' ? 'Stop' : 'Start';
  const result = await call(method, original);
  requireValue(view() === original && result.view?.view_id === original); render(result);
});
bind('logout', async () => {
  const original = view(); requireValue(isID(original));
  const result = await call('Logout', original);
  requireValue(view() === original && result.state === 'signed_out' && result.view === null && result.remote === null); render(result);
});
async function poll() {
  if (!busy && !retired) {
    busy = true; controls();
    try { await refresh(); } catch (error) { failed(error); }
    finally { busy = false; controls(); }
  }
  if (!retired) setTimeout(poll, 2000);
}
controls(); poll();
