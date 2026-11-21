#!/usr/bin/env python3
"""Finite paired Linux operator gate. No custody inputs, node service or deploy."""
import hashlib
import json
import os
from pathlib import Path
import platform
import re
import selectors
import shutil
import signal
import stat
import subprocess
import sys
import tempfile
import time

WALLET_SHA = "dfc5d9dd7e0f604486fbac812d0413fce1f45cd6"
PROFILE = {
    "version": 1, "id": "slug-go1274-no-cse-v1", "go_version": "go1.27.4",
    "gcflags": "all=-d=ssa/generic_cse/off,ssa/lowered_cse/off",
    "scope": "candidate_engineering",
}
LOG_LIMIT = 32 << 20
TOTAL_SECONDS = 2100
ACL_TEST = "TestOperatorPrivateInputLinuxACLRefusal"
ACL_CASES = ["file-access-before", "parent-access-before", "parent-default-before",
             "file-access-after", "parent-default-after"]
FORBIDDEN_SYMBOLS = ("ForTestClock", "UseOwnedCalendarOffsetForTest",
                     "UseCalendarClockForTest", "UseOwnedClockForTest",
                     "UseAuthorityClockForTest")


def digest(path):
    h = hashlib.sha256()
    with path.open("rb") as f:
        for block in iter(lambda: f.read(1 << 20), b""):
            h.update(block)
    return h.hexdigest()


def write_json(path, value):
    raw = json.dumps(value, indent=2, sort_keys=True) + "\n"
    with path.open("x", encoding="utf-8") as f:
        f.write(raw)


def group_empty(pgid):
    try:
        os.killpg(pgid, 0)
    except ProcessLookupError:
        return True
    # EPERM/other probe failure is uncertain, never an empty group.
    return False


