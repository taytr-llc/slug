# Result-derived awards — D78

Mike approved this decision on November 18, 2026 Pacific, during Set49:

> Award permanent records/XP at result finality, with derived points and titles
> on the same rule and provisional results labeled until then.

This applies D62 to the consequences of a result. It changes no XP quantity,
point value, rating coefficient, title-selection rule or chain-consensus rule.

## Effective history

A result is provisional until its own signed checkpoint and at least sixty
descendant blocks establish finality. The inclusion block does not count as a
descendant. Finality of the event's seed does not finalize its fight results.

Professional record, D42 XP, D41 ranking points and every result-driven title
change take effect together, exactly once, at result finality. A provisional
result can be displayed immediately but cannot contribute to effective Skill,
rating, points, standings or title status. Label it provisional wherever its
outcome appears, including fighter pages and rankings. Unrecorded practice
previews remain explicitly unrecorded; they do not become provisional official
results merely because they use the same simulator.

Replay, duplicate observation, restart, lost acknowledgment and re-inclusion
of the same transaction must not award a result twice. One result must account
for both fighters consistently. Preserve the original result identity,
attribution, finality evidence and effective history rather than relying on
mutable displayed status or blindly incrementing a counter.

If the chain later conflicts with that checkpoint, retain the finalized awards
and publicly report the conflict. Community resolution under D62 is separate
from automatic accounting and from node chain selection. A current conflict
label must not erase previously retained finality or relabel finalized history
as a never-finalized provisional result.

## Noon snapshots

Each D76 snapshot reads only awards already effective when it locks. Later
finalization cannot change that event's saved record, condition, rating or
other inputs. Use an auditable, durable ordering and authenticated history;
do not backdate an award from a claimant timestamp, block timestamp, later
observation or restored cache. Application recovery must reconstruct the same
effective history and prevent a late write from altering a saved snapshot.

## Preserved rules and implementation boundaries

- D42 remains +1 XP per completed standard professional fight and +2 for a
  title fight, capped at +15. D65 and condition version 1 are unchanged.
- D41 point values and opponent-quality bonuses stay unchanged. Published
  ranking positions require the separately defined ranking inputs; missing
  ranking authority is not permission to invent an opponent's rank.
- Result-driven title awards and title eligibility effects use D78. D39's
  independent retirement and time-driven vacancies continue to apply.
- C1 remains a zero-professional-record candidate exception. An awarded fighter
  cannot silently reuse that exception. General matchmaking, record filters,
  widening and next-week priority remain separate work.
- Existing Set47/48 evidence retains its original version and scope. New
  accounting needs an explicit authority/version boundary; a copied receipt,
  parsed `Finality` value or caller-provided boolean is not award authority.
- Fixture acceptance does not enroll production authority, launch permanent
  genesis or satisfy the joint D74 design review.

## Required verification

Independent expectations must cover no award before finality, both-fighter
atomicity, all supported outcomes, XP cap, exact replay, concurrent attempts,
restart and uncertain-write recovery, 59/60/61 descendants, a checkpoint on
the wrong result, re-inclusion, retained awards after later conflict, and
before/at/after-cutoff behavior without historical snapshot changes. Verify
every implemented outcome display labels provisional results and distinguishes
retained finalized history from present chain health. Broader point/title
mechanics require their own inputs and tests when those flows are enabled.
