//go:build windows

package updater

import (
	"strings"

	"golang.org/x/sys/windows/registry"
)

// systemProxyURL reads the Windows WinINET proxy (the one configured under
// "Internet Settings" and used by Edge/Chrome). Returns "" when disabled.
//
// Go's net/http ignores this entirely — http.DefaultTransport.Proxy defaults
// to ProxyFromEnvironment, which only reads HTTP_PROXY/HTTPS_PROXY env vars.
// On a machine whose proxy is set via Windows settings (common in regions
// that need a proxy to reach GitHub release downloads at usable speed), Go
// would otherwise try a direct connection and time out on large downloads
// even though the check-update JSON (a few KB) succeeds fast.
func systemProxyURL() string {
	k, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Internet Settings`,
		registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()

	enable, _, err := k.GetIntegerValue("ProxyEnable")
	if err != nil || enable == 0 {
		return ""
	}
	server, _, err := k.GetStringValue("ProxyServer")
	if err != nil || strings.TrimSpace(server) == "" {
		return ""
	}

	// ProxyServer is either "host:port" (all protocols) or
	// "http=h:p;https=h:p;ftp=h:p" (per-protocol). Prefer https, then fall
	// back to the first entry.
	addr := server
	if strings.Contains(server, "=") {
		addr = ""
		for _, part := range strings.Split(server, ";") {
			if strings.HasPrefix(part, "https=") {
				addr = strings.TrimPrefix(part, "https=")
				break
			}
		}
		if addr == "" {
			first := strings.Split(server, ";")[0]
			if i := strings.Index(first, "="); i >= 0 {
				addr = first[i+1:]
			}
		}
	}
	if addr == "" {
		return ""
	}
	if !strings.HasPrefix(addr, "http://") && !strings.HasPrefix(addr, "https://") {
		addr = "http://" + addr
	}
	return addr
}
