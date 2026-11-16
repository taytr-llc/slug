import { Call } from '/wails/runtime.js';

const elements = Object.fromEntries([
  'state', 'status-dot', 'hero-copy', 'mining-toggle', 'action-label', 'alert',
  'attempts', 'rate', 'prospect-count', 'nav-count', 'pending', 'pending-copy',
  'height', 'roster-label', 'empty', 'roster',
  'wallet-state', 'wallet-height', 'spendable', 'immature', 'payout', 'gym-id',
  'export-recovery', 'verify-backup', 'backup-status',
  'account-panel', 'account-state', 'account-copy', 'account-hold', 'login-a', 'login-b', 'account-logout', 'account-reset',
].map((id) => [id, document.getElementById(id)]));

const rarities = new Map([
  ['club', 'Club'], ['professional', 'Professional'], ['contender', 'Contender'],
  ['elite', 'Elite'], ['generational', 'Generational'],
]);
const attributes = [
  ['power', 'Power'], ['speed', 'Speed'], ['chin', 'Chin'],
  ['stamina', 'Stamina'], ['skill', 'Skill'],
];
const states = new Set(['stopped', 'starting', 'running', 'stopping', 'failed']);
const walletStates = new Set(['closed', 'connecting', 'syncing', 'ready', 'failed']);
const backupStates = new Set(['idle', 'running', 'succeeded', 'failed', 'cancelled']);
const backupPhases = new Map([['', ''], ['opening', 'Reading the file'], ['checking', 'Checking the original wallet'], ['restoring', 'Restoring a fresh wallet'], ['syncing', 'Syncing the restored wallet'], ['verifying', 'Comparing identity and funds'], ['complete', 'Complete']]);
const cards = new Map();
const numberFormat = new Intl.NumberFormat();
const rateFormat = new Intl.NumberFormat(undefined, { maximumFractionDigits: 2 });
const coinFormat = new Intl.NumberFormat(undefined, { maximumFractionDigits: 8 });
const dateFormat = new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' });
let snapshot = null;
let polling = null;
let pollTimer = null;
let pollGeneration = 0;
let command = null;
let exporting = null;
let connectionError = '';
let actionMessage = '';
let actionIsError = false;
let firstRender = true;
let disposed = false;

function text(node, value) {
  const next = String(value);
  if (node.textContent !== next) node.textContent = next;
}

function safeMessage(value, fallback = '') {
  const candidate = typeof value === 'string' ? value : value?.message;
  return typeof candidate === 'string' && candidate.trim()
    ? candidate.replace(/\s+/g, ' ').trim().slice(0, 280)
    : fallback;
}

function integer(value, minimum = 0, maximum = Number.MAX_SAFE_INTEGER) {
  return Number.isSafeInteger(value) && value >= minimum && value <= maximum;
}

const accountFixtures = {
  zero256: {
    gymID: 'a227587d837507dbcb5f9d781967339d816070fb7bd574931c690ba6937b88e5',
    publicKey: '03ec7488667fd544b4a1de8fb0ae03730c9a2dc346c652cbaa59a077dbeddca519',
  },
  incrementing256: {
    gymID: '28452fc8a478d28f284dfe671cfc57e7cabeca08ea953aef371f5674d3f4c510',
    publicKey: '033de5e2e3d5ad06f616572fcb3d61f089ce1105ef486debf308027b3b663d6d92',
  },
};

