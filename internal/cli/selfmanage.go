package cli

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const releaseRepo = "umairx25/Envisible"

// httpClient is used for all release downloads with a sane timeout.
var httpClient = &http.Client{Timeout: 60 * time.Second}

// cmdUpdate implements `envis update`. It downloads the latest release binary
// for this OS/arch, verifies its checksum, and atomically replaces the running
// executable in place.
func cmdUpdate(args []string) error {
	if len(args) != 0 {
		return errorf("usage: envis update")
	}

	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locating this executable: %w", err)
	}
	self, err = filepath.EvalSymlinks(self)
	if err != nil {
		return fmt.Errorf("resolving this executable: %w", err)
	}

	infof("Current version: %s", Version)
	infof("Checking for the latest release...")
	tag, err := latestReleaseTag()
	if err != nil {
		return err
	}
	infof("Latest release: %s", tag)

	if Version == tag {
		infof("Already up to date.")
		return nil
	}
	if Version == "dev" {
		infof("Note: this is a dev build; updating to the latest published release.")
	}

	ok, err := confirm(fmt.Sprintf("Update to %s?", tag))
	if err != nil {
		return err
	}
	if !ok {
		infof("Cancelled.")
		return nil
	}

	// Download the matching archive and its checksum.
	archiveName, isZip := assetName()
	base := fmt.Sprintf("https://github.com/%s/releases/download/%s", releaseRepo, tag)

	infof("Downloading %s...", archiveName)
	archive, err := download(base + "/" + archiveName)
	if err != nil {
		return fmt.Errorf("downloading release: %w", err)
	}

	// Verify checksum against SHA256SUMS (best-effort but preferred).
	if sums, err := download(base + "/SHA256SUMS"); err == nil {
		if err := verifyChecksum(archive, archiveName, sums); err != nil {
			return err
		}
		infof("Checksum verified.")
	} else {
		infof("Warning: could not fetch SHA256SUMS; skipping checksum verification.")
	}

	// Extract the envis binary from the archive.
	binName := "envis"
	if runtime.GOOS == "windows" {
		binName = "envis.exe"
	}
	newBin, err := extractBinary(archive, binName, isZip)
	if err != nil {
		return err
	}

	// Replace the running executable atomically: write a sibling temp file,
	// then rename over the original. Rename within the same directory is
	// atomic on Unix. On Windows a running .exe cannot be overwritten, so we
	// move the old one aside first.
	dir := filepath.Dir(self)
	tmp, err := os.CreateTemp(dir, ".envis-update-*")
	if err != nil {
		return fmt.Errorf("creating temp file next to %s: %w", self, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op if the rename succeeded

	if _, err := tmp.Write(newBin); err != nil {
		tmp.Close()
		return fmt.Errorf("writing new binary: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o755); err != nil {
		return err
	}

	if runtime.GOOS == "windows" {
		// Can't overwrite a running exe; move the old one aside, then rename.
		old := self + ".old"
		_ = os.Remove(old)
		if err := os.Rename(self, old); err != nil {
			return fmt.Errorf("moving old binary aside: %w", err)
		}
		if err := os.Rename(tmpName, self); err != nil {
			// Try to restore the original on failure.
			_ = os.Rename(old, self)
			return fmt.Errorf("installing new binary: %w", err)
		}
		_ = os.Remove(old)
	} else {
		if err := os.Rename(tmpName, self); err != nil {
			return fmt.Errorf("installing new binary to %s: %w", self, err)
		}
	}

	infof("Updated envis to %s at %s", tag, self)
	return nil
}

// cmdUninstall implements `envis uninstall`. It removes the installed envis
// binary and offers to remove the shell-hook line from the user's startup
// file. It does NOT touch the local identity or any .envis files.
func cmdUninstall(args []string) error {
	if len(args) != 0 {
		return errorf("usage: envis uninstall")
	}

	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locating this executable: %w", err)
	}
	self, _ = filepath.EvalSymlinks(self)

	infof("This will remove the envis binary at:")
	infof("  %s", self)
	infof("It does NOT delete your identity key or any .envis files.")
	ok, err := confirm("Uninstall envis?")
	if err != nil {
		return err
	}
	if !ok {
		infof("Cancelled.")
		return nil
	}

	// Offer to remove the shell-hook line first (while we can still resolve it).
	if t, found := detectShellTarget(); found {
		if present, _ := hookInstalled(t.rcPath, t.marker); present {
			ok, err := confirm(fmt.Sprintf("Also remove the envis hook line from %s?", t.rcPath))
			if err == nil && ok {
				if err := removeHookLine(t.rcPath, t.marker); err != nil {
					infof("Could not remove the hook line: %v", err)
				} else {
					infof("Removed the hook line from %s.", t.rcPath)
				}
			}
		}
	}

	if err := removeSelf(self); err != nil {
		return fmt.Errorf("removing %s: %w", self, err)
	}
	infof("Removed %s.", self)
	infof("To also delete your local identity, remove the Envis config directory manually.")
	return nil
}

// --- helpers ---------------------------------------------------------------

// assetName returns the release archive filename for the current platform and
// whether it is a zip (Windows) rather than a tar.gz.
func assetName() (name string, isZip bool) {
	if runtime.GOOS == "windows" {
		return fmt.Sprintf("envis-windows-%s.zip", runtime.GOARCH), true
	}
	return fmt.Sprintf("envis-%s-%s.tar.gz", runtime.GOOS, runtime.GOARCH), false
}

// latestReleaseTag queries the GitHub API for the latest release tag.
func latestReleaseTag() (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", releaseRepo)
	resp, err := httpClient.Get(url)
	if err != nil {
		return "", fmt.Errorf("querying latest release: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned %s querying latest release", resp.Status)
	}
	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("decoding release info: %w", err)
	}
	if payload.TagName == "" {
		return "", errorf("could not determine the latest release tag")
	}
	return payload.TagName, nil
}