class Gate:
    def __init__(self, root, wallet, artifacts):
        self.root, self.wallet, self.artifacts = root, wallet, artifacts
        self.started = time.monotonic()
        self.commands = []
        self.tests = []
        self.work = None
        self.env = dict(os.environ)
        for name in ("GOOS", "GOARCH", "GOAMD64", "GO386", "GOARM", "GOARM64",
                     "CGO_CFLAGS", "CGO_CPPFLAGS", "CGO_CXXFLAGS", "CGO_LDFLAGS"):
            self.env.pop(name, None)
        self.env.update(GOENV="off", GOTOOLCHAIN="local", GOFLAGS="", GOEXPERIMENT="",
                        GOMAXPROCS="2", CGO_ENABLED="1", GOOS="linux", GOARCH="amd64")

    def run(self, name, args, cwd=None, seconds=600, env=None):
        if not re.fullmatch(r"[a-z0-9-]+", name):
            raise ValueError("invalid command label")
        available = TOTAL_SECONDS - (time.monotonic() - self.started)
        if available <= 0:
            raise RuntimeError("overall bound reached before " + name)
        bound = min(seconds, available)
        path = self.artifacts / "logs" / (name + ".log")
        selected = dict(self.env)
        if env:
            selected.update(env)
        record = {"name": name, "argv": list(map(str, args)), "cwd": str(cwd or self.root),
                  "bound_seconds": bound, "signals": [], "cleanup_errors": [],
                  "environment": {k: selected.get(k, "") for k in
                                  ("GOWORK", "GOENV", "GOTOOLCHAIN", "GOFLAGS", "GOEXPERIMENT",
                                   "GOOS", "GOARCH", "CGO_ENABLED", "GOMAXPROCS")},
                  "log": str(path.relative_to(self.artifacts)), "group_empty": False}
        self.commands.append(record)
        start = time.monotonic()
        proc = None
        output = bytearray()
        captured_bytes = 0
        try:
            with path.open("xb") as log:
                proc = subprocess.Popen(record["argv"], cwd=record["cwd"], env=selected,
                                        stdin=subprocess.DEVNULL, stdout=subprocess.PIPE,
                                        stderr=subprocess.PIPE, start_new_session=True)
                record.update(pid=proc.pid, pgid=proc.pid)
                with selectors.DefaultSelector() as selector:
                    selector.register(proc.stdout, selectors.EVENT_READ)
                    selector.register(proc.stderr, selectors.EVENT_READ)
                    while selector.get_map():
                        if time.monotonic() - start > bound:
                            raise TimeoutError("command bound: " + name)
                        for key, _ in selector.select(0.1):
                            block = os.read(key.fileobj.fileno(), 65536)
                            if not block:
                                selector.unregister(key.fileobj)
                                continue
                            if captured_bytes + len(block) > LOG_LIMIT:
                                raise RuntimeError("command output bound: " + name)
                            log.write(block)
                            captured_bytes += len(block)
                            # Keep diagnostics in the bounded evidence log, but
                            # return only stdout to structured-output parsers.
                            if key.fileobj is proc.stdout:
                                output.extend(block)
                record["exit_code"] = proc.wait(timeout=max(0.1, bound - (time.monotonic() - start)))
        except BaseException as e:
            record["error"] = type(e).__name__ + ": " + str(e)
        finally:
            if proc is not None:
                # Signal only this exact process group. Any forced termination fails qualification.
                for sig, grace in ((signal.SIGTERM, 5), (signal.SIGKILL, 5)):
                    try:
                        proc.poll()
                        if group_empty(proc.pid):
                            break
                        os.killpg(proc.pid, sig)
                        record["signals"].append(signal.Signals(sig).name)
                        until = time.monotonic() + grace
                        while time.monotonic() < until:
                            proc.poll()
                            if group_empty(proc.pid):
                                break
                            time.sleep(0.05)
                    except ProcessLookupError:
                        break
                    except BaseException as e:
                        record["cleanup_errors"].append(type(e).__name__ + ": " + str(e))
                try:
                    record["exit_code"] = proc.wait(timeout=2)
                    proc.stdout.close()
                    proc.stderr.close()
                    record["group_empty"] = group_empty(proc.pid)
                except BaseException as e:
                    record["cleanup_errors"].append(type(e).__name__ + ": " + str(e))
            record["elapsed_seconds"] = round(time.monotonic() - start, 3)
            record["log_bytes"] = path.stat().st_size if path.exists() else 0
            record["log_sha256"] = digest(path) if path.exists() else None
        record["passed"] = (record.get("exit_code") == 0 and not record.get("error")
                            and not record["signals"] and not record["cleanup_errors"]
                            and record["group_empty"])
        if not record["passed"]:
            raise RuntimeError("qualification command failed: " + name)
        return bytes(output)

    def inventory(self, label):
        result = {}
        for kind, root in (("chain", self.root), ("wallet", self.wallet)):
            head = self.run(label + "-" + kind + "-head", ["git", "rev-parse", "HEAD"], root, 20).decode().strip()
            status = self.run(label + "-" + kind + "-status", ["git", "status", "--porcelain=v1", "--untracked-files=all"], root, 20)
            names = self.run(label + "-" + kind + "-files", ["git", "ls-files", "-z"], root, 20).split(b"\0")
            paths = sorted(set(os.fsdecode(p) for p in names if p))
            if len(paths) > 20000:
                raise RuntimeError("source inventory bound")
            rows = []
            for relative in paths:
                p = Path(relative)
                if p.is_absolute() or ".." in p.parts:
                    raise RuntimeError("invalid tracked source path")
                original = root / p
                info = original.lstat()
                if stat.S_ISLNK(info.st_mode):
                    value = os.fsencode(os.readlink(original))
                    rows.append({"path": relative, "kind": "symlink", "size": len(value),
                                 "sha256": hashlib.sha256(value).hexdigest()})
                elif stat.S_ISREG(info.st_mode):
                    rows.append({"path": relative, "kind": "file", "mode": stat.S_IMODE(info.st_mode),
                                 "size": info.st_size, "sha256": digest(original)})
                else:
                    raise RuntimeError("unsupported tracked source object")
            result[kind] = {"head": head, "status": status.decode(), "files": rows}
        write_json(self.artifacts / (label + ".json"), result)
        return result

    def test(self, label, cwd, packages, workspace, race=False, run=None, required=(), tags=""):
        if tags not in ("", "slugtestpow"):
            raise ValueError("only normal or explicit refusal profile is allowed")
        args = ["sh", self.root / "scripts/candidate-go.sh", "test", "-json", "-tags=" + tags,
                "-p=1", "-parallel=1", "-count=1", "-timeout=10m"]
        if race:
            args.append("-race")
        if run:
            args += ["-run", run]
        args += packages
        raw = self.run(label, args, cwd, 660, {"GOWORK": str(workspace)})
        events = []
        for line in raw.splitlines():
            try:
                item = json.loads(line)
            except (ValueError, UnicodeDecodeError):
                continue  # compiler diagnostics still fail via the process exit
            if item.get("Action") in ("pass", "fail", "skip"):
                events.append({k: item[k] for k in ("Action", "Package", "Test", "Elapsed") if k in item})
        passed = {x.get("Test") for x in events if x["Action"] == "pass"}
        package_passes = [x for x in events if x["Action"] == "pass" and "Test" not in x]
        self.tests.append({"command": label, "tags": tags, "events": events, "required": list(required)})
        if not package_passes or any(x["Action"] == "fail" for x in events) or not set(required) <= passed:
            raise RuntimeError("missing required test execution: " + label)


