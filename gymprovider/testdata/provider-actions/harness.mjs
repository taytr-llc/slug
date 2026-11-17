// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.
// TEST MODEL ONLY. The injected ports below are not Auth0 APIs or a live adapter.

export const MAX_ACTION_MS = 18_000;
export const MAX_UINT64 = (1n << 64n) - 1n;
const DENIED = 'SLUG account operation unavailable.';
const id = value => typeof value === 'string' && /^[0-9a-f]{64}$/.test(value);
const label = value => typeof value === 'string' && /^[A-Za-z0-9._-]{1,128}$/.test(value);
const text = value => typeof value === 'string' && value.length > 0 && Buffer.byteLength(value) <= 1024;
const seconds = value => Number.isSafeInteger(value) && value > 0 && value <= Math.floor(Number.MAX_SAFE_INTEGER / 1000);
const fail = () => { throw new Error(DENIED); };

// A raw JSON integer token, not a parsed Number or a quoted JSON value. This
// boundary must precede Number conversion; rounded values cannot be recovered.
export function parseEpochToken(raw) {
  if (typeof raw !== 'string' || !/^[1-9][0-9]{0,19}$/.test(raw)) fail();
  const value = BigInt(raw);
  if (value > MAX_UINT64) fail();
  return value;
}

export function epochJSON(value) {
  if (typeof value !== 'bigint' || value < 1n || value > MAX_UINT64) fail();
  return value.toString(10);
}

export function nextEpoch(value) {
  epochJSON(value);
  if (value === MAX_UINT64) fail();
  return value + 1n;
}

const loginFields = 'iss aud sub iat exp auth_time nonce slug_auth_epoch provider_version coordinator_id authority_id tenant_id client_id connection_id login_flow_id original_auth_id original_flow_id auth_mode'.split(' ');
const resetFields = 'iss aud sub iat exp event_id stage from_epoch to_epoch verified_at provider_version coordinator_id authority_id tenant_id client_id connection_id reset_nonce provider_operation_id'.split(' ');
const epochFields = new Set(['slug_auth_epoch', 'from_epoch', 'to_epoch']);
const timeFields = new Set(['iat', 'exp', 'auth_time', 'verified_at']);

// Narrow exact serializer for the already-typed fixture claim contract. It is
// deliberately not a JWS signer/parser and does not call JSON.parse on epochs.
export function serializeClaims(kind, claims) {
  const fields = kind === 'login' ? loginFields : kind === 'reset' ? resetFields : fail();
  if (!claims || Object.getPrototypeOf(claims) !== Object.prototype ||
      Object.keys(claims).length !== fields.length || fields.some(k => !Object.hasOwn(claims, k))) fail();
  const json = '{' + fields.map(key => {
    const value = claims[key];
    let encoded;
    if (epochFields.has(key)) encoded = epochJSON(value);
    else if (timeFields.has(key)) {
      if (!seconds(value)) fail();
      encoded = String(value);
    } else if (key === 'provider_version') {
      if (value !== 1) fail();
      encoded = '1';
    } else {
      if (!text(value)) fail();
      encoded = JSON.stringify(value);
    }
    return JSON.stringify(key) + ':' + encoded;
  }).join(',') + '}';
  if (Buffer.byteLength(json) > 8192) fail();
  return json;
}

function selector(event, binding, requireClient) {
  const out = {
    tenant_id: event?.tenant?.id, connection_id: event?.connection?.id,
    subject: event?.user?.user_id, correlation_id: event?.transaction?.correlation_id,
    client_id: event?.client?.client_id,
  };
  if (out.tenant_id !== binding.tenant_id || out.connection_id !== binding.connection_id ||
      !text(out.subject) || !text(out.correlation_id) ||
      (requireClient && out.client_id !== binding.client_id) ||
      (out.client_id !== undefined && out.client_id !== binding.client_id)) fail();
  return out;
}

function boundRecord(record, selected, binding) {
  if (!record || record.version !== 1 || record.subject !== selected.subject ||
      record.correlation_id !== selected.correlation_id) fail();
  for (const key of ['coordinator_id', 'authority_id', 'tenant_id', 'client_id', 'connection_id']) {
    if (record[key] !== binding[key]) fail();
  }
  return record;
}

function resetRecord(record, selected, binding, state) {
  boundRecord(record, selected, binding);
  for (const key of ['event_id', 'reset_nonce', 'provider_operation_id', 'account_id', 'principal_id', 'barrier_event_id']) {
    if (!id(record[key])) fail();
  }
  epochJSON(record.from_epoch);
  if (record.to_epoch !== nextEpoch(record.from_epoch) || !seconds(record.verified_at) || record.provider_state !== state) fail();
  return record;
}

function resetAck(ack, record, phase) {
  if (!ack || ack.version !== 1 || ack.phase !== phase || ack.can_commit !== (phase === 'pending')) fail();
  for (const key of ['coordinator_id', 'authority_id', 'event_id', 'provider_operation_id']) {
    if (ack[key] !== record[key]) fail();
  }
  const b = ack.barrier;
  if (!b || b.version !== 2 || b.status !== phase || b.event_id !== record.barrier_event_id ||
      b.account_id !== record.account_id || b.principal_id !== record.principal_id ||
      b.from_epoch !== record.from_epoch || b.to_epoch !== record.to_epoch ||
      b.verified_at_ms !== record.verified_at * 1000) fail();
}

