import { Call } from '/wails/runtime.js';

// This entry point accepts only the candidate pilot's typed, authenticated view.
const MODE = 'local-manifest-pilot';
const DESCRIPTOR = 'c1a8650a39f695814db67fddf79815ef249a13a5c0cc5af0b906b061c4c3a303';
const GENESIS = '7893676099cf9985bb198dfb6a347d9422a19f2422cf3326a999a7230ce246cd';
const DATE_MAX = 8640000000000000;
const CACHE_MAX = 1 << 20;
const ids = ['state', 'account-state', 'account-copy', 'hold', 'login-a', 'login-b', 'logout', 'reset', 'mining', 'mining-copy', 'alert', 'attempts', 'rate', 'accepted', 'pending', 'height', 'wallet-state', 'wallet-height', 'spendable', 'immature', 'payout', 'gym-id', 'export-recovery', 'verify-backup', 'backup', 'refresh', 'coverage', 'announcement', 'empty', 'empty-copy', 'cards'];
const ui = Object.fromEntries(ids.map((id) => [id, document.getElementById(id)]));
const attrs = ['power', 'speed', 'chin', 'stamina', 'skill'];
const rarity = ['club', 'professional', 'contender', 'elite', 'generational'];
const styles = ['slugger', 'outfighter', 'boxer_puncher', 'pressure', 'boxer'];
const classes = ['light', 'welter', 'middle', 'light_heavy', 'heavy'];
const states = ['stopped', 'starting', 'running', 'stopping', 'failed'];
const accountStates = ['signed_out', 'signing_in', 'signed_in', 'resetting', 'signing_out', 'failed', 'expired'];
const walletStates = ['closed', 'connecting', 'syncing', 'ready', 'failed'];
const backupStates = ['idle', 'running', 'succeeded', 'failed', 'cancelled'];
const backupPhases = { '': '', opening: 'Reading the backup', checking: 'Checking the original wallet', restoring: 'Restoring a fresh wallet', syncing: 'Syncing the restored wallet', verifying: 'Comparing identity and funds', complete: 'Complete' };
const backupErrors = ['', 'backup_unavailable', 'backup_invalid', 'backup_mismatch', 'backup_storage_failed'];
const projectionStates = ['idle', 'loading', 'ready', 'failed', 'cancelled'];
const projectionErrors = ['', 'projection_unavailable', 'projection_corrupt', 'projection_cancelled'];
const statusErrors = new Set(['', 'unavailable', 'outcome_unknown', 'busy', 'capacity', 'rate_limited', 'expired', 'rejected', 'unknown_job', 'unauthorized', 'forbidden', 'invalid_request', 'too_large', 'signer_unavailable', 'clock', 'stale', 'observer_unavailable', 'storage_failed', 'storage_close_failed', 'chain_data_missing', 'node_shutdown_failed', 'node_start_failed', 'wallet_unavailable', 'wallet_shutdown_failed', 'wallet_connection_failed', 'wallet_payout_failed', 'wallet_identity_mismatch', 'wallet_status_failed', 'node_unavailable', 'authority_unavailable', 'authority_identity_mismatch', 'authority_port_busy', 'wallet_signer_unavailable', 'worker_unavailable', 'worker_failed', 'node_exited', 'authority_exited', 'initialization_incomplete', 'initialization_failed', ...projectionErrors]);
const fixtures = {
  zero256: ['a227587d837507dbcb5f9d781967339d816070fb7bd574931c690ba6937b88e5', '03ec7488667fd544b4a1de8fb0ae03730c9a2dc346c652cbaa59a077dbeddca519'],
  incrementing256: ['28452fc8a478d28f284dfe671cfc57e7cabeca08ea953aef371f5674d3f4c510', '033de5e2e3d5ad06f616572fcb3d61f089ce1105ef486debf308027b3b663d6d92'],
};
const number = new Intl.NumberFormat();
const coin = new Intl.NumberFormat(undefined, { maximumFractionDigits: 8 });
const date = new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' });
const cards = new Map();
const images = new Map();
const imageAttempts = new Set();
let current = null;
let enabled = false;
let disposed = false;
let command = '';
let activeCall = null;
let background = null;
let pollWanted = false;
let timer = null;
let epoch = 0;
let viewEpoch = 0;
let connectionError = '';
let actionMessage = '';

