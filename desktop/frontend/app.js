import { Call } from '/wails/runtime.js';

const elements = Object.fromEntries([
  'state', 'status-dot', 'hero-copy', 'mining-toggle', 'action-label', 'alert',
  'attempts', 'rate', 'prospect-count', 'nav-count', 'pending', 'pending-copy',
  'height', 'roster-label', 'empty', 'roster',
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
const cards = new Map();
const numberFormat = new Intl.NumberFormat();
const rateFormat = new Intl.NumberFormat(undefined, { maximumFractionDigits: 2 });
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

function normalize(value) {
  if (!value || !states.has(value.state) || value.mode !== 'local-pilot'
    || !['attempts', 'height', 'pending', 'prospect_count'].every((key) => integer(value[key]))
    || typeof value.hashes_per_second !== 'number' || !Number.isFinite(value.hashes_per_second)
    || value.hashes_per_second < 0 || !Array.isArray(value.prospects) || value.prospects.length > 100
    || value.prospect_count < value.prospects.length) {
    throw new Error('The local gym returned an unreadable update. Retrying…');
  }
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
  text(elements['action-label'], busy ? (command === 'Start' ? 'Starting…' : 'Stopping…') : action);
  elements['status-dot'].classList.toggle('active', state === 'running' && !connectionError);
  elements['mining-toggle'].classList.toggle('running', active);
  elements['mining-toggle'].setAttribute('aria-pressed', String(active));
  elements['mining-toggle'].setAttribute('aria-busy', String(busy || state === 'stopping'));
  // Starting again is also how a stopped gym retries saved uncertain work.
  // A recovery warning must not disable that explicit recovery action.
  elements['mining-toggle'].disabled = busy || exporting !== null || !snapshot || !!connectionError || state === 'stopping';
  document.title = `${stateText} · SLUG Gym`;
  renderAlert();
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

elements.state.setAttribute('aria-live', 'polite');
elements.alert.setAttribute('aria-atomic', 'true');
elements.roster.setAttribute('aria-live', 'off');
elements.roster.setAttribute('aria-busy', 'true');
elements['mining-toggle'].addEventListener('click', toggleMining);
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
