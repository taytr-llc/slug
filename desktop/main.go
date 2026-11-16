// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.
package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/taytr-llc/slug/desktop/internal/bridge"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed frontend/index.html frontend/style.css frontend/app.js
var assets embed.FS

// Gym exposes only public pilot controls and receipt selection to the webview.
// Paths, process arguments, and keys cannot be supplied through the web bridge.
type Gym struct {
	helper   *bridge.Bridge
	app      *application.App
	window   *application.WebviewWindow
	exportMu sync.Mutex
}

func (g *Gym) Snapshot() (pilot.Snapshot, error) {
	response, err := g.helper.Request(context.Background(), "snapshot", 0)
	if err != nil {
		return pilot.Snapshot{}, err
	}
	return *response.Snapshot, nil
}
func (g *Gym) Start() error { _, err := g.helper.Request(context.Background(), "start", 0); return err }
func (g *Gym) Stop() error  { _, err := g.helper.Request(context.Background(), "stop", 0); return err }
func (g *Gym) ExportReceipt(sequence uint64) (string, error) {
	if !g.exportMu.TryLock() {
		return "", bridge.ErrBusy
	}
	defer g.exportMu.Unlock()
	response, err := g.helper.Request(context.Background(), "receipt", sequence)
	if err != nil {
		return "", err
	}
	raw, err := json.MarshalIndent(response.Receipt, "", "  ")
	if err != nil || len(raw) > pilot.MaxResponseSize {
		return "", errors.New("The public receipt is unavailable.")
	}
	path, err := g.app.Dialog.SaveFile().AttachToWindow(g.window).
		SetMessage("Save this local pilot's public prospect receipt.").
		SetFilename("slug-pilot-receipt-"+strconv.FormatUint(sequence, 10)+".json").
		AddFilter("JSON receipt", "*.json").AllowsOtherFileTypes(false).
		CanCreateDirectories(true).PromptForSingleSelection()
	if err != nil {
		return "", errors.New("The save dialog is unavailable.")
	}
	if path == "" {
		return "", nil
	}
	if err = saveReceipt(path, append(raw, '\n')); err != nil {
		return "", errors.New("The receipt could not be saved.")
	}
	return "Receipt saved.", nil
}

const contentPolicy = "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; font-src 'self'; frame-src 'none'; object-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"

func assetSecurity(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", contentPolicy)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}
func run() (resultErr error) {
	dataDefault, err := os.UserConfigDir()
	if err != nil {
		return errors.New("Local application storage is unavailable.")
	}
	dataDefault = filepath.Join(dataDefault, "SLUG", "LocalPilot")
	if value := os.Getenv("SLUG_PILOT_DATA_DIR"); value != "" {
		dataDefault = value
	}
	data := flag.String("data-dir", dataDefault, "Local pilot data directory (never a wallet directory)")
	flag.Parse()
	if flag.NArg() != 0 || !filepath.IsAbs(*data) {
		return errors.New("An absolute local pilot data directory is required.")
	}
	executable, err := os.Executable()
	if err != nil {
		return errors.New("The application location is unavailable.")
	}
	resources := filepath.Clean(filepath.Join(filepath.Dir(executable), "..", "Resources", "bin"))
	helperPath, nodePath := filepath.Join(resources, "slugpilot"), filepath.Join(resources, "slugd")
	if err = os.MkdirAll(filepath.Dir(filepath.Clean(*data)), 0700); err != nil {
		return errors.New("Local application storage is unavailable.")
	}
	helper, err := bridge.Launch(helperPath, filepath.Clean(*data), nodePath)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, helper.Close()) }()
	gym := &Gym{helper: helper}
	frontend, err := fs.Sub(assets, "frontend")
	if err != nil {
		return errors.New("Embedded application assets are unavailable.")
	}
	// Deliberately use the embedded file server, not Wails' development server
	// environment override. There is no remote frontend or external URL input.
	app := application.New(application.Options{
		Name: "SLUG Local Pilot", Description: "An isolated local mining gym using public fixture identity.",
		Services: []application.Service{application.NewService(gym)},
		Assets:   application.AssetOptions{Handler: http.FileServer(http.FS(frontend)), Middleware: assetSecurity, DisableLogging: true},
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Mac:      application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: false},
		OnShutdown: func() {
			if helper.Close() != nil {
				fmt.Fprintln(os.Stderr, "The local pilot reported a shutdown error; retained data was not erased.")
			}
		},
	})
	gym.app = app
	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title: "SLUG — Local Pilot", Name: "SLUG Local Pilot", Width: 1180, Height: 820, MinWidth: 820, MinHeight: 640, URL: "/",
		DevToolsEnabled: false, DefaultContextMenuDisabled: true, EnableFileDrop: false,
		Permissions: map[application.PermissionType]application.Permission{
			application.PermissionMicrophone: application.PermissionDeny, application.PermissionCamera: application.PermissionDeny,
			application.PermissionGeolocation: application.PermissionDeny, application.PermissionNotifications: application.PermissionDeny,
			application.PermissionClipboardRead: application.PermissionDeny,
		},
	})
	gym.window = window
	window.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) { window.Hide(); event.Cancel() })
	menuGate := make(chan struct{}, 1)
	action := func(fn func() error) func(*application.Context) {
		return func(*application.Context) {
			select {
			case menuGate <- struct{}{}:
			default:
				return
			}
			go func() {
				defer func() { <-menuGate }()
				var err error
				for i := 0; i < 5; i++ {
					err = fn()
					if !errors.Is(err, bridge.ErrBusy) {
						break
					}
					time.Sleep(50 * time.Millisecond)
				}
				if err != nil {
					app.Dialog.Error().SetTitle("SLUG Local Pilot").SetMessage(err.Error()).Show()
				}
			}()
		}
	}

	menu := app.NewMenu()
	controls := menu.AddSubmenu("SLUG Local Pilot")
	controls.Add("Show gym").OnClick(func(*application.Context) { window.Show(); window.Focus() })
	controls.Add("Start mining").OnClick(action(gym.Start))
	controls.Add("Stop mining").OnClick(action(gym.Stop))
	controls.AddSeparator()
	controls.Add("Quit").OnClick(func(*application.Context) { app.Quit() })
	app.Menu.Set(menu)
	tray := app.SystemTray.New()
	tray.SetLabel("SLUG")
	trayMenu := app.NewMenu()
	trayMenu.Add("Show gym").OnClick(func(*application.Context) { window.Show(); window.Focus() })
	trayMenu.Add("Start mining").OnClick(action(gym.Start))
	trayMenu.Add("Stop mining").OnClick(action(gym.Stop))
	trayMenu.AddSeparator()
	trayMenu.Add("Quit").OnClick(func(*application.Context) { app.Quit() })
	tray.SetMenu(trayMenu)
	return app.Run()
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