function requireValue(ok) { if (!ok) throw new Error('Invalid candidate pilot response'); }
function int(value, min = 0, max = Number.MAX_SAFE_INTEGER) { return Number.isSafeInteger(value) && value >= min && value <= max; }
function hex(value) { return typeof value === 'string' && /^[a-f0-9]{64}$/.test(value); }
function exact(value, keys) {
  requireValue(value !== null && typeof value === 'object' && !Array.isArray(value));
  const names = Object.keys(value);
  requireValue(names.length === keys.length && names.every((key) => keys.includes(key)));
}
function checkedView(v) {
  exact(v, ['version', 'view_id', 'account_authority_id', 'account_id', 'generation', 'authority_pin', 'authority_origin', 'transport_trust_id']);
  requireValue(v.version === 1 && ['view_id', 'account_authority_id', 'account_id', 'transport_trust_id'].every((key) => hex(v[key])) && int(v.generation, 1));
  exact(v.authority_pin, ['version', 'network', 'genesis', 'authority_id', 'descriptor_id']);
  const p = v.authority_pin;
  requireValue(p.version === 5 && p.network === 0x534c5547 && p.genesis === GENESIS && hex(p.authority_id) && p.descriptor_id === DESCRIPTOR);
  requireValue(typeof v.authority_origin === 'string' && /^http:\/\/127\.0\.0\.1:[1-9][0-9]{0,4}$/.test(v.authority_origin));
  requireValue(int(Number(v.authority_origin.split(':').pop()), 1, 65535));
  return { ...v, authority_pin: { ...p } };
}
function viewKey(v) {
  return v ? [v.version, v.view_id, v.account_authority_id, v.account_id, v.generation, v.authority_pin.version, v.authority_pin.network, v.authority_pin.genesis, v.authority_pin.authority_id, v.authority_pin.descriptor_id, v.authority_origin, v.transport_trust_id].join('|') : '';
}
function checkedItem(p) {
  exact(p, ['sequence', 'id', 'work_hash', 'born_at_ms', 'accepted_at_ms', 'name', 'physical', 'style', 'rarity', 'talent_budget', 'attributes', 'descriptor_id', 'manifest_digest', 'portrait_digest', 'eligibility_mode']);
  requireValue(int(p.sequence, 1, 1000) && ['id', 'work_hash', 'manifest_digest', 'portrait_digest'].every((key) => hex(p[key])) && p.descriptor_id === DESCRIPTOR);
  requireValue(int(p.born_at_ms, 1, DATE_MAX) && int(p.accepted_at_ms, p.born_at_ms, Math.min(DATE_MAX, p.born_at_ms + 299999)));
  exact(p.name, ['first', 'last', 'display']);
  requireValue(['first', 'last'].every((key) => typeof p.name[key] === 'string' && /^[A-Za-z][A-Za-z '\-]{0,63}$/.test(p.name[key])) && p.name.display === `${p.name.first} ${p.name.last}`);
  exact(p.physical, ['height_in', 'reach_in', 'stance', 'weight_lb', 'weight_class']);
  const q = p.physical;
  requireValue(int(q.height_in, 62, 80) && int(q.reach_in, q.height_in - 2, q.height_in + 4) && int(q.weight_lb, 128, 268) && ['orthodox', 'southpaw'].includes(q.stance) && classes.includes(q.weight_class));
  requireValue(q.weight_class === classes[q.weight_lb <= 145 ? 0 : q.weight_lb <= 170 ? 1 : q.weight_lb <= 195 ? 2 : q.weight_lb <= 225 ? 3 : 4]);
  exact(p.attributes, attrs);
  requireValue(attrs.every((key) => int(p.attributes[key], 1, 100)) && int(p.talent_budget, 5, 500) && attrs.reduce((sum, key) => sum + p.attributes[key], 0) === p.talent_budget);
  requireValue(styles.includes(p.style) && rarity.includes(p.rarity) && ['current-parent', 'canonical-winning-block'].includes(p.eligibility_mode));
  return { ...p, name: { ...p.name }, physical: { ...q }, attributes: { ...p.attributes } };
}
function normalize(v) {
  exact(v, ['version', 'mode', 'state', 'attempts', 'hashes_per_second', 'height', 'pending', 'last_error', 'recovery_error', 'wallet_state', 'wallet_height', 'payout_address', 'gym_id', 'gym_public_key', 'spendable_atoms', 'immature_atoms', 'backup_state', 'backup_phase', 'backup_error', 'account', 'view', 'accepted_count', 'projection']);
  requireValue(v.version === 1 && v.mode === MODE && states.includes(v.state) && walletStates.includes(v.wallet_state));
  requireValue(['attempts', 'height', 'wallet_height', 'spendable_atoms', 'immature_atoms'].every((key) => int(v[key])) && int(v.pending, 0, 1000) && int(v.accepted_count, 0, 1000));
  requireValue(typeof v.hashes_per_second === 'number' && Number.isFinite(v.hashes_per_second) && v.hashes_per_second >= 0 && statusErrors.has(v.last_error) && statusErrors.has(v.recovery_error));
  requireValue(backupStates.includes(v.backup_state) && Object.hasOwn(backupPhases, v.backup_phase) && backupErrors.includes(v.backup_error));
  exact(v.account, ['state', 'fixture', 'id', 'generation', 'hold_until_ms', 'held', 'can_export', 'last_error']);
  const a = v.account;
  requireValue(accountStates.includes(a.state) && ['', ...Object.keys(fixtures)].includes(a.fixture) && ['', 'account_unavailable'].includes(a.last_error) && int(a.generation) && int(a.hold_until_ms, 0, DATE_MAX) && typeof a.held === 'boolean' && typeof a.can_export === 'boolean' && (!a.held || (!a.can_export && a.hold_until_ms > 0)));
  exact(v.projection, ['state', 'error', 'revision', 'through_sequence', 'has_older', 'complete_recent', 'items']);
  const p = v.projection;
  requireValue(projectionStates.includes(p.state) && projectionErrors.includes(p.error) && int(p.revision) && int(p.through_sequence, 0, 1000) && typeof p.has_older === 'boolean' && typeof p.complete_recent === 'boolean' && Array.isArray(p.items) && p.items.length <= 8 && p.items.length <= v.accepted_count);
  requireValue((p.state === 'failed' ? ['projection_corrupt', 'projection_unavailable'].includes(p.error) : p.state === 'cancelled' ? ['', 'projection_cancelled'].includes(p.error) : p.error === ''));
  const items = p.items.map(checkedItem);
  const unique = new Set(); let previousSequence = 1001;
  for (const item of items) { requireValue(!unique.has(item.id) && item.sequence < previousSequence && item.sequence <= v.accepted_count); unique.add(item.id); previousSequence = item.sequence; }
  requireValue(p.through_sequence === (items[0]?.sequence ?? 0) && p.has_older === (v.accepted_count > items.length));
  const complete = p.state === 'ready' && items.length === Math.min(8, v.accepted_count) && items.every((item, i) => item.sequence === v.accepted_count - i);
  requireValue(p.complete_recent === (v.view !== null && complete) && (p.state === 'ready' || items.length === 0));
  let view = null;
  if (a.state === 'signed_in') {
    view = checkedView(v.view);
    requireValue(hex(a.id) && a.generation > 0 && view.account_id === a.id && view.generation === a.generation && Object.hasOwn(fixtures, a.fixture));
    requireValue(v.gym_id === fixtures[a.fixture][0] && v.gym_public_key === fixtures[a.fixture][1] && typeof v.payout_address === 'string' && /^[1-9A-HJ-NP-Za-km-z]{26,60}$/.test(v.payout_address));
  } else {
    requireValue(v.view === null && a.id === '' && a.generation === 0 && a.hold_until_ms === 0 && !a.held && !a.can_export && v.gym_id === '' && v.gym_public_key === '' && v.payout_address === '' && ['attempts', 'height', 'wallet_height', 'pending', 'spendable_atoms', 'immature_atoms', 'accepted_count', 'hashes_per_second'].every((key) => v[key] === 0) && items.length === 0 && p.revision === 0 && !p.complete_recent);
  }
  return { ...v, account: { ...a }, view, projection: { ...p, items } };
}
function content(node, value) { const next = String(value); if (node.textContent !== next) node.textContent = next; }
function element(tag, className = '', value) { const n = document.createElement(tag); n.className = className; if (value !== undefined) n.textContent = value; return n; }
function label(value) { return value.replaceAll('_', ' ').replace(/(^| )\w/g, (c) => c.toUpperCase()); }
function age(ms) { const seconds = Math.max(0, Math.floor((Date.now() - ms) / 1000)); return seconds < 60 ? 'Under 1m old' : seconds < 3600 ? `${Math.floor(seconds / 60)}m old` : seconds < 86400 ? `${Math.floor(seconds / 3600)}h old` : `${Math.floor(seconds / 86400)}d old`; }
function itemKey(p) { return `${p.id}|${p.sequence}|${p.manifest_digest}|${p.portrait_digest}`; }
function clearView() {
  viewEpoch++;
  for (const card of cards.values()) { card.image.removeAttribute('src'); card.article.remove(); }
  cards.clear(); images.clear(); imageAttempts.clear();
}
function makeCard(item) {
  const article = element('article', 'candidate'); article.setAttribute('aria-labelledby', `candidate-${item.id}`);
  const picture = element('div', 'portrait-area'); const image = element('img'); image.width = 200; image.height = 200; image.hidden = true; image.alt = `Portrait preview of ${item.name.display}`;
  const placeholder = element('p', 'portrait-placeholder', 'Portrait preview is loading…'); picture.append(image, placeholder);
  const body = element('div', 'card-body'); const meta = element('div', 'card-meta'); meta.append(element('span', '', label(item.rarity)), element('span', '', `#${item.sequence}`));
  const title = element('h3', '', item.name.display); title.id = `candidate-${item.id}`;
  const birth = element('p', 'birth'); birth.title = `Original birth: ${date.format(new Date(item.born_at_ms))}`;
  const physical = element('p', 'physical', `${label(item.physical.weight_class)} · ${item.physical.weight_lb} lb\n${item.physical.height_in} in tall · ${item.physical.reach_in} in reach\n${label(item.physical.stance)} · ${label(item.style)}`); physical.style.whiteSpace = 'pre-line';
  const talent = element('div', 'talent'); talent.append(element('span', '', 'Mined talent budget'), element('strong', '', item.talent_budget)); body.append(meta, title, birth, physical, talent);
  for (const key of attrs) { const track = element('span', 'track'); track.setAttribute('aria-hidden', 'true'); const fill = element('span'); fill.style.width = `${item.attributes[key]}%`; track.append(fill); const row = element('div', 'attribute'); row.append(element('span', '', label(key)), track, element('strong', '', item.attributes[key])); body.append(row); }
  const details = element('details'); details.append(element('summary', '', 'Candidate details'));
  const list = element('dl');
  for (const [term, value] of [['Original birth', date.format(new Date(item.born_at_ms))], ['Accepted', date.format(new Date(item.accepted_at_ms))], ['Prospect ID', item.id], ['Work hash', item.work_hash], ['Descriptor ID', item.descriptor_id], ['Manifest digest', item.manifest_digest], ['Portrait digest', item.portrait_digest], ['Historical eligibility', item.eligibility_mode === 'current-parent' ? 'Current parent at admission' : 'Canonical winning block at admission']]) { list.append(element('dt', '', term), element('dd', term === 'Historical eligibility' ? 'historical' : '', value)); }
  list.append(element('dt', '', 'History only'), element('dd', 'historical', 'This receipt does not establish current chain status or ownership.')); details.append(list);
  const actions = element('div', 'card-actions'); const retry = element('button', '', 'Reload portrait'); retry.type = 'button'; retry.setAttribute('aria-label', `Reload portrait of ${item.name.display}`);
  const save = element('button', '', 'Export public receipt'); save.type = 'button'; save.setAttribute('aria-label', `Export public receipt for ${item.name.display}`);
  retry.addEventListener('click', () => { if (command || !current?.view) return; imageAttempts.delete(itemKey(item)); images.delete(itemKey(item)); image.removeAttribute('src'); image.hidden = true; placeholder.hidden = false; content(placeholder, 'Portrait preview is loading…'); pump(); });
  save.addEventListener('click', () => action('ExportManifestReceipt', [item.sequence], item.id));
  actions.append(retry, save); article.append(picture, body, details, actions);
  return { article, image, placeholder, birth, retry, save, item, fingerprint: JSON.stringify(item) };
}
function renderCards() {
  const items = current?.projection.items ?? []; const present = new Set(items.map((p) => p.id));
  for (const [id, c] of cards) if (!present.has(id)) { c.image.removeAttribute('src'); c.article.remove(); cards.delete(id); images.delete(itemKey(c.item)); imageAttempts.delete(itemKey(c.item)); }
  items.forEach((p, index) => {
    let c = cards.get(p.id); if (!c) { c = makeCard(p); cards.set(p.id, c); }
    content(c.birth, `${age(p.born_at_ms)} · original birth`);
    if (ui.cards.children[index] !== c.article) ui.cards.insertBefore(c.article, ui.cards.children[index] ?? null);
    c.retry.disabled = !!command || !!connectionError; c.save.disabled = !!command || !!connectionError;
  });
  ui.empty.hidden = items.length > 0;
  content(ui['empty-copy'], !current?.view ? 'Sign in to load your demo gym, then start mining when you are ready.' : current.projection.state === 'loading' ? 'Checking saved candidates. You can stop this operation.' : current.projection.state === 'failed' ? 'Saved candidates could not be verified. Stop mining, then try reloading.' : current.accepted_count > 0 ? 'Some saved candidates are still syncing.' : 'Start mining when you are ready. New accepted candidates will appear here automatically.');
}
function render() {
  const s = current; const signed = !!s?.view; const active = s?.state === 'starting' || s?.state === 'running'; const blocked = !!command || !enabled || !!connectionError;
  content(ui.state, connectionError ? 'Connection interrupted' : command ? 'Working…' : s ? label(s.state) : 'Connecting');
  content(ui['account-state'], s ? label(s.account.state) : 'Connecting');
  content(ui['account-copy'], signed ? `${s.account.fixture === 'zero256' ? 'Demo A' : 'Demo B'} is signed in. This demo instance retains the same wallet and gym identity.` : s && ['signing_in', 'resetting'].includes(s.account.state) ? 'Preparing the same demo wallet and checking saved candidates…' : 'Choose a demo account to load its saved candidates. Mining starts only when you choose Start.');
  const transient = s && ['signing_in', 'signing_out', 'resetting'].includes(s.account.state);
  ui['login-a'].disabled = ui['login-b'].disabled = blocked || !s || signed || transient;
  ui.logout.disabled = blocked || !s || (!signed && !['failed', 'expired'].includes(s.account.state));
  ui.reset.disabled = blocked || !signed;
  content(ui.hold, signed && s.account.held ? `Security hold until ${date.format(new Date(s.account.hold_until_ms))}. Viewing, mining and public receipt export remain available. Recovery backup export is paused.` : '');
  const loading = s?.projection.state === 'loading';
  const preparing = s && ['signing_in', 'resetting'].includes(s.account.state);
  content(ui.mining, command === 'Stop' ? 'Stopping…' : active ? 'Stop mining' : preparing ? 'Stop preparing' : loading ? 'Stop loading' : 'Start mining');
  ui.mining.disabled = blocked || (!signed && !preparing) || s?.state === 'stopping' || s?.backup_state === 'running'; ui.mining.setAttribute('aria-pressed', String(!!active)); ui.mining.setAttribute('aria-busy', String(!!command || s?.state === 'stopping'));
  content(ui['mining-copy'], preparing ? 'Preparing your retained demo wallet and checking saved candidates. Stopping drains any initialization already in progress.' : !signed ? 'Sign in to prepare your local gym.' : loading ? 'Checking saved history before more mining. No mining jobs are created by this check.' : active ? 'Mining is active. New accepted candidates and portraits appear automatically.' : 'Start when you are ready. Saved pending work is retried automatically.');
  content(ui.attempts, number.format(s?.attempts ?? 0)); content(ui.accepted, number.format(s?.accepted_count ?? 0)); content(ui.pending, number.format(s?.pending ?? 0)); content(ui.height, number.format(s?.height ?? 0));
  content(ui.rate, signed && s.hashes_per_second > 0 ? `${s.hashes_per_second.toFixed(2)} hashes / second` : 'Not mining');
  content(ui['wallet-state'], s ? `Wallet ${s.wallet_state}` : 'Wallet closed'); content(ui.spendable, signed ? coin.format(s.spendable_atoms / 100000000) : '—'); content(ui.immature, signed ? coin.format(s.immature_atoms / 100000000) : '—');
  content(ui['wallet-height'], signed ? `Wallet height ${number.format(s.wallet_height)}. Rewards mature after 100 blocks.` : 'Mining rewards mature after 100 blocks.'); content(ui.payout, s?.payout_address || 'Sign in to load the demo wallet'); content(ui['gym-id'], s?.gym_id || '—');
  const idle = signed && ['stopped', 'failed'].includes(s.state) && !loading && s.backup_state !== 'running';
  ui['export-recovery'].disabled = blocked || !idle || !s.account.can_export; ui['verify-backup'].disabled = blocked || !idle;
  ui.refresh.disabled = blocked || !idle;
  content(ui.backup, s?.backup_state === 'running' ? `${backupPhases[s.backup_phase] || 'Checking the backup'}…` : s?.backup_state === 'succeeded' ? 'Backup verification completed.' : s?.backup_state === 'failed' ? 'Backup verification failed. The original wallet has been retained.' : s?.backup_state === 'cancelled' ? 'Backup verification was cancelled.' : 'Stop mining to save or verify a demo wallet backup.');
  content(ui.coverage, !signed ? 'Saved candidates load when you sign in. New accepted candidates appear automatically while mining.' : loading ? 'Checking saved candidates…' : !s.projection.complete_recent ? 'Some saved candidates are still syncing.' : s.accepted_count > 8 ? `Showing the latest 8 of ${number.format(s.accepted_count)} accepted candidates. Earlier receipts remain saved.` : `${number.format(s.accepted_count)} accepted ${s.accepted_count === 1 ? 'candidate' : 'candidates'}.`);
  const backend = s?.projection.error ? 'Saved candidates could not be loaded. You can retry while stopped.' : s?.last_error || s?.recovery_error ? (s.last_error === 'capacity' || s.recovery_error === 'capacity' ? 'The local store is full. Saved work has been retained; mining is paused.' : 'The local gym needs attention. Saved work is retained; try stopping and starting again.') : '';
  const message = connectionError || actionMessage || backend; content(ui.alert, message); ui.alert.hidden = !message;
  renderCards();
  document.title = 'SLUG Candidate Pilot';
}
function applySnapshot(next) {
  if (current?.view && next.view && current.view.view_id === next.view.view_id) requireValue(viewKey(current.view) === viewKey(next.view));
  if (viewKey(current?.view) !== viewKey(next.view)) clearView();
  else if (next.view && current) {
    requireValue(next.accepted_count >= current.accepted_count && next.projection.revision >= current.projection.revision);
    if (current.projection.state === 'ready' && next.projection.state === 'ready' && current.projection.revision === next.projection.revision) requireValue(JSON.stringify(current.projection.items) === JSON.stringify(next.projection.items));
    for (const item of next.projection.items) { const c = cards.get(item.id); if (c) requireValue(c.fingerprint === JSON.stringify(item)); }
  }
  const oldCount = current?.accepted_count ?? 0; current = next; connectionError = '';
  if (next.view && next.accepted_count !== oldCount) content(ui.announcement, `${next.accepted_count} candidates accepted in this demo gym.`);
  render();
}
function failConnection() { current = null; clearView(); connectionError = 'The local candidate gym returned no usable update. Retrying…'; render(); }
async function call(method, ...args) {
  requireValue(!activeCall && !disposed);
  const pending = Promise.resolve().then(() => Call.ByName(`main.Gym.${method}`, ...args)); activeCall = pending;
  try { return await pending; } finally { if (activeCall === pending) activeCall = null; }
}
async function fetchSnapshot() {
  const captured = epoch;
  try { const value = normalize(await call('ManifestSnapshot')); if (!disposed && captured === epoch) applySnapshot(value); }
  catch { if (!disposed && captured === epoch) failConnection(); }
}
// Defense in depth for image-only markup; native code verifies the exact SVG
// domain digest before it reaches this page. No browser crypto dependency.
function imageOnlySVG(svg) {
  if (typeof svg !== 'string' || svg.length < 1 || svg.length > 32768 || !/^[\x20-\x7e]+$/.test(svg) || !svg.startsWith('<svg xmlns="http://www.w3.org/2000/svg" ') || !svg.endsWith('</svg>') || /&(?!(?:amp|lt|gt|quot|apos);)|<\s*[!?]|url\s*\(/i.test(svg)) return false;
  const allowed = {
    svg: 'xmlns width height viewBox preserveAspectRatio overflow role aria-label data-renderer', title: '', desc: '',
    rect: 'x y width height rx fill stroke stroke-width stroke-linecap opacity',
    ellipse: 'cx cy rx ry fill stroke stroke-width stroke-linecap opacity transform',
    circle: 'cx cy r fill stroke stroke-width stroke-linecap opacity',
    path: 'd fill stroke stroke-width stroke-linecap opacity',
  };
  let end = 0; let count = 0; const stack = [];
  for (const match of svg.matchAll(/<(\/?)([A-Za-z]+)([^>]*)>/g)) {
    if (++count > 1024 || (match.index !== end && !['title', 'desc'].includes(stack.at(-1))) || svg.slice(end, match.index).includes('<') || !Object.hasOwn(allowed, match[2])) return false;
    const [, close, name, tail] = match; end = match.index + match[0].length;
    if (close) { if (tail !== '' || stack.pop() !== name) return false; continue; }
    if (name === 'svg' ? stack.length !== 0 || match.index !== 0 : stack.length !== 1 || stack[0] !== 'svg') return false;
    const selfClosing = tail.endsWith('/'); const body = selfClosing ? tail.slice(0, -1) : tail; let used = 0; const seen = new Set();
    for (const a of body.matchAll(/ ([A-Za-z][A-Za-z-]*)="([^"<>]*)"/g)) {
      if (a.index !== used || seen.has(a[1]) || !allowed[name].split(' ').includes(a[1])) return false;
      seen.add(a[1]); used += a[0].length;
      if (a[1] === 'data-renderer' && a[2] !== 'slug-fighter-portrait-candidate-v1') return false;
    }
    if (used !== body.length || (name === 'svg' && !seen.has('data-renderer'))) return false;
    if (!selfClosing) stack.push(name);
  }
  return end === svg.length && stack.length === 0;
}
async function portrait(item) {
  const key = itemKey(item); const pin = viewKey(current?.view); const capturedEpoch = epoch; const capturedView = viewEpoch; imageAttempts.add(key);
  const remains = () => !disposed && capturedEpoch === epoch && capturedView === viewEpoch && pin === viewKey(current?.view) && cards.get(item.id)?.fingerprint === JSON.stringify(item);
  try {
    const p = await call('ManifestPortrait', item.sequence);
    if (!remains()) return;
    exact(p, ['version', 'view', 'sequence', 'id', 'manifest_digest', 'portrait_digest', 'svg']);
    requireValue(p.version === 1 && viewKey(checkedView(p.view)) === pin && p.sequence === item.sequence && p.id === item.id && p.manifest_digest === item.manifest_digest && p.portrait_digest === item.portrait_digest);
    requireValue(imageOnlySVG(p.svg));
    if (!remains()) return;
    const source = `data:image/svg+xml;base64,${btoa(p.svg)}`;
    const total = [...images.entries()].reduce((sum, [k, value]) => sum + (k === key ? 0 : value.length), source.length) + JSON.stringify(current.projection.items).length;
    requireValue(total <= CACHE_MAX && images.size <= 8);
    images.set(key, source); const c = cards.get(item.id); c.image.setAttribute('src', source); c.image.hidden = false; c.placeholder.hidden = true;
  } catch {
    if (remains()) { const c = cards.get(item.id); c.image.removeAttribute('src'); c.image.hidden = true; c.placeholder.hidden = false; content(c.placeholder, 'Portrait unavailable. Use Reload portrait to try again.'); }
  } finally { if (!remains()) imageAttempts.delete(key); }
}
function schedulePoll() { if (timer !== null) clearTimeout(timer); timer = null; if (!disposed && enabled && !document.hidden) timer = setTimeout(() => { timer = null; pollWanted = true; pump(); }, 1000); }
function pump() {
  if (background || disposed || !enabled || command || document.hidden) return background;
  background = (async () => {
    while (!disposed && enabled && !command && !document.hidden) {
      if (pollWanted) { pollWanted = false; await fetchSnapshot(); schedulePoll(); continue; }
      const next = current?.view && !connectionError ? current.projection.items.find((p) => !images.has(itemKey(p)) && !imageAttempts.has(itemKey(p))) : null;
      if (!next) break;
      await portrait(next);
    }
  })().finally(() => { background = null; if (pollWanted && !command && !disposed && !document.hidden) pump(); });
  return background;
}
async function action(method, args = [], expectedID = '') {
  if (command || !enabled || disposed || connectionError) return;
  const selected = viewKey(current?.view);
  if (expectedID && !current?.projection.items.some((p) => p.id === expectedID && p.sequence === args[0])) return;
  command = method; epoch++; actionMessage = '';
  if (['LoginFixture', 'Logout', 'ResetFixtureAccount'].includes(method)) { current = null; clearView(); }
  render();
  let result; let failed = false;
  try {
    if (background) await background;
    if (disposed) return;
    result = await call(method, ...args);
    const dialogResults = { ExportManifestReceipt: ['', 'Receipt saved.'], ExportRecovery: ['', 'Test wallet recovery file saved.'], VerifyBackup: ['', 'Backup verification started.'] };
    if (Object.hasOwn(dialogResults, method)) requireValue(dialogResults[method].includes(result));
  } catch { failed = true; }
  if (disposed) return;
  // A native dialog may outlive its view. Read current state before applying any result.
  await fetchSnapshot();
  const sameView = !selected || selected === viewKey(current?.view);
  if (sameView || ['LoginFixture', 'Logout', 'ResetFixtureAccount'].includes(method)) {
    if (failed) actionMessage = method === 'ExportManifestReceipt' ? 'Could not save the public receipt. Try again from the current account.' : 'The requested action did not complete. Check the local gym state and try again.';
    else if (method === 'ExportManifestReceipt' && result !== '') actionMessage = 'Receipt saved.';
    else if (method === 'ExportRecovery' && result !== '') actionMessage = 'Recovery backup saved.';
    else if (method === 'VerifyBackup' && result !== '') actionMessage = 'Backup verification started.';
  }
  command = ''; render(); pollWanted = false; schedulePoll(); pump();
}
ui['login-a'].addEventListener('click', () => action('LoginFixture', ['zero256']));
ui['login-b'].addEventListener('click', () => action('LoginFixture', ['incrementing256']));
ui.logout.addEventListener('click', () => action('Logout'));
ui.reset.addEventListener('click', () => action('ResetFixtureAccount'));
ui.mining.addEventListener('click', () => action(['starting', 'running'].includes(current?.state) || ['signing_in', 'resetting'].includes(current?.account.state) || current?.projection.state === 'loading' ? 'Stop' : 'Start'));
ui.refresh.addEventListener('click', () => action('RefreshManifestProspects'));
ui['export-recovery'].addEventListener('click', () => action('ExportRecovery'));
ui['verify-backup'].addEventListener('click', () => action('VerifyBackup'));
document.addEventListener('visibilitychange', () => { if (document.hidden) { if (timer !== null) clearTimeout(timer); timer = null; } else { pollWanted = true; pump(); } });
window.addEventListener('pagehide', () => { disposed = true; epoch++; if (timer !== null) clearTimeout(timer); timer = null; clearView(); });
async function initialize() {
  render();
  try { requireValue(await call('Mode') === MODE); if (disposed) return; enabled = true; pollWanted = true; pump(); }
  catch { if (!disposed) { connectionError = 'This page requires the local candidate pilot. Close it and reopen the correct mode.'; render(); } }
}
initialize();
