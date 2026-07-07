//go:build windows

package updater

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/netswitcher/netswitcher/pkg/winutil"
)

// batTemplate is the helper batch that swaps the running exe once it exits.
// It is fed the current exe path, the new exe path, and derived dir/base/old
// names via fmt.Sprintf (placeholders %[1]s..%[5]s). Literal percent signs in
// batch variable references are escaped as %% so Sprintf leaves them alone.
//
// Steps:
//  1. wait until the current exe can be renamed (== the process has exited and
//     released the file lock);
//  2. rename it to <name>.old;
//  3. move the new exe into place;
//  4. if the move didn't land, rename .old back (rollback) so the user isn't
//     left with no exe;
//  5. relaunch the exe, then delete the old copy and the batch itself.
const batTemplate = `@echo off
setlocal enabledelayedexpansion
set "LOG=%%TEMP%%\ns-replace.log"
echo [%%date%% %%time%%] === replace start === >> "%%LOG%%"
echo   cur: %[1]s >> "%%LOG%%"
echo   new: %[2]s >> "%%LOG%%"
echo   pid: %[6]d >> "%%LOG%%"
set tries=0
:wait
tasklist /FI "PID eq %[6]d" 2>NUL | find "%[6]d" >NUL
if not errorlevel 1 (
  set /a tries+=1
  if !tries! geq 120 goto abort
  ping -n 2 127.0.0.1 >nul
  goto wait
)
echo [%%time%%] PID gone after !tries! tries >> "%%LOG%%"
ren "%[1]s" "%[5]s" >nul 2>&1
move /y "%[2]s" "%[1]s" >nul
if exist "%[1]s" (
  echo [%%time%%] move OK, cleanup + relaunch >> "%%LOG%%"
  del /f /q "%[3]s\%[5]s" >nul 2>nul
  rmdir /s /q "%[7]s" >nul 2>nul
  start "" "%[1]s"
) else (
  echo [%%time%%] move FAILED, rolling back >> "%%LOG%%"
  ren "%[3]s\%[5]s" "%[4]s" >nul 2>&1
  start "" "%[1]s"
)
echo [%%time%%] === done === >> "%%LOG%%"
del /f /q "%%~f0"
exit /b
:abort
echo [%%time%%] ABORT: PID %[6]d never exited after ~120s >> "%%LOG%%"
del /f /q "%%~f0"
exit /b
`

// ReplaceAndRestart writes the helper batch to a temp path and launches it
// detached, armed to swap newExePath over the currently-running exe.
//
// The caller MUST exit immediately after this returns: the batch only
// completes the rename once the running process releases the exe's file lock.
//
// Launch uses `cmd /c start /b "" <bat>` so the batch becomes an orphan — its
// parent is the transient cmd process, which exits at once. The caller's
// KillChildProcesses (which targets only direct children by ParentProcessID)
// can't reach the orphaned batch, so the swap survives the caller's shutdown.
func ReplaceAndRestart(newExePath string) error {
	curExe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate current exe: %w", err)
	}
	dir := filepath.Dir(curExe)
	base := filepath.Base(curExe)
	old := base + ".old"
	pid := os.Getpid()
	tmpDir := filepath.Dir(newExePath)

	bat := fmt.Sprintf(batTemplate, curExe, newExePath, dir, base, old, pid, tmpDir)
	batPath := filepath.Join(os.TempDir(), "ns-replace.bat")
	if err := os.WriteFile(batPath, []byte(bat), 0o644); err != nil {
		return fmt.Errorf("write helper batch: %w", err)
	}

	// `start /b ""` — the empty title is required because start otherwise
	// treats the next quoted arg as a title; /b means no new window.
	cmd := exec.Command("cmd.exe", "/c", "start", "/b", "", batPath)
	winutil.HideWindow(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start helper: %w", err)
	}
	// Deliberately don't Wait: cmd returns almost instantly after `start /b`,
	// and we want the caller to proceed to its own shutdown so the batch can
	// take over. Releasing cmd here is fine — the batch is already orphaned.
	_ = cmd.Process.Release()
	return nil
}
