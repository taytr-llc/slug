# Statistics accelerator interface

This is a bounded numerical helper for the frozen D75 fitting procedure, not a fight engine or source of normative fight outputs.

`training.dat`: 8 ASCII bytes `D75TRN01`, LE32 row count 4,000, LE32 feature count 5; then exactly 4,000 rows. Each row is five signed LE32 league-attribute-tenths differences, followed by LE32 half-label in {0,1,2}. Total 96,016 bytes. Features are converted to double(diff)/500 and labels to double(half-label)/2.

`indices.dat`: 8 ASCII bytes `D75IDX01`, LE32 replica count 1,000, LE32 row count 4,000; then exactly 1,000 × 4,000 unsigned LE16 absolute training-row indices in replica/class/draw order. The **payload** is 8,000,000 bytes; the full file is 8,000,016 bytes including its 16-byte header. This clarifies the protocol's payload size without changing any sampling byte.

CLI: `fit_accelerator TRAIN INDICES OUT --original` or `fit_accelerator TRAIN INDICES OUT --range FIRST COUNT`. Original uses rows 0 through 3,999 in order; its replica ID is −1. Both input files are fully shape/range validated before exclusive output creation. No overwrite is permitted.

Each successful output line is a JSON object with replica ID, iterations 400, and exactly five `weights_hex` strings. Hex floating point preserves raw binary64 values without decimal formatting ambiguity. Python owns normalization, validity, covariance, intervals and rounded frequencies. Nonfinite arithmetic emits a fixed diagnostic row with replica ID, iteration, error and all five values, then exits unsuccessfully; partial output stays for diagnosis. Negative finite coefficients are preserved.

No executable fit has run during source preparation. The final source/input freeze and root's execution release are separate requirements.
