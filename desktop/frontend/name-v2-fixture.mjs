// Presentation-only transport fixtures. The v2 manifest/SVG bytes are frozen
// independent reference data. Adapting an old namespace or transcript below is
// not new mined work, a signature, a recomputed fight or a cryptographic golden.
import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';
import {readFileSync} from 'node:fs';
import vm from 'node:vm';
import {sharedTransport} from './pool-cycle-fixture.mjs';

export const V1 = 'c1a8650a39f695814db67fddf79815ef249a13a5c0cc5af0b906b061c4c3a303';
export const V2 = 'cd02ae57b5918548244be2cabbe6c0c433feb5a1c165745e2e5c6bf235128e75';
export const clone = value => JSON.parse(JSON.stringify(value));
export function frozen(path, sha256) {
  const bytes = readFileSync(new URL(`../../${path}`, import.meta.url));
  assert.equal(createHash('sha256').update(bytes).digest('hex'), sha256, path);
  return JSON.parse(bytes);
}
export const names = frozen('fighterdescriptor/testdata/name-v2/independent/reference-v2.json', '62c4cd102341476548b9d388437af83750be407b8cbf534b34896719e2fa281b');
export const portraits = frozen('fighterportrait/testdata/reference/portraits_v1.json', '03df75cbf923a6dfc8fcac78e98d704641c0edf65ce563cf307f20a52f250670');
export const conditions = frozen('fightercondition/testdata/reference/condition_v1.json', '4f5be20e0e6ceb6b2ef4ee4063fffd2691fecb7a71d984f4624eb97162a9dbfa');
export const stables = frozen('gymstable/testdata/reference/stable_v1.json', '29f6a1302c2e04ade612fe2775c085c65b8bf33587c010060b727833182d2057');
export const bouts = frozen('gymbout/testdata/reference/bout_v1.json', '09f0430789328b9226fc4a24aae3cdb07beeb8b41d61b42cd33e4ba3121fa2ec');
export const events = frozen('gymevent/testdata/reference/event_v1.json', '9287f92c0cb46c585b44274c70ae57c5d6c9a4234e7078fd1d69a200644f996a');
export const histories = frozen('gymstable/testdata/career-history-reference/corpus.json', '18e51e25c6760f756159e12a92265b517438152533d439d541d4696044e60283');

const bySeed = new Map(names.cases.map(row => [row.seed, row]));
function descriptorTransport(value, descriptorID) {
  if (Array.isArray(value)) return value.map(v => descriptorTransport(v, descriptorID));
  if (value === null || typeof value !== 'object') return value;
  if (descriptorID === V2 && value.manifest_version) {
    const row = bySeed.get(value.dna.seed);
    assert.ok(row, 'Only independently frozen v2 manifests can enter this transport model');
    return clone(row.manifest);
  }
  const out = Object.fromEntries(Object.entries(value).map(([k, v]) => [k, descriptorTransport(v, descriptorID)]));
  if (out.descriptor_id) {
    assert.ok([V1, V2].includes(out.descriptor_id)); out.descriptor_id = descriptorID;
    const row = bySeed.get(out.work_hash);
    if (descriptorID === V2 && row && out.manifest_digest) {
      out.manifest_digest = row.manifest_digest; out.portrait_digest = row.svg_digest;
    }
  }
  return out;
}
export function transport(value, descriptorID = V2) {
  assert.ok([V1, V2].includes(descriptorID));
  return descriptorTransport(sharedTransport(value), descriptorID);
}
class Element {
  constructor() { this.children = []; this.attributes = new Map(); this.listeners = new Map(); this._text = ''; this.disabled = false; this.hidden = false; }
  set textContent(v) { this._text = String(v); this.children = []; }
  get textContent() { return this._text + this.children.map(v => v.textContent).join(''); }
  set innerHTML(_) { throw Error('Unsafe HTML'); }
  append(...nodes) { this.children.push(...nodes); }
  setAttribute(k, v) { this.attributes.set(k, String(v)); }
  removeAttribute(k) { this.attributes.delete(k); }
  addEventListener(k, v) { this.listeners.set(k, v); }
}
export function harness() {
  const source = readFileSync(new URL('./stable.js', import.meta.url), 'utf8');
  const runtime = "import { Call } from '/wails/runtime.js';", end = source.indexOf("ui['event-refresh'].addEventListener");
  assert.ok(source.startsWith(runtime) && end > 0);
  const html = readFileSync(new URL('./stable.html', import.meta.url), 'utf8');
  const elements = new Map([...html.matchAll(/\bid="([^"]+)"/g)].map(m => [m[1], new Element()]));
  const handlers = {}, calls = [];
  const context = vm.createContext({
    document: {getElementById: id => elements.get(id), createElement: () => new Element(), hidden: false},
    window: {matchMedia: () => ({matches: false, addEventListener() {}})},
    Call: {async ByName(name, ...args) { const method = name.slice(9); calls.push({method, args}); assert.ok(handlers[method], `Unexpected ${method}`); return handlers[method](...args); }},
    Intl, Date, btoa: s => Buffer.from(s, 'ascii').toString('base64'), setTimeout() { return 1; }, clearTimeout() {},
  });
  vm.runInContext(source.slice(runtime.length, end), context, {filename: 'stable.js'});
  const run = s => vm.runInContext(s, context);
  const set = (name, value) => { context.inputValue = clone(value); run(`${name}=inputValue`); };
  run('poolMode=true;enabled=true');
  return {context, elements, handlers, calls, run, set, call(name, ...args) { context.args = args; return run(`${name}(...args)`); }};
}
export function installCycle(h, value, descriptorID = V2) {
  const c = transport(value, descriptorID), view = c.views[0], pin = c.pin, ep = pin.source_pin.source_pin.source_pin;
  const own = ep.participants.find(p => p.stable_pin.account_id === view.account_id);
  const fixture = own.stable_pin.identity.gym_id === 'a227587d837507dbcb5f9d781967339d816070fb7bd574931c690ba6937b88e5' ? 'zero256' : 'incrementing256';
  h.set('current', {account: {state: 'signed_in', fixture, held: false}, view, gym_id: own.stable_pin.identity.gym_id});
  h.set('stable', {state: 'ready', view, status: {pin: own.stable_pin, store_id: own.stable_store_id, caught_up: true, pending_operation_id: '', slots: Array(10).fill('')}});
  h.set('eventAuthority', {pin: ep, storeID: pin.source_pin.source_pin.source_store_id, createdMS: 1});
  h.set('seedAuthority', {pin: pin.source_pin.source_pin, storeID: pin.source_pin.source_store_id, createdMS: 1});
  h.set('cardAuthority', {pin: pin.source_pin, storeID: pin.source_store_id, createdMS: c.source_store_created_at_ms});
  h.set('bookedAuthority', {pin, storeID: c.record.store_id, sourceCreatedMS: c.source_store_created_at_ms, createdMS: c.store_created_at_ms});
  h.set('entryDate', c.source.event.saturday);
  h.run('rememberedView=currentKey()');
  return c;
}
