//go:build windows

package tray

import (
	"errors"
	"runtime"
	"sync"
	"syscall"
	"unsafe"
)

const (
	wmDestroy       = 0x0002
	wmClose         = 0x0010
	wmCommand       = 0x0111
	wmLButtonDblClk = 0x0203
	wmRButtonUp     = 0x0205
	wmTray          = 0x8001

	nimAdd     = 0x00000000
	nimDelete  = 0x00000002
	nifMessage = 0x00000001
	nifIcon    = 0x00000002
	nifTip     = 0x00000004

	mfString       = 0x00000000
	tpmRightButton = 0x0002
	tpmNonotify    = 0x0080
	tpmReturnCmd   = 0x0100

	idiApplication = 32512
	idShow         = 1001
	idQuit         = 1002
)

var (
	user32               = syscall.NewLazyDLL("user32.dll")
	shell32              = syscall.NewLazyDLL("shell32.dll")
	kernel32             = syscall.NewLazyDLL("kernel32.dll")
	procRegisterClassEx  = user32.NewProc("RegisterClassExW")
	procCreateWindowEx   = user32.NewProc("CreateWindowExW")
	procDefWindowProc    = user32.NewProc("DefWindowProcW")
	procDestroyWindow    = user32.NewProc("DestroyWindow")
	procGetMessage       = user32.NewProc("GetMessageW")
	procTranslateMessage = user32.NewProc("TranslateMessage")
	procDispatchMessage  = user32.NewProc("DispatchMessageW")
	procPostMessage      = user32.NewProc("PostMessageW")
	procPostQuitMessage  = user32.NewProc("PostQuitMessage")
	procLoadIcon         = user32.NewProc("LoadIconW")
	procLoadCursor       = user32.NewProc("LoadCursorW")
	procCreatePopupMenu  = user32.NewProc("CreatePopupMenu")
	procAppendMenu       = user32.NewProc("AppendMenuW")
	procTrackPopupMenu   = user32.NewProc("TrackPopupMenu")
	procSetForeground    = user32.NewProc("SetForegroundWindow")
	procGetCursorPos     = user32.NewProc("GetCursorPos")
	procDestroyMenu      = user32.NewProc("DestroyMenu")
	procShellNotifyIcon  = shell32.NewProc("Shell_NotifyIconW")
	procGetModuleHandle  = kernel32.NewProc("GetModuleHandleW")
)

type point struct{ X, Y int32 }

type message struct {
	HWnd    uintptr
	Message uint32
	_       uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
	Private uint32
}

type windowClassEx struct {
	Size        uint32
	Style       uint32
	WindowProc  uintptr
	ClassExtra  int32
	WindowExtra int32
	Instance    uintptr
	Icon        uintptr
	Cursor      uintptr
	Background  uintptr
	MenuName    *uint16
	ClassName   *uint16
	SmallIcon   uintptr
}

type notifyIconData struct {
	Size           uint32
	Window         uintptr
	ID             uint32
	Flags          uint32
	Callback       uint32
	Icon           uintptr
	Tip            [128]uint16
	State          uint32
	StateMask      uint32
	Info           [256]uint16
	TimeoutVersion uint32
	InfoTitle      [64]uint16
	InfoFlags      uint32
	GUID           [16]byte
	BalloonIcon    uintptr
}

type windowsController struct {
	mu        sync.Mutex
	window    uintptr
	callbacks Callbacks
	ready     chan error
	done      chan struct{}
}

func newPlatformController() Controller {
	return &windowsController{}
}

func (controller *windowsController) Start(callbacks Callbacks) error {
	controller.mu.Lock()
	if controller.done != nil {
		controller.mu.Unlock()
		return nil
	}
	controller.callbacks = callbacks
	controller.ready = make(chan error, 1)
	controller.done = make(chan struct{})
	controller.mu.Unlock()
	go controller.run()
	return <-controller.ready
}

