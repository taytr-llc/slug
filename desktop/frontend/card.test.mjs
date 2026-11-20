import {sharedTransport} from './pool-cycle-fixture.mjs';
// Frozen independent values plus presentation-state controls. No browser claim.
import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';
import {readFileSync} from 'node:fs';
import test from 'node:test';
import vm from 'node:vm';

const raw = readFileSync(new URL('../../gymcard/testdata/reference/card_v1.json', import.meta.url));
assert.equal(createHash('sha256').update(raw).digest('hex'), 'c52f513f8dfda34c884312c6a2caaf93b429bbf788aaf7c2cba3fbd901329790');
const ref = JSON.parse(raw), old = JSON.parse(readFileSync(new URL('../../gymevent/testdata/reference/event_v1.json', import.meta.url)));
const clone = v => JSON.parse(JSON.stringify(v));
const source = readFileSync(new URL('./stable.js', import.meta.url), 'utf8'), html = readFileSync(new URL('./stable.html', import.meta.url), 'utf8');
const script = source.slice("import { Call } from '/wails/runtime.js';".length, source.indexOf("ui['event-refresh'].addEventListener"));
class Element {
  constructor(tag = 'div') { this.tagName = tag; this.children = []; this.attributes = new Map(); this.listeners = new Map(); this._text = ''; this.disabled = false; }
  set textContent(v) { this._text = String(v); this.children = []; }
  get textContent() { return this._text + this.children.map(v => v.textContent).join(''); }
  set innerHTML(_) { throw Error('Unsafe HTML'); }
  append(...v) { this.children.push(...v); }
  setAttribute(k, v) { this.attributes.set(k, String(v)); }
  removeAttribute(k) { this.attributes.delete(k); }
  addEventListener(k, v) { this.listeners.set(k, v); }
}
function dto(kind, name) { const d = ref.dtos.find(d => d.kind === kind && d.name === name); assert.ok(d, `${kind}/${name}`); return clone(d.value); }
function harness(fixture = 'zero256', pool = false) {
 const transport = pool ? sharedTransport : clone;
  const f = transport(old.fixtures.find(f => f.fixture === fixture)), info = transport(dto('Info', 'empty'));
  const elements = new Map([...html.matchAll(/\bid="([^"]+)"/g)].map(m => [m[1], new Element()])), handlers = {}, calls = [];
  const document = {getElementById: id => elements.get(id), createElement: tag => new Element(tag), hidden: false};
  let id = 1000, active = 0, peak = 0;
  const Call = {async ByName(name, ...args) {
    const method = name.slice(9); calls.push({method, args: transport(args)}); assert.ok(handlers[method], `Unexpected ${method}`);
    active++; peak = Math.max(peak, active); try { return transport(await handlers[method](...args)); } finally { active--; }
  }};
  const context = vm.createContext({document, window: {matchMedia: () => ({matches: false, addEventListener() {}})}, Call, Intl, Date, btoa: s => Buffer.from(s, 'ascii').toString('base64'), setTimeout() { return 1; }, clearTimeout() {}});
  vm.runInContext(script, context, {filename: 'stable.js'});
 context.poolFixtureMode = pool; vm.runInContext('poolMode = poolFixtureMode', context);
  const h = {context, elements, handlers, calls, fixture: f, get peak() { return peak; }, run: s => vm.runInContext(s, context), set(n, v) { context.inputValue = transport(v); vm.runInContext(`${n}=inputValue`, context); }, count: m => calls.filter(c => c.method === m).length};
  h.set('current', {account: {state: 'signed_in', id: f.current_view.account_id, fixture, held: false, can_export: true}, view: f.current_view, gym_id: f.participant_id});
  h.set('stable', {state: 'ready', view: f.current_view, status: {pin: f.entry_pin.stable_pin, store_id: f.entry_pin.stable_store_id, caught_up: true, pending_operation_id: '', slots: Array(10).fill('')}});
  const event = old.dtos.find(d => d.kind === 'EventSnapshot' && d.fixture === fixture && d.value.state === 'ready').value;
  h.set('eventAuthority', {pin: old.pin.value, storeID: old.pin.store_id, createdMS: event.coordinator_created_at_ms}); h.set('eventSnapshot', event);
  h.set('seedAuthority', {pin: info.pin.source_pin, storeID: info.pin.source_store_id, createdMS: info.source_created_at_ms});
  h.run('enabled=true;rememberedView=currentKey()');
  h.reply = (field, value) => ({id: ++id, ok: true, view: transport(f.current_view), store_id: ref.pin.store_id, snapshot: null, status: null, record: null, history: null, job: null, error: '', [field]: transport(value)});
  h.refusal = error => ({id: ++id, ok: false, view: null, store_id: '', snapshot: null, status: null, record: null, history: null, job: null, error});
  context.initial = h.reply('snapshot', dto('CardSnapshot', fixture + '/ready'));
  h.run("cardSnapshot=cardEnvelope(initial,'snapshot').value;entryDate='2026-11-21'");
  return h;
}
const deferred = () => { let resolve; const promise = new Promise(r => { resolve = r; }); return {promise, resolve}; };

test('shared card controls use the same event authority and defer new builds during backup', async () => {
  for (const fixture of ['zero256', 'incrementing256']) {
    const h = harness(fixture, true), queued = dto('CardJob', fixture + '/queued');
    h.run('eventPinValue(eventAuthority.pin)'); assert.equal(h.run('cardReady()'), true);
    h.handlers.BeginCardBuild = () => h.reply('job', queued);
    h.run("current.backup_state='running';renderCard()"); await h.run('cardBegin()');
    assert.equal(h.count('BeginCardBuild'), 0); assert.equal(h.elements.get('card-begin').disabled, true);
    h.run("current.backup_state='idle'"); await h.run('cardBegin()');
    assert.equal(h.count('BeginCardBuild'), 1); assert.equal(h.calls[0].args[0], h.fixture.current_view.view_id);
    assert.equal(h.run('cardWatch.id'), queued.job_id); assert.equal(h.count('Start'), 0);
  }
});
function watch(h, j) { h.set('cardWatch', {id: j.job_id, kind: j.kind, state: 'queued', saturday: j.saturday, cardID: j.card_id}); }

test('all accepted independent card responses reach the current-view consumer', () => {
  let count = 0;
  for (const c of ref.protocol_cases.filter(c => c.accept && c.response.ok)) {
    const h = harness(c.fixture), r = clone(c.response), field = ['snapshot', 'status', 'record', 'history', 'job'].find(k => r[k] !== null);
    if (r.snapshot?.state === 'setup_incomplete') h.run('cardAuthority=null');
    h.context.input = r; assert.doesNotThrow(() => h.run(`cardEnvelope(input,'${field}')`), c.name); count++;
  }
  assert.ok(count >= 20);
});
test('independent matcher plans preserve every reason and pair partition', () => {
  const h = harness(); let count = 0;
  for (const c of ref.matcher_cases.filter(c => c.accept)) { h.context.input = clone(c.plan); assert.doesNotThrow(() => h.run('cardPlanValue(input)'), c.name); count++; }
  assert.equal(count, 53);
});
test('standalone card status cannot predate its source finality', () => {
  const h = harness(), s = dto('Status', 'existing');
  // Model a card store installed before seed finality, so this tests the
  // source-finality bound independently of the store-creation bound.
  h.run('cardAuthority.createdMS=cardAuthority.sourceCreatedMS');
  s.created_at_ms = s.source.status.finalized_at_ms;
  h.context.input = clone(s); assert.doesNotThrow(() => h.run('cardStatusValue(input)'));
  s.created_at_ms--;
  h.context.input = s; assert.throws(() => h.run('cardStatusValue(input)'));
});
test('card snapshot polling never builds, observes the seed or mines', async () => {
  const h = harness(); h.handlers.CardSnapshot = () => h.reply('snapshot', dto('CardSnapshot', 'zero256/ready'));
  await h.run('fetchCardSnapshot()'); assert.deepEqual(h.calls.map(c => c.method), ['CardSnapshot']); assert.equal(h.peak, 1);
});
test('both held fixture gyms may explicitly queue a card', async () => {
  for (const f of ['zero256', 'incrementing256']) {
    const h = harness(f); h.run('current.account.held=true'); h.handlers.BeginCardBuild = () => h.reply('job', dto('CardJob', f + '/queued'));
    await h.run('cardBegin()'); assert.equal(h.count('BeginCardBuild'), 1); assert.equal(h.run('cardWatch.state'), 'queued'); assert.equal(h.run('cardRetainedBytes()'), 294912);
  }
});
test('unknown Build never retries automatically or discards historical details', async () => {
  const h = harness(); h.set('cardDetail', dto('Record', 'retained_source_three_bouts')); const before = h.run('JSON.stringify(cardDetail)');
  h.handlers.BeginCardBuild = () => h.refusal('outcome_unknown'); await h.run('cardBegin()'); await h.run('cardBegin()');
  assert.equal(h.count('BeginCardBuild'), 1); assert.equal(h.run('cardUnknown'), true); assert.equal(h.run('JSON.stringify(cardDetail)'), before);
});
test('late read after logout cannot repopulate a previous gym', async () => {
  const h = harness(), gate = deferred(); h.handlers.CardRecord = () => gate.promise;
  const pending = h.run("cardRead('record')"); h.run('epoch++;clearSeed();current=null;stable=null'); gate.resolve(h.reply('record', dto('ReadResult', 'existing'))); await pending;
  assert.equal(h.run('cardDetail'), null); assert.equal(h.run('cardStatus'), null); assert.equal(h.run('cardWatch'), null);
});
test('late selected-date read and queued Build ACK preserve the new selection', async () => {
  for (const build of [false, true]) {
    const h = harness(), gate = deferred(); h.handlers[build ? 'BeginCardBuild' : 'CardRecord'] = () => gate.promise;
    const pending = h.run(build ? 'cardBegin()' : "cardRead('record')");
    h.run("entryDate='2026-11-28';eventSelectDate(entryDate)");
    const selected = dto('Status', 'absent'), next = ref.calendar.find(v => v.saturday === '2026-11-28');
    selected.event = clone(next.event); selected.card_id = next.card_id; selected.source.status.event = clone(next.event); h.set('cardStatus', selected);
    gate.resolve(h.reply(build ? 'job' : 'record', dto(build ? 'CardJob' : 'ReadResult', build ? 'zero256/queued' : 'existing'))); await pending;
    assert.equal(h.run('JSON.stringify(cardStatus)'), JSON.stringify(selected)); assert.equal(h.run('cardDetail'), null);
    if (build) assert.equal(h.run('cardWatch.saturday'), '2026-11-21');
  }
});
test('an older terminal job cannot overwrite a fresh explicit conflict', async () => {
  const h = harness(), j = dto('CardJob', 'zero256/completed'), s = dto('Status', 'historical_conflict');
  s.observed_at_ms += 100; s.source.status.observed_at_ms = s.observed_at_ms;
  h.context.input = s; h.run('cardAdoptStatus(input)'); watch(h, j); h.handlers.CardJob = () => h.reply('job', j);
  await h.run(`cardFetchJob('${j.job_id}')`); assert.equal(h.run('cardStatus.usable'), false); assert.equal(h.run('cardStatus.observed_at_ms'), s.observed_at_ms);
  assert.equal(h.run('cardDetail.card_id'), j.card_id); assert.match(h.elements.get('card-conflict').textContent, /conflicting evidence/);
});
test('observation uncertainty survives older terminal completion and has no booking claim', async () => {
  const h = harness(), j = dto('CardJob', 'zero256/completed'); h.context.input = dto('Status', 'existing'); h.run("cardAdoptStatus(input);cardRefusal('outcome_unknown');renderCard()");
  assert.match(h.elements.get('card-conflict').textContent, /has not been confirmed/); assert.doesNotMatch(h.elements.get('card-message').textContent, /may have been saved/);
  watch(h, j); h.handlers.CardJob = () => h.reply('job', j); await h.run(`cardFetchJob('${j.job_id}')`);
  assert.equal(h.run('cardOverlayKnown'), false); assert.match(h.elements.get('card-conflict').textContent, /has not been confirmed/);
});
test('unavailable Snapshot invalidates a status-only usability claim', async () => {
  const h = harness(); h.context.input = dto('Status', 'existing'); h.run('cardAdoptStatus(input)');
  const r = h.reply('snapshot', dto('CardSnapshot', 'zero256/unavailable')); r.view = null; r.store_id = '';
  h.handlers.CardSnapshot = () => r; await h.run('fetchCardSnapshot()'); h.run('renderCard()');
  assert.equal(h.run('cardOverlayKnown'), false); assert.equal(h.run('cardOverlayRead'), true); assert.equal(h.run('cardUnavailable'), true);
  assert.match(h.elements.get('card-conflict').textContent, /has not been confirmed/);
});
test('terminal bytes cannot change on a repeat poll', async () => {
  const h = harness(), j = dto('CardJob', 'zero256/completed'); watch(h, j); h.handlers.CardJob = () => h.reply('job', j);
  await h.run(`cardFetchJob('${j.job_id}')`); const before = h.run('JSON.stringify(cardDetail)'), key = h.run('cardTerminal.key');
  j.result.replayed = !j.result.replayed; await h.run(`cardFetchJob('${j.job_id}')`);
  assert.equal(h.run('cardUnknown'), true); assert.equal(h.run('JSON.stringify(cardDetail)'), before); assert.equal(h.run('cardTerminal.key'), key);
});
test('terminal cache rejection restores prior job and selected evidence atomically', async () => {
  const h = harness(), j = dto('CardJob', 'zero256/completed'); watch(h, j); h.set('cardStatus', dto('Status', 'existing'));
  const before = h.run('JSON.stringify([cardWatch,cardLastJob,cardTerminal,cardDetail,cardStatus,cardOverlayKnown,cardUnknown,cardOverlayRead])');
  h.handlers.CardJob = () => h.reply('job', j); h.run('cardCapacity=()=>false'); await h.run(`cardFetchJob('${j.job_id}')`);
  assert.equal(h.run('JSON.stringify([cardWatch,cardLastJob,cardTerminal,cardDetail,cardStatus,cardOverlayKnown,cardUnknown,cardOverlayRead])'), before);
  assert.match(h.elements.get('card-message').textContent, /full/);
});
test('shared cache reservation rejects Build before dispatch and protects old families', async () => {
  const h = harness(); h.set('pageItems', [{padding: 'x'.repeat(800000)}]); await h.run('cardBegin()');
  assert.equal(h.calls.length, 0); assert.equal(h.run('cardWatch'), null); assert.match(h.run('cardMessage'), /full/);
  h.set('pageItems', []); h.set('cardWatch', {id: '', state: 'queued'});
  const remaining = h.run('CACHE_MAX - (previewRetainedBytes()+entryRetainedBytes()+eventRetainedBytes()+seedRetainedBytes()+cardRetainedBytes()+JSON.stringify(pageItems).length)');
  assert.equal(h.run(`previewCapacity(${remaining})`), true); assert.equal(h.run(`previewCapacity(${remaining + 1})`), false);
  assert.match(source, /previewRetainedBytes\(\) \+ entryRetainedBytes\(\) \+ eventRetainedBytes\(\) \+ seedRetainedBytes\(\) \+ cardRetainedBytes\(\)/);
});
test('a wrong history cursor is refused without replacing the accepted page', async () => {
  const h = harness(), prior = dto('HistoryPage', 'empty'); h.set('cardHistory', prior); h.handlers.CardHistory = () => h.reply('history', dto('HistoryPage', 'one'));
  await h.run(`cardRead('history','${'a'.repeat(64)}')`); assert.equal(h.run('JSON.stringify(cardHistory)'), JSON.stringify(prior));
});
test('a lazy portrait cannot consume capacity reserved for a card', async () => {
  const h = harness(), r = dto('Record', 'retained_source_three_bouts');
  const i = {assignment: r.source.source_receipt.input.participants[0].members[0].assignment};
  const portraits = JSON.parse(readFileSync(new URL('../../fighterportrait/testdata/reference/portraits_v1.json', import.meta.url))), svg = portraits.cases[0].svg;
  h.context.item = clone(i); h.context.imageNode = new Element('img'); h.context.placeholderNode = new Element();
  h.run('cards.set(item.assignment.prospect_id,{fingerprint:JSON.stringify(item),image:imageNode,placeholder:placeholderNode})');
  const src = `data:image/svg+xml;base64,${Buffer.from(svg, 'ascii').toString('base64')}`;
  const charge = h.run('previewRetainedBytes()+entryRetainedBytes()+eventRetainedBytes()+seedRetainedBytes()+cardRetainedBytes()');
  h.set('pageItems', [{padding: 'x'.repeat((1 << 20) - charge - 2 * src.length - 1000)}]);
  h.handlers.ManifestPortrait = () => ({version: 1, view: clone(h.fixture.current_view), sequence: i.assignment.sequence, id: i.assignment.prospect_id, manifest_digest: i.assignment.manifest_digest, portrait_digest: i.assignment.portrait_digest, svg});
  await h.run('loadPortrait(item)'); assert.equal(h.run('images.size'), 1); assert.equal(h.context.imageNode.hidden, false);
  h.run('images.clear();cardWatch={id:"",state:"queued"}');
  await h.run('loadPortrait(item)'); assert.equal(h.run('images.size'), 0); assert.equal(h.context.imageNode.hidden, true); assert.equal(h.count('ManifestPortrait'), 2);
});
test('rendering reports every synthetic unmatched reason without granting source authority', () => {
  const h = harness(), plan = clone(ref.matcher_cases.find(c => c.name === 'all_reasons_and_one_pair').plan), r = dto('Record', 'retained_source_three_bouts'), s = dto('Status', 'existing');
  // Renderer-only input: not passed through authoritative Record acceptance.
  r.card_id = plan.card_id; r.plan = plan; s.card_id = plan.card_id; s.counts = clone(plan.counts); s.bout_count = plan.bout_count;
  h.set('cardDetail', r); h.set('cardStatus', s); h.run('cardOverlayKnown=true;renderCard()');
  assert.equal(h.elements.get('card-bouts').children.length, plan.bout_count); assert.equal(h.elements.get('card-unmatched').children.length, plan.counts.unmatched_count);
  assert.equal(h.elements.get('card-gyms').children.length, 2); assert.equal(h.elements.get('card-reasons').children.length, 5);
  for (const e of plan.entrants) assert.ok((h.elements.get('card-bouts').textContent + h.elements.get('card-unmatched').textContent).includes(e.prospect_id));
  assert.match(h.elements.get('card-totals').textContent, new RegExp(`${plan.counts.unmatched_count} unmatched`)); assert.equal(h.calls.length, 0);
});
test('empty complete plan is distinct from waiting for a seed', () => {
  const h = harness(), plan = clone(ref.matcher_cases.find(c => c.name === 'empty').plan), r = dto('Record', 'retained_source_three_bouts'), s = dto('Status', 'existing');
  r.card_id = plan.card_id; r.plan = plan; s.card_id = plan.card_id; s.counts = clone(plan.counts); s.bout_count = 0;
  h.set('cardDetail', r); h.set('cardStatus', s); h.run('cardOverlayKnown=true;renderCard()'); assert.match(h.elements.get('card-detail-hint').textContent, /no entered fighters/);
  h.set('cardDetail', null); h.set('cardStatus', dto('Status', 'absent')); h.run('renderCard()'); assert.doesNotMatch(h.elements.get('card-detail-hint').textContent, /no entered fighters/);
});
