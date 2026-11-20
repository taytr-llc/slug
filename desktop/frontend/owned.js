import { Call } from '/wails/runtime.js';

const $ = (id) => document.getElementById(id);
const isID = (v) => typeof v === 'string' && /^[a-f0-9]{64}$/.test(v);
const safeInt = (v, max = Number.MAX_SAFE_INTEGER) => Number.isSafeInteger(v) && v >= 0 && v <= max;
const requireValue = (v) => { if (!v) throw new Error('The gym response could not be verified. Refresh this view.'); };
const text = (id, value) => { $(id).textContent = String(value ?? ''); };
const short = (id) => typeof id === 'string' ? `${id.slice(0, 12)}…` : '—';
const amount = (n) => safeInt(n, 2100000000000000) ? (n / 1e8).toFixed(8) : '—';
const human = (s) => String(s ?? '').replaceAll('_', ' ');
const date = (n) => safeInt(n, 253402300799999) ? new Date(n).toLocaleString() : 'Unavailable';
let snapshot = null, busy = false, stable = null, page = null, history = null;
let slot = null, entry = null, entryIntent = null, request = '', transfer = null, dialogPending = false;
const view = () => snapshot?.view?.view_id ?? '';
function message(s) { text('alert', s); $('alert').hidden = !s; }
function clearView() {
  stable = page = history = slot = entry = entryIntent = transfer = null;
  request = ''; dialogPending = false;
  for (const id of ['fighters', 'roster', 'history', 'event-members', 'transfer-details']) $(id).replaceChildren();
  for (const id of ['transfer-status', 'finality', 'roster-status', 'entry-pending', 'event-status', 'history-status', 'stable-status', 'entry-status']) text(id, '');
  $('transfer-fighter').value = ''; $('recipient').value = '';
  $('transfer-requests').replaceChildren(new Option('Choose a saved request', ''));
}
function unavailable() {
  snapshot = null; clearView();
  for (const id of ['state', 'account-state', 'wallet-state']) text(id, 'Unavailable');
  for (const id of ['attempts', 'rate', 'pending', 'height', 'spendable', 'immature', 'payout', 'gym-key']) text(id, '—');
  for (const id of ['hold', 'backup']) text(id, '');
  text('mining-copy', 'Waiting for a verified gym status.');
  text('mining', 'Start mining');
}
function controls() {
  const signed = snapshot?.state === 'signed_in' && !!view();
  const held = snapshot?.account?.held === true;
  document.querySelectorAll('button,input,select').forEach((e) => { if (e.id !== 'transfer-fighter') e.disabled = busy || !signed; });
  for (const id of ['login-a', 'login-b']) $(id).disabled = busy || signed || !!snapshot?.pending_reset_id || !snapshot || snapshot.state === 'resetting';
  $('resume-reset').hidden = !snapshot?.pending_reset_id;
  $('resume-reset').disabled = busy || !snapshot?.pending_reset_id;
  $('mining').disabled = busy || !signed || !['running', 'stopped', 'failed'].includes(snapshot?.child?.state);
  const stopped = ['stopped', 'failed'].includes(snapshot?.child?.state);
  $('export-recovery').disabled = busy || !signed || !stopped || !snapshot.account.can_export || snapshot?.child?.backup?.state === 'running';
  $('verify-backup').disabled = busy || !signed || !stopped || snapshot?.child?.backup?.state === 'running';
  $('cancel-backup').disabled = busy || !signed || snapshot?.child?.backup?.state !== 'running';
  $('stable-next').disabled = busy || !signed || !page?.has_more;
  $('history-next').disabled = busy || !signed || !history?.has_more;
  $('slot-apply').disabled = busy || !signed || slot?.state !== 'prepared';
  $('slot-abandon').disabled = busy || !signed || slot?.state !== 'prepared';
  for (const id of ['entry-save', 'entry-withdraw', 'event-refresh', 'event-record']) $(id).disabled = busy || !signed || !entry;
  $('entry-save').disabled = busy || !signed || !entry || !page;
  $('entry-apply').disabled = busy || !signed || entryIntent?.state !== 'prepared';
  $('entry-abandon').disabled = busy || !signed || entryIntent?.state !== 'prepared';
  $('transfer-prepare').disabled = busy || !signed || held || !isID($('transfer-fighter').value) || !/^(02|03)[a-f0-9]{64}$/.test($('recipient').value) || dialogPending;
  for (const id of ['transfer-lookup', 'transfer-resume', 'transfer-finality', 'transfer-abandon', 'send-commit', 'observe-commit', 'send-spend', 'observe-spend']) $(id).disabled = busy || !signed || !isID(request) || dialogPending;
  for (const id of ['transfer-resume', 'transfer-abandon', 'send-commit', 'send-spend']) if (held) $(id).disabled = true;
  $('transfer-confirm').disabled = busy || !signed || held || transfer?.state !== 'review' || transfer?.request_id !== request;
}
async function call(method, ...args) { return await Call.ByName(`main.Gym.${method}`, ...args); }
async function owned(method, ...args) {
  const original = view(); requireValue(isID(original));
  const value = await call(method, original, ...args);
  requireValue(view() === original); return value;
}
async function task(fn) {
  if (busy) return;
  busy = true; controls(); message('');
  try { await fn(); } catch (e) { message(e?.message || 'This request did not complete. Check its saved status before retrying.'); }
  finally { busy = false; controls(); }
}
function bind(id, fn) { $(id).addEventListener('click', () => task(fn)); }
function validateSnapshot(s) {
  requireValue(s?.version === 1 && s.mode === 'local-owned-gym' && s.account?.state === s.state);
  if (s.state === 'signed_in') requireValue(isID(s.view?.view_id) && isID(s.view?.account_id) && s.view.account_id === s.account.id && s.child);
  else requireValue(s.view === null && s.child === null);
  return s;
}
async function refresh() {
  const prior = view(), s = validateSnapshot(await call('OwnedSnapshot'));
  snapshot = s;
  if (prior !== view()) clearView();
  text('state', human(s.child?.state ?? s.state)); text('account-state', human(s.state));
  text('hold', s.account.held ? `Security hold until ${date(s.account.hold_until_ms)}. Transfers and recovery export remain blocked.` : '');
  text('mining-copy', view() ? 'Mining uses this gym’s current wallet.' : 'Sign in to prepare your gym.');
  text('mining', s.child?.state === 'running' ? 'Stop mining' : 'Start mining');
  text('attempts', s.child?.attempts ?? 0); text('rate', `${((s.child?.hashes_per_second_milli ?? 0) / 1000).toFixed(2)} hashes / second`);
  text('pending', s.child?.pending ?? 0); text('height', s.child?.height ?? 0);
  text('wallet-state', human(s.child?.wallet_state ?? 'closed'));
  text('spendable', amount(s.child?.spendable_atoms)); text('immature', amount(s.child?.immature_atoms));
  text('payout', s.child?.payout_address || '—'); text('gym-key', s.view?.gym_public_key ?? '—');
  text('backup', s.child?.backup ? `${human(s.child.backup.state)} ${human(s.child.backup.phase)} ${human(s.child.backup.error)}` : 'Stop mining to save or verify a backup.');
  if (dialogPending && isID(request) && view()) {
    const status = await owned('OwnedTransferStatus', request); renderTransfer(status);
  }
}
function button(label, fn) { const b = document.createElement('button'); b.type = 'button'; b.textContent = label; b.disabled = busy; b.addEventListener('click', () => task(fn)); return b; }
function line(parent, label, value) { const dt = document.createElement('dt'), dd = document.createElement('dd'); dt.textContent = label; dd.textContent = String(value ?? ''); parent.append(dt, dd); }
function renderStable(p) {
  requireValue(p?.version === 1 && Array.isArray(p.items) && p.items.length <= 8 && p.status?.slots?.length === 10 && safeInt(p.after));
  page = p; $('fighters').replaceChildren();
  text('stable-status', `Owned fighters at block ${p.head.chain_height}, captured ${date(p.as_of_ms)}.${p.head.conflict_id ? ' A chain conflict has been reported; official finalized ownership is retained.' : ''}`);
  for (const item of p.items) {
    const id = item.ownership?.prospect_id; requireValue(isID(id) && item.ownership.owner_public_key === snapshot.view.gym_public_key);
    const card = document.createElement('article'); card.className = 'panel';
    const heading = document.createElement('h3'); heading.textContent = item.manifest.name.display || `${item.manifest.name.first} ${item.manifest.name.last}`;
    const identity = document.createElement('p'); identity.className = 'note'; identity.textContent = `Fighter ${id}`;
    const details = document.createElement('p'); details.textContent = `${human(item.condition.phase)} · rating ${(item.condition.rating_hundredths / 100).toFixed(2)} · ${item.condition.pro_xp} finalized XP`;
    const slotPick = document.createElement('select'); slotPick.setAttribute('aria-label', `Roster slot for ${heading.textContent}`);
    for (let n = 1; n <= 10; n++) slotPick.add(new Option(`Roster slot ${n}`, String(n)));
    card.append(heading, identity, details, button('Transfer', async () => { $('transfer-fighter').value = id; $('recipient').focus(); }), button('History', () => loadHistory(id)), slotPick,
      button('Prepare roster selection', async () => { const intent = await owned('OwnedPrepareSlot', { expected_revision: p.status.roster_revision, slot: Number(slotPick.value), prospect_id: id, expected_state_id: item.ownership.state_id, head: p.head }); slot = { state: 'prepared', intent }; text('roster-status', `Review slot ${intent.request.slot}: fighter ${id}. Apply to save this roster change.`); }));
    $('fighters').append(card);
  }
  $('roster').replaceChildren();
  p.status.slots.forEach((id, i) => { const li = document.createElement('li'); li.textContent = `Slot ${i + 1}: ${id || 'Empty'}`; if (id) li.append(button('Prepare removal', async () => { const intent = await owned('OwnedPrepareSlot', { expected_revision: p.status.roster_revision, slot: i + 1, prospect_id: '', expected_state_id: '', head: p.head }); slot = { state: 'prepared', intent }; text('roster-status', `Review removal from slot ${i + 1}. Apply to save.`); })); $('roster').append(li); });
}
async function captureStable() { stable = await owned('OwnedCaptureStable'); requireValue(stable?.view_id === view() && isID(stable.capture_id)); renderStable(stable.page); }
async function requests() {
  const rows = await owned('OwnedTransferRequests'); requireValue(Array.isArray(rows) && rows.length <= 128);
  $('transfer-requests').replaceChildren(new Option('Choose a saved request', ''));
  for (const r of rows) { requireValue(isID(r.request_id)); $('transfer-requests').add(new Option(`${date(r.created_at_ms)} · ${short(r.prospect_id)} · ${short(r.request_id)}`, r.request_id)); }
  $('transfer-requests').value = request;
}
function renderTransfer(v) {
  requireValue(v?.version === 1 && v.view_id === view() && v.request_id === request);
  transfer = v; dialogPending = ['dialog', 'confirming', 'delivering'].includes(v.state);
  text('transfer-status', `${human(v.state)}${v.error ? ` · ${human(v.error)}` : ''}`);
  const details = $('transfer-details'); details.replaceChildren();
  line(details, 'Request', v.request_id); line(details, 'Fighter', v.prospect_id); line(details, 'Recipient gym', v.recipient_gym);
  const r = v.review?.details ?? v.delivery?.details ?? v.operation?.preparation?.review;
  if (r) { line(details, 'Total network fee', `${amount(r.total_fee_atoms)} SLUG`); line(details, 'Return address', r.return_address); }
  if (v.operation) renderOperation(v.operation);
}
function renderOperation(op) {
  requireValue(op?.version === 1 && op.request_id === request);
  if (transfer) transfer.operation = op;
  text('transfer-status', op.stage === 'complete' ? 'Transactions delivered. Check ownership finality below.' : human(op.stage));
  const d = $('transfer-details'); d.replaceChildren(); line(d, 'Request', op.request_id); line(d, 'Recipient gym', op.request.recipient_gym);
  for (const leg of ['commit', 'spend']) { const o = op[`${leg}_observation`]; if (o) line(d, leg === 'commit' ? 'Commit observation' : 'Reveal observation', `${human(o.state)} · ${o.observed_txid || o.expected_txid} · ${o.confirmations} confirmations`); }
}
async function loadHistory(prospect) {
  const capture = await owned('OwnedCaptureHistory', prospect); requireValue(isID(capture.history_id));
  history = await owned('OwnedHistoryPage', capture.history_id, 0, 8); renderHistory();
}
function renderHistory() {
  requireValue(history?.version === 1 && Array.isArray(history.items) && history.items.length <= 8);
  text('history-status', `Finalized history captured ${date(history.capture.as_of_ms)} for ${history.capture.ownership.prospect_id}. Provisional fights do not contribute records or XP.`);
  $('history').replaceChildren(); for (const item of history.items) { const li = document.createElement('li'); li.textContent = `${item.event.saturday} · ${item.earned.wins ? 'Win' : item.earned.losses ? 'Loss' : 'Draw'} · ${human(item.event.method)} · +${item.xp_applied} XP · record ${item.totals_after.wins}–${item.totals_after.losses}–${item.totals_after.draws}`; $('history').append(li); }
}
function renderEntry() { text('entry-status', `${entry.event.saturday} · ${human(entry.phase)} · cutoff ${date(entry.event.cutoff_ms)} · ${entry.saved_ids.length} entered fighters`); }
async function prepareEntry(action) {
  requireValue(entry && (action === 'withdraw' || page));
  const r = { version: 1, event_id: entry.event.event_id, saturday: entry.event.saturday, action, expected_revision: entry.revision, expected_roster_revision: action === 'withdraw' ? 0 : page.status.roster_revision, prospect_ids: action === 'withdraw' ? [] : page.status.slots.filter(Boolean).sort() };
  const intent = await owned('OwnedPrepareEntry', r); entryIntent = { state: 'prepared', intent }; text('entry-pending', `Review ${action} for ${intent.request.saturday}: ${intent.request.prospect_ids.length} fighters. Apply to save.`);
}
bind('login-a', async () => { await call('OwnedLoginFixture', 'zero256'); await refresh(); });
bind('login-b', async () => { await call('OwnedLoginFixture', 'incrementing256'); await refresh(); });
bind('logout', async () => { await owned('OwnedLogout'); await refresh(); });
bind('reset', async () => { await owned('OwnedResetFixture'); await refresh(); });
bind('resume-reset', async () => { await call('OwnedResumeReset'); await refresh(); });
bind('mining', async () => { await owned(snapshot.child.state === 'running' ? 'OwnedStop' : 'OwnedStart'); await refresh(); });
for (const [id, method] of [['export-recovery', 'OwnedExportRecovery'], ['verify-backup', 'OwnedVerifyBackup'], ['cancel-backup', 'OwnedCancelBackup']]) bind(id, async () => { const result = await owned(method); if (result) message(result); await refresh(); });
bind('refresh-stable', captureStable);
bind('stable-next', async () => renderStable(await owned('OwnedStablePage', stable.capture_id, page.next_after, 8)));
bind('slot-lookup', async () => { slot = await owned('OwnedPendingSlot'); text('roster-status', slot ? `Saved roster change: ${human(slot.state)} · slot ${slot.intent.request.slot} · ${slot.intent.request.prospect_id || 'remove fighter'}` : 'No pending roster change.'); });
bind('slot-apply', async () => { await owned('OwnedApplySlot', slot.intent.operation_id); slot = null; await captureStable(); });
bind('slot-abandon', async () => { slot = await owned('OwnedAbandonSlot', slot.intent.operation_id); text('roster-status', human(slot.state)); });
$('recipient').addEventListener('input', controls);
$('transfer-requests').addEventListener('change', () => { request = $('transfer-requests').value; transfer = null; dialogPending = false; text('transfer-status', 'Load this exact request to inspect it.'); text('finality', ''); $('transfer-details').replaceChildren(); controls(); });
bind('requests-refresh', requests);
bind('transfer-prepare', async () => { const v = await owned('OwnedPrepareTransfer', $('transfer-fighter').value, $('recipient').value); request = v.request_id; renderTransfer(v); await requests(); });
bind('transfer-resume', async () => renderTransfer(await owned('OwnedResumeTransfer', request)));
bind('transfer-confirm', async () => renderTransfer(await owned('OwnedConfirmTransfer', request)));
bind('transfer-lookup', async () => renderOperation(await owned('OwnedTransferOperation', request)));
bind('transfer-abandon', async () => renderOperation(await owned('OwnedAbandonTransfer', request)));
for (const leg of ['commit', 'spend']) { bind(`send-${leg}`, async () => renderTransfer(await owned('OwnedDeliverTransfer', request, leg))); bind(`observe-${leg}`, async () => renderOperation(await owned('OwnedObserveTransfer', request, leg))); }
bind('transfer-finality', async () => {
  const v = await owned('OwnedTransferFinality', request); requireValue(v.version === 1 && v.request_id === request && ['unsigned', 'not_observed', 'pending', 'void', 'finalized'].includes(v.state));
  const labels = { unsigned: 'This request has not been signed.', not_observed: 'No canonical reveal is recorded at this captured head.', pending: 'Transfer pending signed checkpoint and 60 descendant blocks.', void: 'This competing transfer is void. Earlier valid chain order won.', finalized: 'Ownership finalized to the recipient gym.' };
  text('finality', `${labels[v.state]} Captured at block ${v.head.chain_height}.${v.public_evidence_pending ? ' Portable public evidence publication is still pending.' : ''}${v.conflict_id ? ' A later chain conflict is reported; the official finalized record is retained.' : ''}`);
});
bind('entry-refresh', async () => { entry = await owned('OwnedEntryUpcoming'); requireValue(entry?.version === 1 && Array.isArray(entry.saved_ids)); renderEntry(); });
bind('entry-save', () => prepareEntry('replace')); bind('entry-withdraw', () => prepareEntry('withdraw'));
bind('entry-lookup', async () => { entryIntent = await owned('OwnedPendingEntry'); text('entry-pending', entryIntent ? `Saved entry change: ${human(entryIntent.state)} · ${entryIntent.intent.request.action} · ${entryIntent.intent.request.saturday}` : 'No pending entry change.'); });
bind('entry-apply', async () => { await owned('OwnedApplyEntry', entryIntent.intent.operation_id); entryIntent = null; entry = await owned('OwnedEntryUpcoming'); renderEntry(); });
bind('entry-abandon', async () => { entryIntent = await owned('OwnedAbandonEntry', entryIntent.intent.operation_id); text('entry-pending', human(entryIntent.state)); });
bind('event-refresh', async () => { const v = await owned('OwnedEventStatus', entry.event.saturday); text('event-status', `${v.event.saturday}: ${human(v.phase)} · ${v.captured_participants.length} gyms captured.`); });
bind('event-record', async () => { const v = await owned('OwnedEventRecord', entry.event.saturday); text('event-status', `Immutable noon snapshot: ${v.snapshot_digest}`); $('event-members').replaceChildren(); for (const p of v.input.participants) { const li = document.createElement('li'); li.textContent = `Gym ${short(p.owner.gym_id)}: ${p.members.length} locked fighters, ${p.excluded.length} excluded. ${p.members.map((m) => `Slot ${m.slot}: ${short(m.ownership.prospect_id)} (${human(m.class)}, rating ${(m.condition_at_cutoff.rating_hundredths / 100).toFixed(2)})`).join('; ')}${p.excluded.length ? ' Excluded: ' + p.excluded.map((m) => `${short(m.prospect_id)} — ${human(m.reason)}`).join('; ') : ''}`; $('event-members').append(li); } });
bind('history-next', async () => { history = await owned('OwnedHistoryPage', history.capture.history_id, history.next, 8); renderHistory(); });
async function poll() { if (!busy) { busy = true; try { await refresh(); } catch (e) { unavailable(); message(e?.message || 'The gym is unavailable.'); } finally { busy = false; controls(); } } setTimeout(poll, 2000); }
controls(); poll();
