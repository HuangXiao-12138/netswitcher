//go:build windows

package winutil

import (
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// jobHandle is a package-level root, so it stays open for the process
// lifetime. The OS closes it on exit, firing JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
// so all child processes (msedgewebview2.exe, route.exe, …) die with the
// parent instead of becoming orphans after a crash or taskkill.
var jobHandle windows.Handle

// AssignSelfToKillOnCloseJob puts the current process in a new Job Object.
//
// NOTE: we intentionally do NOT set JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE. The
// elevated-replacement instance inherits this job via ShellExecute runas
// (confirmed: inJob=true on the new pid), and KILL_ON_JOB_CLOSE would kill it
// the moment this instance exits — silently sinking the takeover. Child
// cleanup moves to KillChildProcesses(), called explicitly from OnShutdown.
//
// Caveat: if this process crashes before OnShutdown runs, children are
// orphaned (the old KILL_ON_JOB_CLOSE would have caught that). Acceptable
// trade-off — crashes are rare, and the alternative (relaunch never works) is
// worse.
func AssignSelfToKillOnCloseJob() error {
	h, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return err
	}
	if err := windows.AssignProcessToJobObject(h, windows.CurrentProcess()); err != nil {
		return err
	}
	jobHandle = h
	return nil
}

// KillChildProcesses terminates all direct children of the current process
// (msedgewebview2.exe, route.exe, …). Replaces JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
// (which we can't use — see AssignSelfToKillOnCloseJob). Called from OnShutdown.
func KillChildProcesses() {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return
	}
	defer windows.CloseHandle(snap)
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	myPid := windows.GetCurrentProcessId()
	for err = windows.Process32First(snap, &pe); err == nil; err = windows.Process32Next(snap, &pe) {
		if pe.ProcessID == 0 || pe.ProcessID == myPid || pe.ParentProcessID != myPid {
			continue
		}
		name := strings.ToLower(windows.UTF16ToString(pe.ExeFile[:]))
		if name == "netswitcher.exe" {
			// The elevated replacement launched via ShellExecute runas is our
			// child by PID, but must survive us — don't kill it.
			continue
		}
		if h, e := windows.OpenProcess(windows.PROCESS_TERMINATE, false, pe.ProcessID); e == nil {
			_ = windows.TerminateProcess(h, 1)
			_ = windows.CloseHandle(h)
		}
	}
}

var procIsProcessInJob = windows.NewLazySystemDLL("kernel32.dll").NewProc("IsProcessInJob")

// InJob reports whether the current process is a member of any job. The
// elevated-replacement instance logs this on startup to detect whether it
// inherited the previous (non-elevated) instance's KILL_ON_JOB_CLOSE job —
// if so, the previous instance exiting kills this one too, silently sinking
// the takeover.
func InJob() bool {
	var inJob int32
	ret, _, _ := procIsProcessInJob.Call(
		uintptr(windows.CurrentProcess()),
		0,
		uintptr(unsafe.Pointer(&inJob)),
	)
	if ret == 0 {
		return false // call failed — treat as "not in job"
	}
	return inJob != 0
}
