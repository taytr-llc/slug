# Change set 35 — exact candidate current condition

Branch: `slug-fighter-condition`, stacked on set 34 implementation
`242bbaf1c116918bf5c7171707d806a45ba6aa84`.

## What changed

`fightercondition.Project` validates complete canonical DNA and projects the
specified D21/D22/D43 aging model from positive supplied birth/observation
milliseconds. Completed seven-day duration weeks determine prime, decline and
retirement; timezone/DST and calendar event locks do not change this duration.
Aging uses exact integer tenths and nearest-even rounding. Original attributes
remain unchanged, including low attributes whose aged values are below 1.0.
Final declined condition is retained after retirement for exhibitions, with
no extra floor. Accounted professional XP is added afterward to Skill, without
an additional Skill cap. The exact D65 rating is in integer hundredths and
excludes weekly training and fight form.

The helper has no clock, IO, event history or authority. Callers must separately
supply verified immutable birth, operation time and already-accounted XP.
It returns no ownership, admission or general professional-eligibility claim.
The new offline `slugfighter condition` route accepts explicit public hash,
birth/time and XP inputs. Existing DNA, manifest and portrait routes remain.
`FIGHTER_CONDITION.md` freezes the candidate arithmetic and DTO contract.

## Independent validation

The independent reference checks twelve canonical fighters, 13950 exact full
snapshots, 48 invalid inputs, all 291600 valid attribute/decline/step/loss cells
and all 7836 exact half-ties. It includes millisecond boundaries, retirement,
near-int64-maximum times, age beyond a 32-bit integer, XP ordering, canonical
attribute 1 and post-XP Skill above 100. Three complete reference regenerations
passed under varied timezone/hash seed with fifteen captured files unchanged.

The Python prototype has 2698 binary-float half-tie disagreements and 1802
selected rating differences under an explicit source-order binary64 model;
these are retained as nonnormative diagnostics,
not copied into golden values. The exact integer model preserves the specified
percentages and one-decimal rounding intent across platforms. Initial integrated
regeneration exposed Python 3.9/3.14 float-sum differences in diagnostic metadata
only. The fixed oracle separates that metadata from normative snapshots and
uses the explicit legacy diagnostic model. Both exact interpreters pass, and
all original snapshots, DNA, negatives, coverage, grid and ties are unchanged.
The first attempt and prior reference are preserved in the workspace.

Staged normal/race tests passed in 0.396/3.259 seconds with all 26 captured files
unchanged. Vet/format and 844568 fuzz executions passed. A Linux/386 test binary
compiled; it was not runtime-tested. Independent review found no actionable
issue. Root integration, full builds, CLI checks and normal-PoW smoke have
separate workspace reports; seed release and observation are not implied by
these local results.

The hash-pinned verbatim lifecycle excerpt retains its original terminal blank
line. A single-path Git whitespace attribute preserves those reference bytes;
ordinary code formatting and whitespace checks remain enabled.

## Preserved behavior and remaining integration

No consensus, wallet, job, receipt, birth or ownership rule changes. The
projection does not itself grant completed-fight XP, select roster slots,
vacate titles or authorize exhibitions/transfers. These need authoritative
history and event rules. It does not mutate mined attributes or independently
establish a fighter's age from caller assertions.

The current Mac GUI remains set32; this is a pure library/CLI slice. Mike chose
to keep the portrait rendering check pending; actual visual/screen-reader
acceptance remains open before player-facing portrait integration. D74's
thorough joint design review is still required before product release.
