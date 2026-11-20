# Native recovery clarification for Set68

The frozen private wire still requires a complete nonempty OperationID on every
`withdrawal_action` request. The native `WithdrawalAction(viewID, operationID,
actionID)` entry point additionally permits an empty operationID only when its
retained current action already binds that exact actionID and original viewID to
the same concrete helper instance. Native substitutes its retained OperationID,
not an ID from a snapshot, latest-operation reply, another action or a new helper.

This resolves a lost prepare response when the exact native-known action has
finished before an app intent exists. A foreign view/action or replaced helper
is refused. A supplied nonempty OperationID must still equal the retained one.
No mutation replay, token substitution or additional wire field is authorized.

This engineering clarification was reviewed by the root agent before its native
implementation. The original frozen contract and reference outputs stay intact.
