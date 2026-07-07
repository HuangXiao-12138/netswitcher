// Package updater checks GitHub Releases for a newer NetSwitcher build and
// (in a later phase) will download + replace the running binary.
//
// Version comparison uses golang.org/x/mod/semver, which requires a leading
// "v". Build scripts inject the version via -ldflags; CI uses the git tag
// directly (e.g. "v1.2.0"), local builds use `git describe --tags --dirty`
// (e.g. "v1.2.0-3-gabcdef" or "v1.2.0-dirty"). Anything that isn't a clean
// "vX.Y.Z" is treated as a dev build — see IsReleaseBuild.
package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

// RepoAPI is the GitHub API endpoint for the latest release. Bound to the
// project's ACTUAL GitHub repo (HuangXiao-12138/netswitcher) — note the Go
// module path in go.mod (github.com/netswitcher/netswitcher) is just an import
// name and does NOT match the real repo URL.
const RepoAPI = "https://api.github.com/repos/HuangXiao-12138/netswitcher/releases/latest"

// ErrorKind categorizes a fetch failure so the UI can show a friendly,
// localized message instead of a raw Go error string.
type ErrorKind string

const (
	ErrNetwork  ErrorKind = "network"  // connection refused / DNS / timeout
	ErrNotFound ErrorKind = "notfound" // HTTP 404 — no releases, or wrong repo
	ErrHTTP     ErrorKind = "http"     // any other non-200 status
	ErrParse    ErrorKind = "parse"    // response body wasn't valid JSON
	ErrUnknown  ErrorKind = "unknown"  // fallback when the error isn't a *FetchError
)

// FetchError carries an ErrorKind alongside a short diagnostic message.
type FetchError struct {
	Kind ErrorKind
	Msg  string
}

func (e *FetchError) Error() string { return string(e.Kind) + ": " + e.Msg }

// Release describes the relevant fields of a GitHub release.
type Release struct {
	TagName     string    // release tag, e.g. "v1.2.0"
	HTMLURL     string    // release page (open in browser for manual download)
	Body        string    // release notes (markdown)
	PublishedAt time.Time // when the release was published
	ZipURL      string    // browser_download_url of the portable zip asset
}

// FetchLatest queries the given GitHub releases/latest endpoint. GitHub
// rejects requests without a User-Agent header, so one is set; the JSON media
// type is requested via Accept so the API returns the canonical payload.
// Failures are returned as *FetchError so callers can branch on ErrorKind.
func FetchLatest(ctx context.Context, endpoint string) (*Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, &FetchError{Kind: ErrNetwork, Msg: "build request: " + err.Error()}
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "NetSwitcher")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// ctx cancel / deadline surfaces here too — classify as network so the
		// UI shows "无法连接" rather than a raw deadline-exceeded string.
		return nil, &FetchError{Kind: ErrNetwork, Msg: err.Error()}
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, &FetchError{Kind: ErrNotFound, Msg: "github api 404 (no releases yet, or wrong repo)"}
	case resp.StatusCode != http.StatusOK:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, &FetchError{Kind: ErrHTTP, Msg: fmt.Sprintf("github api %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))}
	}

	var raw struct {
		TagName     string    `json:"tag_name"`
		HTMLURL     string    `json:"html_url"`
		Body        string    `json:"body"`
		PublishedAt time.Time `json:"published_at"`
		Assets      []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, &FetchError{Kind: ErrParse, Msg: err.Error()}
	}

	rel := &Release{
		TagName:     raw.TagName,
		HTMLURL:     raw.HTMLURL,
		Body:        raw.Body,
		PublishedAt: raw.PublishedAt,
	}
	// Pick the portable zip asset (CI names it
	// NetSwitcher-<ver>-x86_64-Portable.zip). Left empty if absent — callers
	// can still fall back to HTMLURL.
	for _, asset := range raw.Assets {
		name := strings.ToLower(asset.Name)
		if strings.Contains(name, "portable") && strings.HasSuffix(name, ".zip") {
			rel.ZipURL = asset.BrowserDownloadURL
			break
		}
	}
	return rel, nil
}

// canonical ensures v has a leading "v" — semver requires it, but older local
// build paths produced a bare "0.1.0".
func canonical(v string) string {
	if v == "" {
		return ""
	}
	if strings.HasPrefix(v, "v") {
		return v
	}
	return "v" + v
}

// IsReleaseBuild reports whether v is a clean semver release tag (vX.Y.Z with
// no pre-release suffix). "v1.2.3" → true; "v1.2.3-rc1", "v1.2.3-dirty",
// "v1.2.3-3-gabcdef", "dev", or a bare hash → false.
func IsReleaseBuild(v string) bool {
	c := canonical(v)
	return semver.IsValid(c) && semver.Prerelease(c) == ""
}

// HasNewer reports whether latest is strictly newer than current. Returns
// false if either side isn't valid semver — callers should gate the "current"
// side on IsReleaseBuild first.
func HasNewer(current, latest string) bool {
	c, l := canonical(current), canonical(latest)
	if !semver.IsValid(c) || !semver.IsValid(l) {
		return false
	}
	return semver.Compare(l, c) > 0
}
