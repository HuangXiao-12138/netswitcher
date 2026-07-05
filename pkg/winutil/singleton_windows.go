//go:build windows

package winutil

import (
	"fmt"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Single-instance lock built on a named mutex (ownership) plus a named
// auto-reset event (second instance → first instance "show yourself" signal).
//
// Flow:
//   1. main calls AcquireSingleton() before launching the GUI.
//   2. If it returns owned=false, another instance is running; main calls
//      SignalSingletonShow() (sets the event) and exits.
//   3. If owned=true, the GUI owns the lock. WaitForSingletonShow()
//      returns a channel that fires each time a second instance signals.
//      The GUI wires that to runtime.WindowShow.

const (
	singletonMutexName = `Global\NetSwitcher-Single-Instance`
	singletonEventName = `Global\NetSwitcher-Show-Event`
)

var (
	singletonMu    sync.Mutex
	singletonState struct {
		mutex windows.Handle
		event windows.Handle
		got   bool
	}
)

// AcquireSingleton tries to grab the singleton mutex. Returns owned=true if
// this is the first instance (caller proceeds to run the GUI); owned=false
// if another instance already holds it (caller should signal + exit).
func AcquireSingleton() (owned bool, err error) {
	singletonMu.Lock()
	defer singletonMu.Unlock()
	if singletonState.got {
		return true, nil // already acquired in this process
	}
	mname, _ := windows.UTF16PtrFromString(singletonMutexName)
	mutex, err := windows.CreateMutex(nil, false, mname)
	if err != nil {
		return false, fmt.Errorf("create mutex: %w", err)
	}
	// ERROR_ALREADY_EXISTS (183) means another instance owns the mutex.
	if windows.GetLastError() == windows.ERROR_ALREADY_EXISTS {
		// We still got a handle to it; close it since we're not the owner.
		_ = windows.CloseHandle(mutex)
		return false, nil
	}
	// We're the owner. Also create the show-event (auto-reset).
	ename, _ := windows.UTF16PtrFromString(singletonEventName)
	event, err := windows.CreateEvent(nil, 0, 0, ename)
	if err != nil {
		_ = windows.CloseHandle(mutex)
		return false, fmt.Errorf("create event: %w", err)
	}
	singletonState.mutex = mutex
	singletonState.event = event
	singletonState.got = true
	return true, nil
}

// SignalSingletonShow sets the show event so the running first instance brings
// its window to the foreground. Used by a second instance before it exits.
func SignalSingletonShow() error {
	ename, _ := windows.UTF16PtrFromString(singletonEventName)
	ev, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, ename)
	if err != nil {
		return fmt.Errorf("open show event: %w", err)
	}
	defer windows.CloseHandle(ev)
	return windows.SetEvent(ev)
}

// WaitSingletonShow returns a channel that fires each time a second instance
// signals "show yourself". The channel never closes; read it in a loop. The
// caller must be the owner (AcquireSingleton returned owned=true).
func WaitSingletonShow() <-chan struct{} {
	ch := make(chan struct{}, 1)
	go func() {
		for {
			// Wait indefinitely for the auto-reset event.
			status, err := windows.WaitForSingleObject(singletonState.event, windows.INFINITE)
			if err != nil || status == windows.WAIT_FAILED {
				return
			}
			select {
			case ch <- struct{}{}:
			default:
			}
		}
	}()
	return ch
}

// _ unsafe import retained to keep windows.Handle conversions consistent.
var _ = unsafe.Sizeof(uintptr(0))
