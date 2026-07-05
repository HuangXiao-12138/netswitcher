//go:build windows

package winutil

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// jobHandle is a package-level root, so it stays open for the process
// lifetime. The OS closes it on exit, firing JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
// so all child processes (msedgewebview2.exe, route.exe, …) die with the
// parent instead of becoming orphans after a crash or taskkill.
var jobHandle windows.Handle

// AssignSelfToKillOnCloseJob puts the current process in a new Job Object
// flagged KILL_ON_JOB_CLOSE. Any child process this process later spawns
// inherits the job; when this process dies, the job's last open handle is
// released by the OS and every process in the job is killed.
//
// Call once, early, from the GUI process (before wails.Run spawns webview2).
func AssignSelfToKillOnCloseJob() error {
	h, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	if _, err := windows.SetInformationJobObject(
		h,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		return err
	}
	if err := windows.AssignProcessToJobObject(h, windows.CurrentProcess()); err != nil {
		return err
	}
	jobHandle = h
	return nil
}
