import { Call } from '/wails/runtime.js';

// Separate stable entry: only typed, current-view, candidate-only native data.
const MODE = 'local-manifest-pilot';
const DESCRIPTOR = 'c1a8650a39f695814db67fddf79815ef249a13a5c0cc5af0b906b061c4c3a303';
const GENESIS = '7893676099cf9985bb198dfb6a347d9422a19f2422cf3326a999a7230ce246cd';
const DATE_MAX = 8640000000000000;
const CACHE_MAX = 1 << 20;
const ids = ['state', 'account-state', 'account-copy', 'hold', 'login-a', 'login-b', 'logout', 'reset', 'mining', 'mining-copy', 'alert', 'attempts', 'rate', 'accepted', 'pending', 'height', 'wallet-state', 'wallet-height', 'spendable', 'immature', 'payout', 'gym-id', 'export-recovery', 'verify-backup', 'backup', 'refresh', 'coverage', 'announcement', 'empty', 'empty-copy', 'cards', 'candidates-heading', 'stable-state', 'stable-count', 'stable-progress', 'sync', 'pause-sync', 'recent', 'inventory', 'roster-details', 'previous', 'next', 'page-label', 'roster-slots', 'roster-count', 'mutation-state', 'reconcile', 'apply-saved', 'abandon', 'cancel-job'];
ids.push('preview-state', 'preview-selection', 'preview-begin', 'preview-cancel', 'preview-clear', 'preview-message', 'preview-replay', 'preview-asof', 'preview-fighters', 'preview-play', 'preview-previous', 'preview-next', 'preview-restart', 'preview-result', 'preview-motion', 'preview-step', 'preview-events', 'preview-outcome', 'preview-finish', 'preview-scores');
ids.push('entry-state', 'entry-date', 'entry-upcoming', 'entry-status', 'entry-cutoff', 'entry-saved', 'entry-warning', 'entry-count', 'entry-draft', 'entry-save', 'entry-withdraw', 'entry-check', 'entry-clear', 'entry-cancel', 'entry-message', 'entry-pending', 'entry-reconcile', 'entry-apply', 'entry-abandon', 'entry-eligibility', 'entry-history', 'entry-history-next', 'entry-history-list');
ids.push('event-state', 'event-cutoff', 'event-progress', 'event-refresh', 'event-capture', 'event-cancel', 'event-record', 'event-clear', 'event-message', 'event-seed', 'event-deadline', 'event-digest', 'event-members', 'event-history', 'event-history-next', 'event-history-list');
ids.push('seed-state', 'seed-refresh', 'seed-prepare', 'seed-publish', 'seed-reconcile', 'seed-attest', 'seed-cancel', 'seed-record', 'seed-clear', 'seed-history', 'seed-history-next', 'seed-message', 'seed-observation', 'seed-finality', 'seed-incident', 'seed-identity', 'seed-history-list');
ids.push('card-state', 'card-refresh', 'card-begin', 'card-cancel', 'card-record', 'card-clear', 'card-history', 'card-history-next', 'card-message', 'card-totals', 'card-source', 'card-conflict', 'card-incident', 'card-evidence', 'card-gyms', 'card-bouts', 'card-unmatched', 'card-reasons', 'card-history-list', 'card-detail-hint');
ids.push('bout-state', 'bout-refresh', 'bout-list', 'bout-empty', 'bout-run', 'bout-open', 'bout-check', 'bout-cancel', 'bout-clear', 'bout-message', 'bout-source', 'bout-overlay', 'bout-replay', 'bout-asof', 'bout-fighters', 'bout-play', 'bout-previous', 'bout-next', 'bout-restart', 'bout-result', 'bout-motion', 'bout-step', 'bout-events', 'bout-outcome', 'bout-finish', 'bout-scores', 'bout-evidence');
ids.push('result-panel', 'result-state', 'result-connection', 'result-observation', 'result-read', 'result-publish', 'result-reconcile', 'result-attest', 'result-export', 'result-cancel', 'result-disconnect', 'result-clear', 'result-message', 'result-finality', 'result-conflict', 'result-exported', 'result-evidence');
ids.push('history-panel', 'history-state', 'history-fighter', 'history-asof', 'history-totals', 'history-message', 'history-list', 'history-previous', 'history-next', 'history-page-label', 'history-refresh', 'history-cancel', 'history-clear', 'history-result');
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

function requireValue(ok) { if (!ok) throw new Error('Invalid candidate pilot response'); }
function int(value, min = 0, max = Number.MAX_SAFE_INTEGER) { return Number.isSafeInteger(value) && !Object.is(value, -0) && value >= min && value <= max; }
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
  requireValue(p.version === 5 && p.network === 0x534c5547 && p.genesis === GENESIS && hex(p.authority_id) && p.descriptor_id === DESCRIPTOR && p.authority_id !== v.account_authority_id);
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


const ROOTS = {
  zero256: 'JgduaQiYjRcnjcD9t1MfTP1qo8eWaAWPQtS9nChXwtAeyTsoEGVZ8SjVqa5qxxvvKqX8tqC77wMSjaMHXNY8JSYEASdzX6PT31ktXqKPJvkhyy31',
  incrementing256: 'JgduaQiYjRcnjcD9syNDf4x3txTZkg8HpvTKZtposTTw7bTLpRzvd7ibV3buRixMEqAwVAyreX36pAw5DxLXwG5gW6EGNWeRcJfJitkx2kCJ54pC',
};
const STABLE_ERRORS = new Set(['invalid_request', 'unauthorized', 'forbidden', 'stale_view', 'busy', 'stable_unavailable', 'stable_corrupt', 'stable_capacity', 'stable_clock', 'stable_sync_required', 'stable_conflict', 'stable_pending', 'stable_not_found', 'stable_cancelled', 'outcome_unknown']);
const WEEK = 604800000;
const own = (v) => JSON.parse(JSON.stringify(v));
const same = (a, b) => JSON.stringify(a) === JSON.stringify(b);
const optionalID = (v) => v === '' || hex(v);
const number = new Intl.NumberFormat();
const coin = new Intl.NumberFormat(undefined, { maximumFractionDigits: 8 });
const date = new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' });
function slots(v) {
  requireValue(Array.isArray(v) && v.length === 10 && v.every(optionalID));
  const selected = v.filter(Boolean); requireValue(new Set(selected).size === selected.length); return [...v];
}
function status(v, view, fixture) {
  exact(v, ['version', 'store_id', 'pin', 'checkpoint', 'observed_source_count', 'caught_up', 'roster_revision', 'slots', 'operation_count', 'pending_operation_id', 'last_operation_id', 'last_as_of_ms']);
  requireValue(v.version === 1 && hex(v.store_id) && int(v.checkpoint, 0, 1000) && int(v.observed_source_count, v.checkpoint, 1000) && v.caught_up === (v.checkpoint === v.observed_source_count) && int(v.operation_count, 0, 4096) && int(v.roster_revision, 0, v.operation_count) && int(v.last_as_of_ms, 0, DATE_MAX));
  requireValue(optionalID(v.pending_operation_id) && optionalID(v.last_operation_id) && ((v.operation_count === 0) === (v.last_operation_id === '')) && (!v.pending_operation_id || v.pending_operation_id === v.last_operation_id));
  slots(v.slots); requireValue(v.slots.filter(Boolean).length <= v.checkpoint);
  const p = v.pin; exact(p, ['version', 'account_authority_id', 'account_id', 'identity', 'source', 'condition_version', 'projector_id']);
  requireValue(p.version === 1 && p.account_authority_id === view.account_authority_id && p.account_id === view.account_id && p.condition_version === 1 && p.projector_id === 'slug-fighter-condition-candidate-v1');
  exact(p.source, ['version', 'network', 'genesis', 'authority_id', 'descriptor_id']); requireValue(same(p.source, view.authority_pin));
  exact(p.identity, ['network', 'genesis', 'profile', 'root_public_key', 'gym_public_key', 'gym_id']);
  requireValue(Object.hasOwn(ROOTS, fixture) && p.identity.network === 0x534c5547 && p.identity.genesis === GENESIS && p.identity.profile === 'slug-phase-a-fixture-bip39-v1' && p.identity.root_public_key === ROOTS[fixture] && p.identity.gym_id === fixtures[fixture][0] && p.identity.gym_public_key === fixtures[fixture][1]);
  return own(v); // Native validates the configuration digest; no browser crypto authority.
}
function stableSnapshot(v, manifest) {
  exact(v, ['version', 'state', 'error', 'view', 'status', 'active_job_id', 'queued_job_id', 'last_job_id', 'pending_operation_id']);
  requireValue(v.version === 1 && ['unavailable', 'syncing', 'ready', 'paused', 'failed'].includes(v.state) && (v.error === '' || STABLE_ERRORS.has(v.error)) && ['active_job_id', 'queued_job_id', 'last_job_id', 'pending_operation_id'].every((k) => optionalID(v[k])));
  if (v.view === null || v.status === null) {
    requireValue(v.view === null && v.status === null && v.state === 'unavailable' && ['active_job_id', 'queued_job_id', 'last_job_id', 'pending_operation_id'].every((k) => v[k] === ''));
  } else {
    requireValue(v.state !== 'unavailable');
    const view = checkedView(v.view); requireValue(manifest?.view && viewKey(view) === viewKey(manifest.view));
    status(v.status, view, manifest.account.fixture);
    requireValue(v.pending_operation_id === v.status.pending_operation_id && (v.state !== 'ready' || v.status.caught_up));
    requireValue(!v.active_job_id || v.active_job_id !== v.queued_job_id);
  }
  return own(v);
}
function assignment(v, identity) {
  exact(v, ['version', 'sequence', 'prospect_id', 'gym_id', 'work_hash', 'born_at_ms', 'accepted_at_ms', 'descriptor_id', 'manifest_digest', 'portrait_digest', 'source_record_digest']);
  requireValue(v.version === 1 && int(v.sequence, 1, 1000) && ['prospect_id', 'gym_id', 'work_hash', 'manifest_digest', 'portrait_digest', 'source_record_digest'].every((k) => hex(v[k])) && v.gym_id === identity.gym_id && v.descriptor_id === DESCRIPTOR && int(v.born_at_ms, 1, DATE_MAX) && int(v.accepted_at_ms, v.born_at_ms, Math.min(DATE_MAX, v.born_at_ms + 299999)));
}
function manifestValue(m, seed, boundedASCIIName = false) {
  exact(m, ['manifest_version', 'generator_id', 'table_hash', 'dna', 'name', 'physical', 'style', 'portrait']);
  requireValue(m.manifest_version === 1 && m.generator_id === 'slug-fighter-manifest-candidate-v1' && m.table_hash === '72231fbe707cea9a65dd5b92f7b2719953795bceadbbf9e3b8e0b8f4aaef3f44');
  const d = m.dna; exact(d, ['dna_version', 'seed', 'rarity', 'talent_budget', 'attributes', 'prime_weeks', 'decline_weeks']);
  requireValue(d.dna_version === 1 && d.seed === seed && rarity.includes(d.rarity) && int(d.talent_budget, 250, 489) && int(d.prime_weeks, 26, 52) && int(d.decline_weeks, 13, 39));
  exact(d.attributes, attrs); requireValue(attrs.every((k) => int(d.attributes[k], 1, 100)) && attrs.reduce((n, k) => n + d.attributes[k], 0) === d.talent_budget);
  const limits = [[250, 299], [300, 349], [350, 399], [400, 449], [450, 489]][rarity.indexOf(d.rarity)]; requireValue(int(d.talent_budget, ...limits));
  exact(m.name, ['first', 'last', 'display']); requireValue(['first', 'last'].every((k) => typeof m.name[k] === 'string' && (boundedASCIIName ? /^[\x00-\x7f]{1,64}$/.test(m.name[k]) : /^[A-Za-z][A-Za-z '\-]{0,63}$/.test(m.name[k]))) && m.name.display === `${m.name.first} ${m.name.last}`);
  exact(m.physical, ['height_in', 'reach_in', 'stance', 'weight_lb', 'weight_class']); const p = m.physical;
  requireValue(int(p.height_in, 62, 80) && int(p.reach_in, p.height_in - 2, p.height_in + 4) && int(p.weight_lb, 128, 268) && ['orthodox', 'southpaw'].includes(p.stance) && p.weight_class === classes[p.weight_lb <= 145 ? 0 : p.weight_lb <= 170 ? 1 : p.weight_lb <= 195 ? 2 : p.weight_lb <= 225 ? 3 : 4] && styles.includes(m.style));
  const r = m.portrait; exact(r, ['portrait_version', 'skin', 'hair_style', 'hair_color', 'facial_hair', 'head_shape', 'mouth', 'brow', 'background', 'scar', 'shoulder_jitter']);
  requireValue(r.portrait_version === 1 && int(r.skin, 0, 5) && int(r.hair_style, 0, 7) && int(r.hair_color, 0, 5) && int(r.facial_hair, 0, 4) && int(r.head_shape, 0, 3) && int(r.mouth, 0, 3) && int(r.brow, 0, 2) && int(r.background, 0, 7) && ['none', 'left_brow', 'right_cheek'].includes(r.scar) && int(r.shoulder_jitter, -6, 6) && !Object.is(r.shoulder_jitter, -0));
}
function halfEven(n, d) { const q = Math.floor(n / d), r = n % d; return q + (2 * r > d || (2 * r === d && q % 2 === 1) ? 1 : 0); }
function condition(c, item, asOf) {
  exact(c, ['version', 'projector_id', 'dna_version', 'seed', 'born_at_ms', 'as_of_ms', 'age_weeks', 'phase', 'elapsed_decline_weeks', 'prime_remaining_weeks', 'prime_remaining_ms', 'career_remaining_ms', 'pro_xp', 'aged_attributes_tenths', 'league_attributes_tenths', 'rating_hundredths']);
  const d = item.manifest.dna, a = item.assignment;
  requireValue(c.version === 1 && c.projector_id === 'slug-fighter-condition-candidate-v1' && c.dna_version === 1 && c.seed === d.seed && c.born_at_ms === a.born_at_ms && c.as_of_ms === asOf && int(asOf, a.accepted_at_ms, DATE_MAX) && c.pro_xp === 0);
  const ageWeeks = Math.floor((asOf - c.born_at_ms) / WEEK), step = Math.min(d.decline_weeks, Math.max(0, ageWeeks - d.prime_weeks));
  const phase = ageWeeks < d.prime_weeks ? 'prime' : ageWeeks < d.prime_weeks + d.decline_weeks ? 'decline' : 'retired';
  requireValue(c.age_weeks === ageWeeks && c.phase === phase && c.elapsed_decline_weeks === step && c.prime_remaining_weeks === Math.max(0, d.prime_weeks - ageWeeks) && c.prime_remaining_ms === Math.max(0, d.prime_weeks * WEEK - (asOf - c.born_at_ms)) && c.career_remaining_ms === Math.max(0, (d.prime_weeks + d.decline_weeks) * WEEK - (asOf - c.born_at_ms)));
  exact(c.aged_attributes_tenths, attrs); exact(c.league_attributes_tenths, attrs);
  for (const [i, k] of attrs.entries()) { const wanted = halfEven(d.attributes[k] * (100 * d.decline_weeks - [20, 40, 30, 30, 10][i] * step), 10 * d.decline_weeks); requireValue(c.aged_attributes_tenths[k] === wanted && c.league_attributes_tenths[k] === wanted); }
  requireValue(c.rating_hundredths === attrs.reduce((sum, k, i) => sum + c.league_attributes_tenths[k] * [10, 25, 2, 4, 8][i], 0));
}
function fullItem(i, st, asOf) { exact(i, ['assignment', 'manifest', 'condition']); assignment(i.assignment, st.pin.identity); manifestValue(i.manifest, i.assignment.work_hash); condition(i.condition, i, asOf); requireValue(i.assignment.sequence <= st.checkpoint && JSON.stringify(i).length <= 8192); return own(i); }
function request(r) { exact(r, ['expected_revision', 'slot', 'prospect_id']); requireValue(int(r.expected_revision, 0, 4096) && int(r.slot, 1, 10) && optionalID(r.prospect_id)); }
function intent(v) { exact(v, ['version', 'sequence', 'operation_id', 'generation', 'view_id', 'request', 'prepared_at_ms']); requireValue(v.version === 1 && int(v.sequence, 1, 4096) && hex(v.operation_id) && int(v.generation, 1) && hex(v.view_id) && int(v.prepared_at_ms, 1, DATE_MAX)); request(v.request); return own(v); }
function receipt(v) {
  exact(v, ['version', 'operation_id', 'generation', 'view_id', 'request', 'previous_prospect_id', 'changed', 'previous_revision', 'revision', 'slots', 'committed_at_ms']);
  requireValue(v.version === 1 && hex(v.operation_id) && int(v.generation, 1) && hex(v.view_id) && optionalID(v.previous_prospect_id) && typeof v.changed === 'boolean' && int(v.previous_revision, 0, 4096) && int(v.revision, 0, 4096) && v.revision === v.previous_revision + (v.changed ? 1 : 0) && int(v.committed_at_ms, 1, DATE_MAX));
  request(v.request); slots(v.slots); requireValue(v.request.expected_revision === v.previous_revision && v.slots[v.request.slot - 1] === v.request.prospect_id && v.changed === (v.previous_prospect_id !== v.request.prospect_id)); return own(v);
}
function operation(v) {
  exact(v, ['version', 'intent', 'state', 'receipt', 'abandoned_at_ms']); requireValue(v.version === 1 && ['prepared', 'committed', 'abandoned'].includes(v.state)); const i = intent(v.intent);
  if (v.state === 'committed') { const r = receipt(v.receipt); requireValue(r.operation_id === i.operation_id && r.generation === i.generation && r.view_id === i.view_id && same(r.request, i.request) && r.committed_at_ms >= i.prepared_at_ms && v.abandoned_at_ms === 0); }
  else requireValue(v.receipt === null && (v.state === 'abandoned' ? int(v.abandoned_at_ms, i.prepared_at_ms, DATE_MAX) : v.abandoned_at_ms === 0));
  return own(v);
}
function pageValue(p, kind, view, fixture) {
  const keys = kind === 'sync' ? ['version', 'status', 'as_of_ms', 'items'] : kind === 'inventory' ? ['version', 'status', 'as_of_ms', 'after', 'next_after', 'has_more', 'items'] : ['version', 'status', 'as_of_ms', 'first_slot', 'next_slot', 'items'];
  exact(p, keys); requireValue(p.version === 1 && int(p.as_of_ms, 1, DATE_MAX) && Array.isArray(p.items) && p.items.length <= 8); const st = status(p.status, view, fixture); requireValue(st.last_as_of_ms === p.as_of_ms);
  const seen = new Set(); let last = 0;
  for (const row of p.items) {
    if (kind === 'roster') { exact(row, ['slot', 'item']); requireValue(int(row.slot, p.first_slot, 10) && row.slot > last); last = row.slot; }
    const i = fullItem(kind === 'roster' ? row.item : row, st, p.as_of_ms); requireValue(!seen.has(i.assignment.prospect_id)); seen.add(i.assignment.prospect_id);
    if (kind === 'roster') requireValue(st.slots[row.slot - 1] === i.assignment.prospect_id); else { requireValue(i.assignment.sequence > last); last = i.assignment.sequence; }
  }
  if (kind === 'inventory') requireValue(int(p.after, 0, st.checkpoint) && int(p.next_after, p.after, st.checkpoint) && p.next_after === (p.items.at(-1)?.assignment.sequence ?? p.after) && p.items.every((i, n) => i.assignment.sequence === p.after + n + 1) && p.has_more === (p.next_after < st.checkpoint));
  if (kind === 'sync') requireValue(p.items.length === 0 || p.items.every((i, n) => i.assignment.sequence === st.checkpoint - p.items.length + n + 1));
  if (kind === 'roster') {
    requireValue(int(p.first_slot, 1, 10) && (p.next_slot === 0 || int(p.next_slot, p.first_slot + 1, 10)));
    const end = p.next_slot || 11, expected = st.slots.map((id, n) => ({ id, slot: n + 1 })).filter((r) => r.id && r.slot >= p.first_slot && r.slot < end);
    requireValue(expected.length === p.items.length && expected.every((r, n) => p.items[n].slot === r.slot && p.items[n].item.assignment.prospect_id === r.id));
  }
  return own(p);
}
function job(v, manifest, stable) {
  exact(v, ['version', 'job_id', 'kind', 'state', 'view', 'store_id', 'operation_id', 'error', 'sync', 'page', 'roster', 'intent', 'mutation']);
  requireValue(v.version === 1 && hex(v.job_id) && ['sync', 'inventory', 'roster', 'prepare', 'apply'].includes(v.kind) && ['queued', 'running', 'done', 'cancelled', 'failed', 'outcome_unknown'].includes(v.state) && viewKey(checkedView(v.view)) === viewKey(manifest?.view) && stable?.status && v.store_id === stable.status.store_id && (v.error === '' || STABLE_ERRORS.has(v.error)));
  requireValue(v.kind === 'apply' ? hex(v.operation_id) : v.operation_id === '');
  const field = { sync: 'sync', inventory: 'page', roster: 'roster', prepare: 'intent', apply: 'mutation' }[v.kind];
  for (const k of ['sync', 'page', 'roster', 'intent', 'mutation']) requireValue(v.state === 'done' && k === field ? v[k] !== null : v[k] === null);
  requireValue(['queued', 'running', 'done'].includes(v.state) ? v.error === '' : v.error !== '');
  if (v.state === 'outcome_unknown') requireValue(v.error === 'outcome_unknown');
  if (v.state === 'done') {
    if (['sync', 'inventory', 'roster'].includes(v.kind)) { const p = pageValue(v[field], v.kind, v.view, manifest.account.fixture); requireValue(p.status.store_id === v.store_id && same(p.status.pin, stable.status.pin)); }
    if (v.kind === 'prepare') { const i = intent(v.intent); requireValue(i.generation === v.view.generation && i.view_id === v.view.view_id); }
    if (v.kind === 'apply') { exact(v.mutation, ['version', 'receipt', 'replayed']); const r = receipt(v.mutation.receipt); requireValue(v.mutation.version === 1 && typeof v.mutation.replayed === 'boolean' && r.operation_id === v.operation_id && r.generation === v.view.generation && r.view_id === v.view.view_id); }
  }
  return own(v);
}
function response(v, kind, manifest, stable, allowNull = false) {
  requireValue(v !== null && typeof v === 'object' && JSON.stringify(v).length <= 131072 && int(v.id, 1) && typeof v.ok === 'boolean');
  if (!v.ok) { exact(v, ['id', 'ok', 'error']); requireValue(STABLE_ERRORS.has(v.error)); return { ok: false, error: v.error }; }
  exact(v, ['id', 'ok', kind]); requireValue(v[kind] !== null);
  if (kind === 'snapshot') return { ok: true, value: stableSnapshot(v.snapshot, manifest) };
  if (kind === 'job') return { ok: true, value: job(v.job, manifest, stable) };
  const o = v.operation; exact(o, ['version', 'view', 'store_id', 'operation']);
  requireValue(o.version === 1 && viewKey(checkedView(o.view)) === viewKey(manifest?.view) && stable?.status && o.store_id === stable.status.store_id);
  requireValue(o.operation !== null || allowNull); if (o.operation !== null) operation(o.operation);
  return { ok: true, value: own(o) };
}

let careerMode = false, careerPage = null, careerActivation = null, careerSelectedJob = "";
let current = null, stable = null, enabled = false, disposed = false, epoch = 0;
let background = null, activeCall = null, timer = null, pollWanted = false, command = '';
let tasks = [], watched = null, flow = null, savedOperation = null, lastViewedJob = '';
let rememberedView = '', connectionError = '', message = '', mutationMessage = '';
let mode = 'recent', pageAfter = 0, pageNext = 0, rosterFirst = 1, rosterNext = 0, pageItems = [], pageRevision = null;
const cards = new Map(), images = new Map(), imageAttempts = new Set(), rosterNodes = [];
function content(node, value) { const s = String(value); if (node.textContent !== s) node.textContent = s; }
function element(tag, className = '', value) { const n = document.createElement(tag); n.className = className; if (value !== undefined) n.textContent = value; return n; }
function label(value) { return value.replaceAll('_', ' ').replace(/(^| )\w/g, (c) => c.toUpperCase()); }
function key(i) { const a = i.assignment; return `${a.prospect_id}|${a.sequence}|${a.manifest_digest}|${a.portrait_digest}`; }
function currentKey() { return viewKey(current?.view); }
function describeChoice(r) { const name = pageItems.find((i) => i.assignment.prospect_id === r.prospect_id)?.manifest.name.display; return r.prospect_id ? `Place ${name || `candidate ${r.prospect_id}`} in slot ${r.slot}.` : `Clear slot ${r.slot}.`; }
function accountReady() { return !disposed && !connectionError && current?.account.state === 'signed_in' && !!current.view; }
function ready() { return accountReady() && stable?.view && currentKey() === viewKey(stable.view); }
function canEdit() { return ready() && stable.status.caught_up && !stable.status.pending_operation_id && !watched && !flow && !command && !['failed', 'unavailable'].includes(stable.state); }
function clearCards() { for (const c of cards.values()) { c.image.removeAttribute('src'); c.article.remove(); } cards.clear(); images.clear(); imageAttempts.clear(); }
function forgetView(clearFlow = true) {
  epoch++; careerPage = null; careerSelectedJob = ""; clearHistory(); clearEvent(); clearEntry(); clearPreview(); clearCards(); pageItems = []; watched = null; savedOperation = null; lastViewedJob = ''; pageAfter = pageNext = 0; rosterFirst = 1; rosterNext = 0; pageRevision = null; mode = 'recent';
  if (clearFlow) { flow = null; mutationMessage = ''; }
}
function disconnected() { forgetView(false); current = stable = null; connectionError = 'The local gym is unavailable. Saved changes remain on disk; reconnect before continuing.'; render(); }
function textError(code) {
  return ({ career_candidate_ineligible: 'C1 bookings require zero professional records. The affected fighters remain in your inventory.', career_inactive: 'Career accounting is waiting for both fixture gyms.', career_unfenced: 'Career accounting has not sealed this noon cutoff yet.', career_unavailable: 'Career evidence is unavailable. No zero-record fallback was used.', career_capacity: 'Career cutoff storage reached its bounded capacity.', busy: 'Another local task is finishing. Try again shortly.', stable_sync_required: 'Saved candidates are still syncing. Resume syncing, then retry this choice.', stable_conflict: 'The roster changed. Refresh it before making a new choice.', stable_pending: 'A saved choice is awaiting reconciliation.', stable_capacity: 'This demo store reached its supported capacity. Existing candidates and outcomes remain available.', stable_clock: 'The authority clock needs attention. Condition and edits are unavailable.', stable_cancelled: 'The request was cancelled.', stale_view: 'This account view has expired. Sign in again.', unauthorized: 'Sign in again to continue.', forbidden: 'This action is unavailable for the current session.', stable_not_found: 'That saved result is unavailable. Reload the page or check the saved change.', outcome_unknown: 'The change may have been saved. Check its recorded outcome before trying another choice.' })[code] ?? 'The local stable could not complete the request. Its saved data has been retained.';
}
function freshStatus(next) {
  const old = stable?.status, n = next?.status;
  if (old && n && viewKey(next.view) === viewKey(stable.view)) {
    requireValue(old.store_id === n.store_id && same(old.pin, n.pin) && n.checkpoint >= old.checkpoint && n.observed_source_count >= old.observed_source_count && n.operation_count >= old.operation_count && n.roster_revision >= old.roster_revision && n.last_as_of_ms >= old.last_as_of_ms);
    if (n.roster_revision === old.roster_revision) requireValue(same(old.slots, n.slots));
  }
}
function renderRoster() {
  while (rosterNodes.length < 10) {
    const slot = rosterNodes.length + 1, li = element('li', 'roster-slot'), title = element('strong', '', `Slot ${slot}`), who = element('span', '', 'Empty');
    const details = element('button', '', 'View'), clear = element('button', '', 'Clear'); details.type = clear.type = 'button';
    details.setAttribute('aria-label', `View roster slot ${slot}`); clear.setAttribute('aria-label', `Clear roster slot ${slot}`);
    details.addEventListener('click', () => navigate('roster', slot <= 8 ? 1 : 9));
    clear.addEventListener('click', () => choose(slot, ''));
    li.append(title, who, details, clear); ui['roster-slots'].append(li); rosterNodes.push({ who, details, clear });
  }
  const list = ready() ? stable.status.slots : Array(10).fill('');
  content(ui['roster-count'], `${list.filter(Boolean).length} / 10`);
  for (const [i, node] of rosterNodes.entries()) { const id = list[i]; content(node.who, id ? (pageItems.find((v) => v.assignment.prospect_id === id)?.manifest.name.display ?? `Candidate ${id.slice(0, 12)}…`) : 'Empty'); node.who.setAttribute('title', id || 'Empty slot'); node.details.disabled = !ready() || !id || !!command; node.clear.disabled = !id || !canEdit(); }
}
const CAREER_POLICY = '2bdf6089dc4bc641dc6e6e87dab21a15196e6b31708c9f84ab8dd728049e50d2';
const CAREER_ERRORS = new Set([...STABLE_ERRORS, 'career_invalid', 'career_inactive', 'career_unfenced', 'career_closed', 'career_poisoned', 'career_cancelled', 'career_corrupt', 'career_candidate_ineligible']);
const CAREER_ACTION_ERRORS = ['career_candidate_ineligible', 'career_inactive', 'career_unfenced', 'career_unavailable', 'career_capacity'];
function careerRetainedBytes() { return (careerPage ? entryBytes(careerPage) : 0) + (careerActivation ? entryBytes(careerActivation) : 0); }
function careerForItem(i) { return careerPage?.view === currentKey() && careerPage.as_of_ms === i.condition.as_of_ms ? careerPage.items.find(x => x.prospect_id === i.assignment.prospect_id && x.assignment_key === key(i)) : null; }
function careerCondition(c, i, asOf, xp) {
  requireValue(int(xp, 0, 15) && c.pro_xp === xp); const zero = own(c); zero.pro_xp = 0; zero.league_attributes_tenths = own(c.aged_attributes_tenths); zero.rating_hundredths = attrs.reduce((n, k, x) => n + zero.aged_attributes_tenths[k] * [10, 25, 2, 4, 8][x], 0); condition(zero, i, asOf);
  for (const k of attrs) requireValue(c.league_attributes_tenths[k] === c.aged_attributes_tenths[k] + (k === 'skill' ? xp * 10 : 0));
  requireValue(c.rating_hundredths === attrs.reduce((n, k, x) => n + c.league_attributes_tenths[k] * [10, 25, 2, 4, 8][x], 0));
}
function careerPageValue(r, kind, position, limit, jobID, terminalSource) {
  exact(r, ['id', 'job_id', 'ok', 'view', 'store_id', 'page', 'roster', 'error']); requireValue(int(r.id, 1) && typeof r.ok === 'boolean' && entryBytes(r) + 1 <= 212992);
  if (!r.ok) { requireValue(r.job_id === '' && r.view === null && r.store_id === '' && r.page === null && r.roster === null && CAREER_ERRORS.has(r.error)); return {ok:false,error:r.error}; }
  requireValue(r.job_id === jobID && hex(jobID) && r.error === '' && viewKey(checkedView(r.view)) === currentKey() && r.store_id === stable.status.store_id && (r.page !== null) === (kind !== 'roster') && (r.roster !== null) === (kind === 'roster'));
  const p = kind === 'roster' ? r.roster : r.page; exact(p, ['version','activation','career_digest','as_of_ms','source','items']); requireValue(p.version === 1 && hex(p.career_digest) && entryBytes(p) <= 196608);
  const a = p.activation; exact(a, ['version','policy_id','activation_id','result_store_id','result_created_at_ms','source_created_at_ms','activated_at_ms']);
  requireValue(a.version === 1 && a.policy_id === CAREER_POLICY && hex(a.activation_id) && hex(a.result_store_id) && int(a.source_created_at_ms,1,DATE_MAX) && int(a.result_created_at_ms,a.source_created_at_ms,DATE_MAX) && int(a.activated_at_ms,a.result_created_at_ms,DATE_MAX) && int(p.as_of_ms,a.activated_at_ms,DATE_MAX) && (!careerActivation || same(careerActivation,a)));
  // Native verifies the activation hash and its concrete Result incarnation.
  if (publicationAuthority) requireValue(a.result_store_id === publicationAuthority.storeID && a.result_created_at_ms === publicationAuthority.createdMS && a.source_created_at_ms === publicationAuthority.sourceCreatedMS);
  const source = pageValue(p.source, kind === 'roster' ? 'roster' : 'inventory', current.view, current.account.fixture); requireValue(same(source.status.pin,stable.status.pin) && source.status.store_id === r.store_id && p.as_of_ms === source.as_of_ms && same(source,terminalSource));
  if (kind === 'roster') requireValue(source.first_slot === position && source.next_slot === (position+limit>10?0:position+limit) && source.status.roster_revision === stable.status.roster_revision); else requireValue(source.after === position);
  const items = kind === 'roster' ? source.items.map(x=>x.item) : source.items; requireValue(Array.isArray(p.items) && p.items.length === items.length && p.items.length <= limit);
  p.items.forEach((x,n) => { exact(x,['prospect_id','totals','condition']); requireValue(x.prospect_id === items[n].assignment.prospect_id); const t=x.totals; exact(t,['wins','losses','draws','xp','base_points']); requireValue(['wins','losses','draws'].every(k=>int(t[k],0,256)) && t.wins+t.losses+t.draws<=256 && t.xp===Math.min(15,t.wins+t.losses+t.draws) && int(t.base_points,10*t.wins+4*t.draws,14*t.wins+4*t.draws) && (t.base_points-10*t.wins-4*t.draws)%4===0); careerCondition(x.condition,items[n],p.as_of_ms,t.xp); });
  return {ok:true,page:p,source,items};
}
async function fetchCareerPage(kind, position, jobID, terminalSource, limit) {
  if (!ready()) return; const captured=epoch, view=currentKey();
  try {
    const raw=await call(kind==='roster'?'CareerRoster':'CareerInventory',current.view.view_id,jobID,position,limit);
    if(disposed||captured!==epoch||view!==currentKey()||careerSelectedJob!==jobID)return;
    const r=careerPageValue(raw,kind,position,limit,jobID,terminalSource); if(!r.ok){message=r.error==='career_inactive'?'Career accounting is waiting for both fixture gyms to finish setup.':textError(r.error);render();return;}
    const before=[pageItems,careerPage,careerActivation,mode,pageAfter,pageNext,pageRevision,rosterFirst,rosterNext];
    careerPage={view,activation_id:r.page.activation.activation_id,career_digest:r.page.career_digest,as_of_ms:r.page.as_of_ms,items:r.page.items.map((x,n)=>({...own(x),assignment_key:key(r.items[n])}))};careerActivation=own(r.page.activation);
    pageItems=own(r.items);mode=kind;if(kind==='recent')pageItems.reverse();if(kind==='roster'){rosterFirst=r.source.first_slot;rosterNext=r.source.next_slot;pageRevision=r.source.status.roster_revision;}else{pageAfter=r.source.after;pageNext=r.source.next_after;pageRevision=null;}
    if(!previewCapacity()){[pageItems,careerPage,careerActivation,mode,pageAfter,pageNext,pageRevision,rosterFirst,rosterNext]=before;message='The display cache is full. The previous page is retained.';render();return;}
    const awarded=new Set(r.page.items.filter(x=>x.totals.xp>0).map(x=>x.prospect_id));
    if(entryDraft.some(x=>awarded.has(x.id))){entryDraft=entryDraft.filter(x=>!awarded.has(x.id));entryMessage='Finalized professional records make these fighters ineligible for new C1 bookings.';}
    message='Finalized career totals at the stated observation time. Provisional outcomes do not award XP or base points.';render();
  } catch { if(!disposed&&captured===epoch&&view===currentKey()&&careerSelectedJob===jobID){message='The career page could not be validated. The previous dated page is retained.';render();} }
}
function careerAffected(j) {
  if (j.error !== 'career_candidate_ineligible') return '';
  return ' Affected fighters: ' + j.ineligible_prospect_ids.map(id => pageItems.find(i => i.assignment.prospect_id === id)?.manifest.name.display ?? id).join(', ') + '.';
}
function careerJobFields(j, fields) {
  const has=Object.hasOwn(j,'ineligible_prospect_ids'); exact(j,has?[...fields,'ineligible_prospect_ids']:fields);
  if(j.error==='career_candidate_ineligible'){requireValue(has && Array.isArray(j.ineligible_prospect_ids) && j.ineligible_prospect_ids.length>=1 && j.ineligible_prospect_ids.length<=10 && j.ineligible_prospect_ids.every((x,n)=>hex(x)&&(n===0||j.ineligible_prospect_ids[n-1]<x)));}
  else requireValue(!has);
}
function makeCard(i) {
  const a = i.assignment, m = i.manifest, career = careerForItem(i), c = career?.condition ?? i.condition;
  const article = element('article', 'candidate'), area = element('div', 'portrait-area'), image = element('img'), placeholder = element('p', 'portrait-placeholder', 'Loading verified portrait…');
  image.setAttribute('width', '200'); image.setAttribute('height', '200'); image.setAttribute('alt', `Candidate portrait of ${m.name.display}`); image.hidden = true; area.append(image, placeholder); article.append(area);
  const body = element('div', 'card-body'), meta = element('div', 'card-meta'); meta.append(element('span', '', label(m.physical.weight_class)), element('span', '', label(m.dna.rarity))); body.append(meta, element('h3', '', m.name.display));
  body.append(element('p', 'physical', `${m.physical.height_in} in tall · ${m.physical.reach_in} in reach · ${m.physical.weight_lb} lb\n${label(m.physical.stance)} · ${label(m.style)}`));
  body.append(element('p', 'condition-phase', `${label(c.phase)} · ${c.age_weeks} completed weeks`), element('p', 'birth', `Condition as of ${date.format(new Date(c.as_of_ms))}`), element('p', 'birth', `Original birth ${date.format(new Date(a.born_at_ms))}`));
  const table = element('table', 'condition-table'), head = element('tr');
  for (const name of ['Attribute', 'Mined', 'Current']) { const th = element('th', '', name); th.setAttribute('scope', 'col'); head.append(th); } const thead = element('thead'); thead.append(head); table.append(thead); const tbody = element('tbody');
  for (const k of attrs) { const row = element('tr'), th = element('th', '', label(k)); th.setAttribute('scope', 'row'); row.append(th, element('td', '', String(m.dna.attributes[k])), element('td', '', (c.league_attributes_tenths[k] / 10).toFixed(1))); tbody.append(row); }
  table.append(tbody); body.append(table, element('p', 'rating', `Rating ${(c.rating_hundredths / 100).toFixed(2)}`), element('p', 'note', `Prime remaining: ${(c.prime_remaining_ms / WEEK).toFixed(2)} weeks · Career remaining: ${(c.career_remaining_ms / WEEK).toFixed(2)} weeks. ${career ? `Finalized professional record: ${career.totals.wins}–${career.totals.losses}–${career.totals.draws}. Recorded professional XP: ${career.totals.xp}. Base-point history: ${career.totals.base_points} (not a ranking).` : careerMode ? 'Career accounting unavailable for this page; legacy condition only.' : 'Recorded professional XP: 0.'}`));
  const selectLabel = element('label', 'select-label', 'Choose roster slot'), select = element('select', 'slot-select'); select.setAttribute('aria-label', `Roster slot for ${m.name.display}`);
  for (let n = 1; n <= 10; n++) { const o = element('option', '', `Slot ${n}`); o.value = String(n); select.append(o); } select.value = '1'; selectLabel.append(select); body.append(selectLabel); article.append(body);
  const actions = element('div', 'card-actions'), assign = element('button', '', 'Save to slot'), save = element('button', '', 'Export public receipt'), retry = element('button', '', 'Reload portrait'); assign.type = save.type = retry.type = 'button';
  assign.addEventListener('click', () => { const slot = Number(select.value); if (int(slot, 1, 10)) choose(slot, a.prospect_id); });
  save.addEventListener('click', () => oldAction('ExportManifestReceipt', [a.sequence], a.prospect_id));
  retry.addEventListener('click', () => { images.delete(key(i)); imageAttempts.delete(key(i)); image.removeAttribute('src'); image.hidden = true; placeholder.hidden = false; pump(); });
  const preview = element('button', '', 'Select for preview'); preview.type = 'button'; preview.addEventListener('click', () => selectPreview(i));
  const entry = element('button', '', 'Add to Saturday draft'); entry.type = 'button'; entry.addEventListener('click', () => selectEntry(i));
  actions.append(assign, save, retry, preview, entry); article.append(actions);
  const history = element('button', '', 'View finalized history'); history.type = 'button'; history.hidden = !careerMode; history.addEventListener('click', () => selectHistory(i)); actions.append(history);
  const details = element('details'), summary = element('summary', '', 'Verified identity'); details.append(summary); const dl = element('dl');
  for (const [name, value] of [['Prospect ID', a.prospect_id], ['Assignment sequence', a.sequence], ['Accepted', date.format(new Date(a.accepted_at_ms))], ['Manifest digest', a.manifest_digest], ['Portrait digest', a.portrait_digest], ['Source receipt digest', a.source_record_digest]]) dl.append(element('dt', '', name), element('dd', '', value));
  details.append(dl); article.append(details);
  return { article, image, placeholder, assign, save, retry, preview, entry, history, select, details, item: i, fingerprint: JSON.stringify([i, careerForItem(i)]) };
}
function renderCards() {
  const want = new Set(pageItems.map((i) => i.assignment.prospect_id));
  for (const [id, c] of cards) if (!want.has(id)) { c.image.removeAttribute('src'); c.article.remove(); cards.delete(id); images.delete(key(c.item)); imageAttempts.delete(key(c.item)); }
  if (!previewCapacity()) { previewImages.clear(); previewRendered = ''; }
  if (!previewCapacity()) {
    images.clear();
    for (const c of cards.values()) { c.image.removeAttribute('src'); c.image.hidden = true; c.placeholder.hidden = false; }
  }
  requireValue(previewCapacity());
  for (const [position, i] of pageItems.entries()) {
    const id = i.assignment.prospect_id; let c = cards.get(id);
    if (c && c.fingerprint !== JSON.stringify([i, careerForItem(i)])) { const previous = c; requireValue(same(c.item.assignment, i.assignment) && same(c.item.manifest, i.manifest)); c = makeCard(i); c.select.value = previous.select.value; c.details.open = previous.details.open; previous.image.removeAttribute('src'); previous.article.remove(); cards.set(id, c); }
    if (!c) { c = makeCard(i); cards.set(id, c); }
    if (ui.cards.children[position] !== c.article) ui.cards.insertBefore(c.article, ui.cards.children[position] ?? null);
    c.preview.disabled = (careerMode && (!careerForItem(i) || !careerPreviewReady())) || !ready() || !!command || previewSelected.some((v) => v.id === id) || previewSelected.length >= 2;
    c.entry.disabled = (careerMode && (!careerForItem(i) || careerForItem(i).totals.xp > 0)) || !entryReady() || !!command || entryDraft.length >= 5 || entryDraft.some((d) => d.id === id) || !stable.status.slots.includes(id) || i.condition.phase === 'retired';
    c.history.hidden = !careerMode; c.history.disabled = !ready() || !!command || !!historyWatch || historyUnknown;
    c.assign.disabled = !canEdit(); c.select.disabled = !canEdit(); c.save.disabled = !ready() || !!command || !!watched; c.retry.disabled = !ready() || !!command;
    if (images.has(key(i))) { c.image.setAttribute('src', images.get(key(i))); c.image.hidden = false; c.placeholder.hidden = true; }
  }
  ui.empty.hidden = pageItems.length !== 0;
}
function render() {
  const navigationBusy = !!watched && !['sync', 'inventory', 'roster'].includes(watched.kind);
  const s = current, signed = ready(), accountSigned = accountReady(), live = ['starting', 'running'].includes(s?.state), preparing = ['signing_in', 'resetting'].includes(s?.account.state), st = stable?.status;
  content(ui.state, s ? label(s.state) : 'Connecting'); content(ui['account-state'], label(s?.account.state ?? 'signed_out'));
  content(ui['account-copy'], accountSigned ? `${s.account.fixture === 'zero256' ? 'Demo A' : 'Demo B'} keeps the same wallet, gym and candidate stable.` : preparing ? 'Preparing the retained demo gym…' : 'Choose a demo account. Saved candidates sync automatically; mining starts only with Start.');
  content(ui.hold, s?.account.held ? 'The 24-hour recovery hold is active. Viewing, mining and reversible roster moves are available; recovery backup export and test-result signing remain held.' : '');
  for (const id of ['login-a', 'login-b']) ui[id].disabled = !enabled || !!command || !s || !['signed_out', 'failed', 'expired'].includes(s.account.state) || !!s.view;
  ui.logout.disabled = !accountSigned || !!command; ui.reset.disabled = !accountSigned || !!command;
  ui.mining.disabled = !enabled || !!command || (!accountSigned && !preparing) || s?.state === 'stopping'; content(ui.mining, live ? 'Stop mining' : preparing ? 'Stop preparing' : 'Start mining');
  content(ui['mining-copy'], publicationNodeConnected() ? 'Connected for publication. Start mining when ready, or disconnect the test nodes below.' : preparing ? 'Preparing your retained identity. Stop drains work already in progress.' : live ? 'Mining is active. New saved candidates update the stable automatically.' : 'Syncing and viewing do not create mining jobs. Start only when you are ready.');
  for (const [id, value] of [['attempts', s?.attempts ?? 0], ['accepted', s?.accepted_count ?? 0], ['pending', s?.pending ?? 0], ['height', s?.height ?? 0]]) content(ui[id], number.format(value));
  content(ui.rate, live ? `${number.format(s.hashes_per_second)} hashes / second` : 'Not mining'); content(ui['wallet-state'], label(s?.wallet_state ?? 'closed'));
  content(ui.spendable, accountSigned ? coin.format(s.spendable_atoms / 1e8) : '—'); content(ui.immature, accountSigned ? coin.format(s.immature_atoms / 1e8) : '—'); content(ui['wallet-height'], accountSigned ? `Wallet height ${number.format(s.wallet_height)} · Rewards mature after 100 blocks.` : 'Rewards mature after 100 blocks.');
  content(ui.payout, accountSigned ? s.payout_address : 'Sign in to load the demo wallet'); content(ui['gym-id'], accountSigned ? s.gym_id : '—');
  ui['export-recovery'].disabled = !accountSigned || !!command || live || publicationNodeConnected() || !!watched || !s.account.can_export || s.backup_state === 'running'; ui['verify-backup'].disabled = !accountSigned || !!command || live || publicationNodeConnected() || !!watched || s.backup_state === 'running';
  content(ui.backup, s?.backup_state === 'running' ? `${backupPhases[s.backup_phase] || 'Checking the backup'}…` : s?.backup_state === 'succeeded' ? 'Backup verification completed.' : s?.backup_state === 'failed' ? 'Backup verification failed; the original wallet remains retained.' : publicationNodeConnected() ? 'Disconnect test nodes before saving or verifying a wallet backup.' : 'Stop mining to save or verify a wallet backup.');
  content(ui['stable-state'], stable ? label(stable.state) : 'Not connected'); content(ui['stable-count'], signed ? `${number.format(st.checkpoint)} assigned candidates · ${number.format(st.observed_source_count)} saved receipts last observed` : 'Sign in to load your assigned candidates.');
  content(ui['stable-progress'], signed ? (st.caught_up ? 'Caught up to the last observed receipt count. New arrivals are checked automatically.' : `Syncing saved history: ${st.checkpoint} of ${st.observed_source_count}. Verified partial pages remain available.`) : '');
  ui.sync.disabled = !signed || !!command || !!watched; ui['pause-sync'].disabled = !signed || !!command || (!stable.active_job_id && !stable.queued_job_id && stable.state !== 'syncing');
  ui['cancel-job'].disabled = !signed || !!command || !(watched?.id || stable.active_job_id || stable.queued_job_id);
  for (const id of ['recent', 'inventory', 'roster-details', 'refresh']) ui[id].disabled = !signed || !!command || navigationBusy;
  ui.previous.disabled = !signed || !!command || navigationBusy || (mode === 'roster' ? rosterFirst === 1 : mode !== 'inventory' || pageAfter === 0);
  ui.next.disabled = !signed || !!command || navigationBusy || (mode === 'roster' ? rosterNext === 0 : mode !== 'inventory' || pageNext >= st.checkpoint);
  content(ui['candidates-heading'], mode === 'roster' ? 'Roster details' : mode === 'inventory' ? 'Candidate inventory' : 'Recent candidates');
  content(ui['page-label'], pageItems.length ? (mode === 'roster' ? `Roster slots ${rosterFirst}–${Math.min(rosterFirst + 7, 10)}` : `Showing ${pageItems.length} verified candidates${mode === 'inventory' ? ` after sequence ${pageAfter}` : ''}`) : 'No verified page loaded');
  content(ui.coverage, 'Condition is dated by the authority on each card. Browser time never changes its attributes or rating. Selecting a roster does not grant fight eligibility.');
  content(ui['empty-copy'], signed ? (st.checkpoint ? 'Choose Recent, Full inventory or Roster details to load a verified page.' : 'Saved history is syncing. Start mining separately when you are ready.') : 'Sign in to load your retained demo gym.');
  content(ui['mutation-state'], signed ? mutationMessage || (st.pending_operation_id ? 'A saved roster choice needs review before another change.' : 'Roster choices take effect only after their durable outcome is confirmed.') : '');
  ui.reconcile.disabled = !signed || !!command || !!watched || !(flow || st.pending_operation_id || st.last_operation_id);
  const pending = savedOperation?.state === 'prepared' ? savedOperation : null;
  ui['apply-saved'].disabled = !signed || !!command || !!watched || !pending || pending.intent.view_id !== s.view.view_id || pending.intent.generation !== s.view.generation || !st.caught_up;
  ui.abandon.disabled = !signed || !!command || !!watched || !pending;
  content(ui.alert, connectionError || message || (stable?.error ? textError(stable.error) : '')); ui.alert.hidden = !ui.alert.textContent;
  renderRoster(); renderCards(); renderPreview(); renderEntry(); renderEvent(); renderSeed(); renderCard(); renderBooked(); renderPublication(); renderHistory();
}
async function call(method, ...args) { requireValue(!activeCall); const p = Call.ByName(`main.Gym.${method}`, ...args); activeCall = p; try { return await p; } finally { activeCall = null; } }
function schedule() { if (timer !== null) clearTimeout(timer); timer = null; if (!disposed && enabled && !document.hidden) timer = setTimeout(() => { timer = null; pollWanted = true; pump(); }, 1000); }
function queue(fn, name, invalidate = false) {
  if (!enabled || disposed || command) return;
  if (invalidate) { forgetView(); current = stable = null; rememberedView = ''; }
  command = name; render(); tasks.unshift(async () => { try { await fn(); } finally { command = ''; render(); pollWanted = true; } }); pump();
}
async function fetchSnapshots() {
  const captured = epoch;
  try {
    const m = normalize(await call('ManifestSnapshot')); if (disposed || captured !== epoch) return;
    const r = response(await call('StableSnapshot'), 'snapshot', m, stable); if (disposed || captured !== epoch) return;
    requireValue(r.ok); const next = r.value, nextKey = viewKey(m.view);
    if (nextKey !== rememberedView) { forgetView(); rememberedView = nextKey; stable = null; message = ''; }
    if (!nextKey) { current = m; stable = next; connectionError = ''; render(); return; }
    freshStatus(next); current = m; stable = next; connectionError = '';
    if (mode === 'roster' && pageRevision !== null && stablyChangedRoster()) { clearCards(); pageItems = []; pageRevision = null; message = 'The roster changed. Reload its details to view the current selection.'; }
    render();
    const id = watched?.id || next.last_job_id;
    if (id && (watched || id !== lastViewedJob)) await fetchJob(id);
    if (!disposed && ready() && !tasks.length) await fetchPreviewSnapshot();
    if (!disposed && ready() && !tasks.length) await fetchEntrySnapshot();
    if (!disposed && ready() && !tasks.length) await fetchEventSnapshot();
    if (!disposed && ready() && !tasks.length) await fetchSeedSnapshot();
    if (!disposed && ready() && !tasks.length) await fetchCardSnapshot();
    if (!disposed && ready() && !tasks.length) await fetchBookedSnapshot();
    if (!disposed && ready() && !tasks.length) await fetchPublicationSnapshot();
    if (!disposed && ready() && !tasks.length && careerMode && (historySelection || historyUnknown)) await fetchHistorySnapshot();
  } catch { if (!disposed && captured === epoch) disconnected(); }
}
function stablyChangedRoster() { return stable.status.roster_revision !== pageRevision; }
function acceptItems(items, kind, data) {
  const priorPage = [pageItems, mode, pageAfter, pageNext, pageRevision, rosterFirst, rosterNext];
  if (kind === 'sync') {
    if (mode !== 'recent') { message = 'New candidates were saved. Your selected page remains open.'; return; }
    const merged = new Map(pageItems.map((i) => [i.assignment.prospect_id, i])); for (const i of items) { const prior = merged.get(i.assignment.prospect_id); requireValue(!prior || same(prior.assignment, i.assignment) && same(prior.manifest, i.manifest)); merged.set(i.assignment.prospect_id, i); }
    pageItems = [...merged.values()].sort((a, b) => b.assignment.sequence - a.assignment.sequence).slice(0, 8);
  } else {
    pageItems = own(kind === 'roster' ? items.map((v) => v.item) : items);
    if (kind === 'roster') { mode = 'roster'; rosterFirst = data.first_slot; rosterNext = data.next_slot; pageRevision = data.status.roster_revision; }
    else { pageAfter = data.after; pageNext = data.next_after; pageRevision = null; if (mode === 'recent') pageItems.reverse(); }
  }
  requireValue(pageItems.length <= 8);
  if (!previewCapacity()) { [pageItems, mode, pageAfter, pageNext, pageRevision, rosterFirst, rosterNext] = priorPage; message = 'The display cache is full. Clear a detail or replay before loading another page; the current page is retained.'; render(); return; }
  render(); content(ui.announcement, `${pageItems.length} verified candidates shown. Conditions keep their stated observation time.`);
}
async function fetchJob(id) {
  if (!ready()) return; const captured = epoch, view = currentKey(), w = watched;
  try {
    const r = response(await call('StableJob', current.view.view_id, id), 'job', current, stable);
    if (disposed || captured !== epoch || view !== currentKey()) return;
    if (!r.ok) { if (w?.id === id) { watched = null; if (['prepare', 'apply'].includes(w.kind)) await reconcile(w.operationID ?? ''); else message = textError(r.error); } lastViewedJob = id; return; }
    const j = r.value; requireValue(j.job_id === id);
    if (w?.id === id) requireValue(j.kind === w.kind && (j.kind !== 'apply' || j.operation_id === w.operationID));
    if (['queued', 'running'].includes(j.state)) { render(); return; }
    lastViewedJob = id; if (w?.id === id) watched = null;
    if (j.state !== 'done') {
      if (['prepare', 'apply'].includes(j.kind) && w?.id === id) { mutationMessage = textError(j.error); if (j.state === 'outcome_unknown') await reconcile(w.operationID ?? ''); else { flow = null; savedOperation = null; } }
      else message = textError(j.error); render(); return;
    }
    if (j.kind === 'prepare') {
      if (w?.id === id && w.request) {
        requireValue(same(j.intent.request, w.request)); flow = { request: w.request, id: j.intent.operation_id, view, generation: current.view.generation, phase: 'prepared' };
        savedOperation = { version: 1, intent: j.intent, state: 'prepared', receipt: null, abandoned_at_ms: 0 };
        await beginJob('apply', [j.intent.operation_id], { operationID: j.intent.operation_id });
      } else if (savedOperation?.intent.operation_id !== j.intent.operation_id) { mutationMessage = `${describeChoice(j.intent.request)} Check its saved outcome before continuing.`; }
    } else if (j.kind === 'apply') {
      if (w?.id === id && flow) requireValue(same(j.mutation.receipt.request, flow.request));
      flow = null; savedOperation = null; mutationMessage = j.mutation.receipt.changed ? 'Roster saved. Refreshing the current selection…' : 'The roster was already set to that choice.'; pollWanted = true;
    } else {
      const data = j[{ sync: 'sync', inventory: 'page', roster: 'roster' }[j.kind]];
      if (w?.id === id && w.args) { if (j.kind === 'inventory') requireValue(w.recent ? data.after === Math.max(data.status.checkpoint - 8, 0) && data.next_after === data.status.checkpoint && data.items.length <= 8 : data.after === w.args[0] && data.items.length <= w.args[1]); if (j.kind === 'roster') requireValue(data.first_slot === w.args[0] && data.next_slot === (w.args[0] + w.args[1] > 10 ? 0 : w.args[0] + w.args[1])); }
      if (j.kind === 'roster' && data.status.roster_revision !== stable.status.roster_revision) message = 'The roster changed. Reload its details.';
      else if (j.kind === 'sync' || w?.id === id) {
        if (!careerMode) acceptItems(data.items, j.kind, data);
        else if (j.kind === 'sync') { if (mode === 'recent') await beginJob('inventory', [], {recent:true,careerMode:'recent'}); }
        else if (careerSelectedJob === j.job_id) {
          const selectedMode=w?.careerMode ?? (w?.recent?'recent':j.kind), position=j.kind==='roster'?data.first_slot:data.after, limit=w?.recent?8:w.args[1];
          await fetchCareerPage(selectedMode,position,j.job_id,data,limit);
        }
      }
    }
    render();
  } catch {
    if (disposed || captured !== epoch || view !== currentKey()) return;
    if (w?.id === id && ['prepare', 'apply'].includes(w.kind)) { watched = null; mutationMessage = textError('outcome_unknown'); if (flow) flow.phase = 'unknown'; }
    else { watched = null; message = 'The saved page could not be validated. Reload it to continue.'; }
    render();
  }
}
async function beginJob(kind, args = [], extra = {}) {
  if (!ready()) return; const captured = epoch, view = currentKey();
  const method = extra.recent ? 'BeginStableRecent' : { sync: 'BeginStableSync', inventory: 'BeginStableInventory', roster: 'BeginStableRoster', prepare: 'BeginStablePrepare', apply: 'BeginStableApply' }[kind];
  try {
    const r = response(await call(method, current.view.view_id, ...args), 'job', current, stable);
    if (disposed || captured !== epoch || view !== currentKey()) return;
    if (!r.ok) { if (['prepare', 'apply'].includes(kind)) { mutationMessage = textError(r.error); if (r.error === 'outcome_unknown') await reconcile(extra.operationID ?? ''); else if (kind === 'prepare') flow = null; } else message = textError(r.error); return; }
    requireValue(r.value.kind === kind && (kind !== 'apply' || r.value.operation_id === extra.operationID));
    watched = { id: r.value.job_id, kind, args, ...extra }; if(careerMode) careerSelectedJob=r.value.job_id; pollWanted = true;
  } catch {
    if (disposed || captured !== epoch || view !== currentKey()) return;
    if (['prepare', 'apply'].includes(kind)) { mutationMessage = textError('outcome_unknown'); if (flow) flow.phase = 'unknown'; await reconcile(extra.operationID ?? ''); }
    else message = 'The request could not be acknowledged. Check the stable before retrying.';
  }
  render();
}
async function reconcile(id = '') {
  if (!ready()) return; const captured = epoch, view = currentKey();
  const selected = id || flow?.id || stable.status.pending_operation_id;
  try {
    const method = selected ? 'StableOperation' : 'StablePending';
    const r = response(await call(method, current.view.view_id, ...(selected ? [selected] : [])), 'operation', current, stable, !selected);
    if (disposed || captured !== epoch || view !== currentKey()) return;
    if (!r.ok) { mutationMessage = textError(r.error); return; }
    const op = r.value.operation; if (op && selected) requireValue(op.intent.operation_id === selected);
    savedOperation = op;
    if (!op) { flow = null; mutationMessage = 'No unapplied roster choice is saved. Refresh the roster before making another choice.'; }
    else if (op.state === 'prepared') {
      flow = { id: op.intent.operation_id, request: op.intent.request, view: viewKey({ ...current.view, view_id: op.intent.view_id, generation: op.intent.generation }), generation: op.intent.generation, phase: 'prepared' };
      mutationMessage = `${describeChoice(op.intent.request)} ${op.intent.view_id === current.view.view_id && op.intent.generation === current.view.generation ? 'The saved choice has not been applied. Apply it explicitly or discard it.' : 'This choice belongs to an earlier sign-in. It cannot execute now; discard it before making a new choice.'}`;
    } else { flow = null; mutationMessage = op.state === 'committed' ? `Saved outcome confirmed at roster revision ${op.receipt.revision}. The strip shows the current roster.` : 'The saved choice was discarded. The roster was not changed.'; }
    pollWanted = true;
  } catch { if (captured === epoch && view === currentKey()) mutationMessage = 'The recorded outcome is unavailable. Keep this choice unresolved and check again.'; }
  render();
}
function choose(slot, id) {
  if (!canEdit()) return; const r = { expected_revision: stable.status.roster_revision, slot, prospect_id: id }; request(r);
  queue(async () => { if (!ready()) return; flow = { request: r, id: '', view: currentKey(), generation: current.view.generation, phase: 'preparing' }; mutationMessage = 'Saving the roster choice…'; await beginJob('prepare', [r.expected_revision, r.slot, r.prospect_id], { request: r }); }, 'prepare');
}
function navigate(kind, position = 0) {
  if (!ready() || command) return;
  queue(async () => {
    if (watched) { await cancelJob(watched.id); if (!ready()) return; }
    if (careerMode) {
      message='Verifying the selected fighter page…';
      if(kind==='roster'){const first=position||1;await beginJob('roster',[first,Math.min(8,11-first)],{careerMode:kind});}
      else if(kind==='recent')await beginJob('inventory',[],{recent:true,careerMode:kind});
      else await beginJob('inventory',[position,8],{careerMode:kind});
      return;
    }
    mode = kind; message = ''; clearCards(); pageItems = []; pageRevision = null;
    if (kind === 'roster') { rosterFirst = position || 1; await beginJob('roster', [rosterFirst, Math.min(8, 11 - rosterFirst)]); }
    else if (kind === 'recent') { await beginJob('inventory', [], { recent: true }); }
    else { pageAfter = position; await beginJob('inventory', [pageAfter, 8]); }
  }, 'page');
}
async function cancelJob(id) {
  if (!ready() || !id) return; const captured = epoch;
  try { const r = response(await call('CancelStableJob', current.view.view_id, id), 'job', current, stable); if (captured !== epoch) return; if (!r.ok) message = textError(r.error); else { requireValue(r.value.job_id === id); if (watched?.id === id) { if (['prepare', 'apply'].includes(watched.kind)) await reconcile(watched.operationID ?? ''); watched = null; } message = 'Cancellation requested. Any committed choice is still retained.'; } }
  catch { if (captured === epoch) message = 'Cancellation could not be confirmed. Stop or check the saved result.'; }
}
async function oldAction(method, args = [], expectedID = '') {
  if (!enabled || command) return;
  pauseBooked();
  if (method === 'Stop') { clearHistory(); entryChanged(); if (eventStatus && eventAutoAttempted.size < 256) eventAutoAttempted.add(eventStatus.event.event_id); }
  const capturedView = currentKey(); if (expectedID && !pageItems.some((i) => i.assignment.prospect_id === expectedID)) return;
  const invalidate = ['LoginFixture', 'Logout', 'ResetFixtureAccount'].includes(method);
  queue(async () => {
    let result, failed = false;
    try { result = await call(method, ...args); const allowed = { ExportManifestReceipt: ['', 'Receipt saved.'], ExportRecovery: ['', 'Test wallet recovery file saved.'], VerifyBackup: ['', 'Backup verification started.'] }; if (allowed[method]) requireValue(allowed[method].includes(result)); }
    catch { failed = true; }
    await fetchSnapshots();
    if (disposed || (!invalidate && capturedView !== currentKey())) return;
    message = failed ? 'The requested action did not complete. Check the current gym and try again.' : typeof result === 'string' ? result : '';
  }, method, invalidate);
}
async function loadPortrait(i) {
  const captured = epoch, view = currentKey(), fingerprint = JSON.stringify(i), k = key(i); imageAttempts.add(k);
  const remains = () => !disposed && captured === epoch && view === currentKey() && cards.get(i.assignment.prospect_id)?.fingerprint === fingerprint;
  try {
    const p = await call('ManifestPortrait', i.assignment.sequence); if (!remains()) return;
    exact(p, ['version', 'view', 'sequence', 'id', 'manifest_digest', 'portrait_digest', 'svg']); const a = i.assignment;
    requireValue(p.version === 1 && viewKey(checkedView(p.view)) === view && p.sequence === a.sequence && p.id === a.prospect_id && p.manifest_digest === a.manifest_digest && p.portrait_digest === a.portrait_digest && imageOnlySVG(p.svg));
    const src = `data:image/svg+xml;base64,${btoa(p.svg)}`; requireValue([...images.entries()].reduce((n, [other, v]) => n + (other === k ? 0 : 2 * v.length), 2 * src.length) + JSON.stringify(pageItems).length + careerRetainedBytes() + previewRetainedBytes() + entryRetainedBytes() + eventRetainedBytes() + seedRetainedBytes() + cardRetainedBytes() + bookedRetainedBytes() + publicationRetainedBytes() + historyRetainedBytes() <= CACHE_MAX && images.size <= 8);
    if (!remains()) return; images.set(k, src); const c = cards.get(a.prospect_id); c.image.setAttribute('src', src); c.image.hidden = false; c.placeholder.hidden = true;
  } catch { if (remains()) { const c = cards.get(i.assignment.prospect_id); c.image.removeAttribute('src'); c.image.hidden = true; c.placeholder.hidden = false; content(c.placeholder, 'Portrait unavailable. Reload it from the current verified page.'); } }
}
function pump() {
  if (background || disposed || !enabled || document.hidden) return background;
  background = (async () => {
    while (!disposed && enabled && !document.hidden) {
      if (tasks.length) { await tasks.shift()(); continue; }
      if (command) break;
      if (pollWanted) { pollWanted = false; await fetchSnapshots(); schedule(); continue; }
      const next = ready() ? pageItems.find((i) => !images.has(key(i)) && !imageAttempts.has(key(i))) : null;
      if (next) { await loadPortrait(next); continue; }
      const bookedPortrait = bookedNextPortrait(); if (bookedPortrait) { await loadBookedPortrait(bookedPortrait); continue; }
      const portrait = previewNextPortrait(); if (portrait) { await loadPreviewPortrait(portrait); continue; }
      break;
    }
  })().catch(() => { if (!disposed) disconnected(); }).finally(() => { background = null; if ((tasks.length || pollWanted) && !disposed && enabled && !document.hidden) pump(); });
  return background;
}
const PREVIEW_ENGINE = 'slug-fighter-bout-candidate-v1';
const PREVIEW_ERRORS = new Set(['invalid_request', 'unauthorized', 'forbidden', 'stale_view', 'busy', 'stable_corrupt', 'stable_clock', 'stable_conflict', 'stable_sync_required', 'outcome_unknown', 'preview_not_found', 'preview_class_mismatch', 'preview_capacity', 'preview_cancelled', 'preview_unavailable', 'preview_observation_unknown']);
const FINISHES = ['decision', 'decision_draw', 'ko', 'tko_three_knockdowns', 'tko_referee', 'tko_corner', 'double_corner_stoppage'];
function pairArray(v, fn) { requireValue(Array.isArray(v) && v.length === 2); v.forEach(fn); }
function boutState(v) {
  exact(v, ['stamina_q', 'damage_taken_q', 'thrown', 'landed', 'significant', 'damage_dealt_q', 'knockdowns', 'control_q', 'round_knockdowns']);
  requireValue(int(v.stamina_q, 0, 214000000) && int(v.damage_taken_q, 0, 2699999999) && int(v.damage_dealt_q, 0, 2699999999) && int(v.control_q, 0, 630000000));
  requireValue(int(v.thrown, 0, 180) && int(v.landed, 0, v.thrown) && int(v.significant, 0, v.landed) && int(v.knockdowns, 0, 180) && int(v.round_knockdowns, 0, 3));
}
function boutValue(v, ids) {
  exact(v, ['version', 'engine_id', 'input_id', 'initial', 'rounds', 'final', 'winner', 'method', 'end_round', 'end_tick', 'scores', 'draw_counts']);
  requireValue(v.version === 1 && v.engine_id === PREVIEW_ENGINE && hex(v.input_id) && FINISHES.includes(v.method) && int(v.end_round, 1, 3) && int(v.end_tick, 1, 60));
  pairArray(v.initial, (i) => { exact(i, ['form_q', 'jitter_q', 'effective', 'max_stamina_q', 'ko_threshold_q']); exact(i.effective, attrs); requireValue(int(i.form_q, -100000, 100000) && int(i.jitter_q, 920000, 1080000) && int(i.max_stamina_q, 60000000, 214000000) && int(i.ko_threshold_q, 64000000, 130000000) && attrs.every((k) => int(i.effective[k], 1, k === 'skill' ? 130000000 : 110000000))); });
  requireValue(Array.isArray(v.rounds) && v.rounds.length === v.end_round);
  let count = 0;
  for (const [n, r] of v.rounds.entries()) {
    exact(r, ['number', 'ticks', 'end', 'recovery']); requireValue(r.number === n + 1 && Array.isArray(r.ticks) && r.ticks.length === (n + 1 === v.end_round ? v.end_tick : 60));
    for (const [t, e] of r.ticks.entries()) {
      exact(e, ['number', 'actor', 'strike', 'hit', 'damage_q', 'significant', 'knockdown', 'stoppage', 'states']);
      requireValue(++count <= 180 && e.number === t + 1 && int(e.actor, 0, 1) && ['jab', 'cross', 'hook', 'uppercut', 'body'].includes(e.strike) && typeof e.hit === 'boolean' && int(e.damage_q, 0, 8999999) && typeof e.significant === 'boolean' && typeof e.knockdown === 'boolean' && ['', 'ko', 'tko_three_knockdowns', 'tko_referee'].includes(e.stoppage));
      requireValue(e.hit ? e.damage_q > 0 : e.damage_q === 0 && !e.significant && !e.knockdown && e.stoppage === '');
      pairArray(e.states, boutState); if (e.stoppage) requireValue(n + 1 === v.end_round && t + 1 === v.end_tick && e.stoppage === v.method);
    }
    pairArray(r.end, boutState); requireValue(same(r.end, r.ticks.at(-1).states));
    const recovery = r.number < 3 && r.ticks.length === 60 && !r.ticks.at(-1).stoppage;
    requireValue((r.recovery !== null) === recovery);
    if (r.recovery !== null) { exact(r.recovery, ['states', 'corner_stopped']); pairArray(r.recovery.states, boutState); pairArray(r.recovery.corner_stopped, (x) => requireValue(typeof x === 'boolean')); if (r.number !== v.end_round) requireValue(!r.recovery.corner_stopped.some(Boolean)); }
  }
  pairArray(v.final, boutState); const last = v.rounds.at(-1); requireValue(same(v.final, last.recovery?.states ?? last.end));
  pairArray(v.scores, (s, n) => {
    const keys = ['damage_tenths', 'knockdown_tenths', 'accuracy_tenths', 'volume_tenths', 'control_tenths', 'total_tenths']; exact(s, keys); const f = v.final[n];
    requireValue(keys.every((k) => int(s[k], 0, 48534)) && s.damage_tenths === halfEven(f.damage_dealt_q, 100000) && s.knockdown_tenths === 100 * f.knockdowns && s.accuracy_tenths === (f.thrown ? halfEven(250 * f.landed, f.thrown) : 0) && s.volume_tenths === 6 * f.significant && s.control_tenths === halfEven(7 * f.control_q, 2000000) && s.total_tenths === keys.slice(0, 5).reduce((sum, k) => sum + s[k], 0));
  });
  const draw = ['decision_draw', 'double_corner_stoppage'].includes(v.method); requireValue(draw ? v.winner === '' : ids.includes(v.winner));
  if (v.method.startsWith('decision')) { requireValue(v.end_round === 3 && v.end_tick === 60 && last.ticks.at(-1).stoppage === '' && (v.scores[0].total_tenths === v.scores[1].total_tenths) === draw); if (!draw) requireValue(v.winner === ids[v.scores[0].total_tenths > v.scores[1].total_tenths ? 0 : 1]); }
  if (['tko_corner', 'double_corner_stoppage'].includes(v.method)) { requireValue(v.end_round < 3 && v.end_tick === 60 && last.recovery !== null); const flags = last.recovery.corner_stopped; requireValue(flags.filter(Boolean).length === (draw ? 2 : 1)); if (!draw) requireValue(v.winner === ids[flags[0] ? 1 : 0]); }
  exact(v.draw_counts, ['form_a', 'form_b', 'chin_a', 'chin_b', 'combat']); const d = Object.values(v.draw_counts); requireValue(d.every((x) => int(x, 1, 65536)) && d.reduce((a, b) => a + b, 0) <= 65536);
  requireValue(JSON.stringify(v).length <= 196608);
}
function previewEnvelope(v, kind) {
  requireValue(v !== null && typeof v === 'object' && JSON.stringify(v).length < 262144 && v.version === 1 && int(v.id, 1) && typeof v.ok === 'boolean');
  if (!v.ok) { exact(v, ['id', 'version', 'ok', 'error']); requireValue(PREVIEW_ERRORS.has(v.error)); return { ok: false, error: v.error }; }
  exact(v, ['id', 'version', 'ok', kind]); const value = v[kind];
  if (kind === 'snapshot') {
    exact(value, ['version', 'state', 'error', 'view', 'store_id', 'engine_version', 'engine_id', 'active_job_id', 'queued_job_id', 'last_job_id']);
    requireValue(value.version === 1 && ['unavailable', 'idle', 'busy', 'ready', 'failed', 'cancelled'].includes(value.state) && ['active_job_id', 'queued_job_id', 'last_job_id'].every((k) => optionalID(value[k])));
    if (value.view === null) requireValue(value.state === 'unavailable' && value.error === '' && value.store_id === '' && value.engine_version === 0 && value.engine_id === '' && value.active_job_id === '' && value.queued_job_id === '' && value.last_job_id === '');
    else { previewPins(value); requireValue(value.state !== 'unavailable' && (['failed', 'cancelled'].includes(value.state) ? PREVIEW_ERRORS.has(value.error) && value.error !== 'outcome_unknown' : value.error === '')); requireValue(value.state === 'busy' ? !!(value.active_job_id || value.queued_job_id) && !(value.active_job_id && value.queued_job_id) && !value.last_job_id : !value.active_job_id && !value.queued_job_id); requireValue(value.state === 'idle' ? !value.last_job_id : value.state === 'busy' || !!value.last_job_id); if (value.state === 'cancelled') requireValue(value.error === 'preview_cancelled'); }
  } else {
    exact(value, ['version', 'job_id', 'state', 'view', 'store_id', 'engine_version', 'engine_id', 'fighter_a_id', 'fighter_b_id', 'error', 'result']); previewPins(value);
    requireValue(hex(value.job_id) && hex(value.fighter_a_id) && hex(value.fighter_b_id) && value.fighter_a_id < value.fighter_b_id && ['queued', 'running', 'done', 'failed', 'cancelled'].includes(value.state));
    requireValue(['queued', 'running', 'done'].includes(value.state) ? value.error === '' : PREVIEW_ERRORS.has(value.error) && value.error !== 'outcome_unknown'); if (value.state === 'cancelled') requireValue(value.error === 'preview_cancelled');
    requireValue((value.result !== null) === (value.state === 'done'));
    if (value.result !== null) {
      const r = value.result; exact(r, ['version', 'pair', 'recipe', 'seed', 'engine_result', 'engine_result_digest']); requireValue(r.version === 1 && hex(r.seed) && hex(r.engine_result_digest) && JSON.stringify(r).length <= 229376);
      const p = r.pair; exact(p, ['version', 'status', 'as_of_ms', 'fighters']); requireValue(p.version === 1 && int(p.as_of_ms, 1, DATE_MAX) && p.status.last_as_of_ms === p.as_of_ms && p.status.store_id === value.store_id && same(p.status.pin, stable.status.pin) && JSON.stringify(p).length <= 24576); status(p.status, value.view, current.account.fixture); pairArray(p.fighters, (i) => fullItem(i, p.status, p.as_of_ms));
      const ids = [value.fighter_a_id, value.fighter_b_id]; requireValue(p.fighters.every((i, n) => i.assignment.prospect_id === ids[n]) && p.fighters[0].manifest.physical.weight_class === p.fighters[1].manifest.physical.weight_class);
      const recipe = r.recipe; exact(recipe, ['version', 'engine_version', 'engine_id', 'network', 'genesis', 'store_id', 'source_authority_id', 'descriptor_id', 'as_of_ms', 'rounds', 'training', 'fighters']);
      requireValue(recipe.version === 1 && recipe.engine_version === 1 && recipe.engine_id === PREVIEW_ENGINE && recipe.network === 0x534c5547 && recipe.genesis === GENESIS && recipe.store_id === value.store_id && recipe.source_authority_id === p.status.pin.source.authority_id && recipe.descriptor_id === DESCRIPTOR && recipe.as_of_ms === p.as_of_ms && recipe.rounds === 3 && JSON.stringify(recipe).length <= 4096);
      pairArray(recipe.training, (x) => requireValue(x === 0)); pairArray(recipe.fighters, (f, n) => { exact(f, ['prospect_id', 'source_record_digest', 'manifest_digest', 'condition_digest']); const a = p.fighters[n].assignment; requireValue(f.prospect_id === ids[n] && f.source_record_digest === a.source_record_digest && f.manifest_digest === a.manifest_digest && hex(f.condition_digest)); }); boutValue(r.engine_result, ids);
    }
  }
  return { ok: true, value: own(value) }; // Native checks canonical digest bindings.
}
const CAREER_PREVIEW_ERRORS = new Set([...PREVIEW_ERRORS, 'career_invalid', 'career_inactive', 'career_unfenced', 'career_closed', 'career_poisoned', 'career_cancelled', 'career_corrupt']);
const CAREER_PREVIEW_UNAVAILABLE = new Set(['', 'preview_unavailable', 'career_inactive', 'career_unfenced', 'career_closed', 'career_poisoned', 'career_corrupt']);
function careerPreviewActivationValue(a) {
  exact(a, ['version','policy_id','activation_id','result_store_id','result_created_at_ms','source_created_at_ms','activated_at_ms']);
  requireValue(a.version === 1 && a.policy_id === CAREER_POLICY && hex(a.activation_id) && hex(a.result_store_id) && int(a.source_created_at_ms,1,DATE_MAX) && int(a.result_created_at_ms,a.source_created_at_ms,DATE_MAX) && int(a.activated_at_ms,a.result_created_at_ms,DATE_MAX) && (!careerActivation || same(careerActivation,a)));
  if (publicationAuthority) requireValue(a.result_store_id === publicationAuthority.storeID && a.result_created_at_ms === publicationAuthority.createdMS && a.source_created_at_ms === publicationAuthority.sourceCreatedMS);
}
function careerPreviewFighter(i, st, asOf) {
  exact(i, ['assignment','manifest','totals','condition']); assignment(i.assignment, st.pin.identity); manifestValue(i.manifest, i.assignment.work_hash);
  const t=i.totals; exact(t,['wins','losses','draws','xp','base_points']);
  requireValue(['wins','losses','draws'].every(k=>int(t[k],0,256)) && t.wins+t.losses+t.draws<=256 && t.xp===Math.min(15,t.wins+t.losses+t.draws) && int(t.base_points,10*t.wins+4*t.draws,14*t.wins+4*t.draws) && (t.base_points-10*t.wins-4*t.draws)%4===0);
  careerCondition(i.condition,i,asOf,t.xp); requireValue(i.assignment.sequence <= st.checkpoint && entryBytes(i)<=8448);
}
function activePreviewEnvelope(v, kind) { return careerMode ? careerPreviewEnvelope(v, kind) : previewEnvelope(v, kind); }
function previewMethod(kind) { return ({snapshot:careerMode?'CareerPreviewSnapshot':'PreviewSnapshot',begin:careerMode?'BeginCareerFightPreview':'BeginFightPreview',job:careerMode?'CareerFightPreviewJob':'FightPreviewJob',cancel:careerMode?'CancelCareerFightPreview':'CancelFightPreview',portrait:careerMode?'CareerFightPreviewPortrait':'FightPreviewPortrait'})[kind]; }
function careerPreviewReady() { return !careerMode || !!(previewSnapshot?.view && previewSnapshot?.activation && previewSnapshot.state !== 'unavailable'); }
function careerPreviewEnvelope(v, kind) {
  requireValue(v !== null && typeof v === 'object' && JSON.stringify(v).length < 262144 && v.version === 1 && int(v.id, 1) && typeof v.ok === 'boolean');
  if (!v.ok) { exact(v, ['id', 'version', 'ok', 'error']); requireValue(CAREER_PREVIEW_ERRORS.has(v.error)); return { ok: false, error: v.error }; }
  exact(v, ['id', 'version', 'ok', kind]); const value = v[kind];
  if (kind === 'snapshot') {
    exact(value, ['version', 'state', 'error', 'view', 'store_id', 'engine_version', 'engine_id', 'activation', 'active_job_id', 'queued_job_id', 'last_job_id']);
    requireValue(value.version === 1 && ['unavailable', 'idle', 'busy', 'ready', 'failed', 'cancelled'].includes(value.state) && ['active_job_id', 'queued_job_id', 'last_job_id'].every((k) => optionalID(value[k])));
    if (value.view === null) requireValue(value.state === 'unavailable' && CAREER_PREVIEW_UNAVAILABLE.has(value.error) && value.activation === null && value.store_id === '' && value.engine_version === 0 && value.engine_id === '' && value.active_job_id === '' && value.queued_job_id === '' && value.last_job_id === '');
    else { previewPins(value); careerPreviewActivationValue(value.activation); requireValue(value.state !== 'unavailable' && (['failed', 'cancelled'].includes(value.state) ? CAREER_PREVIEW_ERRORS.has(value.error) && value.error !== 'outcome_unknown' : value.error === '')); requireValue(value.state === 'busy' ? !!(value.active_job_id || value.queued_job_id) && !(value.active_job_id && value.queued_job_id) && !value.last_job_id : !value.active_job_id && !value.queued_job_id); requireValue(value.state === 'idle' ? !value.last_job_id : value.state === 'busy' || !!value.last_job_id); if (value.state === 'cancelled') requireValue(value.error === 'preview_cancelled'); }
  } else {
    exact(value, ['version', 'job_id', 'state', 'view', 'store_id', 'engine_version', 'engine_id', 'activation_id', 'fighter_a_id', 'fighter_b_id', 'error', 'result']); previewPins(value); requireValue(hex(value.activation_id) && value.activation_id === (careerActivation?.activation_id ?? previewSnapshot?.activation?.activation_id));
    requireValue(hex(value.job_id) && hex(value.fighter_a_id) && hex(value.fighter_b_id) && value.fighter_a_id < value.fighter_b_id && ['queued', 'running', 'done', 'failed', 'cancelled'].includes(value.state));
    requireValue(['queued', 'running', 'done'].includes(value.state) ? value.error === '' : CAREER_PREVIEW_ERRORS.has(value.error) && value.error !== 'outcome_unknown'); if (value.state === 'cancelled') requireValue(value.error === 'preview_cancelled');
    requireValue((value.result !== null) === (value.state === 'done'));
    if (value.result !== null) {
      const r = value.result; exact(r, ['version', 'pair', 'recipe', 'seed', 'engine_result', 'engine_result_digest']); requireValue(r.version === 1 && hex(r.seed) && hex(r.engine_result_digest) && JSON.stringify(r).length <= 229376);
      const p = r.pair; exact(p, ['version', 'status', 'activation', 'career_digest', 'as_of_ms', 'fighters']); requireValue(p.version === 1 && int(p.as_of_ms, 1, DATE_MAX) && p.status.last_as_of_ms === p.as_of_ms && p.status.store_id === value.store_id && same(p.status.pin, stable.status.pin) && JSON.stringify(p).length <= 24576); status(p.status, value.view, current.account.fixture); careerPreviewActivationValue(p.activation); requireValue(p.activation.activation_id === value.activation_id && hex(p.career_digest) && p.as_of_ms >= p.activation.activated_at_ms); pairArray(p.fighters, (i) => careerPreviewFighter(i, p.status, p.as_of_ms));
      const ids = [value.fighter_a_id, value.fighter_b_id]; requireValue(p.fighters.every((i, n) => i.assignment.prospect_id === ids[n]) && p.fighters[0].manifest.physical.weight_class === p.fighters[1].manifest.physical.weight_class);
      const recipe = r.recipe; exact(recipe, ['version', 'engine_version', 'engine_id', 'network', 'genesis', 'store_id', 'source_authority_id', 'descriptor_id', 'activation_id', 'career_digest', 'as_of_ms', 'rounds', 'training', 'fighters']);
      requireValue(recipe.version === 1 && recipe.engine_version === 1 && recipe.engine_id === PREVIEW_ENGINE && recipe.network === 0x534c5547 && recipe.genesis === GENESIS && recipe.store_id === value.store_id && recipe.source_authority_id === p.status.pin.source.authority_id && recipe.descriptor_id === DESCRIPTOR && recipe.activation_id === value.activation_id && recipe.career_digest === p.career_digest && recipe.as_of_ms === p.as_of_ms && recipe.rounds === 3 && JSON.stringify(recipe).length <= 4096);
      pairArray(recipe.training, (x) => requireValue(x === 0)); pairArray(recipe.fighters, (f, n) => { exact(f, ['prospect_id', 'source_record_digest', 'manifest_digest', 'condition_digest', 'totals']); const a = p.fighters[n].assignment; requireValue(f.prospect_id === ids[n] && f.source_record_digest === a.source_record_digest && f.manifest_digest === a.manifest_digest && hex(f.condition_digest) && same(f.totals, p.fighters[n].totals)); }); boutValue(r.engine_result, ids);
    }
  }
  return { ok: true, value: own(value) }; // Native checks canonical digest bindings.
}
function previewPins(v) { requireValue(v.version === 1 && ready() && viewKey(checkedView(v.view)) === currentKey() && v.store_id === stable.status.store_id && v.engine_version === 1 && v.engine_id === PREVIEW_ENGINE); }

let previewSnapshot = null, previewSelected = [], previewWatch = null, previewDiscovery = null, previewResult = null, previewSerial = 0, previewMessage = '';
let playbackTimer = null, playbackIndex = 0, playbackSteps = [], previewImages = new Map(), previewImageAttempts = new Set(), previewRendered = '';
const motionPreference = typeof window.matchMedia === 'function' ? window.matchMedia('(prefers-reduced-motion: reduce)') : null;
function pausePreview() { if (playbackTimer !== null) clearTimeout(playbackTimer); playbackTimer = null; }
function discardPreviewResult() { pausePreview(); previewResult = null; playbackSteps = []; playbackIndex = 0; previewImages.clear(); previewImageAttempts.clear(); previewRendered = ''; ui['preview-fighters'].textContent = ''; ui['preview-events'].textContent = ''; ui['preview-scores'].textContent = ''; }
function clearPreview() { previewSerial++; discardPreviewResult(); previewSelected = []; previewWatch = null; previewDiscovery = null; previewSnapshot = null; previewMessage = ''; ui['preview-selection'].textContent = ''; }
function previewRetainedBytes() { return JSON.stringify(previewSelected).length + (previewResult ? JSON.stringify(previewResult).length + 65536 : 0) + [...previewImages.values()].reduce((n, src) => n + 2 * src.length, 0); }
function previewCapacity(extra = 0) { return careerRetainedBytes() + previewRetainedBytes() + entryRetainedBytes() + eventRetainedBytes() + seedRetainedBytes() + cardRetainedBytes() + bookedRetainedBytes() + publicationRetainedBytes() + historyRetainedBytes() + JSON.stringify(pageItems).length + [...images.values()].reduce((n, src) => n + 2 * src.length, 0) + extra <= CACHE_MAX; }
function previewError(code) { return ({ preview_class_mismatch: 'Choose two candidates from the same weight class.', preview_not_found: 'This preview is no longer retained. Select your pair and explicitly preview again.', preview_capacity: 'The local preview cache is full. Clear the preview and retry.', preview_cancelled: 'Preview cancelled. No fight was recorded.', preview_observation_unknown: 'The authority could not confirm its clock observation. Reopen the gym before another preview. No fight was recorded.', outcome_unknown: 'The preview may have started. Check its status before explicitly trying again.', preview_unavailable: 'Fight preview is unavailable. Your saved candidates are retained.' })[code] ?? textError(code); }
function previewRefusal(code) {
  if (['unauthorized', 'stale_view'].includes(code)) clearPreview();
  else if (['preview_unavailable', 'preview_not_found', 'preview_observation_unknown', 'career_inactive', 'career_unfenced', 'career_closed', 'career_poisoned', 'career_corrupt'].includes(code)) {
    discardPreviewResult(); previewWatch = null; previewDiscovery = null; previewSnapshot = null;
  }
  previewMessage = previewError(code);
}
function selectionChanged(next) {
  const old = previewWatch, oldView = current?.view?.view_id;
  previewSerial++; previewSelected = next; previewWatch = null; previewDiscovery = null; discardPreviewResult(); previewMessage = '';
  if (old && oldView) tasks.unshift(async () => { if (ready() && current.view.view_id === oldView) { try { await call(previewMethod('cancel'), oldView, old.id); } catch {} } });
  render(); pump();
}
function selectPreview(i) {
  if (careerMode && (!careerForItem(i) || !careerPreviewReady())) { previewMessage = 'Career practice is unavailable. Refresh the career page and try again.'; renderPreview(); return; }
  if (!ready() || command || previewSelected.length >= 2 || previewSelected.some((v) => v.id === i.assignment.prospect_id)) return;
  const next = { id: i.assignment.prospect_id, name: i.manifest.name.display, weightClass: i.manifest.physical.weight_class };
  if (previewSelected.length && previewSelected[0].weightClass !== next.weightClass) { previewMessage = previewError('preview_class_mismatch'); renderPreview(); return; }
  selectionChanged([...previewSelected, next]);
}
async function fetchPreviewSnapshot() {
  const captured = epoch, serial = previewSerial, view = currentKey();
  try {
    const r = activePreviewEnvelope(await call(previewMethod('snapshot')), 'snapshot'); if (disposed || captured !== epoch || view !== currentKey()) return;
    if (!r.ok) { previewRefusal(r.error); renderPreview(); return; }
    previewSnapshot = r.value;
    if (r.value.view === null) { const code=r.value.error; clearPreview(); if(code) previewMessage=previewError(code); renderPreview(); return; }
    if (careerMode) careerActivation=own(r.value.activation);
    // Reconcile the replay present at this observation before awaiting a job.
    // A busy snapshot can precede a done job; it cannot evict that newer reply.
    if (previewResult && r.value.last_job_id !== previewResult.job_id) discardPreviewResult();
    // Discovery is only used for the exact still-selected pair and epoch.
    const id = previewWatch?.id || (previewDiscovery?.serial === previewSerial ? r.value.active_job_id || r.value.queued_job_id || r.value.last_job_id : '');
    if (id && serial === previewSerial && previewResult?.job_id !== id && !tasks.length) await fetchPreviewJob(id, serial);
    renderPreview();
  } catch { if (!disposed && captured === epoch && view === currentKey()) { discardPreviewResult(); previewSnapshot = null; previewMessage = previewError('preview_unavailable'); renderPreview(); } }
}
async function fetchPreviewJob(id, serial = previewSerial) {
  if (!ready() || previewSelected.length !== 2) return;
  const captured = epoch, view = currentKey(), ids = previewSelected.map((s) => s.id).sort();
  const r = activePreviewEnvelope(await call(previewMethod('job'), current.view.view_id, id), 'job');
  if (disposed || captured !== epoch || serial !== previewSerial || view !== currentKey()) return;
  if (!r.ok) { previewWatch = null; previewRefusal(r.error); return; }
  const j = r.value; requireValue(j.job_id === id);
  if (j.fighter_a_id !== ids[0] || j.fighter_b_id !== ids[1]) return;
  if (['queued', 'running'].includes(j.state)) { previewWatch = { id, serial }; previewDiscovery = null; return; }
  previewWatch = null; previewDiscovery = null;
  if (j.state !== 'done') { previewMessage = previewError(j.error); return; }
  discardPreviewResult(); previewResult = j; requireValue(previewCapacity());
  playbackSteps = j.result.engine_result.rounds.flatMap((round) => round.ticks.map((tick) => ({ round: round.number, tick })));
  previewMessage = 'Preview ready. Playback is paused; use Play or step through the fight.'; content(ui.announcement, 'Unrecorded fight preview ready.');
}
async function beginPreview() {
  if (!ready() || previewSelected.length !== 2 || previewWatch || !previewSnapshot?.view || !careerPreviewReady()) return;
  const captured = epoch, serial = previewSerial, view = currentKey(), viewID = current.view.view_id, ids = previewSelected.map((s) => s.id).sort();
  previewDiscovery = { serial, ids };
  const r = activePreviewEnvelope(await call(previewMethod('begin'), viewID, ...ids), 'job');
  if (disposed || captured !== epoch || serial !== previewSerial || view !== currentKey()) { if (!disposed && r.ok && ready() && current.view.view_id === viewID) await call(previewMethod('cancel'), viewID, r.value.job_id); return; }
  if (!r.ok) { previewRefusal(r.error); if (r.error === 'outcome_unknown') discardPreviewResult(); else previewDiscovery = null; return; }
  requireValue(r.value.state === 'queued' && r.value.fighter_a_id === ids[0] && r.value.fighter_b_id === ids[1]); discardPreviewResult(); previewWatch = { id: r.value.job_id, serial }; previewDiscovery = null; previewMessage = 'Preparing fight…'; pollWanted = true;
}
async function cancelPreview() {
  if (!ready() || !previewWatch) return; const captured = epoch, serial = previewSerial, id = previewWatch.id;
  const r = activePreviewEnvelope(await call(previewMethod('cancel'), current.view.view_id, id), 'job');
  if (captured !== epoch || serial !== previewSerial) return;
  if (!r.ok) previewRefusal(r.error);
  else { requireValue(r.value.job_id === id); previewMessage = 'Cancellation requested. No fight is recorded.'; if (!['queued', 'running'].includes(r.value.state)) previewWatch = null; }
  pollWanted = true;
}
function previewNextPortrait() { return previewResult?.result.pair.fighters.find((i) => !previewImageAttempts.has(i.assignment.prospect_id)) ?? null; }
async function loadPreviewPortrait(item) {
  const captured = epoch, serial = previewSerial, j = previewResult, a = item.assignment; if (!j) return; previewImageAttempts.add(a.prospect_id);
  const remains = () => !disposed && captured === epoch && serial === previewSerial && previewResult === j && ready();
  try {
    const r = await call(previewMethod('portrait'), j.view.view_id, j.job_id, a.prospect_id); if (!remains()) return;
    requireValue(r?.version === 1 && typeof r.ok === 'boolean' && JSON.stringify(r).length <= 98304);
    if (!r.ok) { exact(r, ['version', 'ok', 'error']); requireValue((careerMode?CAREER_PREVIEW_ERRORS:PREVIEW_ERRORS).has(r.error)); previewRefusal(r.error); renderPreview(); return; }
    exact(r, ['version', 'ok', 'portrait']); const p = r.portrait;
    exact(p, ['version', 'view', 'store_id', 'job_id', 'prospect_id', ...(careerMode?['activation_id','career_digest']:[]), 'result_digest', 'descriptor_id', 'manifest_digest', 'portrait_digest', 'svg']);
    if(careerMode) requireValue(p.activation_id===j.activation_id && p.career_digest===j.result.pair.career_digest);
    requireValue(p.version === 1 && viewKey(checkedView(p.view)) === currentKey() && p.store_id === j.store_id && p.job_id === j.job_id && p.prospect_id === a.prospect_id && p.result_digest === j.result.engine_result_digest && p.descriptor_id === DESCRIPTOR && p.manifest_digest === a.manifest_digest && p.portrait_digest === a.portrait_digest && imageOnlySVG(p.svg));
    const src = `data:image/svg+xml;base64,${btoa(p.svg)}`; requireValue(previewCapacity(2 * src.length) && previewImages.size < 2); if (!remains()) return; previewImages.set(a.prospect_id, src); renderPreview();
  } catch { if (remains()) { previewRefusal('preview_unavailable'); renderPreview(); } }
}
function stepPreview(n) { pausePreview(); playbackIndex = Math.max(0, Math.min(playbackSteps.length, n)); renderPreview(); }
function playPreview() {
  if (!previewResult || motionPreference?.matches || document.hidden) return;
  if (playbackTimer !== null) { pausePreview(); renderPreview(); return; }
  if (playbackIndex >= playbackSteps.length) playbackIndex = 0;
  const serial = previewSerial;
  const tick = () => { playbackTimer = null; if (disposed || serial !== previewSerial || !previewResult || document.hidden || motionPreference?.matches) return; playbackIndex++; if (playbackIndex < playbackSteps.length) playbackTimer = setTimeout(tick, 250); renderPreview(); };
  playbackTimer = setTimeout(tick, 250); renderPreview();
}
function renderPreview() {
  content(ui['preview-state'], previewWatch ? 'Preparing fight' : previewResult ? 'Unrecorded preview · Ready to replay' : 'Choose two candidates'); content(ui['preview-message'], previewMessage);
  const selectionKey = JSON.stringify(previewSelected);
  if (ui['preview-selection']._selectionKey !== selectionKey) {
    ui['preview-selection']._selectionKey = selectionKey; ui['preview-selection'].textContent = '';
    for (const s of previewSelected) { const li = element('li', '', `${s.name} · ${label(s.weightClass)}`), remove = element('button', '', 'Remove'); remove.type = 'button'; remove.setAttribute('aria-label', `Remove ${s.name} from preview`); remove.addEventListener('click', () => selectionChanged(previewSelected.filter((x) => x.id !== s.id))); li.append(remove); ui['preview-selection'].append(li); }
  }
  ui['preview-begin'].disabled = !ready() || !previewSnapshot?.view || !careerPreviewReady() || previewSelected.length !== 2 || !!previewWatch || !!command;
  ui['preview-cancel'].disabled = !ready() || !previewWatch || !!command; ui['preview-clear'].disabled = previewSelected.length === 0;
  ui['preview-replay'].hidden = !previewResult; if (!previewResult) return;
  const j = previewResult, r = j.result.engine_result, fighters = j.result.pair.fighters, names = fighters.map((i) => i.manifest.name.display);
  content(ui['preview-asof'], `Both conditions observed ${date.format(new Date(j.result.pair.as_of_ms))}. This replay keeps that observation fixed.`);
  const rendered = j.result.engine_result_digest + JSON.stringify([...previewImages.keys()]);
  if (previewRendered !== rendered) {
    previewRendered = rendered; ui['preview-fighters'].textContent = '';
    for (const [n, i] of fighters.entries()) { const box = element('div', 'preview-fighter'), src = previewImages.get(i.assignment.prospect_id); if (src) { const image = element('img'); image.setAttribute('src', src); image.setAttribute('alt', `Candidate portrait of ${names[n]}`); image.setAttribute('width', '160'); image.setAttribute('height', '160'); box.append(image); } box.append(element('strong', '', names[n]), element('span', 'note', `${label(i.condition.phase)} · Rating ${(i.condition.rating_hundredths / 100).toFixed(2)} · XP ${i.condition.pro_xp}`)); ui['preview-fighters'].append(box); }
    ui['preview-scores'].textContent = ''; ui['preview-scores'].append(element('caption', '', 'Unrecorded preview · Whole-fight scorecard')); const head = element('tr'); for (const title of ['Component', ...names]) { const th = element('th', '', title); th.setAttribute('scope', 'col'); head.append(th); } const thead = element('thead'); thead.append(head); ui['preview-scores'].append(thead); const body = element('tbody');
    for (const [key, title] of [['damage_tenths', 'Damage'], ['knockdown_tenths', 'Knockdowns'], ['accuracy_tenths', 'Accuracy'], ['volume_tenths', 'Volume'], ['control_tenths', 'Control'], ['total_tenths', 'Total']]) { const row = element('tr'), th = element('th', '', title); th.setAttribute('scope', 'row'); row.append(th, ...r.scores.map((score) => element('td', '', (score[key] / 10).toFixed(1)))); body.append(row); } ui['preview-scores'].append(body);
  }
  content(ui['preview-play'], playbackTimer === null ? 'Play' : 'Pause'); ui['preview-play'].disabled = !!motionPreference?.matches; ui['preview-previous'].disabled = playbackIndex === 0; ui['preview-next'].disabled = playbackIndex >= playbackSteps.length;
  content(ui['preview-motion'], motionPreference?.matches ? 'Reduced motion is on. Use Previous step, Next step or Show result.' : 'Playback is local. Pause or step through at any time.');
  const last = playbackSteps[playbackIndex - 1]; content(ui['preview-step'], last ? `Round ${last.round}, tick ${last.tick.number} · Step ${playbackIndex} of ${playbackSteps.length}` : 'Ready at the opening bell.');
  ui['preview-events'].textContent = '';
  for (const event of playbackSteps.slice(0, playbackIndex)) { const t = event.tick; ui['preview-events'].append(element('li', '', `Round ${event.round}, tick ${t.number}: ${names[t.actor]} ${t.hit ? 'lands' : 'misses'} a ${t.strike}.${t.knockdown ? ' Knockdown.' : ''}${t.stoppage ? ` ${label(t.stoppage)}.` : ''}`)); }
  ui['preview-outcome'].hidden = playbackIndex < playbackSteps.length;
  const winner = fighters.find((i) => i.assignment.prospect_id === r.winner)?.manifest.name.display;
  content(ui['preview-finish'], `Unrecorded preview: ${r.method === 'decision_draw' ? 'Draw — equal whole-fight scores' : r.method === 'double_corner_stoppage' ? 'Draw — both corners stopped the fight' : `${winner} wins by ${label(r.method)}.`}`);
}
motionPreference?.addEventListener('change', () => { pausePreview(); renderPreview(); });
// Candidate entry is a separate closed family on the same serialized scheduler.
const ENTRY_CALENDAR = '1351d63c43cd08d760ca4a88c60bc7ecee9dcb0e331961f93b16d95425385450';
const ENTRY_RULES = 'fdd7e3f43df60986cea1d5eeb8a0f3512c30c73229890b84a213d057b16b44e2';
const ENTRY_MAX = 131072;
const ENTRY_ERRORS = new Set([...CAREER_ACTION_ERRORS, 'invalid_request', 'unauthorized', 'forbidden', 'stale_view', 'busy', 'entry_unavailable', 'entry_corrupt', 'entry_capacity', 'entry_clock', 'entry_calendar', 'entry_closed', 'entry_conflict', 'entry_pending', 'entry_not_found', 'entry_sync_required', 'entry_not_rostered', 'entry_retired', 'entry_class_conflict', 'entry_cancelled', 'outcome_unknown']);
let entrySnapshot = null, entryStore = '', entryNamespace = '', entryUnavailable = false, entryDate = '', entryStatus = null;
let entryDraft = [], entrySerial = 0, entryWatch = null, entryFlow = null, entryOperation = null, entryTerminal = null, entryHistory = null, entryMessage = '', entryUnknown = false;
function entryReady() { return ready() && !entryUnavailable && !!entrySnapshot?.view && entryStore === entrySnapshot.store_id; }
function entryBytes(value) { return JSON.stringify(value).length; } // All validated text is bounded ASCII.
function entryRetainedBytes() { return entryBytes([entrySnapshot, entryDate, entryStatus, entryDraft, entryWatch, entryFlow, entryOperation, entryTerminal, entryHistory, entryMessage]) + (entryWatch ? 65536 : 0); }
function entryCapacity() { return entryRetainedBytes() <= ENTRY_MAX && previewCapacity(); }
function clearEntry() {
  entrySerial++; entrySnapshot = null; entryStore = entryNamespace = entryDate = ''; entryUnavailable = false; entryStatus = null; entryDraft = []; entryWatch = entryFlow = entryOperation = entryTerminal = entryHistory = null; entryUnknown = false; entryMessage = '';
  for (const id of ['entry-draft', 'entry-eligibility', 'entry-history-list']) ui[id].textContent = '';
}
function entryDateValue(s) {
  requireValue(typeof s === 'string' && /^[0-9]{4}-[0-9]{2}-[0-9]{2}$/.test(s)); const d = new Date(`${s}T00:00:00.000Z`);
  requireValue(!Number.isNaN(d.getTime()) && d.toISOString().slice(0, 10) === s && d.getUTCFullYear() >= 2026 && d.getUTCFullYear() <= 2099 && d.getUTCDay() === 6); return s;
}
function entryIDs(ids, min = 0) { requireValue(Array.isArray(ids) && ids.length >= min && ids.length <= 5 && ids.every(hex) && ids.every((id, n) => n === 0 || ids[n - 1] < id)); }
function entryEvent(e) {
  exact(e, ['version', 'event_id', 'namespace_id', 'calendar_id', 'rules_id', 'saturday', 'cutoff_ms']);
  requireValue(e.version === 1 && hex(e.event_id) && hex(e.namespace_id) && (!entryNamespace || e.namespace_id === entryNamespace) && e.calendar_id === ENTRY_CALENDAR && e.rules_id === ENTRY_RULES && int(e.cutoff_ms, 1, DATE_MAX)); entryDateValue(e.saturday); return e;
}
function entryRequestValue(r) {
  exact(r, ['version', 'event_id', 'saturday', 'action', 'expected_revision', 'expected_roster_revision', 'prospect_ids']);
  requireValue(r.version === 1 && hex(r.event_id) && ['replace', 'withdraw'].includes(r.action) && int(r.expected_revision, 0, 4096) && int(r.expected_roster_revision, 0, 4096)); entryDateValue(r.saturday); entryIDs(r.prospect_ids, r.action === 'replace' ? 1 : 0);
  if (r.action === 'withdraw') requireValue(r.prospect_ids.length === 0 && r.expected_roster_revision === 0); return r;
}
function entryIntent(i) {
  exact(i, ['version', 'sequence', 'operation_id', 'generation', 'view_id', 'request', 'prepared_at_ms']);
  requireValue(i.version === 1 && int(i.sequence, 1, 4096) && hex(i.operation_id) && int(i.generation, 1) && hex(i.view_id) && int(i.prepared_at_ms, 1, DATE_MAX)); entryRequestValue(i.request); return i;
}
function entrySavedCondition(c, a, asOf) {
  exact(c, ['version', 'projector_id', 'dna_version', 'seed', 'born_at_ms', 'as_of_ms', 'age_weeks', 'phase', 'elapsed_decline_weeks', 'prime_remaining_weeks', 'prime_remaining_ms', 'career_remaining_ms', 'pro_xp', 'aged_attributes_tenths', 'league_attributes_tenths', 'rating_hundredths']);
  requireValue(c.version === 1 && c.projector_id === 'slug-fighter-condition-candidate-v1' && c.dna_version === 1 && c.seed === a.work_hash && c.born_at_ms === a.born_at_ms && c.as_of_ms === asOf && asOf >= a.accepted_at_ms && c.pro_xp === 0 && ['prime', 'decline'].includes(c.phase));
  requireValue(int(c.age_weeks, 0, 91) && c.age_weeks === Math.floor((asOf - a.born_at_ms) / WEEK) && int(c.elapsed_decline_weeks, 0, 39) && int(c.prime_remaining_weeks, 0, 52) && int(c.prime_remaining_ms, 0, 52 * WEEK) && int(c.career_remaining_ms, 1, 91 * WEEK));
  exact(c.aged_attributes_tenths, attrs); exact(c.league_attributes_tenths, attrs); requireValue(attrs.every((k) => int(c.aged_attributes_tenths[k], 0, 1000) && c.league_attributes_tenths[k] === c.aged_attributes_tenths[k])); requireValue(c.rating_hundredths === attrs.reduce((sum, k, n) => sum + c.league_attributes_tenths[k] * [10, 25, 2, 4, 8][n], 0));
}
function entryReceipt(r) {
  exact(r, ['version', 'store_id', 'entry_id', 'event', 'operation_id', 'operation_sequence', 'generation', 'view_id', 'action', 'previous_revision', 'revision', 'roster_revision', 'stable_checkpoint', 'observed_source_count', 'committed_at_ms', 'members']); entryEvent(r.event);
  requireValue(r.version === 1 && r.store_id === entryStore && ['entry_id', 'operation_id', 'view_id'].every((k) => hex(r[k])) && int(r.operation_sequence, 1, 4096) && int(r.generation, 1) && ['replace', 'withdraw'].includes(r.action) && int(r.previous_revision, 0, 4095) && r.revision === r.previous_revision + 1 && int(r.roster_revision, 0, 4096) && int(r.stable_checkpoint, 0, 1000) && int(r.observed_source_count, r.stable_checkpoint, 1000) && int(r.committed_at_ms, 1, r.event.cutoff_ms - 1));
  requireValue(Array.isArray(r.members) && r.members.length <= 5 && (r.action === 'replace' ? r.members.length > 0 : r.members.length === 0 && r.roster_revision === 0)); const seenSlots = new Set(), seenClasses = new Set(); let previous = '';
  for (const m of r.members) { exact(m, ['slot', 'assignment', 'class', 'condition_at_save']); assignment(m.assignment, stable.status.pin.identity); requireValue(int(m.slot, 1, 10) && classes.includes(m.class) && !seenSlots.has(m.slot) && !seenClasses.has(m.class) && m.assignment.prospect_id > previous && m.assignment.sequence <= r.stable_checkpoint); seenSlots.add(m.slot); seenClasses.add(m.class); previous = m.assignment.prospect_id; entrySavedCondition(m.condition_at_save, m.assignment, r.committed_at_ms); }
  return r;
}
function entryOperationValue(o) {
  exact(o, ['version', 'intent', 'state', 'receipt', 'abandoned_at_ms']); requireValue(o.version === 1 && ['prepared', 'committed', 'abandoned'].includes(o.state)); const i = entryIntent(o.intent);
  if (o.state === 'committed') { const r = entryReceipt(o.receipt), q = i.request; requireValue(r.operation_id === i.operation_id && r.operation_sequence === i.sequence && r.generation === i.generation && r.view_id === i.view_id && r.event.event_id === q.event_id && r.event.saturday === q.saturday && r.action === q.action && r.previous_revision === q.expected_revision && r.roster_revision === q.expected_roster_revision && same(r.members.map((m) => m.assignment.prospect_id), q.prospect_ids) && r.committed_at_ms >= i.prepared_at_ms && o.abandoned_at_ms === 0); }
  else requireValue(o.receipt === null && (o.state === 'prepared' ? o.abandoned_at_ms === 0 : int(o.abandoned_at_ms, i.prepared_at_ms, DATE_MAX))); return o;
}
function entryStatusValue(s) {
  exact(s, ['version', 'store_id', 'event', 'exists', 'phase', 'entry_id', 'revision', 'latest_operation_id', 'latest_receipt_digest', 'saved_ids', 'saved_at_ms', 'saved_roster_revision', 'current_roster_revision', 'roster_changed', 'eligibility', 'pending_operation_id', 'observed_at_ms', 'closed_observed_at_ms']); entryEvent(s.event); entryIDs(s.saved_ids);
  requireValue(s.version === 1 && s.store_id === entryStore && typeof s.exists === 'boolean' && s.phase === (s.observed_at_ms >= s.event.cutoff_ms ? 'closed_pending_lock' : 'accepting') && hex(s.entry_id) && int(s.revision, 0, 4096) && optionalID(s.latest_operation_id) && optionalID(s.latest_receipt_digest) && optionalID(s.pending_operation_id) && int(s.saved_at_ms, 0, s.event.cutoff_ms - 1) && int(s.observed_at_ms, Math.max(s.saved_at_ms, 1), DATE_MAX) && int(s.saved_roster_revision, 0, 4096) && int(s.current_roster_revision, 0, 4096) && s.roster_changed === !!(s.saved_ids.length && s.saved_roster_revision !== s.current_roster_revision) && s.eligibility === 'not_checked');
  requireValue(s.revision ? s.exists && hex(s.latest_operation_id) && hex(s.latest_receipt_digest) && s.saved_at_ms > 0 : s.latest_operation_id === '' && s.latest_receipt_digest === '' && s.saved_ids.length === 0 && s.saved_at_ms === 0 && s.saved_roster_revision === 0);
  requireValue(!s.exists || s.phase === 'accepting' ? s.closed_observed_at_ms === 0 : int(s.closed_observed_at_ms, s.event.cutoff_ms, s.observed_at_ms)); if (!s.exists) requireValue(s.revision === 0); return s;
}
function entryCheckValue(c) {
  exact(c, ['version', 'status', 'as_of_ms', 'roster_revision', 'members']); entryStatusValue(c.status); requireValue(c.version === 1 && c.as_of_ms === c.status.observed_at_ms && c.roster_revision === c.status.current_roster_revision && Array.isArray(c.members) && c.members.length === c.status.saved_ids.length);
  for (const [n, m] of c.members.entries()) { exact(m, ['slot', 'item', 'eligibility']); requireValue(int(m.slot, 0, 10)); fullItem(m.item, { pin: stable.status.pin, checkpoint: 1000 }, c.as_of_ms); requireValue(m.item.assignment.prospect_id === c.status.saved_ids[n] && m.eligibility === (m.item.condition.phase === 'retired' ? 'retired' : m.slot ? 'eligible' : 'not_rostered')); } return c;
}
function entryJobValue(j) {
  careerJobFields(j, ['version', 'job_id', 'kind', 'state', 'view', 'store_id', 'event_id', 'saturday', 'operation_id', 'error', 'intent', 'mutation', 'check']);
  requireValue(j.version === 1 && hex(j.job_id) && ['prepare', 'apply', 'check'].includes(j.kind) && ['queued', 'running', 'complete', 'failed', 'cancelled'].includes(j.state) && viewKey(checkedView(j.view)) === currentKey() && j.store_id === entryStore && hex(j.event_id) && optionalID(j.operation_id) && (j.error === '' || ENTRY_ERRORS.has(j.error))); entryDateValue(j.saturday);
  const field = { prepare: 'intent', apply: 'mutation', check: 'check' }[j.kind]; for (const k of ['intent', 'mutation', 'check']) requireValue((j[k] !== null) === (j.state === 'complete' && k === field));
  if (j.state === 'complete') {
    requireValue(j.error === '');
    if (j.kind === 'prepare') { const i = entryIntent(j.intent); requireValue(i.operation_id === j.operation_id && i.view_id === j.view.view_id && i.generation === j.view.generation && i.request.event_id === j.event_id && i.request.saturday === j.saturday); }
    if (j.kind === 'apply') { const m = j.mutation; exact(m, ['version', 'receipt', 'replayed']); requireValue(m.version === 1 && typeof m.replayed === 'boolean'); const r = entryReceipt(m.receipt); requireValue(r.operation_id === j.operation_id && r.view_id === j.view.view_id && r.generation === j.view.generation && r.event.event_id === j.event_id && r.event.saturday === j.saturday); }
    if (j.kind === 'check') { entryCheckValue(j.check); requireValue(j.check.status.event.event_id === j.event_id && j.check.status.event.saturday === j.saturday); }
  } else if (['queued', 'running'].includes(j.state)) requireValue(j.error === '' && (j.kind !== 'prepare' || j.operation_id === ''));
  return j;
}
function entryEnvelope(r, field, allowNull = false) {
  requireValue(entryBytes(r) <= ENTRY_MAX); exact(r, ['id', 'ok', 'view', 'store_id', 'snapshot', 'status', 'history', 'job', 'operation', 'error']); requireValue(int(r.id, 1) && typeof r.ok === 'boolean'); const fields = ['snapshot', 'status', 'history', 'job', 'operation'];
  if (!r.ok) { requireValue(r.view === null && r.store_id === '' && fields.every((k) => r[k] === null) && ENTRY_ERRORS.has(r.error)); return { ok: false, error: r.error }; }
  requireValue(r.error === '' && fields.every((k) => (r[k] !== null) === (k === field)));
  if (field === 'snapshot' && r.snapshot.state === 'unavailable') { const s = r.snapshot; exact(s, ['version', 'state', 'error', 'view', 'store_id', 'pending_operation_id', 'active_job_id', 'queued_job_id', 'last_job_id']); requireValue(s.version === 1 && r.view === null && r.store_id === '' && s.view === null && ['store_id', 'pending_operation_id', 'active_job_id', 'queued_job_id', 'last_job_id'].every((k) => s[k] === '') && (s.error === '' || ENTRY_ERRORS.has(s.error))); return { ok: true, value: own(s) }; }
  requireValue(ready() && viewKey(checkedView(r.view)) === currentKey() && hex(r.store_id) && (!entryStore || entryStore === r.store_id));
  // Native derives the entry Pin/ID from its authenticated stable Pin; browser hashing is not a second authority.
  if (!entryStore) { requireValue(field === 'snapshot'); entryStore = r.store_id; }
  const v = r[field];
  if (field === 'snapshot') { exact(v, ['version', 'state', 'error', 'view', 'store_id', 'pending_operation_id', 'active_job_id', 'queued_job_id', 'last_job_id']); requireValue(v.version === 1 && ['idle', 'busy', 'failed'].includes(v.state) && (v.error === '' || ENTRY_ERRORS.has(v.error)) && viewKey(checkedView(v.view)) === currentKey() && v.store_id === entryStore && ['pending_operation_id', 'active_job_id', 'queued_job_id', 'last_job_id'].every((k) => optionalID(v[k])) && (!v.active_job_id || v.active_job_id !== v.queued_job_id)); }
  if (field === 'status') entryStatusValue(v);
  if (field === 'job') entryJobValue(v);
  if (field === 'operation') { exact(v, ['version', 'store_id', 'operation']); requireValue(v.version === 1 && v.store_id === entryStore && (v.operation !== null || allowNull)); if (v.operation !== null) entryOperationValue(v.operation); }
  if (field === 'history') { exact(v, ['version', 'store_id', 'after_event_id', 'next_event_id', 'has_more', 'observed_at_ms', 'events']); requireValue(v.version === 1 && v.store_id === entryStore && optionalID(v.after_event_id) && optionalID(v.next_event_id) && typeof v.has_more === 'boolean' && int(v.observed_at_ms, 1, DATE_MAX) && Array.isArray(v.events) && v.events.length <= 8); for (const [n, e] of v.events.entries()) { entryStatusValue(e); requireValue(e.exists && e.observed_at_ms === v.observed_at_ms && (!n || v.events[n - 1].event.cutoff_ms < e.event.cutoff_ms)); } requireValue(v.next_event_id === (v.has_more && v.events.length ? v.events.at(-1).event.event_id : '')); }
  return { ok: true, value: own(v) };
}
function entryError(code) { return ({ busy: 'Another local task is finishing. Your entry choice is retained.', entry_unavailable: 'Saturday entry is unavailable in this app route.', entry_corrupt: 'The local entry history could not be validated. Reopen the gym; saved evidence is retained.', entry_capacity: 'The supported entry capacity is full. Saved outcomes remain available.', entry_clock: 'The authority clock needs attention. No new entry was confirmed.', entry_calendar: 'Choose a Saturday from 2026 through 2099.', entry_closed: 'Entry intake is closed. The Saturday snapshot uses the state at noon Pacific.', entry_conflict: 'The entry or roster changed. Refresh and review before a new save.', entry_pending: 'A prepared entry choice needs review before another change.', entry_not_found: 'That recorded outcome is unavailable. Refresh status and history before another explicit choice.', entry_sync_required: 'Sync saved candidates before checking this selection.', entry_not_rostered: 'Every selected fighter must be on your roster when the entry is saved.', entry_retired: 'A selected fighter has retired. Review the draft before another save.', entry_class_conflict: 'Choose at most one fighter from each weight class.', entry_cancelled: 'The task was cancelled. A prepared or committed choice may still be retained.', outcome_unknown: 'This change may have been recorded. Check the recorded outcome before another choice.' })[code] ?? textError(code); }
function entryRefusal(code) { if (['unauthorized', 'stale_view'].includes(code)) { clearEntry(); entryUnavailable = true; } if (code === 'entry_unavailable') { entryUnavailable = true; entrySnapshot = null; } entryMessage = entryError(code); }
function entryContext(serial = entrySerial) { return { epoch, view: currentKey(), viewID: current?.view?.view_id, serial }; }
function entryCurrent(c, selection = false) { return !disposed && ready() && c.epoch === epoch && c.view === currentKey() && (!selection || c.serial === entrySerial); }
function entryChanged() { entrySerial++; if (entryFlow) entryFlow.auto = false; }
function selectEntry(i) {
  if (careerMode && (!careerForItem(i) || careerForItem(i).totals.xp > 0)) { entryMessage = 'C1 bookings require a zero professional record. This fighter remains in your inventory.'; renderEntry(); return; }
  if (!entryReady() || command || entryDraft.length >= 5 || entryDraft.some((v) => v.id === i.assignment.prospect_id) || !stable.status.slots.includes(i.assignment.prospect_id) || i.condition.phase === 'retired') return;
  if (entryDraft.some((v) => v.weightClass === i.manifest.physical.weight_class)) { entryMessage = entryError('entry_class_conflict'); renderEntry(); return; }
  entryChanged(); entryDraft = [...entryDraft, { id: i.assignment.prospect_id, name: i.manifest.name.display, weightClass: i.manifest.physical.weight_class }].sort((a, b) => a.id.localeCompare(b.id)); entryMessage = 'Draft updated. Your saved entry has not changed.'; requireValue(entryCapacity()); render();
}
function entryAdoptStatus(s) {
  if (s.event.saturday !== entryDate) return;
  if (entryStatus && entryStatus.event.event_id === s.event.event_id) { requireValue(s.revision >= entryStatus.revision && s.observed_at_ms >= entryStatus.observed_at_ms); if (s.revision === entryStatus.revision) requireValue(s.latest_operation_id === entryStatus.latest_operation_id && s.latest_receipt_digest === entryStatus.latest_receipt_digest && same(s.saved_ids, entryStatus.saved_ids)); }
  const previous = entryStatus, namespace = entryNamespace; entryNamespace = s.event.namespace_id; entryStatus = s; if (!entryCapacity()) { entryStatus = previous; entryNamespace = namespace; throw new Error('Entry cache full'); }
}
async function entryReadStatus(upcoming = false) {
  if (!entryReady()) return; const c = entryContext(), requested = entryDate;
  try { const r = entryEnvelope(await call(upcoming ? 'EntryUpcoming' : 'EntryStatus', c.viewID, ...(upcoming ? [] : [requested])), 'status'); if (!entryCurrent(c, true)) return; if (!r.ok) { entryRefusal(r.error); return; } requireValue(upcoming ? r.value.event.cutoff_ms > r.value.observed_at_ms : r.value.event.saturday === requested); if (upcoming) { entryChanged(); entryDate = r.value.event.saturday; eventSelectDate(entryDate); entryStatus = null; } entryAdoptStatus(r.value); }
  catch { if (entryCurrent(c)) entryMessage = entryError('entry_unavailable'); } render();
}
async function entryQuery(operationID = '') {
  if (!entryReady()) return; const c = entryContext(); const id = operationID || entryFlow?.operationID || entryOperation?.intent.operation_id || entrySnapshot.pending_operation_id;
  try { const r = entryEnvelope(await call(id ? 'EntryOperation' : 'EntryPending', c.viewID, ...(id ? [id] : [])), 'operation', !id); if (!entryCurrent(c)) return; if (!r.ok) { entryRefusal(r.error); return; } const o = r.value.operation; requireValue(!o || !id || o.intent.operation_id === id); requireValue(id || o === null || o.state === 'prepared'); const priorOperation = entryOperation, priorFlow = entryFlow; entryOperation = o; entryUnknown = false; if (entryFlow) entryFlow.auto = false;
    if (!o) { entryFlow = null; entryMessage = 'No pending choice was found. This does not prove an earlier dispatch never happened. Review status and history before a new explicit save.'; }
    else if (o.state === 'prepared') { entryFlow = { operationID: o.intent.operation_id, request: own(o.intent.request), auto: false, context: c }; entryMessage = o.intent.view_id === c.viewID && o.intent.generation === current.view.generation ? 'A prepared choice is retained. Review it, then explicitly apply or abandon it.' : 'This prepared choice belongs to an earlier sign-in. You can inspect or abandon it, but cannot apply it.'; }
    else { entryFlow = null; entryMessage = o.state === 'committed' ? `Saved outcome confirmed at entry revision ${o.receipt.revision}. This query did not change the entry.` : 'The prepared choice was abandoned. Any prior saved entry is unchanged.'; }
    if (!entryCapacity()) { entryOperation = priorOperation; entryFlow = priorFlow; entryUnknown = true; entryMessage = entryError('entry_capacity'); return; } if (entryDate && !tasks.length) await entryReadStatus();
  } catch { if (entryCurrent(c)) { entryUnknown = true; entryMessage = entryError('outcome_unknown'); } } render();
}
async function fetchEntrySnapshot() {
  if (!ready() || entryUnavailable) return; const c = entryContext();
  try { const r = entryEnvelope(await call('EntrySnapshot'), 'snapshot'); if (!entryCurrent(c)) return; if (!r.ok) { entryRefusal(r.error); renderEntry(); return; } entrySnapshot = r.value; if (!entrySnapshot.view) { entryUnavailable = true; renderEntry(); return; }
    if (entryWatch) await entryFetchJob(entryWatch.id);
    else if (entrySnapshot.pending_operation_id && entryOperation?.intent.operation_id !== entrySnapshot.pending_operation_id && !tasks.length) await entryQuery(entrySnapshot.pending_operation_id);
    if (entryCurrent(c) && !entryWatch && !tasks.length && entrySnapshot.state !== 'busy') await entryReadStatus(!entryDate);
    render();
  } catch { if (entryCurrent(c)) { entryUnavailable = true; entryMessage = entryError('entry_unavailable'); renderEntry(); } }
}
async function entryBegin(kind, q = null, op = null, auto = false) {
  if (!entryReady() || entryWatch) return; const c = entryContext(); const selected = kind === 'apply' ? op.intent.request : q; const target = selected || { event_id: entryStatus?.event.event_id, saturday: entryDate };
  if (kind === 'apply') requireValue(op.state === 'prepared' && op.intent.view_id === c.viewID && op.intent.generation === current.view.generation);
  const prior = entryTerminal; entryTerminal = null; const reservation = { id: '', kind, eventID: target.event_id, saturday: target.saturday, operationID: op?.intent.operation_id ?? '', request: selected ? own(selected) : null, context: c }; entryWatch = reservation;
  if (!entryCapacity()) { entryWatch = null; entryTerminal = prior; entryMessage = entryError('entry_capacity'); return; }
  const method = { prepare: 'BeginEntryPrepare', apply: 'BeginEntryApply', check: 'BeginEntryCheck' }[kind]; const arg = kind === 'prepare' ? q : kind === 'apply' ? op.intent.operation_id : target.saturday;
  if (kind === 'prepare') entryFlow = { operationID: '', request: own(q), auto, context: c };
  try { const r = entryEnvelope(await call(method, c.viewID, arg), 'job'); if (!entryCurrent(c)) return;
    if (!r.ok) { entryWatch = null; entryTerminal = prior; if (['prepare', 'apply'].includes(kind) && r.error === 'outcome_unknown') { entryUnknown = true; if (entryFlow) entryFlow.auto = false; await entryQuery(reservation.operationID); } else { if (kind === 'prepare') entryFlow = null; entryRefusal(r.error); } return; }
    const j = r.value; requireValue(j.state === 'queued' && j.kind === kind && j.event_id === target.event_id && j.saturday === target.saturday && (kind === 'apply' ? j.operation_id === op.intent.operation_id : j.operation_id === '')); entryWatch = { ...reservation, id: j.job_id }; entryMessage = kind === 'prepare' ? 'Preparing this exact entry choice…' : kind === 'apply' ? 'Checking and saving this entry…' : 'Checking the saved selection…'; pollWanted = true;
  } catch { if (entryCurrent(c)) { entryWatch = null; entryTerminal = prior; if (['prepare', 'apply'].includes(kind)) { entryUnknown = true; if (entryFlow) entryFlow.auto = false; entryMessage = entryError('outcome_unknown'); await entryQuery(reservation.operationID); } else entryMessage = entryError('entry_unavailable'); } }
  render();
}
async function entryFetchJob(id) {
  if (!entryReady() || !entryWatch || entryWatch.id !== id) return; const w = entryWatch, c = entryContext();
  try { const r = entryEnvelope(await call('EntryJob', c.viewID, id), 'job'); if (!entryCurrent(c) || entryWatch !== w) return; if (!r.ok) { if (r.error === 'busy') return; entryWatch = null; entryRefusal(r.error); if (w.kind !== 'check') { entryUnknown = true; await entryQuery(w.operationID); } return; }
    const j = r.value; requireValue(j.job_id === id && j.kind === w.kind && j.event_id === w.eventID && j.saturday === w.saturday && (w.kind !== 'apply' || j.operation_id === w.operationID));
    if (['queued', 'running'].includes(j.state)) return;
    entryWatch = null;
    if (j.state !== 'complete') { entryMessage = entryError(j.error || 'entry_cancelled') + careerAffected(j); if (j.kind !== 'check') { if (entryFlow) entryFlow.auto = false; if (j.error === 'outcome_unknown') { entryUnknown = true; await entryQuery(w.operationID); } else await entryQuery(w.operationID); } return; }
    if (j.kind === 'prepare') {
      requireValue(same(j.intent.request, w.request)); entryOperation = { version: 1, intent: own(j.intent), state: 'prepared', receipt: null, abandoned_at_ms: 0 };
      const automatic = entryFlow?.auto && entryCurrent(w.context, true) && same(entryFlow.request, j.intent.request) && entryDate === j.saturday && !tasks.length;
      entryFlow = { operationID: j.operation_id, request: own(j.intent.request), auto: false, context: w.context }; entryTerminal = j; requireValue(entryCapacity());
      if (automatic) await entryBegin('apply', null, entryOperation); else entryMessage = 'The exact choice was prepared. Review it and apply explicitly, or abandon it. Nothing has been entered yet.';
    } else if (j.kind === 'apply') {
      const receipt = j.mutation.receipt; requireValue(receipt.previous_revision === w.request.expected_revision && receipt.roster_revision === w.request.expected_roster_revision && receipt.action === w.request.action && same(receipt.members.map((m) => m.assignment.prospect_id), w.request.prospect_ids)); entryTerminal = j; entryOperation = entryFlow = null; entryUnknown = false; entryMessage = receipt.action === 'withdraw' ? `Saved entry withdrawn at revision ${receipt.revision}.` : `Entry saved at revision ${receipt.revision}. No event lock or bout has been created.`; requireValue(entryCapacity()); if (entryDate === j.saturday) await entryReadStatus();
    } else { entryTerminal = j; requireValue(entryCapacity()); if (entryDate === j.saturday && entryCurrent(w.context, true)) entryAdoptStatus(j.check.status); entryMessage = 'Saved eligibility checked at the displayed authority time. Saved history was not changed.'; }
  } catch { if (entryCurrent(c)) { entryWatch = null; if (w.kind !== 'check') { entryUnknown = true; if (entryFlow) entryFlow.auto = false; entryMessage = entryError('outcome_unknown'); } else entryMessage = entryError('entry_unavailable'); } } render();
}
async function entrySave(action) {
  if (!entryReady() || !entryStatus || entryStatus.event.saturday !== entryDate || entryStatus.phase !== 'accepting' || entryWatch || entryUnknown || entryOperation?.state === 'prepared' || entrySnapshot.pending_operation_id) return;
  if (action === 'replace' && (!entryDraft.length || !stable.status.caught_up || entryDraft.some((x) => !stable.status.slots.includes(x.id)))) return;
  const q = { version: 1, event_id: entryStatus.event.event_id, saturday: entryDate, action, expected_revision: entryStatus.revision, expected_roster_revision: action === 'replace' ? stable.status.roster_revision : 0, prospect_ids: action === 'replace' ? entryDraft.map((x) => x.id).sort() : [] }; entryRequestValue(q); await entryBegin('prepare', q, null, true);
}
async function entryCancel() {
  if (!entryReady() || !entryWatch?.id) return; const w = entryWatch, c = entryContext(); if (entryFlow) entryFlow.auto = false; entryChanged();
  try { const r = entryEnvelope(await call('CancelEntryJob', c.viewID, w.id), 'job'); if (!entryCurrent(c)) return; if (!r.ok) { entryRefusal(r.error); return; } requireValue(r.value.job_id === w.id && r.value.kind === w.kind && r.value.event_id === w.eventID && r.value.saturday === w.saturday && (w.kind !== 'apply' || r.value.operation_id === w.operationID) && (r.value.kind !== 'prepare' || r.value.state !== 'complete' || same(r.value.intent.request, w.request))); entryMessage = 'Cancellation requested. A durable prepared or saved choice remains until its recorded outcome is checked.'; if (!['queued', 'running'].includes(r.value.state)) { entryWatch = null; if (w.kind !== 'check') await entryQuery(w.operationID); } }
  catch { if (entryCurrent(c)) { entryUnknown = w.kind !== 'check'; entryMessage = entryError('outcome_unknown'); } } render();
}
async function entryAbandon() {
  if (!entryReady() || entryOperation?.state !== 'prepared') return; const c = entryContext(), id = entryOperation.intent.operation_id; if (entryFlow) entryFlow.auto = false;
  try { const r = entryEnvelope(await call('AbandonEntryOperation', c.viewID, id), 'operation'); if (!entryCurrent(c)) return; if (!r.ok) { entryRefusal(r.error); if (r.error === 'outcome_unknown') { entryUnknown = true; await entryQuery(id); } return; } requireValue(r.value.operation.intent.operation_id === id && r.value.operation.state !== 'prepared'); entryOperation = r.value.operation; entryFlow = null; entryUnknown = false; entryMessage = entryOperation.state === 'abandoned' ? 'Prepared choice abandoned. Any prior saved entry is unchanged.' : 'This choice had already committed; abandoning does not withdraw it.'; await entryReadStatus(); }
  catch { if (entryCurrent(c)) { entryUnknown = true; entryMessage = entryError('outcome_unknown'); await entryQuery(id); } } render();
}
async function entryReadHistory(after = '') {
  if (!entryReady()) return; const c = entryContext(); try { const r = entryEnvelope(await call('EntryHistory', c.viewID, after, 8), 'history'); if (!entryCurrent(c)) return; if (!r.ok) { entryRefusal(r.error); return; } requireValue(r.value.after_event_id === after); const old = entryHistory; entryHistory = r.value; if (!entryCapacity()) { entryHistory = old; entryMessage = entryError('entry_capacity'); } } catch { if (entryCurrent(c)) entryMessage = entryError('entry_unavailable'); } render();
}
function entryNavigate(dateValue) {
  try { entryDateValue(dateValue); } catch { entryMessage = entryError('entry_calendar'); renderEntry(); return; }
  entryChanged(); entryDate = dateValue; eventSelectDate(entryDate); entryStatus = null; entryMessage = 'Loading this Saturday. Draft and pending choices have not been entered or retargeted.';
  queue(() => entryReadStatus(), 'entry-date'); render();
}
function renderEntry() {
  const active = entryReady(), s = entryStatus, pending = entryOperation?.state === 'prepared' ? entryOperation : null, busy = !!command || !!entryWatch, accepting = active && s?.event.saturday === entryDate && s.phase === 'accepting';
  content(ui['entry-state'], entryUnavailable ? 'Unavailable' : !active ? 'Sign in to view entries' : entryWatch ? 'Entry request in progress' : s?.phase === 'closed_pending_lock' ? 'Intake closed' : 'Candidate intake');
  if (ui['entry-date'].value !== entryDate) ui['entry-date'].value = entryDate; ui['entry-date'].disabled = !active; ui['entry-upcoming'].disabled = !active || !!command; ui['entry-status'].disabled = !active || !entryDate || !!command;
  content(ui['entry-cutoff'], s ? `Saturday ${s.event.saturday} · Noon Pacific cutoff · ${new Date(s.event.cutoff_ms).toISOString()} (authority UTC)` : 'Choose an authority-provided Upcoming Saturday or a supported Saturday date.');
  content(ui['entry-saved'], s ? s.revision ? `Saved entry revision ${s.revision} · ${s.saved_ids.length} fighter${s.saved_ids.length === 1 ? '' : 's'} · Saved ${date.format(new Date(s.saved_at_ms))}.` : s.exists ? 'A choice is prepared; no entry has been saved yet.' : 'No saved entry for this Saturday.' : '');
  content(ui['entry-warning'], s?.phase === 'closed_pending_lock' ? entryError('entry_closed') : s?.roster_changed || s?.saved_ids.length && s.saved_roster_revision !== stable?.status.roster_revision ? 'Your roster changed. Check this saved entry again. The entry has not been automatically withdrawn.' : s?.saved_ids.length ? 'Current eligibility has not been checked by this status read.' : '');
  content(ui['entry-count'], `${entryDraft.length} / 5`); const draftKey = JSON.stringify(entryDraft);
  if (ui['entry-draft']._key !== draftKey) { ui['entry-draft']._key = draftKey; ui['entry-draft'].textContent = ''; for (const d of entryDraft) { const li = element('li', '', `${d.name} · ${label(d.weightClass)}`), remove = element('button', '', 'Remove from draft'); remove.type = 'button'; remove.setAttribute('aria-label', `Remove ${d.name} from Saturday draft`); remove.addEventListener('click', () => { entryChanged(); entryDraft = entryDraft.filter((x) => x.id !== d.id); render(); }); li.append(remove, element('small', '', d.id)); ui['entry-draft'].append(li); } }
  const blocked = busy || entryUnknown || !!pending || !!entrySnapshot?.pending_operation_id;
  ui['entry-save'].disabled = !accepting || blocked || !entryDraft.length || !stable?.status.caught_up || entryDraft.some((x) => !stable.status.slots.includes(x.id)); content(ui['entry-save'], s?.revision ? 'Replace saved entry' : 'Save entry');
  ui['entry-withdraw'].disabled = !accepting || blocked || !s?.revision || !s.saved_ids.length; ui['entry-check'].disabled = !active || busy || !s; ui['entry-clear'].disabled = !entryDraft.length; ui['entry-cancel'].disabled = !active || !!command || !entryWatch?.id;
  content(ui['entry-message'], entryMessage); content(ui['entry-pending'], pending ? `Prepared ${pending.intent.request.action === 'withdraw' ? 'withdrawal' : `${pending.intent.request.prospect_ids.length}-fighter choice`} for ${pending.intent.request.saturday}. ${pending.intent.view_id === current?.view?.view_id && pending.intent.generation === current?.view?.generation ? 'It is not saved until Apply succeeds.' : 'It belongs to an earlier sign-in and cannot execute now.'}${pending.intent.request.prospect_ids.length ? ` Fighter IDs: ${pending.intent.request.prospect_ids.join(', ')}.` : ''}` : entryUnknown ? 'The outcome remains unresolved. Do not start a replacement operation.' : '');
  ui['entry-reconcile'].disabled = !active || !!command || !!entryWatch || !(entryUnknown || entryFlow || entryOperation || entrySnapshot.pending_operation_id);
  ui['entry-apply'].disabled = !active || busy || entryUnknown || !pending || pending.intent.view_id !== current?.view?.view_id || pending.intent.generation !== current?.view?.generation || pending.intent.request.saturday !== entryDate || !accepting;
  ui['entry-abandon'].disabled = !active || busy || !pending; ui['entry-history'].disabled = !active || !!command; ui['entry-history-next'].disabled = !active || !!command || !entryHistory?.has_more;
  const check = entryTerminal?.kind === 'check' && entryTerminal.saturday === entryDate ? entryTerminal.check : null, checkKey = JSON.stringify(check);
  if (ui['entry-eligibility']._key !== checkKey) { ui['entry-eligibility']._key = checkKey; ui['entry-eligibility'].textContent = ''; if (check) { ui['entry-eligibility'].append(element('li', '', `Checked ${date.format(new Date(check.as_of_ms))}; this observation stays fixed until another Check.`)); for (const m of check.members) ui['entry-eligibility'].append(element('li', '', `${m.item.manifest.name.display} · ${label(m.item.manifest.physical.weight_class)} · ${label(m.eligibility)}${m.slot ? ` · Slot ${m.slot}` : ''}`)); } }
  const historyKey = JSON.stringify(entryHistory);
  if (ui['entry-history-list']._key !== historyKey) { ui['entry-history-list']._key = historyKey; ui['entry-history-list'].textContent = ''; if (entryHistory) for (const e of entryHistory.events) { const li = element('li'), b = element('button', '', e.event.saturday); b.type = 'button'; b.addEventListener('click', () => entryNavigate(e.event.saturday)); li.append(b, element('span', '', `Revision ${e.revision} · ${e.saved_ids.length} saved · ${e.phase === 'closed_pending_lock' ? 'Intake closed; awaiting event lock' : 'Intake accepting'}`)); ui['entry-history-list'].append(li); } }
}
// Immutable candidate noon snapshots. Full source/projection and canonical
// digest validation belong to the native boundary; the browser accepts only
// its bounded, current-view, explicitly pinned presentation.
const EVENT_MAX = 131072;
const EVENT_RENDER_KEY_BYTES = 4096; // Fixed charge for the two bounded DOM identity strings.
const EVENT_ERRORS = new Set([...CAREER_ACTION_ERRORS, 'invalid_request', 'unauthorized', 'forbidden', 'stale_view', 'busy', 'event_unavailable', 'event_setup_incomplete', 'event_corrupt', 'event_capacity', 'event_clock', 'event_calendar', 'event_not_due', 'event_created_too_late', 'event_incomplete', 'event_conflict', 'event_not_found', 'event_cancelled', 'outcome_unknown']);
let eventSnapshot = null, eventAuthority = null, eventStatus = null, eventHistory = null, eventDetail = null;
let eventWatch = null, eventLastJob = '', eventDate = '', eventSerial = 0, eventMessage = '', eventUnavailable = false, eventUnknown = false;
let eventAutoAttempted = new Set();
function eventRetainedBytes() { return entryBytes([eventSnapshot, eventAuthority, eventStatus, eventHistory, eventDetail, eventWatch, eventLastJob, eventDate, eventMessage, [...eventAutoAttempted]]) + EVENT_RENDER_KEY_BYTES + (eventWatch ? 65536 : 0); }
function eventCapacity() { return eventRetainedBytes() <= EVENT_MAX && previewCapacity(); }
function eventReady() { return ready() && !eventUnavailable && !!eventSnapshot?.pin && !!eventAuthority && viewKey(eventSnapshot.view) === currentKey(); }
function clearEvent() {
  clearSeed();
  eventSerial++; eventSnapshot = eventStatus = eventHistory = eventDetail = eventWatch = null; eventLastJob = eventDate = eventMessage = ''; eventUnavailable = eventUnknown = false; eventAutoAttempted = new Set();
  // Retain the helper-lifetime public authority, never its old account view.
  for (const id of ['event-members', 'event-history-list']) { ui[id].textContent = ''; ui[id]._key = ''; }
}
function eventSelectDate(value) {
  cardSelectDate();
  seedStatus = seedDetail = null; seedMessage = ''; seedSerial++;
  eventSerial++; eventDate = value; eventStatus = eventDetail = null; eventMessage = '';
  // An issued job is still tracked, but cannot repopulate another Saturday.
}
function eventContext() { return { epoch, view: currentKey(), viewID: current?.view?.view_id, serial: eventSerial, date: entryDate }; }
function queueEventSelection(fn, name) { const c = eventContext(); queue(() => eventCurrent(c, true) ? fn() : undefined, name); }
function eventCurrent(c, selection = false) { return !disposed && ready() && c.epoch === epoch && c.view === currentKey() && (!selection || c.serial === eventSerial && c.date === entryDate); }
function eventPinValue(p) {
  exact(p, ['version', 'calendar_id', 'rules_id', 'participants']); requireValue(p.version === 1 && p.calendar_id === ENTRY_CALENDAR && hex(p.rules_id)); pairArray(p.participants, (e) => {
    exact(e, ['version', 'namespace_id', 'calendar_id', 'rules_id', 'stable_store_id', 'stable_pin']); requireValue(e.version === 1 && hex(e.namespace_id) && e.calendar_id === ENTRY_CALENDAR && e.rules_id === ENTRY_RULES && hex(e.stable_store_id));
    const s = e.stable_pin; exact(s, ['version', 'account_authority_id', 'account_id', 'identity', 'source', 'condition_version', 'projector_id']); requireValue(s.version === 1 && s.account_authority_id === current.view.account_authority_id && hex(s.account_id) && s.condition_version === 1 && s.projector_id === 'slug-fighter-condition-candidate-v1');
    const i = s.identity, a = s.source; exact(i, ['network', 'genesis', 'profile', 'root_public_key', 'gym_public_key', 'gym_id']); const f = Object.keys(fixtures).find((name) => fixtures[name][0] === i.gym_id); requireValue(!!f && i.network === 0x534c5547 && i.genesis === GENESIS && i.profile === 'slug-phase-a-fixture-bip39-v1' && i.root_public_key === ROOTS[f] && i.gym_public_key === fixtures[f][1]);
    exact(a, ['version', 'network', 'genesis', 'authority_id', 'descriptor_id']); requireValue(a.version === 5 && a.network === i.network && a.genesis === i.genesis && a.descriptor_id === DESCRIPTOR && hex(a.authority_id));
  });
  const [a, b] = p.participants; requireValue(a.stable_pin.identity.gym_id < b.stable_pin.identity.gym_id && a.stable_pin.account_id !== b.stable_pin.account_id && a.stable_store_id !== b.stable_store_id && a.namespace_id === b.namespace_id);
  const ownPin = p.participants.find((x) => x.stable_pin.identity.gym_id === current.gym_id); requireValue(!!ownPin && same(ownPin.stable_pin, stable.status.pin) && ownPin.stable_store_id === stable.status.store_id && (!entryNamespace || ownPin.namespace_id === entryNamespace)); return p;
}
function eventValue(e) {
  exact(e, ['version', 'event_id', 'source_event_id', 'store_id', 'namespace_id', 'calendar_id', 'rules_id', 'saturday', 'cutoff_ms', 'incident_deadline_ms']);
  requireValue(!!eventAuthority && e.version === 1 && ['event_id', 'source_event_id', 'store_id', 'namespace_id'].every((k) => hex(e[k])) && e.store_id === eventAuthority.storeID && e.namespace_id === eventAuthority.pin.participants[0].namespace_id && e.calendar_id === ENTRY_CALENDAR && e.rules_id === eventAuthority.pin.rules_id && int(e.cutoff_ms, 1, DATE_MAX) && int(e.incident_deadline_ms, e.cutoff_ms + 167 * 3600000, e.cutoff_ms + 169 * 3600000)); entryDateValue(e.saturday); return e;
}
function eventStatusValue(s) {
  exact(s, ['version', 'store_id', 'event', 'exists', 'phase', 'captured_participants', 'snapshot_digest', 'coordinator_created_at_ms', 'created_at_ms', 'completed_at_ms', 'observed_at_ms', 'seed_authority']); eventValue(s.event);
  requireValue(s.version === 1 && s.store_id === eventAuthority.storeID && typeof s.exists === 'boolean' && ['before_cutoff', 'awaiting_capture', 'capturing', 'snapshotted'].includes(s.phase) && s.coordinator_created_at_ms === eventAuthority.createdMS && int(s.observed_at_ms, s.coordinator_created_at_ms, DATE_MAX) && int(s.created_at_ms, 0, s.observed_at_ms) && int(s.completed_at_ms, 0, s.observed_at_ms) && optionalID(s.snapshot_digest) && s.seed_authority === 'unavailable');
  const ids = eventAuthority.pin.participants.map((p) => p.stable_pin.identity.gym_id); requireValue(Array.isArray(s.captured_participants) && s.captured_participants.length <= 2 && s.captured_participants.every((x, n) => ids.includes(x) && (!n || s.captured_participants[n - 1] < x)));
  if (!s.exists) requireValue(s.created_at_ms === 0 && s.completed_at_ms === 0 && s.snapshot_digest === '' && s.captured_participants.length === 0 && s.phase === (s.observed_at_ms < s.event.cutoff_ms ? 'before_cutoff' : 'awaiting_capture'));
  else requireValue(s.created_at_ms >= s.event.cutoff_ms && s.coordinator_created_at_ms < s.event.cutoff_ms && s.phase === (s.captured_participants.length === 2 ? 'snapshotted' : s.captured_participants.length === 1 ? 'capturing' : 'awaiting_capture'));
  requireValue(s.phase === 'snapshotted' ? hex(s.snapshot_digest) && s.completed_at_ms >= s.created_at_ms : s.snapshot_digest === '' && s.completed_at_ms === 0); return s;
}
function eventFence(f, store, cutoff) { exact(f, ['version', 'store_id', 'cutoff_ms', 'observed_at_ms', 'operation_count']); requireValue(f.version === 1 && f.store_id === store && f.cutoff_ms === cutoff && int(f.observed_at_ms, cutoff, DATE_MAX) && int(f.operation_count, 0, 4096)); }
function eventCondition(c, a, cutoff) {
  exact(c, ['version', 'projector_id', 'dna_version', 'seed', 'born_at_ms', 'as_of_ms', 'age_weeks', 'phase', 'elapsed_decline_weeks', 'prime_remaining_weeks', 'prime_remaining_ms', 'career_remaining_ms', 'pro_xp', 'aged_attributes_tenths', 'league_attributes_tenths', 'rating_hundredths']);
  requireValue(c.version === 1 && c.projector_id === 'slug-fighter-condition-candidate-v1' && c.dna_version === 1 && c.seed === a.work_hash && c.born_at_ms === a.born_at_ms && c.as_of_ms === cutoff && a.accepted_at_ms < cutoff && c.age_weeks === Math.floor((cutoff - a.born_at_ms) / WEEK) && ['prime', 'decline', 'retired'].includes(c.phase) && c.pro_xp === 0 && int(c.elapsed_decline_weeks, 0, 39) && int(c.prime_remaining_weeks, 0, 52) && int(c.prime_remaining_ms, 0, 52 * WEEK) && int(c.career_remaining_ms, 0, 91 * WEEK));
  requireValue((c.phase === 'retired') === (c.career_remaining_ms === 0)); exact(c.aged_attributes_tenths, attrs); exact(c.league_attributes_tenths, attrs); requireValue(attrs.every((k) => int(c.aged_attributes_tenths[k], 0, 1000) && c.league_attributes_tenths[k] === c.aged_attributes_tenths[k]) && c.rating_hundredths === attrs.reduce((n, k, i) => n + c.league_attributes_tenths[k] * [10, 25, 2, 4, 8][i], 0));
}
function eventSavedReceipt(r, p, e, store) {
  exact(r, ['version', 'store_id', 'entry_id', 'event', 'operation_id', 'operation_sequence', 'generation', 'view_id', 'action', 'previous_revision', 'revision', 'roster_revision', 'stable_checkpoint', 'observed_source_count', 'committed_at_ms', 'members']); entryEvent(r.event);
  requireValue(r.version === 1 && r.store_id === store && r.event.event_id === e.source_event_id && r.event.saturday === e.saturday && r.event.cutoff_ms === e.cutoff_ms && ['entry_id', 'operation_id', 'view_id'].every((k) => hex(r[k])) && int(r.operation_sequence, 1, 4096) && int(r.generation, 1) && ['replace', 'withdraw'].includes(r.action) && int(r.previous_revision, 0, 4095) && r.revision === r.previous_revision + 1 && int(r.roster_revision, 0, 4096) && int(r.stable_checkpoint, 0, 1000) && int(r.observed_source_count, r.stable_checkpoint, 1000) && int(r.committed_at_ms, 1, e.cutoff_ms - 1));
  requireValue(Array.isArray(r.members) && r.members.length <= 5 && (r.action === 'replace' ? r.members.length > 0 : r.members.length === 0 && r.roster_revision === 0)); const slotsSeen = new Set(), classSeen = new Set(); let previous = '';
  for (const m of r.members) { exact(m, ['slot', 'assignment', 'class', 'condition_at_save']); assignment(m.assignment, p.stable_pin.identity); requireValue(int(m.slot, 1, 10) && classes.includes(m.class) && !slotsSeen.has(m.slot) && !classSeen.has(m.class) && m.assignment.prospect_id > previous && m.assignment.sequence <= r.stable_checkpoint); slotsSeen.add(m.slot); classSeen.add(m.class); previous = m.assignment.prospect_id; entrySavedCondition(m.condition_at_save, m.assignment, r.committed_at_ms); }
}
function eventParticipant(s, e) {
  exact(s, ['version', 'participant_id', 'entry_store_id', 'owner', 'roster', 'entry', 'members']); const p = eventAuthority.pin.participants.find((x) => x.stable_pin.identity.gym_id === s.participant_id); requireValue(s.version === 1 && !!p && hex(s.entry_store_id));
  const o = s.owner; exact(o, ['ownership_basis', 'account_authority_id', 'account_id', 'gym_id']); requireValue(o.ownership_basis === 'candidate_assignment' && o.account_authority_id === p.stable_pin.account_authority_id && o.account_id === p.stable_pin.account_id && o.gym_id === s.participant_id);
  const r = s.roster; exact(r, ['version', 'store_id', 'cutoff_ms', 'committed_count', 'last_sequence', 'history_digest', 'revision', 'slots', 'assigned_at_ms']); slots(r.slots); requireValue(r.version === 1 && r.store_id === p.stable_store_id && r.cutoff_ms === e.cutoff_ms && int(r.committed_count, 0, 4096) && int(r.last_sequence, r.committed_count, 4096) && int(r.revision, 0, r.committed_count) && hex(r.history_digest) && Array.isArray(r.assigned_at_ms) && r.assigned_at_ms.length === 10); r.slots.forEach((x, n) => requireValue(x ? int(r.assigned_at_ms[n], 1, e.cutoff_ms - 1) : r.assigned_at_ms[n] === 0));
  const t = s.entry; exact(t, ['version', 'store_id', 'event', 'entry_id', 'committed_count', 'last_sequence', 'history_digest', 'latest_receipt']); entryEvent(t.event); requireValue(t.version === 1 && t.store_id === s.entry_store_id && t.event.event_id === e.source_event_id && t.event.cutoff_ms === e.cutoff_ms && t.event.saturday === e.saturday && hex(t.entry_id) && hex(t.history_digest) && int(t.committed_count, 0, 4096) && int(t.last_sequence, t.committed_count, 4096));
  if (t.latest_receipt !== null) { eventSavedReceipt(t.latest_receipt, p, e, s.entry_store_id); requireValue(t.committed_count > 0 && t.latest_receipt.entry_id === t.entry_id && t.latest_receipt.operation_sequence === t.last_sequence); } else requireValue(t.committed_count === 0 && t.last_sequence === 0);
  const saved = t.latest_receipt?.members ?? []; requireValue(Array.isArray(s.members) && s.members.length === saved.length && s.members.length <= 5);
  for (const [n, m] of s.members.entries()) { exact(m, ['slot', 'slot_assigned_at_ms', 'assignment', 'class', 'condition_at_cutoff', 'eligibility']); assignment(m.assignment, p.stable_pin.identity); requireValue(same(m.assignment, saved[n].assignment) && m.class === saved[n].class && int(m.slot, 0, 10) && m.slot === r.slots.indexOf(m.assignment.prospect_id) + 1 && (m.slot ? m.slot_assigned_at_ms === r.assigned_at_ms[m.slot - 1] && m.slot_assigned_at_ms >= m.assignment.accepted_at_ms : m.slot_assigned_at_ms === 0)); eventCondition(m.condition_at_cutoff, m.assignment, e.cutoff_ms); requireValue(m.eligibility === (m.condition_at_cutoff.phase === 'retired' ? 'retired' : m.slot ? 'eligible' : 'not_rostered')); } return s;
}
function eventEvidence(v, s, e) { exact(v, ['version', 'participant_id', 'stable_fence', 'entry_fence', 'captured_at_ms']); requireValue(v.version === 1 && v.participant_id === s.participant_id); eventFence(v.stable_fence, s.roster.store_id, e.cutoff_ms); eventFence(v.entry_fence, s.entry_store_id, e.cutoff_ms); requireValue(v.entry_fence.observed_at_ms >= v.stable_fence.observed_at_ms && int(v.captured_at_ms, v.entry_fence.observed_at_ms, DATE_MAX) && v.stable_fence.operation_count >= s.roster.last_sequence && v.entry_fence.operation_count >= s.entry.last_sequence); }
function eventCapture(v, e) { exact(v, ['version', 'snapshot', 'evidence']); requireValue(v.version === 1); eventParticipant(v.snapshot, e); eventEvidence(v.evidence, v.snapshot, e); return v; }
function eventRecordValue(v) {
  exact(v, ['version', 'input', 'snapshot_digest', 'captures', 'completed_at_ms']); const i = v.input; exact(i, ['version', 'event', 'participants']); eventValue(i.event); requireValue(v.version === 1 && i.version === 1 && hex(v.snapshot_digest)); pairArray(i.participants, (p) => eventParticipant(p, i.event)); pairArray(v.captures, () => {});
  i.participants.forEach((p, n) => { requireValue(p.participant_id === eventAuthority.pin.participants[n].stable_pin.identity.gym_id); eventEvidence(v.captures[n], p, i.event); }); requireValue(int(v.completed_at_ms, Math.max(...v.captures.map((c) => c.captured_at_ms)), DATE_MAX)); return v;
}
function eventJobValue(j) {
  careerJobFields(j, ['version', 'job_id', 'kind', 'state', 'view', 'store_id', 'event_id', 'saturday', 'participant_id', 'error', 'result']); requireValue(j.version === 1 && hex(j.job_id) && j.kind === 'capture' && ['queued', 'running', 'completed', 'failed', 'cancelled'].includes(j.state) && viewKey(checkedView(j.view)) === currentKey() && j.store_id === eventAuthority.storeID && hex(j.event_id) && j.participant_id === current.gym_id); entryDateValue(j.saturday);
  if (j.state === 'completed') { requireValue(j.error === '' && j.result !== null); const r = j.result; exact(r, ['version', 'status', 'capture', 'replayed']); requireValue(r.version === 1 && typeof r.replayed === 'boolean'); eventStatusValue(r.status); eventCapture(r.capture, r.status.event); requireValue(r.status.event.event_id === j.event_id && r.status.event.saturday === j.saturday && r.capture.snapshot.participant_id === j.participant_id && r.status.captured_participants.includes(j.participant_id) && r.capture.evidence.captured_at_ms <= r.status.observed_at_ms); }
  else requireValue(j.result === null && (['queued', 'running'].includes(j.state) ? j.error === '' : j.state === 'cancelled' ? j.error === 'event_cancelled' : EVENT_ERRORS.has(j.error))); return j;
}
function eventEnvelope(r, field) {
  requireValue(entryBytes(r) <= EVENT_MAX); exact(r, ['id', 'ok', 'view', 'store_id', 'snapshot', 'status', 'record', 'history', 'job', 'error']); requireValue(int(r.id, 1) && typeof r.ok === 'boolean'); const fields = ['snapshot', 'status', 'record', 'history', 'job'];
  if (!r.ok) { requireValue(r.view === null && r.store_id === '' && fields.every((k) => r[k] === null) && EVENT_ERRORS.has(r.error)); return { ok: false, error: r.error }; }
  requireValue(r.error === '' && fields.every((k) => (r[k] !== null) === (k === field))); const v = r[field];
  if (field === 'snapshot') {
    exact(v, ['version', 'state', 'error', 'view', 'store_id', 'pin', 'coordinator_created_at_ms', 'participant_id', 'active_job_id', 'queued_job_id', 'last_job_id']); requireValue(v.version === 1 && ['unavailable', 'setup_incomplete', 'ready', 'busy', 'failed'].includes(v.state) && ['store_id', 'participant_id', 'active_job_id', 'queued_job_id', 'last_job_id'].every((k) => optionalID(v[k])));
    if (v.state === 'unavailable') { requireValue(r.view === null && v.view === null && r.store_id === '' && v.pin === null && v.error === 'event_unavailable' && v.coordinator_created_at_ms === 0 && ['store_id', 'participant_id', 'active_job_id', 'queued_job_id', 'last_job_id'].every((k) => v[k] === '')); return { ok: true, value: own(v) }; }
    requireValue(ready() && viewKey(checkedView(r.view)) === currentKey() && viewKey(checkedView(v.view)) === currentKey() && r.store_id === v.store_id && v.participant_id === current.gym_id);
    if (v.state === 'setup_incomplete') { requireValue(!eventAuthority && v.pin === null && v.store_id === '' && v.coordinator_created_at_ms === 0 && v.error === 'event_setup_incomplete' && ['active_job_id', 'queued_job_id', 'last_job_id'].every((k) => v[k] === '')); return { ok: true, value: own(v) }; }
    eventPinValue(v.pin); requireValue(hex(v.store_id) && int(v.coordinator_created_at_ms, 1, DATE_MAX) && (v.state === 'failed' ? EVENT_ERRORS.has(v.error) && v.error !== 'outcome_unknown' : v.error === '') && (!v.active_job_id || v.active_job_id !== v.queued_job_id));
    const a = { pin: own(v.pin), storeID: v.store_id, createdMS: v.coordinator_created_at_ms }; requireValue(!eventAuthority || same(eventAuthority, a)); if (!eventAuthority) eventAuthority = a; return { ok: true, value: own(v) };
  }
  requireValue(eventReady() && viewKey(checkedView(r.view)) === currentKey() && r.store_id === eventAuthority.storeID);
  if (field === 'status') eventStatusValue(v);
  if (field === 'record') eventRecordValue(v);
  if (field === 'job') eventJobValue(v);
  if (field === 'history') { exact(v, ['version', 'store_id', 'after_event_id', 'next_event_id', 'has_more', 'observed_at_ms', 'events']); requireValue(v.version === 1 && v.store_id === eventAuthority.storeID && optionalID(v.after_event_id) && optionalID(v.next_event_id) && typeof v.has_more === 'boolean' && int(v.observed_at_ms, eventAuthority.createdMS, DATE_MAX) && Array.isArray(v.events) && v.events.length <= 8); v.events.forEach((s, n) => { eventStatusValue(s); requireValue(s.exists && s.observed_at_ms === v.observed_at_ms && (!n || v.events[n - 1].event.cutoff_ms < s.event.cutoff_ms)); }); requireValue(v.next_event_id === (v.has_more && v.events.length ? v.events.at(-1).event.event_id : '')); }
  return { ok: true, value: own(v) };
}
function eventError(code) { return ({ event_unavailable: 'Saturday snapshots are unavailable in this app route.', event_setup_incomplete: 'Sign in to both demo gyms once to enable their shared snapshots.', event_not_due: 'The selected Saturday has not reached noon Pacific.', event_created_too_late: 'Snapshot setup completed after this Saturday’s lock. This event cannot be captured retrospectively.', event_incomplete: 'Both gyms must capture their noon state before the complete snapshot is available.', event_not_found: 'No recorded snapshot was found for that request.', event_capacity: 'The snapshot store or display reached its supported capacity. Existing records remain available.', event_clock: 'The authority clock needs attention. No new capture was confirmed.', event_corrupt: 'The saved history could not be verified. Reopen the gym; retained evidence has not been discarded.', event_conflict: 'The snapshot evidence conflicts. Review the retained record before continuing.', event_cancelled: 'Capture was cancelled. Check its status because a completed capture remains saved.', event_calendar: 'Choose a supported Saturday in the entry panel.', outcome_unknown: 'The capture may be saved. Refresh snapshot status and inspect the recorded outcome before retrying.' })[code] ?? textError(code); }
function eventRefusal(code, capture = false) { if (['unauthorized', 'stale_view'].includes(code)) { clearEvent(); eventUnavailable = true; } if (code === 'event_unavailable') eventUnavailable = true; if (capture && code === 'outcome_unknown') eventUnknown = true; eventMessage = code === 'outcome_unknown' && !capture ? 'The authority could not confirm this status observation. Reopen the gym before another query; this does not confirm a capture.' : eventError(code); }
function eventAdoptStatus(s) {
  if (s.event.saturday !== entryDate) return; const old = eventStatus;
  if (old && old.event.event_id === s.event.event_id) requireValue(s.observed_at_ms >= old.observed_at_ms && old.captured_participants.every((p) => s.captured_participants.includes(p)) && (!old.snapshot_digest || s.snapshot_digest === old.snapshot_digest));
  eventStatus = s; if (!eventCapacity()) { eventStatus = old; throw new Error('Snapshot cache full'); }
}
async function eventReadStatus(auto = false) {
  if (!eventReady() || !entryDate) return; const c = eventContext(), dateValue = entryDate;
  try { const r = eventEnvelope(await call('EventStatus', c.viewID, dateValue), 'status'); if (!eventCurrent(c, true)) return; if (!r.ok) { eventRefusal(r.error); return; } requireValue(r.value.event.saturday === dateValue); eventAdoptStatus(r.value); if (r.value.captured_participants.includes(current.gym_id)) eventUnknown = false;
    const s = eventStatus; if (auto && s && s.event.cutoff_ms <= s.observed_at_ms && s.coordinator_created_at_ms < s.event.cutoff_ms && !s.captured_participants.includes(current.gym_id) && !eventWatch && !eventAutoAttempted.has(s.event.event_id) && eventAutoAttempted.size < 256 && !tasks.length && eventSnapshot.state === 'ready') { eventAutoAttempted.add(s.event.event_id); await eventBegin(); }
  } catch { if (eventCurrent(c)) eventMessage = eventError('event_unavailable'); } renderEvent();
}
async function fetchEventSnapshot() {
  if (!ready() || eventUnavailable) return; const c = eventContext();
  try { const r = eventEnvelope(await call('EventSnapshot'), 'snapshot'); if (!eventCurrent(c)) return; if (!r.ok) { eventRefusal(r.error); return; } const old = eventSnapshot; eventSnapshot = r.value; if (!eventCapacity()) { eventSnapshot = old; throw new Error('Snapshot cache full'); }
    if (eventSnapshot.state === 'unavailable') { eventUnavailable = true; return; } if (eventSnapshot.state === 'setup_incomplete') { eventMessage = eventError('event_setup_incomplete'); return; }
    if (eventDate !== entryDate) eventSelectDate(entryDate);
    if (eventWatch?.id) await eventFetchJob(eventWatch.id);
    else if (!tasks.length) { const id = eventSnapshot.active_job_id || eventSnapshot.queued_job_id || eventSnapshot.last_job_id; if (id && id !== eventLastJob) await eventFetchJob(id); }
    // The entry authority, never the browser clock, prompts the transition.
    if (eventReady() && !eventWatch && !tasks.length && eventSnapshot.state === 'ready' && entryDate && (!eventStatus || eventStatus.phase === 'before_cutoff' && entryStatus?.observed_at_ms >= eventStatus.event.cutoff_ms)) await eventReadStatus(true);
  } catch { if (eventCurrent(c)) { eventUnavailable = true; eventMessage = eventError('event_unavailable'); } } finally { renderEvent(); }
}
async function eventBegin() {
  if (!eventReady() || !eventStatus || eventStatus.event.saturday !== entryDate || eventWatch || eventUnknown || eventStatus.coordinator_created_at_ms >= eventStatus.event.cutoff_ms || eventStatus.observed_at_ms < eventStatus.event.cutoff_ms) return;
  const c = eventContext(), e = eventStatus.event, reservation = { id: '', eventID: e.event_id, saturday: e.saturday, participantID: current.gym_id }; eventWatch = reservation; if (!eventCapacity()) { eventWatch = null; eventMessage = eventError('event_capacity'); renderEvent(); return; }
  try { const r = eventEnvelope(await call('BeginEventCapture', c.viewID, e.saturday), 'job'); if (!eventCurrent(c)) return; if (!r.ok) { eventWatch = null; eventRefusal(r.error, true); return; } const j = r.value; requireValue(j.state === 'queued' && j.event_id === e.event_id && j.saturday === e.saturday && j.participant_id === reservation.participantID); eventDetail = null; eventWatch = { ...reservation, id: j.job_id }; eventMessage = 'Reconstructing and verifying this gym at noon Pacific…'; pollWanted = true;
  } catch { if (eventCurrent(c)) { eventWatch = null; eventUnknown = true; eventMessage = eventError('outcome_unknown'); } } renderEvent();
}
async function eventFetchJob(id) {
  if (!eventReady()) return; const c = eventContext(), w = eventWatch;
  try { const r = eventEnvelope(await call('EventJob', c.viewID, id), 'job'); if (!eventCurrent(c) || eventWatch !== w) return; if (!r.ok) { if (r.error === 'busy') return; eventWatch = null; eventRefusal(r.error, true); return; } const j = r.value; requireValue(j.job_id === id && (!w || j.event_id === w.eventID && j.saturday === w.saturday && j.participant_id === w.participantID));
    if (['queued', 'running'].includes(j.state)) { eventWatch = { id, eventID: j.event_id, saturday: j.saturday, participantID: j.participant_id }; requireValue(eventCapacity()); return; }
    eventWatch = null; eventLastJob = id;
    if (j.state === 'completed') { eventUnknown = false; if (j.saturday === entryDate) { const oldDetail = eventDetail; eventDetail = { kind: 'capture', value: j.result }; try { eventAdoptStatus(j.result.status); requireValue(eventCapacity()); } catch (error) { eventDetail = oldDetail; throw error; } } eventMessage = j.result.replayed ? 'The original saved capture was recovered unchanged.' : 'This gym’s noon state is saved. Later changes cannot alter it.'; }
    else { eventRefusal(j.error, true); eventMessage += careerAffected(j); } requireValue(eventCapacity());
  } catch { if (eventCurrent(c)) { eventWatch = null; eventUnknown = true; eventMessage = eventError('outcome_unknown'); } } renderEvent();
}
async function eventCancel() {
  if (!eventReady() || !eventWatch?.id) return; const c = eventContext(), w = eventWatch;
  try { const r = eventEnvelope(await call('CancelEventJob', c.viewID, w.id), 'job'); if (!eventCurrent(c)) return; if (!r.ok) { eventRefusal(r.error, true); return; } requireValue(r.value.job_id === w.id && r.value.event_id === w.eventID && r.value.saturday === w.saturday && r.value.participant_id === w.participantID); eventMessage = 'Cancellation requested. Check the recorded status; a completed capture remains saved.'; await eventFetchJob(w.id); }
  catch { if (eventCurrent(c)) { eventUnknown = true; eventMessage = eventError('outcome_unknown'); } } renderEvent();
}
async function eventReadRecord() {
  if (!eventReady() || !entryDate) return; const c = eventContext(), dateValue = entryDate;
  try { const r = eventEnvelope(await call('EventRecord', c.viewID, dateValue), 'record'); if (!eventCurrent(c, true)) return; if (!r.ok) { eventRefusal(r.error); return; } requireValue(r.value.input.event.saturday === dateValue); const old = eventDetail; eventDetail = { kind: 'record', value: r.value }; if (!eventCapacity()) { eventDetail = old; eventMessage = eventError('event_capacity'); renderEvent(); return; } eventUnknown = false; eventMessage = 'Complete immutable candidate snapshot loaded.'; }
  catch { if (eventCurrent(c)) eventMessage = eventError('event_unavailable'); } renderEvent();
}
async function eventReadHistory(after = '') {
  if (!eventReady()) return; const c = eventContext(); try { const r = eventEnvelope(await call('EventHistory', c.viewID, after, 8), 'history'); if (!eventCurrent(c)) return; if (!r.ok) { eventRefusal(r.error); return; } requireValue(r.value.after_event_id === after); const old = eventHistory; eventHistory = r.value; if (!eventCapacity()) { eventHistory = old; eventMessage = eventError('event_capacity'); } } catch { if (eventCurrent(c)) eventMessage = eventError('event_unavailable'); } renderEvent();
}
function eventDetailRenderKey(details) {
  if (!details) return '';
  const v = details.value, captures = details.kind === 'record' ? v.captures : [v.capture.evidence];
  const evidence = captures.map((c) => [c.participant_id, c.captured_at_ms, c.stable_fence.observed_at_ms, c.stable_fence.operation_count, c.entry_fence.observed_at_ms, c.entry_fence.operation_count]);
  return JSON.stringify(details.kind === 'record' ? ['record', v.snapshot_digest, v.completed_at_ms, evidence] : ['capture', v.status.event.event_id, v.capture.snapshot.entry.history_digest, v.capture.snapshot.roster.history_digest, evidence]);
}
function eventHistoryRenderKey(history) { return history ? JSON.stringify(history.events.map((e) => [e.event.event_id, e.phase, e.captured_participants.length])) : ''; }
function renderEvent() {
  const active = eventReady(), s = eventStatus?.event.saturday === entryDate ? eventStatus : null, busy = !!command || !!eventWatch || eventSnapshot?.state === 'busy';
  content(ui['event-state'], eventUnavailable ? 'Unavailable' : !ready() ? 'Sign in to view snapshots' : eventSnapshot?.state === 'setup_incomplete' ? 'Setup incomplete' : eventWatch ? 'Capturing noon state' : s?.phase === 'snapshotted' ? 'Snapshot complete' : s?.phase === 'capturing' ? 'One gym captured' : 'Candidate snapshot');
  content(ui['event-cutoff'], s ? `Saturday ${s.event.saturday} · State fixed at noon Pacific · ${new Date(s.event.cutoff_ms).toISOString()}` : 'Select a Saturday in the entry panel.');
  content(ui['event-progress'], s ? s.coordinator_created_at_ms >= s.event.cutoff_ms ? eventError('event_created_too_late') : `${s.captured_participants.length} of 2 gyms captured.${s.phase === 'before_cutoff' ? ' Waiting for noon Pacific.' : s.phase !== 'snapshotted' ? ' Each demo gym captures its own historical state after sign-in.' : ' The complete snapshot is immutable.'}` : '');
  content(ui['event-message'], eventMessage); content(ui['event-deadline'], s ? `If the required seed is still unfinalized at the following lock (${new Date(s.event.incident_deadline_ms).toISOString()}), escalate to the operator as a chain incident. This is the policy deadline; the seed panel reports observed evidence.` : '');
  content(ui['event-seed'], 'The seed panel reports candidate publication and checkpoint evidence. The event never receives a substitute seed and never automatically cancels.');
  content(ui['event-digest'], s?.snapshot_digest ? `Snapshot digest: ${s.snapshot_digest}` : '');
  ui['event-refresh'].disabled = !active || !entryDate || busy; ui['event-capture'].disabled = !active || busy || eventUnknown || !s || s.observed_at_ms < s.event.cutoff_ms || s.coordinator_created_at_ms >= s.event.cutoff_ms;
  content(ui['event-capture'], s?.captured_participants.includes(current?.gym_id) ? 'Recover this gym’s capture' : 'Capture this gym'); ui['event-cancel'].disabled = !active || !!command || !eventWatch?.id; ui['event-record'].disabled = !active || busy || s?.phase !== 'snapshotted'; ui['event-clear'].disabled = !eventDetail; ui['event-history'].disabled = !active || busy; ui['event-history-next'].disabled = !active || busy || !eventHistory?.has_more;
  const details = eventDetail && (eventDetail.kind === 'record' ? eventDetail.value.input.event.saturday : eventDetail.value.status.event.saturday) === entryDate ? eventDetail : null, detailKey = eventDetailRenderKey(details);
  if (ui['event-members']._key !== detailKey) { ui['event-members']._key = detailKey; ui['event-members'].textContent = ''; if (details) { const participants = details.kind === 'record' ? details.value.input.participants : [details.value.capture.snapshot], evidence = details.kind === 'record' ? details.value.captures : [details.value.capture.evidence]; participants.forEach((p, n) => { const name = p.participant_id === fixtures.zero256[0] ? 'Demo A' : 'Demo B'; ui['event-members'].append(element('li', '', `${name} · Candidate assignment at noon · Captured ${new Date(evidence[n].captured_at_ms).toISOString()}${p.members.length ? '' : ' · No submitted fighters at cutoff'}`)); for (const m of p.members) ui['event-members'].append(element('li', '', `${label(m.class)} · ${m.assignment.prospect_id} · ${label(m.eligibility)} · Noon rating ${(m.condition_at_cutoff.rating_hundredths / 100).toFixed(2)}${m.slot ? ` · Slot ${m.slot}` : ''}`)); }); } }
  const historyKey = eventHistoryRenderKey(eventHistory); requireValue(detailKey.length + historyKey.length <= EVENT_RENDER_KEY_BYTES); if (ui['event-history-list']._key !== historyKey) { ui['event-history-list']._key = historyKey; ui['event-history-list'].textContent = ''; if (eventHistory) for (const e of eventHistory.events) { const li = element('li'), button = element('button', '', e.event.saturday); button.type = 'button'; button.addEventListener('click', () => entryNavigate(e.event.saturday)); li.append(button, element('span', '', `${e.captured_participants.length} of 2 captured · ${e.phase === 'snapshotted' ? 'Complete' : 'Incomplete'}`)); ui['event-history-list'].append(li); } }
}
// Seed controls consume the native-validated fixed source authority. The
// browser has no transaction, key, outpoint, RPC, clock or checkpoint input.
const SEED_MAX = 262144, SEED_METADATA_MAX = 16384;
const SEED_ERRORS = new Set(['invalid_request', 'unauthorized', 'forbidden', 'stale_view', 'busy', 'outcome_unknown', 'seed_unavailable', 'seed_setup_incomplete', 'seed_corrupt', 'seed_capacity', 'seed_clock', 'seed_calendar', 'seed_created_too_late', 'seed_source_incomplete', 'seed_not_found', 'seed_not_prepared', 'seed_node_offline', 'seed_waiting_for_funds', 'seed_scan_incomplete', 'seed_conflict', 'seed_target_unavailable', 'seed_not_mature', 'seed_cancelled']);
let seedAuthority = null, seedSnapshot = null, seedStatus = null, seedDetail = null, seedHistory = null;
let seedWatch = null, seedLastJob = '', seedMessage = '', seedUnavailable = false, seedUnknown = false, seedSerial = 0;
function seedMetadataBytes() { return entryBytes([seedAuthority, seedSnapshot && { ...seedSnapshot, pin: null }, seedStatus, seedWatch, seedLastJob, seedMessage]) + 4096; }
function seedRetainedBytes() { if (!seedAuthority && !seedSnapshot && !seedWatch) return 0; return seedMetadataBytes() + Math.max(entryBytes(seedDetail), seedWatch ? 245760 : 0) + entryBytes(seedHistory); }
function seedCapacity() { return seedMetadataBytes() <= SEED_METADATA_MAX && seedRetainedBytes() <= SEED_MAX && previewCapacity(); }
function seedReady() { return ready() && !seedUnavailable && !!seedAuthority && !!seedSnapshot?.pin && viewKey(seedSnapshot.view) === currentKey(); }
function clearSeed() { clearCard(); seedSerial++; seedSnapshot = seedStatus = seedDetail = seedHistory = seedWatch = null; seedLastJob = seedMessage = ''; seedUnavailable = seedUnknown = false; if (ui['seed-history-list']) { ui['seed-history-list'].textContent = ''; ui['seed-history-list']._key = ''; } }
function seedContext() { return { epoch, view: currentKey(), viewID: current?.view?.view_id, serial: seedSerial, date: entryDate }; }
function seedCurrent(c, selection = false) { return !disposed && ready() && c.epoch === epoch && c.view === currentKey() && (!selection || c.serial === seedSerial && c.date === entryDate); }
function seedHex(v, bytes) { return Number.isInteger(bytes) && bytes > 0 && typeof v === 'string' && v.length === bytes * 2 && /^[0-9a-f]+$/.test(v); }
function seedTip(t) { exact(t, ['height', 'hash']); requireValue(int(t.height, 0, 2147483647) && hex(t.hash)); }
function seedCheckpoint(c) { exact(c, ['version', 'network', 'genesis', 'height', 'hash', 'signature']); requireValue(c.version === 1 && c.network === 0x534c5547 && c.genesis === GENESIS && int(c.height, 1, 2147483647) && hex(c.hash) && seedHex(c.signature, 64)); }
function seedPin(p) {
  exact(p, ['version', 'source_store_id', 'source_pin', 'publisher_gym_id', 'publisher_account_id', 'publisher_source_authority_id', 'publisher_chain_id', 'network', 'genesis', 'floor_height', 'floor_hash', 'publisher_public_key', 'checkpoint_public_key', 'funding_public_key', 'funding_script', 'rules_id']);
  requireValue(p.version === 1 && !!eventAuthority && p.source_store_id === eventAuthority.storeID && same(p.source_pin, eventAuthority.pin) && p.publisher_gym_id === fixtures.zero256[0] && p.network === 0x534c5547 && p.genesis === GENESIS && p.floor_height === 0 && p.floor_hash === GENESIS);
  for (const k of ['publisher_account_id', 'publisher_source_authority_id', 'publisher_chain_id', 'publisher_public_key', 'checkpoint_public_key', 'rules_id']) requireValue(hex(p[k]));
  const zero = p.source_pin.participants.find((v) => v.stable_pin.identity.gym_id === fixtures.zero256[0]); requireValue(!!zero && zero.stable_pin.account_id === p.publisher_account_id && zero.stable_pin.source.authority_id === p.publisher_source_authority_id && seedHex(p.funding_public_key, 33) && seedHex(p.funding_script, 22));
  return p;
}
function seedStatusValue(s) {
  exact(s, ['version', 'store_id', 'event_key', 'event', 'exists', 'phase', 'publication_state', 'scan_height', 'scan_tip_height', 'anchor_height', 'anchor_hash', 'target_height', 'target_hash', 'target_descendants', 'checkpoint_present', 'finalized_at_ms', 'conflict_count', 'conflict_overflow', 'incident_at_ms', 'prepared_at_ms', 'observed_at_ms', 'chain_observed_at_ms', 'node_state', 'funding_state', 'current_finality', 'escalation_due', 'scan_generation', 'funding_generation']);
  eventValue(s.event); requireValue(s.version === 1 && s.store_id === seedAuthority.storeID && hex(s.event_key) && ['absent', 'prepared', 'waiting_anchor', 'waiting_target', 'provisional', 'finalized', 'conflict'].includes(s.phase) && ['none', 'retained', 'unknown', 'submitted'].includes(s.publication_state));
  for (const k of ['exists', 'checkpoint_present', 'conflict_overflow', 'current_finality', 'escalation_due']) requireValue(typeof s[k] === 'boolean');
  for (const k of ['scan_height', 'scan_tip_height']) requireValue(int(s[k], -1, 2147483647));
  for (const k of ['anchor_height', 'target_height', 'target_descendants']) requireValue(int(s[k], 0, 2147483647));
  for (const k of ['finalized_at_ms', 'incident_at_ms', 'prepared_at_ms', 'chain_observed_at_ms']) requireValue(int(s[k], 0, s.observed_at_ms));
  requireValue(int(s.observed_at_ms, seedAuthority.createdMS, DATE_MAX) && optionalID(s.anchor_hash) && optionalID(s.target_hash) && int(s.conflict_count, 0, 8) && int(s.scan_generation, 0, 16) && int(s.funding_generation, 0, 16));
  requireValue(['unobserved', 'online', 'offline', 'unavailable'].includes(s.node_state) && ['unobserved', 'scanning', 'ready', 'waiting_for_funds', 'unavailable'].includes(s.funding_state));
  requireValue(!s.current_finality || s.finalized_at_ms > 0 && s.target_descendants >= 60 && s.chain_observed_at_ms > 0 && !s.conflict_count && !s.conflict_overflow);
  requireValue(s.escalation_due === (s.observed_at_ms >= s.event.incident_deadline_ms && (!s.finalized_at_ms || s.conflict_count > 0 || s.conflict_overflow)));
  return s;
}
function seedCursor(c) { exact(c, ['generation', 'height', 'hash', 'scan_digest', 'complete_to_height', 'complete_to_hash']); requireValue(int(c.generation, 0, 16) && int(c.height, -1, 2147483647) && int(c.complete_to_height, -1, c.height) && optionalID(c.hash) && optionalID(c.complete_to_hash) && hex(c.scan_digest)); }
function seedDeclaration(s) {
  exact(s, ['version', 'declaration', 'signature']); requireValue(s.version === 1 && seedHex(s.signature, 64)); const d = s.declaration;
  exact(d, ['version', 'event_key', 'store_id', 'source_store_id', 'event', 'snapshot_digest', 'receipt_digest', 'publisher_chain_id', 'network', 'genesis', 'floor_height', 'floor_hash', 'rules_id', 'publisher_public_key', 'checkpoint_public_key']); eventValue(d.event);
  requireValue(d.version === 1 && d.network === 0x534c5547 && d.genesis === GENESIS && d.floor_height === 0 && d.floor_hash === GENESIS && ['event_key', 'store_id', 'source_store_id', 'snapshot_digest', 'receipt_digest', 'publisher_chain_id', 'rules_id', 'publisher_public_key', 'checkpoint_public_key'].every((k) => hex(d[k]))); return d;
}
function seedRecordValue(r) {
  exact(r, ['version', 'store_id', 'event_key', 'source_receipt', 'declaration', 'prepared_at_ms', 'publication', 'cursor', 'scan_restarts', 'anchor', 'target', 'checkpoint', 'finality', 'conflicts', 'conflict_overflow', 'incident_at_ms', 'observation']);
  requireValue(entryBytes(r) <= 229376 && r.version === 1 && r.store_id === seedAuthority.storeID && hex(r.event_key)); eventRecordValue(r.source_receipt); const d = seedDeclaration(r.declaration), e = r.source_receipt.input.event;
  for (const k of ['publication', 'anchor', 'target', 'checkpoint', 'finality']) requireValue(r[k] === null || !!r[k] && typeof r[k] === 'object' && !Array.isArray(r[k]));
  requireValue(d.store_id === r.store_id && d.event_key === r.event_key && same(d.event, e) && e.cutoff_ms > seedAuthority.createdMS && int(r.prepared_at_ms, e.cutoff_ms, DATE_MAX)); seedCursor(r.cursor);
  if (r.publication) { const p = r.publication; exact(p, ['version', 'raw_tx', 'txid', 'coin', 'built_at_ms', 'dispatch_count', 'first_dispatch_at_ms', 'last_dispatch_at_ms', 'state']); requireValue(p.version === 1 && typeof p.raw_tx === 'string' && p.raw_tx.length <= 2048 && seedHex(p.raw_tx, p.raw_tx.length / 2) && hex(p.txid) && ['retained', 'unknown', 'submitted'].includes(p.state) && int(p.dispatch_count)); exact(p.coin, ['outpoint', 'value_satoshis', 'script', 'block_height', 'block_hash', 'coinbase']); exact(p.coin.outpoint, ['txid', 'index']); requireValue(hex(p.coin.outpoint.txid) && int(p.coin.outpoint.index, 0, 4294967295) && int(p.coin.value_satoshis, 1294, 2100000000000000) && p.coin.script === seedAuthority.pin.funding_script && int(p.coin.block_height, 0, 2147483647) && hex(p.coin.block_hash) && typeof p.coin.coinbase === 'boolean'); for (const k of ['built_at_ms', 'first_dispatch_at_ms', 'last_dispatch_at_ms']) requireValue(int(p[k], 0, DATE_MAX)); }
  requireValue(Array.isArray(r.scan_restarts) && r.scan_restarts.length <= 16 && r.cursor.generation === r.scan_restarts.length); for (const v of r.scan_restarts) { exact(v, ['generation', 'previous', 'observed_at_ms', 'observed_tip', 'replacement_hash']); seedCursor(v.previous); seedTip(v.observed_tip); requireValue(v.generation === v.previous.generation + 1 && int(v.observed_at_ms, r.prepared_at_ms, DATE_MAX) && optionalID(v.replacement_hash)); }
  if (r.anchor) { const a = r.anchor; exact(a, ['height', 'hash', 'header', 'txid', 'raw_tx_digest', 'raw_tx_size', 'tx_count', 'tx_index', 'output_index', 'payload', 'merkle_branch']); requireValue(int(a.height, 0, 2147483647) && ['hash', 'txid', 'raw_tx_digest'].every((k) => hex(a[k])) && seedHex(a.header, 80) && seedHex(a.payload, 73) && int(a.raw_tx_size, 1, 4000000) && int(a.tx_count, 1, 1000000) && int(a.tx_index, 0, a.tx_count - 1) && int(a.output_index, 0, 4294967295) && Array.isArray(a.merkle_branch) && a.merkle_branch.length <= 20 && a.merkle_branch.every(hex)); }
  if (r.target) { const t = r.target; exact(t, ['height', 'hash', 'header', 'observed_at_ms', 'observed_tip']); requireValue(!!r.anchor && t.height === r.anchor.height + 1 && hex(t.hash) && seedHex(t.header, 80) && int(t.observed_at_ms, r.prepared_at_ms, DATE_MAX)); seedTip(t.observed_tip); }
  if (r.checkpoint) seedCheckpoint(r.checkpoint);
  if (r.finality) { const f = r.finality; exact(f, ['checkpoint', 'observed_at_ms', 'observed_tip', 'target_descendants']); seedCheckpoint(f.checkpoint); seedTip(f.observed_tip); requireValue(!!r.target && f.checkpoint.height === r.target.height && f.checkpoint.hash === r.target.hash && f.target_descendants === f.observed_tip.height - r.target.height && f.target_descendants >= 60 && int(f.observed_at_ms, r.prepared_at_ms, DATE_MAX)); }
  requireValue(Array.isArray(r.conflicts) && r.conflicts.length <= 8 && typeof r.conflict_overflow === 'boolean'); for (const c of r.conflicts) { exact(c, ['kind', 'observed_at_ms', 'declaration', 'checkpoint', 'expected_height', 'expected_hash', 'observed_hash', 'observed_tip']); requireValue(['declaration', 'checkpoint', 'chain'].includes(c.kind) && int(c.observed_at_ms, r.prepared_at_ms, DATE_MAX) && int(c.expected_height, 0, 2147483647) && optionalID(c.expected_hash) && optionalID(c.observed_hash)); if (c.declaration) seedDeclaration(c.declaration); if (c.checkpoint) seedCheckpoint(c.checkpoint); if (c.observed_tip) seedTip(c.observed_tip); }
  const o = r.observation; exact(o, ['observed_at_ms', 'chain_observed_at_ms', 'tip', 'node_state', 'target_descendants', 'current_finality']); requireValue((o.observed_at_ms === 0 || int(o.observed_at_ms, r.prepared_at_ms, DATE_MAX)) && int(o.chain_observed_at_ms, 0, o.observed_at_ms) && ['unobserved', 'online', 'offline', 'unavailable'].includes(o.node_state) && int(o.target_descendants, 0, 2147483647) && typeof o.current_finality === 'boolean' && int(r.incident_at_ms, 0, DATE_MAX)); requireValue((o.chain_observed_at_ms === 0) === (o.tip === null)); if (o.tip !== null) seedTip(o.tip); return r;
}
function seedEnvelope(r, field) {
  exact(r, ['id', 'ok', 'view', 'store_id', 'snapshot', 'status', 'record', 'history', 'job', 'error']); requireValue(int(r.id, 1) && typeof r.ok === 'boolean' && entryBytes(r) + 1 <= SEED_MAX);
  const payloads = ['snapshot', 'status', 'record', 'history', 'job']; if (!r.ok) { requireValue(SEED_ERRORS.has(r.error) && r.view === null && r.store_id === '' && payloads.every((k) => r[k] === null)); return { ok: false, error: r.error }; }
  requireValue(r.error === '' && payloads.every((k) => k === field ? r[k] !== null : r[k] === null)); const v = r[field];
  if (field === 'snapshot') {
    exact(v, ['version', 'state', 'error', 'view', 'store_id', 'pin', 'created_at_ms', 'publisher_account_id', 'node_state', 'active_job_id', 'queued_job_id', 'last_job_id']); requireValue(v.version === 1 && ['ready', 'busy', 'setup_incomplete', 'unavailable', 'failed'].includes(v.state) && ['ready', 'offline'].includes(v.node_state));
    if (v.state === 'unavailable') { requireValue(r.view === null && v.view === null && v.pin === null && r.store_id === '' && v.store_id === '' && v.created_at_ms === 0 && v.publisher_account_id === '' && v.error === 'seed_unavailable' && v.node_state === 'offline' && ['active_job_id', 'queued_job_id', 'last_job_id'].every((k) => v[k] === '')); return { ok: true, value: v }; }
    requireValue(viewKey(checkedView(v.view)) === currentKey() && viewKey(checkedView(r.view)) === currentKey() && r.store_id === v.store_id);
    if (v.state === 'setup_incomplete') { requireValue(!seedAuthority && v.pin === null && v.store_id === '' && v.created_at_ms === 0 && v.publisher_account_id === '' && v.error === 'seed_setup_incomplete' && v.node_state === 'offline' && ['active_job_id', 'queued_job_id', 'last_job_id'].every((k) => v[k] === '')); return { ok: true, value: v }; }
    seedPin(v.pin); requireValue(hex(v.store_id) && int(v.created_at_ms, 1, DATE_MAX) && v.publisher_account_id === v.pin.publisher_account_id && ['active_job_id', 'queued_job_id', 'last_job_id'].every((k) => optionalID(v[k])) && (v.state === 'failed' ? ['seed_unavailable', 'seed_corrupt', 'seed_clock'].includes(v.error) : v.error === ''));
    const authority = { pin: v.pin, storeID: v.store_id, createdMS: v.created_at_ms }; requireValue(!seedAuthority || same(seedAuthority, authority)); if (!seedAuthority) seedAuthority = authority; v.pin = seedAuthority.pin; return { ok: true, value: v };
  }
  requireValue(seedReady() && r.store_id === seedAuthority.storeID && viewKey(checkedView(r.view)) === currentKey());
  if (field === 'status') seedStatusValue(v);
  if (field === 'record') seedRecordValue(v);
  if (field === 'history') { exact(v, ['version', 'store_id', 'after_event_key', 'next_event_key', 'has_more', 'observed_at_ms', 'events']); requireValue(v.version === 1 && v.store_id === seedAuthority.storeID && optionalID(v.after_event_key) && optionalID(v.next_event_key) && typeof v.has_more === 'boolean' && int(v.observed_at_ms, seedAuthority.createdMS, DATE_MAX) && Array.isArray(v.events) && v.events.length <= 8); v.events.forEach((s, n) => { seedStatusValue(s); requireValue(s.exists && s.observed_at_ms === v.observed_at_ms && (!n || v.events[n - 1].event.cutoff_ms < s.event.cutoff_ms)); }); requireValue(v.next_event_key === (v.has_more && v.events.length ? v.events.at(-1).event_key : '')); }
  if (field === 'job') { exact(v, ['version', 'job_id', 'kind', 'state', 'view', 'store_id', 'event_key', 'saturday', 'error', 'result']); requireValue(v.version === 1 && hex(v.job_id) && hex(v.event_key) && v.store_id === seedAuthority.storeID && viewKey(checkedView(v.view)) === currentKey() && ['prepare', 'publish', 'reconcile', 'attest'].includes(v.kind) && ['queued', 'running', 'completed', 'failed', 'cancelled'].includes(v.state)); entryDateValue(v.saturday); if (v.state === 'completed') { requireValue(v.error === '' && v.result !== null); seedRecordValue(v.result); requireValue(v.result.event_key === v.event_key && v.result.source_receipt.input.event.saturday === v.saturday); } else requireValue(v.result === null && (['queued', 'running'].includes(v.state) ? v.error === '' : SEED_ERRORS.has(v.error) && (v.state !== 'cancelled' || v.error === 'seed_cancelled'))); }
  return { ok: true, value: v };
}
function seedError(code, kind = '') { if (code === 'outcome_unknown' && kind === 'reconcile') return 'The chain observation may have been recorded. No transaction was sent. Read saved evidence before another explicit chain check.'; return ({ seed_unavailable: 'Seed evidence is unavailable in this app route.', seed_setup_incomplete: 'Sign in to both demo gyms to finish the shared seed setup.', seed_source_incomplete: 'Complete both noon snapshots before preparing the event seed.', seed_created_too_late: 'This seed journal was established after the selected lock. Choose a later Saturday.', seed_not_prepared: 'Prepare this event’s declaration first.', seed_node_offline: 'The fixed Demo A node pair is offline. Start that gym explicitly to inspect its chain.', seed_waiting_for_funds: 'The separate test publisher has no mature funding output. No gym wallet will be spent.', seed_scan_incomplete: 'The bounded chain scan is incomplete. Choose Check chain again to continue.', seed_conflict: 'The fixed seed evidence conflicts. Retained evidence stays available; refer the incident to the operator.', seed_target_unavailable: 'Waiting for the exact block after publication. No substitute will be selected.', seed_not_mature: 'The required activation or confirmation depth has not been reached.', seed_not_found: 'No seed record has been prepared for this Saturday.', seed_capacity: 'The supported seed evidence or display capacity is full. Existing evidence is retained.', seed_clock: 'The local authority clock needs attention.', seed_corrupt: 'Saved seed evidence failed validation. Reopen the gym; no evidence has been discarded.', seed_cancelled: 'The request was cancelled. Check the saved record before another action.', seed_calendar: 'Choose a supported Saturday in the entry panel.', outcome_unknown: 'The operation may have been recorded or sent. Read the saved record and check the chain before another explicit action.' })[code] ?? textError(code); }
function seedRefusal(code, action = false, kind = '') { if (['unauthorized', 'stale_view'].includes(code)) { clearSeed(); seedUnavailable = true; } if (code === 'seed_unavailable') seedUnavailable = true; if (action && code === 'outcome_unknown') seedUnknown = true; seedMessage = seedError(code, kind); }
async function fetchSeedSnapshot() {
  if (!ready() || seedUnavailable) return; const c = seedContext();
  try { const raw = await call('SeedSnapshot'); if (!seedCurrent(c)) return; const old = [seedAuthority, seedSnapshot]; const r = seedEnvelope(raw, 'snapshot'); if (!r.ok) { seedRefusal(r.error); return; } seedSnapshot = r.value; if (!seedCapacity()) { [seedAuthority, seedSnapshot] = old; throw new Error('Seed cache full'); } if (seedSnapshot.state === 'unavailable') { seedUnavailable = true; return; }
    const id = seedWatch?.id || seedSnapshot.active_job_id || seedSnapshot.queued_job_id || seedSnapshot.last_job_id; if (id && (seedWatch || id !== seedLastJob)) await seedFetchJob(id);
  } catch { if (seedCurrent(c)) seedMessage = seedError('seed_unavailable'); } renderSeed();
}
async function seedRead(field = 'status', after = '') {
  if (!seedReady() || field !== 'history' && !entryDate) return; const c = seedContext();
  try { const args = field === 'history' ? [after, 8] : [c.date]; const raw = await call({ status: 'SeedStatus', record: 'SeedRecord', history: 'SeedHistory' }[field], c.viewID, ...args); if (!seedCurrent(c, field !== 'history')) return; const r = seedEnvelope(raw, field); if (!r.ok) { seedRefusal(r.error); return; }
    // Present one complete selected observation; never mix a stale Record with a newer Status.
    const old = [seedStatus, seedDetail, seedHistory, seedUnknown]; if (field === 'status') { requireValue(r.value.event.saturday === c.date); seedStatus = r.value; seedDetail = null; } if (field === 'record') { requireValue(r.value.source_receipt.input.event.saturday === c.date); seedDetail = r.value; seedStatus = null; seedUnknown = false; } if (field === 'history') { requireValue(r.value.after_event_key === after); seedHistory = r.value; }
    if (!seedCapacity()) { [seedStatus, seedDetail, seedHistory, seedUnknown] = old; throw new Error('Seed cache full'); } seedMessage = field === 'record' ? 'Retained seed evidence loaded. This read did not check the chain or send a transaction.' : '';
  } catch { if (seedCurrent(c)) seedMessage = seedError('seed_unavailable'); } renderSeed();
}
async function seedBegin(kind) {
  if (!seedReady() || !entryDate || seedWatch || seedUnknown && kind !== 'reconcile' || seedSnapshot.state === 'busy' || !['prepare', 'publish', 'reconcile', 'attest'].includes(kind)) return;
  if (['publish', 'attest'].includes(kind) && (current.account.held || current.account.id !== seedAuthority.pin.publisher_account_id)) { seedRefusal('forbidden'); return; }
  const c = seedContext(); seedWatch = { id: '', kind, saturday: c.date, eventKey: '' }; if (!seedCapacity()) { seedWatch = null; seedMessage = seedError('seed_capacity'); return; }
  try { const raw = await call({ prepare: 'BeginSeedPrepare', publish: 'BeginSeedPublish', reconcile: 'BeginSeedReconcile', attest: 'BeginSeedAttest' }[kind], c.viewID, c.date); if (!seedCurrent(c)) return; const r = seedEnvelope(raw, 'job'); if (!r.ok) { seedWatch = null; seedRefusal(r.error, true, kind); return; } const j = r.value; requireValue(j.kind === kind && j.saturday === c.date && j.state === 'queued'); seedDetail = seedHistory = null; seedWatch = { id: j.job_id, kind, saturday: j.saturday, eventKey: j.event_key }; seedMessage = 'Seed request queued.';
  } catch { if (seedCurrent(c)) { seedWatch = null; seedRefusal('outcome_unknown', true, kind); } } renderSeed();
}
async function seedFetchJob(id) {
  if (!seedReady()) return; const c = seedContext(), expected = seedWatch;
  try { const raw = await call('SeedJob', c.viewID, id); if (!seedCurrent(c) || seedWatch !== expected) return; const r = seedEnvelope(raw, 'job'); if (!r.ok) { if (r.error !== 'busy') { seedWatch = null; seedRefusal(r.error, true, expected?.kind); } return; } const j = r.value; requireValue(j.job_id === id && (!expected || j.kind === expected.kind && j.saturday === expected.saturday && (!expected.eventKey || j.event_key === expected.eventKey)));
    if (['queued', 'running'].includes(j.state)) { seedWatch = { id, kind: j.kind, saturday: j.saturday, eventKey: j.event_key }; requireValue(seedCapacity()); return; }
    const retained = [seedWatch, seedLastJob, seedDetail, seedUnknown, seedStatus]; seedWatch = null; seedLastJob = id;
    if (j.state === 'completed') { seedUnknown = false; if (j.saturday === entryDate) { seedDetail = j.result; seedStatus = null; } seedMessage = j.kind === 'reconcile' ? 'Chain evidence checked. No transaction was sent.' : 'The seed operation completed. Its retained evidence is shown below.'; } else seedRefusal(j.error, true, j.kind); if (!seedCapacity()) { [seedWatch, seedLastJob, seedDetail, seedUnknown, seedStatus] = retained; seedMessage = seedError('seed_capacity'); }
  } catch { if (seedCurrent(c)) { seedWatch = null; seedRefusal('outcome_unknown', true, expected?.kind); } } renderSeed();
}
async function seedCancel() {
  if (!seedReady() || !seedWatch?.id) return; const c = seedContext(), w = seedWatch;
  try { const raw = await call('CancelSeedJob', c.viewID, w.id); if (!seedCurrent(c)) return; const r = seedEnvelope(raw, 'job'); if (!r.ok) { seedRefusal(r.error, true, w.kind); return; } requireValue(r.value.job_id === w.id && r.value.kind === w.kind && r.value.saturday === w.saturday && r.value.event_key === w.eventKey); await seedFetchJob(w.id); } catch { if (seedCurrent(c)) seedRefusal('outcome_unknown', true, w.kind); } renderSeed();
}
function seedRecordPhase(r) { return r.conflicts.length || r.conflict_overflow ? 'conflict' : r.finality ? 'finalized' : r.target ? 'provisional' : r.anchor ? 'waiting_target' : r.publication ? 'waiting_anchor' : 'prepared'; }
function renderSeed() {
  const active = seedReady(), s = seedStatus?.event.saturday === entryDate ? seedStatus : null, r = seedDetail?.source_receipt.input.event.saturday === entryDate ? seedDetail : null;
  const busy = !!command || !!seedWatch || seedSnapshot?.state === 'busy', publisher = active && !current.account.held && current.account.id === seedAuthority.pin.publisher_account_id;
  content(ui['seed-state'], !ready() ? 'Sign in to view seed evidence' : seedUnavailable ? 'Unavailable in this route' : seedSnapshot?.state === 'setup_incomplete' ? 'Setup incomplete' : seedWatch ? `${label(seedWatch.kind)} in progress` : s ? label(s.phase) : r ? label(seedRecordPhase(r)) : 'Candidate seed evidence');
  content(ui['seed-message'], seedMessage); const o = r?.observation, chainTime = o?.chain_observed_at_ms ?? s?.chain_observed_at_ms ?? 0;
  content(ui['seed-observation'], `Fixed Demo A nodes: ${seedSnapshot?.node_state ?? 'offline'}. ${chainTime ? `Last verified chain observation: ${new Date(chainTime).toISOString()}.` : 'No verified chain observation yet.'} ${s ? `Funding: ${label(s.funding_state)}. Scan through height ${s.scan_height}.` : ''}`);
  const finalized = r?.finality?.observed_at_ms ?? s?.finalized_at_ms ?? 0, eligible = o?.current_finality ?? s?.current_finality ?? false, depth = o?.target_descendants ?? s?.target_descendants ?? 0;
  content(ui['seed-finality'], finalized ? `First finality recorded ${new Date(finalized).toISOString()}. Eligible at the last verified observation: ${eligible ? 'yes' : 'no'}; ${depth} target descendants. This is retained candidate evidence, not continuous chain monitoring.` : 'Waiting for the exact seed block and its checkpoint with 60 descendants. No substitute seed or automatic cancellation.');
  const incident = r?.incident_at_ms ?? s?.incident_at_ms ?? 0; content(ui['seed-incident'], incident ? `Chain incident first recorded ${new Date(incident).toISOString()}. Refer the evidence to the operator under the checkpoint/community rule.` : s?.escalation_due ? 'Chain incident escalation is due; no first incident has been journaled for this event.' : s ? `Escalation boundary: ${new Date(s.event.incident_deadline_ms).toISOString()}. The event waits.` : '');
  content(ui['seed-identity'], r ? `Event ${r.event_key} · ${r.publication ? `Retained transaction ${r.publication.txid} (${r.publication.state})` : 'No retained transaction'} · Anchor ${r.anchor?.hash ?? 'pending'} · Target ${r.target?.hash ?? 'pending'}` : '');
  for (const k of ['refresh', 'record']) ui[`seed-${k}`].disabled = !active || busy || !entryDate;
  for (const k of ['prepare', 'publish', 'reconcile', 'attest']) ui[`seed-${k}`].disabled = !active || busy || !entryDate || seedUnknown && k !== 'reconcile' || ['publish', 'attest'].includes(k) && !publisher;
  ui['seed-cancel'].disabled = !active || !!command || !seedWatch?.id; ui['seed-clear'].disabled = !seedDetail && !seedHistory; ui['seed-history'].disabled = !active || busy; ui['seed-history-next'].disabled = !active || busy || !seedHistory?.has_more;
  const historyKey = seedHistory ? JSON.stringify(seedHistory.events.map((v) => [v.event_key, v.phase, v.incident_at_ms])) : ''; requireValue(historyKey.length + ui['seed-identity'].textContent.length <= 4096);
  if (ui['seed-history-list']._key !== historyKey) { ui['seed-history-list']._key = historyKey; ui['seed-history-list'].textContent = ''; if (seedHistory) for (const s of seedHistory.events) { const li = element('li'), button = element('button', '', s.event.saturday); button.type = 'button'; button.addEventListener('click', () => entryNavigate(s.event.saturday)); li.append(button, element('span', '', label(s.phase))); ui['seed-history-list'].append(li); } }
}
// C1 candidate cards: the native boundary owns source truth and canonical hashes.
// This consumer checks bounded shape, count/pair relationships and current-view
// correlation. It never regenerates names, hashes proof headers or books on poll.
const CARD_RULES = '879f012a9520a8c3066b0a90e47f7069472a2d7b8ab9b32f3b80d98495eed76d';
const CARD_MAX = 294912, CARD_METADATA_MAX = 49152, CARD_RESULT_MAX = 245760;
const CARD_BASIS = 'candidate_zero_record_no_results_authority';
const CARD_REASONS = ['retired_at_cutoff', 'not_rostered_at_cutoff', 'no_opponent_in_class', 'opponent_ineligible_at_cutoff', 'outside_rating_band'];
const CARD_ERRORS = new Set([...CAREER_ACTION_ERRORS, 'invalid_request', 'unauthorized', 'forbidden', 'stale_view', 'busy', 'outcome_unknown', 'card_unavailable', 'card_setup_incomplete', 'card_corrupt', 'card_capacity', 'card_clock', 'card_calendar', 'card_not_found', 'card_source_incomplete', 'card_source_conflict', 'card_source_changed', 'card_cancelled']);
const cardReasonText = {
  retired_at_cutoff: 'Retired at the noon cutoff',
  not_rostered_at_cutoff: 'Off the roster at the noon cutoff',
  no_opponent_in_class: 'No opposing entrant in this weight class',
  opponent_ineligible_at_cutoff: 'The opposing entrant was ineligible at noon',
  outside_rating_band: 'The opposing rating falls outside the half-to-double band',
};
let cardAuthority = null, cardSnapshot = null, cardStatus = null, cardDetail = null, cardHistory = null;
let cardWatch = null, cardTerminal = null, cardLastJob = '', cardMessage = '', cardSerial = 0;
let cardUnavailable = false, cardUnknown = false, cardOverlayKnown = false, cardOverlayRead = false;

function cardMetadataBytes() {
  return entryBytes([cardAuthority, cardSnapshot && { ...cardSnapshot, pin: null }, cardStatus, cardHistory, cardWatch, cardTerminal, cardLastJob, cardMessage]) + 4096;
}
function cardRetainedBytes() {
  if (!cardAuthority && !cardSnapshot && !cardWatch && !cardDetail) return 0;
  if (cardWatch) return CARD_MAX;
  return cardMetadataBytes() + Math.max(entryBytes(cardDetail), cardWatch ? CARD_RESULT_MAX : 0);
}
function cardCapacity() { return cardMetadataBytes() <= CARD_METADATA_MAX && entryBytes(cardDetail) <= CARD_RESULT_MAX && cardRetainedBytes() <= CARD_MAX && previewCapacity(); }
function cardReady() { return ready() && !cardUnavailable && !!cardAuthority && !!cardSnapshot?.pin && viewKey(cardSnapshot.view) === currentKey(); }
function clearCard() {
  clearBooked();
  cardSerial++; cardSnapshot = cardStatus = cardDetail = cardHistory = cardWatch = cardTerminal = null;
  cardLastJob = cardMessage = ''; cardUnavailable = cardUnknown = cardOverlayKnown = cardOverlayRead = false;
  for (const id of ['card-gyms', 'card-bouts', 'card-unmatched', 'card-reasons', 'card-history-list']) {
    if (ui[id]) { ui[id].textContent = ''; ui[id]._key = ''; }
  }
}
function cardSelectDate() { bookedSelectDate(); cardSerial++; cardStatus = cardDetail = null; cardMessage = ''; cardOverlayKnown = cardOverlayRead = false; }
function cardCacheError() { const e = new Error('Card cache full'); e.cardCode = 'card_capacity'; return e; }
function cardContext() { return { epoch, view: currentKey(), viewID: current?.view?.view_id, serial: cardSerial, date: entryDate }; }
function cardCurrent(c, selection = false) { return !disposed && ready() && c.epoch === epoch && c.view === currentKey() && (!selection || c.serial === cardSerial && c.date === entryDate); }
function cardPinValue(p) {
  exact(p, ['version', 'source_store_id', 'source_pin', 'rules_id']);
  requireValue(!!seedAuthority && p.version === 1 && p.source_store_id === seedAuthority.storeID && same(p.source_pin, seedAuthority.pin) && p.rules_id === CARD_RULES);
}
function cardCountsValue(c, max = 10) {
  exact(c, ['total_count', 'eligible_count', 'paired_count', 'unmatched_count', 'reasons']); exact(c.reasons, CARD_REASONS);
  requireValue(['total_count', 'eligible_count', 'paired_count', 'unmatched_count'].every(k => int(c[k], 0, max)) && c.eligible_count <= c.total_count && c.paired_count <= c.eligible_count && c.total_count === c.paired_count + c.unmatched_count);
  requireValue(CARD_REASONS.every(k => int(c.reasons[k], 0, max)) && CARD_REASONS.reduce((n, k) => n + c.reasons[k], 0) === c.unmatched_count);
  return c;
}
function cardSourceValue(s) {
  exact(s, ['version', 'store_id', 'event_key', 'source_receipt', 'declaration', 'anchor', 'target', 'finality']);
  requireValue(s.version === 1 && s.store_id === seedAuthority.storeID && hex(s.event_key) && entryBytes(s) <= 163840);
  eventRecordValue(s.source_receipt); const d = seedDeclaration(s.declaration), p = seedAuthority.pin;
  requireValue(d.store_id === s.store_id && d.event_key === s.event_key && same(d.event, s.source_receipt.input.event) && d.snapshot_digest === s.source_receipt.snapshot_digest);
  for (const k of ['source_store_id', 'publisher_chain_id', 'network', 'genesis', 'floor_height', 'floor_hash', 'rules_id', 'publisher_public_key', 'checkpoint_public_key']) requireValue(d[k] === p[k]);
  const a = s.anchor; exact(a, ['height', 'hash', 'header', 'txid', 'raw_tx_digest', 'raw_tx_size', 'tx_count', 'tx_index', 'output_index', 'payload', 'merkle_branch']);
  requireValue(int(a.height, 0, 2147483646) && ['hash', 'txid', 'raw_tx_digest'].every(k => hex(a[k])) && seedHex(a.header, 80) && seedHex(a.payload, 73) && int(a.raw_tx_size, 1, 4000000) && int(a.tx_count, 1, 1000000) && int(a.tx_index, 0, a.tx_count - 1) && int(a.output_index, 0, 4294967295) && Array.isArray(a.merkle_branch) && a.merkle_branch.length <= 20 && a.merkle_branch.every(hex));
  const t = s.target; exact(t, ['height', 'hash', 'header', 'observed_at_ms', 'observed_tip']); seedTip(t.observed_tip);
  requireValue(t.height === a.height + 1 && hex(t.hash) && seedHex(t.header, 80) && t.header.slice(8, 72) === a.hash.match(/../g).reverse().join('') && int(t.observed_at_ms, s.source_receipt.completed_at_ms, DATE_MAX) && t.observed_tip.height >= t.height && (t.observed_tip.height !== t.height || t.observed_tip.hash === t.hash));
  const f = s.finality; exact(f, ['checkpoint', 'observed_at_ms', 'observed_tip', 'target_descendants']); seedCheckpoint(f.checkpoint); seedTip(f.observed_tip);
  requireValue(f.checkpoint.height === t.height && f.checkpoint.hash === t.hash && int(f.observed_at_ms, t.observed_at_ms, DATE_MAX) && int(f.target_descendants, 60, 2147483647) && f.target_descendants === f.observed_tip.height - t.height);
  return s;
}
function cardStateValue(s) {
  exact(s, ['version', 'status', 'source_digest']); requireValue(s.version === 1 && optionalID(s.source_digest)); seedStatusValue(s.status);
  requireValue((s.source_digest !== '') === (s.status.finalized_at_ms > 0)); return s;
}
function cardPlanValue(p, source = null) {
  exact(p, ['version', 'card_id', 'input_id', 'seed', 'record_basis', 'counts', 'bout_count', 'gyms', 'entrants', 'bouts']);
  requireValue(p.version === 1 && ['card_id', 'input_id', 'seed'].every(k => hex(p[k])) && p.record_basis === CARD_BASIS && int(p.bout_count, 0, 5) && entryBytes(p) <= 16384);
  cardCountsValue(p.counts); requireValue(p.counts.paired_count === 2 * p.bout_count);
  requireValue(Array.isArray(p.gyms) && p.gyms.length === 2 && Array.isArray(p.entrants) && p.entrants.length === p.counts.total_count && p.entrants.length <= 10 && Array.isArray(p.bouts) && p.bouts.length === p.bout_count);
  const pins = cardAuthority.pin.source_pin.source_pin.participants;
  p.gyms.forEach((g, i) => { exact(g, ['gym_id', 'counts']); requireValue(g.gym_id === pins[i].stable_pin.identity.gym_id); cardCountsValue(g.counts, 5); });
  const byID = new Map(), byClassGym = new Map(); let previous = '';
  for (const e of p.entrants) {
    exact(e, ['prospect_id', 'gym_id', 'account_id', 'display_name', 'class', 'rating_hundredths', 'cutoff_eligibility', 'professional_record', 'state', 'reason', 'bout_id']);
    const pin = pins.find(v => v.stable_pin.identity.gym_id === e.gym_id), key = `${classes.indexOf(e.class)}|${e.gym_id}|${e.prospect_id}`;
    requireValue(!!pin && hex(e.prospect_id) && e.account_id === pin.stable_pin.account_id && classes.includes(e.class) && key > previous && !byID.has(e.prospect_id) && !byClassGym.has(`${e.class}|${e.gym_id}`)); previous = key;
    requireValue(typeof e.display_name === 'string' && /^[\x00-\x7f]{1,128}$/.test(e.display_name) && int(e.rating_hundredths, 0, 100000) && ['eligible', 'retired', 'not_rostered'].includes(e.cutoff_eligibility) && (e.cutoff_eligibility !== 'eligible' || e.rating_hundredths > 0));
    exact(e.professional_record, ['wins', 'losses', 'draws']); requireValue(['wins', 'losses', 'draws'].every(k => e.professional_record[k] === 0));
    requireValue(['paired', 'unmatched'].includes(e.state) && (e.state === 'paired' ? e.reason === '' && hex(e.bout_id) : CARD_REASONS.includes(e.reason) && e.bout_id === ''));
    byID.set(e.prospect_id, e); byClassGym.set(`${e.class}|${e.gym_id}`, e);
  }
  for (const e of p.entrants) {
    const opponent = p.entrants.find(v => v.class === e.class && v.gym_id !== e.gym_id);
    const reason = e.cutoff_eligibility === 'retired' ? 'retired_at_cutoff' : e.cutoff_eligibility === 'not_rostered' ? 'not_rostered_at_cutoff' : !opponent ? 'no_opponent_in_class' : opponent.cutoff_eligibility !== 'eligible' ? 'opponent_ineligible_at_cutoff' : 2 * Math.min(e.rating_hundredths, opponent.rating_hundredths) < Math.max(e.rating_hundredths, opponent.rating_hundredths) ? 'outside_rating_band' : '';
    requireValue(e.reason === reason && e.state === (reason ? 'unmatched' : 'paired'));
  }
  const used = new Set(), boutIDs = new Set(); let priorClass = -1;
  for (const b of p.bouts) {
    exact(b, ['bout_id', 'class', 'corner_a', 'corner_b', 'seed']); const a = byID.get(b.corner_a), c = byID.get(b.corner_b), i = classes.indexOf(b.class);
    requireValue(hex(b.bout_id) && hex(b.seed) && !boutIDs.has(b.bout_id) && i > priorClass && !!a && !!c && b.corner_a < b.corner_b && a.gym_id !== c.gym_id && a.class === b.class && c.class === b.class && a.state === 'paired' && c.state === 'paired' && a.bout_id === b.bout_id && c.bout_id === b.bout_id && !used.has(b.corner_a) && !used.has(b.corner_b));
    priorClass = i; used.add(b.corner_a); used.add(b.corner_b); boutIDs.add(b.bout_id);
  }
  requireValue(used.size === p.counts.paired_count && p.entrants.every(e => (e.state === 'paired') === used.has(e.prospect_id)));
  const tally = entries => ({ total_count: entries.length, eligible_count: entries.filter(e => e.cutoff_eligibility === 'eligible').length, paired_count: entries.filter(e => e.state === 'paired').length, unmatched_count: entries.filter(e => e.state === 'unmatched').length, reasons: Object.fromEntries(CARD_REASONS.map(k => [k, entries.filter(e => e.reason === k).length])) });
  requireValue(same(p.counts, tally(p.entrants))); p.gyms.forEach(g => requireValue(same(g.counts, tally(p.entrants.filter(e => e.gym_id === g.gym_id)))));
  if (source) {
    const all = source.source_receipt.input.participants.flatMap(v => v.members.map(m => ({ m, owner: v.owner })));
    requireValue(all.length === p.entrants.length);
    for (const e of p.entrants) { const row = all.find(v => v.m.assignment.prospect_id === e.prospect_id); requireValue(!!row && e.gym_id === row.owner.gym_id && e.account_id === row.owner.account_id && e.class === row.m.class && e.rating_hundredths === row.m.condition_at_cutoff.rating_hundredths && e.cutoff_eligibility === row.m.eligibility); }
  }
  return p;
}
function cardRecordValue(r) {
  exact(r, ['version', 'store_id', 'card_id', 'source', 'source_digest', 'plan', 'created_at_ms']);
  requireValue(r.version === 1 && r.store_id === cardAuthority.storeID && hex(r.card_id) && hex(r.source_digest) && entryBytes(r) <= 196608); cardSourceValue(r.source); cardPlanValue(r.plan, r.source);
  requireValue(r.plan.card_id === r.card_id && int(r.created_at_ms, Math.max(cardAuthority.createdMS, r.source.finality.observed_at_ms), DATE_MAX)); return r;
}
function cardStatusValue(s) {
  exact(s, ['version', 'store_id', 'card_id', 'event', 'exists', 'record_digest', 'created_at_ms', 'observed_at_ms', 'counts', 'bout_count', 'source', 'usable']); eventValue(s.event); cardStateValue(s.source); cardCountsValue(s.counts);
  requireValue(s.version === 1 && s.store_id === cardAuthority.storeID && hex(s.card_id) && typeof s.exists === 'boolean' && typeof s.usable === 'boolean' && same(s.event, s.source.status.event) && int(s.observed_at_ms, cardAuthority.createdMS, DATE_MAX) && s.source.status.observed_at_ms <= s.observed_at_ms && int(s.bout_count, 0, 5) && 2 * s.bout_count === s.counts.paired_count);
  if (s.exists) requireValue(hex(s.record_digest) && int(s.created_at_ms, Math.max(cardAuthority.createdMS, s.source.status.finalized_at_ms), s.observed_at_ms) && hex(s.source.source_digest));
  else requireValue(s.record_digest === '' && s.created_at_ms === 0 && s.counts.total_count === 0 && s.bout_count === 0 && !s.usable);
  // A standalone status cannot compare the saved record's source digest.
  requireValue(!s.usable || s.exists && s.source.status.finalized_at_ms > 0 && s.source.status.conflict_count === 0 && !s.source.status.conflict_overflow); return s;
}
function cardResultValue(v, build = false) {
  exact(v, build ? ['version', 'record', 'status', 'replayed'] : ['version', 'record', 'status']); requireValue(v.version === 1 && (!build || typeof v.replayed === 'boolean') && entryBytes(v) <= 229376);
  const r = cardRecordValue(v.record), s = cardStatusValue(v.status);
  requireValue(s.exists && s.card_id === r.card_id && s.created_at_ms === r.created_at_ms && same(s.event, r.source.source_receipt.input.event) && s.source.source_digest === r.source_digest && same(s.counts, r.plan.counts) && s.bout_count === r.plan.bout_count);
  const state = s.source.status, source = r.source;
  requireValue(state.event_key === source.event_key && state.anchor_height === source.anchor.height && state.anchor_hash === source.anchor.hash && state.target_height === source.target.height && state.target_hash === source.target.hash && state.finalized_at_ms === source.finality.observed_at_ms && s.usable === (state.conflict_count === 0 && !state.conflict_overflow)); return v;
}
function cardJobValue(j) {
  careerJobFields(j, ['version', 'job_id', 'kind', 'state', 'view', 'store_id', 'card_id', 'saturday', 'error', 'result']);
  requireValue(j.version === 1 && hex(j.job_id) && j.kind === 'build' && ['queued', 'running', 'completed', 'failed', 'cancelled'].includes(j.state) && viewKey(checkedView(j.view)) === currentKey() && j.store_id === cardAuthority.storeID && hex(j.card_id) && entryBytes(j) <= CARD_RESULT_MAX); entryDateValue(j.saturday);
  if (j.state === 'completed') { requireValue(j.error === '' && j.result !== null); cardResultValue(j.result, true); requireValue(j.result.record.card_id === j.card_id && j.result.status.event.saturday === j.saturday); }
  else requireValue(j.result === null && (['queued', 'running'].includes(j.state) ? j.error === '' : j.state === 'cancelled' ? j.error === 'card_cancelled' : CARD_ERRORS.has(j.error)));
  return j;
}
function cardEnvelope(r, field) {
  exact(r, ['id', 'ok', 'view', 'store_id', 'snapshot', 'status', 'record', 'history', 'job', 'error']); requireValue(int(r.id, 1) && typeof r.ok === 'boolean' && entryBytes(r) + 1 <= 262144);
  const fields = ['snapshot', 'status', 'record', 'history', 'job'];
  if (!r.ok) { requireValue(CARD_ERRORS.has(r.error) && r.view === null && r.store_id === '' && fields.every(k => r[k] === null)); return { ok: false, error: r.error }; }
  requireValue(r.error === '' && fields.every(k => (r[k] !== null) === (k === field))); const v = r[field];
  if (field === 'snapshot') {
    exact(v, ['version', 'state', 'error', 'view', 'store_id', 'pin', 'source_created_at_ms', 'created_at_ms', 'active_job_id', 'queued_job_id', 'last_job_id']);
    requireValue(v.version === 1 && ['ready', 'busy', 'setup_incomplete', 'unavailable', 'failed'].includes(v.state));
    if (v.state === 'unavailable') { requireValue(r.view === null && v.view === null && v.pin === null && r.store_id === '' && v.store_id === '' && v.source_created_at_ms === 0 && v.created_at_ms === 0 && v.error === 'card_unavailable' && ['active_job_id', 'queued_job_id', 'last_job_id'].every(k => v[k] === '')); return { ok: true, value: v }; }
    requireValue(viewKey(checkedView(v.view)) === currentKey() && viewKey(checkedView(r.view)) === currentKey() && r.store_id === v.store_id);
    if (v.state === 'setup_incomplete') { requireValue(!cardAuthority && v.pin === null && v.store_id === '' && v.source_created_at_ms === 0 && v.created_at_ms === 0 && v.error === 'card_setup_incomplete' && ['active_job_id', 'queued_job_id', 'last_job_id'].every(k => v[k] === '')); return { ok: true, value: v }; }
    cardPinValue(v.pin); requireValue(hex(v.store_id) && v.source_created_at_ms === seedAuthority.createdMS && int(v.created_at_ms, v.source_created_at_ms, DATE_MAX) && ['active_job_id', 'queued_job_id', 'last_job_id'].every(k => optionalID(v[k])) && !(v.active_job_id && v.queued_job_id) && (v.state === 'failed' ? ['card_unavailable', 'card_corrupt', 'card_clock', 'card_source_changed'].includes(v.error) : v.error === ''));
    const ids = [v.active_job_id, v.queued_job_id, v.last_job_id].filter(Boolean); requireValue(new Set(ids).size === ids.length);
    requireValue((!v.active_job_id && !v.queued_job_id) || v.state === 'busy');
    const authority = { pin: v.pin, storeID: v.store_id, sourceCreatedMS: v.source_created_at_ms, createdMS: v.created_at_ms };
    requireValue(!cardAuthority || same(cardAuthority, authority)); if (!cardAuthority) cardAuthority = authority; v.pin = cardAuthority.pin; return { ok: true, value: v };
  }
  requireValue(cardReady() && r.store_id === cardAuthority.storeID && viewKey(checkedView(r.view)) === currentKey());
  if (field === 'status') cardStatusValue(v);
  if (field === 'record') cardResultValue(v);
  if (field === 'job') cardJobValue(v);
  if (field === 'history') {
    exact(v, ['version', 'store_id', 'after_card_id', 'next_card_id', 'has_more', 'observed_at_ms', 'cards']);
    requireValue(v.version === 1 && v.store_id === cardAuthority.storeID && optionalID(v.after_card_id) && optionalID(v.next_card_id) && typeof v.has_more === 'boolean' && int(v.observed_at_ms, cardAuthority.createdMS, DATE_MAX) && Array.isArray(v.cards) && v.cards.length <= 8 && entryBytes(v) <= 139264);
    v.cards.forEach((s, i) => { cardStatusValue(s); requireValue(s.exists && s.observed_at_ms === v.observed_at_ms && (!i || v.cards[i - 1].event.cutoff_ms < s.event.cutoff_ms)); });
    requireValue(v.next_card_id === (v.has_more && v.cards.length ? v.cards.at(-1).card_id : ''));
  }
  return { ok: true, value: v };
}
function cardError(code, build = false) {
  if (code === 'outcome_unknown') return build ? 'The card may have been saved. Open the saved card or check its status before another explicit request. No transaction was sent.' : 'The local status observation could not be confirmed. The saved card is unchanged; refresh after reopening the gym. No booking or transaction was requested.';
  return ({ card_unavailable: 'Candidate cards are unavailable in this route. Saved evidence is retained.', card_setup_incomplete: 'Sign in to both demo gyms to finish the shared setup.', card_source_incomplete: 'The complete noon snapshot and its fixed finalized seed are required before a card can be created.', card_source_conflict: 'The fixed seed has conflicting evidence. A new card cannot be created; existing card evidence remains available.', card_source_changed: 'The retained source identity changed. The displayed card is historical; its current usability is unknown.', card_not_found: 'No saved card was found for this Saturday.', card_capacity: 'The card store or display is full. Clear a detail view to free display space; saved cards remain retained.', card_clock: 'The local authority clock needs attention. No new card was confirmed.', card_corrupt: 'Saved card evidence failed validation. Reopen the gym; no history was discarded.', card_cancelled: 'The request was cancelled. Check the saved card before another request.', card_calendar: 'Choose a supported Saturday in the entry panel.' })[code] ?? textError(code);
}
function cardRefusal(code, build = false) {
  if (['unauthorized', 'stale_view'].includes(code)) { clearCard(); cardUnavailable = true; }
  if (code === 'card_unavailable') cardUnavailable = true;
  if (['card_unavailable', 'card_source_changed', 'card_corrupt', 'outcome_unknown'].includes(code)) { cardOverlayKnown = false; cardOverlayRead = true; }
  if (build && code === 'outcome_unknown') cardUnknown = true;
  cardMessage = cardError(code, build);
}
function cardAdoptStatus(s) {
  if (s.event.saturday !== entryDate) return;
  if (cardStatus && cardStatus.card_id === s.card_id) {
    requireValue(s.observed_at_ms >= cardStatus.observed_at_ms && (!cardStatus.exists || s.exists && s.record_digest === cardStatus.record_digest && s.created_at_ms === cardStatus.created_at_ms && same(s.counts, cardStatus.counts)));
  }
  if (cardDetail) requireValue(cardDetail.card_id === s.card_id && s.exists && cardDetail.source_digest === s.source.source_digest && same(cardDetail.plan.counts, s.counts));
  const old = [cardStatus, cardOverlayKnown, cardOverlayRead]; cardStatus = s; cardOverlayKnown = cardOverlayRead = true;
  if (!cardCapacity()) { [cardStatus, cardOverlayKnown, cardOverlayRead] = old; throw cardCacheError(); }
}
function cardAdoptResult(result, terminal = false) {
  if (result.status.event.saturday !== entryDate) return;
  const r = result.record, s = result.status;
  if (cardDetail) requireValue(cardDetail.card_id === r.card_id && same(cardDetail, r));
  const newer = cardStatus?.card_id === s.card_id && cardStatus.observed_at_ms > s.observed_at_ms;
  if (cardStatus?.exists) requireValue(cardStatus.card_id === s.card_id && cardStatus.record_digest === s.record_digest && cardStatus.source.source_digest === r.source_digest && same(cardStatus.counts, r.plan.counts));
  requireValue(!newer || terminal);
  const old = [cardDetail, cardStatus, cardOverlayKnown, cardUnknown, cardOverlayRead]; cardDetail = cardDetail ?? r;
  // A fresh explicit overlay, including a later observation failure, outranks
  // the immutable status saved inside a completed job.
  if (!(terminal && (newer || cardOverlayRead))) { cardStatus = s; cardOverlayKnown = true; }
  if (!terminal) cardOverlayRead = true; cardUnknown = false;
  if (!cardCapacity()) { [cardDetail, cardStatus, cardOverlayKnown, cardUnknown, cardOverlayRead] = old; throw cardCacheError(); }
}
async function fetchCardSnapshot() {
  if (!ready() || !seedAuthority || cardUnavailable) return; const c = cardContext(), prior = [cardSnapshot, cardAuthority];
  try {
    const raw = await call('CardSnapshot'); if (!cardCurrent(c)) return;
    const r = cardEnvelope(raw, 'snapshot'); if (!r.ok) { cardRefusal(r.error); return; }
    cardSnapshot = r.value; if (!cardCapacity()) { [cardSnapshot, cardAuthority] = prior; cardMessage = cardError('card_capacity'); return; }
    if (cardSnapshot.state === 'unavailable') { cardRefusal('card_unavailable'); return; }
    if (cardSnapshot.state === 'failed') { cardRefusal(cardSnapshot.error); return; }
    const id = cardWatch?.id || cardSnapshot.active_job_id || cardSnapshot.queued_job_id || cardSnapshot.last_job_id;
    if (id && (cardWatch || id !== cardLastJob)) await cardFetchJob(id);
  } catch { if (cardCurrent(c)) { [cardSnapshot, cardAuthority] = prior; cardRefusal('card_unavailable'); } }
}
async function cardRead(kind, after = '') {
  if (!cardReady() || !['status', 'record', 'history'].includes(kind) || kind !== 'history' && !entryDate) return;
  const c = cardContext();
  try {
    const raw = kind === 'history' ? await call('CardHistory', c.viewID, after, 8) : await call(kind === 'record' ? 'CardRecord' : 'CardStatus', c.viewID, c.date);
    if (!cardCurrent(c, kind !== 'history')) return; const r = cardEnvelope(raw, kind); if (!r.ok) { cardRefusal(r.error); return; }
    if (kind === 'history') { requireValue(r.value.after_card_id === after); const old = cardHistory; cardHistory = r.value; if (!cardCapacity()) { cardHistory = old; cardMessage = cardError('card_capacity'); return; } }
    else { const s = kind === 'record' ? r.value.status : r.value; requireValue(s.event.saturday === c.date); if (kind === 'record') cardAdoptResult(r.value); else { cardAdoptStatus(s); if (s.exists) cardUnknown = false; } }
    cardMessage = kind === 'record' ? 'The original saved card and its latest local source status are shown below.' : kind === 'history' ? 'Saved card history loaded.' : 'Local card status checked. The seed chain was not refreshed.';
  } catch (e) { if (cardCurrent(c, kind !== 'history')) cardRefusal(e.cardCode ?? 'card_unavailable'); } renderCard();
}
async function cardBegin() {
  if (!cardReady() || !entryDate || cardWatch || cardUnknown || cardSnapshot.state === 'busy') return;
  const c = cardContext(); cardWatch = { id: '', kind: 'build', state: 'queued', saturday: c.date, cardID: '' };
  if (!cardCapacity()) { cardWatch = null; cardMessage = cardError('card_capacity'); return; }
  try {
    const raw = await call('BeginCardBuild', c.viewID, c.date); if (!cardCurrent(c)) return; const r = cardEnvelope(raw, 'job');
    if (!r.ok) { cardWatch = null; cardRefusal(r.error, true); return; } const j = r.value;
    requireValue(j.state === 'queued' && j.saturday === c.date);
    if (cardCurrent(c, true)) { cardDetail = cardStatus = null; cardOverlayKnown = cardOverlayRead = false; }
    cardTerminal = null;
    cardWatch = { id: j.job_id, kind: j.kind, state: j.state, saturday: j.saturday, cardID: j.card_id }; cardMessage = 'Candidate card request queued.';
  } catch { if (cardCurrent(c)) { cardWatch = null; cardRefusal('outcome_unknown', true); } } renderCard();
}
function cardTerminalKey(j) {
  // The bridge checks the canonical full-job digest. Here its verified record
  // digest plus complete status retain compact presentation correlation.
  return JSON.stringify([j.job_id, j.kind, j.state, viewKey(j.view), j.store_id, j.card_id, j.saturday, j.error, j.ineligible_prospect_ids ?? [], j.result?.status ?? null, j.result?.replayed ?? null]);
}
async function cardFetchJob(id) {
  if (!cardReady()) return; const c = cardContext(), expected = cardWatch;
  let rollback = null;
  try {
    const raw = await call('CardJob', c.viewID, id); if (!cardCurrent(c) || cardWatch !== expected) return; const r = cardEnvelope(raw, 'job');
    if (!r.ok) { if (r.error !== 'busy') { cardWatch = null; cardRefusal(r.error, true); } return; } const j = r.value;
    requireValue(j.job_id === id && (!expected || j.kind === expected.kind && j.saturday === expected.saturday && (!expected.cardID || j.card_id === expected.cardID) && !(expected.state === 'running' && j.state === 'queued')));
    if (['queued', 'running'].includes(j.state)) { const old = cardWatch; cardWatch = { id, kind: j.kind, state: j.state, saturday: j.saturday, cardID: j.card_id }; if (!cardCapacity()) { cardWatch = old; throw cardCacheError(); } return; }
    const key = cardTerminalKey(j); if (cardTerminal?.id === id) requireValue(cardTerminal.key === key);
    rollback = [cardWatch, cardLastJob, cardTerminal, cardDetail, cardStatus, cardOverlayKnown, cardUnknown, cardOverlayRead, cardMessage, cardUnavailable];
    cardWatch = null; cardLastJob = id; cardTerminal = { id, key };
    if (j.state === 'completed') { cardAdoptResult(j.result, true); cardMessage = j.result.replayed ? 'The same saved card was recovered. Its original pairings and creation time are unchanged.' : 'The candidate card is saved. Every locked entrant is accounted for below.'; }
    else { cardRefusal(j.error, true); cardMessage += careerAffected(j); }
    if (!cardCapacity()) throw cardCacheError();
  } catch (e) { if (cardCurrent(c)) {
    if (rollback) [cardWatch, cardLastJob, cardTerminal, cardDetail, cardStatus, cardOverlayKnown, cardUnknown, cardOverlayRead, cardMessage, cardUnavailable] = rollback;
    if (e.cardCode === 'card_capacity') cardMessage = cardError('card_capacity');
    else { cardWatch = null; cardRefusal('outcome_unknown', true); }
  } } renderCard();
}
async function cardCancel() {
  if (!cardReady() || !cardWatch?.id) return; const c = cardContext(), w = cardWatch;
  try { const raw = await call('CancelCardJob', c.viewID, w.id); if (!cardCurrent(c)) return; const r = cardEnvelope(raw, 'job'); if (!r.ok) { cardRefusal(r.error, true); return; } requireValue(r.value.job_id === w.id && r.value.kind === w.kind && r.value.saturday === w.saturday && r.value.card_id === w.cardID); await cardFetchJob(w.id); }
  catch { if (cardCurrent(c)) cardRefusal('outcome_unknown', true); } renderCard();
}
function cardGymName(id) { return id === fixtures.zero256[0] ? 'Demo A' : id === fixtures.incrementing256[0] ? 'Demo B' : 'Unknown gym'; }
function renderCard() {
  const active = cardReady(), s = cardStatus?.event.saturday === entryDate ? cardStatus : null;
  const r = cardDetail?.source.source_receipt.input.event.saturday === entryDate ? cardDetail : null, plan = r?.plan;
  const busy = !!command || !!cardWatch || cardSnapshot?.state === 'busy';
  const conflict = !!s && (s.source.status.conflict_count > 0 || s.source.status.conflict_overflow);
  content(ui['card-state'], !ready() ? 'Sign in to view cards' : cardUnavailable ? 'Unavailable in this route' : cardSnapshot?.state === 'setup_incomplete' ? 'Setup incomplete' : cardWatch ? 'Card request in progress' : s?.exists ? !cardOverlayKnown ? 'Saved card · source status unavailable' : conflict ? 'Saved card · seed conflict' : !s.usable ? 'Saved card · source unavailable' : 'Candidate card saved' : 'No saved card');
  content(ui['card-message'], cardMessage);
  content(ui['card-totals'], s?.exists ? `${s.counts.total_count} entered · ${s.counts.eligible_count} eligible at noon · ${s.counts.paired_count} paired · ${s.counts.unmatched_count} unmatched · ${s.bout_count} bouts` : 'Create a card after both noon snapshots and the fixed seed are ready.');
  const source = s?.source.status, chainTime = source?.chain_observed_at_ms ?? 0;
  content(ui['card-source'], source ? `Saturday ${source.event.saturday} · Noon cutoff ${new Date(source.event.cutoff_ms).toISOString()}. ${source.finalized_at_ms ? `First seed finality ${new Date(source.finalized_at_ms).toISOString()}.` : 'Waiting for the fixed finalized seed.'} ${chainTime ? `Last verified chain observation ${new Date(chainTime).toISOString()}.` : 'No verified chain observation yet.'}` : 'The card uses the locked selection and condition for the Saturday chosen above.');
  content(ui['card-conflict'], !cardOverlayKnown && (r || s?.exists) ? 'This is retained historical booking evidence. Current source usability has not been confirmed.' : conflict ? 'The fixed seed has conflicting evidence. The original card is retained and is not currently usable.' : s?.exists ? s.usable ? 'Retained candidate source is usable at the last local status check. This is not continuous chain monitoring.' : 'The saved card is retained, but its source is not currently usable.' : '');
  content(ui['card-incident'], source?.incident_at_ms ? `Chain incident first recorded ${new Date(source.incident_at_ms).toISOString()}; refer the retained seed evidence to the operator. The event has not been cancelled or reseeded.` : '');
  content(ui['card-evidence'], r ? `Card ${r.card_id} · Created ${new Date(r.created_at_ms).toISOString()} · Fixed seed ${r.source.target.hash} · Source ${r.source_digest}` : '');
  const retainedKey = r ? `${r.card_id}|${s?.record_digest ?? ''}` : '';
  if (ui['card-bouts']._key !== retainedKey) {
    for (const id of ['card-gyms', 'card-bouts', 'card-unmatched']) ui[id].textContent = '';
    if (plan) {
      for (const g of plan.gyms) { const row = element('tr'); for (const value of [cardGymName(g.gym_id), g.counts.total_count, g.counts.paired_count, g.counts.unmatched_count]) row.append(element('td', '', String(value))); ui['card-gyms'].append(row); }
      for (const b of plan.bouts) {
        const a = plan.entrants.find(e => e.prospect_id === b.corner_a), c = plan.entrants.find(e => e.prospect_id === b.corner_b), li = element('li'), detail = element('details');
        detail.append(element('summary', '', `${label(b.class)}: ${a.display_name} (${cardGymName(a.gym_id)}, ${(a.rating_hundredths / 100).toFixed(2)}) vs ${c.display_name} (${cardGymName(c.gym_id)}, ${(c.rating_hundredths / 100).toFixed(2)})`), element('p', 'note card-identities', `${a.display_name}: ${a.prospect_id} · ${c.display_name}: ${c.prospect_id}`)); li.append(detail); ui['card-bouts'].append(li);
      }
      for (const e of plan.entrants.filter(e => e.state === 'unmatched')) {
        const li = element('li'), detail = element('details'); detail.append(element('summary', '', `${e.display_name} · ${cardGymName(e.gym_id)} · ${label(e.class)} · ${(e.rating_hundredths / 100).toFixed(2)}: ${cardReasonText[e.reason]}`), element('p', 'note card-identities', e.prospect_id)); li.append(detail); ui['card-unmatched'].append(li);
      }
    }
    ui['card-bouts']._key = retainedKey;
  }
  const reasonKey = s?.exists ? JSON.stringify(s.counts.reasons) : '';
  if (ui['card-reasons']._key !== reasonKey) { ui['card-reasons'].textContent = ''; if (s?.exists) for (const k of CARD_REASONS) if (s.counts.reasons[k]) ui['card-reasons'].append(element('li', '', `${s.counts.reasons[k]}: ${cardReasonText[k]}`)); ui['card-reasons']._key = reasonKey; }
  content(ui['card-detail-hint'], plan ? plan.counts.total_count === 0 ? 'This complete saved card has no entered fighters.' : plan.counts.unmatched_count === 0 ? 'Every entered fighter is paired.' : 'Every unmatched entrant is listed below.' : s?.exists ? 'Open the saved card to inspect fighter names, pairings and individual reasons.' : '');
  for (const k of ['refresh', 'record']) ui[`card-${k}`].disabled = !active || busy || !entryDate;
  const knownIncomplete = s && !s.exists && (!s.source.status.finalized_at_ms || conflict);
  ui['card-begin'].disabled = !active || busy || !entryDate || cardUnknown || !!knownIncomplete;
  ui['card-cancel'].disabled = !active || !!command || !cardWatch?.id;
  ui['card-clear'].disabled = !cardDetail && !cardHistory; ui['card-history'].disabled = !active || busy; ui['card-history-next'].disabled = !active || busy || !cardHistory?.has_more;
  const historyKey = cardHistory ? JSON.stringify(cardHistory.cards.map(v => [v.card_id, v.event.saturday, v.counts.total_count, v.counts.paired_count, v.counts.unmatched_count, v.usable, v.source.status.incident_at_ms, v.source.status.conflict_count, v.source.status.conflict_overflow])) : '';
  requireValue(entryBytes([retainedKey, reasonKey, historyKey, ui['card-evidence'].textContent]) <= 4096);
  if (ui['card-history-list']._key !== historyKey) { ui['card-history-list'].textContent = ''; if (cardHistory) for (const v of cardHistory.cards) { const li = element('li'), button = element('button', '', v.event.saturday); button.type = 'button'; button.addEventListener('click', () => entryNavigate(v.event.saturday)); li.append(button, element('span', '', `${v.counts.total_count} entered, ${v.counts.paired_count} paired, ${v.counts.unmatched_count} unmatched${v.usable ? '' : ' · source conflict or unavailable'}`)); ui['card-history-list'].append(li); } ui['card-history-list']._key = historyKey; }
}

// Saved candidate bouts are a separate family. Native owns canonical hashes and
// authorization; this presentation validator never derives a manifest or fight.
const BOOKED_RULES = '5f1e8b5ee9e7caaa0cc796dc8d486b453a3c7c7d5dc1ee399667d7f6051e7247';
const BOOKED_MAX = 393216, BOOKED_RESULT_MAX = 253952, BOOKED_METADATA_MAX = 73728;
const BOOKED_ERRORS = new Set(['invalid_request', 'unauthorized', 'forbidden', 'stale_view', 'busy', 'outcome_unknown', 'bout_unavailable', 'bout_setup_incomplete', 'bout_corrupt', 'bout_capacity', 'bout_clock', 'bout_calendar', 'bout_not_found', 'bout_source_incomplete', 'bout_source_conflict', 'bout_source_changed', 'bout_cancelled']);
let bookedAuthority = null, bookedSnapshot = null, bookedPage = null, bookedStatus = null, bookedRecord = null;
let bookedSelected = '', bookedSerial = 0, bookedWatch = null, bookedTerminal = null, bookedLastJob = '', bookedMessage = '';
let bookedUnavailable = false, bookedUnknown = null, bookedOverlayKnown = false, bookedOverlayRead = false;
let bookedTimer = null, bookedStep = 0, bookedRendered = '', bookedImages = new Map(), bookedImageAttempts = new Set();
function bookedMetadataBytes() {
  return entryBytes([bookedAuthority, bookedSnapshot && { ...bookedSnapshot, pin: null }, bookedPage, bookedStatus, bookedSelected, bookedWatch, bookedTerminal, bookedLastJob, bookedMessage, bookedUnknown, [...bookedImageAttempts]]) + 4096;
}
function bookedRetainedBytes() {
  if (!bookedAuthority && !bookedSnapshot && !bookedWatch && !bookedRecord) return 0;
  if (bookedWatch) return BOOKED_MAX;
  return bookedMetadataBytes() + entryBytes(bookedRecord) + [...bookedImages.values()].reduce((n, v) => n + 2 * v.length, 0);
}
function bookedCapacity() { return bookedMetadataBytes() <= BOOKED_METADATA_MAX && entryBytes(bookedRecord) <= BOOKED_RESULT_MAX && [...bookedImages.values()].every(v => 2 * v.length <= 32768) && bookedRetainedBytes() <= BOOKED_MAX && previewCapacity(); }
function bookedCacheError() { const e = new Error('Bout display full'); e.boutCode = 'bout_capacity'; return e; }
function bookedReady() { return ready() && !bookedUnavailable && !!bookedAuthority && !!bookedSnapshot?.pin && viewKey(bookedSnapshot.view) === currentKey(); }
function pauseBooked() { if (bookedTimer !== null) clearTimeout(bookedTimer); bookedTimer = null; }
function bookedClearImages() { bookedImages.clear(); bookedImageAttempts.clear(); bookedRendered = ''; ui['bout-fighters'].textContent = ''; }
function bookedClearRecord() { pauseBooked(); bookedRecord = null; bookedStep = 0; bookedClearImages(); ui['bout-events'].textContent = ''; ui['bout-scores'].textContent = ''; }
function clearBooked() {
  clearPublication();
  bookedSerial++; bookedClearRecord(); bookedAuthority = bookedSnapshot = bookedPage = bookedStatus = bookedWatch = bookedTerminal = bookedUnknown = null;
  bookedSelected = bookedLastJob = bookedMessage = ''; bookedUnavailable = bookedOverlayKnown = bookedOverlayRead = false;
  ui['bout-list'].textContent = ''; ui['bout-list']._key = '';
}
function bookedSelectDate() { publicationSelect(); bookedSerial++; bookedClearRecord(); bookedPage = bookedStatus = null; bookedSelected = ''; bookedMessage = ''; bookedOverlayKnown = bookedOverlayRead = false; }
function bookedContext() { return { epoch, view: currentKey(), viewID: current?.view?.view_id, serial: bookedSerial, date: entryDate, boutID: bookedSelected }; }
function bookedCurrent(c, selection = false) { return !disposed && ready() && c.epoch === epoch && c.view === currentKey() && (!selection || c.serial === bookedSerial && c.date === entryDate && c.boutID === bookedSelected); }
function bookedBoutValue(b) { exact(b, ['bout_id', 'class', 'corner_a', 'corner_b', 'seed']); requireValue(['bout_id', 'corner_a', 'corner_b', 'seed'].every(k => hex(b[k])) && b.corner_a < b.corner_b && classes.includes(b.class)); }
function bookedSourceState(s) {
  exact(s, ['version', 'card_id', 'event', 'card_exists', 'card_record_digest', 'card_source_digest', 'card_created_at_ms', 'finalized_at_ms', 'conflict_count', 'conflict_overflow', 'incident_at_ms', 'escalation_due', 'chain_observed_at_ms', 'target_descendants', 'current_finality', 'node_state', 'observed_at_ms', 'usable']); eventValue(s.event);
  requireValue(s.version === 1 && hex(s.card_id) && ['card_exists', 'conflict_overflow', 'escalation_due', 'current_finality', 'usable'].every(k => typeof s[k] === 'boolean') && int(s.observed_at_ms, cardAuthority.createdMS, DATE_MAX) && int(s.conflict_count, 0, 8) && (!s.conflict_overflow || s.conflict_count === 8) && int(s.target_descendants, 0, 2147483647) && ['unobserved', 'online', 'offline', 'unavailable'].includes(s.node_state));
  for (const k of ['finalized_at_ms', 'incident_at_ms', 'chain_observed_at_ms']) requireValue(int(s[k], 0, s.observed_at_ms));
  if (s.card_exists) requireValue(hex(s.card_record_digest) && hex(s.card_source_digest) && int(s.card_created_at_ms, Math.max(cardAuthority.createdMS, s.finalized_at_ms), s.observed_at_ms));
  else requireValue(s.card_record_digest === '' && s.card_source_digest === '' && s.card_created_at_ms === 0 && !s.usable);
  requireValue(!s.usable || s.card_exists && s.finalized_at_ms > 0 && !s.conflict_count && !s.conflict_overflow);
  requireValue(!s.incident_at_ms || s.incident_at_ms >= s.event.incident_deadline_ms);
  requireValue(s.escalation_due === (s.observed_at_ms >= s.event.incident_deadline_ms && (!s.finalized_at_ms || s.conflict_count > 0 || s.conflict_overflow)));
  requireValue(s.chain_observed_at_ms > 0 || s.target_descendants === 0 && !s.current_finality);
  requireValue(!s.current_finality || s.finalized_at_ms > 0 && s.chain_observed_at_ms > 0 && s.target_descendants >= 60 && !s.conflict_count && !s.conflict_overflow);
  return s;
}
function bookedSavedValue(s, source, observed) {
  exact(s, ['bout', 'source_digest', 'exists', 'record_digest', 'created_at_ms', 'input_id', 'result_digest', 'winner', 'method', 'end_round', 'end_tick', 'usable']); bookedBoutValue(s.bout);
  requireValue(hex(s.source_digest) && typeof s.exists === 'boolean' && typeof s.usable === 'boolean' && entryBytes(s) <= 2048);
  if (!s.exists) requireValue(['record_digest', 'input_id', 'result_digest', 'winner', 'method'].every(k => s[k] === '') && ['created_at_ms', 'end_round', 'end_tick'].every(k => s[k] === 0) && !s.usable);
  else requireValue(['record_digest', 'input_id', 'result_digest'].every(k => hex(s[k])) && int(s.created_at_ms, Math.max(bookedAuthority.createdMS, source.card_created_at_ms, source.finalized_at_ms), observed) && FINISHES.includes(s.method) && int(s.end_round, 1, 3) && int(s.end_tick, 1, 60) && (['decision_draw', 'double_corner_stoppage'].includes(s.method) ? s.winner === '' : [s.bout.corner_a, s.bout.corner_b].includes(s.winner)));
  requireValue(!s.usable || s.exists && source.usable); return s;
}
function bookedStatusValue(s, page = false) {
  exact(s, ['version', 'store_id', 'card_id', 'observed_at_ms', 'source', page ? 'bouts' : 'saved']); bookedSourceState(s.source);
  requireValue(s.version === 1 && s.store_id === bookedAuthority.storeID && s.card_id === s.source.card_id && int(s.observed_at_ms, bookedAuthority.createdMS, DATE_MAX) && s.observed_at_ms >= s.source.observed_at_ms && entryBytes(s) <= (page ? 16384 : 8192));
  if (!page) { requireValue(s.source.card_exists); bookedSavedValue(s.saved, s.source, s.observed_at_ms); }
  else {
    requireValue(Array.isArray(s.bouts) && s.bouts.length <= 5 && (s.source.card_exists || !s.bouts.length));
    const ids = new Set(), corners = new Set(); let prior = -1;
    for (const v of s.bouts) { bookedSavedValue(v, s.source, s.observed_at_ms); const b = v.bout, ci = classes.indexOf(b.class); requireValue(ci > prior && !ids.has(b.bout_id) && !corners.has(b.corner_a) && !corners.has(b.corner_b)); prior = ci; ids.add(b.bout_id); corners.add(b.corner_a); corners.add(b.corner_b); }
  } return s;
}
function bookedSourceValue(s) {
  exact(s, ['version', 'store_id', 'card_id', 'card_record_digest', 'card_source_digest', 'card_created_at_ms', 'event', 'snapshot_digest', 'seed_event_key', 'target_height', 'target_hash', 'finalized_at_ms', 'bout', 'corners']); eventValue(s.event); bookedBoutValue(s.bout);
  requireValue(s.version === 1 && s.store_id === cardAuthority.storeID && ['card_id', 'card_record_digest', 'card_source_digest', 'snapshot_digest', 'seed_event_key', 'target_hash'].every(k => hex(s[k])) && int(s.target_height, 1, 2147483647) && int(s.finalized_at_ms, s.event.cutoff_ms, DATE_MAX) && int(s.card_created_at_ms, Math.max(cardAuthority.createdMS, s.finalized_at_ms), DATE_MAX) && entryBytes(s) <= 16384);
  const owners = new Set(); pairArray(s.corners, (v, n) => {
    exact(v, ['owner', 'member']); const o = v.owner, m = v.member; exact(o, ['ownership_basis', 'account_authority_id', 'account_id', 'gym_id']);
    const p = eventAuthority.pin.participants.find(p => p.stable_pin.identity.gym_id === o.gym_id); requireValue(!!p && !owners.has(o.gym_id) && o.ownership_basis === 'candidate_assignment' && o.account_authority_id === p.stable_pin.account_authority_id && o.account_id === p.stable_pin.account_id); owners.add(o.gym_id);
    exact(m, ['slot', 'slot_assigned_at_ms', 'assignment', 'class', 'condition_at_cutoff', 'eligibility']); assignment(m.assignment, p.stable_pin.identity); eventCondition(m.condition_at_cutoff, m.assignment, s.event.cutoff_ms);
    requireValue(m.assignment.prospect_id === [s.bout.corner_a, s.bout.corner_b][n] && m.class === s.bout.class && m.eligibility === 'eligible' && int(m.slot, 1, 10) && int(m.slot_assigned_at_ms, m.assignment.accepted_at_ms, s.event.cutoff_ms - 1));
    const c = m.condition_at_cutoff; requireValue(['prime', 'decline'].includes(c.phase) && int(c.age_weeks, 0, 90) && int(c.elapsed_decline_weeks, 0, 38) && attrs.every(k => int(c.aged_attributes_tenths[k], 1, 1000)) && (c.phase === 'prime' ? c.elapsed_decline_weeks === 0 && c.prime_remaining_weeks > 0 : c.prime_remaining_weeks === 0 && c.prime_remaining_ms === 0));
  });
  const ratings = s.corners.map(v => v.member.condition_at_cutoff.rating_hundredths); requireValue(2 * Math.min(...ratings) >= Math.max(...ratings)); return s;
}
function bookedRecordValue(r) {
  exact(r, ['version', 'store_id', 'card_id', 'bout_id', 'source', 'source_digest', 'input', 'input_id', 'result', 'result_digest', 'created_at_ms']); bookedSourceValue(r.source);
  requireValue(r.version === 1 && r.store_id === bookedAuthority.storeID && r.card_id === r.source.card_id && r.bout_id === r.source.bout.bout_id && ['source_digest', 'input_id', 'result_digest'].every(k => hex(r[k])) && int(r.created_at_ms, Math.max(bookedAuthority.createdMS, r.source.card_created_at_ms), DATE_MAX) && entryBytes(r) <= 237568);
  const i = r.input; exact(i, ['version', 'engine_id', 'mode', 'seed_kind', 'as_of_ms', 'seed', 'a', 'b']);
  requireValue(i.version === 1 && i.engine_id === PREVIEW_ENGINE && i.mode === 'standard' && i.seed_kind === 'explicit' && i.seed === r.source.bout.seed && i.as_of_ms === r.source.event.cutoff_ms && entryBytes(i) <= 16384);
  for (const [n, f] of [i.a, i.b].entries()) { exact(f, ['prospect_id', 'born_at_ms', 'manifest', 'pro_xp', 'training_skill']); const m = r.source.corners[n].member; requireValue(f.prospect_id === m.assignment.prospect_id && f.born_at_ms === m.assignment.born_at_ms && f.pro_xp === 0 && f.training_skill === 0); manifestValue(f.manifest, m.assignment.work_hash, true); requireValue(f.manifest.physical.weight_class === m.class); }
  boutValue(r.result, [i.a.prospect_id, i.b.prospect_id]); requireValue(r.result.input_id === r.input_id); return r;
}
function bookedMatchRecord(r, s) {
  const v = s.saved, source = s.source;
  requireValue(v.exists && s.card_id === r.card_id && v.bout.bout_id === r.bout_id && same(v.bout, r.source.bout) && v.source_digest === r.source_digest && v.created_at_ms === r.created_at_ms && v.input_id === r.input_id && v.result_digest === r.result_digest && v.winner === r.result.winner && v.method === r.result.method && v.end_round === r.result.end_round && v.end_tick === r.result.end_tick);
  requireValue(same(source.event, r.source.event) && source.card_record_digest === r.source.card_record_digest && source.card_source_digest === r.source.card_source_digest && source.card_created_at_ms === r.source.card_created_at_ms && source.finalized_at_ms === r.source.finalized_at_ms && v.usable === source.usable);
}
function bookedResultValue(v, run = false) { exact(v, run ? ['version', 'record', 'status', 'replayed'] : ['version', 'record', 'status']); requireValue(v.version === 1 && (!run || typeof v.replayed === 'boolean') && entryBytes(v) <= 249856); bookedRecordValue(v.record); bookedStatusValue(v.status); bookedMatchRecord(v.record, v.status); return v; }
function bookedJobValue(j) {
  exact(j, ['version', 'job_id', 'kind', 'state', 'view', 'store_id', 'card_id', 'saturday', 'bout_id', 'error', 'result']); entryDateValue(j.saturday);
  requireValue(j.version === 1 && hex(j.job_id) && j.kind === 'run' && ['queued', 'running', 'completed', 'failed', 'cancelled'].includes(j.state) && viewKey(checkedView(j.view)) === currentKey() && j.store_id === bookedAuthority.storeID && hex(j.card_id) && hex(j.bout_id) && entryBytes(j) <= BOOKED_RESULT_MAX);
  if (j.state === 'completed') { requireValue(j.error === '' && j.result !== null); bookedResultValue(j.result, true); requireValue(j.card_id === j.result.record.card_id && j.bout_id === j.result.record.bout_id && j.saturday === j.result.record.source.event.saturday); }
  else { requireValue(j.result === null); if (['queued', 'running'].includes(j.state)) requireValue(j.error === ''); else requireValue(BOOKED_ERRORS.has(j.error) && j.error !== 'bout_setup_incomplete' && (j.state !== 'cancelled' || j.error === 'bout_cancelled')); }
  return j;
}
function bookedEnvelope(r, field) {
  exact(r, ['id', 'ok', 'view', 'store_id', 'snapshot', 'card_status', 'status', 'record', 'job', 'error']); requireValue(int(r.id, 1) && typeof r.ok === 'boolean' && entryBytes(r) + 1 <= 262144);
  const fields = ['snapshot', 'card_status', 'status', 'record', 'job'];
  if (!r.ok) { requireValue(r.view === null && r.store_id === '' && fields.every(k => r[k] === null) && BOOKED_ERRORS.has(r.error)); return { ok: false, error: r.error }; }
  requireValue(r.error === '' && fields.every(k => k === field ? r[k] !== null : r[k] === null)); const v = r[field];
  if (field === 'snapshot') {
    exact(v, ['version', 'state', 'error', 'view', 'store_id', 'pin', 'source_created_at_ms', 'created_at_ms', 'active_job_id', 'queued_job_id', 'last_job_id']);
    requireValue(v.version === 1 && ['ready', 'busy', 'failed', 'unavailable', 'setup_incomplete'].includes(v.state) && int(v.source_created_at_ms, 0, DATE_MAX) && int(v.created_at_ms, 0, DATE_MAX) && ['active_job_id', 'queued_job_id', 'last_job_id'].every(k => optionalID(v[k])) && same(r.view, v.view) && r.store_id === v.store_id);
    if (['unavailable', 'setup_incomplete'].includes(v.state)) { requireValue(v.pin === null && v.store_id === '' && v.source_created_at_ms === 0 && v.created_at_ms === 0 && !v.active_job_id && !v.queued_job_id && !v.last_job_id && v.error === (v.state === 'unavailable' ? 'bout_unavailable' : 'bout_setup_incomplete')); requireValue(v.state === 'unavailable' ? v.view === null : viewKey(checkedView(v.view)) === currentKey()); }
    else {
      requireValue(!!cardAuthority && viewKey(checkedView(v.view)) === currentKey() && hex(v.store_id)); exact(v.pin, ['version', 'source_store_id', 'source_pin', 'rules_id']);
      requireValue(v.pin.version === 1 && v.pin.source_store_id === cardAuthority.storeID && same(v.pin.source_pin, cardAuthority.pin) && v.pin.rules_id === BOOKED_RULES && v.source_created_at_ms === cardAuthority.createdMS && int(v.created_at_ms, v.source_created_at_ms, DATE_MAX));
      const ids = [v.active_job_id, v.queued_job_id, v.last_job_id].filter(Boolean); requireValue(new Set(ids).size === ids.length && !(v.active_job_id && v.queued_job_id) && (!(v.active_job_id || v.queued_job_id) || v.state === 'busy') && (v.state === 'failed' ? ['bout_unavailable', 'bout_corrupt', 'bout_clock', 'bout_source_changed'].includes(v.error) : v.error === '') && (v.state !== 'failed' || !v.active_job_id && !v.queued_job_id));
      const authority = { pin: v.pin, storeID: v.store_id, sourceCreatedMS: v.source_created_at_ms, createdMS: v.created_at_ms }; requireValue(!bookedAuthority || same(bookedAuthority, authority));
      if (!bookedAuthority) { v.pin.source_pin = cardAuthority.pin; bookedAuthority = authority; } v.pin = bookedAuthority.pin;
    } return { ok: true, value: v };
  }
  requireValue(bookedReady() && viewKey(checkedView(r.view)) === currentKey() && r.store_id === bookedAuthority.storeID);
  if (field === 'status' || field === 'card_status') bookedStatusValue(v, field === 'card_status');
  else if (field === 'record') bookedResultValue(v); else bookedJobValue(v);
  return { ok: true, value: v };
}
function bookedError(code, run = false) {
  if (code === 'outcome_unknown') return run ? 'The candidate transcript may have been saved. Use Check saved result before another explicit Run.' : 'The local source observation could not be confirmed. Check saved result after reopening the gym. No fight was requested by this read.';
  return ({ bout_setup_incomplete: 'The shared card setup is not ready. Sign in to both demo gyms first.', bout_unavailable: 'Saved bouts are unavailable in this route. Durable results remain retained.', bout_source_incomplete: 'A complete saved card with its fixed finalized seed is required.', bout_source_conflict: 'The retained seed has conflicting evidence. New runs are blocked; saved transcripts remain historical evidence.', bout_source_changed: 'The selected source identity changed. Reopen the gym before disclosing this saved transcript.', bout_not_found: 'No saved transcript was found. A new run requires a separate explicit Run action.', bout_capacity: 'The saved-bout store or display is full. Clear displayed details to release display space; saved transcripts are retained.', bout_clock: 'The local authority clock needs attention. Current usability could not be confirmed.', bout_corrupt: 'Saved evidence failed validation. Reopen the gym; no saved record was replaced.', bout_cancelled: 'The request was cancelled. Check saved result before another Run.', bout_calendar: 'Choose a supported Saturday in the entry panel.' })[code] ?? textError(code);
}
function bookedRefusal(code, run = false, selection = null) {
  if (['unauthorized', 'stale_view'].includes(code)) { clearBooked(); bookedUnavailable = true; }
  else if (!['busy', 'invalid_request', 'bout_calendar', 'bout_cancelled', 'bout_capacity'].includes(code)) { pauseBooked(); bookedClearImages(); bookedOverlayKnown = false; bookedOverlayRead = true; }
  if (code === 'bout_unavailable') bookedUnavailable = true;
  if (run && code === 'outcome_unknown') bookedUnknown = { date: selection?.date ?? entryDate, boutID: selection?.boutID ?? bookedSelected };
  bookedMessage = bookedError(code, run); renderBooked();
}
function bookedStatusInvariant(old, next) {
  if (!old || old.saved.bout.bout_id !== next.saved.bout.bout_id) return;
  requireValue(old.card_id === next.card_id && same(old.saved.bout, next.saved.bout) && old.saved.source_digest === next.saved.source_digest && (!old.saved.exists || next.saved.exists && ['record_digest', 'created_at_ms', 'input_id', 'result_digest', 'winner', 'method', 'end_round', 'end_tick'].every(k => old.saved[k] === next.saved[k])));
}
function bookedAdoptStatus(s, terminal = false) {
  bookedStatusInvariant(bookedStatus, s); if (bookedRecord) bookedMatchRecord(bookedRecord, s);
  if (terminal && (bookedOverlayRead || bookedStatus && bookedStatus.observed_at_ms > s.observed_at_ms)) return;
  requireValue(!bookedStatus || s.observed_at_ms >= bookedStatus.observed_at_ms);
  bookedStatus = s; bookedOverlayKnown = true; if (!terminal) bookedOverlayRead = true;
}
function bookedAdoptResult(v, terminal = false) {
  if (bookedRecord) requireValue(same(bookedRecord, v.record));
  const previous = [bookedRecord, bookedStatus, bookedOverlayKnown, bookedOverlayRead, bookedUnknown];
  bookedAdoptStatus(v.status, terminal); bookedRecord = bookedRecord ?? v.record;
  if (bookedUnknown?.boutID === v.record.bout_id && bookedUnknown.date === v.record.source.event.saturday) bookedUnknown = null;
  if (!bookedCapacity()) { [bookedRecord, bookedStatus, bookedOverlayKnown, bookedOverlayRead, bookedUnknown] = previous; throw bookedCacheError(); }
  pauseBooked(); bookedStep = 0;
}
async function fetchBookedSnapshot() {
  if (!ready() || !cardAuthority || bookedUnavailable) return; const c = bookedContext(), prior = [bookedSnapshot, bookedAuthority];
  try {
    const raw = await call('BoutSnapshot'); if (!bookedCurrent(c)) return; const r = bookedEnvelope(raw, 'snapshot'); if (!r.ok) { bookedRefusal(r.error); return; }
    bookedSnapshot = r.value; if (!bookedCapacity()) { [bookedSnapshot, bookedAuthority] = prior; throw bookedCacheError(); }
    if (['unavailable', 'failed'].includes(r.value.state)) { bookedRefusal(r.value.error); return; }
    const id = bookedWatch?.id || r.value.active_job_id || r.value.queued_job_id || r.value.last_job_id;
    if (id && (bookedWatch || id !== bookedLastJob)) await bookedFetchJob(id);
  } catch (e) { if (bookedCurrent(c)) { [bookedSnapshot, bookedAuthority] = prior; bookedRefusal(e.boutCode ?? 'bout_unavailable'); } } renderBooked();
}
async function bookedRead(kind) {
  if (!bookedReady() || !entryDate || !['card_status', 'status', 'record'].includes(kind) || kind !== 'card_status' && !bookedSelected) return;
  const c = bookedContext();
  try {
    const raw = await call({ card_status: 'BoutCardStatus', status: 'BoutStatus', record: 'BoutRecord' }[kind], c.viewID, c.date, ...(kind === 'card_status' ? [] : [c.boutID]));
    if (!bookedCurrent(c, true)) return; const r = bookedEnvelope(raw, kind); if (!r.ok) { bookedRefusal(r.error); return; }
    const s = kind === 'record' ? r.value.status : r.value; requireValue(s.source.event.saturday === c.date && (kind === 'card_status' || s.saved.bout.bout_id === c.boutID));
    if (kind === 'card_status') {
      if (bookedPage) { requireValue(s.card_id === bookedPage.card_id && s.observed_at_ms >= bookedPage.observed_at_ms); for (const old of bookedPage.bouts) { const next = s.bouts.find(v => v.bout.bout_id === old.bout.bout_id); requireValue(!!next); bookedStatusInvariant({ card_id: s.card_id, saved: old }, { card_id: s.card_id, saved: next }); } }
      const prior = [bookedPage, bookedStatus, bookedOverlayKnown, bookedOverlayRead];
      try { bookedPage = s; const selected = s.bouts.find(v => v.bout.bout_id === c.boutID); if (selected) bookedAdoptStatus({ version: 1, store_id: s.store_id, card_id: s.card_id, observed_at_ms: s.observed_at_ms, source: s.source, saved: selected }); if (!bookedCapacity()) throw bookedCacheError(); }
      catch (e) { [bookedPage, bookedStatus, bookedOverlayKnown, bookedOverlayRead] = prior; throw e; }
    } else if (kind === 'record') bookedAdoptResult(r.value);
    else { const prior = [bookedStatus, bookedOverlayKnown, bookedOverlayRead]; bookedAdoptStatus(s); if (!bookedCapacity()) { [bookedStatus, bookedOverlayKnown, bookedOverlayRead] = prior; throw bookedCacheError(); } }
    if (kind !== 'card_status' && bookedUnknown?.date === c.date && bookedUnknown.boutID === c.boutID) bookedUnknown = null;
    bookedMessage = kind === 'record' ? 'Saved candidate transcript opened. Playback is paused.' : 'Saved bout status checked. This read did not refresh the chain or run a fight.';
  } catch (e) { if (bookedCurrent(c, true)) bookedRefusal(e.boutCode ?? 'bout_unavailable'); } renderBooked();
}
function bookedSelect(id) {
  const row = bookedPage?.bouts.find(v => v.bout.bout_id === id); if (!row || !bookedReady() || command) return;
  publicationSelect(); bookedSerial++; bookedClearRecord(); bookedSelected = id; bookedStatus = { version: 1, store_id: bookedPage.store_id, card_id: bookedPage.card_id, observed_at_ms: bookedPage.observed_at_ms, source: bookedPage.source, saved: row }; bookedOverlayKnown = true; bookedOverlayRead = true; bookedMessage = row.exists ? 'Open the saved candidate transcript to replay it.' : 'This booked bout has not been run. Run is an explicit action.'; renderBooked();
}
function bookedCheck() {
  if (bookedUnknown?.boutID && bookedUnknown.date) { const pending = bookedUnknown; if (entryDate !== pending.date) { entryDate = pending.date; ui['entry-date'].value = entryDate; eventSelectDate(entryDate); } if (bookedSelected !== pending.boutID) { publicationSelect(); bookedSerial++; bookedClearRecord(); bookedStatus = null; bookedSelected = pending.boutID; bookedOverlayKnown = bookedOverlayRead = false; } }
  const c = bookedContext(); queue(() => bookedCurrent(c, true) ? bookedRead('status') : undefined, 'bout-check');
}
async function bookedBegin() {
  const selected = bookedStatus?.saved;
  if (!bookedReady() || !bookedSelected || bookedWatch || bookedUnknown || bookedSnapshot.state === 'busy' || !bookedOverlayKnown || !bookedStatus?.source.usable || !selected || selected.exists || bookedRecord || selected.bout.bout_id !== bookedSelected) return;
  const c = bookedContext(), priorWatch = bookedWatch; bookedWatch = { id: '', state: 'queued', date: c.date, boutID: c.boutID, cardID: bookedStatus.card_id };
  if (!bookedCapacity()) { bookedWatch = priorWatch; bookedMessage = bookedError('bout_capacity'); return; } pauseBooked();
  try {
    const raw = await call('BeginBoutRun', c.viewID, c.date, c.boutID); if (!bookedCurrent(c)) return; const r = bookedEnvelope(raw, 'job');
    if (!r.ok) { bookedWatch = priorWatch; bookedRefusal(r.error, true, c); return; }
    const j = r.value; requireValue(j.state === 'queued' && j.saturday === c.date && j.bout_id === c.boutID && j.card_id === bookedWatch.cardID);
    bookedTerminal = null; bookedWatch = { id: j.job_id, state: j.state, date: j.saturday, boutID: j.bout_id, cardID: j.card_id };
    if (bookedCurrent(c, true)) { bookedClearRecord(); bookedOverlayRead = false; } bookedMessage = 'Candidate bout queued. No record or award is granted.';
  } catch { if (bookedCurrent(c)) { bookedWatch = priorWatch; bookedRefusal('outcome_unknown', true, c); } } renderBooked();
}
function bookedTerminalKey(j) { return JSON.stringify([j.job_id, j.state, viewKey(j.view), j.store_id, j.card_id, j.saturday, j.bout_id, j.error, j.result?.status ?? null, j.result?.replayed ?? null]); }
async function bookedFetchJob(id) {
  if (!bookedReady()) return; const c = bookedContext(), expected = bookedWatch; let rollback = null;
  try {
    const raw = await call('BoutJob', c.viewID, id); if (!bookedCurrent(c) || expected !== bookedWatch) return; const r = bookedEnvelope(raw, 'job');
    if (!r.ok) { if (r.error !== 'busy') { bookedWatch = null; bookedRefusal(r.error, !expected || !!expected.id, expected ? { date: expected.date, boutID: expected.boutID } : c); } return; }
    const j = r.value; requireValue(j.job_id === id && (!expected || j.saturday === expected.date && j.bout_id === expected.boutID && j.card_id === expected.cardID && !(expected.state === 'running' && j.state === 'queued')));
    if (['queued', 'running'].includes(j.state)) { const prior = bookedWatch; bookedWatch = { id, state: j.state, date: j.saturday, boutID: j.bout_id, cardID: j.card_id }; if (!bookedCapacity()) { bookedWatch = prior; throw bookedCacheError(); } return; }
    const key = bookedTerminalKey(j); if (bookedTerminal?.id === id) requireValue(bookedTerminal.key === key);
    rollback = [bookedWatch, bookedLastJob, bookedTerminal, bookedRecord, bookedStatus, bookedOverlayKnown, bookedOverlayRead, bookedUnknown];
    bookedWatch = null; bookedLastJob = id; bookedTerminal = { id, key };
    if (j.state === 'completed') {
      if (bookedCurrent(c, true) && j.saturday === entryDate && j.bout_id === bookedSelected) bookedAdoptResult(j.result, true);
      bookedMessage = j.result.replayed ? 'The original saved candidate transcript was recovered.' : 'The candidate transcript was saved. Open it to replay. Saving alone does not award records, XP or prizes; result finality is shown separately.';
    } else bookedRefusal(j.error, true, { date: j.saturday, boutID: j.bout_id });
    if (!bookedCapacity()) throw bookedCacheError();
  } catch (e) { if (bookedCurrent(c)) { if (rollback) [bookedWatch, bookedLastJob, bookedTerminal, bookedRecord, bookedStatus, bookedOverlayKnown, bookedOverlayRead, bookedUnknown] = rollback; if (e.boutCode === 'bout_capacity') bookedMessage = bookedError(e.boutCode); else { bookedWatch = null; bookedRefusal('outcome_unknown', true, expected ? { date: expected.date, boutID: expected.boutID } : c); } } } renderBooked();
}
async function bookedCancel() {
  if (!bookedReady() || !bookedWatch?.id) return; const c = bookedContext(), w = bookedWatch;
  try { const raw = await call('CancelBoutJob', c.viewID, w.id); if (!bookedCurrent(c)) return; const r = bookedEnvelope(raw, 'job'); if (!r.ok) { bookedRefusal(r.error, true, { date: w.date, boutID: w.boutID }); return; } requireValue(r.value.job_id === w.id && r.value.saturday === w.date && r.value.bout_id === w.boutID && r.value.card_id === w.cardID && !(w.state === 'running' && r.value.state === 'queued')); await bookedFetchJob(w.id); }
  catch { if (bookedCurrent(c)) bookedRefusal('outcome_unknown', true, { date: w.date, boutID: w.boutID }); } renderBooked();
}
function bookedNextPortrait() { return bookedReady() && bookedRecord && bookedOverlayKnown ? bookedRecord.source.corners.find(v => !bookedImageAttempts.has(v.member.assignment.prospect_id)) ?? null : null; }
async function loadBookedPortrait(corner) {
  const c = bookedContext(), record = bookedRecord, a = corner.member.assignment; if (!record) return; bookedImageAttempts.add(a.prospect_id);
  const remains = () => bookedCurrent(c, true) && bookedRecord === record;
  try {
    const r = await call('BoutPortrait', c.viewID, c.date, c.boutID, a.prospect_id); if (!remains()) return;
    exact(r, ['version', 'ok', 'portrait', 'status', 'error']); requireValue(r.version === 1 && typeof r.ok === 'boolean' && entryBytes(r) <= 49152);
    if (!r.ok) { requireValue(r.portrait === null && r.status === null && BOOKED_ERRORS.has(r.error) && r.error !== 'bout_setup_incomplete'); bookedRefusal(r.error); renderBooked(); return; }
    requireValue(r.error === ''); const p = r.portrait;
    exact(p, ['version', 'view', 'store_id', 'card_id', 'saturday', 'bout_id', 'prospect_id', 'record_digest', 'input_id', 'result_digest', 'descriptor_id', 'manifest_digest', 'portrait_digest', 'svg']);
    requireValue(p.version === 1 && viewKey(checkedView(p.view)) === c.view && p.store_id === bookedAuthority.storeID && p.card_id === record.card_id && p.saturday === c.date && p.bout_id === c.boutID && p.prospect_id === a.prospect_id && p.record_digest === bookedStatus.saved.record_digest && p.input_id === record.input_id && p.result_digest === record.result_digest && p.descriptor_id === DESCRIPTOR && p.manifest_digest === a.manifest_digest && p.portrait_digest === a.portrait_digest && entryBytes(p.svg) <= 32768 && imageOnlySVG(p.svg));
    bookedStatusValue(r.status); bookedMatchRecord(record, r.status); const old = [bookedStatus, bookedOverlayKnown, bookedOverlayRead]; bookedAdoptStatus(r.status);
    const src = `data:image/svg+xml;base64,${btoa(p.svg)}`, priorImage = bookedImages.get(a.prospect_id); bookedImages.set(a.prospect_id, src);
    if (!bookedCapacity()) { if (priorImage) bookedImages.set(a.prospect_id, priorImage); else bookedImages.delete(a.prospect_id); [bookedStatus, bookedOverlayKnown, bookedOverlayRead] = old; throw bookedCacheError(); }
  } catch (e) { if (remains()) bookedRefusal(e.boutCode ?? 'bout_unavailable'); } renderBooked();
}
function bookedTicks() { return bookedRecord ? bookedRecord.result.rounds.reduce((n, r) => n + r.ticks.length, 0) : 0; }
function stepBooked(n) { pauseBooked(); bookedStep = Math.max(0, Math.min(bookedTicks(), n)); renderBooked(); }
function playBooked() {
  if (!bookedRecord || !bookedReady() || motionPreference?.matches || document.hidden || command) return;
  if (bookedTimer !== null) { pauseBooked(); renderBooked(); return; } if (bookedStep >= bookedTicks()) bookedStep = 0;
  const c = bookedContext(), r = bookedRecord; const tick = () => { bookedTimer = null; if (!bookedCurrent(c, true) || bookedRecord !== r || document.hidden || motionPreference?.matches || command) return; bookedStep++; if (bookedStep < bookedTicks()) bookedTimer = setTimeout(tick, 250); renderBooked(); }; bookedTimer = setTimeout(tick, 250); renderBooked();
}
function renderBooked() {
  const active = bookedReady(), selected = bookedStatus?.saved, busy = !!command || !!bookedWatch || bookedSnapshot?.state === 'busy';
  const publication = publicationForSelectedBout(), outcome = publicationOutcomeLabel(publication);
  content(ui['bout-state'], !ready() ? 'Sign in to view saved bouts' : bookedUnavailable ? 'Unavailable in this route' : bookedSnapshot?.state === 'setup_incomplete' ? 'Setup incomplete' : bookedWatch ? 'Candidate bout in progress' : bookedRecord ? `Saved candidate transcript · ${outcome}` : 'Booked bouts'); content(ui['bout-message'], bookedUnknown ? `${bookedError('outcome_unknown', true)} Pending Saturday ${bookedUnknown.date || 'unknown'} · ${bookedUnknown.boutID || 'waiting for job identity'}.` : bookedMessage);
  ui['bout-refresh'].disabled = !active || busy || !entryDate; ui['bout-run'].disabled = !active || busy || !selected || selected.exists || !!bookedRecord || !bookedOverlayKnown || !bookedStatus.source.usable || !!bookedUnknown;
  ui['bout-check'].disabled = !active || busy || !(bookedSelected || bookedUnknown?.boutID); ui['bout-open'].disabled = !active || busy || !selected?.exists; ui['bout-cancel'].disabled = !active || !!command || !bookedWatch?.id; ui['bout-clear'].disabled = !bookedRecord && !bookedPage;
  const displayed = bookedPage?.bouts.map(v => bookedStatus?.saved.bout.bout_id === v.bout.bout_id ? bookedStatus.saved : v) ?? [];
  const key = bookedPage ? JSON.stringify([bookedPage.card_id, bookedSelected, bookedRecord?.bout_id ?? '', busy, displayed.map(v => [v.bout.bout_id, v.exists, v.method, v.usable])]) : '';
  if (ui['bout-list']._key !== key) { ui['bout-list']._key = key; ui['bout-list'].textContent = ''; if (bookedPage) for (const v of displayed) { const b = v.bout, li = element('li'), button = element('button', '', `${label(b.class)} · ${v.exists || bookedRecord?.bout_id === b.bout_id ? 'Saved candidate transcript' : 'Not run'}`); button.type = 'button'; button.disabled = !active || !!command; button.setAttribute('aria-pressed', b.bout_id === bookedSelected ? 'true' : 'false'); button.addEventListener('click', () => bookedSelect(b.bout_id)); const detail = element('details'); detail.append(element('summary', '', 'Booked fighter identities'), element('p', 'note card-identities', `${b.corner_a} vs ${b.corner_b}`)); li.append(button, detail); ui['bout-list'].append(li); } }
  const source = bookedStatus?.source ?? bookedPage?.source, conflict = source && (source.conflict_count > 0 || source.conflict_overflow);
  content(ui['bout-source'], source ? `${source.event.saturday} · Original noon condition ${new Date(source.event.cutoff_ms).toISOString()}. ${source.chain_observed_at_ms ? `Last verified chain observation ${new Date(source.chain_observed_at_ms).toISOString()}.` : 'No verified chain observation.'} Local status ${new Date(bookedStatus?.observed_at_ms ?? bookedPage.observed_at_ms).toISOString()}.` : 'Choose a Saturday above and read its booked-bout status. Unmatched fighters remain listed in the card.');
  content(ui['bout-overlay'], !bookedOverlayKnown && bookedStatus ? 'Current source usability is unconfirmed. The retained transcript is historical evidence.' : conflict ? 'Seed conflict: new runs are blocked. The saved transcript and its original noon inputs remain unchanged.' : source ? source.usable ? 'Candidate source usable at the last local observation; this is not continuous chain monitoring.' : 'The source is not currently usable for a new candidate run.' : '');
  if (source?.incident_at_ms) content(ui['bout-overlay'], `${ui['bout-overlay'].textContent} Incident first observed ${new Date(source.incident_at_ms).toISOString()}. No event cancellation or reseed.`);
  content(ui['bout-empty'], bookedPage ? bookedPage.bouts.length ? '' : bookedPage.source.card_exists ? 'This saved card has no booked bouts. Unmatched entrants have no loss.' : 'No card is saved for this Saturday.' : '');
  renderPublication(); ui['bout-replay'].hidden = !bookedRecord; if (!bookedRecord) return;
  const r = bookedRecord, result = r.result, fighters = [r.input.a, r.input.b], names = fighters.map(v => v.manifest.name.display);
  content(ui['bout-asof'], `${outcome}. ${publication?.phase === 'conflict' ? 'Conflict reported. ' : ''}${!publication ? 'Finality has not been read for this outcome. ' : ''}Noon inputs ${new Date(r.input.as_of_ms).toISOString()} · Transcript saved ${new Date(r.created_at_ms).toISOString()}. Later retirement does not change these inputs.`);
  const rendered = r.result_digest + JSON.stringify([...bookedImages.keys()]);
  if (bookedRendered !== rendered) {
    bookedRendered = rendered; ui['bout-fighters'].textContent = ''; for (const [n, f] of fighters.entries()) { const box = element('div', 'preview-fighter'), corner = r.source.corners[n], src = bookedImages.get(f.prospect_id); if (src) { const img = element('img'); img.setAttribute('src', src); img.setAttribute('alt', `Candidate portrait of ${names[n]}`); img.setAttribute('width', '160'); img.setAttribute('height', '160'); box.append(img); } box.append(element('strong', '', names[n]), element('span', 'note', `${cardGymName(corner.owner.gym_id)} at noon · ${label(corner.member.condition_at_cutoff.phase)} · Rating ${(corner.member.condition_at_cutoff.rating_hundredths / 100).toFixed(2)} · XP 0 · Training 0`)); ui['bout-fighters'].append(box); }
    ui['bout-scores'].textContent = ''; ui['bout-scores'].append(element('caption', '', 'Whole-fight scorecard')); const head = element('tr'); for (const title of ['Component', ...names]) { const th = element('th', '', title); th.setAttribute('scope', 'col'); head.append(th); } const thead = element('thead'); thead.append(head); ui['bout-scores'].append(thead); const body = element('tbody');
    for (const [k, title] of [['damage_tenths', 'Damage'], ['knockdown_tenths', 'Knockdowns'], ['accuracy_tenths', 'Accuracy'], ['volume_tenths', 'Volume'], ['control_tenths', 'Control'], ['total_tenths', 'Total']]) { const row = element('tr'), th = element('th', '', title); th.setAttribute('scope', 'row'); row.append(th, ...result.scores.map(s => element('td', '', (s[k] / 10).toFixed(1)))); body.append(row); } ui['bout-scores'].append(body);
  }
  content(ui['bout-scores'].children[0], `${outcome} · Whole-fight scorecard`);
  content(ui['bout-play'], bookedTimer === null ? 'Play' : 'Pause'); ui['bout-play'].disabled = !active || !!command || !!motionPreference?.matches; ui['bout-previous'].disabled = bookedStep === 0; ui['bout-next'].disabled = bookedStep >= bookedTicks(); content(ui['bout-motion'], motionPreference?.matches ? 'Reduced motion is on. Step through or show the result.' : 'Playback is local. Pause or step through at any time.');
  ui['bout-events'].textContent = ''; let n = 0, last = null; for (const round of result.rounds) for (const t of round.ticks) { if (++n > bookedStep) break; last = { round: round.number, tick: t.number }; ui['bout-events'].append(element('li', '', `Round ${round.number}, tick ${t.number}: ${names[t.actor]} ${t.hit ? 'lands' : 'misses'} a ${t.strike}.${t.knockdown ? ' Knockdown.' : ''}${t.stoppage ? ` ${label(t.stoppage)}.` : ''}`)); }
  content(ui['bout-step'], last ? `Round ${last.round}, tick ${last.tick} · Step ${bookedStep} of ${bookedTicks()}` : 'Paused at the opening bell.'); ui['bout-outcome'].hidden = bookedStep < bookedTicks();
  const winner = fighters.find(v => v.prospect_id === result.winner)?.manifest.name.display; content(ui['bout-finish'], `${outcome}: ${result.method === 'decision_draw' ? 'Candidate draw — equal whole-fight scores' : result.method === 'double_corner_stoppage' ? 'Candidate draw — both corners stopped the fight' : `${winner} wins the candidate bout by ${label(result.method)}.`}`);
  content(ui['bout-evidence'], `Bout ${r.bout_id} · Input ${r.input_id} · Result ${r.result_digest}`); requireValue(entryBytes([key, rendered, ui['bout-evidence'].textContent]) <= 4096);
}
motionPreference?.addEventListener('change', () => { pauseBooked(); renderBooked(); });

// Result publication consumes saved evidence. Native owns cryptographic and
// account authority; presentation never signs, hashes a proof or runs a fight.
const RESULT_MAX = 294912, RESULT_JOB_MAX = 245760, RESULT_META_MAX = 49152;
const RESULT_CONSTANTS = { publisher_public_key: '984ebcc3fd73fcf64dd8ebb6a03b01a6ed2403548a479ce3089dca1392841bcf', checkpoint_public_key: 'aee7dd24b6a03c5a02f452f5f588c8ca7e1dba000d9e21a736993e5842b30ca4', funding_public_key: '02c6047f9441ed7d6d3045406e95c07cd85c778e4b8cef3ca7abac09b95c709ee5', funding_script: '001406afd46bcdfd22ef94ac122aa11f241244a37ecc', rules_id: 'b388cec7124548b4af5d5b096252b881f0a472878d5ee5e23f805d3a53371816' };
const RESULT_ERRORS = new Set(['invalid_request', 'unauthorized', 'forbidden', 'stale_view', 'busy', 'outcome_unknown', 'result_unavailable', 'result_corrupt', 'result_capacity', 'result_clock', 'result_calendar', 'result_not_found', 'result_not_prepared', 'result_source_incomplete', 'result_source_changed', 'result_source_conflict', 'result_node_offline', 'result_waiting_for_funds', 'result_scan_incomplete', 'result_not_mature', 'result_conflict', 'result_cancelled', 'result_proof_unavailable', 'result_setup_incomplete', 'invalid_frame']);
let publicationAuthority = null, publicationSnapshot = null, publicationStatus = null, publicationRecord = null, publicationExport = null;
let publicationWatch = null, publicationTerminal = null, publicationUnknown = null, publicationLastJob = '', publicationMessage = '';
let publicationUnavailable = false, publicationOverlayKnown = false, publicationOverlayRead = false, publicationSerial = 0;
function publicationMetadataBytes() { return entryBytes([publicationAuthority, publicationSnapshot && { ...publicationSnapshot, pin: null }, publicationStatus, publicationExport, publicationWatch, publicationTerminal, publicationUnknown, publicationLastJob, publicationMessage]) + 4096; }
function publicationRetainedBytes() { if (!publicationAuthority && !publicationSnapshot && !publicationWatch && !publicationRecord) return 0; return publicationWatch ? RESULT_MAX : publicationMetadataBytes() + entryBytes(publicationRecord); }
function publicationCapacity() { return publicationMetadataBytes() <= RESULT_META_MAX && entryBytes(publicationRecord) <= RESULT_JOB_MAX && publicationRetainedBytes() <= RESULT_MAX && previewCapacity(); }
function publicationCacheError() { const e = new Error('Publication display full'); e.resultCode = 'result_capacity'; return e; }
function publicationReady() { return bookedReady() && !publicationUnavailable && !!publicationAuthority && !!publicationSnapshot?.pin && viewKey(publicationSnapshot.view) === currentKey(); }
function publicationContext() { return { ...bookedContext(), publicationSerial }; }
function publicationCurrent(c, selection = false) { return bookedCurrent(c, selection) && (!selection || c.publicationSerial === publicationSerial); }
function publicationSelect() { publicationSerial++; publicationStatus = publicationRecord = publicationExport = null; publicationOverlayKnown = publicationOverlayRead = false; publicationMessage = ''; }
function clearPublication() { publicationSelect(); publicationAuthority = publicationSnapshot = publicationWatch = publicationTerminal = publicationUnknown = null; publicationLastJob = ''; publicationUnavailable = false; }
function publicationPin(p) {
  exact(p, ['version', 'source_store_id', 'source_pin', 'publisher_gym_id', 'publisher_account_id', 'publisher_source_authority_id', 'publisher_chain_id', 'network', 'genesis', 'floor_height', 'floor_hash', 'publisher_public_key', 'checkpoint_public_key', 'funding_public_key', 'funding_script', 'rules_id']);
  requireValue(p.version === 1 && !!bookedAuthority && p.source_store_id === bookedAuthority.storeID && same(p.source_pin, bookedAuthority.pin) && p.publisher_gym_id === fixtures.zero256[0] && p.network === 0x534c5547 && p.genesis === GENESIS && p.floor_height === 0 && p.floor_hash === GENESIS);
  requireValue(Object.entries(RESULT_CONSTANTS).every(([k, v]) => p[k] === v));
  const seed = bookedAuthority.pin.source_pin.source_pin;
  for (const k of ['publisher_account_id', 'publisher_source_authority_id', 'publisher_chain_id']) requireValue(hex(p[k]) && p[k] === seed[k]);
}
function publicationSource(v) {
  exact(v, ['version', 'store_id', 'store_created_at_ms', 'record_digest', 'source_digest', 'input_id', 'result_digest', 'created_at_ms', 'source', 'summary']); bookedSourceValue(v.source);
  requireValue(v.version === 1 && v.store_id === bookedAuthority.storeID && v.store_created_at_ms === bookedAuthority.createdMS && ['record_digest', 'source_digest', 'input_id', 'result_digest'].every(k => hex(v[k])) && int(v.created_at_ms, Math.max(v.store_created_at_ms, v.source.card_created_at_ms, v.source.finalized_at_ms), DATE_MAX) && entryBytes(v) <= 20480);
  const s = v.summary; exact(s, ['winner', 'method', 'end_round', 'end_tick']); requireValue(FINISHES.includes(s.method) && int(s.end_round, 1, 3) && int(s.end_tick, 1, 60) && (['decision_draw', 'double_corner_stoppage'].includes(s.method) ? s.winner === '' : [v.source.bout.corner_a, v.source.bout.corner_b].includes(s.winner)));
}
function publicationStatement(v) {
  exact(v, ['version', 'statement', 'signature']); requireValue(v.version === 1 && seedHex(v.signature, 64)); const s = v.statement, p = publicationAuthority.pin;
  exact(s, ['version', 'result_key', 'store_id', 'source_store_id', 'source', 'publisher_chain_id', 'network', 'genesis', 'rules_id', 'publisher_public_key', 'checkpoint_public_key']); publicationSource(s.source);
  requireValue(s.version === 1 && hex(s.result_key) && s.store_id === publicationAuthority.storeID && s.source_store_id === p.source_store_id && ['publisher_chain_id', 'network', 'genesis', 'rules_id', 'publisher_public_key', 'checkpoint_public_key'].every(k => s[k] === p[k])); return s;
}
function publicationAnchor(a) {
  exact(a, ['height', 'hash', 'header', 'txid', 'raw_tx_digest', 'raw_tx_size', 'tx_count', 'tx_index', 'output_index', 'payload', 'merkle_branch']);
  requireValue(int(a.height, 1, 2147483647) && ['hash', 'txid', 'raw_tx_digest'].every(k => hex(a[k])) && seedHex(a.header, 80) && seedHex(a.payload, 73) && a.payload.startsWith('534c554752534c5401') && !/^0{64}$/.test(a.payload.slice(18, 82)) && !/^0{64}$/.test(a.payload.slice(82)) && int(a.raw_tx_size, 1, 1024) && int(a.tx_count, 1, 400000) && int(a.tx_index, 0, a.tx_count - 1) && a.output_index === 0 && Array.isArray(a.merkle_branch) && a.merkle_branch.length <= 20 && a.merkle_branch.every(hex));
}
function publicationInclusion(v, r) {
  exact(v, ['version', 'generation', 'anchor', 'observed_at_ms', 'observed_tip']); publicationAnchor(v.anchor); seedTip(v.observed_tip);
  requireValue(v.version === 1 && int(v.generation, 0, 16) && int(v.observed_at_ms, r.prepared_at_ms, DATE_MAX) && v.observed_tip.height >= v.anchor.height && !!r.publication && v.anchor.txid === r.publication.txid && v.anchor.raw_tx_size * 2 === r.publication.raw_tx.length && v.anchor.payload.slice(18, 82) === r.result_key);
}
function publicationRecordValue(r) {
  exact(r, ['version', 'store_id', 'result_key', 'statement', 'prepared_at_ms', 'publication', 'cursor', 'scan_restarts', 'inclusion', 'prior_inclusions', 'checkpoint', 'finality', 'equivocation', 'conflicts', 'conflict_overflow', 'observation']);
  requireValue(r.version === 1 && r.store_id === publicationAuthority.storeID && hex(r.result_key) && entryBytes(r) <= 196608); const s = publicationStatement(r.statement);
  requireValue(r.result_key === s.result_key && int(r.prepared_at_ms, Math.max(publicationAuthority.createdMS, s.source.created_at_ms), DATE_MAX)); seedCursor(r.cursor);
  for (const k of ['publication', 'inclusion', 'checkpoint', 'finality', 'equivocation']) requireValue(r[k] === null || !!r[k] && typeof r[k] === 'object' && !Array.isArray(r[k]));
  if (r.publication) {
    const p = r.publication; exact(p, ['version', 'raw_tx', 'txid', 'coin', 'built_at_ms', 'dispatch_count', 'first_dispatch_at_ms', 'last_dispatch_at_ms', 'state']);
    requireValue(p.version === 1 && typeof p.raw_tx === 'string' && p.raw_tx.length <= 2048 && seedHex(p.raw_tx, p.raw_tx.length / 2) && hex(p.txid) && ['built', 'dispatch_unknown', 'submitted'].includes(p.state) && int(p.dispatch_count, 0, 4294967295) && int(p.built_at_ms, r.prepared_at_ms, DATE_MAX));
    exact(p.coin, ['outpoint', 'value_satoshis', 'script', 'block_height', 'block_hash', 'coinbase']); exact(p.coin.outpoint, ['txid', 'index']); requireValue(hex(p.coin.outpoint.txid) && int(p.coin.outpoint.index, 0, 4294967295) && int(p.coin.value_satoshis, 1294, 2100000000000000) && p.coin.script === RESULT_CONSTANTS.funding_script && int(p.coin.block_height, 0, 2147483647) && hex(p.coin.block_hash) && typeof p.coin.coinbase === 'boolean');
    requireValue(p.dispatch_count === 0 ? p.state === 'built' && p.first_dispatch_at_ms === 0 && p.last_dispatch_at_ms === 0 : p.state !== 'built' && int(p.first_dispatch_at_ms, p.built_at_ms, DATE_MAX) && int(p.last_dispatch_at_ms, p.first_dispatch_at_ms, DATE_MAX));
  }
  requireValue(Array.isArray(r.scan_restarts) && r.scan_restarts.length <= 16 && r.cursor.generation === r.scan_restarts.length);
  for (const [n, v] of r.scan_restarts.entries()) { exact(v, ['generation', 'previous', 'observed_at_ms', 'observed_tip', 'replacement_hash']); seedCursor(v.previous); seedTip(v.observed_tip); requireValue(v.generation === n + 1 && v.previous.generation === n && int(v.observed_at_ms, r.prepared_at_ms, DATE_MAX) && optionalID(v.replacement_hash)); }
  if (r.inclusion) publicationInclusion(r.inclusion, r);
  requireValue(Array.isArray(r.prior_inclusions) && r.prior_inclusions.length <= 16);
  for (const v of r.prior_inclusions) { exact(v, ['version', 'inclusion', 'orphaned_at_ms', 'observed_tip', 'replacement_hash']); publicationInclusion(v.inclusion, r); seedTip(v.observed_tip); requireValue(v.version === 1 && int(v.orphaned_at_ms, v.inclusion.observed_at_ms, DATE_MAX) && optionalID(v.replacement_hash)); }
  if (r.checkpoint) { seedCheckpoint(r.checkpoint); requireValue(!!r.inclusion && r.checkpoint.height === r.inclusion.anchor.height && r.checkpoint.hash === r.inclusion.anchor.hash); }
  if (r.finality) { const f = r.finality; exact(f, ['checkpoint', 'inclusion', 'observed_at_ms', 'observed_tip', 'descendants']); seedCheckpoint(f.checkpoint); publicationInclusion(f.inclusion, r); seedTip(f.observed_tip); requireValue(same(f.checkpoint, r.checkpoint) && same(f.inclusion, r.inclusion) && int(f.observed_at_ms, f.inclusion.observed_at_ms, DATE_MAX) && int(f.descendants, 60, 2147483647) && f.descendants === f.observed_tip.height - f.inclusion.anchor.height); }
  if (r.equivocation) { const e = publicationStatement(r.equivocation); requireValue(e.result_key === r.result_key && e.source.source.card_id === s.source.source.card_id && e.source.source.bout.bout_id === s.source.source.bout.bout_id && !same(e, s)); }
  requireValue(Array.isArray(r.conflicts) && r.conflicts.length <= 8 && typeof r.conflict_overflow === 'boolean');
  for (const c of r.conflicts) { exact(c, ['kind', 'observed_at_ms', 'expected_height', 'expected_hash', 'observed_hash', 'observed_tip', 'checkpoint']); requireValue(['source_chain_conflict', 'checkpoint_chain_conflict', 'checkpoint_equivocation', 'statement_equivocation'].includes(c.kind) && int(c.observed_at_ms, r.prepared_at_ms, DATE_MAX) && int(c.expected_height, 0, 2147483647) && optionalID(c.expected_hash) && optionalID(c.observed_hash)); if (c.observed_tip !== null) seedTip(c.observed_tip); if (c.checkpoint !== null) seedCheckpoint(c.checkpoint); }
  const o = r.observation; exact(o, ['observed_at_ms', 'chain_observed_at_ms', 'tip', 'node_state', 'descendants', 'current_finality']); requireValue(int(o.observed_at_ms, 0, DATE_MAX) && int(o.chain_observed_at_ms, 0, o.observed_at_ms) && ['unknown', 'online', 'offline'].includes(o.node_state) && int(o.descendants, 0, 2147483647) && typeof o.current_finality === 'boolean' && (o.chain_observed_at_ms === 0) === (o.tip === null)); if (o.tip !== null) seedTip(o.tip);
  requireValue(!o.current_finality || !!r.finality && !r.conflicts.length && !r.conflict_overflow && !r.equivocation && o.node_state === 'online' && o.tip.height - r.inclusion.anchor.height === o.descendants && o.descendants >= 60);
  return r;
}
function publicationStatusValue(s) {
  exact(s, ['version', 'store_id', 'result_key', 'card_id', 'bout_id', 'event', 'exists', 'phase', 'record_digest', 'source_record_digest', 'input_id', 'result_digest', 'prepared_at_ms', 'publication_state', 'txid', 'scan_height', 'scan_tip_height', 'scan_generation', 'inclusion_height', 'inclusion_hash', 'inclusion_generation', 'prior_inclusion_count', 'descendants', 'checkpoint_present', 'finalized_at_ms', 'conflict_count', 'conflict_overflow', 'observed_at_ms', 'chain_observed_at_ms', 'node_state', 'funding_state', 'current_finality', 'source']);
  bookedStatusValue(s.source); eventValue(s.event); requireValue(s.version === 1 && s.store_id === publicationAuthority.storeID && ['result_key', 'card_id', 'bout_id'].every(k => hex(s[k])) && s.card_id === s.source.card_id && s.bout_id === s.source.saved.bout.bout_id && same(s.event, s.source.source.event) && entryBytes(s) <= 16384);
  requireValue(['absent', 'prepared', 'provisional', 'finalized', 'conflict'].includes(s.phase) && ['none', 'built', 'dispatch_unknown', 'submitted'].includes(s.publication_state) && ['unknown', 'online', 'offline'].includes(s.node_state) && ['unknown', 'scanning', 'available', 'waiting_for_funds'].includes(s.funding_state));
  for (const k of ['exists', 'checkpoint_present', 'conflict_overflow', 'current_finality']) requireValue(typeof s[k] === 'boolean');
  requireValue(int(s.observed_at_ms, Math.max(publicationAuthority.createdMS, s.source.observed_at_ms), DATE_MAX));
  for (const k of ['prepared_at_ms', 'finalized_at_ms', 'chain_observed_at_ms']) requireValue(int(s[k], 0, s.observed_at_ms));
  for (const k of ['scan_height', 'scan_tip_height']) requireValue(int(s[k], -1, 2147483647));
  for (const k of ['inclusion_height', 'descendants']) requireValue(int(s[k], 0, 2147483647));
  for (const k of ['scan_generation', 'inclusion_generation', 'prior_inclusion_count']) requireValue(int(s[k], 0, 16));
  requireValue(int(s.conflict_count, 0, 8) && optionalID(s.inclusion_hash) && optionalID(s.txid) && (s.publication_state === 'none') === (s.txid === '') && (s.inclusion_height === 0) === (s.inclusion_hash === ''));
  if (!s.exists) requireValue(s.phase === 'absent' && ['record_digest', 'source_record_digest', 'input_id', 'result_digest', 'txid', 'inclusion_hash'].every(k => s[k] === '') && ['prepared_at_ms', 'finalized_at_ms', 'chain_observed_at_ms', 'inclusion_height', 'inclusion_generation', 'prior_inclusion_count', 'descendants', 'conflict_count', 'scan_generation'].every(k => s[k] === 0) && !s.checkpoint_present && !s.conflict_overflow && !s.current_finality && s.publication_state === 'none' && s.scan_height === -1 && s.scan_tip_height === -1);
  else { const b = s.source.saved; requireValue(b.exists && ['record_digest', 'source_record_digest', 'input_id', 'result_digest'].every(k => hex(s[k])) && s.source_record_digest === b.record_digest && s.input_id === b.input_id && s.result_digest === b.result_digest && int(s.prepared_at_ms, Math.max(publicationAuthority.createdMS, b.created_at_ms), s.observed_at_ms)); }
  requireValue(!s.current_finality || s.finalized_at_ms > 0 && s.descendants >= 60 && s.chain_observed_at_ms > 0 && s.node_state === 'online' && !s.conflict_count && !s.conflict_overflow && s.scan_tip_height - s.inclusion_height === s.descendants);
  const conflict = s.conflict_count > 0 || s.conflict_overflow || s.exists && !s.source.saved.usable;
  requireValue(!conflict || s.phase === 'conflict'); requireValue(s.phase !== 'finalized' || s.finalized_at_ms > 0 && !conflict); requireValue(!s.finalized_at_ms || s.checkpoint_present && s.inclusion_height > 0);
  return s;
}
function publicationMatch(r, s) {
  requireValue(s.exists && r.result_key === s.result_key && r.store_id === s.store_id && r.prepared_at_ms === s.prepared_at_ms); const src = r.statement.statement.source, b = s.source.saved;
  requireValue(src.record_digest === s.source_record_digest && src.input_id === s.input_id && src.result_digest === s.result_digest && src.source_digest === b.source_digest && src.created_at_ms === b.created_at_ms && same(src.source.bout, b.bout) && ['winner', 'method', 'end_round', 'end_tick'].every(k => src.summary[k] === b[k]));
  requireValue(src.source.card_id === s.card_id && same(src.source.event, s.event) && ['card_record_digest', 'card_source_digest', 'card_created_at_ms', 'finalized_at_ms'].every(k => src.source[k] === s.source.source[k]));
  requireValue((r.publication?.state ?? 'none') === s.publication_state && (r.publication?.txid ?? '') === s.txid && (r.inclusion?.anchor.height ?? 0) === s.inclusion_height && (r.inclusion?.anchor.hash ?? '') === s.inclusion_hash && (r.inclusion?.generation ?? 0) === s.inclusion_generation && r.prior_inclusions.length === s.prior_inclusion_count && r.conflicts.length === s.conflict_count && r.conflict_overflow === s.conflict_overflow && !!r.checkpoint === s.checkpoint_present && (r.finality?.observed_at_ms ?? 0) === s.finalized_at_ms && r.cursor.height === s.scan_height && r.cursor.generation === s.scan_generation);
  for (const k of ['chain_observed_at_ms', 'node_state', 'descendants', 'current_finality']) requireValue(r.observation[k] === s[k]); requireValue((r.observation.tip?.height ?? -1) === s.scan_tip_height);
}
function publicationResult(v) { exact(v, ['version', 'record', 'status']); requireValue(v.version === 1 && entryBytes(v) <= 229376); publicationRecordValue(v.record); publicationStatusValue(v.status); publicationMatch(v.record, v.status); return v; }
function publicationProofSummary(v) {
  exact(v, ['version', 'result_key', 'card_id', 'bout_id', 'record_digest', 'source_record_digest', 'input_id', 'result_digest', 'txid', 'sha256', 'bytes', 'header_count', 'block_count', 'finalized_at_ms', 'observed_tip']);
  requireValue(v.version === 1 && ['result_key', 'card_id', 'bout_id', 'record_digest', 'source_record_digest', 'input_id', 'result_digest', 'txid', 'sha256'].every(k => hex(v[k])) && int(v.bytes, 1, 25165824) && int(v.header_count, 1, 512) && int(v.block_count, 1, 3) && int(v.finalized_at_ms, 0, DATE_MAX) && entryBytes(v) <= 2048); seedTip(v.observed_tip); return v;
}
function publicationJobValue(j) {
  exact(j, ['version', 'job_id', 'kind', 'state', 'view', 'store_id', 'card_id', 'saturday', 'bout_id', 'result_key', 'error', 'result', 'export']); entryDateValue(j.saturday);
  requireValue(j.version === 1 && ['job_id', 'card_id', 'bout_id', 'result_key'].every(k => hex(j[k])) && ['publish', 'reconcile', 'attest', 'export'].includes(j.kind) && ['queued', 'running', 'completed', 'failed', 'cancelled'].includes(j.state) && viewKey(checkedView(j.view)) === currentKey() && j.store_id === publicationAuthority.storeID && entryBytes(j) <= RESULT_JOB_MAX);
  if (j.state === 'completed') { requireValue(j.error === ''); if (j.kind === 'export') { requireValue(j.result === null && j.export !== null); publicationProofSummary(j.export); } else { requireValue(j.export === null && j.result !== null); publicationResult(j.result); requireValue(j.saturday === j.result.status.event.saturday); } const p = j.kind === 'export' ? j.export : j.result.status; requireValue(['card_id', 'bout_id', 'result_key'].every(k => p[k] === j[k])); }
  else requireValue(j.result === null && j.export === null && (['queued', 'running'].includes(j.state) ? j.error === '' : RESULT_ERRORS.has(j.error) && j.error !== 'invalid_frame' && (j.state !== 'cancelled' || j.error === 'result_cancelled')));
  return j;
}
function publicationEnvelope(r, field) {
  exact(r, ['id', 'ok', 'view', 'store_id', 'snapshot', 'status', 'record', 'job', 'error']); requireValue(int(r.id, 1) && typeof r.ok === 'boolean' && entryBytes(r) + 1 <= 262144); const fields = ['snapshot', 'status', 'record', 'job'];
  if (!r.ok) { requireValue(r.view === null && r.store_id === '' && fields.every(k => r[k] === null) && RESULT_ERRORS.has(r.error)); return { ok: false, error: r.error }; }
  requireValue(r.error === '' && fields.every(k => k === field ? r[k] !== null : r[k] === null)); const v = r[field];
  if (field === 'snapshot') {
    exact(v, ['version', 'state', 'error', 'view', 'store_id', 'pin', 'source_created_at_ms', 'created_at_ms', 'publisher_account_id', 'node_state', 'node_owner', 'active_job_id', 'queued_job_id', 'last_job_id']);
    requireValue(v.version === 1 && ['ready', 'busy', 'setup_incomplete', 'unavailable', 'failed'].includes(v.state) && int(v.source_created_at_ms, 0, DATE_MAX) && int(v.created_at_ms, 0, DATE_MAX) && ['active_job_id', 'queued_job_id', 'last_job_id'].every(k => optionalID(v[k])) && same(r.view, v.view) && r.store_id === v.store_id);
    requireValue(['offline', 'starting', 'online'].includes(v.node_state) && (v.node_state === 'offline' ? v.node_owner === 'none' : v.node_state === 'starting' ? v.node_owner === 'publication' : ['publication', 'mining'].includes(v.node_owner)));
    if (['unavailable', 'setup_incomplete'].includes(v.state)) { requireValue(v.pin === null && v.store_id === '' && v.publisher_account_id === '' && v.source_created_at_ms === 0 && v.created_at_ms === 0 && v.node_state === 'offline' && !v.active_job_id && !v.queued_job_id && !v.last_job_id && v.error === (v.state === 'unavailable' ? 'result_unavailable' : 'result_setup_incomplete')); requireValue(v.state === 'unavailable' ? v.view === null : viewKey(checkedView(v.view)) === currentKey()); }
    else {
      requireValue(viewKey(checkedView(v.view)) === currentKey() && hex(v.store_id)); publicationPin(v.pin); requireValue(v.publisher_account_id === v.pin.publisher_account_id && v.source_created_at_ms === bookedAuthority.createdMS && int(v.created_at_ms, v.source_created_at_ms, DATE_MAX));
      const ids = [v.active_job_id, v.queued_job_id, v.last_job_id].filter(Boolean); requireValue(new Set(ids).size === ids.length && !(v.active_job_id && v.queued_job_id) && (!(v.active_job_id || v.queued_job_id) || v.state === 'busy') && (v.state === 'failed' ? ['result_unavailable', 'result_corrupt', 'result_clock', 'result_source_changed'].includes(v.error) && !v.active_job_id && !v.queued_job_id : v.error === ''));
      const a = { pin: v.pin, storeID: v.store_id, sourceCreatedMS: v.source_created_at_ms, createdMS: v.created_at_ms }; requireValue(!publicationAuthority || same(publicationAuthority, a)); if (!publicationAuthority) { v.pin.source_pin = bookedAuthority.pin; publicationAuthority = a; } v.pin = publicationAuthority.pin;
    }
    return { ok: true, value: v };
  }
  requireValue(publicationReady() && viewKey(checkedView(r.view)) === currentKey() && r.store_id === publicationAuthority.storeID);
  if (field === 'status') publicationStatusValue(v); else if (field === 'record') publicationResult(v); else publicationJobValue(v);
  return { ok: true, value: v };
}
function publicationError(code, kind = '') {
  if (code === 'outcome_unknown') return kind === 'publish' ? 'The test result may have been saved or submitted. Read saved publication and explicitly check its status before another publish request.' : kind === 'export' ? 'The proof file may have been saved. Check the selected destination before exporting again.' : kind === 'attest' ? 'The result checkpoint may have been saved. Read saved publication before another finalization request.' : 'The observation could not be confirmed. Read saved publication after reopening the gym. This check did not submit a transaction.';
  return ({ result_setup_incomplete: 'Sign in to both demo gyms to complete the shared result setup.', result_unavailable: 'Result publication is unavailable in this route. Saved bouts remain retained.', result_source_incomplete: 'Choose a saved bout with its original finalized event source.', result_source_changed: 'The saved source identity changed. Reopen the gym before using this evidence.', result_source_conflict: 'The saved source conflicts. Publishing and finalizing are blocked; the original outcome remains available.', result_conflict: 'The result has conflicting evidence. Its first transaction and checkpoint remain retained.', result_waiting_for_funds: 'The separate test publication fund has no mature unreserved output. No automatic funding was requested.', result_scan_incomplete: 'The bounded scan is incomplete. Choose Check status again to continue.', result_node_offline: 'The owned test nodes are unavailable. Another explicit network action can reconnect them.', result_not_prepared: 'No publication has been prepared. Publish test result is the separate explicit action.', result_not_found: 'No saved publication was found for this bout.', result_not_mature: 'The result needs 60 descendant blocks before finalization.', result_proof_unavailable: 'A complete proof is not available for the retained inclusion and funding history. No partial proof is saved.', result_capacity: 'The publication store or display is full. Clear displayed details to free display space; saved evidence stays retained.', result_clock: 'The local authority clock needs attention. Reopen the gym before continuing.', result_corrupt: 'Saved result evidence failed validation. Reopen the gym; no evidence was replaced.', result_cancelled: 'The request was cancelled. Read saved publication before another action.', result_calendar: 'Choose a supported Saturday in the entry panel.', forbidden: 'Publishing and finalizing require a fresh, unheld Demo A login. Held sessions can still view and check evidence.' })[code] ?? textError(code);
}
function publicationRefusal(code, kind = '', selection = null) {
  if (['unauthorized', 'stale_view'].includes(code)) { clearPublication(); publicationUnavailable = true; }
  else if (!['busy', 'invalid_request', 'result_calendar', 'result_cancelled', 'result_capacity'].includes(code)) { publicationOverlayKnown = false; publicationOverlayRead = true; }
  if (code === 'result_unavailable') publicationUnavailable = true;
  if (['result_source_changed', 'result_source_conflict'].includes(code)) { pauseBooked(); bookedClearImages(); bookedOverlayKnown = false; bookedOverlayRead = true; }
  if (code === 'outcome_unknown' && kind) publicationUnknown = { kind, date: selection?.date ?? entryDate, boutID: selection?.boutID ?? bookedSelected, jobID: selection?.id ?? '' };
  publicationMessage = publicationError(code, kind);
}
function publicationStatusInvariant(old, next) {
  if (!old || old.bout_id !== next.bout_id || old.event.saturday !== next.event.saturday) return;
  requireValue(old.result_key === next.result_key && old.card_id === next.card_id);
  if (old.exists) requireValue(next.exists && ['source_record_digest', 'input_id', 'result_digest', 'prepared_at_ms'].every(k => old[k] === next[k]));
  if (old.txid) requireValue(old.txid === next.txid);
  if (old.finalized_at_ms) requireValue(old.finalized_at_ms === next.finalized_at_ms);
  if (old.checkpoint_present) requireValue(next.checkpoint_present);
  requireValue(next.conflict_count >= old.conflict_count && (!old.conflict_overflow || next.conflict_overflow));
}
function publicationRecordInvariant(old, next, historical = false) {
  if (!old) return;
  requireValue(old.result_key === next.result_key && old.prepared_at_ms === next.prepared_at_ms && same(old.statement, next.statement));
  if (old.publication && (!historical || next.publication)) requireValue(!!next.publication && ['raw_tx', 'txid', 'coin', 'built_at_ms'].every(k => same(old.publication[k], next.publication[k])));
  for (const k of ['checkpoint', 'finality', 'equivocation']) if (old[k] && (!historical || next[k])) requireValue(same(old[k], next[k]));
  if (old.checkpoint && (!historical || next.checkpoint)) requireValue(same(old.inclusion, next.inclusion));
}
function publicationAdoptStatus(s, terminal = false) {
  const sameSelection = publicationStatus?.bout_id === s.bout_id && publicationStatus.event.saturday === s.event.saturday;
  if (terminal && (publicationOverlayRead || sameSelection && publicationStatus.observed_at_ms > s.observed_at_ms)) return false;
  publicationStatusInvariant(publicationStatus, s); requireValue(!sameSelection || s.observed_at_ms >= publicationStatus.observed_at_ms);
  publicationStatus = s; publicationOverlayKnown = true; if (!terminal) publicationOverlayRead = true;
  return true;
}
function publicationCommitBooked(terminal = false) {
  const s = publicationStatus;
  if (publicationOverlayKnown && s && bookedStatus?.saved.bout.bout_id === s.bout_id && s.source.observed_at_ms >= bookedStatus.observed_at_ms && (!terminal || !bookedOverlayRead)) {
    bookedAdoptStatus(s.source, terminal);
    if (!s.source.saved.usable) { pauseBooked(); bookedClearImages(); }
  }
}
function publicationAdoptResult(v, terminal = false) {
  const historical = terminal && (publicationOverlayRead || publicationStatus && publicationStatus.observed_at_ms > v.status.observed_at_ms);
  publicationRecordInvariant(publicationRecord, v.record, historical);
  if (historical) return;
  const accepted = publicationAdoptStatus(v.status, terminal);
  // A newer explicit observation owns the mutable record as well as its badge.
  if (accepted) { publicationRecord = v.record; publicationExport = null; }
}
function publicationSavedState() { return [publicationStatus, publicationRecord, publicationExport, publicationWatch, publicationTerminal, publicationUnknown, publicationLastJob, publicationOverlayKnown, publicationOverlayRead, bookedStatus, bookedOverlayKnown, bookedOverlayRead]; }
function publicationRestoreState(v) { [publicationStatus, publicationRecord, publicationExport, publicationWatch, publicationTerminal, publicationUnknown, publicationLastJob, publicationOverlayKnown, publicationOverlayRead, bookedStatus, bookedOverlayKnown, bookedOverlayRead] = v; }
async function fetchPublicationSnapshot() {
  if (!ready() || !bookedAuthority || publicationUnavailable) return; const c = publicationContext(), previous = [publicationAuthority, publicationSnapshot];
  try {
    const raw = await call('ResultSnapshot'); if (!publicationCurrent(c)) return; const r = publicationEnvelope(raw, 'snapshot'); if (!r.ok) { publicationRefusal(r.error); return; }
    publicationSnapshot = r.value; if (!publicationCapacity()) { [publicationAuthority, publicationSnapshot] = previous; throw publicationCacheError(); }
    if (['failed', 'unavailable'].includes(r.value.state)) { publicationRefusal(r.value.error); return; }
    const id = publicationWatch?.id || r.value.active_job_id || r.value.queued_job_id || r.value.last_job_id;
    if (id && (publicationWatch || id !== publicationLastJob)) await publicationFetchJob(id);
  } catch (e) { if (publicationCurrent(c)) { [publicationAuthority, publicationSnapshot] = previous; publicationRefusal(e.resultCode ?? 'result_unavailable'); } }
  renderPublication();
}
async function publicationRead() {
  if (!publicationReady() || !bookedSelected || !entryDate) return; const c = publicationContext();
  try {
    let r = publicationEnvelope(await call('ResultStatus', c.viewID, c.date, c.boutID), 'status'); if (!publicationCurrent(c, true)) return;
    if (!r.ok) { publicationRefusal(r.error); return; }
    requireValue(r.value.event.saturday === c.date && r.value.bout_id === c.boutID);
    const old = publicationSavedState(); publicationAdoptStatus(r.value); if (!publicationCapacity()) { publicationRestoreState(old); throw publicationCacheError(); } publicationCommitBooked(); renderBooked();
    if (r.value.exists) {
      const raw = await call('ResultRecord', c.viewID, c.date, c.boutID); if (!publicationCurrent(c, true)) return; r = publicationEnvelope(raw, 'record'); if (!r.ok) { publicationRefusal(r.error); return; }
      requireValue(r.value.status.event.saturday === c.date && r.value.status.bout_id === c.boutID); const before = publicationSavedState();
      publicationAdoptResult(r.value); if (!publicationCapacity()) { publicationRestoreState(before); throw publicationCacheError(); } publicationCommitBooked();
    }
    // Only a complete explicit read resolves an unacknowledged request. A
    // cached terminal for this bout may predate the uncertain operation.
    if (publicationUnknown?.boutID === c.boutID && publicationUnknown.date === c.date) publicationUnknown = null;
    publicationMessage = 'Saved publication read. This read did not connect nodes, check the chain or submit a transaction.';
  } catch (e) { if (publicationCurrent(c, true)) publicationRefusal(e.resultCode ?? 'outcome_unknown'); }
  renderPublication(); renderBooked();
}
function publicationCanAct(kind) {
  if (!publicationReady() || !entryDate || !bookedSelected || publicationWatch || publicationSnapshot.state === 'busy' || !['publish', 'reconcile', 'attest', 'export'].includes(kind)) return false;
  if (current?.view?.account_id !== publicationAuthority.pin.publisher_account_id) return false;
  const s = publicationStatus, selected = bookedStatus?.saved;
  if (['publish', 'attest'].includes(kind)) {
    if (current.account.held || !bookedOverlayKnown || !selected?.exists || !selected.usable || selected.bout.bout_id !== bookedSelected || publicationUnknown) return false;
    if (s && (!publicationOverlayKnown || s.phase === 'conflict' || !s.source.saved.usable)) return false;
  }
  if (kind === 'publish') return true;
  if (!s?.exists || s.bout_id !== bookedSelected || s.event.saturday !== entryDate) return false;
  if (kind === 'attest') return publicationOverlayKnown && s.inclusion_height > 0 && s.descendants >= 60 && s.node_state === 'online' && !s.finalized_at_ms;
  if (kind === 'export') return s.inclusion_height > 0;
  return true;
}
async function publicationBegin(kind) {
  if (!publicationCanAct(kind)) return; const c = publicationContext(), prior = publicationSavedState(), selected = bookedStatus;
  publicationWatch = { id: '', kind, state: 'queued', date: c.date, boutID: c.boutID, cardID: selected.card_id, resultKey: publicationStatus?.result_key ?? '' };
  if (!publicationCapacity()) { publicationWatch = prior[3]; publicationMessage = publicationError('result_capacity'); return; }
  try {
    const method = { publish: 'BeginResultPublish', reconcile: 'BeginResultReconcile', attest: 'BeginResultAttest', export: 'ExportResultProof' }[kind];
    const raw = await call(method, c.viewID, c.date, c.boutID); if (!publicationCurrent(c)) return;
    if (kind === 'export' && raw === null) { publicationWatch = prior[3]; return; }
    const r = publicationEnvelope(raw, 'job'); if (!r.ok) { publicationWatch = prior[3]; publicationRefusal(r.error, kind, c); return; }
    const j = r.value; requireValue(j.state === 'queued' && j.kind === kind && j.saturday === c.date && j.bout_id === c.boutID && j.card_id === selected.card_id && (!publicationWatch.resultKey || j.result_key === publicationWatch.resultKey));
    publicationTerminal = null; publicationWatch = { id: j.job_id, kind, state: j.state, date: j.saturday, boutID: j.bout_id, cardID: j.card_id, resultKey: j.result_key };
    // Retain prior selected evidence until a completed, correlated response.
    if (publicationCurrent(c, true)) publicationOverlayRead = false;
    publicationMessage = kind === 'publish' ? 'Test publication queued. A queued request is not confirmation of submission.' : kind === 'reconcile' ? 'Explicit chain check queued. It will not submit a transaction.' : kind === 'attest' ? 'Test result finalization queued.' : 'Public proof export queued.';
  } catch { if (publicationCurrent(c)) { publicationWatch = prior[3]; publicationRefusal('outcome_unknown', kind, c); } }
  renderPublication();
}
function publicationTerminalKey(j) { return JSON.stringify([j.job_id, j.kind, j.state, viewKey(j.view), j.store_id, j.card_id, j.saturday, j.bout_id, j.result_key, j.error, j.result?.status ?? null, j.export]); }
async function publicationFetchJob(id) {
  if (!publicationReady()) return; const c = publicationContext(), expected = publicationWatch; let rollback = null;
  try {
    const raw = await call('ResultJob', c.viewID, id); if (!publicationCurrent(c) || expected !== publicationWatch) return; const r = publicationEnvelope(raw, 'job');
    if (!r.ok) { if (r.error !== 'busy') { publicationWatch = null; publicationRefusal(r.error, expected?.kind ?? '', expected); } return; }
    const j = r.value; requireValue(j.job_id === id && (!expected || j.kind === expected.kind && j.saturday === expected.date && j.bout_id === expected.boutID && j.card_id === expected.cardID && (!expected.resultKey || j.result_key === expected.resultKey) && !(expected.state === 'running' && j.state === 'queued')));
    if (['queued', 'running'].includes(j.state)) { const prior = publicationWatch; publicationWatch = { id, kind: j.kind, state: j.state, date: j.saturday, boutID: j.bout_id, cardID: j.card_id, resultKey: j.result_key }; if (!publicationCapacity()) { publicationWatch = prior; throw publicationCacheError(); } return; }
    const key = publicationTerminalKey(j); if (publicationTerminal?.id === id) requireValue(publicationTerminal.key === key);
    rollback = publicationSavedState(); publicationWatch = null; publicationLastJob = id; publicationTerminal = { id, key };
    if (j.state === 'completed') {
      if (publicationCurrent(c, true) && j.saturday === entryDate && j.bout_id === bookedSelected) {
        if (j.kind === 'export') {
          const s = publicationStatus; requireValue(!!s && ['result_key', 'card_id', 'bout_id', 'source_record_digest', 'input_id', 'result_digest', 'txid', 'finalized_at_ms'].every(k => j.export[k] === s[k]));
          publicationExport = j.export;
        } else publicationAdoptResult(j.result, true);
      }
      if (publicationUnknown?.jobID === id && publicationUnknown.kind === j.kind && publicationUnknown.date === j.saturday && publicationUnknown.boutID === j.bout_id) publicationUnknown = null;
      publicationMessage = j.kind === 'export' ? 'The complete public proof was saved.' : j.kind === 'reconcile' ? 'Chain check completed. No transaction was submitted by this check.' : 'The test result operation completed. Its publication and finality remain separate from the saved outcome.';
    } else publicationRefusal(j.error, j.kind, { date: j.saturday, boutID: j.bout_id, id });
    if (!publicationCapacity()) throw publicationCacheError();
    publicationCommitBooked(true);
  } catch (e) { if (publicationCurrent(c)) { if (rollback) publicationRestoreState(rollback); if (e.resultCode === 'result_capacity') publicationMessage = publicationError(e.resultCode); else { publicationWatch = null; publicationRefusal('outcome_unknown', expected?.kind ?? '', expected); } } }
  renderPublication(); renderBooked();
}
async function publicationCancel() {
  if (!publicationReady() || !publicationWatch?.id) return; const c = publicationContext(), w = publicationWatch;
  try { const raw = await call('CancelResultJob', c.viewID, w.id); if (!publicationCurrent(c)) return; const r = publicationEnvelope(raw, 'job'); if (!r.ok) { publicationRefusal(r.error, w.kind, w); return; } const j = r.value; requireValue(j.job_id === w.id && j.kind === w.kind && j.saturday === w.date && j.bout_id === w.boutID && j.card_id === w.cardID && j.result_key === w.resultKey && !(w.state === 'running' && j.state === 'queued')); await publicationFetchJob(w.id); }
  catch { if (publicationCurrent(c)) publicationRefusal('outcome_unknown', w.kind, w); }
  renderPublication();
}
function publicationNodeConnected() { return !!publicationSnapshot && publicationSnapshot.node_owner === 'publication' && ['starting', 'online'].includes(publicationSnapshot.node_state); }
function publicationForSelectedBout() {
  const s = publicationStatus, b = bookedStatus?.saved, r = bookedRecord;
  if (!ready() || !publicationAuthority || !bookedAuthority || viewKey(publicationSnapshot?.view) !== currentKey() || viewKey(bookedSnapshot?.view) !== currentKey() || !s || !b?.exists) return null;
  if (s.store_id !== publicationAuthority.storeID || s.source.store_id !== bookedAuthority.storeID || s.event.saturday !== entryDate || s.bout_id !== bookedSelected || s.card_id !== bookedStatus.card_id || !same(s.source.saved.bout, b.bout)) return null;
  if (!['record_digest', 'source_digest', 'input_id', 'result_digest', 'created_at_ms'].every(k => s.source.saved[k] === b[k])) return null;
  if (s.exists && (s.source_record_digest !== b.record_digest || s.input_id !== b.input_id || s.result_digest !== b.result_digest)) return null;
  if (r && (r.store_id !== bookedAuthority.storeID || r.card_id !== s.card_id || r.bout_id !== s.bout_id || !['source_digest', 'input_id', 'result_digest', 'created_at_ms'].every(k => r[k] === b[k]))) return null;
  return s;
}
// Historical result finality survives later conflict or an unavailable fresh
// observation. Seed finality, connection state and the phase badge cannot grant it.
function publicationOutcomeLabel(s) { return s?.finalized_at_ms > 0 ? 'Finalized result' : 'Provisional result'; }
function renderPublication() {
  const active = publicationReady(), s = publicationForSelectedBout(), busy = !!command || !!publicationWatch || publicationSnapshot?.state === 'busy';
  const selected = bookedStatus?.saved, connected = publicationNodeConnected();
  ui['result-panel'].hidden = publicationUnavailable;
  const outcome = publicationOutcomeLabel(s);
  content(ui['result-state'], !ready() ? 'Sign in to view publication' : publicationSnapshot?.state === 'setup_incomplete' ? 'Setup incomplete' : !selected?.exists ? 'Choose a saved bout' : `${outcome} · ${!s ? 'Publication not read' : s.phase === 'absent' ? 'Saved outcome · unpublished' : s.phase === 'conflict' ? 'Conflicting evidence' : s.finalized_at_ms ? 'Historical test finality' : s.inclusion_height ? 'Included' : s.publication_state === 'dispatch_unknown' ? 'Submission uncertain' : s.publication_state === 'submitted' ? 'Submitted · not included' : 'Prepared · not published'}`);
  content(ui['result-connection'], publicationSnapshot?.node_state === 'starting' ? 'Connecting test nodes. Mining remains stopped.' : publicationSnapshot?.node_state === 'online' ? publicationSnapshot.node_owner === 'publication' ? 'Test nodes connected for publication. No mining or admission worker is running for this connection.' : 'Using the existing mining connection.' : 'Test nodes are disconnected. An explicit publication action can connect Demo A’s nodes.');
  content(ui['result-observation'], s ? `${s.chain_observed_at_ms ? `Last chain check ${date.format(s.chain_observed_at_ms)}.` : 'No chain inclusion check is retained.'} Local evidence read ${date.format(s.observed_at_ms)}. Disconnection does not erase historical evidence.` : 'Read saved publication to inspect retained evidence without connecting nodes.');
  content(ui['result-message'], publicationUnknown ? `${publicationError('outcome_unknown', publicationUnknown.kind)} The pending request belongs to Saturday ${publicationUnknown.date}.` : publicationMessage);
  content(ui['result-finality'], s?.finalized_at_ms ? `First test finality observed ${date.format(s.finalized_at_ms)}. ${s.current_finality ? 'The inclusion met the depth rule at its last chain check.' : 'Current chain finality is not confirmed by the last check.'}` : s?.inclusion_height ? `${s.descendants} descendant blocks observed for this result. ${s.descendants < 60 ? '60 are required, followed by its own signed test checkpoint.' : 'The depth threshold is reached; finalization is a separate explicit action.'}` : 'A finalized event seed does not finalize this result. Signed preparation and submission are not chain inclusion.');
  const ownConflict = s && (s.conflict_count > 0 || s.conflict_overflow), sourceConflict = s?.exists && !s.source.saved.usable;
  content(ui['result-conflict'], !publicationOverlayKnown && s ? 'Current source usability is unconfirmed. Read saved publication before signing or publishing.' : [ownConflict ? 'Result conflict is retained. The first transaction, certificate and historical finality are preserved.' : '', sourceConflict ? 'The saved source is currently unusable. Publishing and finalizing are blocked even if an earlier chain check met the finality rule.' : ''].filter(Boolean).join(' '));
  content(ui['result-exported'], publicationExport ? `Public proof saved: ${number.format(publicationExport.bytes)} bytes, ${publicationExport.header_count} headers and ${publicationExport.block_count} full blocks. No wallet recovery material is included.` : '');
  content(ui['result-evidence'], s?.exists ? `Publication ${s.result_key} · Transaction ${s.txid || 'not built'} · Saved outcome ${s.result_digest}${publicationExport ? ` · File SHA256 ${publicationExport.sha256}` : ''}` : '');
  ui['result-read'].disabled = !active || busy || !bookedSelected;
  for (const kind of ['publish', 'reconcile', 'attest', 'export']) ui[`result-${kind}`].disabled = busy || !publicationCanAct(kind);
  ui['result-cancel'].disabled = !active || !!command || !publicationWatch?.id;
  ui['result-disconnect'].disabled = !active || !!command || !connected;
  ui['result-clear'].disabled = !publicationRecord && !publicationStatus && !publicationExport;
  requireValue(entryBytes([ui['result-evidence'].textContent, ui['result-state'].textContent]) <= 4096);
}

// Finalized history is a captured read. Native owns source verification,
// cryptographic identities, original authority and cross-page commitments.
const HISTORY_ERRORS = new Set(['invalid_request', 'unauthorized', 'forbidden', 'stale_view', 'busy', 'outcome_unknown', 'career_inactive', 'history_unavailable', 'history_corrupt', 'history_clock', 'history_capacity', 'history_not_found', 'history_source_changed', 'history_cancelled', 'history_observation_unknown']);
const HISTORY_TOTALS = ['wins', 'losses', 'draws', 'xp', 'base_points'];
let historySnapshot = null, historySelection = null, historyJob = null, historyPage = null, historyWatch = '', historyUnknown = false, historyMessage = '', historyOverlay = null, historyIntent = null, historySerial = 0;
function historyMetadataBytes() { return entryBytes([historySnapshot, historySelection, historyJob, historyWatch, historyUnknown, historyMessage, historyOverlay, historyIntent]) + 512; }
function historyRetainedBytes() { return historyWatch || historyUnknown ? 49152 : historySelection || historySnapshot ? historyMetadataBytes() + (historyPage ? entryBytes(historyPage) : 0) : 0; }
function historyCapacity() { return historyMetadataBytes() <= 16384 && (!historyPage || entryBytes(historyPage) <= 32768) && previewCapacity(); }
function clearHistory() { historySerial++; historySnapshot = historySelection = historyJob = historyPage = historyOverlay = historyIntent = null; historyWatch = historyMessage = ''; historyUnknown = false; }
function historyContext() { return {epoch, view: currentKey(), serial: historySerial, viewID: current?.view?.view_id}; }
function historyCurrent(c) { return !disposed && ready() && c.epoch === epoch && c.view === currentKey() && c.serial === historySerial; }
function historyError(code) { return ({history_not_found: 'This captured history is no longer selected. Open a new history from the fighter’s card.', history_source_changed: 'The source changed. Open a new history from the fighter’s card.', history_corrupt: 'History evidence could not be validated.', history_clock: 'The authority clock needs attention.', history_unavailable: 'History is currently unavailable.', history_capacity: 'The display or history cache is full. Clear a detail before trying again.', history_cancelled: 'The history request was cancelled.', history_observation_unknown: 'The source observation is uncertain. History is unavailable until the original store is recovered.', outcome_unknown: 'The request may have started. Check its recorded state before starting another history.'})[code] ?? textError(code); }
function historyRefusal(code) {
  historyMessage = historyError(code);
  if (['unauthorized', 'stale_view', 'forbidden'].includes(code)) { clearHistory(); historyMessage = historyError(code); }
  else if (['history_unavailable', 'history_corrupt', 'history_clock', 'history_observation_unknown', 'career_inactive'].includes(code)) { historySerial++; historySnapshot = historyJob = historyPage = historyOverlay = historyIntent = historySelection = null; historyWatch = ''; historyUnknown = false; }
  else if (['history_source_changed', 'history_not_found'].includes(code)) { historySerial++; historyJob = historyPage = historyOverlay = historyIntent = historySelection = null; historyWatch = ''; historyUnknown = false; }
  else if (code === 'outcome_unknown') historyUnknown = true;
}
function historyActivation(a) {
  exact(a, ['version','policy_id','activation_id','result_store_id','result_created_at_ms','source_created_at_ms','activated_at_ms']);
  requireValue(a.version === 1 && a.policy_id === CAREER_POLICY && hex(a.activation_id) && hex(a.result_store_id) && int(a.source_created_at_ms,1,DATE_MAX) && int(a.result_created_at_ms,a.source_created_at_ms,DATE_MAX) && int(a.activated_at_ms,a.result_created_at_ms,DATE_MAX));
  requireValue(!careerActivation || same(careerActivation,a));
  if (historySnapshot?.activation) requireValue(same(historySnapshot.activation,a));
  if (publicationAuthority) requireValue(a.result_store_id === publicationAuthority.storeID && a.result_created_at_ms === publicationAuthority.createdMS && a.source_created_at_ms === bookedAuthority?.createdMS);
}
function historyTotals(t, count) {
  exact(t,HISTORY_TOTALS); const n=t.wins+t.losses+t.draws;
  requireValue(['wins','losses','draws'].every(k=>int(t[k],0,256)) && n<=256 && (count===undefined || n===count) && t.xp===Math.min(15,n) && int(t.base_points,10*t.wins+4*t.draws,14*t.wins+4*t.draws) && (t.base_points-10*t.wins-4*t.draws)%4===0);
}
function historyCapture(c) {
  exact(c,['version','status','activation','career_digest','as_of_ms','assignment','prefix_count','event_count','totals','history_id']);
  requireValue(c.version===1 && hex(c.career_digest) && hex(c.history_id) && entryBytes(c)<=8192 && entryBytes(c.status)<=4096);
  status(c.status,current.view,current.account.fixture); historyActivation(c.activation); assignment(c.assignment,c.status.pin.identity);
  requireValue(c.status.store_id===stable.status.store_id && same(c.status.pin,stable.status.pin) && c.assignment.sequence<=c.status.checkpoint && int(c.as_of_ms,Math.max(c.activation.activated_at_ms,c.assignment.accepted_at_ms),DATE_MAX) && c.status.last_as_of_ms===c.as_of_ms && int(c.prefix_count,0,256) && int(c.event_count,0,c.prefix_count));
  historyTotals(c.totals,c.event_count); return c;
}
function historyRow(row,c) {
  exact(row,['ordinal','event','earned','xp_before','xp_applied','totals_after']); requireValue(int(row.ordinal,1,c.event_count) && entryBytes(row)<=1536);
  const e=row.event, pid=c.assignment.prospect_id;
  exact(e,['version','award_id','result_key','card_id','bout_id','source_record_digest','input_id','result_digest','finality_digest','effective_at_ms','saturday','corner_a','corner_b','winner','method']);
  requireValue(e.version===1 && ['award_id','result_key','card_id','bout_id','source_record_digest','input_id','result_digest','finality_digest'].every(k=>hex(e[k])) && int(e.effective_at_ms,c.activation.activated_at_ms,c.as_of_ms) && entryBytes(e)<=1344);
  requireValue(typeof e.saturday==='string' && /^[0-9]{4}-[0-9]{2}-[0-9]{2}$/.test(e.saturday)); const day=new Date(`${e.saturday}T00:00:00.000Z`); requireValue(!Number.isNaN(day.getTime()) && day.getUTCFullYear()>=1 && day.getUTCDay()===6 && day.toISOString().slice(0,10)===e.saturday);
  for(const corner of [e.corner_a,e.corner_b]) { exact(corner,['prospect_id','account_id','gym_id']); requireValue(Object.values(corner).every(hex)); }
  requireValue(['prospect_id','account_id','gym_id'].every(k=>e.corner_a[k]!==e.corner_b[k]));
  const corner=e.corner_a.prospect_id===pid?e.corner_a:e.corner_b; requireValue(corner.prospect_id===pid && corner.account_id===c.status.pin.account_id && corner.gym_id===c.assignment.gym_id);
  const draw=['decision_draw','double_corner_stoppage'].includes(e.method); requireValue(FINISHES.includes(e.method) && (draw?e.winner==='':[e.corner_a.prospect_id,e.corner_b.prospect_id].includes(e.winner)));
  const won=e.winner===pid, earned={wins:won?1:0,losses:!draw&&!won?1:0,draws:draw?1:0,xp:1,base_points:draw?4:won?(e.method==='decision'?10:14):0};
  exact(row.earned,HISTORY_TOTALS); requireValue(same(row.earned,earned) && row.xp_before===Math.min(15,row.ordinal-1) && row.xp_applied===Math.min(15,row.ordinal)-row.xp_before);
  historyTotals(row.totals_after,row.ordinal); const prior={...row.totals_after,xp:row.xp_before}; for(const k of ['wins','losses','draws','base_points'])prior[k]-=earned[k]; historyTotals(prior,row.ordinal-1);
  if(row.ordinal===c.event_count)requireValue(same(row.totals_after,c.totals)); return row;
}
function historyPageValue(p,after,limit) {
  exact(p,['version','capture','after','next','has_more','items']); requireValue(p.version===1 && entryBytes(p)<=32768); const c=historyCapture(p.capture);
  requireValue(historyJob?.state==='done' && same(c,historyJob.capture) && p.after===after && int(after,0,c.event_count) && int(limit,1,8) && Array.isArray(p.items) && p.items.length===Math.min(limit,c.event_count-after) && p.next===after+p.items.length && p.has_more===(p.next<c.event_count));
  const awards=new Set(),results=new Set();
  p.items.forEach((r,n)=>{historyRow(r,c);requireValue(r.ordinal===c.event_count-after-n && !awards.has(r.event.award_id) && !results.has(r.event.result_key));awards.add(r.event.award_id);results.add(r.event.result_key);
    if(n){const newer=p.items[n-1];requireValue(r.event.effective_at_ms<newer.event.effective_at_ms || r.event.effective_at_ms===newer.event.effective_at_ms && r.event.award_id<newer.event.award_id);for(const k of ['wins','losses','draws','base_points'])requireValue(r.totals_after[k]+newer.earned[k]===newer.totals_after[k]);requireValue(Math.min(15,r.totals_after.xp+newer.earned.xp)===newer.totals_after.xp);}
  }); return p;
}
function historyPins(v) { requireValue(viewKey(checkedView(v.view))===currentKey() && v.store_id===stable?.status.store_id); }
function historyEnvelope(v,kind,after=0,limit=8) {
  exact(v,['id','version','ok','snapshot','job','page','error']); requireValue(int(v.id,1) && v.version===1 && typeof v.ok==='boolean' && entryBytes(v)<=36863);
  const arms=['snapshot','job','page'];
  if(!v.ok){requireValue(arms.every(k=>v[k]===null) && HISTORY_ERRORS.has(v.error));return {ok:false,error:v.error};}
  requireValue(v.error==='' && arms.every(k=>k===kind?v[k]!==null:v[k]===null)); const x=v[kind];
  if(kind==='snapshot') {
    exact(x,['version','state','error','view','store_id','activation','active_job_id','queued_job_id','last_job_id']); requireValue(x.version===1 && entryBytes(x)<=4096 && ['active_job_id','queued_job_id','last_job_id'].every(k=>optionalID(x[k])));
    if(x.state==='unavailable')requireValue(x.view===null && x.activation===null && x.store_id==='' && ['active_job_id','queued_job_id','last_job_id'].every(k=>x[k]==='') && ['history_unavailable','career_inactive','history_clock','history_corrupt'].includes(x.error));
    else {historyPins(x);historyActivation(x.activation);requireValue(['idle','busy','ready','failed','cancelled'].includes(x.state));
      if(x.state==='busy')requireValue(!!x.active_job_id!==!!x.queued_job_id && x.last_job_id==='' && x.error==='');
      else {requireValue(!x.active_job_id&&!x.queued_job_id);if(x.state==='idle')requireValue(!x.last_job_id&&!x.error);else {requireValue(hex(x.last_job_id));if(x.state==='ready')requireValue(!x.error);else requireValue(x.state==='cancelled'?x.error==='history_cancelled':HISTORY_ERRORS.has(x.error)&&!['busy','outcome_unknown','history_cancelled'].includes(x.error));}}
    }
  } else if(kind==='job') {
    exact(x,['version','job_id','state','view','store_id','activation_id','prospect_id','error','capture']); historyPins(x);
    requireValue(x.version===1 && ['job_id','activation_id','prospect_id'].every(k=>hex(x[k])) && x.activation_id===historySnapshot?.activation?.activation_id && ['queued','running','done','failed','cancelled'].includes(x.state) && entryBytes(x)<=(x.capture?12288:2048));
    if(x.state==='done'){const c=historyCapture(x.capture);requireValue(x.error===''&&c.activation.activation_id===x.activation_id&&c.assignment.prospect_id===x.prospect_id);}
    else {requireValue(x.capture===null);if(['queued','running'].includes(x.state))requireValue(x.error==='');else requireValue(x.state==='cancelled'?x.error==='history_cancelled':HISTORY_ERRORS.has(x.error)&&!['busy','outcome_unknown','history_cancelled'].includes(x.error));}
    if(historyJob?.job_id===x.job_id){requireValue(!(['done','failed','cancelled'].includes(historyJob.state))||same(historyJob,x));requireValue(!(historyJob.state==='running'&&x.state==='queued'));}
  } else {
    exact(x,['view','store_id','job_id','page']); historyPins(x); requireValue(x.job_id===historyJob?.job_id); historyPageValue(x.page,after,limit);
  }
  return {ok:true,value:own(x)};
}
async function fetchHistorySnapshot() {
  if(!ready()||!careerMode)return;const c=historyContext();
  try {const r=historyEnvelope(await call('CareerHistorySnapshot'),'snapshot');if(!historyCurrent(c))return;if(!r.ok){historyRefusal(r.error);return;}
    const previous=historySnapshot;historySnapshot=r.value;if(!historyCapacity()){historySnapshot=previous;historyRefusal('history_capacity');return;}
    if(r.value.state==='unavailable'){historyRefusal(r.value.error);return;}
    const id=r.value.active_job_id||r.value.queued_job_id||r.value.last_job_id;
    if(id&&(historyUnknown||historyWatch||id!==historyJob?.job_id))await fetchHistoryJob(id);
    else if(historyUnknown&&!id){historyUnknown=false;historyIntent=null;historyMessage='No history request is retained. Choose View finalized history to start a new capture.';}
  } catch {if(historyCurrent(c))historyRefusal('outcome_unknown');} finally {renderHistory();}
}
async function selectHistory(i) {
  if(!ready()||!careerMode||command||historyWatch||historyUnknown)return;
  ui['history-panel'].scrollIntoView?.({block:'start',behavior:motionPreference?.matches?'auto':'smooth'});document.getElementById('history-heading').focus?.({preventScroll:true});
  const chosen={id:i.assignment.prospect_id,name:i.manifest.name.display,assignment:own(i.assignment)}, c=historyContext();
  queue(async()=>{
    if(!historyCurrent(c))return;
    await fetchHistorySnapshot(); if(!historyCurrent(c)||!historySnapshot?.activation||historyWatch||historyUnknown)return;
    if(!previewCapacity(Math.max(0,49152-historyRetainedBytes()))){historyRefusal('history_capacity');return;}historyIntent=chosen;
    try {const r=historyEnvelope(await call('BeginCareerHistory',c.viewID,chosen.id),'job');if(!historyCurrent(c))return;if(!r.ok){historyIntent=null;historyRefusal(r.error);return;}
      requireValue(r.value.state==='queued'&&r.value.prospect_id===chosen.id);historySerial++;historySelection=chosen;historyJob=r.value;historyPage=historyOverlay=null;historyWatch=r.value.job_id;historyUnknown=false;historyIntent=null;historyMessage='Capturing finalized history...';
    } catch {if(historyCurrent(c)){historyUnknown=true;historyMessage=historyError('outcome_unknown');}}
    renderHistory();
  },'history-begin');
}
async function fetchHistoryJob(id) {
  if(!ready()||!historySnapshot?.activation)return;const c=historyContext();
  try {const r=historyEnvelope(await call('CareerHistoryJob',c.viewID,id),'job');if(!historyCurrent(c))return;if(!r.ok){historyRefusal(r.error);return;}
    const j=r.value;requireValue(j.job_id===id); if(j.state==='done'&&(historyIntent||historySelection)?.assignment)requireValue(same((historyIntent||historySelection).assignment,j.capture.assignment)); if((historyIntent||historySelection)&&j.prospect_id!==(historyIntent||historySelection).id){historyUnknown=false;historyIntent=null;historyMessage='A different history request is retained. Choose this fighter again to replace it.';return;}
    const before=[historyJob,historyWatch,historyUnknown,historySelection,historyPage,historyOverlay,historyIntent];if(historyIntent){historySelection=historyIntent;historyPage=historyOverlay=null;}historyJob=j;historyWatch=['queued','running'].includes(j.state)?id:'';historyUnknown=false;historyIntent=null;
    if(!historySelection)historySelection={id:j.prospect_id,name:'Selected fighter',assignment:j.capture?.assignment??null};
    if(!historyCapacity()){[historyJob,historyWatch,historyUnknown,historySelection,historyPage,historyOverlay,historyIntent]=before;historyRefusal('history_capacity');return;}
    if(j.state==='done'){historyMessage='Finalized history captured. New awards appear in a new capture.';if(!historyPage)await readHistoryPage(0);}
    else if(!historyWatch)historyRefusal(j.error);
  } catch {if(historyCurrent(c))historyRefusal('outcome_unknown');} finally {renderHistory();}
}
async function readHistoryPage(after) {
  if(!ready()||historyJob?.state!=='done'||historyUnknown)return;const c=historyContext(),jid=historyJob.job_id;
  try {const raw=await call('CareerHistoryPage',c.viewID,jid,after,8);if(!historyCurrent(c)||historyJob?.job_id!==jid)return;const r=historyEnvelope(raw,'page',after,8);if(!r.ok){historyRefusal(r.error);return;}
    const before=[historyPage,historyOverlay];historyPage=r.value.page;historyOverlay=null;if(!historyCapacity()){[historyPage,historyOverlay]=before;historyRefusal('history_capacity');return;}historyMessage='';
  } catch {if(historyCurrent(c))historyMessage='The page could not be validated. The previous captured page is retained.';} finally {renderHistory();}
}
async function cancelHistory() {
  if(!historyWatch||!ready())return;const c=historyContext(),id=historyWatch;
  try {const r=historyEnvelope(await call('CancelCareerHistory',c.viewID,id),'job');if(!historyCurrent(c))return;if(!r.ok){historyRefusal(r.error);return;}requireValue(r.value.job_id===id);await fetchHistoryJob(id);}
  catch {if(historyCurrent(c))historyRefusal('outcome_unknown');}finally {renderHistory();}
}
async function historyReadResult(ordinal) {
  if(!ready()||!historyPage||historyUnknown)return;const row=historyPage.items.find(r=>r.ordinal===ordinal);if(!row)return;const c=historyContext(),jid=historyJob.job_id,e=row.event;
  historyOverlay=null;
  try {const r=await call('CareerHistoryResult',c.viewID,jid,ordinal);if(!historyCurrent(c)||historyJob?.job_id!==jid||!historyPage?.items.some(x=>x.ordinal===ordinal))return;
    const checked=publicationEnvelope(r,'record');
    if(!checked.ok){if(['unauthorized','stale_view','forbidden'].includes(checked.error))historyRefusal(checked.error);else historyMessage='Current result detail is unavailable. The captured finalized award is retained.';return;}
    requireValue(r.store_id===historyJob.capture.activation.result_store_id);
    const value=checked.value,rec=value.record,s=value.status,src=rec.statement.statement.source,b=src.source.bout;
    requireValue(rec.result_key===e.result_key&&src.record_digest===e.source_record_digest&&src.input_id===e.input_id&&src.result_digest===e.result_digest&&src.source.card_id===e.card_id&&b.bout_id===e.bout_id&&b.corner_a===e.corner_a.prospect_id&&b.corner_b===e.corner_b.prospect_id&&src.summary.winner===e.winner&&src.summary.method===e.method&&s.event.saturday===e.saturday&&rec.finality?.observed_at_ms===e.effective_at_ms);
    // The native link also checks the retained first-Finality digest and original
    // corner/account identities. Only this small fresh overlay is retained here.
    historyOverlay={ordinal,observed_at_ms:s.observed_at_ms,chain_observed_at_ms:s.chain_observed_at_ms,conflict:s.conflict_count>0||s.conflict_overflow||!s.source.saved.usable,current_finality:s.current_finality,result_key:e.result_key};requireValue(historyCapacity());historyMessage='Saved result read. This action did not connect nodes or check the chain.';
  }catch {if(historyCurrent(c)){historyOverlay=null;historyMessage='Current result detail could not be matched to the captured award. The award is retained.';}}finally {renderHistory();}
}
function renderHistory() {
  ui['history-panel'].hidden=!careerMode;const c=historyJob?.capture,p=historyPage,busy=!!command||!!historyWatch,active=ready()&&careerMode;
  content(ui['history-state'],!active?'Sign in to view history':historyUnknown?'Request needs reconciliation':historyWatch?'Capturing history':c?'Captured finalized results':'Choose a fighter');
  content(ui['history-fighter'],historySelection?`${historySelection.name} · ${historySelection.id}`:'Use “View finalized history” on a fighter’s card.');
  content(ui['history-asof'],c?`Captured ${date.format(c.as_of_ms)}. Later awards do not change these pages.`:'');
  content(ui['history-totals'],c?`${c.totals.wins} wins · ${c.totals.losses} losses · ${c.totals.draws} draws · Applied professional XP ${c.totals.xp}/15 · Base points (not ranking): ${c.totals.base_points}`:'');
  content(ui['history-message'],historyMessage);content(ui['history-page-label'],p?`${p.items.length?p.after+1:0}–${p.next} of ${p.capture.event_count}`:'');
  const renderedKey=[historyJob?.job_id,p?.capture.history_id,p?.after,p?.next,active,busy,historyUnknown].join('|');
  if(ui['history-list']._historyKey!==renderedKey){ui['history-list']._historyKey=renderedKey;
  ui['history-list'].textContent='';if(p&&!p.capture.event_count)ui['history-list'].append(element('li','','No finalized fights at this capture.'));
  for(const row of p?.items??[]){const e=row.event,pid=p.capture.assignment.prospect_id,opponent=(e.corner_a.prospect_id===pid?e.corner_b:e.corner_a).prospect_id,outcome=e.winner===pid?'Win':e.winner?'Loss':'Draw';
    const li=element('li'),open=element('button','','Read current result');open.type='button';open.disabled=!active||busy||historyUnknown;open.setAttribute('aria-label',`Read current result for ${e.saturday}, fight ${row.ordinal}`);open.addEventListener('click',()=>queue(()=>historyReadResult(row.ordinal),'history-result'));
    li.append(element('strong','',`${e.saturday} · ${outcome} · ${label(e.method)}`),element('p','note',`Opponent ${opponent}`),element('p','note',`Finalized ${date.format(e.effective_at_ms)} · Earned XP ${row.earned.xp} · Applied XP ${row.xp_applied} · Base points +${row.earned.base_points}`),open);ui['history-list'].append(li);
  }}
  const o=historyOverlay;content(ui['history-result'],o?`Fight ${o.ordinal}: ${o.conflict?'Conflict reported; finalized award retained.':'No conflict in this saved result read.'} Local evidence read ${date.format(o.observed_at_ms)}. ${o.chain_observed_at_ms?`Chain last checked ${date.format(o.chain_observed_at_ms)}; ${o.current_finality?'finality met at that check.':'current finality was not confirmed.'}`:'Current chain status not checked.'}`:'Current status not checked.');
  ui['history-previous'].disabled=!active||busy||historyUnknown||!p||p.after===0;ui['history-next'].disabled=!active||busy||historyUnknown||!p?.has_more;
  ui['history-refresh'].disabled=!active||!!command||(!historySelection&&!historyUnknown);ui['history-cancel'].disabled=!active||!!command||!historyWatch;ui['history-clear'].disabled=busy||(!historySelection&&!historyPage);
}

for (const kind of ['publish', 'reconcile', 'attest', 'export']) ui[`result-${kind}`].addEventListener('click', () => { const c = publicationContext(); queue(() => publicationCurrent(c, true) ? publicationBegin(kind) : undefined, `result-${kind}`); });
ui['result-read'].addEventListener('click', () => { const c = publicationContext(); queue(() => publicationCurrent(c, true) ? publicationRead() : undefined, 'result-read'); });
ui['result-cancel'].addEventListener('click', () => queue(publicationCancel, 'result-cancel'));
ui['result-disconnect'].addEventListener('click', () => oldAction('Stop'));
ui['result-clear'].addEventListener('click', () => { publicationSelect(); renderBooked(); });

ui['history-refresh'].addEventListener('click',()=>queue(fetchHistorySnapshot,'history-status'));
ui['history-cancel'].addEventListener('click',()=>queue(cancelHistory,'history-cancel'));
ui['history-previous'].addEventListener('click',()=>queue(()=>readHistoryPage(Math.max(0,(historyPage?.after??0)-8)),'history-page'));
ui['history-next'].addEventListener('click',()=>queue(()=>historyPage?.has_more?readHistoryPage(historyPage.next):undefined,'history-page'));
ui['history-clear'].addEventListener('click',()=>{clearHistory();renderHistory();});

ui['event-refresh'].addEventListener('click', () => queueEventSelection(() => eventReadStatus(), 'event-status'));
ui['event-capture'].addEventListener('click', () => queueEventSelection(eventBegin, 'event-capture'));
ui['event-cancel'].addEventListener('click', () => queue(eventCancel, 'event-cancel'));
ui['event-record'].addEventListener('click', () => queueEventSelection(eventReadRecord, 'event-record'));
ui['event-clear'].addEventListener('click', () => { eventDetail = null; renderEvent(); });
ui['event-history'].addEventListener('click', () => queue(() => eventReadHistory(), 'event-history'));
ui['event-history-next'].addEventListener('click', () => queue(() => eventReadHistory(eventHistory?.next_event_id ?? ''), 'event-history'));

for (const kind of ['prepare', 'publish', 'reconcile', 'attest']) ui[`seed-${kind}`].addEventListener('click', () => { const c = seedContext(); queue(() => seedCurrent(c, true) ? seedBegin(kind) : undefined, `seed-${kind}`); });
ui['seed-refresh'].addEventListener('click', () => { const c = seedContext(); queue(() => seedCurrent(c, true) ? seedRead('status') : undefined, 'seed-status'); });
ui['seed-record'].addEventListener('click', () => { const c = seedContext(); queue(() => seedCurrent(c, true) ? seedRead('record') : undefined, 'seed-record'); });
ui['seed-cancel'].addEventListener('click', () => queue(seedCancel, 'seed-cancel'));
ui['seed-clear'].addEventListener('click', () => { seedDetail = seedHistory = null; renderSeed(); });
ui['seed-history'].addEventListener('click', () => queue(() => seedRead('history'), 'seed-history'));
ui['seed-history-next'].addEventListener('click', () => queue(() => seedRead('history', seedHistory?.next_event_key ?? ''), 'seed-history'));
for (const kind of ['status', 'record']) ui[`card-${kind === 'status' ? 'refresh' : kind}`].addEventListener('click', () => { const c = cardContext(); queue(() => cardCurrent(c, true) ? cardRead(kind) : undefined, `card-${kind}`); });
ui['card-begin'].addEventListener('click', () => { const c = cardContext(); queue(() => cardCurrent(c, true) ? cardBegin() : undefined, 'card-build'); });
ui['card-cancel'].addEventListener('click', () => queue(cardCancel, 'card-cancel'));
ui['card-clear'].addEventListener('click', () => { cardDetail = cardHistory = null; renderCard(); });
ui['card-history'].addEventListener('click', () => queue(() => cardRead('history'), 'card-history'));
ui['card-history-next'].addEventListener('click', () => { const c = cardContext(), after = cardHistory?.next_card_id ?? ''; queue(() => cardCurrent(c) && after ? cardRead('history', after) : undefined, 'card-history'); });

ui['entry-date'].addEventListener('change', () => entryNavigate(ui['entry-date'].value));
ui['entry-upcoming'].addEventListener('click', () => { entryChanged(); queue(() => entryReadStatus(true), 'entry-upcoming'); });
ui['entry-status'].addEventListener('click', () => queue(() => entryReadStatus(), 'entry-status'));
ui['entry-save'].addEventListener('click', () => queue(() => entrySave('replace'), 'entry-save'));
ui['entry-withdraw'].addEventListener('click', () => queue(() => entrySave('withdraw'), 'entry-withdraw'));
ui['entry-check'].addEventListener('click', () => queue(() => entryBegin('check'), 'entry-check'));
ui['entry-clear'].addEventListener('click', () => { entryChanged(); entryDraft = []; render(); });
ui['entry-cancel'].addEventListener('click', () => queue(entryCancel, 'entry-cancel'));
ui['entry-reconcile'].addEventListener('click', () => queue(() => entryQuery(), 'entry-query'));
ui['entry-apply'].addEventListener('click', () => queue(() => entryBegin('apply', null, entryOperation), 'entry-apply'));
ui['entry-abandon'].addEventListener('click', () => queue(entryAbandon, 'entry-abandon'));
ui['entry-history'].addEventListener('click', () => queue(() => entryReadHistory(), 'entry-history'));
ui['entry-history-next'].addEventListener('click', () => queue(() => entryReadHistory(entryHistory?.next_event_id ?? ''), 'entry-history'));

for (const [id, kind] of [['bout-refresh', 'card_status'], ['bout-open', 'record']]) ui[id].addEventListener('click', () => { const c = bookedContext(); queue(() => bookedCurrent(c, true) ? bookedRead(kind) : undefined, `bout-${kind}`); });
ui['bout-check'].addEventListener('click', bookedCheck);
ui['bout-run'].addEventListener('click', () => { const c = bookedContext(); queue(() => bookedCurrent(c, true) ? bookedBegin() : undefined, 'bout-run'); });
ui['bout-cancel'].addEventListener('click', () => queue(bookedCancel, 'bout-cancel'));
ui['bout-clear'].addEventListener('click', () => { publicationSelect(); bookedSerial++; bookedClearRecord(); bookedPage = bookedStatus = null; bookedSelected = ''; bookedOverlayKnown = bookedOverlayRead = false; renderBooked(); });
ui['bout-play'].addEventListener('click', playBooked); ui['bout-previous'].addEventListener('click', () => stepBooked(bookedStep - 1)); ui['bout-next'].addEventListener('click', () => stepBooked(bookedStep + 1)); ui['bout-restart'].addEventListener('click', () => stepBooked(0)); ui['bout-result'].addEventListener('click', () => stepBooked(bookedTicks()));

ui['preview-begin'].addEventListener('click', () => queue(async () => { try { await beginPreview(); } catch { discardPreviewResult(); previewMessage = previewError('outcome_unknown'); } }, 'preview'));
ui['preview-cancel'].addEventListener('click', () => queue(async () => { try { await cancelPreview(); } catch { previewMessage = previewError('preview_unavailable'); } }, 'preview-cancel'));
ui['preview-clear'].addEventListener('click', () => selectionChanged([]));
ui['preview-play'].addEventListener('click', playPreview); ui['preview-previous'].addEventListener('click', () => stepPreview(playbackIndex - 1)); ui['preview-next'].addEventListener('click', () => stepPreview(playbackIndex + 1)); ui['preview-restart'].addEventListener('click', () => stepPreview(0)); ui['preview-result'].addEventListener('click', () => stepPreview(playbackSteps.length));
ui['login-a'].addEventListener('click', () => oldAction('LoginFixture', ['zero256']));
ui['login-b'].addEventListener('click', () => oldAction('LoginFixture', ['incrementing256']));
ui.logout.addEventListener('click', () => oldAction('Logout')); ui.reset.addEventListener('click', () => oldAction('ResetFixtureAccount'));
ui.mining.addEventListener('click', () => oldAction(['starting', 'running'].includes(current?.state) || ['signing_in', 'resetting'].includes(current?.account.state) ? 'Stop' : 'Start'));
ui['pause-sync'].addEventListener('click', () => oldAction('Stop'));
ui.sync.addEventListener('click', () => queue(() => beginJob('sync'), 'sync'));
ui.recent.addEventListener('click', () => navigate('recent')); ui.inventory.addEventListener('click', () => navigate('inventory')); ui['roster-details'].addEventListener('click', () => navigate('roster', 1));
ui.refresh.addEventListener('click', () => navigate(mode, mode === 'roster' ? rosterFirst : pageAfter));
ui.previous.addEventListener('click', () => navigate(mode, mode === 'roster' ? 1 : Math.max(0, pageAfter - 8)));
ui.next.addEventListener('click', () => navigate(mode, mode === 'roster' ? rosterNext : pageNext));
ui.reconcile.addEventListener('click', () => queue(() => reconcile(flow?.id || stable?.status?.pending_operation_id || stable?.status?.last_operation_id || ''), 'reconcile'));
ui['apply-saved'].addEventListener('click', () => { if (savedOperation?.state === 'prepared' && ready() && savedOperation.intent.view_id === current.view.view_id && savedOperation.intent.generation === current.view.generation) queue(() => beginJob('apply', [savedOperation.intent.operation_id], { operationID: savedOperation.intent.operation_id }), 'apply'); });
ui.abandon.addEventListener('click', () => queue(async () => {
  if (!ready() || savedOperation?.state !== 'prepared') return; const id = savedOperation.intent.operation_id, captured = epoch;
  try { const r = response(await call('AbandonStableOperation', current.view.view_id, id), 'operation', current, stable); if (captured !== epoch) return; if (!r.ok) { mutationMessage = textError(r.error); return; } requireValue(r.value.operation?.intent.operation_id === id && r.value.operation.state !== 'prepared'); savedOperation = r.value.operation; flow = null; mutationMessage = savedOperation.state === 'abandoned' ? 'The saved choice was discarded; roster slots are unchanged.' : 'This choice had already committed. The current roster will be refreshed.'; }
  catch { if (captured === epoch) { mutationMessage = textError('outcome_unknown'); await reconcile(id); } }
}, 'abandon'));
ui['cancel-job'].addEventListener('click', () => queue(() => cancelJob(watched?.id || stable?.active_job_id || stable?.queued_job_id), 'cancel'));
ui['export-recovery'].addEventListener('click', () => oldAction('ExportRecovery')); ui['verify-backup'].addEventListener('click', () => oldAction('VerifyBackup'));
document.addEventListener('visibilitychange', () => { if (document.hidden) { pausePreview(); pauseBooked(); } if (!document.hidden && enabled) { pollWanted = true; pump(); } else if (timer !== null) { clearTimeout(timer); timer = null; } });
window.addEventListener('pagehide', () => { disposed = true; epoch++; clearHistory(); clearEvent(); clearEntry(); clearPreview(); tasks = []; if (timer !== null) clearTimeout(timer); timer = null; clearCards(); pageItems = []; current = stable = null; });
render();
(async () => { try { requireValue(await call('Mode') === MODE); const selectedCareer = await call('CareerMode'); requireValue(typeof selectedCareer === 'boolean'); careerMode = selectedCareer; if (disposed) return; enabled = true; pollWanted = true; pump(); } catch { connectionError = 'Open this page through the candidate stable mode.'; render(); } })();