function normalize(value) {
  if (!value || !states.has(value.state) || !['local-wallet-pilot', 'local-account-pilot'].includes(value.mode)
    || !walletStates.has(value.wallet_state) || !backupStates.has(value.backup_state) || !backupPhases.has(value.backup_phase)
    || !['wallet_height', 'spendable_atoms', 'immature_atoms'].every((key) => integer(value[key]))
    || !['payout_address', 'gym_id', 'gym_public_key', 'backup_phase', 'backup_error'].every((key) => typeof value[key] === 'string' && value[key].length <= 280)
    || (value.payout_address !== '' && !/^[1-9A-HJ-NP-Za-km-z]{26,60}$/.test(value.payout_address))
    || (value.gym_id !== '' && !/^[a-f\d]{64}$/.test(value.gym_id))
    || (value.gym_public_key !== '' && !/^(02|03)[a-f\d]{64}$/.test(value.gym_public_key))
    || !['attempts', 'height', 'pending', 'prospect_count'].every((key) => integer(value[key]))
    || typeof value.hashes_per_second !== 'number' || !Number.isFinite(value.hashes_per_second)
    || value.hashes_per_second < 0 || !Array.isArray(value.prospects) || value.prospects.length > 100
    || value.prospect_count < value.prospects.length) {
    throw new Error('The local gym returned an unreadable update. Retrying…');
  }
  if (value.mode === 'local-account-pilot') {
    const a = value.account;
    if (!a || !['signed_out', 'signing_in', 'signed_in', 'resetting', 'signing_out', 'failed', 'expired'].includes(a.state)
      || !['', 'zero256', 'incrementing256'].includes(a.fixture) || !['', 'account_unavailable'].includes(a.last_error)
      || !integer(a.generation) || !integer(a.hold_until_ms, 0, 8640000000000000) || typeof a.held !== 'boolean' || typeof a.can_export !== 'boolean'
      || typeof a.id !== 'string' || (a.id !== '' && !/^[a-f\d]{64}$/.test(a.id))
      || (a.held && (a.can_export || a.hold_until_ms === 0))) throw new Error('The account returned an unreadable update.');
    if (a.state === 'signed_in') {
      const fixture = accountFixtures[a.fixture];
      if (!fixture || !/^[a-f\d]{64}$/.test(a.id) || a.generation < 1
        || value.gym_id !== fixture.gymID || value.gym_public_key !== fixture.publicKey) {
        throw new Error('The account identity does not match this gym.');
      }
    } else if (a.id !== '' || a.generation !== 0 || a.hold_until_ms !== 0 || a.held || a.can_export
      || value.gym_id !== '' || value.gym_public_key !== '' || value.payout_address !== ''
      || value.spendable_atoms !== 0 || value.immature_atoms !== 0 || value.prospect_count !== 0 || value.prospects.length !== 0) {
      throw new Error('The signed-out account returned unexpected wallet data.');
    }
  } else if (value.account) throw new Error('The account mode does not match this gym.');
  const seen = new Set();
  const prospects = value.prospects.map((prospect) => {
    if (!prospect || !integer(prospect.sequence, 1) || !/^[a-f\d]{64}$/i.test(prospect.id)
      || !/^[a-f\d]{64}$/i.test(prospect.work_hash) || !integer(prospect.born_at_ms, 1, 8640000000000000)
      || seen.has(prospect.id.toLowerCase())) {
      throw new Error('The local gym returned an unreadable prospect. Retrying…');
    }
    seen.add(prospect.id.toLowerCase());
    return { ...prospect, id: prospect.id.toLowerCase(), work_hash: prospect.work_hash.toLowerCase() };
  });
  prospects.sort((left, right) => right.sequence - left.sequence || left.id.localeCompare(right.id));
  return { ...value, prospects };
}

function ageLabel(bornAt, now) {
  const age = now - bornAt;
  if (age < 0) return `Born ${dateFormat.format(new Date(bornAt))}`;
  if (age < 60000) return 'Under 1m old';
  if (age < 3600000) return `${Math.floor(age / 60000)}m old`;
  if (age < 86400000) return `${Math.floor(age / 3600000)}h old`;
  if (age < 604800000) return `${Math.floor(age / 86400000)}d old`;
  return `${Math.floor(age / 604800000)}w old`;
}

function element(tag, className, content) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (content !== undefined) node.textContent = content;
  return node;
}

