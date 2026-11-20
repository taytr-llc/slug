# Owned gym desktop protocol v1

Engineering attachment of the approved D83 ownership and recovery contracts to
an explicit `--candidate-owned-gym` helper and `LocalOwnedGymPilot` data root.
The existing screen regions and fist artwork remain the product surface. This
protocol does not alter legacy request families or grant source authority.

Each request and response is one canonical, ordered JSON object on a private
native/helper pipe. One request may be in flight; the response repeats its exact
version, integer id and command. Maximum request8192 bytes, response1048576
bytes. No queue, automatic resubmission or credential appears in the wire data.
All fields are present, including empty inactive fields and null inactive union
arms. Unknown, reordered, repeated, alternate-case and additional fields refuse;
integers have no exponent/fraction and are at most2^53−1. Existing owned family
bounds and validators apply recursively; a page contains at most8 fighters.

The controller owns the source, account epoch, concrete child and original
view. Native keeps the original helper instance and complete observed view with
each pending operation, dialog, capture and response. No replacement login can
complete another view's request. Public source identity comes from the selected
root's original setup; an object echoed through the webview is not configuration.
Owned DTOs remain distinct from assignment-only legacy DTOs.

The scope `carrier` object preserves its inherited exact ordered fields
`JournalID`, `Pin` (capitalized). The embedded Pin retains its existing explicit
lowercase field names. These are spelling requirements, not case-insensitive
aliases. Carrier BindingID and persisted journal markers continue to use their
separate existing lowercase `journal_id`/`pin` wrappers; no digest changes.

Startup returns only the original childless source identity and account/reset
state. Native retains it, then loads both complete fixture scopes from that
same helper. A pending reset permits only snapshot and exact resume_reset until
the barrier completes; opening wallet scopes waits. Replies cannot change the
retained source. Native menu actions capture their original view at the click
and dispatch once, without retrying against a later login.

Control commands: snapshot, login_fixture, logout, reset_fixture, resume_reset,
start, stop.
All child commands carry the original view_id. Stable commands: capture_stable,
stable_page, prepare_slot, apply_slot, slot_operation, pending_slot, abandon_slot.
History commands: capture_history, history_page. Entry commands: entry_upcoming,
entry_status, prepare_entry, apply_entry, entry_operation, pending_entry,
abandon_entry. Event commands: event_status, event_record. These return only the
original concrete captured data, bounded pages and retained mutations.

Transfer commands: transfer_prepare, transfer_resume, transfer_confirm,
transfer_review_delivery, transfer_deliver, transfer_observe,
transfer_operation, transfer_abandon, transfer_finality. Prepare carries the native preselected,
retained request_id, prospect_id and recipient_gym. Resume uses that exact
request_id/prospect_id. Confirmation carries only the original process-local
review_id. Delivery review selects an exact retained request and commit/spend
leg; delivery consumes only its original review_id. Existing signed transaction
bytes are never accepted as request input. Opening, observing and recovering a
request never broadcast. Explicit retry transmits only its retained leg.
Native persists immutable public request correlation before helper preparation,
so a restart can select the exact original request. Delivery completion is not
ownership finality: transfer_finality independently reads the public replay's
pending, void or finalized state at its captured head.

Recovery export carries the original selected destination descriptor and view;
the native dialog retains the physical selection through helper handoff. The
existing export2 authority gates publication. Backup verification uses an
inspection-only schema12 scratch wallet. Independent recovery activates schema15
only after ready scan and uses public ownership reconstruction, without hosted
sessions or their private journals. Its separate packaged command must preserve
that boundary. Account sign-in/provider work remains deferred.

Native confirms a transfer with the complete fighter identity, sender and
recipient gym keys, fee and return details from the retained preparation. The
backend signs and retains the exact pair before any send. UI ownership is
pending until checkpoint plus60 descendants, void for a losing competitor, and
finalized with separately visible later conflict. Historical noon captures do
not move with live ownership.

Success responses carry exactly the typed result selected by command. A null
pending operation is allowed only for an explicit pending lookup. Errors carry
no result or raw implementation error. Native marks uncertain mutations as
unknown and offers exact lookup/review; it does not create another operation.
Failed native descriptor cleanup retains the actual owner and a sticky error;
another dialog or a repeated close cannot clear that uncertainty.