func (controller *windowsController) Stop() error {
	controller.mu.Lock()
	window := controller.window
	done := controller.done
	controller.mu.Unlock()
	if window != 0 {
		procPostMessage.Call(window, wmClose, 0, 0)
	}
	if done != nil {
		<-done
	}
	return nil
}

func (controller *windowsController) run() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(controller.done)

	instance, _, _ := procGetModuleHandle.Call(0)
	className, _ := syscall.UTF16PtrFromString("Non24PlannerTrayWindow")
	windowProc := syscall.NewCallback(controller.windowProc)
	icon, _, _ := procLoadIcon.Call(0, idiApplication)
	cursor, _, _ := procLoadCursor.Call(0, 32512)
	class := windowClassEx{
		Size: uint32(unsafe.Sizeof(windowClassEx{})), WindowProc: windowProc,
		Instance: instance, Icon: icon, Cursor: cursor, ClassName: className, SmallIcon: icon,
	}
	if atom, _, callErr := procRegisterClassEx.Call(uintptr(unsafe.Pointer(&class))); atom == 0 {
		controller.ready <- errors.New("register Windows tray class: " + callErr.Error())
		return
	}
	window, _, callErr := procCreateWindowEx.Call(0, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(className)), 0, 0, 0, 0, 0, 0, 0, instance, 0)
	if window == 0 {
		controller.ready <- errors.New("create Windows tray window: " + callErr.Error())
		return
	}
	controller.mu.Lock()
	controller.window = window
	controller.mu.Unlock()

	data := notifyIconData{Size: uint32(unsafe.Sizeof(notifyIconData{})), Window: window, ID: 1, Flags: nifMessage | nifIcon | nifTip, Callback: wmTray, Icon: icon}
	copy(data.Tip[:], syscall.StringToUTF16("ZeitBoard - double-click to open"))
	if result, _, callErr := procShellNotifyIcon.Call(nimAdd, uintptr(unsafe.Pointer(&data))); result == 0 {
		controller.ready <- errors.New("add Windows tray icon: " + callErr.Error())
		procDestroyWindow.Call(window)
		return
	}
	controller.ready <- nil

	var msg message
	for {
		result, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(result) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
	}
	procShellNotifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(&data)))
	controller.mu.Lock()
	controller.window = 0
	controller.mu.Unlock()
}

func (controller *windowsController) windowProc(window uintptr, message uint32, wParam, lParam uintptr) uintptr {
	switch message {
	case wmTray:
		switch uint32(lParam) {
		case wmLButtonDblClk:
			if controller.callbacks.Show != nil {
				controller.callbacks.Show()
			}
		case wmRButtonUp:
			controller.showMenu(window)
		}
		return 0
	case wmCommand:
		switch uint32(wParam & 0xffff) {
		case idShow:
			if controller.callbacks.Show != nil {
				controller.callbacks.Show()
			}
		case idQuit:
			if controller.callbacks.Quit != nil {
				controller.callbacks.Quit()
			}
		}
		return 0
	case wmClose:
		procDestroyWindow.Call(window)
		return 0
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	result, _, _ := procDefWindowProc.Call(window, uintptr(message), wParam, lParam)
	return result
}

func (controller *windowsController) showMenu(window uintptr) {
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer procDestroyMenu.Call(menu)
	show, _ := syscall.UTF16PtrFromString("Open ZeitBoard")
	quit, _ := syscall.UTF16PtrFromString("Quit")
	procAppendMenu.Call(menu, mfString, idShow, uintptr(unsafe.Pointer(show)))
	procAppendMenu.Call(menu, mfString, idQuit, uintptr(unsafe.Pointer(quit)))
	var cursor point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&cursor)))
	procSetForeground.Call(window)
	command, _, _ := procTrackPopupMenu.Call(menu, tpmRightButton|tpmNonotify|tpmReturnCmd, uintptr(cursor.X), uintptr(cursor.Y), 0, window, 0)
	if command != 0 {
		procPostMessage.Call(window, wmCommand, command, 0)
	}
}
