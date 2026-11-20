# Independent wallet-activity wire reference

The public inputs and producer were reviewed and frozen before first execution.
The corpus contains synthetic transport/identity controls, not authenticated
accounts, new wallet observations or finality claims. Receiver first-seen time
is modeled as zero; only outgoing time uses the old sender observation.

The generator never imports project code or consumes the separate core
producer's outputs. The source case plan states coverage and limits. The first
Python 3.14 run, independent Python 3.9 reproduction and portable reproduction
produce identical bytes (SHA256 fd0f28ff06b4cb0502e7822ad401330941cdb71e9a4d4b5e31d5a017f911dc2b).

From this directory, reproduce to a fresh external path:

```sh
python3 -B producer/generate_wire.py --execute --input-root inputs \
  --approved-producer-lock producer-lock.json \
  --approved-lock-sha256 502b29d2ed03347a31bec10797d75b2a213b1e331d10e7c53d7bc3359de1130b \
  --output /tmp/wallet-activity-wire-fresh.json
```

Case contexts carry exact original expected bindings. Shape negatives must
fail decoding; correlation negatives remain valid shapes and must fail the
original native request/capture/selection comparison. Lifecycle tables are
declarative expectations for separate runtime tests.