function makeCard(prospect) {
  const article = element('article', 'prospect');
  const top = element('div', 'card-top');
  const meta = element('div', 'card-meta');
  const rarity = element('span');
  const identifier = element('span', '', prospect.id.slice(0, 8));
  identifier.title = `Prospect ID: ${prospect.id}`;
  meta.append(rarity, identifier);
  const title = element('h4', '', `Prospect #${numberFormat.format(prospect.sequence)}`);
  title.id = `prospect-${prospect.id}`;
  article.setAttribute('aria-labelledby', title.id);
  const birth = element('div', 'birth');
  birth.title = `Birth: ${dateFormat.format(new Date(prospect.born_at_ms))}`;
  top.append(meta, title, birth);
  const body = element('div', 'card-body');
  const talent = element('div', 'talent');
  const budget = element('strong');
  talent.append(element('span', '', 'Talent budget'), budget);
  body.append(talent);
  const bars = new Map();
  for (const [key, label] of attributes) {
    const row = element('div', 'attribute');
    const track = element('span', 'track');
    track.setAttribute('aria-hidden', 'true');
    const fill = element('i');
    track.append(fill);
    const value = element('b');
    row.append(element('span', '', label), track, value);
    body.append(row);
    bars.set(key, { fill, value });
  }
  const button = element('button', 'receipt-button', 'Export receipt');
  button.type = 'button';
  button.setAttribute('aria-label', `Export receipt for prospect #${prospect.sequence}`);
  button.addEventListener('click', () => exportReceipt(prospect.sequence));
  article.append(top, body, button);
  return { article, rarity, birth, budget, bars, button, sequence: prospect.sequence };
}

function renderProspects(prospects, total) {
  const present = new Set(prospects.map((prospect) => prospect.id));
  for (const [id, card] of cards) {
    if (!present.has(id)) {
      card.article.remove();
      cards.delete(id);
    }
  }
  const now = Date.now();
  prospects.forEach((prospect, index) => {
    let card = cards.get(prospect.id);
    if (!card) {
      card = makeCard(prospect);
      cards.set(prospect.id, card);
    }
    text(card.rarity, rarities.get(prospect.rarity) ?? 'Rarity unavailable');
    text(card.birth, ageLabel(prospect.born_at_ms, now));
    text(card.budget, integer(prospect.talent_budget, 0, 500) ? numberFormat.format(prospect.talent_budget) : '—');
    for (const [key] of attributes) {
      const value = prospect.attributes?.[key];
      const valid = integer(value, 0, 100);
      const bar = card.bars.get(key);
      text(bar.value, valid ? value : '—');
      const width = `${valid ? value : 0}%`;
      if (bar.fill.style.width !== width) bar.fill.style.width = width;
    }
    const expected = elements.roster.children[index];
    if (expected !== card.article) elements.roster.insertBefore(card.article, expected ?? null);
  });
  elements.empty.hidden = total !== 0;
  text(elements['roster-label'], total === 0 ? 'A place for every find'
    : total > prospects.length ? `Latest ${numberFormat.format(prospects.length)} of ${numberFormat.format(total)}`
      : `${numberFormat.format(total)} ${total === 1 ? 'prospect' : 'prospects'} in your gym`);
  renderExportButtons();
}

function renderExportButtons() {
  for (const card of cards.values()) {
    const active = exporting === card.sequence;
    card.button.disabled = exporting !== null || command !== null;
    card.button.setAttribute('aria-busy', String(active));
    text(card.button, active ? 'Exporting…' : 'Export receipt');
  }
}

function renderAlert() {
  const recovery = safeMessage(snapshot?.recovery_error);
  const backend = safeMessage(snapshot?.last_error);
  const message = connectionError || recovery || (actionIsError ? actionMessage : '') || backend || actionMessage;
  elements.alert.hidden = !message;
  text(elements.alert, message);
}

