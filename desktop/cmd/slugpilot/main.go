// slugpilot owns only a disposable loopback mining environment and public
// fixture signer. Its stdout is exclusively the bounded JSON-lines protocol.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/taytr-llc/slug/desktop/accountpilot"
	"github.com/taytr-llc/slug/desktop/pilot"
	"io"
	"os"
	"os/signal"
	"syscall"
)

func run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("slugpilot", flag.ContinueOnError)
	flags.SetOutput(stderr)
	data := flags.String("data-dir", "", "Explicit disposable pilot directory")
	node := flags.String("node-binary", "", "Explicit bundled normal SLUG node binary")
	accounts := flags.Bool("account-fixtures", false, "Use the separate two-public-fixture account pilot")
	candidates := flags.Bool("candidate-manifests", false, "Use the separate candidate fixture desktop")
	stable := flags.Bool("candidate-stable", false, "Enable the candidate stable within candidate-manifests")
	entries := flags.Bool("candidate-entries", false, "Enable Saturday entry intake within candidate-stable")
	candidateEvents := flags.Bool("candidate-events", false, "Enable immutable noon snapshots within candidate-entries")
	candidateSeeds := flags.Bool("candidate-seeds", false, "Enable candidate seed publication within candidate-events")
	if e := flags.Parse(args); e != nil {
		return e
	}
	if flags.NArg() != 0 || *data == "" || *node == "" {
		return errors.New("explicit data-dir and node-binary are required")
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
	if *candidateSeeds && (!*candidateEvents || !*entries || !*stable || !*candidates || *accounts) {
		return errors.New("candidate-seeds requires candidate-events, candidate-entries, candidate-stable and candidate-manifests")
	}
	if *candidates {
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
