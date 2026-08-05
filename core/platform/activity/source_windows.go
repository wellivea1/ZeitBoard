//go:build windows

package activity

import (
	"errors"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"non24.app/core/ingest"
)

var (
	user32              = windows.NewLazySystemDLL("user32.dll")
	kernel32            = windows.NewLazySystemDLL("kernel32.dll")
	procGetLastInput    = user32.NewProc("GetLastInputInfo")
	procOpenInputDesk   = user32.NewProc("OpenInputDesktop")
	procCloseDesktop    = user32.NewProc("CloseDesktop")
	procGetTickCount64  = kernel32.NewProc("GetTickCount64")
	errLastInputUnknown = errors.New("last input time is unavailable")
)

// lastInputInfo mirrors LASTINPUTINFO. It reports only *when* input last
// happened — never what the input was. There is no variant of this call that
// exposes content, which is why it is the right primitive here.
type lastInputInfo struct {
	cbSize uint32
	dwTime uint32
}

// windowsSource reads machine state through two narrow system calls.
//
// GetLastInputInfo gives time-since-input with no visibility into the input.
// OpenInputDesktop fails with ERROR_ACCESS_DENIED while the workstation is
// locked, which is the standard way to observe lock state without registering
// for session notifications — a message loop would be a larger surface for no
// extra evidence.
type windowsSource struct{}

func platformSource() Source { return windowsSource{} }

func (windowsSource) Capabilities() ingest.Capabilities {
	return ingest.Capabilities{
		ActiveIdle:   true,
		SessionState: true,
		// Suspend and resume are inferred from wall-clock gaps rather than
		// observed directly, so they are not claimed as a power-event
		// capability. Claiming one the source does not have would make a
		// downstream consumer trust a signal that is not there.
		PowerEvents: false,
		ScreenState: false,
	}
}

func (s windowsSource) Sample(now time.Time) (Sample, error) {
	sample := Sample{At: now}

	idle, err := idleDuration()
	if err == nil {
		sample.IdleFor = idle
		sample.IdleKnown = true
	}

	locked, lockErr := workstationLocked()
	if lockErr == nil {
		sample.Locked = locked
		sample.LockedKnown = true
	}

	if !sample.IdleKnown && !sample.LockedKnown {
		return sample, err
	}
	return sample, nil
}

// idleDuration returns time since the last input event.
func idleDuration() (time.Duration, error) {
	info := lastInputInfo{cbSize: uint32(unsafe.Sizeof(lastInputInfo{}))}
	ret, _, _ := procGetLastInput.Call(uintptr(unsafe.Pointer(&info)))
	if ret == 0 {
		return 0, errLastInputUnknown
	}
	ticks, _, _ := procGetTickCount64.Call()
	// Both values count milliseconds since boot. GetLastInputInfo truncates to
	// 32 bits and wraps about every 49 days, so subtract in 32-bit space and
	// widen afterwards; doing it the other way produces a ~49-day idle time
	// shortly after each wrap.
	elapsed := uint32(uint64(ticks)) - info.dwTime
	return time.Duration(elapsed) * time.Millisecond, nil
}

// workstationLocked reports whether the interactive desktop is locked.
func workstationLocked() (bool, error) {
	const desktopSwitchDesktop = 0x0100
	handle, _, err := procOpenInputDesk.Call(0, 0, uintptr(desktopSwitchDesktop))
	if handle == 0 {
		if errors.Is(err, windows.ERROR_ACCESS_DENIED) || errors.Is(err, windows.ERROR_INVALID_FUNCTION) {
			return true, nil
		}
		// Any other failure means the answer is unknown, not that the machine
		// is locked. Reporting a guess would fabricate evidence.
		return false, err
	}
	defer procCloseDesktop.Call(handle)
	return false, nil
}