function renderControls() {
  const state = snapshot?.state;
  const busy = command !== null;
  const active = state === 'running' || state === 'starting';
  const labels = {
    stopped: ['Stopped', 'Start mining', 'Your gym is ready. Start mining to discover your next prospect.'],
    starting: ['Starting', 'Stop mining', 'Getting your training gym ready.'],
    running: ['Mining', 'Stop mining', 'Mining is running in the background. New prospects appear here automatically.'],
    stopping: ['Stopping', 'Stopping…', 'Finishing the current attempt and saving your work.'],
    failed: ['Needs attention', 'Try starting again', 'Mining has stopped. Check the message below before trying again.'],
  };
  const [stateText, action, copy] = labels[state] ?? ['Connecting', 'Connecting…', 'Connecting to your local training gym.'];
  text(elements.state, connectionError ? 'Connection interrupted' : stateText);
  text(elements['hero-copy'], copy);
  text(elements['action-label'], busy ? (command === 'Start' ? 'Starting…' : command === 'Stop' ? 'Stopping…' : 'Please wait…') : action);
  elements['status-dot'].classList.toggle('active', state === 'running' && !connectionError);
  elements['mining-toggle'].classList.toggle('running', active);
  elements['mining-toggle'].setAttribute('aria-pressed', String(active));
  elements['mining-toggle'].setAttribute('aria-busy', String(busy || state === 'stopping'));
  // Starting again is also how a stopped gym retries saved uncertain work.
  // A recovery warning must not disable that explicit recovery action.
  elements['mining-toggle'].disabled = busy || exporting !== null || !snapshot || !!connectionError || state === 'stopping' || snapshot.backup_state === 'running' || (!!snapshot.account && snapshot.account.state !== 'signed_in');
  document.title = `${stateText} · SLUG Gym`;
  renderWallet();
  renderAccount();
  renderAlert();
}

function renderWallet() {
  const value = snapshot;
  const labels = { closed: 'Wallet closed', connecting: 'Connecting wallet', syncing: 'Syncing wallet', ready: 'Wallet ready', failed: 'Wallet needs attention' };
  text(elements['wallet-state'], labels[value?.wallet_state] ?? 'Connecting…');
  text(elements['wallet-height'], value ? `Synced through block ${numberFormat.format(value.wallet_height)}` : 'Waiting for wallet');
  text(elements.spendable, value ? coinFormat.format(value.spendable_atoms / 100000000) : '—');
  text(elements.immature, value ? coinFormat.format(value.immature_atoms / 100000000) : '—');
  text(elements.payout, value?.payout_address || 'Created on your first Start');
  text(elements['gym-id'], value?.gym_id || 'Created on your first Start');
  const unavailable = !value?.payout_address || !['stopped', 'failed'].includes(value?.state)
    || value?.backup_state === 'running' || exporting !== null || command !== null || !!connectionError;
  elements['export-recovery'].disabled = unavailable || (!!value?.account && !value.account.can_export);
  elements['verify-backup'].disabled = unavailable || (!!value?.account && value.account.state !== 'signed_in');
  elements['export-recovery'].setAttribute('aria-busy', String(exporting === 'ExportRecovery'));
  elements['verify-backup'].setAttribute('aria-busy', String(exporting === 'VerifyBackup' || value?.backup_state === 'running'));
  const messages = {
    idle: 'Stop mining to save or verify your test wallet backup.',
    running: 'Verifying a fresh restore. Mining is paused; Quit cancels verification.',
    succeeded: 'Backup verified: wallet identity and funds match. Prospect receipts stay in this gym.',
    failed: 'Backup verification failed. Check the recovery file and try again.',
    cancelled: 'Backup verification canceled. Your existing wallet is unchanged.',
  };
  const progress = value?.backup_state === 'running' ? `${backupPhases.get(value.backup_phase)}. ` : '';
  text(elements['backup-status'], progress + (messages[value?.backup_state] ?? messages.idle));
}