// One shared deadline for the ENTIRE hook, including resolver work, the signed
// coordinator call and durable permit acknowledgement. No per-call reset/retry.
// A timeout cannot cancel an already committed remote write; callers retain the
// same operation and reconcile it, never replace it with a fresh reset.
async function bounded(ms, run) {
  const controller = new AbortController();
  let timer;
  const timeout = new Promise((_, reject) => {
    timer = setTimeout(() => { controller.abort(); reject(new Error(DENIED)); }, ms);
  });
  const call = async fn => {
    if (controller.signal.aborted) fail();
    const result = await fn(controller.signal);
    if (controller.signal.aborted) fail();
    return result;
  };
  try { return await Promise.race([run(call), timeout]); }
  finally { clearTimeout(timer); controller.abort(); }
}

// The resolver is an explicitly trusted, durable fixture service. An Action
// cache, user_metadata, request.body or correlation_id alone cannot supply it.
// The coordinator port represents authenticated signed-evidence transport; it
// does not accept an unsigned tuple at any real gymprovider API.
export function createActionHarness({binding, resolver, coordinator, budgetMS = MAX_ACTION_MS}) {
  if (!Number.isInteger(budgetMS) || budgetMS < 1 || budgetMS > MAX_ACTION_MS ||
      !binding || binding.version !== 1 || !id(binding.authority_id) || !id(binding.coordinator_id) ||
      !label(binding.tenant_id) || !label(binding.client_id) || !label(binding.connection_id)) fail();
  for (const method of ['resolveReset', 'authorizeReset', 'resolveLogin']) {
    if (typeof resolver?.[method] !== 'function') fail();
  }
  for (const method of ['beginReset', 'completeReset', 'finishLogin']) {
    if (typeof coordinator?.[method] !== 'function') fail();
  }
  binding = Object.freeze({...binding});

  return Object.freeze({
    // Actual Auth0 handler spelling. No MFA change is requested by this model.
    async onExecutePostChallenge(event, api) {
      try {
        return await bounded(budgetMS, async call => {
          const selected = selector(event, binding, true);
          const r = resetRecord(await call(signal => resolver.resolveReset(selected, {signal})), selected, binding, 'prepared');
          const ack = await call(signal => coordinator.beginReset(structuredClone(r), {signal}));
          resetAck(ack, r, 'pending');
          // An ACK alone is not a bearer permit: consume only against the same
          // immutable provider operation; later password commit is at most once.
          const saved = await call(signal => resolver.authorizeReset(structuredClone(r), structuredClone(ack), {signal}));
          if (saved?.authorized !== true || saved.event_id !== r.event_id || saved.provider_operation_id !== r.provider_operation_id) fail();
          return {ok: true};
        });
      } catch {
        api.access.deny(DENIED);
        return {ok: false};
      }
    },

    async onExecutePostChangePassword(event) {
      try {
        return await bounded(budgetMS, async call => {
          // Auth0's post-change event has no documented client member. The
          // exact durable mapping must recover the pinned client, not guess it.
          const selected = selector(event, binding, false);
          const r = resetRecord(await call(signal => resolver.resolveReset(selected, {signal})), selected, binding, 'committed');
          const ack = await call(signal => coordinator.completeReset(structuredClone(r), {signal}));
          resetAck(ack, r, 'completed');
          return {ok: true};
        });
      } catch {
        // This asynchronous hook CANNOT roll back a provider password change.
        // Pending authority state stays closed until the SAME operation resumes.
        return {ok: false, outcome: 'reconciliation-required'};
      }
    },

    async onExecutePostLogin(event, api) {
      try {
        return await bounded(budgetMS, async call => {
          const selected = selector(event, binding, true);
          const protocol = event?.transaction?.protocol;
          if (!text(protocol)) fail();
          selected.protocol = protocol;
          // These are public resolver keys, never the refresh token value.
          selected.session_id = event?.session?.id;
          selected.refresh_token_id = event?.refresh_token?.id;
          selected.transaction_id = event?.transaction?.id;
          const r = boundRecord(await call(signal => resolver.resolveLogin(selected, {signal})), selected, binding);
          for (const key of ['login_flow_id', 'nonce', 'original_auth_id', 'original_flow_id']) {
            if (!id(r[key])) fail();
          }
          epochJSON(r.credential_epoch);
          if (!seconds(r.authenticated_at) || !['fresh', 'reuse'].includes(r.mode)) fail();
          if (r.mode === 'fresh' && r.original_flow_id !== r.login_flow_id) fail();
          if (protocol === 'oauth2-refresh-token' && r.mode !== 'reuse') fail();
          const result = await call(signal => coordinator.finishLogin(structuredClone(r), {signal}));
          if (result?.accepted !== true || result.login_flow_id !== r.login_flow_id ||
              result.original_auth_id !== r.original_auth_id || result.credential_epoch !== r.credential_epoch) fail();
          // No session secret or epoch is put in Auth0 token metadata by this
          // model. Provider token delivery/origin proof remains unimplemented.
          return {ok: true};
        });
      } catch {
        api.access.deny(DENIED);
        return {ok: false};
      }
    },
  });
}
