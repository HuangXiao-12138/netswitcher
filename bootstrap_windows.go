//go:build windows

package gui

import (
	_ "embed"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows/registry"

	"github.com/netswitcher/netswitcher/pkg/winutil"
)

//go:embed build/windows/MicrosoftEdgeWebview2Setup.exe
var webview2Bootstrapper []byte

const webview2RegPath = `SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`

// EnsureWebView2 checks whether the WebView2 runtime is installed; if not,
// extracts the bundled Microsoft bootstrapper and installs it silently.
// Called before wails.Run so the GUI can render.
func EnsureWebView2(elevated bool) error {
	if isWebView2Installed() {
		return nil
	}
	slog.Info("WebView2 runtime not found, installing from bundled bootstrapper")

	// Extract bootstrapper to a temp file.
	tmpPath := filepath.Join(os.TempDir(), "ns-webview2-setup.exe")
	if err := os.WriteFile(tmpPath, webview2Bootstrapper, 0o755); err != nil {
		return fmt.Errorf("extract webview2 bootstrapper: %w", err)
	}
	defer os.Remove(tmpPath)

	if elevated {
		// Already admin — run directly, no UAC.
		cmd := exec.Command(tmpPath, "/silent", "/install")
		if out, err := cmd.CombinedOutput(); err != nil {
			slog.Warn("webview2 bootstrapper returned error", "err", err, "output", string(out))
		}
	} else {
		// Not elevated — ShellExecute runas triggers a UAC prompt for the
		// bootstrapper (which needs admin to install system-wide).
		if err := winutil.RunElevated(tmpPath, "/silent /install"); err != nil {
			slog.Warn("webview2 bootstrapper elevation failed", "err", err)
		}
	}

	// Poll the registry for up to 90s (the bootstrapper downloads + installs
	// the actual runtime, which can take a while on slow connections).
	for i := 0; i < 45; i++ {
		time.Sleep(2 * time.Second)
		if isWebView2Installed() {
			slog.Info("WebView2 runtime installed successfully")
			return nil
		}
	}
	return fmt.Errorf("WebView2 installation timed out after 90s")
}

func isWebView2Installed() bool {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, webview2RegPath, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	pv, _, err := k.GetStringValue("pv")
	if err != nil {
		return false
	}
	return pv != "" && pv != "0.0.0.0"
}
