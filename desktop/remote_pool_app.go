// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// A failed join is sticky and never becomes permission to exit normally.
type nativeRemoteQuitState struct {
	mu              sync.Mutex
	started, joined bool
	failure         error
}

func (q *nativeRemoteQuitState) begin() (allow, start bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.joined {
		return true, false
	}
	if q.started {
		return false, false
	}
	q.started = true
	return false, true
}
func (q *nativeRemoteQuitState) finish(e error) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.started || q.joined || q.failure != nil {
		return q.joined
	}
	q.failure = e
	q.joined = e == nil
	return q.joined
}

func runNativeRemote(args []string) (resultErr error) {
	selection, e := parseNativeRemoteArgs(args)
	if e != nil {
		return e
	}
	executable, e := os.Executable()
	if e != nil {
		return errNativeRemoteUnavailable
	}
	helper := filepath.Clean(filepath.Join(filepath.Dir(executable), "..", "Resources", "bin", "slugpilot"))
	signals := newNativeRemoteSignals()
	defer signals.close()
	owner, e := openNativeRemote(signals.life, helper, selection)
	if e != nil {
		return e
	}
	frontend, e := fs.Sub(assets, "frontend")
	if e != nil {
		return owner.failure(errNativeRemoteUnavailable, true)
	}
	gym := &RemoteGym{owner: owner}
	const title = "SLUG Remote Pool Pilot"
	var app *application.App
	var quit nativeRemoteQuitState
	var menuWork sync.WaitGroup
	defer func() {
		quit.mu.Lock()
		quit.started = true
		quit.mu.Unlock()
		e := owner.close()
		menuWork.Wait()
		if e != nil {
			resultErr = &nativeRemoteError{cause: errors.Join(resultErr, e), retained: owner}
		}
	}()
	requestQuit := func() bool { return false }
	requestQuit = func() bool {
		allow, start := quit.begin()
		if start {
			go func() {
				e := owner.close()
				menuWork.Wait()
				if quit.finish(e) {
					app.Quit()
					return
				}
				// Keep the original failed bridge/config owner reachable. There
				// is no second helper, forced process kill or clean-exit claim.
				app.Dialog.Error().SetTitle(title).SetMessage(errNativeRemoteCleanup.Error()).Show()
			}()
		}
		return allow
	}
	app = application.New(application.Options{
		Name: title, Description: "Explicit remote pool attachment for an existing demo wallet.", Icon: appIcon,
		Services:                    []application.Service{application.NewService(gym)},
		Assets:                      application.AssetOptions{Handler: http.FileServer(http.FS(frontend)), Middleware: assetSecurity, DisableLogging: true},
		Logger:                      slog.New(slog.NewTextHandler(io.Discard, nil)),
		Mac:                         application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: false},
		DisableDefaultSignalHandler: true,
		ShouldQuit:                  requestQuit,
		OnShutdown: func() {
			// Every supported Quit route already joined. This defensive close
			// still retains the original owner for a framework shutdown error.
			_ = owner.close()
		},
	})
	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title: title, Name: title, Width: 1180, Height: 820, MinWidth: 820, MinHeight: 640, URL: "/remote.html",
		DevToolsEnabled: false, DefaultContextMenuDisabled: true, EnableFileDrop: false,
		Permissions: map[application.PermissionType]application.Permission{
			application.PermissionMicrophone: application.PermissionDeny, application.PermissionCamera: application.PermissionDeny,
			application.PermissionGeolocation: application.PermissionDeny, application.PermissionNotifications: application.PermissionDeny,
			application.PermissionClipboardRead: application.PermissionDeny,
		},
	})
	window.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) { window.Hide(); event.Cancel() })
	menuGate := make(chan struct{}, 1)
	action := func(command string) func(*application.Context) {
		return func(*application.Context) {
			// Serialize Add with quit admission. Once quitting starts, no later
			// menu goroutine can be added behind the shutdown join.
			quit.mu.Lock()
			if quit.started {
				quit.mu.Unlock()
				return
			}
			select {
			case menuGate <- struct{}{}:
			default:
				quit.mu.Unlock()
				return
			}
			dispatch := gym.menuAction(command) // capture the original opaque view NOW
			menuWork.Add(1)
			quit.mu.Unlock()
			go func() {
				defer menuWork.Done()
				defer func() { <-menuGate }()
				if e := dispatch(); e != nil {
					app.Dialog.Error().SetTitle(title).SetMessage(e.Error()).Show()
				}
			}()
		}
	}
	show := func(*application.Context) { window.Show(); window.Focus() }
	quitClick := func(*application.Context) {
		if requestQuit() {
			app.Quit()
		}
	}
	menu := app.NewMenu()
	controls := menu.AddSubmenu(title)
	controls.Add("Show gym").OnClick(show)
	controls.Add("Start mining").OnClick(action("start"))
	controls.Add("Stop mining").OnClick(action("stop"))
	controls.AddSeparator()
	controls.Add("Quit").OnClick(quitClick)
	app.Menu.Set(menu)
	tray := app.SystemTray.New()
	tray.SetIconPosition(application.NSImageOnly).SetTemplateIcon(menuBarIcon)
	tray.SetTooltip(title)
	trayMenu := app.NewMenu()
	trayMenu.Add("Show gym").OnClick(show)
	trayMenu.Add("Start mining").OnClick(action("start"))
	trayMenu.Add("Stop mining").OnClick(action("stop"))
	trayMenu.AddSeparator()
	trayMenu.Add("Quit").OnClick(quitClick)
	tray.SetMenu(trayMenu)
	// The same signal owner has protected bootstrap from before helper launch.
	// Handoff occurs only after Wails has a running application. A signal in
	// the pre-Run gap is remembered by life cancellation and joined here.
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		if !signals.handoff(func() { requestQuit() }) {
			requestQuit()
		}
	})
	if e = signals.life.Err(); e != nil {
		return owner.failure(e, true)
	}
	if e = app.Run(); e != nil {
		return &nativeRemoteError{cause: e}
	}
	return nil
}
