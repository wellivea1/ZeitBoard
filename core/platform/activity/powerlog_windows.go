//go:build windows

package activity

import (
	"errors"
	"fmt"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The System log is read through the Windows Event Log API, asking for the
// six power entries by query so that no other entry is handed over.
var (
	wevtapi       = windows.NewLazySystemDLL("wevtapi.dll")
	procEvtQuery  = wevtapi.NewProc("EvtQuery")
	procEvtNext   = wevtapi.NewProc("EvtNext")
	procEvtRender = wevtapi.NewProc("EvtRender")
	procEvtClose  = wevtapi.NewProc("EvtClose")
)

const (
	evtQueryChannelPath      = 0x1
	evtQueryForwardDirection = 0x100
	evtRenderEventXML        = 1
	evtBatch                 = 64
	evtWaitMilliseconds      = 5000

	// maxRenderedEntry bounds one entry's rendering. A power entry is about
	// a kilobyte; anything near this is not one.
	maxRenderedEntry = 64 << 10
)

type windowsPowerLog struct{}

func platformPowerLog() PowerLog { return windowsPowerLog{} }

func (windowsPowerLog) After(since time.Time, limit int) ([]PowerEvent, error) {
	if limit <= 0 {
		return nil, nil
	}
	channel, err := windows.UTF16PtrFromString("System")
	if err != nil {
		return nil, err
	}
	query, err := windows.UTF16PtrFromString(powerLogQuery(since))
	if err != nil {
		return nil, err
	}
	results, _, callErr := procEvtQuery.Call(0, uintptr(unsafe.Pointer(channel)), uintptr(unsafe.Pointer(query)),
		evtQueryChannelPath|evtQueryForwardDirection)
	if results == 0 {
		return nil, fmt.Errorf("the System log could not be read: %w", callErr)
	}
	defer procEvtClose.Call(results)

	var events []PowerEvent
	handles := make([]uintptr, evtBatch)
	buffer := make([]uint16, 2048)
	for len(events) < limit {
		var returned uint32
		ok, _, callErr := procEvtNext.Call(results, uintptr(len(handles)), uintptr(unsafe.Pointer(&handles[0])),
			evtWaitMilliseconds, 0, uintptr(unsafe.Pointer(&returned)))
		if ok == 0 {
			if errors.Is(callErr, windows.ERROR_NO_MORE_ITEMS) {
				break
			}
			return events, fmt.Errorf("the System log could not be read: %w", callErr)
		}
		for _, handle := range handles[:returned] {
			rendered, renderErr := renderEntry(handle, &buffer)
			procEvtClose.Call(handle)
			if renderErr != nil || len(events) >= limit {
				continue
			}
			if event, known := parsePowerEntry(rendered); known {
				events = append(events, event)
			}
		}
	}
	return events, nil
}

// renderEntry renders one entry as XML, growing the buffer once if the entry
// needs more room.
func renderEntry(handle uintptr, buffer *[]uint16) ([]byte, error) {
	for {
		var used, properties uint32
		ok, _, err := procEvtRender.Call(0, handle, evtRenderEventXML, uintptr(len(*buffer)*2),
			uintptr(unsafe.Pointer(&(*buffer)[0])), uintptr(unsafe.Pointer(&used)), uintptr(unsafe.Pointer(&properties)))
		if ok != 0 {
			return []byte(windows.UTF16ToString((*buffer)[:used/2])), nil
		}
		if !errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER) || int(used/2) <= len(*buffer) || used > maxRenderedEntry {
			return nil, err
		}
		*buffer = make([]uint16, used/2)
	}
}
