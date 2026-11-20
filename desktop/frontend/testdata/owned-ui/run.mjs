// Portable behavioral checks for the exact checkout; no native/wallet authority.
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const desktop = path.resolve(here, '../../..'), root = path.dirname(desktop);
const output = process.argv[2];
if (!output || !path.isAbsolute(output)) throw new Error('Supply a fresh absolute artifact directory.');
const sha = p => crypto.createHash('sha256').update(fs.readFileSync(p)).digest('hex');
const corpus = path.join(root, 'gymstable/testdata/owned-reference/owned-consumers-v1.json');
if (sha(corpus) !== '44f3d12b96a94f6d2d071270b6c90296cae1db50249ff4725ec5e28adc9bfa15') throw new Error('The independently frozen owned-consumer corpus changed.');
const js = path.join(desktop, 'frontend/owned.js'), html = path.join(desktop, 'frontend/owned.html');
const native = ['owned.go', 'owned_recovery.go'].map(p => path.join(desktop, p));
const sources = [js, html, ...native,
  path.join(desktop, 'pilot/owned_gym_protocol.go'), path.join(desktop, 'pilot/owned_gym_types.go'),
  ...['gymstable/owned_types.go', 'gymentry/types.go', 'gymevent/owned_types.go', 'internal/careerstate/history.go'].map(p => path.join(root, p)),
  corpus, ...['run.mjs', 'test.mjs', 'harness.mjs', 'README.md'].map(p => path.join(here, p))];
const lock = { version: 1, scope: 'Exact checkout browser behavior with mocked native calls; no visible GUI or transaction-authority claim', js, html, native, corpus,
  files: sources.map(p => ({ path: p, sha256: sha(p) })) };
fs.mkdirSync(output, { recursive: false });
const lockPath = path.join(output, 'input-lock.json');
fs.writeFileSync(lockPath, JSON.stringify(lock, null, 2) + '\n', { flag: 'wx' });
process.argv = [process.execPath, path.join(here, 'test.mjs'), lockPath, path.join(output, 'result')];
await import('./test.mjs');
