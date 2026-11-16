module github.com/taytr-llc/slug/desktop

go 1.27.3

require (
	github.com/syndtr/goleveldb v1.0.1-0.20220721030215-126854af5e6d
	github.com/taytr-llc/slug v0.0.0
	github.com/taytr-llc/slugwallet v0.0.0-20261116234108-e9e578c3bb94
	github.com/wailsapp/wails/v3 v3.1.0
)

require (
	github.com/coder/websocket v1.8.14 // indirect
	github.com/ebitengine/purego v0.11.1 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/term v0.46.0 // indirect
)

replace github.com/taytr-llc/slug => ..

replace github.com/taytr-llc/slugwallet => ../../slugwallet
