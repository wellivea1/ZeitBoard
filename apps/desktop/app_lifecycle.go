package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/options"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"non24.app/desktop/platform/autostart"
	"non24.app/desktop/platform/tray"
)

// Window state is separate from worker lifetime. Tray callbacks and DOM-ready
// can arrive on different goroutines; neither may strand a hidden first launch.
type desktopWindow struct {
	mu              sync.Mutex
	ctx             context.Context
	ready           bool
	trayInitialized bool
	trayAvailable   bool
	startHidden     bool
	showPending     bool
	actions         windowActions
}

type windowActions struct {
	show func(context.Context)
	hide func(context.Context)
	quit func(context.Context)
}

func newDesktopWindow() desktopWindow {
	return desktopWindow{actions: windowActions{
		show: func(ctx context.Context) { wailsruntime.WindowUnminimise(ctx); wailsruntime.WindowShow(ctx) },
		hide: wailsruntime.WindowHide,
		quit: wailsruntime.Quit,
	}}
}

func launchedInBackground(args []string) bool {
	for _, arg := range args {
		if arg == "--background" {
			return true
		}
	}
	return false
}

func desktopInstanceID(dir string) string {
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	digest := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(dir))))
	return "zeitboard_" + hex.EncodeToString(digest[:16])
}

func (a *App) onDomReady(ctx context.Context) {
	a.window.mu.Lock()
	a.window.ctx, a.window.ready = ctx, true
	show := a.window.showPending || (a.window.startHidden && a.window.trayInitialized && !a.window.trayAvailable)
	a.window.showPending = false
	a.window.mu.Unlock()
	if show {
		a.window.actions.show(ctx)
	}
}

func (a *App) showDesktopWindow() {
	a.window.mu.Lock()
	ctx, ready := a.window.ctx, a.window.ready
	if !ready {
		a.window.showPending = true
	}
	a.window.mu.Unlock()
	if ready {
		a.window.actions.show(ctx)
	}
}

func (a *App) setTrayAvailable(available bool) {
	a.window.mu.Lock()
	changed := !a.window.trayInitialized || a.window.trayAvailable != available
	a.window.trayInitialized, a.window.trayAvailable = true, available
	ctx, ready := a.window.ctx, a.window.ready
	a.window.mu.Unlock()
	if changed && !available && ready && !a.closing.Load() {
		a.window.actions.show(ctx)
	}
}

func (a *App) startDesktopTray(ctx context.Context) {
	err := a.tray.Start(tray.Callbacks{
		Show:                a.showDesktopWindow,
		Quit:                func() { a.closing.Store(true); a.window.actions.quit(ctx) },
		AvailabilityChanged: a.setTrayAvailable,
	})
	if err != nil {
		a.setTrayAvailable(false)
		a.setMedicationReminderError("Desktop notifications are unavailable; enabled reminders will not be shown.")
	} else {
		a.startMedicationReminderService(ctx)
	}
}

func (a *App) beforeClose(ctx context.Context) bool {
	if a.closing.Load() {
		return false
	}
	a.window.mu.Lock()
	canHide := a.window.ready && a.window.trayAvailable
	a.window.mu.Unlock()
	if canHide {
		a.window.actions.hide(ctx)
		return true
	}
	a.closing.Store(true)
	return false
}

func (a *App) HideWindow() error {
	a.window.mu.Lock()
	ctx, canHide := a.window.ctx, a.window.ready && a.window.trayAvailable
	a.window.mu.Unlock()
	if !canHide {
		return errors.New("The tray is unavailable; keep the window open or quit ZeitBoard.")
	}
	a.window.actions.hide(ctx)
	return nil
}

func (a *App) QuitApp() {
	a.window.mu.Lock()
	ctx, ready := a.window.ctx, a.window.ready
	a.window.mu.Unlock()
	if ready {
		a.closing.Store(true)
		a.window.actions.quit(ctx)
	}
}

func (a *App) onSecondInstance(data options.SecondInstanceData) {
	if !launchedInBackground(data.Args) {
		a.showDesktopWindow()
	}
}

type StartupSettingsInput struct {
	Enabled     bool `json:"enabled"`
	StartHidden bool `json:"startHidden"`
}

type StartupSettingsDTO struct {
	Available      bool   `json:"available"`
	Registered     bool   `json:"registered"`
	MatchesCurrent bool   `json:"matchesCurrent"`
	StartHidden    bool   `json:"startHidden"`
	TrayAvailable  bool   `json:"trayAvailable"`
	Message        string `json:"message"`
}

func (a *App) startupControllerLocked() (autostart.Controller, error) {
	if a.startupControl != nil {
		return a.startupControl, nil
	}
	if strings.TrimSpace(os.Getenv("ZEITBOARD_DATA_DIR")) != "" {
		return nil, errors.New("Login startup is unavailable for an isolated data profile.")
	}
	executable, err := os.Executable()
	if err != nil {
		return nil, errors.New("The current executable could not be located.")
	}
	a.startupControl = autostart.New(executable)
	return a.startupControl, nil
}

func (a *App) startupSettingsLocked() (StartupSettingsDTO, error) {
	a.window.mu.Lock()
	result := StartupSettingsDTO{TrayAvailable: a.window.trayAvailable}
	a.window.mu.Unlock()
	controller, err := a.startupControllerLocked()
	if err != nil {
		result.Message = err.Error()
		return result, nil
	}
	registration, err := controller.Read()
	if err != nil {
		return result, errors.New("Windows startup registration could not be read. Refresh to try again.")
	}
	result.Available, result.Registered, result.MatchesCurrent, result.StartHidden = registration.Available, registration.Registered, registration.MatchesCurrent, registration.StartHidden
	if !result.Available {
		result.Message = "Login startup is unavailable on this platform."
	}
	if result.Registered && !result.MatchesCurrent {
		result.Message = "The startup entry points to another installation. Save to use this executable, or disable login startup."
	}
	return result, nil
}

func (a *App) GetStartupSettings() (StartupSettingsDTO, error) {
	a.startupMu.Lock()
	defer a.startupMu.Unlock()
	return a.startupSettingsLocked()
}

func (a *App) SetStartupSettings(input StartupSettingsInput) (StartupSettingsDTO, error) {
	a.startupMu.Lock()
	defer a.startupMu.Unlock()
	if a.closing.Load() {
		return StartupSettingsDTO{}, errors.New("ZeitBoard is quitting; reopen it to change login startup.")
	}
	controller, err := a.startupControllerLocked()
	if err != nil {
		return StartupSettingsDTO{}, err
	}
	if err := controller.Set(input.Enabled, input.StartHidden); err != nil {
		return StartupSettingsDTO{}, errors.New("Login startup could not be changed. Check that this executable is in a permanent location with a shorter path.")
	}
	return a.startupSettingsLocked()
}
