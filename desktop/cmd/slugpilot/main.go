// slugpilot owns explicit local fixture modes or an existing-wallet remote
// pool attachment; remote mode never starts a node. Its stdout is exclusively the bounded JSON-lines protocol.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/taytr-llc/slug/desktop/accountpilot"
	"github.com/taytr-llc/slug/desktop/internal/poolselection"
	"github.com/taytr-llc/slug/desktop/pilot"
	"io"
	"os"
	"os/signal"
	"syscall"
)

func run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("slugpilot", flag.ContinueOnError)
	flags.SetOutput(stderr)
	remoteArgs := remoteLaunchRequested(args)
	if remoteArgs {
		flags.SetOutput(io.Discard)
	}
	data := flags.String("data-dir", "", "Explicit disposable pilot directory")
	node := flags.String("node-binary", "", "Explicit bundled normal SLUG node binary")
	dailyWallet := flags.Bool("daily-wallet-helper", false, "Select the explicit interactive DAILY wallet pipe")
	dailyWalletVersion := flags.String("daily-wallet-helper-version", "1", "Explicit local DAILY wallet wire version (2 adds own activity)")
	recoverableIssue := flags.Bool("daily-recoverable-issue", false, "Select exact-request daily remote issuance")
	daily := flags.Bool("daily", false, "Select the explicit daily headless remote worker")
	remote := flags.Bool("candidate-remote-pool", false, "Attach the original wallet to an explicit remote pool")
	attachment := flags.String("remote-attachment-root", "", "Explicit remote attachment directory")
	remoteConfig := flags.String("remote-config", "", "Protected public remote configuration file")
	remoteRPC := flags.String("remote-node-rpc", "", "Protected existing loopback RPC configuration file")
	poolCredential := flags.String("remote-pool-credential", "", "Protected pool credential file")
	admissionCredential := flags.String("remote-admission-credential", "", "Protected admission credential file")
	owned := flags.Bool("candidate-owned-gym", false, "Use the separate owned-fighter gym")
	pool := flags.Bool("candidate-pool", false, "Use the separate shared public-fixture pool desktop")
	hosted := flags.Bool("candidate-hosted-pool", false, "Use the separate signed public-fixture hosted pool desktop")
	recovery := flags.Bool("candidate-hosted-recovery", false, "Use the separate hosted allocation recovery fixture desktop")
	nameVersion := flags.String("candidate-name-version", "1", "Explicit immutable pool candidate name version (1 or 2)")
	accounts := flags.Bool("account-fixtures", false, "Use the separate two-public-fixture account pilot")
	candidates := flags.Bool("candidate-manifests", false, "Use the separate candidate fixture desktop")
	stable := flags.Bool("candidate-stable", false, "Enable the candidate stable within candidate-manifests")
	entries := flags.Bool("candidate-entries", false, "Enable Saturday entry intake within candidate-stable")
	candidateEvents := flags.Bool("candidate-events", false, "Enable immutable noon snapshots within candidate-entries")
	candidateSeeds := flags.Bool("candidate-seeds", false, "Enable candidate seed publication within candidate-events")
	candidateBouts := flags.Bool("candidate-bouts", false, "Enable saved candidate bout transcripts within candidate-cards")
	candidateCards := flags.Bool("candidate-cards", false, "Enable bounded candidate cards within candidate-seeds")
	candidateResults := flags.Bool("candidate-results", false, "Enable saved candidate result publication within candidate-bouts")
	career := flags.Bool("career", false, "Enable finality-timed candidate career accounting within candidate-results")
	if e := flags.Parse(args); e != nil {
		if remoteArgs {
			return errRemoteLaunchInvalid
		}
		return e
	}
	dataExplicit, poolExplicit, nameExplicit, legacyExplicit, hostedExplicit, recoveryExplicit, ownedExplicit := false, false, false, false, false, false, false
	flags.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "candidate-owned-gym":
			ownedExplicit = true
		case "data-dir":
			dataExplicit = true
		case "candidate-pool":
			poolExplicit = true
			legacyExplicit = true
		case "candidate-name-version":
			nameExplicit = true
		case "candidate-hosted-pool":
			hostedExplicit = true
		case "candidate-hosted-recovery":
			recoveryExplicit = true
		case "account-fixtures", "candidate-manifests", "candidate-stable", "candidate-entries", "candidate-events", "candidate-seeds", "candidate-cards", "candidate-bouts", "candidate-results", "career":
			legacyExplicit = true
		}
	})
	if remoteArgs {
		if (!*remote && !*recoverableIssue) || legacyExplicit || hostedExplicit || recoveryExplicit || ownedExplicit || flags.NArg() != 0 || validateRemoteLaunchArgs(args) != nil {
			return errRemoteLaunchInvalid
		}
		options, e := poolselection.HostedOptions(*nameVersion, nameExplicit, *data, dataExplicit)
		if e != nil {
			return errRemoteLaunchInvalid
		}
		if *dailyWallet {
			input, ok := stdin.(io.ReadCloser)
			if !ok {
				return errRemoteLaunchInvalid
			}
			output, ok := stdout.(io.WriteCloser)
			if !ok {
				return errRemoteLaunchInvalid
			}
			if *dailyWalletVersion == "2" {
				return runDailyWalletLaunch2(ctx, input, output, remoteLaunchPaths{*data, *attachment, *remoteConfig, *remoteRPC, *poolCredential, *admissionCredential}, options)
			}
			return runDailyWalletLaunch(ctx, input, output, remoteLaunchPaths{*data, *attachment, *remoteConfig, *remoteRPC, *poolCredential, *admissionCredential}, options)
		}
		if *daily || *recoverableIssue {
			output, ok := stdout.(*os.File)
			if !ok {
				return errRemoteLaunchInvalid
			}
			return runDailyRemoteLaunchSelection(ctx, output, remoteLaunchPaths{*data, *attachment, *remoteConfig, *remoteRPC, *poolCredential, *admissionCredential}, options, *recoverableIssue)
		}
		input, ok := stdin.(io.ReadCloser)
		if !ok {
			return errRemoteLaunchInvalid
		}
		output, ok := stdout.(io.WriteCloser)
		if !ok {
			return errRemoteLaunchInvalid
		}
		return runRemoteLaunch(ctx, input, output, remoteLaunchPaths{*data, *attachment, *remoteConfig, *remoteRPC, *poolCredential, *admissionCredential}, options)
	}
	if flags.NArg() != 0 || *data == "" || *node == "" {
		return errors.New("explicit data-dir and node-binary are required")
	}
	if ownedExplicit {
		if !*owned || legacyExplicit || hostedExplicit || recoveryExplicit {
			return errors.New("candidate-owned-gym requires a separate explicit true selection")
		}
		options, e := poolselection.Options(true, true, *nameVersion, nameExplicit, *data, dataExplicit)
		if e != nil {
			return e
		}
		input, ok := stdin.(io.ReadCloser)
		if !ok {
			return errors.New("owned input must be closeable")
		}
		output, ok := stdout.(io.WriteCloser)
		if !ok {
			return errors.New("owned output must be closeable")
		}
		c, e := accountpilot.NewOwnedGymPool(ctx, pilot.Config{DataDir: *data, NodeBinary: *node}, options)
		if e != nil {
			return e
		}
		return pilot.ServeOwnedGym(ctx, input, output, c)
	}
	if *recovery {
		if legacyExplicit || hostedExplicit {
			return errors.New("candidate-hosted-recovery is a separate explicit mode")
		}
		options, err := poolselection.HostedRecoveryOptions(*nameVersion, nameExplicit, *data, dataExplicit)
		if err != nil {
			return err
		}
		c, err := accountpilot.NewHostedRecoveryPool(ctx, pilot.Config{DataDir: *data, NodeBinary: *node}, options)
		if err != nil {
			return err
		}
		return pilot.ServeHostedRecovery(ctx, stdin, stdout, c)
	}
	if *hosted {
		if legacyExplicit || recoveryExplicit {
			return errors.New("candidate-hosted-pool is a separate explicit mode")
		}
		options, err := poolselection.HostedOptions(*nameVersion, nameExplicit, *data, dataExplicit)
		if err != nil {
			return err
		}
		c, err := accountpilot.NewHostedFixturePool(ctx, pilot.Config{DataDir: *data, NodeBinary: *node}, options)
		if err != nil {
			return err
		}
		return pilot.ServePool(ctx, stdin, stdout, c)
	}
	if recoveryExplicit {
		return errors.New("candidate-hosted-recovery requires explicit true selection")
	}
	options, selectionErr := poolselection.Options(*pool, poolExplicit, *nameVersion, nameExplicit, *data, dataExplicit)
	if selectionErr != nil {
		return selectionErr
	}
	if *pool {
		if *accounts || *candidates || *stable || *entries || *candidateEvents || *candidateSeeds || *candidateCards || *candidateBouts || *candidateResults || *career {
			return errors.New("candidate-pool is a separate mode")
		}
		c, e := accountpilot.NewPoolWithOptions(ctx, pilot.Config{DataDir: *data, NodeBinary: *node}, options)
		if e != nil {
			return e
		}
		return pilot.ServePool(ctx, stdin, stdout, c)
	}
	if *stable && (!*candidates || *accounts) {
		return errors.New("candidate-stable requires candidate-manifests")
	}
	if *entries && (!*stable || !*candidates || *accounts) {
		return errors.New("candidate-entries requires candidate-stable and candidate-manifests")
	}
	if *candidateEvents && (!*entries || !*stable || !*candidates || *accounts) {
		return errors.New("candidate-events requires candidate-entries, candidate-stable and candidate-manifests")
	}
	if *candidateResults && (!*candidateBouts || !*candidateCards || !*candidateSeeds || !*candidateEvents || !*entries || !*stable || !*candidates || *accounts) {
		return errors.New("candidate-results requires candidate-bouts and all earlier candidate modes")
	}
	if *candidateBouts && (!*candidateCards || !*candidateSeeds || !*candidateEvents || !*entries || !*stable || !*candidates || *accounts) {
		return errors.New("candidate-bouts requires candidate-cards and all earlier candidate modes")
	}
	if *candidateCards && (!*candidateSeeds || !*candidateEvents || !*entries || !*stable || !*candidates || *accounts) {
		return errors.New("candidate-cards requires candidate-seeds, candidate-events, candidate-entries, candidate-stable and candidate-manifests")
	}
	if *candidateSeeds && (!*candidateEvents || !*entries || !*stable || !*candidates || *accounts) {
		return errors.New("candidate-seeds requires candidate-events, candidate-entries, candidate-stable and candidate-manifests")
	}
	if *career && (!*candidateResults || !*candidateBouts || !*candidateCards || !*candidateSeeds || !*candidateEvents || !*entries || !*stable || !*candidates || *accounts) {
		return errors.New("career requires candidate-results and all earlier candidate modes")
	}
	if *candidates {
		if *career {
			c, e := accountpilot.NewCareers(ctx, pilot.Config{DataDir: *data, NodeBinary: *node})
			if e != nil {
				return e
			}
			return pilot.ServeCareers(ctx, c, stdin, stdout)
		}
		if *candidateResults {
			c, e := accountpilot.NewResults(ctx, pilot.Config{DataDir: *data, NodeBinary: *node})
			if e != nil {
				return e
			}
			return pilot.ServeResults(ctx, c, stdin, stdout)
		}
		if *candidateBouts {
			controller, e := accountpilot.NewBouts(ctx, pilot.Config{DataDir: *data, NodeBinary: *node})
			if e != nil {
				return e
			}
			return pilot.ServeBouts(ctx, controller, stdin, stdout)
		}
		if *candidateCards {
			controller, e := accountpilot.NewCards(pilot.Config{DataDir: *data, NodeBinary: *node})
			if e != nil {
				return e
			}
			return pilot.ServeCards(ctx, controller, stdin, stdout)
		}
		if *candidateSeeds {
			controller, err := accountpilot.NewSeeds(pilot.Config{DataDir: *data, NodeBinary: *node})
			if err != nil {
				return err
			}
			return pilot.ServeSeeds(ctx, controller, stdin, stdout)
		}
		if *candidateEvents {
			controller, err := accountpilot.NewEvents(pilot.Config{DataDir: *data, NodeBinary: *node})
			if err != nil {
				return err
			}
			return pilot.ServeEvents(ctx, controller, stdin, stdout)
		}
		if *entries {
			controller, err := accountpilot.NewEntries(pilot.Config{DataDir: *data, NodeBinary: *node})
			if err != nil {
				return err
			}
			return pilot.ServeEntries(ctx, controller, stdin, stdout)
		}
		if *stable {
			controller, err := accountpilot.NewStable(pilot.Config{DataDir: *data, NodeBinary: *node})
			if err != nil {
				return err
			}
			return pilot.ServePreview(ctx, stdin, stdout, controller)
		}
		if *accounts {
			return errors.New("candidate-manifests and account-fixtures are separate modes")
		}
		controller, err := accountpilot.NewManifest(pilot.Config{DataDir: *data, NodeBinary: *node})
		if err != nil {
			return err
		}
		return pilot.ServeManifest(ctx, stdin, stdout, controller)
	}
	var controller pilot.ControllerAPI
	var e error
	if *accounts {
		controller, e = accountpilot.New(pilot.Config{DataDir: *data, NodeBinary: *node})
	} else {
		controller, e = pilot.New(pilot.Config{DataDir: *data, NodeBinary: *node})
	}
	if e != nil {
		return e
	}
	return pilot.Serve(ctx, stdin, stdout, controller)
}
func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if e := run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr); e != nil && !errors.Is(e, flag.ErrHelp) {
		fmt.Fprintln(os.Stderr, "slugpilot:", e)
		os.Exit(1)
	}
}
