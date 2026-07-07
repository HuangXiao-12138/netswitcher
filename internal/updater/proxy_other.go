//go:build !windows

package updater

// systemProxyURL is Windows-only. On other platforms net/http's default
// ProxyFromEnvironment behavior is sufficient.
func systemProxyURL() string { return "" }