function renderAccount() {
  const a = snapshot?.account;
  elements['account-panel'].hidden = !a;
  if (!a) return;
  const labels = { signed_out: 'Signed out', signing_in: 'Opening your gym…', signed_in: a.fixture === 'zero256' ? 'Demo A signed in' : 'Demo B signed in', resetting: 'Recovering the same wallet…', signing_out: 'Saving and signing out…', failed: 'Account needs attention', expired: 'Sign-in expired' };
  text(elements['account-state'], labels[a.state]);
  const busy = command !== null || exporting !== null || ['signing_in', 'resetting', 'signing_out'].includes(a.state) || !!connectionError;
  const signedIn = a.state === 'signed_in';
  elements['login-a'].disabled = busy || !['signed_out', 'failed'].includes(a.state);
  elements['login-b'].disabled = elements['login-a'].disabled;
  elements['account-logout'].disabled = busy || ['signed_out'].includes(a.state);
  elements['account-reset'].disabled = busy || !signedIn || snapshot.backup_state === 'running';
  text(elements['account-copy'], a.state === 'failed' ? 'Account access could not finish. Retained wallet data has not been replaced. Sign out and try again.' : a.state === 'expired' ? 'Sign out and sign in again to continue with the same wallet and gym.' : signedIn ? 'Your wallet and personal gym stay the same across sign-ins and verified resets.' : 'Choose a demo account. Its wallet and personal gym are created automatically; mining starts only when you choose Start.');
  text(elements['account-hold'], a.held ? `Security hold until ${dateFormat.format(new Date(a.hold_until_ms))}. Withdrawals, fighter transfers, recovery export, recovery-authority changes, marketplace purchases and listings are blocked. Viewing and mining remain available.` : signedIn && !a.can_export ? 'Sign out and sign in again before exporting recovery: a fresh sign-in is required.' : '');
}

async function accountAction(method, fixture) {
  if (command !== null || exporting !== null) return;
  command = method;
  actionMessage = '';
  actionIsError = false;
  renderControls();
  renderExportButtons();
  try {
    if (polling) await polling;
    if (fixture) await Call.ByName(`main.Gym.${method}`, fixture);
    else await Call.ByName(`main.Gym.${method}`);
  } catch (error) {
    actionMessage = safeMessage(error, 'The account action could not start.');
    actionIsError = true;
  } finally {
    command = null;
    await refresh();
    renderControls();
    renderExportButtons();
  }
}

function render(value) {
  snapshot = value;
  text(elements.attempts, numberFormat.format(value.attempts));
  text(elements['prospect-count'], numberFormat.format(value.prospect_count));
  text(elements['nav-count'], numberFormat.format(value.prospect_count));
  text(elements.pending, numberFormat.format(value.pending));
  text(elements.height, numberFormat.format(value.height));
  text(elements['pending-copy'], value.pending === 0 ? 'Nothing waiting' : 'Saved work awaiting delivery');
  text(elements.rate, value.state === 'running'
    ? `${rateFormat.format(value.hashes_per_second)} attempts / second`
    : value.state === 'starting' ? 'Getting ready' : value.state === 'stopping' ? 'Finishing current work' : 'Mining is stopped');
  renderProspects(value.prospects, value.prospect_count);
  renderControls();
  if (firstRender) {
    firstRender = false;
    elements.roster.removeAttribute('aria-busy');
    // Loading existing receipts is not a stream of new discoveries. Announce
    // only subsequent additions, leaving age/stat updates quiet.
    requestAnimationFrame(() => {
      if (!disposed) elements.roster.setAttribute('aria-live', 'polite');
    });
  }
}

function refresh() {
  if (polling) return polling;
  // The native supervisor admits one request at a time. A user operation owns
  // that slot until it finishes; periodic refreshes never compete with it.
  if (command !== null || exporting !== null) return Promise.resolve();
  polling = (async () => {
    try {
      const value = normalize(await Call.ByName('main.Gym.Snapshot'));
      if (disposed) return;
      connectionError = '';
      render(value);
    } catch (error) {
      if (disposed) return;
      connectionError = safeMessage(error, 'Cannot reach your local gym. Retrying…');
      renderControls();
    }
  })().finally(() => { polling = null; });
  return polling;
}

