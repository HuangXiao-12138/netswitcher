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
//   3. If owned=true, the GUI owns the lock. WaitSingletonShow() returns a
//      channel that fires each time a second instance signals.
//
// We use a direct CreateMutexW syscall (not windows.CreateMutex) because the
// ERROR_ALREADY_EXISTS status is only reliable if captured atomically with
// the syscall — the Go runtime clobbers GetLastError between calls. The
// Local\ namespace is session-scoped (one instance per login session) and
// needs no special privilege.

const (
	singletonMutexName = `Local\NetSwitcher-Single-Instance`
	singletonEventName = `Local\NetSwitcher-Show-Event`
)

var procCreateMutexW = windows.NewLazySystemDLL("kernel32.dll").NewProc("CreateMutexW")

var (
	singletonMu    sync.Mutex
	singletonState struct {
		mutex windows.Handle
		event windows.Handle
		got   bool
	}
)

// createMutex calls CreateMutexW directly and returns the captured last-error
// atomically so ERROR_ALREADY_EXISTS is reliable.
func createMutex(name string) (handle windows.Handle, alreadyExists bool, err error) {
	namePtr, _ := windows.UTF16PtrFromString(name)
	r0, _, e1 := procCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(namePtr)))
	if r0 == 0 {
		return 0, false, fmt.Errorf("CreateMutexW failed: %w", e1)
	}
	return windows.Handle(r0), e1 == windows.ERROR_ALREADY_EXISTS, nil
}

// AcquireSingleton tries to grab the singleton mutex. Returns owned=true if
// this is the first instance (caller proceeds to run the GUI); owned=false
// if another instance already holds it (caller should signal + exit).
func AcquireSingleton() (owned bool, err error) {
	singletonMu.Lock()
	defer singletonMu.Unlock()
	if singletonState.got {
		return true, nil // already acquired in this process
	}
	mutex, alreadyExists, err := createMutex(singletonMutexName)
	if err != nil {
		return false, fmt.Errorf("create mutex: %w", err)
	}
	if alreadyExists {
		// Another instance owns the mutex; we just got a handle to it.
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
