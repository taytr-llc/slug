# Set80 implementation split

Source-only map against contract `f3e019a840d6b69d853350763364a3fc41f10ea899f00ed6f164db5d58a95610` and Set79 base `fdd40d8edd9a685613f1f667f0074e56ec4e7da5`. No Go authored; implementation starts only after root qualifies the corrected independent reference.

## Shared declarations first

Transport/core lane owns these declarations and pure codecs before the other lanes compile against them:

* `registry.IssueProfile`: `Version uint32`, `BaseSetupID string`, `SourceConfigID string`, `MaxRequests uint64`, in approved JSON order. `IssueProfileID` implements the approved framed domain.
* `registry.IssueIntent`: `Version uint32`, `ProfileID`, `RequestID`, `GymID`, `PublicKey`, `TemplateSHA256 string`. `IssueIntentID(IssueIntent) (string,error)` is a pure canonical digest helper; it grants no issuance authority. The contract's `Intent` argument refers to this type.
* `registry.IssuerRequest`: `Version uint32`, `ProfileID`, `RequestID`, `IntentID`, `GymID`, `PublicKey`, `State`, `JobID string`, `SealedAtMS int64`, `Reason string`; opaque `PreparedIssueRequest` retains the original store/profile/intent/context.
* `poolshare.IssueResult`: `Version uint32`, `State`, `IntentID`, `JobID string`, `ResolvedAtMS int64`, `Reason string`, `Work *MiningWork`. Required null remains serialized for non-work states. Embedded job/template and all share receipts remain unchanged.
* Service owns `RecoverableDailyClientConfig{Version,Base DailyClientConfig,IssueProfile registry.IssueProfile}`, `IssueRequest{RequestID}`, and ordered `IssueResponse{Version,BindingID,SettlementConfigID,RecipientScript,IssueProfileID,RequestID,GymID,PublicKey,Result poolshare.IssueResult}`.

Root accepted two support seams, without new serialized fields or authority:

* `(*poolshare.Pool).CheckDailyServiceSelection(ctx, expected *registry.IssueProfile) error`: nil requires plain DAILY; nonnil requires exact recoverable selection on the original live owner. The existing `DailyBinding` remains available to the same BaseConfig2 financial consumers. Old and new service constructors still perform their full binding/source audits.
* `registry.ValidateRecoverableOperatorProfile(OperatorProfile, IssueProfile) error`: pure initial selected request/job/metadata plus existing publication-reserve fit check, before any provisioning/root/issuer effect. Root independently validates the full BaseSetup2/profile relationship; registry cannot import the desktop Setup type.

## Disjoint production ownership

**Transport/core agent — `registry` and `poolshare`.** Add narrow `issue_*` leaves for types/codecs, selected constructors, full namespace audit, request operations and reservation arithmetic. Existing registry touch points: `manifest_types.go` (private selection), `manifest_store.go` (marker/meta codec, commit and closed census), `manifest_jobs.go` and `manifest_staged.go` (all old issue entrypoint fences), `operator_store.go`/`public_store.go` (explicit owner construction and every-write additive holdback), `public_audit.go` (selected namespace/cross-reference validation). Keep the existing ownership guard and three-entry directory inventory.

Existing source touch points: `types.go` (private selection), `store.go` (wrapper metadata, all commits, full audit and source work completion), `config.go`/`daily.go` (explicit constructors/context/issuer checks), `pool.go` (legacy Issue refusal and existing original session/effects handling), `daily_binding.go`, `daily_operator_admission.go`/`daily_operator_audit.go` (effective request-byte occupancy without changing the financial envelope). Reuse exact original `withSession`, `prepareContext`, `saveSubmission` and work keys; completed request conversion needs one atomic batch alongside `w`, inverse, counters and template removal, rather than calling existing `saveWork` and then separately resolving the request.

**Service agent — `poolservice` and `poolingress`.** Add selected config/trust/codec/client/server leaves; minimally branch existing private `Client`/`Server`, path dispatch and old-constructor checks. Keep the same concrete client, original session owners and 30-second operations. Ordinary `Client.Issue` refuses selected mode. `ResumeIssue`/`LookupIssue` return the full authenticated result envelope. New v3 work/share/balance/page envelopes are transport projections of unchanged daily payloads; validate the added profile before applying existing Base2 payload validators. Do not widen the existing financial DTO validators to accept v3 globally. Ingress wraps Config2 and selects only `/pool/v3/`; admission v3 and ownership v2 remain exact.

**Desktop agent — `prospectworker`, `desktop/pilot`, `desktop/accountpilot`, `desktop/cmd/slugpilot`.** Own recoverable outbox/worker wrappers, retained q rows, scan reference and shared counts/reserves; original daily lease capture and attached-wallet path; explicit helper selection/flags. Keep exact concrete `*poolservice.Client` inside the original credential lease, rather than an arbitrary Authority claiming protocol3. Compare immutable profile/source/money/trust plus the complete wrapper; no credential getter. Old NewDailyPool must reject an already-open selected outbox. No shared source/service files.

**Root — `desktop/pooloperator` and `desktop/cmd/slugpoold`.** Own setup/provision wrappers, filenames/backup, before-effect capacity preflight, original registry/source/ledger/controller/server/ingress composition and headless operator selection. Root owns all runtime qualification and the acceptance runner. The miner helper CLI belongs to desktop, the operator CLI to root. `poolminer` and financial/round/scheduler formats require no change.

Existing `remotePoolPoolLease` and `remotePoolAuthority` wrap the concrete client to enforce protected-file/expiry checks and original Mining/View capture. Root selected keeping those private forwarding wrappers plus the same concrete client, with opaque nonserialized IssueResponse provenance minted only after that client's full validation. Before local commit, the worker checks original-client identity and unchanged canonical request/result bytes. Service and desktop coordinate `CheckRecoverableDailySelection` and `CheckIssueResponse` signatures; no credential getter, injected callback or serialized field is added. Core request storage is independent of these client support declarations.

## Dependency and commit order

1. Qualify the corrected independent reference; freeze shared type/constructor signatures and pure support helpers above.
2. Core declares registry/source APIs. Service and desktop may then implement against those declarations in their disjoint packages while core completes storage behavior. Service publishes concrete client config/trust/result signatures immediately for desktop; no fake implementation or permissive stub is admitted to runtime.
3. Root composes exact owners only once core is ready. Complete registry/source selected audits must preserve pending/absent, pending/issued and pending/sealed crash states. Selected issue writers are source-coordinated; old direct/staged issuer writers refuse. Full matched issuer request/job counts prevent an unrelated sealed row hiding behind equal metadata counts.
4. Core signals package readiness before root executes any build/test. Same-ID retries reuse original job/template/time; positive terminal seals retain all history. No whole issuance retry or error clears a local request. Existing financial audits, pending payout recovery, D80 and D84 stay intact.

No user or economic choice blocks this split. Remaining engineering gates are reference PASS, exact shared declarations, pure preflight and the two original-live-owner cross-mode fences. The finite capacity and cold-audit limits remain unchanged; this slice is not continuous service or public-host readiness.