def expected_members(root, wallet):
    chain = [root] + [root / x for x in ("address", "btcec", "btcutil", "chaincfg", "chainhash",
                                         "psbt", "txscript", "v2transport", "wire")]
    wallets = [wallet] + [wallet / x for x in ("wallet/txauthor", "wallet/txrules", "wallet/txsizes", "walletdb", "wtxmgr")]
    return set(chain + wallets)


def remove_temporary_work(path):
    # Go's downloaded module directories are intentionally read-only. All
    # children have joined before this private temporary tree is released.
    info = path.lstat()
    if (not path.name.startswith(".operator-linux-work-") or
            not stat.S_ISDIR(info.st_mode) or info.st_uid != os.geteuid()):
        raise RuntimeError("unexpected temporary work owner")
    for directory, _, _ in os.walk(path, topdown=True, followlinks=False):
        current = Path(directory)
        info = current.lstat()
        if not stat.S_ISDIR(info.st_mode) or info.st_uid != os.geteuid():
            raise RuntimeError("unexpected temporary directory owner")
        os.chmod(current, stat.S_IMODE(info.st_mode) | stat.S_IRWXU)
    shutil.rmtree(path)


def main():
    root = Path(__file__).resolve().parents[1]
    wallet, desktop = root.parent / "slugwallet", root / "desktop"
    target = os.environ.get("SLUG_LINUX_ARTIFACTS", "")
    if not target or not Path(target).is_absolute() or Path(target).exists():
        raise RuntimeError("a fresh absolute SLUG_LINUX_ARTIFACTS directory is required")
    artifacts = Path(target)
    artifacts.mkdir(mode=0o700, parents=False)
    (artifacts / "logs").mkdir(mode=0o700)
    (artifacts / "bin").mkdir(mode=0o700)
    gate = Gate(root, wallet, artifacts)
    report = {"version": 1, "passed": False, "scope": "focused_linux_operator",
              "provisioning_qualified": False, "deployed_host_qualified": False,
              "daily_actual_qualified": False,
              "platform": platform.platform(), "uname": list(platform.uname()),
              "uid": os.geteuid(), "profile": PROFILE, "errors": [], "source_unchanged": False}
    before = None
    try:
        if platform.system() != "Linux" or platform.machine() != "x86_64":
            raise RuntimeError("this finite gate requires native Linux amd64")
        expected = os.environ.get("SLUG_EXPECTED_CHAIN", "")
        if (not re.fullmatch(r"[0-9a-f]{40}", expected) or
                not re.fullmatch(r"[0-9a-f]{40}", WALLET_SHA) or
                os.environ.get("SLUG_EXPECTED_WALLET") != WALLET_SHA):
            raise RuntimeError("exact paired revisions are required")
        # All temporary compiler/test material stays outside the upload directory.
        gate.work = Path(tempfile.mkdtemp(prefix=".operator-linux-work-", dir=root.parent))
        for name in ("tmp", "cache", "modules"):
            (gate.work / name).mkdir(mode=0o700)
        gate.env.update(TMPDIR=str(gate.work / "tmp"), GOCACHE=str(gate.work / "cache"),
                        GOMODCACHE=str(gate.work / "modules"), GOWORK=str(desktop / "go.work"))
        before = gate.inventory("sources-before")
        for key, sha in (("chain", expected), ("wallet", WALLET_SHA)):
            if before[key]["head"] != sha or before[key]["status"]:
                raise RuntimeError("selected source must be exact and clean: " + key)
        exact_profile = (json.dumps(PROFILE, indent=2) + "\n").encode()
        if (root / "build-profile.json").read_bytes() != exact_profile:
            raise RuntimeError("candidate profile bytes differ")
        gate.run("candidate-profile", ["sh", root / "scripts/candidate-go.sh", "check"], seconds=30)
        go_env = json.loads(gate.run("go-environment", ["go", "env", "-json", "GOVERSION", "GOOS", "GOARCH", "CGO_ENABLED", "CC", "GOCACHE", "GOMODCACHE", "GOWORK", "GOFLAGS", "GOEXPERIMENT"], seconds=30))
        report["go_environment"] = go_env
        if go_env["GOVERSION"] != "go1.27.4" or go_env["GOOS"] != "linux" or go_env["GOARCH"] != "amd64":
            raise RuntimeError("unexpected native toolchain")
        report["compiler"] = gate.run("c-compiler", [go_env["CC"], "--version"], seconds=20).decode()
        for label, workspace, extra in (("desktop", desktop / "go.work", {desktop}), ("wallet", wallet / "go.work", set())):
            value = json.loads(gate.run("workspace-" + label, ["go", "work", "edit", "-json", workspace], seconds=30))
            members = {(workspace.parent / row["DiskPath"]).resolve() for row in value["Use"]}
            if members != expected_members(root, wallet) | extra or value.get("Replace"):
                raise RuntimeError("workspace does not select the exact paired modules")
        wire = json.loads(gate.run("normal-wire-selection", ["go", "list", "-json", "github.com/taytr-llc/slug/wire/v2"], seconds=180))
        if Path(wire["Dir"]).resolve() != root / "wire" or "pow_production.go" not in wire["GoFiles"] or "pow_testpow.go" in wire["GoFiles"]:
            raise RuntimeError("normal proof-of-work source not selected")
        gate.test("normal-wire-guard", root / "wire", ["."], desktop / "go.work",
                  run="^TestBlockHashMemoizationPreservesArgon2$", required=["TestBlockHashMemoizationPreservesArgon2"])
        for name, cwd, package, cgo in (("slugd", root, ".", "0"),
                                       ("slugpoold", desktop, "./cmd/slugpoold", "0"),
                                       ("slugpilot", desktop, "./cmd/slugpilot", "1")):
            binary = artifacts / "bin" / name
            gate.run("build-" + name, ["sh", root / "scripts/candidate-go.sh", "build", "-p=1", "-trimpath", "-tags=", "-o", binary, package], cwd, 600, {"CGO_ENABLED": cgo})
            metadata = gate.run("verify-" + name, ["sh", root / "scripts/candidate-go.sh", "verify-binary", binary], seconds=30).decode()
            symbols = gate.run("symbols-" + name, ["go", "tool", "nm", binary], seconds=60).decode()
            if any(tag in metadata for tag in ("slugtestpow", "slugtestclock")) or any(x in symbols for x in FORBIDDEN_SYMBOLS):
                raise RuntimeError("diagnostic profile leaked into normal artifact")
            if "CGO_ENABLED=" + cgo not in metadata or "GOOS=linux" not in metadata or "GOARCH=amd64" not in metadata:
                raise RuntimeError("normal binary target mismatch")
            report.setdefault("binaries", []).append({"path": "bin/" + name, "bytes": binary.stat().st_size,
                                                       "sha256": digest(binary), "profile_verified": True,
                                                       "normal_tags_and_symbols": True})
        for race in (False, True):
            mode = "race" if race else "normal"
            gate.test(mode + "-authority", root, ["./internal/operatorauthority"], desktop / "go.work", race,
                      required=["TestOperatorAuthorityExtendedACLRefusal", "TestOperatorAuthorityIndependentNamedSignatures"])
            gate.test(mode + "-wallet", wallet, ["./waddrmgr", "./wallet"], wallet / "go.work", race,
                      run="^(TestPool(Settlement|Wallet)|TestDailyPool(Settlement|Wallet))",
                      required=["TestPoolSettlementManagerIndependentSigning", "TestPoolWalletOwnerLifecycle",
                                "TestDailyPoolSettlementConditionalIndependentSignatures",
                                "TestDailyPoolSettlementConditional721AndCancellation",
                                "TestDailyPoolSettlementOwnedInputsAndShapeRefusals",
                                "TestDailyPoolWalletBindingReference",
                                "TestDailyPoolWalletFreshOwnerAndLegacyRefusal"])
            gate.test(mode + "-operator", desktop, ["./pooloperator"], desktop / "go.work", race,
                      run="^Test(Operator|DailyOperator|DailyProvision|DailyIntake)", required=[ACL_TEST] + [ACL_TEST + "/" + case for case in ACL_CASES] + [
                          "TestOperatorIntakeSourceBusyErrorTrees",
                          "TestOperatorIntakeMutexCancellationJoinsHolder",
                          "TestDailyIntakeOriginalIssuerBusyPreservesDrainAndPayout",
                          "TestDailyIntakeOriginalIssuerCancellationNeverEnables",
                          "TestDailyOperatorIndependentComposition",
                          "TestDailyOperatorIndependentFraming",
                          "TestDailyProvisionOriginalWalletAndRuntimeReopen",
                          "TestDailyProvisionResumesOriginalMaterialAndRefusesOtherMode",
                          "TestDailyOperatorFilesSelectedMarkers",
                          "TestDailyProvisionCapacityBeforeCreation",
                          "TestDailyOperatorCommandExplicitSelection",
                          "TestDailyOperatorCanceledCommandBeforeEffects",
                          "TestOperatorLiveRoundOriginalCatchupAndDefault",
                          "TestOperatorLiveRoundCapacityRefresh",
                          "TestOperatorLiveRoundSelectionRefusals",
                          "TestOperatorLiveRoundCloseJoinsSelection"])
            gate.test(mode + "-daily-public-transport", root,
                      ["./poolminer", "./poolservice", "./poolingress"], desktop / "go.work", race,
                      required=["TestDailyMinerIndependentBytesAndBindings",
                                "TestDailyMinerRotationAudienceAndReopen",
                                "TestDailyTransportIndependentBindings",
                                "TestDailyTransportIndependentMoneyPages",
                                "TestDailyTransportIndependentFraming",
                                "TestDailyClientUnknownIssueNeverRetries",
                                "TestDailyServerOriginalOwnersAndMoneyBinding",
                                "TestDailyServerRefusesBeforeEffects",
                                "TestDailyServerBusyAndRunJoins",
                                "TestDailyIngressCatalogAndCrossMode",
                                "TestDailyIngressOriginalTLSAndJoin"])
            gate.test(mode + "-daily-public-worker", root, ["./prospectworker"],
                      desktop / "go.work", race, run="^Test(DailyPool|Pool)", required=[
                          "TestDailyPoolIndependentBytesAndModes",
                          "TestDailyPoolIndependentJournalAuditAndReopen",
                          "TestDailyPoolIssueOriginalAttemptAndUncertainty",
                          "TestDailyPoolIssueSyncFailuresKeepOriginalEvidence",
                          "TestDailyPoolHeldReopenStillReconcilesOriginalPending",
                          "TestDailyPoolRunUnknownIssuePersistsAcrossOriginalReopen",
                          "TestDailyPoolConstructorFailureRetainsOriginalCleanupOwner"])
            gate.test(mode + "-daily-public-desktop", desktop,
                      ["./pilot", "./accountpilot", "./cmd/slugpilot"], desktop / "go.work", race,
                      run="^Test(Daily|RemotePool)", required=[
                          "TestDailyRemoteIndependentConfigAndMoneyBinding",
                          "TestDailyRemoteIndependentCredentialGrammar",
                          "TestDailyRemoteSelectionBeforeOriginalOwnerEffects",
                          "TestDailyRemoteOriginalWalletRecipientAndMiningEpoch",
                          "TestDailyRemoteProtectedOriginalAudienceAndCancellation",
                          "TestDailyRemoteMarkersAndExactOutboxRemainSeparate",
                          "TestDailyRemoteControllerKeepsOriginalEpochAndRefusesLegacyWire",
                          "TestDailyRemoteControllerModeRefusesBeforeRoot",
                          "TestDailyRemoteCancellationCauseAndLegacyParity",
                          "TestDailyRemoteStartRetainsObservedCloseFailure",
                          "TestDailyRemoteOriginalAccountCancellationProvenance",
                          "TestDailyRemoteObservedEpochLossSurvivesCancellation",
                          "TestDailyHeadlessParentCancellationInterruptsOriginalPipe",
                          "TestDailyHeadlessOutputFailureSurvivesParentCancellation",
                          "TestDailyHeadlessCancellationPreservesOtherCauses",
                          "TestDailyHeadlessFixedStatusAtomicFrame",
                          "TestDailyHeadlessOriginalFileOwnersJoinBeforeCancellationSuccess",
                          "TestDailyHeadlessObservedIntegrityIsNotCancellation",
                          "TestDailyHeadlessFlagsAndCancellationBeforeEffects",
                          "TestDailyHeadlessOutputCancellationJoins"])
            gate.test(mode + "-daily-mining-lease", root, ["./internal/mininglease"],
                      desktop / "go.work", race, required=[
                          "TestDailyCaptureOriginalCancellationAndLegacyParity",
                          "TestDailyCapturePreservesObservedIndependentFailures",
                          "TestDailyCaptureSuccessfulLeaseRetainsOriginalCommitFences"])
            # These complete package suites preserve V1 controls as well as
            # requiring the new daily boundary cases. No actual-test build tag.
            gate.test(mode + "-daily-source-round", root, ["./internal/poolcapacity", "./poolshare", "./poolround"],
                      desktop / "go.work", race, required=[
                          "TestDailyRecoveryIndependentReference",
                          "TestDailyPublicBindingOriginalOwner",
                          "TestDailyEnvelopeIndependentReference",
                          "TestDailyCapacityProofFrames",
                          "TestRecoveryIndependentTwoAxisReference",
                          "TestDailyOperatorDerivedFullUnseenCanonicalReopen",
                          "TestDailyOperatorSelectionMembershipAndOwnedBudget",
                          "TestDailyOperatorSourceHoldbackBeforeIssuer",
                          "TestDailyOperatorAuditPrecedenceAndOriginalMetadata",
                          "TestDailyOperatorUncertainCommitAndExactSavedRecovery",
                          "TestDailyOperatorDrainOnlyPendingAndPostCeilingHold",
                          "TestDailyOperatorOriginalCancellationCloseAndErrorClass",
                          "TestDailyOperatorAliasesCountWholeBudget",
                          "TestDailyOperatorPhysicalFullAuditAndLegacyParity",
                          "TestDailyOperatorFinalSessionClockAndCapacityError",
                          "TestDailySourceIndependentGrammarAndKeys",
                          "TestDailySource721OriginalJobsAndReopen",
                          "TestDailySourceCanonicalStorageRefusals",
                          "TestDailySourceModesCancellationAndLimits",
                          "TestDailySourceMemoOwnedWarmColdAndTamper",
                          "TestDailyRoundIndependentBytes",
                          "TestDailyRoundLifetimeAndWindowBounds",
                          "TestDailyRoundOriginalSourceRestartAndFences",
                          "TestRoundLiveOriginalOwnerSyncAndRefresh",
                          "TestRoundLiveLateWinnerAndReopen",
                          "TestRoundLiveTouchedCorruptionRevokes",
                          "TestRoundLiveDoesNotWeakenFullAudits",
                          "TestRoundLiveUncertainWriteAndExplicitReanchor",
                          "TestRoundLiveSourceFencesCancellationAndOwnership",
                          "TestRoundLiveOriginalCloseJoins",
                          "TestRoundLiveReadOnlyRefusal",
                          "TestRoundLiveFullSyncFailureIsAtomic",
                          "TestRoundLiveIndependentModeledPages",
                          "TestRoundSourceExactBusyClassification"])
            gate.test(mode + "-daily-source-fences", root, ["./registry", "./poolservice"],
                      desktop / "go.work", race,
                      run="^(TestManifestTryJob|TestPoolServiceDoesNotAdoptDailySource)", required=[
                          "TestManifestTryJobOriginalReadOnlyAndNonblocking",
                          "TestManifestTryJobCancellationAndClosedState",
                          "TestPoolServiceDoesNotAdoptDailySource"])
            gate.test(mode + "-daily-node-shape", root, ["./pooltxshape", "./poolsettlenode"],
                      desktop / "go.work", race, required=[
                          "TestNativeP2WPKHIndependentReference",
                          "TestNativeP2WPKHRejectsMachineIntExtremes",
                          "TestDailyNode721AndLegacyBounds",
                          "TestDailyNodeWholeFrontierBeyondTransactionLimit",
                          "TestDailyNodeFrontierAdmissionBound",
                          "TestDailyNodeCarriedOriginsAndLateFailure",
                          "TestDailyNodeFourWorkersCancelAndClose",
                          "TestDailyNodeFeeRawAndWeightProfile",
                          "TestDailyNodeObservedWeightAndOwnedBroadcast"])
            gate.test(mode + "-daily-ledger-schedule", root, ["./poolsettle", "./poolschedule"],
                      desktop / "go.work", race, required=[
                          "TestSettlementReplayV1Parity",
                          "TestDailyPayoutIndependentReference",
                          "TestDailyPayoutPageOriginalOwnerCaptureAndRestart",
                          "TestDailyPayoutPageFinalSessionFence",
                          "TestDailyPayoutPageOriginalSparseSlotTraversal",
                          "TestDailyCompletedRefreshGuardsBeforeEffects",
                          "TestDailyCompletedRefreshRefusesOriginalV1",
                          "TestDailyCompletedRefreshProfileAndMissingOwner",
                          "TestDailyScheduleCompletedRefreshIndependentDecisions",
                          "TestDailyScheduleCompletedRefreshExpiryRetry",
                          "TestDailyScheduleCompletedRefreshImportsOnceAcrossReopen",
                          "TestScheduleRetryIndependentErrorTrees",
                          "TestDailyScheduleOriginalRoundBusyRecoversWithoutNewPayment",
                          "TestDailyScheduleEmptyCursorContentionImportsOnce",
                          "TestDailyAdmissionContentionErrorTrees",
                          "TestDailyAdmissionContentionBudgetDoesNotRenew",
                          "TestDailyAdmissionOriginalLedgerWaitCancellationAndClose",
                          "TestDailyAdmissionPostSignRoundContentionUsesOriginalAttempt",
                          "TestDailyLedgerIndependentCodecReplay",
                          "TestDailyLedgerForgedInverseIndicesAndCancellation",
                          "TestDailyLedgerStagedImportAtomic",
                          "TestDailyLedgerWriteUncertaintyRetainsExactSignedHistory",
                          "TestDailyLedgerConditionalWholeFrontierWeightDeferral",
                          "TestDailyAdmissionFullIntakePreservesPendingPayoutAcrossOwners",
                          "TestDailyOperatorSourcePendingRecoveryAndNextDay",
                          "TestDailySigningIndependentBytes",
                          "TestDailySigningOpaqueOriginalLifetime",
                          "TestDailySigningBoundsAndOwnerCopies",
                          "TestDailyScheduleIndependentBindings",
                          "TestDailyScheduleIndependentCodecAndMode",
                          "TestDailyScheduleIndependentCursorAndD84",
                          "TestDailyScheduleOriginalOwnerModesAndReopen",
                          "TestDailyScheduleRunCancelAndJoin"])
        # Refusals only: no positive custody/source test is relabeled as cheap
        # proof, and the opt-in slugdailyactual tag is never admitted here.
        cheap_wire = json.loads(gate.run("cheap-wire-selection", ["go", "list", "-tags=slugtestpow", "-json",
                                             "github.com/taytr-llc/slug/wire/v2"], seconds=180))
        if (Path(cheap_wire["Dir"]).resolve() != root / "wire" or
                "pow_testpow.go" not in cheap_wire["GoFiles"] or
                "pow_production.go" in cheap_wire["GoFiles"]):
            raise RuntimeError("cheap refusal proof-of-work source not selected")
        cheap_core = ["TestDailyOperatorCheapRefusal", "TestDailySourceCheapRefusesBeforeEffects",
                      "TestDailyRoundProfileRefusal", "TestDailyNodeCheapProfile",
                      "TestDailyLedgerCheapRefusalBeforeEffects", "TestDailySigningCheapRefusal",
                      "TestDailyScheduleCheapAndCanceledBeforeEffects",
                      "TestManifestTryJobCheapRefusesBeforeOwner", "TestRoundLiveCheapProfileRefusal",
                      "TestDailyPublicBindingCheapRefusal",
                      "TestDailyCompletedRefreshProfileAndMissingOwner",
                      "TestRoundSourceExactBusyClassification",
                      "TestScheduleRetryIndependentErrorTrees",
                      "TestDailyAdmissionContentionErrorTrees",
                      "TestDailyAdmissionContentionBudgetDoesNotRenew"]
        gate.test("cheap-daily-core", root,
                  ["./poolshare", "./poolround", "./poolsettlenode", "./poolsettle",
                   "./poolschedule", "./registry"], desktop / "go.work",
                  run="^(" + "|".join(cheap_core) + ")$", required=cheap_core, tags="slugtestpow")
        cheap_wallet = ["TestDailyPoolSettlementCheapRefusal", "TestDailyPoolWalletCheapRefusal"]
        gate.test("cheap-daily-wallet", wallet, ["./waddrmgr", "./wallet"], wallet / "go.work",
                  run="^(" + "|".join(cheap_wallet) + ")$", required=cheap_wallet, tags="slugtestpow")
        cheap_public = ["TestDailyMinerRefusalBeforeEffects", "TestDailyTransportCheapRefuses",
                        "TestPoolCheapProfileRefusesBeforeState"]
        gate.test("cheap-daily-public", root, ["./poolminer", "./poolservice", "./prospectworker"],
                  desktop / "go.work", run="^(" + "|".join(cheap_public) + ")$",
                  required=cheap_public, tags="slugtestpow")
        gate.test("cheap-daily-desktop", desktop, ["./pilot"], desktop / "go.work",
                  run="^TestDailyRemoteSelectionBeforeOriginalOwnerEffects$",
                  required=["TestDailyRemoteSelectionBeforeOriginalOwnerEffects"], tags="slugtestpow")
        intake_controls = ["TestOperatorIntakeSourceBusyErrorTrees",
                           "TestOperatorIntakeMutexCancellationJoinsHolder"]
        gate.test("cheap-intake-controls", desktop, ["./pooloperator"], desktop / "go.work",
                  run="^(" + "|".join(intake_controls) + ")$",
                  required=intake_controls, tags="slugtestpow")
        cancellation_controls = [
            "TestDailyCaptureOriginalCancellationAndLegacyParity",
            "TestDailyCapturePreservesObservedIndependentFailures",
            "TestDailyCaptureSuccessfulLeaseRetainsOriginalCommitFences",
            "TestDailyRemoteCancellationCauseAndLegacyParity",
            "TestDailyRemoteStartRetainsObservedCloseFailure",
            "TestDailyHeadlessParentCancellationInterruptsOriginalPipe",
            "TestDailyHeadlessOutputFailureSurvivesParentCancellation",
            "TestDailyHeadlessCancellationPreservesOtherCauses",
            "TestDailyHeadlessFixedStatusAtomicFrame",
            "TestDailyHeadlessOriginalFileOwnersJoinBeforeCancellationSuccess",
            "TestDailyHeadlessObservedIntegrityIsNotCancellation",
            "TestDailyHeadlessFlagsAndCancellationBeforeEffects",
            "TestDailyHeadlessOutputCancellationJoins"]
        gate.test("cheap-cancellation-controls", desktop,
                  ["../internal/mininglease", "./pilot", "./cmd/slugpilot"], desktop / "go.work",
                  run="^(" + "|".join(cancellation_controls) + ")$",
                  required=cancellation_controls, tags="slugtestpow")
        for label, cwd, packages, workspace in (
            ("authority", root, ["./internal/operatorauthority"], desktop / "go.work"),
            ("wallet", wallet, ["./waddrmgr", "./wallet"], wallet / "go.work"),
            ("operator", desktop, ["./pooloperator", "./pilot", "./accountpilot", "./cmd/slugpilot"], desktop / "go.work"),
            ("daily-components", root,
             ["./internal/mininglease", "./internal/poolcapacity", "./poolshare", "./poolround", "./pooltxshape", "./poolsettlenode",
              "./poolsettle", "./poolschedule", "./registry", "./poolservice", "./poolminer",
              "./poolingress", "./prospectworker"], desktop / "go.work")):
            gate.run("vet-" + label, ["go", "vet", "-p=1", "-tags=", "-gcflags=" + PROFILE["gcflags"]] + packages,
                     cwd, 300, {"GOWORK": str(workspace)})
    except BaseException as e:
        report["errors"].append(type(e).__name__ + ": " + str(e))
    finally:
        # Post-failure evidence is independent: do not lose original command results.
        try:
            after = gate.inventory("sources-after")
            report["source_unchanged"] = before is not None and after == before
        except BaseException as e:
            report["errors"].append("source audit: " + type(e).__name__ + ": " + str(e))
        for binary in report.get("binaries", []):
            try:
                selected = artifacts / binary["path"]
                binary["unchanged_after_checks"] = (selected.is_file() and not selected.is_symlink()
                                                    and selected.stat().st_size == binary["bytes"]
                                                    and digest(selected) == binary["sha256"])
                if not binary["unchanged_after_checks"]:
                    report["errors"].append("binary changed after qualification: " + binary["path"])
            except BaseException as e:
                report["errors"].append("binary audit: " + type(e).__name__ + ": " + str(e))
        if gate.work is not None:
            joined = all(c["group_empty"] and not c["cleanup_errors"] for c in gate.commands)
            if not joined:
                # A failed/uncertain child still owns these paths. Never delete
                # its working material or upload that private temporary tree.
                report["temporary_material_retained"] = True
                report["errors"].append("temporary material retained because child cleanup is uncertain")
            else:
                try:
                    remove_temporary_work(gate.work)
                    report["temporary_material_removed"] = True
                except BaseException as e:
                    report["errors"].append("temporary cleanup: " + type(e).__name__ + ": " + str(e))
        report["commands"] = gate.commands
        report["test_results"] = gate.tests
        report["clean_join"] = bool(gate.commands) and all(c["group_empty"] and not c["signals"] and not c["cleanup_errors"] for c in gate.commands)
        report["elapsed_seconds"] = round(time.monotonic() - gate.started, 3)
        report["passed"] = (not report["errors"] and report["source_unchanged"] and report["clean_join"]
                            and report.get("temporary_material_removed", False)
                            and all(c["passed"] for c in gate.commands))
        write_json(artifacts / "report.json", report)
        summary = ("Operator Linux qualification: " + ("PASS" if report["passed"] else "FAIL") + "\n\n"
                   + "Focused authority, wallet16, fixture provisioning, daily public transport/worker/desktop and accounting checks; deployed-host and long actual acceptance are excluded.\n"
                   + "Source unchanged: " + str(report["source_unchanged"]) + "; clean join: " + str(report["clean_join"]) + "\n")
        if report["errors"]:
            summary += "Errors: " + "; ".join(report["errors"]) + "\n"
        (artifacts / "SUMMARY.md").write_text(summary)
        if os.environ.get("GITHUB_STEP_SUMMARY"):
            with open(os.environ["GITHUB_STEP_SUMMARY"], "a", encoding="utf-8") as out:
                out.write(summary)
        print(summary)
    return 0 if report["passed"] else 1


if __name__ == "__main__":
    sys.exit(main())