async function poll(generation = pollGeneration) {
  await refresh();
  if (!disposed && generation === pollGeneration) {
    pollTimer = setTimeout(() => poll(generation), 1000);
  }
}

async function toggleMining() {
  if (elements['mining-toggle'].disabled || command || exporting !== null || !snapshot) return;
  command = snapshot.state === 'running' || snapshot.state === 'starting' ? 'Stop' : 'Start';
  actionMessage = '';
  actionIsError = false;
  renderControls();
  renderExportButtons();
  try {
    if (polling) await polling;
    await Call.ByName(`main.Gym.${command}`);
  } catch (error) {
    actionMessage = safeMessage(error, 'The mining control could not finish. Please try again.');
    actionIsError = true;
  } finally {
    command = null;
    await refresh();
    renderControls();
    renderExportButtons();
  }
}

async function exportReceipt(sequence) {
  if (exporting !== null || command !== null || !integer(sequence, 1)) return;
  exporting = sequence;
  actionMessage = '';
  actionIsError = false;
  renderExportButtons();
  renderControls();
  try {
    if (polling) await polling;
    const message = await Call.ByName('main.Gym.ExportReceipt', sequence);
    actionMessage = message === '' ? 'Export canceled.' : safeMessage(message, 'Receipt export did not return a result.');
    actionIsError = typeof message !== 'string';
  } catch (error) {
    actionMessage = safeMessage(error, 'The receipt could not be exported. Please try again.');
    actionIsError = true;
  } finally {
    exporting = null;
    renderExportButtons();
    await refresh();
    renderControls();
  }
}

async function walletAction(method, button) {
  if (button.disabled || exporting !== null || command !== null) return;
  exporting = method;
  actionMessage = '';
  actionIsError = false;
  renderExportButtons();
  renderControls();
  try {
    if (polling) await polling;
    const message = await Call.ByName(`main.Gym.${method}`);
    actionMessage = message === '' ? 'Selection canceled.' : safeMessage(message, 'The wallet action did not return a result.');
    actionIsError = typeof message !== 'string';
  } catch (error) {
    actionMessage = safeMessage(error, 'The wallet action could not finish. Please try again.');
    actionIsError = true;
  } finally {
    exporting = null;
    renderExportButtons();
    await refresh();
    renderControls();
  }
}

elements.state.setAttribute('aria-live', 'polite');
elements.alert.setAttribute('aria-atomic', 'true');
elements.roster.setAttribute('aria-live', 'off');
elements.roster.setAttribute('aria-busy', 'true');
elements['mining-toggle'].addEventListener('click', toggleMining);
elements['export-recovery'].addEventListener('click', () => walletAction('ExportRecovery', elements['export-recovery']));
elements['verify-backup'].addEventListener('click', () => walletAction('VerifyBackup', elements['verify-backup']));
elements['login-a'].addEventListener('click', () => { if (!elements['login-a'].disabled) return accountAction('LoginFixture', 'zero256'); });
elements['login-b'].addEventListener('click', () => { if (!elements['login-b'].disabled) return accountAction('LoginFixture', 'incrementing256'); });
elements['account-logout'].addEventListener('click', () => { if (!elements['account-logout'].disabled) return accountAction('Logout'); });
elements['account-reset'].addEventListener('click', () => { if (!elements['account-reset'].disabled) return accountAction('ResetFixtureAccount'); });
document.addEventListener('visibilitychange', () => {
  if (!document.hidden && !disposed) void refresh();
});
window.addEventListener('pagehide', () => {
  disposed = true;
  pollGeneration += 1;
  clearTimeout(pollTimer);
});
window.addEventListener('pageshow', () => {
  if (disposed) {
    disposed = false;
    void poll();
  }
});
renderControls();
void poll();
