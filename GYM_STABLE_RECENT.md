# Set41 engineering addendum: explicit Recent selection

Base contract SHA256: f977e8a7cef5ab470e5019f4f1b9d88cdb4fd03d5ee1feb6acc32321389d84d2.
Root approves this narrow implementation clarification within existing set41
scope. It adds no product decision, source trust, new proof path or permission.

A fixed inventory page can have the same after/limit values as a Recent page.
Inferring intent from those numbers would either unprotect a fixed last page or
leave Recent unable to receive new arrivals. Make that choice explicit:

- `stable_recent` has required fields `id`, `command`, `view_id` only, in that
  canonical order. Its result is the existing StableJob with kind `inventory`.
- Child/parent method: `BeginStableRecent(context.Context,string) (StableJob,error)`.
  Native typed method: `Gym.BeginStableRecent(viewID string) pilot.StableResponse`.
- Its tracked task carries a private `followLatest` choice. On execution it reads
  the current local checkpoint, then calls Inventory with after=max(C-8,0) and
  limit8. It performs at most eight serial source verifications, on the same lane.
- Only successful original-view publication clears a protected page and selects
  the existing Recent merge/eviction policy. Failed/cancelled navigation cannot
  silently unprotect the previously selected evidence/export.
- Ordinary `stable_inventory` always selects a fixed interval, even when it is
  the final eight records. Live Sync preserves that fixed page and export slot.
- Native/frontend correlate Recent's terminal page against that page's own
  checkpoint, not the older snapshot when the request was queued. Its next_after
  equals checkpoint; after equals max(checkpoint-8,0). Job IDs and exact current
  view/store binding remain unchanged. No result is rewritten under an old ID.
- The old Manifest protocol, all core APIs/DTOs, capacity and public error
  vocabulary remain unchanged. Add request/transport/DOM regressions distinguishing
  fixed last-page selection from an explicit return to Recent.

The same review clarified a private cross-package upgrade validator:
`pilot.ValidateStableUpgradeChild(Config,accountAuthorityID,accountID) error`.
It validates an existing required child intent and exact identity before using
existing bounded component validation. Recognized pre-Prepare new state and
complete READY-assisted initialization retain the recovery paths documented in
GYM_STABLE_INTEGRATION.md; partial or mismatched established state is refused. It never calls Prepare, cold proof
loading, mining, or exports, and accepts no trust/creation flag. Existing store
validation may open/close DB handles and lock files; describe it as validation,
not a guarantee that no filesystem metadata changes. Parent prevalidates its
mandatory root marker before opening a missing instance lock and rereads under
the lock before publishing v2. Unknown/partial data is retained and refused.
