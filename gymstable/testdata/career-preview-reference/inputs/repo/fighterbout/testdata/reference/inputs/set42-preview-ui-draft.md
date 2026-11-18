# Set42 preview interaction draft

Root engineering draft, November17 UTC. This prepares the same candidate app for
an unrecorded bout; it is not Mike's D74 design review, a rendering check or an
approved model/protocol contract. Exact model and integration contracts must
freeze before implementation or normative test values.

## Player journey

Add a compact Fight Preview section to the existing gym, retaining the approved
logo and current account, wallet, mining, stable and roster controls. It uses
saved candidates from the signed-in gym. No separate app, signup route, sample
fighter array, new wallet or data namespace.

Each verified inventory/roster card has a Select for preview action. Selection
retains at most two full IDs plus bounded static display data, survives ordinary
paging, and clears when the full account view changes. It does not reserve a
roster slot or pin an old current-condition observation as a fresh combat input.
Selecting the same fighter twice is refused. Once one is selected, explain that
the second must be from the same weight class; the helper independently checks
this. If there is no suitable candidate, describe that state without silently
mining, altering a class or supplying a synthetic opponent.

The panel shows the two selected names and class, a clear/remove action per
selection, and Preview fight. The persistent explanatory line is: "Unrecorded
preview. No fight record, training or XP is awarded." The action is explicit;
new arrivals, polling, paging, roster edits and replay never start a new bout.
Current authenticated held sessions may preview under existing D72 rules.

Begin captures the current full view and pair. While verification is queued or
running, show a short Preparing fight state and a Cancel action. Existing Stop,
Sign out and Reset controls retain priority. Changing a selection immediately stops playback, releases the old result and
timers, and cancels its obsolete queued/running preview. It never automatically
begins a replacement; Preview fight remains an explicit action. Late replies
cannot replace the new selection or another account's view. The exact backend
job API and cancellation semantics are owned by the integration contract.

A successful result replaces selection display data with the two authoritative
inputs computed at the same observation time. Show that dated condition near
the preview and keep it fixed throughout playback. Canonical engine corner IDs
are presentation order; swapping selection order is not a request to reroll the
same recipe. There is no UI seed, form, XP, training, round-count or winner input.

## Replay and scorecard

Playback uses the already computed bounded transcript. It does not rerun the
engine, read source receipts, hash work, contact a node or alter the saved gym.
Pause/Play, Previous step, Next step, Replay and Show result operate locally.
The default motion should be restrained: existing portrait images, a textual
round/tick summary and visible event list, without flashing or mandatory
animation. Reduced-motion preference disables automatic timed advancement;
all content is available through stepping and text alone. An exact initial
playback cadence is presentation-only and can be adjusted without a model
version change.

Use only structured event enums and validated numeric fields to construct text;
never inject helper-provided HTML. Show the final outcome/finish reason and the
single whole-fight scorecard with each settled scoring component and its total.
Round separators help navigation but do not imply round-by-round judging.
An early finish ends playback immediately; no later event is fabricated. A
scorecard decision draw must be the exact published score tie established by
the engine contract. Any separately defined terminal draw, such as a mutual
corner stoppage, must display its distinct finish reason rather than claim a
tied decision.

No title belt, league points, prize, wager, training counter, permanent damage
or record appears as an earned result. A later recorded exhibition is a separate
operation with its own authority, acceptance and idempotent awards. Displaying
this preview is not a shortcut to that state.

## State, ownership and accessibility

Keep the latest result only and release old images, object URLs, timers and
arrays when a result is replaced, a view is lost or the page is unloaded.
Charge retained selection summaries/portraits, the latest transcript and existing
page/images against one explicit aggregate cache budget; selection does not
create an unbounded second image cache. Account polling remains serialized with
new native work. Local playback must
not occupy the native request scheduler or prevent account controls. A generic
engine failure clears no roster, assignment, operation or wallet state.

If source verification fails, show a fixed understandable error and keep the
selected IDs available for a deliberate retry when the current view still
exists. If the view is revoked, clear the selection/result. Cancellation after
publication may mean the completed preview is discarded; there is no durable
fight mutation to reconcile. Existing stable mutation reconciliation remains
unchanged and takes priority over this optional view operation.

Provide named headings, ordinary native HTML buttons, visible focus, keyboard
step controls, a textual score table with headers, and concise status messages.
Do not announce every timed strike to a screen reader. Announce readiness,
cancellation, view loss and completion; let the event list be read on demand.
Move focus only after an explicit navigation action, preserving it through
polling. DOM and reduced-motion tests are evidence of behavior, not a claim of
actual screen-reader or pixel acceptance.

## Required evidence

Use actual admitted same-gym, same-class candidates in packaged acceptance;
existing three-per-gym references do not guarantee a pair. Mining remains an
explicit test action on owned private nodes. Independently verify both full
source records, common condition time and the exact final transcript. Capture
that replay makes no new helper request and that successful/cancelled previews
leave inventory checkpoint, assignments, roster revision/slots, operation
history, XP and wallet unchanged. Existing observation metadata may advance
only as the trusted-read contract explicitly permits.

Test empty selection, duplicate/mismatched class, paging with one selection,
selection replacement, stale job responses, cancellation, login/logout/reset,
held current session, unavailable stable data, keyboard/reduced-motion playback,
terminal finish handling and old view cleanup. Verify Play/Replay are local-only,
a changed reduced-motion preference stops timed playback, detached selection or
result replies are discarded, and account/Stop controls work during playback. Keep old frontend behavior and
closed protocol grammars under regression tests. Real rendering, native
interaction, accessibility and the thorough joint D74 session remain pending.
