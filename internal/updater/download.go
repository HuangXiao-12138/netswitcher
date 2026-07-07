package updater

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// DownloadAndExtract fetches the portable zip at zipURL and extracts the
// NetSwitcher exe it contains into destDir, returning the extracted exe's
// path. The archive is downloaded fully into memory (the release is ~20 MB,
// capped at 100 MB as a sanity bound) so we can validate the zip before
// touching disk.
//
// Failures reuse FetchError's ErrorKind so the UI shows the same kind of
// friendly message as CheckUpdate.
func DownloadAndExtract(ctx context.Context, zipURL, destDir string, onProgress ProgressFunc) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, zipURL, nil)
	if err != nil {
		return "", &FetchError{Kind: ErrNetwork, Msg: "build request: " + err.Error()}
	}
	req.Header.Set("User-Agent", "NetSwitcher")
	req.Header.Set("Accept", "application/octet-stream")

	resp, err := httpClient().Do(req)
	if err != nil {
		return "", &FetchError{Kind: ErrNetwork, Msg: err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", &FetchError{Kind: ErrHTTP, Msg: fmt.Sprintf("download returned %d", resp.StatusCode)}
	}

	pr := &progressReader{
		r:      io.LimitReader(resp.Body, 100<<20),
		total:  resp.ContentLength,
		onProg: onProgress,
	}
	body, err := io.ReadAll(pr)
	if err != nil {
		return "", &FetchError{Kind: ErrNetwork, Msg: "read body: " + err.Error()}
	}

	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return "", &FetchError{Kind: ErrParse, Msg: "zip parse: " + err.Error()}
	}

	// Find the exe inside the zip. The portable zip ships NetSwitcher.exe at
	// its root; be tolerant of a single nested directory just in case.
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if strings.ToLower(filepath.Base(f.Name)) != "netswitcher.exe" {
			continue
		}
		dest := filepath.Join(destDir, "NetSwitcher-new.exe")
		if err := extractZipFile(f, dest); err != nil {
			return "", fmt.Errorf("extract exe: %w", err)
		}
		return dest, nil
	}
	return "", &FetchError{Kind: ErrParse, Msg: "zip contains no NetSwitcher.exe"}
}

// extractZipFile writes a single zip entry to dest, truncating any existing
// file.
func extractZipFile(zf *zip.File, dest string) error {
	rc, err := zf.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, rc)
	return err
}

// ProgressFunc reports cumulative bytes downloaded and the total (or -1 if the
// server didn't send Content-Length). Called periodically — not on every Read
// — to avoid flooding the event channel.
type ProgressFunc func(downloaded, total int64)

// progressReader wraps a reader and reports cumulative progress every ~200 KB.
type progressReader struct {
	r       io.Reader
	total   int64
	read    int64
	lastRep int64
	onProg  ProgressFunc
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.r.Read(p)
	if n > 0 {
		pr.read += int64(n)
		if pr.onProg != nil && pr.read-pr.lastRep >= 200<<10 {
			pr.lastRep = pr.read
			pr.onProg(pr.read, pr.total)
		}
	}
	return n, err
}
