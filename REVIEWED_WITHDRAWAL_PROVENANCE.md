# Set68 journal handle provenance addendum

Additive API clarification to frozen REVIEWED_WITHDRAWALS.md; original frozen bytes remain unchanged. No persisted field, digest, transaction, expected vector or authority rule changes.

The concrete wallet/node adapter already retains one exact `*walletwithdraw.Journal`. A private token from a different live journal must not be accepted merely because its own owner is live. Add:

```go
func (j *Journal) OwnsDelivery(d *Delivery) bool
func (j *Journal) OwnsReconciliation(r *Reconciliation) bool
```

Both return false for nil/foreign/closed/cancelled/poisoned owners and require the exact journal pointer retained by the token's private Owner. OwnsDelivery also requires that this token is the currently issued delivery ticket of that Owner; OwnsReconciliation verifies the retained operation/digest/retained-or-abandoned state through its existing Check. Neither acquires a lock, changes state, consumes a ticket, substitutes an account capability or authorizes delivery/reconciliation itself. They are called only while the concrete adapter retains the nonconcurrent Owner, outside account leases. Journal pointer equality is lifetime identity, not an identity value taken from serialized metadata.

A taken delivery token may still belong for a post-call owner check; `Delivery.Take` remains the sole one-use extraction boundary and refuses a second extraction. Existing original-action publication, node context/lifetime checks and exact wallet reservation matching remain required.

Tests require two valid live journals with distinct independently supplied namespaces; each recognizes only its own tokens, ownership stops after Owner.Close/Journal cancellation, and provenance inspection cannot restore consumed delivery permission. No new expected hash or golden is needed.
