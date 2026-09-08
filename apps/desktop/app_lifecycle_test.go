package main

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/wailsapp/wails/v2/pkg/options"
	"non24.app/desktop/platform/autostart"
)

func lifecycleApp(t *testing.T) (*App, *atomic.Int32, *atomic.Int32, *atomic.Int32) {
	t.Helper()
	a := newTestApp(t)
	show, hide, quit := &atomic.Int32{}, &atomic.Int32{}, &atomic.Int32{}
	a.window.actions = windowActions{show: func(context.Context) { show.Add(1) }, hide: func(context.Context) { hide.Add(1) }, quit: func(context.Context) { quit.Add(1) }}
	return a, show, hide, quit
}

func TestHiddenLaunchRecoversWhenTrayUnavailableInEitherStartupOrder(t *testing.T) {
	for _, domFirst := range []bool{false, true} {
		a, show, _, _ := lifecycleApp(t)
		a.window.startHidden = true
		if domFirst {
			a.onDomReady(context.Background())
			a.setTrayAvailable(false)
		} else {
			a.setTrayAvailable(false)
			a.onDomReady(context.Background())
		}
		if show.Load() != 1 {
			t.Fatal("hidden launch was stranded or shown twice")
		}
		if a.beforeClose(context.Background()) {
			t.Fatal("tray failure prevented closing")
		}
		if err := a.HideWindow(); err == nil {
			t.Fatal("hid without a recovery surface")
		}
	}
}

func TestCloseHidesButExplicitQuitAndTrayLossDoNot(t *testing.T) {
	a, show, hide, quit := lifecycleApp(t)
	a.window.startHidden = true
	a.setTrayAvailable(true)
	a.onDomReady(context.Background())
	if show.Load() != 0 {
		t.Fatal("background launch opened a window despite usable tray")
	}
	if !a.beforeClose(context.Background()) || hide.Load() != 1 || a.closing.Load() {
		t.Fatal("ordinary close stopped the process")
	}
	a.setTrayAvailable(false)
	if show.Load() != 1 {
		t.Fatal("lost tray did not reveal the window")
	}
	a.setTrayAvailable(true)
	a.QuitApp()
	if quit.Load() != 1 || a.beforeClose(context.Background()) || !a.closing.Load() {
		t.Fatal("explicit quit was intercepted as hide")
	}
}

func TestSecondLaunchReopensExistingWindowWithoutInterruptingBackgroundLaunch(t *testing.T) {
	a, show, _, _ := lifecycleApp(t)
	a.window.startHidden = true
	a.setTrayAvailable(true)
	a.onSecondInstance(options.SecondInstanceData{Args: []string{"--background"}})
	a.onSecondInstance(options.SecondInstanceData{Args: []string{}})
	if show.Load() != 0 {
		t.Fatal("show happened before webview readiness")
	}
	a.onDomReady(context.Background())
	if show.Load() != 1 {
		t.Fatal("manual second launch was dropped")
	}
	a.onSecondInstance(options.SecondInstanceData{Args: []string{"--background"}})
	if show.Load() != 1 {
		t.Fatal("background launch interrupted the window")
	}
	a.onSecondInstance(options.SecondInstanceData{})
	if show.Load() != 2 {
		t.Fatal("manual launch did not reopen")
	}
	first := filepath.Join(t.TempDir(), "first")
	if desktopInstanceID(first) == desktopInstanceID(first+"-other") {
		t.Fatal("isolated profiles share an instance")
	}
}

type fakeAutostart struct {
	value   autostart.Registration
	writes  int
	failure bool
}

func (f *fakeAutostart) Read() (autostart.Registration, error) { return f.value, nil }
func (f *fakeAutostart) Set(enabled, hidden bool) error {
	if f.failure {
		return errors.New("synthetic denied registry write")
	}
	f.writes++
	f.value = autostart.Registration{Available: true, Registered: enabled, MatchesCurrent: enabled, StartHidden: hidden}
	return nil
}

func TestLoginStartupIsExplicitAndDoesNotGrantCollectionOrSync(t *testing.T) {
	a, _, _, _ := lifecycleApp(t)
	fake := &fakeAutostart{value: autostart.Registration{Available: true}}
	a.startupControl = fake
	if s, err := a.GetStartupSettings(); err != nil || s.Registered || fake.writes != 0 {
		t.Fatalf("implicit startup write: %#v %v", s, err)
	}
	if s, err := a.SetStartupSettings(StartupSettingsInput{Enabled: true, StartHidden: true}); err != nil || !s.Registered || fake.writes != 1 {
		t.Fatalf("explicit startup: %#v %v", s, err)
	}
	activity, err := a.GetActivityCollection()
	if err != nil || activity.Enabled {
		t.Fatal("startup granted activity consent")
	}
	sync, err := a.GetBackendSyncStatus()
	if err != nil || sync.Enabled {
		t.Fatal("startup enrolled a backend")
	}
	fake.failure = true
	if _, err := a.SetStartupSettings(StartupSettingsInput{}); err == nil {
		t.Fatal("failed write reported success")
	}
	if s, err := a.GetStartupSettings(); err != nil || !s.Registered {
		t.Fatal("failed write forgot durable registration")
	}
}
