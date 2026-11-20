"""Independent conservative capacity arithmetic, not product execution.

Literal JSON field shapes/caps and the reviewed lineage proof only. No product
imports, expected transaction bytes, persistent financial state or authority.
The predecessor bounds.py/bounds.json remain unchanged.
"""
import json
import math

MAX_MONEY = 21_000_000 * 100_000_000
MAX_I64 = (1 << 63) - 1
MAX_STATE = 64 << 20
STATUS_RESERVE = 256 * (4 * 4096 + 2048) + 4096
MAX_ENTRY = 64 << 10
ROUND_STATUS = 4096
H = 'f' * 64
SCRIPT = '0014' + 'f' * 40


def size(value):
    return len(json.dumps(value, separators=(',', ':'), ensure_ascii=True).encode('ascii'))


def case(n, r, gyms=64):
    assert 1 <= r <= n <= 256 and 1 <= gyms <= 64
    allocations = n  # global partition bound, even for an old unknown payee
    claims = allocations + r
    batches = 2 * r - 1
    # R completed disjoint nonempty windows contain <=N shares. Any one
    # window therefore contains <=N-R+1. This L is intentionally larger than
    # the lcm of the actual window sizes and still divides every remainder.
    window = n - r + 1
    denominator = 100 * math.lcm(*range(1, window + 1))
    fraction = dict(numerator=str(MAX_MONEY * denominator), denominator=str(denominator))
    claim = dict(round_hash=H, round_height=2147483647, gym_id=H, amount=fraction)
    coin = dict(txid=H, index=4294967295, atoms=MAX_MONEY, script=SCRIPT,
                coinbase=False, block_hash=H, block_height=2147483647, claims=[])
    credit = dict(round_hash=H, round_digest=H, coin=coin)
    payment = dict(gym_id=H, script=SCRIPT, atoms=MAX_MONEY)
    # 20 date characters and int64 maximum conservatively cover even times
    # outside the scheduler's supported range; no year-9999 assumption.
    batch = dict(day='9' * 20, created_at_ms=MAX_I64, inputs=[], payments=[],
                 fee_atoms=MAX_MONEY, change=None, txid=H, raw='', state='confirmed')
    inputs = r + batches  # each root/change may appear as input at most once
    claim_copies = 2 * claims * r  # input plus change along <=R batch lineage
    outputs_per_batch = min(gyms, n) + 1
    # Native22-byte outputs; input base41 + witness<=109 (DER<=72 plus
    # sighash, compressed pubkey). 16 covers version/marker/varints/locktime,
    # including 3-byte varints at the existing256-input boundary.
    signed_bytes_total = 16 * batches + 150 * inputs + 31 * outputs_per_batch * batches
    state = (65536 + STATUS_RESERVE
             + r * (size(credit) + 1) + claims * (size(claim) + 1)
             + batches * (size(batch) + 1)
             + (inputs + batches) * (size(coin) + 1)
             + claim_copies * (size(claim) + 1)
             + batches * min(gyms, n) * (size(payment) + 1)
             + 2 * signed_bytes_total)
    allocation = dict(gym_id=H, shares=n, numerator=str(MAX_MONEY * n), denominator=str(n))
    round_base = dict(version=1, sequence=n, block_hash=H, block_height=2147483647,
                      first_accepted=n, last_accepted=n, share_keys=[], pool_target=H,
                      payout_address='p' * 128, reward_atoms=MAX_MONEY, allocations=[], winner=None)
    # Every actual Winner has already passed poolround's64KiB accounting
    # entry limit. Closed windows partition share keys and allocations.
    round_bytes = (r * (size(round_base) + MAX_ENTRY + ROUND_STATUS)
                   + n * 67 + allocations * (size(allocation) + 1))
    largest_round = size(round_base) + MAX_ENTRY + n * 67 + n * (size(allocation) + 1)
    return dict(submissions=n, rounds=r, allocations=allocations, claims=claims,
                batches=batches, largest_window=window, denominator_digits=len(str(denominator)),
                max_claim_bytes=size(claim), historical_inputs=inputs,
                historical_claim_copies=claim_copies, signed_bytes_total=signed_bytes_total,
                state_bytes=state, round_bytes=round_bytes, largest_round_bytes=largest_round)


def bound(n, gyms=64):
    rows = [case(n, r, gyms) for r in range(1, n + 1)]
    state = max(rows, key=lambda row: row['state_bytes'])
    rounds = max(rows, key=lambda row: row['round_bytes'])
    return dict(submissions=n, max_batches=2 * n - 1,
                state_bytes=state['state_bytes'], state_max_at_rounds=state['rounds'],
                round_bytes=rounds['round_bytes'], largest_round_bytes=max(x['largest_round_bytes'] for x in rows),
                state_fits=state['state_bytes'] <= MAX_STATE - STATUS_RESERVE,
                round_row_fits=max(x['largest_round_bytes'] for x in rows) <= 2 << 20)


def envelope(source_max, rounds, round_bytes, credit_rounds, batches, gyms):
    best = 0
    for n in range(1, min(source_max, rounds, credit_rounds, 256) + 1):
        b = bound(n, gyms)
        if b['max_batches'] <= batches and b['round_bytes'] <= round_bytes and b['state_fits'] and b['round_row_fits']:
            best = n
    return best


if __name__ == '__main__':
    output = dict(version=2,
                  scope='independent conservative arithmetic; lineage proof pending parent review; not runtime evidence',
                  max_state_bytes=MAX_STATE, status_reserve_bytes=STATUS_RESERVE,
                  new_state_limit=MAX_STATE-STATUS_RESERVE,
                  rows=[bound(n) for n in (1,4,16,64,127,128,183,184,256)],
                  maximum_current_configs=envelope(4096,256,1<<30,256,366,64),
                  current_256_mib_rounds=envelope(4096,256,256<<20,256,366,64),
                  four_row_eight_batch=envelope(4,4,8<<20,4,8,2),
                  round_4096_bytes=envelope(4096,256,4096,256,366,64))
    print(json.dumps(output, indent=2))
