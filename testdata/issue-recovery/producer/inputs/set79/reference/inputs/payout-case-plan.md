# Set67 independent core and wallet oracle — locked case recipes

This phase contains no private-wire vectors yet. The separate desktop wire producer will freeze before its own outputs. These recipes and all producer/input bytes freeze before this phase first runs. Source code, not production output, defines the exact mutation lists.

The only actual data are the separately extracted Set61 original public state, original summary and READY recipient bindings. Two actual positive projections retain the original signed transaction's original output positions and amounts. Neither is a claim of current confirmation, a fresh wallet observation, or authentication supplied by these JSON files. The expected binding is independently recomputed from the old complete source configuration and checked against the original summary/READY markers. Both original paid totals are independently checked. No current app, database, node, wallet or Argon2 computation is invoked.

The inherited Balance fraction ceiling comes from the unchanged pre-Set67 `poolservice/client.go` fraction helper: values, not numerator digit strings, are at most MaxMoney. Existing source binding and legacy wallet result field definitions are separately hash-pinned references. These exact reference bytes are copied from completed Set66 commit247edc868a620dd39ac3bfdf678241066fa4f844 into inherited/, not read from the changing Set67 worktree. No referenced implementation is imported or executed.

## Phase-one outputs

One canonical compact `reference-v1.json` with top-level version, semantics, producer_files, input_files, core, transactions, wallet_rpc. All nested raw JSON/transaction bytes are retained verbatim as UTF-8 strings or hex. `report.json` reports independently computed counts/digest, not implementation acceptance. Expected location after qualification is `poolsettle/testdata/payout-history/reference-v1.json` with this portable producer/input bundle beside it.

### Core: 34 positive values, 43 negative values, 12 raw HTTP cases

Positives: two original journal gym projections; explicitly modeled row counts0/1/8/9/16/256/366; newest prepared and broadcast among9 rows; configured initial report-zero; unconfigured empty and unconfigured unpaid entitlement; all four fraction components exactly4096 digits using coprime values near one; maximum date/index/atoms; all18 inherited Reason strings on mathematically consistent modeled unpaid balances. Modeled rows use a fixed public SHA256 domain and increasing UTC dates from2025-01-01. They are shape/arithmetic scenarios, not invented signed transactions.

Negatives: required/closed/type/version/source-gym/script fields; paid/reserved sums and known reasons; reduced/canonical/nonnegative/bounded fractions including inherited value ceiling; date/calendar/observed-time limits; index/atom/type/state bounds; uppercase txid; duplicate txid/day; ascending order; a balanced older unfinished row;367 rows; unconfigured balance with an incorrect reason. The older unfinished negative adjusts paid/reserved/confirmed so it specifically violates position rather than merely failing the sum.

Raw cases: permitted ASCII padding and an exactly131072-byte body;131073 bytes; Unicode padding; duplicate key; internal whitespace; trailing value; escaped key; negative zero; integer exponent; wrong source; root key reordering. The two permitted raw cases are expected true, the other ten false. All limits count the body newline/padding.

### Transactions: 36 cases

Six valid cases: both original selected outputs in the same actual signed batch; modeled unsigned legacy transaction; modeled witness with one serialized zero-length item (not an absent stack); modeled256 inputs; modeled257 outputs. The latter four are serialization fixtures only.

Thirty rejected cases: four wrong selected txid/index/amount/script controls; trailing byte; seven explicit truncation lengths; eighteen syntax/count/money mutations covering nonminimal CompactSize for input/output/script/witness, zero outputs, negative/over-limit/sum-over-limit values, unknown witness flag, all-empty superfluous witness,257 inputs,258 outputs, huge counts/lengths and raw-size overflow. `parse_only` separates malformed serialization checks from original expected-selection checks. Valid parser outputs include independently recomputed non-witness TxID, input/output counts, witness flag, exact stripped serialization and integer outputs. The original stored TxID remains a separate hard anchor; production hashes never supply it.

### Wallet RPC: 37 cases (6 valid,31 rejected)

Every RPC response and synchronized-tip observation is explicitly modeled around the original signed transaction; none is labeled newly observed actual wallet state. Valid cases cover confirmed/unconfirmed/exact typed missing, optional legacy fields,258 details,256 ignored conflict entries. Amount/Details floats deliberately disagree with selected output values to prove they are not amount oracles.

Rejected cases cover before/after unsynchronized, changed height, same-height changed hash; exact absence code/message/result requirements; envelope ID/version/field grammar; confirmation/type/block requirements; txid/uppercase raw; bounded/non-null details/conflicts; optional-null and unknown detail/result fields; negative vout/long string; missing hex; duplicate names; nonfinite ignored float;524289-byte body. Selection membership/current View/account checks belong to integration tests, not these arbitrary-data models. Empty/filled conflict arrays never assert conflict absence.

## Production independence and execution

`common.py`, `transaction.py`, `wallet.py`, and `generate.py` are standard-library-only. Their source and case recipes must be approved before `reproduce.py` executes. The wrapper checks every locked source/input/contract/dependency hash before producing a new output directory. Failed source-authoring attempts, if any, remain separate; never replace output to fit Go.

The output producer metadata is not a source authenticity signature. No expected network, wallet, key-control, spendability, chain-finality or capacity-performance claim is made. The independent core/HTTP sizing report remains a separate earlier conservative bound and is not rerun or shortened here.
