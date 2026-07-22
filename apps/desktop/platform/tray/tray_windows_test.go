//go:build windows

package tray

import (
	"errors"
	"testing"
)

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

func TestWindowsTrayNotificationRequiresStartedTray(t *testing.T) {
	controller := newPlatformController().(*windowsController)
	if err := controller.Notify("Medication reminder", "Reminder you set: label."); !errors.Is(err, ErrNotificationsUnavailable) {
		t.Fatalf("notification before tray start error = %v", err)
	}
}

func TestCopyUTF16TruncatedPreservesTerminationAndSurrogatePairs(t *testing.T) {
	buffer := []uint16{0xffff, 0xffff, 0xffff}
	if err := copyUTF16Truncated(buffer, "A\U0001F600B"); err != nil {
		t.Fatal(err)
	}
	if buffer[0] != uint16('A') || buffer[1] != 0 || buffer[2] != 0 {
		t.Fatalf("truncation left a partial surrogate pair: %#v", buffer)
	}
	if err := copyUTF16Truncated(buffer, "bad\x00text"); err == nil {
		t.Fatal("notification text with a null character was accepted")
	}
}
