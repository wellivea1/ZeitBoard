//go:build windows

package tray

import "testing"

func TestWindowsTrayDispatchesShowAndQuitCallbacks(t *testing.T) {
	controller := newPlatformController().(*windowsController)
	showCount := 0
	quitCount := 0
	controller.callbacks = Callbacks{
		Show: func() { showCount++ },
		Quit: func() { quitCount++ },
	}

	controller.windowProc(0, wmTray, 0, wmLButtonDblClk)
	controller.windowProc(0, wmCommand, idShow, 0)
	controller.windowProc(0, wmCommand, idQuit, 0)

	if showCount != 2 {
		t.Fatalf("show callback dispatched %d times, want 2", showCount)
	}
	if quitCount != 1 {
		t.Fatalf("quit callback dispatched %d times, want 1", quitCount)
	}
}
