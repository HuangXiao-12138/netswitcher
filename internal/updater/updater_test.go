package updater

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestIsReleaseBuild(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"v1.2.3", true},
		{"1.2.3", true}, // bare — canonical adds "v"
		{"v0.1.0", true},
		{"v1.2.3-rc1", false},
		{"v1.2.3-dirty", false},
		{"v1.2.3-3-gabcdef", false}, // git describe ahead of tag
		{"dev", false},
		{"master-42", false},
		{"", false},
		{"abc1234", false}, // bare commit hash
	}
	for _, tt := range tests {
		if got := IsReleaseBuild(tt.in); got != tt.want {
			t.Errorf("IsReleaseBuild(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestHasNewer(t *testing.T) {
	tests := []struct {
		name    string
		current string
		latest  string
		want    bool
	}{
		{"patch behind", "v1.0.0", "v1.0.1", true},
		{"minor behind", "v1.0.0", "v1.2.0", true},
		{"major behind", "v1.9.0", "v2.0.0", true},
		{"equal", "v1.2.0", "v1.2.0", false},
		{"current ahead", "v1.3.0", "v1.2.0", false},
		{"bare current canonicalized", "1.2.0", "v1.3.0", true},
		{"prerelease latest still counts as newer", "v1.2.0", "v2.0.0-rc1", true},
		{"dev current never newer", "dev", "v1.0.0", false},
		{"invalid latest", "v1.0.0", "oops", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasNewer(tt.current, tt.latest); got != tt.want {
				t.Errorf("HasNewer(%q, %q) = %v, want %v", tt.current, tt.latest, got, tt.want)
			}
		})
	}
}

func TestFetchLatest_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("request missing User-Agent header")
		}
		_, _ = w.Write([]byte(`{
			"tag_name": "v1.2.3",
			"html_url": "https://example.com/release",
			"body": "release notes",
			"published_at": "2026-01-02T03:04:05Z",
			"assets": [
				{"name": "NetSwitcher-v1.2.3-x86_64-Portable.zip", "browser_download_url": "https://example.com/zip"},
				{"name": "checksum.txt", "browser_download_url": "https://example.com/sum"}
			]
		}`))
	}))
	defer srv.Close()

	rel, err := FetchLatest(context.Background(), srv.URL+"/releases/latest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rel.TagName != "v1.2.3" {
		t.Errorf("TagName = %q, want v1.2.3", rel.TagName)
	}
	if rel.HTMLURL != "https://example.com/release" {
		t.Errorf("HTMLURL = %q", rel.HTMLURL)
	}
	if rel.ZipURL != "https://example.com/zip" {
		t.Errorf("ZipURL = %q, want the portable zip asset", rel.ZipURL)
	}
	if rel.PublishedAt.IsZero() {
		t.Error("PublishedAt not parsed")
	}
}

func TestFetchLatest_ClassifiesErrors(t *testing.T) {
	cases := []struct {
		name    string
		handler http.HandlerFunc
		want    ErrorKind
	}{
		{"not found", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) }, ErrNotFound},
		{"http error", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) }, ErrHTTP},
		{"bad json", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("not json")) }, ErrParse},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()
			_, err := FetchLatest(context.Background(), srv.URL+"/releases/latest")
			fe, ok := err.(*FetchError)
			if !ok {
				t.Fatalf("want *FetchError, got %T (%v)", err, err)
			}
			if fe.Kind != tc.want {
				t.Errorf("Kind = %q, want %q", fe.Kind, tc.want)
			}
		})
	}
}

func TestFetchLatest_NetworkError(t *testing.T) {
	// Port 1 is unopened on loopback → connection refused → ErrNetwork. Bound
	// with a short ctx so a firewall DROP can't hang the test.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := FetchLatest(ctx, "http://127.0.0.1:1/releases/latest")
	fe, ok := err.(*FetchError)
	if !ok {
		t.Fatalf("want *FetchError, got %T (%v)", err, err)
	}
	if fe.Kind != ErrNetwork {
		t.Errorf("Kind = %q, want network", fe.Kind)
	}
}

// makeZip builds an in-memory zip from the given file map (name → content).
func makeZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func requireFetchError(t *testing.T, err error, want ErrorKind) {
	t.Helper()
	fe, ok := err.(*FetchError)
	if !ok {
		t.Fatalf("want *FetchError, got %T (%v)", err, err)
	}
	if fe.Kind != want {
		t.Errorf("Kind = %q, want %q", fe.Kind, want)
	}
}

func TestDownloadAndExtract(t *testing.T) {
	zipBytes := makeZip(t, map[string]string{
		"NetSwitcher.exe": "fake-binary-content",
		"readme.txt":      "ignore me",
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(zipBytes)
	}))
	defer srv.Close()

	dir := t.TempDir()
	exe, err := DownloadAndExtract(context.Background(), srv.URL, dir, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if filepath.Base(exe) != "NetSwitcher-new.exe" {
		t.Errorf("exe name = %q, want NetSwitcher-new.exe", filepath.Base(exe))
	}
	got, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "fake-binary-content" {
		t.Errorf("exe content = %q", got)
	}
}

func TestDownloadAndExtract_Errors(t *testing.T) {
	t.Run("http error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) }))
		defer srv.Close()
		_, err := DownloadAndExtract(context.Background(), srv.URL, t.TempDir(), nil)
		requireFetchError(t, err, ErrHTTP)
	})
	t.Run("bad zip", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("not a zip")) }))
		defer srv.Close()
		_, err := DownloadAndExtract(context.Background(), srv.URL, t.TempDir(), nil)
		requireFetchError(t, err, ErrParse)
	})
	t.Run("no exe in zip", func(t *testing.T) {
		zipBytes := makeZip(t, map[string]string{"readme.txt": "no exe here"})
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(zipBytes) }))
		defer srv.Close()
		_, err := DownloadAndExtract(context.Background(), srv.URL, t.TempDir(), nil)
		requireFetchError(t, err, ErrParse)
	})
}

func TestDownloadAndExtract_Progress(t *testing.T) {
	// Random bytes are incompressible → the zip stays ~content-sized, enough to
	// trip the 200 KB report threshold multiple times.
	content := make([]byte, 500<<10)
	if _, err := rand.Read(content); err != nil {
		t.Fatal(err)
	}
	zipBytes := makeZip(t, map[string]string{"NetSwitcher.exe": string(content)})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(zipBytes)))
		_, _ = w.Write(zipBytes)
	}))
	defer srv.Close()

	var lastDown, lastTotal int64
	var calls int
	_, err := DownloadAndExtract(context.Background(), srv.URL, t.TempDir(), func(d, total int64) {
		lastDown = d
		lastTotal = total
		calls++
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls == 0 {
		t.Error("progress callback was never called")
	}
	if lastTotal != int64(len(zipBytes)) {
		t.Errorf("total = %d, want %d", lastTotal, len(zipBytes))
	}
	if lastDown > lastTotal {
		t.Errorf("downloaded (%d) > total (%d)", lastDown, lastTotal)
	}
}
