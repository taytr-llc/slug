# Set81 complete-lease expiry provenance correction

Apply this additive overlay after the clock-only overlay15b91a96. Original files remain untouched. The corrected hand table contains54 cases.

The selected preflight privately captures its registry result and returns nil from the existing lease callback. This lets unchanged `Lease.Do` retain a nonnil outer authorizer failure instead of replacing it with the callback's expired sentinel. Expiry requires complete original lease success, the exact captured authoritative helper error and a successful original post-check. It does not require a new lease API or change the legacy caller contract.

The added case covers an authoritative expiry followed by an independent joined outer failure, even when a later check would succeed. The expected result is unknown, never expired; all admission counts stay zero. A poisoned clock write remains poisoned and unknown even when combined with unauthorized or outer lease failure. No disk rollback or metadata durability is inferred.

Root selected this correction during source review, before Set81 product execution. It is a narrower implementation of the original fail-closed intent, not an acceptance relaxation based on runtime results. The independent actual-selection producer remains separately frozen and unaffected.