// download fetches a URL and returns its body, following redirects.
func download(url string) ([]byte, error) {
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s returned %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// verifyChecksum confirms archive's SHA256 matches the entry for archiveName in
// a SHA256SUMS file ("<hex>  <name>" lines).
func verifyChecksum(archive []byte, archiveName string, sums []byte) error {
	sum := sha256.Sum256(archive)
	actual := hex.EncodeToString(sum[:])
	for _, line := range strings.Split(string(sums), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == archiveName {
			if !strings.EqualFold(fields[0], actual) {
				return errorf("checksum mismatch for %s (expected %s, got %s)", archiveName, fields[0], actual)
			}
			return nil
		}
	}
	return errorf("no checksum entry found for %s", archiveName)
}

// extractBinary pulls the named binary out of a tar.gz or zip archive.
func extractBinary(archive []byte, binName string, isZip bool) ([]byte, error) {
	if isZip {
		zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
		if err != nil {
			return nil, fmt.Errorf("reading zip: %w", err)
		}
		for _, f := range zr.File {
			if filepath.Base(f.Name) == binName {
				rc, err := f.Open()
				if err != nil {
					return nil, err
				}
				defer rc.Close()
				return io.ReadAll(rc)
			}
		}
		return nil, errorf("%s not found in archive", binName)
	}

	gzr, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("reading gzip: %w", err)
	}
	defer gzr.Close()
	tr := tar.NewReader(gzr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading tar: %w", err)
		}
		if filepath.Base(hdr.Name) == binName {
			return io.ReadAll(tr)
		}
	}
	return nil, errorf("%s not found in archive", binName)
}

// removeSelf deletes the running binary. On Windows a running exe cannot be
// deleted directly, so it is renamed aside; the OS cleans it up on exit/reboot.
func removeSelf(path string) error {
	if runtime.GOOS == "windows" {
		return os.Rename(path, path+".old")
	}
	return os.Remove(path)
}
