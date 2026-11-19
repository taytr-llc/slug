// Presentation-only namespace adaptation of independently frozen fixtures.
// IDs, signatures and transcripts stay opaque correlated transport values;
// these are not newly mined, signed or cryptographic golden vectors.
import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';
import {readFileSync} from 'node:fs';

const bytes = readFileSync(new URL('../../gymstable/testdata/shared-reference/shared_v1.json', import.meta.url));
assert.equal(createHash('sha256').update(bytes).digest('hex'), '29b0aa42752e3db62afed4c5800373a9ada39fb1c9fd4319110d9725eafab225');
const reference = JSON.parse(bytes), sharedSource = reference.pins[0].pin.source;

export function sharedTransport(value) {
  if (Array.isArray(value)) return value.map(sharedTransport);
  if (value === null || typeof value !== 'object') return value;
  if (value.version === 5 && value.descriptor_id && value.authority_id && value.genesis) return structuredClone(sharedSource);
  if (value.identity?.gym_id && value.condition_version && value.projector_id && value.source) {
    const fixture = reference.pins.find(v => v.pin.identity.gym_id === value.identity.gym_id);
    assert.ok(fixture, 'Only the two published fixture gyms may enter this transport model');
    return {...structuredClone(value), version: fixture.pin.version, source: structuredClone(sharedSource)};
  }
  const result = Object.fromEntries(Object.entries(value).map(([key, item]) => [key, sharedTransport(item)]));
  for (const key of ['publisher_source_authority_id', 'source_authority_id']) {
    if (typeof result[key] === 'string') result[key] = sharedSource.authority_id;
  }
  return result;
}
