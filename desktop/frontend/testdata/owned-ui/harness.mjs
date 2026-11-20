// Source-only DOM/bridge harness. No wallet, helper, network, or cryptographic authority.
import fs from 'node:fs';
import vm from 'node:vm';

export class Element {
  constructor(tag, id = '') { this.tagName = tag.toUpperCase(); this.id = id; this.children = []; this.listeners = {}; this.disabled = false; this.hidden = false; this.value = ''; this._text = ''; }
  set textContent(v) { this._text = String(v); this.children = []; }
  get textContent() { return this._text + this.children.map(c => typeof c === 'string' ? c : c.textContent).join(''); }
  append(...nodes) { this.children.push(...nodes); }
  replaceChildren(...nodes) { this.children = []; this._text = ''; this.append(...nodes); if (this.tagName === 'SELECT') this.value = nodes[0]?.value ?? ''; }
  add(option) { this.append(option); if (this.children.length === 1) this.value = option.value; }
  setAttribute(name, value) { this[name] = value; }
  addEventListener(type, fn) { (this.listeners[type] ??= []).push(fn); }
  focus() {}
  async fire(type = 'click') { if (this.disabled) return; for (const fn of this.listeners[type] ?? []) await fn({ target: this }); }
}
export class Option extends Element { constructor(label, value) { super('option'); this.textContent = label; this.value = value; } }
const clone = v => structuredClone(v);
export async function loadUI(jsPath, htmlPath, initial) {
  const html = fs.readFileSync(htmlPath, 'utf8'), source = fs.readFileSync(jsPath, 'utf8');
  const elements = new Map();
  for (const m of html.matchAll(/<([a-z][a-z0-9-]*)\b[^>]*\bid="([^"]+)"[^>]*>/g)) {
    if (elements.has(m[2])) throw new Error(`Duplicate HTML ID ${m[2]}`);
    elements.set(m[2], new Element(m[1], m[2]));
  }
  const calls = [], handlers = new Map(), timers = [];
  let response = clone(initial);
  const walk = e => [e, ...e.children.filter(c => c instanceof Element).flatMap(walk)];
  const document = {
    getElementById(id) { const e = elements.get(id); if (!e) throw new Error(`Unknown DOM ID ${id}`); return e; },
    createElement(tag) { return new Element(tag); },
    querySelectorAll(selector) { const tags = selector.toUpperCase().split(','); return [...elements.values()].flatMap(walk).filter(e => tags.includes(e.tagName)); },
  };
  const context = vm.createContext({ document, Option, console, setTimeout(fn, ms) { timers.push({ fn, ms }); }, Call: { async ByName(name, ...args) {
    const method = name.replace('main.Gym.', ''); calls.push({ method, args: clone(args) });
    if (handlers.has(method)) return await handlers.get(method)(...args);
    if (method === 'OwnedSnapshot') return clone(response);
    throw new Error(`Unexpected native call: ${method}`);
  } } });
  const instrument = `\nglobalThis.testUI={refresh,controls,renderStable,renderTransfer,renderOperation,renderHistory,prepareEntry,amount,clearView,
    set(v){if('snapshot'in v)snapshot=v.snapshot;if('stable'in v)stable=v.stable;if('page'in v)page=v.page;if('history'in v)history=v.history;if('entry'in v)entry=v.entry;if('slot'in v)slot=v.slot;if('entryIntent'in v)entryIntent=v.entryIntent;if('request'in v)request=v.request;if('transfer'in v)transfer=v.transfer;if('dialogPending'in v)dialogPending=v.dialogPending;},
    state(){return {snapshot,stable,page,history,entry,slot,entryIntent,request,transfer,dialogPending,busy};}};`;
  // Strip only the one browser module import, leaving all implementation and startup code intact.
  const executable = source.replace(/^import \{ Call \} from '\/wails\/runtime\.js';\r?\n/, '');
  if (executable === source) throw new Error('Unexpected module import');
  vm.runInContext(executable + instrument, context, { filename: jsPath, timeout: 1000 });
  for (let n = 0; n < 12; n++) await Promise.resolve();
  if (context.testUI.state().busy) throw new Error('Initial poll did not join');
  calls.length = 0;
  return { $: id => document.getElementById(id), api: context.testUI, calls, handlers, timers, snapshot(v) { response = clone(v); }, all: () => document.querySelectorAll('button,input,select') };
}
export function deferred() { let resolve, reject; const promise = new Promise((a, b) => { resolve = a; reject = b; }); return { promise, resolve, reject }; }
