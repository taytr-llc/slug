# Independent remote pool bytes

The public synthetic corpus `reference-v1.json` contains four complete selections,
32 cross-binding refusals, ten raw refusals and four credential fixtures. Its
563,867 bytes have SHA256
`5f67fcaaba0c78b97b0f116a4c598eb0ec62ee1436bbcf280edcc747856e5177`.
Python 3.14.3 and 3.9.6 reproduced identical bytes before Go consumed expectations.

Run `python3 reference/reproduce.py /absolute/new/output.json` to reproduce it
from the locked, portable public inputs. The producer checks its input lock and
requires a fresh output. The original operator setup and local recipient vectors
are independently derived upstream inputs, not output captured from Go.

The preserved JSON-encoded successor diff corrects an invalid initial fixture: operator setup
index0 used the same change destination as the first miner. The successor selects
the existing index2 operator, keeps both local recipients unchanged and explicitly
rejects the collision. The first failed acceptance is not reclassified as a pass.
These are published test identities and credentials, never deployment defaults.
