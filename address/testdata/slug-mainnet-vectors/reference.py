#!/usr/bin/env python3
"""Independent, standard-library-only Base58Check/Bech32 fixture translation."""
import hashlib
import json
from pathlib import Path
import sys

BASE58 = '123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz'
BECH32 = 'qpzry9x8gf2tvdw0s3jn54khce6mua7l'
GENERATORS = (0x3b6a57b2, 0x26508e6d, 0x1ea119fa, 0x3d4233dd, 0x2a1462b3)


def sha256d(raw):
    return hashlib.sha256(hashlib.sha256(raw).digest()).digest()


def b58encode(raw):
    n = int.from_bytes(raw, 'big')
    out = ''
    while n:
        n, digit = divmod(n, 58)
        out = BASE58[digit] + out
    return '1' * (len(raw) - len(raw.lstrip(b'\0'))) + out


def b58decode(text):
    n = 0
    for c in text:
        n = n * 58 + BASE58.index(c)
    raw = b'\0' * (len(text) - len(text.lstrip('1')))
    raw += n.to_bytes((n.bit_length() + 7) // 8, 'big')
    assert len(raw) == 25 and sha256d(raw[:-4])[:4] == raw[-4:]
    assert b58encode(raw) == text
    return raw[0], raw[1:-4]


def b58check(version, payload):
    body = bytes([version]) + payload
    return b58encode(body + sha256d(body)[:4])


def polymod(values):
    chk = 1
    for value in values:
        top = chk >> 25
        chk = ((chk & 0x1ffffff) << 5) ^ value
        for i, g in enumerate(GENERATORS):
            if (top >> i) & 1:
                chk ^= g
    return chk


def expand(hrp):
    return [ord(c) >> 5 for c in hrp] + [0] + [ord(c) & 31 for c in hrp]


def convert(data, frombits, tobits, pad):
    accumulator = 0
    bits = 0
    out = []
    for value in data:
        assert 0 <= value < (1 << frombits)
        accumulator = (accumulator << frombits) | value
        bits += frombits
        while bits >= tobits:
            bits -= tobits
            out.append((accumulator >> bits) & ((1 << tobits) - 1))
    if pad and bits:
        out.append((accumulator << (tobits - bits)) & ((1 << tobits) - 1))
    elif not pad:
        assert bits < frombits and not ((accumulator << (tobits - bits)) & ((1 << tobits) - 1))
    return out


def bechencode(hrp, data, constant):
    mod = polymod(expand(hrp) + data + [0] * 6) ^ constant
    check = [(mod >> (5 * (5 - i))) & 31 for i in range(6)]
    return hrp + '1' + ''.join(BECH32[n] for n in data + check)


def bechdecode(text):
    assert text == text.lower() or text == text.upper()
    text = text.lower()
    split = text.rfind('1')
    hrp = text[:split]
    values = [BECH32.index(c) for c in text[split + 1:]]
    assert len(values) >= 7 and 0 <= values[0] <= 16
    constant = 1 if values[0] == 0 else 0x2bc830a3
    assert polymod(expand(hrp) + values) == constant
    data = values[:-6]
    program = bytes(convert(data[1:], 5, 8, False))
    assert data[1:] == convert(program, 8, 5, True)
    assert bechencode(hrp, data, constant) == text
    return hrp, data[0], program, data, constant


def main():
    # Independent standard algorithm controls, not values from the Go output.
    assert b58check(0, bytes(20)) == '1111111111111111111114oLvT2'
    assert polymod(expand('a') + [BECH32.index(c) for c in '2uel5l']) == 1
    assert polymod(expand('a') + [BECH32.index(c) for c in 'lqfn3a']) == 0x2bc830a3
    base = Path(__file__).resolve().parent
    lock = json.loads((base / 'source-ready.json').read_text())
    for item in lock['inputs']:
        raw = (base / item['path']).read_bytes()
        assert len(raw) == item['bytes']
        assert hashlib.sha256(raw).hexdigest() == item['sha256']
    recipe = json.loads((base / 'inputs/recipe.json').read_text())
    vectors = []
    for row in recipe['base58']:
        version, payload = b58decode(row['original'])
        assert version == row['old_version']
        changed = b58check(row['new_version'], payload)
        assert b58decode(changed) == (row['new_version'], payload)
        vectors.append(dict(row, encoded=changed, payload_hex=payload.hex()))
    for row in recipe['segwit']:
        hrp, version, payload, data, constant = bechdecode(row['original'])
        assert (hrp, version, payload.hex()) == (row['old_hrp'], row['witness_version'], row['program_hex'])
        changed = bechencode(row['new_hrp'], data, constant)
        assert bechdecode(changed) == (row['new_hrp'], version, payload, data, constant)
        vectors.append(dict(row, encoded=changed, payload_hex=payload.hex()))
    negatives = []
    for index, alphabet, decode in ((0, BASE58, b58decode), (7, BECH32, bechdecode)):
        good = vectors[index]['encoded']
        bad = good[:-1] + alphabet[(alphabet.index(good[-1]) + 1) % len(alphabet)]
        try:
            decode(bad)
        except AssertionError:
            pass
        else:
            raise AssertionError('mutated checksum accepted by independent decoder')
        negatives.append({'name': vectors[index]['name'] + ' bad checksum', 'original_positive': good, 'encoded': bad})
    out = {'version': 1, 'vectors': vectors, 'checksum_refusals': negatives}
    Path(sys.argv[1]).write_text(json.dumps(out, indent=2) + '\n')


if __name__ == '__main__':
    main()
