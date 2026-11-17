import { Call } from '/wails/runtime.js';

// Separate stable entry: only typed, current-view, candidate-only native data.
const MODE = 'local-manifest-pilot';
const DESCRIPTOR = 'c1a8650a39f695814db67fddf79815ef249a13a5c0cc5af0b906b061c4c3a303';
const GENESIS = '7893676099cf9985bb198dfb6a347d9422a19f2422cf3326a999a7230ce246cd';
const DATE_MAX = 8640000000000000;
const CACHE_MAX = 1 << 20;
const ids = ['state', 'account-state', 'account-copy', 'hold', 'login-a', 'login-b', 'logout', 'reset', 'mining', 'mining-copy', 'alert', 'attempts', 'rate', 'accepted', 'pending', 'height', 'wallet-state', 'wallet-height', 'spendable', 'immature', 'payout', 'gym-id', 'export-recovery', 'verify-backup', 'backup', 'refresh', 'coverage', 'announcement', 'empty', 'empty-copy', 'cards', 'candidates-heading', 'stable-state', 'stable-count', 'stable-progress', 'sync', 'pause-sync', 'recent', 'inventory', 'roster-details', 'previous', 'next', 'page-label', 'roster-slots', 'roster-count', 'mutation-state', 'reconcile', 'apply-saved', 'abandon', 'cancel-job'];
ids.push('preview-state', 'preview-selection', 'preview-begin', 'preview-cancel', 'preview-clear', 'preview-message', 'preview-replay', 'preview-asof', 'preview-fighters', 'preview-play', 'preview-previous', 'preview-next', 'preview-restart', 'preview-result', 'preview-motion', 'preview-step', 'preview-events', 'preview-outcome', 'preview-finish', 'preview-scores');
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
function manifestValue(m, seed) {
  exact(m, ['manifest_version', 'generator_id', 'table_hash', 'dna', 'name', 'physical', 'style', 'portrait']);
  requireValue(m.manifest_version === 1 && m.generator_id === 'slug-fighter-manifest-candidate-v1' && m.table_hash === '72231fbe707cea9a65dd5b92f7b2719953795bceadbbf9e3b8e0b8f4aaef3f44');
  const d = m.dna; exact(d, ['dna_version', 'seed', 'rarity', 'talent_budget', 'attributes', 'prime_weeks', 'decline_weeks']);
  requireValue(d.dna_version === 1 && d.seed === seed && rarity.includes(d.rarity) && int(d.talent_budget, 250, 489) && int(d.prime_weeks, 26, 52) && int(d.decline_weeks, 13, 39));
  exact(d.attributes, attrs); requireValue(attrs.every((k) => int(d.attributes[k], 1, 100)) && attrs.reduce((n, k) => n + d.attributes[k], 0) === d.talent_budget);
  const limits = [[250, 299], [300, 349], [350, 399], [400, 449], [450, 489]][rarity.indexOf(d.rarity)]; requireValue(int(d.talent_budget, ...limits));
  exact(m.name, ['first', 'last', 'display']); requireValue(['first', 'last'].every((k) => typeof m.name[k] === 'string' && /^[A-Za-z][A-Za-z '\-]{0,63}$/.test(m.name[k])) && m.name.display === `${m.name.first} ${m.name.last}`);
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
  epoch++; clearPreview(); clearCards(); pageItems = []; watched = null; savedOperation = null; lastViewedJob = ''; pageAfter = pageNext = 0; rosterFirst = 1; rosterNext = 0; pageRevision = null; mode = 'recent';
  if (clearFlow) { flow = null; mutationMessage = ''; }
}
function disconnected() { forgetView(false); current = stable = null; connectionError = 'The local gym is unavailable. Saved changes remain on disk; reconnect before continuing.'; render(); }
function textError(code) {
  return ({ busy: 'Another local task is finishing. Try again shortly.', stable_sync_required: 'Saved candidates are still syncing. Resume syncing, then retry this choice.', stable_conflict: 'The roster changed. Refresh it before making a new choice.', stable_pending: 'A saved choice is awaiting reconciliation.', stable_capacity: 'This demo store reached its supported capacity. Existing candidates and outcomes remain available.', stable_clock: 'The authority clock needs attention. Condition and edits are unavailable.', stable_cancelled: 'The request was cancelled.', stale_view: 'This account view has expired. Sign in again.', unauthorized: 'Sign in again to continue.', forbidden: 'This action is unavailable for the current session.', stable_not_found: 'That saved result is unavailable. Reload the page or check the saved change.', outcome_unknown: 'The change may have been saved. Check its recorded outcome before trying another choice.' })[code] ?? 'The local stable could not complete the request. Its saved data has been retained.';
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
function makeCard(i) {
  const a = i.assignment, m = i.manifest, c = i.condition;
  const article = element('article', 'candidate'), area = element('div', 'portrait-area'), image = element('img'), placeholder = element('p', 'portrait-placeholder', 'Loading verified portrait…');
  image.setAttribute('width', '200'); image.setAttribute('height', '200'); image.setAttribute('alt', `Candidate portrait of ${m.name.display}`); image.hidden = true; area.append(image, placeholder); article.append(area);
  const body = element('div', 'card-body'), meta = element('div', 'card-meta'); meta.append(element('span', '', label(m.physical.weight_class)), element('span', '', label(m.dna.rarity))); body.append(meta, element('h3', '', m.name.display));
  body.append(element('p', 'physical', `${m.physical.height_in} in tall · ${m.physical.reach_in} in reach · ${m.physical.weight_lb} lb\n${label(m.physical.stance)} · ${label(m.style)}`));
  body.append(element('p', 'condition-phase', `${label(c.phase)} · ${c.age_weeks} completed weeks`), element('p', 'birth', `Condition as of ${date.format(new Date(c.as_of_ms))}`), element('p', 'birth', `Original birth ${date.format(new Date(a.born_at_ms))}`));
  const table = element('table', 'condition-table'), head = element('tr');
  for (const name of ['Attribute', 'Mined', 'Current']) { const th = element('th', '', name); th.setAttribute('scope', 'col'); head.append(th); } const thead = element('thead'); thead.append(head); table.append(thead); const tbody = element('tbody');
  for (const k of attrs) { const row = element('tr'), th = element('th', '', label(k)); th.setAttribute('scope', 'row'); row.append(th, element('td', '', String(m.dna.attributes[k])), element('td', '', (c.league_attributes_tenths[k] / 10).toFixed(1))); tbody.append(row); }
  table.append(tbody); body.append(table, element('p', 'rating', `Rating ${(c.rating_hundredths / 100).toFixed(2)}`), element('p', 'note', `Prime remaining: ${(c.prime_remaining_ms / WEEK).toFixed(2)} weeks · Career remaining: ${(c.career_remaining_ms / WEEK).toFixed(2)} weeks. Recorded professional XP: 0.`));
  const selectLabel = element('label', 'select-label', 'Choose roster slot'), select = element('select', 'slot-select'); select.setAttribute('aria-label', `Roster slot for ${m.name.display}`);
  for (let n = 1; n <= 10; n++) { const o = element('option', '', `Slot ${n}`); o.value = String(n); select.append(o); } select.value = '1'; selectLabel.append(select); body.append(selectLabel); article.append(body);
  const actions = element('div', 'card-actions'), assign = element('button', '', 'Save to slot'), save = element('button', '', 'Export public receipt'), retry = element('button', '', 'Reload portrait'); assign.type = save.type = retry.type = 'button';
  assign.addEventListener('click', () => { const slot = Number(select.value); if (int(slot, 1, 10)) choose(slot, a.prospect_id); });
  save.addEventListener('click', () => oldAction('ExportManifestReceipt', [a.sequence], a.prospect_id));
  retry.addEventListener('click', () => { images.delete(key(i)); imageAttempts.delete(key(i)); image.removeAttribute('src'); image.hidden = true; placeholder.hidden = false; pump(); });
  const preview = element('button', '', 'Select for preview'); preview.type = 'button'; preview.addEventListener('click', () => selectPreview(i));
  actions.append(assign, save, retry, preview); article.append(actions);
  const details = element('details'), summary = element('summary', '', 'Verified identity'); details.append(summary); const dl = element('dl');
  for (const [name, value] of [['Prospect ID', a.prospect_id], ['Assignment sequence', a.sequence], ['Accepted', date.format(new Date(a.accepted_at_ms))], ['Manifest digest', a.manifest_digest], ['Portrait digest', a.portrait_digest], ['Source receipt digest', a.source_record_digest]]) dl.append(element('dt', '', name), element('dd', '', value));
  details.append(dl); article.append(details);
  return { article, image, placeholder, assign, save, retry, preview, select, details, item: i, fingerprint: JSON.stringify(i) };
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
    if (c && c.fingerprint !== JSON.stringify(i)) { const previous = c; requireValue(same(c.item.assignment, i.assignment) && same(c.item.manifest, i.manifest)); c = makeCard(i); c.select.value = previous.select.value; c.details.open = previous.details.open; previous.image.removeAttribute('src'); previous.article.remove(); cards.set(id, c); }
    if (!c) { c = makeCard(i); cards.set(id, c); }
    if (ui.cards.children[position] !== c.article) ui.cards.insertBefore(c.article, ui.cards.children[position] ?? null);
    c.preview.disabled = !ready() || !!command || previewSelected.some((v) => v.id === id) || previewSelected.length >= 2;
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
  content(ui.hold, s?.account.held ? 'The 24-hour recovery hold is active. Viewing, mining and reversible roster moves are available; recovery backup export remains held.' : '');
  for (const id of ['login-a', 'login-b']) ui[id].disabled = !enabled || !!command || !s || !['signed_out', 'failed', 'expired'].includes(s.account.state) || !!s.view;
  ui.logout.disabled = !accountSigned || !!command; ui.reset.disabled = !accountSigned || !!command;
  ui.mining.disabled = !enabled || !!command || (!accountSigned && !preparing) || s?.state === 'stopping'; content(ui.mining, live ? 'Stop mining' : preparing ? 'Stop preparing' : 'Start mining');
  content(ui['mining-copy'], preparing ? 'Preparing your retained identity. Stop drains work already in progress.' : live ? 'Mining is active. New saved candidates update the stable automatically.' : 'Syncing and viewing do not create mining jobs. Start only when you are ready.');
  for (const [id, value] of [['attempts', s?.attempts ?? 0], ['accepted', s?.accepted_count ?? 0], ['pending', s?.pending ?? 0], ['height', s?.height ?? 0]]) content(ui[id], number.format(value));
  content(ui.rate, live ? `${number.format(s.hashes_per_second)} hashes / second` : 'Not mining'); content(ui['wallet-state'], label(s?.wallet_state ?? 'closed'));
  content(ui.spendable, accountSigned ? coin.format(s.spendable_atoms / 1e8) : '—'); content(ui.immature, accountSigned ? coin.format(s.immature_atoms / 1e8) : '—'); content(ui['wallet-height'], accountSigned ? `Wallet height ${number.format(s.wallet_height)} · Rewards mature after 100 blocks.` : 'Rewards mature after 100 blocks.');
  content(ui.payout, accountSigned ? s.payout_address : 'Sign in to load the demo wallet'); content(ui['gym-id'], accountSigned ? s.gym_id : '—');
  ui['export-recovery'].disabled = !accountSigned || !!command || live || !!watched || !s.account.can_export || s.backup_state === 'running'; ui['verify-backup'].disabled = !accountSigned || !!command || live || !!watched || s.backup_state === 'running';
  content(ui.backup, s?.backup_state === 'running' ? `${backupPhases[s.backup_phase] || 'Checking the backup'}…` : s?.backup_state === 'succeeded' ? 'Backup verification completed.' : s?.backup_state === 'failed' ? 'Backup verification failed; the original wallet remains retained.' : 'Stop mining to save or verify a wallet backup.');
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
  renderRoster(); renderCards(); renderPreview();
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
  } catch { if (!disposed && captured === epoch) disconnected(); }
}
function stablyChangedRoster() { return stable.status.roster_revision !== pageRevision; }
function acceptItems(items, kind, data) {
  if (kind === 'sync') {
    if (mode !== 'recent') { message = 'New candidates were saved. Your selected page remains open.'; return; }
    const merged = new Map(pageItems.map((i) => [i.assignment.prospect_id, i])); for (const i of items) { const prior = merged.get(i.assignment.prospect_id); requireValue(!prior || same(prior.assignment, i.assignment) && same(prior.manifest, i.manifest)); merged.set(i.assignment.prospect_id, i); }
    pageItems = [...merged.values()].sort((a, b) => b.assignment.sequence - a.assignment.sequence).slice(0, 8);
  } else {
    pageItems = own(kind === 'roster' ? items.map((v) => v.item) : items);
    if (kind === 'roster') { mode = 'roster'; rosterFirst = data.first_slot; rosterNext = data.next_slot; pageRevision = data.status.roster_revision; }
    else { pageAfter = data.after; pageNext = data.next_after; pageRevision = null; if (mode === 'recent') pageItems.reverse(); }
  }
  requireValue(pageItems.length <= 8); render(); content(ui.announcement, `${pageItems.length} verified candidates shown. Conditions keep their stated observation time.`);
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
      else if (j.kind === 'sync' || w?.id === id) acceptItems(data.items, j.kind, data);
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
    watched = { id: r.value.job_id, kind, args, ...extra }; pollWanted = true;
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
    const src = `data:image/svg+xml;base64,${btoa(p.svg)}`; requireValue([...images.entries()].reduce((n, [other, v]) => n + (other === k ? 0 : 2 * v.length), 2 * src.length) + JSON.stringify(pageItems).length + previewRetainedBytes() <= CACHE_MAX && images.size <= 8);
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
function previewPins(v) { requireValue(v.version === 1 && ready() && viewKey(checkedView(v.view)) === currentKey() && v.store_id === stable.status.store_id && v.engine_version === 1 && v.engine_id === PREVIEW_ENGINE); }

let previewSnapshot = null, previewSelected = [], previewWatch = null, previewDiscovery = null, previewResult = null, previewSerial = 0, previewMessage = '';
let playbackTimer = null, playbackIndex = 0, playbackSteps = [], previewImages = new Map(), previewImageAttempts = new Set(), previewRendered = '';
const motionPreference = typeof window.matchMedia === 'function' ? window.matchMedia('(prefers-reduced-motion: reduce)') : null;
function pausePreview() { if (playbackTimer !== null) clearTimeout(playbackTimer); playbackTimer = null; }
function discardPreviewResult() { pausePreview(); previewResult = null; playbackSteps = []; playbackIndex = 0; previewImages.clear(); previewImageAttempts.clear(); previewRendered = ''; ui['preview-fighters'].textContent = ''; ui['preview-events'].textContent = ''; ui['preview-scores'].textContent = ''; }
function clearPreview() { previewSerial++; discardPreviewResult(); previewSelected = []; previewWatch = null; previewDiscovery = null; previewSnapshot = null; previewMessage = ''; ui['preview-selection'].textContent = ''; }
function previewRetainedBytes() { return JSON.stringify(previewSelected).length + (previewResult ? JSON.stringify(previewResult).length + 65536 : 0) + [...previewImages.values()].reduce((n, src) => n + 2 * src.length, 0); }
function previewCapacity(extra = 0) { return previewRetainedBytes() + JSON.stringify(pageItems).length + [...images.values()].reduce((n, src) => n + 2 * src.length, 0) + extra <= CACHE_MAX; }
function previewError(code) { return ({ preview_class_mismatch: 'Choose two candidates from the same weight class.', preview_not_found: 'This preview is no longer retained. Select your pair and explicitly preview again.', preview_capacity: 'The local preview cache is full. Clear the preview and retry.', preview_cancelled: 'Preview cancelled. No fight was recorded.', preview_observation_unknown: 'The authority could not confirm its clock observation. Reopen the gym before another preview. No fight was recorded.', outcome_unknown: 'The preview may have started. Check its status before explicitly trying again.', preview_unavailable: 'Fight preview is unavailable. Your saved candidates are retained.' })[code] ?? textError(code); }
function previewRefusal(code) {
  if (['unauthorized', 'stale_view'].includes(code)) clearPreview();
  else if (['preview_unavailable', 'preview_not_found', 'preview_observation_unknown'].includes(code)) {
    discardPreviewResult(); previewWatch = null; previewDiscovery = null; previewSnapshot = null;
  }
  previewMessage = previewError(code);
}
function selectionChanged(next) {
  const old = previewWatch, oldView = current?.view?.view_id;
  previewSerial++; previewSelected = next; previewWatch = null; previewDiscovery = null; discardPreviewResult(); previewMessage = '';
  if (old && oldView) tasks.unshift(async () => { if (ready() && current.view.view_id === oldView) { try { await call('CancelFightPreview', oldView, old.id); } catch {} } });
  render(); pump();
}
function selectPreview(i) {
  if (!ready() || command || previewSelected.length >= 2 || previewSelected.some((v) => v.id === i.assignment.prospect_id)) return;
  const next = { id: i.assignment.prospect_id, name: i.manifest.name.display, weightClass: i.manifest.physical.weight_class };
  if (previewSelected.length && previewSelected[0].weightClass !== next.weightClass) { previewMessage = previewError('preview_class_mismatch'); renderPreview(); return; }
  selectionChanged([...previewSelected, next]);
}
async function fetchPreviewSnapshot() {
  const captured = epoch, serial = previewSerial, view = currentKey();
  try {
    const r = previewEnvelope(await call('PreviewSnapshot'), 'snapshot'); if (disposed || captured !== epoch || view !== currentKey()) return;
    if (!r.ok) { previewRefusal(r.error); renderPreview(); return; }
    previewSnapshot = r.value;
    if (r.value.view === null) { clearPreview(); renderPreview(); return; }
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
  const r = previewEnvelope(await call('FightPreviewJob', current.view.view_id, id), 'job');
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
  if (!ready() || previewSelected.length !== 2 || previewWatch || !previewSnapshot?.view) return;
  const captured = epoch, serial = previewSerial, view = currentKey(), viewID = current.view.view_id, ids = previewSelected.map((s) => s.id).sort();
  previewDiscovery = { serial, ids };
  const r = previewEnvelope(await call('BeginFightPreview', viewID, ...ids), 'job');
  if (disposed || captured !== epoch || serial !== previewSerial || view !== currentKey()) { if (!disposed && r.ok && ready() && current.view.view_id === viewID) await call('CancelFightPreview', viewID, r.value.job_id); return; }
  if (!r.ok) { previewRefusal(r.error); if (r.error === 'outcome_unknown') discardPreviewResult(); else previewDiscovery = null; return; }
  requireValue(r.value.state === 'queued' && r.value.fighter_a_id === ids[0] && r.value.fighter_b_id === ids[1]); discardPreviewResult(); previewWatch = { id: r.value.job_id, serial }; previewDiscovery = null; previewMessage = 'Preparing fight…'; pollWanted = true;
}
async function cancelPreview() {
  if (!ready() || !previewWatch) return; const captured = epoch, serial = previewSerial, id = previewWatch.id;
  const r = previewEnvelope(await call('CancelFightPreview', current.view.view_id, id), 'job');
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
    const r = await call('FightPreviewPortrait', j.view.view_id, j.job_id, a.prospect_id); if (!remains()) return;
    requireValue(r?.version === 1 && typeof r.ok === 'boolean' && JSON.stringify(r).length <= 98304);
    if (!r.ok) { exact(r, ['version', 'ok', 'error']); requireValue(PREVIEW_ERRORS.has(r.error)); previewRefusal(r.error); renderPreview(); return; }
    exact(r, ['version', 'ok', 'portrait']); const p = r.portrait;
    exact(p, ['version', 'view', 'store_id', 'job_id', 'prospect_id', 'result_digest', 'descriptor_id', 'manifest_digest', 'portrait_digest', 'svg']);
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
  content(ui['preview-state'], previewWatch ? 'Preparing fight' : previewResult ? 'Ready to replay' : 'Choose two candidates'); content(ui['preview-message'], previewMessage);
  const selectionKey = JSON.stringify(previewSelected);
  if (ui['preview-selection']._selectionKey !== selectionKey) {
    ui['preview-selection']._selectionKey = selectionKey; ui['preview-selection'].textContent = '';
    for (const s of previewSelected) { const li = element('li', '', `${s.name} · ${label(s.weightClass)}`), remove = element('button', '', 'Remove'); remove.type = 'button'; remove.setAttribute('aria-label', `Remove ${s.name} from preview`); remove.addEventListener('click', () => selectionChanged(previewSelected.filter((x) => x.id !== s.id))); li.append(remove); ui['preview-selection'].append(li); }
  }
  ui['preview-begin'].disabled = !ready() || !previewSnapshot?.view || previewSelected.length !== 2 || !!previewWatch || !!command;
  ui['preview-cancel'].disabled = !ready() || !previewWatch || !!command; ui['preview-clear'].disabled = previewSelected.length === 0;
  ui['preview-replay'].hidden = !previewResult; if (!previewResult) return;
  const j = previewResult, r = j.result.engine_result, fighters = j.result.pair.fighters, names = fighters.map((i) => i.manifest.name.display);
  content(ui['preview-asof'], `Both conditions observed ${date.format(new Date(j.result.pair.as_of_ms))}. This replay keeps that observation fixed.`);
  const rendered = j.result.engine_result_digest + JSON.stringify([...previewImages.keys()]);
  if (previewRendered !== rendered) {
    previewRendered = rendered; ui['preview-fighters'].textContent = '';
    for (const [n, i] of fighters.entries()) { const box = element('div', 'preview-fighter'), src = previewImages.get(i.assignment.prospect_id); if (src) { const image = element('img'); image.setAttribute('src', src); image.setAttribute('alt', `Candidate portrait of ${names[n]}`); image.setAttribute('width', '160'); image.setAttribute('height', '160'); box.append(image); } box.append(element('strong', '', names[n]), element('span', 'note', `${label(i.condition.phase)} · Rating ${(i.condition.rating_hundredths / 100).toFixed(2)} · XP 0`)); ui['preview-fighters'].append(box); }
    ui['preview-scores'].textContent = ''; ui['preview-scores'].append(element('caption', '', 'Whole-fight scorecard')); const head = element('tr'); for (const title of ['Component', ...names]) { const th = element('th', '', title); th.setAttribute('scope', 'col'); head.append(th); } const thead = element('thead'); thead.append(head); ui['preview-scores'].append(thead); const body = element('tbody');
    for (const [key, title] of [['damage_tenths', 'Damage'], ['knockdown_tenths', 'Knockdowns'], ['accuracy_tenths', 'Accuracy'], ['volume_tenths', 'Volume'], ['control_tenths', 'Control'], ['total_tenths', 'Total']]) { const row = element('tr'), th = element('th', '', title); th.setAttribute('scope', 'row'); row.append(th, ...r.scores.map((score) => element('td', '', (score[key] / 10).toFixed(1)))); body.append(row); } ui['preview-scores'].append(body);
  }
  content(ui['preview-play'], playbackTimer === null ? 'Play' : 'Pause'); ui['preview-play'].disabled = !!motionPreference?.matches; ui['preview-previous'].disabled = playbackIndex === 0; ui['preview-next'].disabled = playbackIndex >= playbackSteps.length;
  content(ui['preview-motion'], motionPreference?.matches ? 'Reduced motion is on. Use Previous step, Next step or Show result.' : 'Playback is local. Pause or step through at any time.');
  const last = playbackSteps[playbackIndex - 1]; content(ui['preview-step'], last ? `Round ${last.round}, tick ${last.tick.number} · Step ${playbackIndex} of ${playbackSteps.length}` : 'Ready at the opening bell.');
  ui['preview-events'].textContent = '';
  for (const event of playbackSteps.slice(0, playbackIndex)) { const t = event.tick; ui['preview-events'].append(element('li', '', `Round ${event.round}, tick ${t.number}: ${names[t.actor]} ${t.hit ? 'lands' : 'misses'} a ${t.strike}.${t.knockdown ? ' Knockdown.' : ''}${t.stoppage ? ` ${label(t.stoppage)}.` : ''}`)); }
  ui['preview-outcome'].hidden = playbackIndex < playbackSteps.length;
  const winner = fighters.find((i) => i.assignment.prospect_id === r.winner)?.manifest.name.display;
  content(ui['preview-finish'], r.method === 'decision_draw' ? 'Draw — equal whole-fight scores' : r.method === 'double_corner_stoppage' ? 'Draw — both corners stopped the fight' : `${winner} wins by ${label(r.method)}.`);
}
motionPreference?.addEventListener('change', () => { pausePreview(); renderPreview(); });
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
document.addEventListener('visibilitychange', () => { if (document.hidden) pausePreview(); if (!document.hidden && enabled) { pollWanted = true; pump(); } else if (timer !== null) { clearTimeout(timer); timer = null; } });
window.addEventListener('pagehide', () => { disposed = true; epoch++; clearPreview(); tasks = []; if (timer !== null) clearTimeout(timer); timer = null; clearCards(); pageItems = []; current = stable = null; });
render();
(async () => { try { requireValue(await call('Mode') === MODE); if (disposed) return; enabled = true; pollWanted = true; pump(); } catch { connectionError = 'Open this page through the candidate stable mode.'; render(); } })();
