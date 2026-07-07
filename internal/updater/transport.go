package updater

import (
	"net/http"
	"net/url"
	"os"
)

// httpClient returns an HTTP client whose proxy comes from the OS proxy
// (Windows Internet Settings) when set, otherwise from HTTP_PROXY env vars.
// This fixes downloads in environments where the proxy is configured via the
// OS rather than env vars — Go's http.DefaultClient ignores the OS proxy.
//
// If HTTP_PROXY/HTTPS_PROXY/NO_PROXY env vars are set they take precedence
// (ProxyFromEnvironment): this lets tests force direct connections via
// NO_PROXY=* instead of inheriting the host's system proxy.
func httpClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	switch {
	case os.Getenv("HTTP_PROXY") != "" || os.Getenv("HTTPS_PROXY") != "" ||
		os.Getenv("NO_PROXY") != "" || os.Getenv("no_proxy") != "":
		transport.Proxy = http.ProxyFromEnvironment
	default:
		if u := systemProxyURL(); u != "" {
			if parsed, err := url.Parse(u); err == nil {
				transport.Proxy = http.ProxyURL(parsed)
			}
		}
	}
	return &http.Client{Transport: transport}
}
