//go:build windows

package tray

import (
	"testing"
	"time"
)

func TestWindowsTrayDispatchesShowAndQuitCallbacks(t *testing.T) {
	controller := newPlatformController().(*windowsController)
	show := make(chan struct{}, 1)
	quit := make(chan struct{}, 1)
	if err := controller.Start(Callbacks{
		Show: func() { show <- struct{}{} },
		Quit: func() { quit <- struct{}{} },
	}); err != nil {
		t.Fatal(err)
	}
	defer controller.Stop()

	controller.mu.Lock()
	window := controller.window
	controller.mu.Unlock()
	if window == 0 {
		t.Fatal("tray message window was not created")
	}
	procPostMessage.Call(window, wmTray, 0, wmLButtonDblClk)
	select {
	case <-show:
	case <-time.After(2 * time.Second):
		t.Fatal("show callback was not dispatched")
	}

	procPostMessage.Call(window, wmCommand, idQuit, 0)
	select {
	case <-quit:
	case <-time.After(2 * time.Second):
		t.Fatal("quit callback was not dispatched")
	}
}
