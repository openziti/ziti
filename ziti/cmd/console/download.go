/*
	Copyright NetFoundry Inc.

	Licensed under the Apache License, Version 2.0 (the "License");
	you may not use this file except in compliance with the License.
	You may obtain a copy of the License at

	https://www.apache.org/licenses/LICENSE-2.0

	Unless required by applicable law or agreed to in writing, software
	distributed under the License is distributed on an "AS IS" BASIS,
	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
	See the License for the specific language governing permissions and
	limitations under the License.
*/

package console

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	// zacRepo is the GitHub repository that publishes ZAC releases.
	zacRepo = "openziti/ziti-console"
	// zacTagPrefix is the release tag prefix used by the SPA app (distinct from library releases).
	zacTagPrefix = "app-ziti-console-v"
	// zacAssetName is the release asset that holds the built SPA bundle.
	zacAssetName = "ziti-console.zip"
	// zacIndexFile is the SPA entry point used to validate an assets directory.
	zacIndexFile = "index.html"
	// zacChecksumFile records the sha256 of a downloaded archive next to the install.
	zacChecksumFile = ".sha256"
	// zacVersionFile records the installed version next to the install.
	zacVersionFile = ".version"
	// maxEntryBytes caps a single extracted archive entry (512 MiB), guarding against decompression bombs.
	maxEntryBytes = 512 << 20
	// maxReleaseListBytes caps the releases JSON response read into memory (4 MiB).
	maxReleaseListBytes = 4 << 20
)

// downloadURL returns the release asset URL for a concrete (un-prefixed) version.
func downloadURL(version string) string {
	tag := zacTagPrefix + version
	return fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", zacRepo, tag, zacAssetName)
}

// normalizeVersion trims whitespace and a leading "v" from a version string.
func normalizeVersion(version string) string {
	return strings.TrimPrefix(strings.TrimSpace(version), "v")
}

// resolveVersion normalizes v and resolves it to a concrete published version, treating "" or
// "latest" as a request for the newest release.
func resolveVersion(v string) (string, error) {
	v = normalizeVersion(v)
	if v == "" || strings.EqualFold(v, "latest") {
		resolved, err := resolveLatestVersion()
		if err != nil {
			return "", fmt.Errorf("failed to resolve latest ZAC version: %w", err)
		}
		return resolved, nil
	}
	return v, nil
}

// resolveLatestVersion lists releases and returns the highest semver bearing the ZAC app
// tag prefix. The repo also publishes library releases under a different prefix, so we
// filter rather than trusting /releases/latest.
func resolveLatestVersion() (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases?per_page=100", zacRepo)
	resp, err := httpGet(url)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("listing releases failed: %s", resp.Status)
	}

	var releases []ghRelease
	if err = json.NewDecoder(io.LimitReader(resp.Body, maxReleaseListBytes)).Decode(&releases); err != nil {
		return "", fmt.Errorf("failed to parse releases: %w", err)
	}

	var versions []string
	for _, r := range releases {
		if r.Draft || r.Prerelease || !strings.HasPrefix(r.TagName, zacTagPrefix) {
			continue
		}
		versions = append(versions, strings.TrimPrefix(r.TagName, zacTagPrefix))
	}
	if len(versions) == 0 {
		return "", fmt.Errorf("no %s* releases found in %s", zacTagPrefix, zacRepo)
	}
	sort.Slice(versions, func(i, j int) bool { return compareVersions(versions[i], versions[j]) > 0 })
	return versions[0], nil
}

// downloadRelease fetches the given version (must be concrete, not "latest") and installs it
// into destDir. The archive is streamed to a temp file, its sha256 is recorded, it is
// expanded into a staging dir with zip-slip protection, and then atomically swapped into
// destDir. The returned string is the hex sha256 of the downloaded archive.
func downloadRelease(version, destDir string) (string, error) {
	version = normalizeVersion(version)
	if version == "" {
		return "", fmt.Errorf("a concrete version is required")
	}
	if strings.EqualFold(version, "latest") {
		return "", fmt.Errorf("a concrete version is required; resolve \"latest\" first")
	}

	url := downloadURL(version)
	resp, err := httpGet(url)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download of %s failed: %s", url, resp.Status)
	}

	tmp, err := os.CreateTemp("", "ziti-console-*.zip")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	hasher := sha256.New()
	if _, err = io.Copy(io.MultiWriter(tmp, hasher), resp.Body); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("failed to download archive: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return "", fmt.Errorf("failed to flush archive: %w", err)
	}
	sum := hex.EncodeToString(hasher.Sum(nil))

	// Unique sibling staging dir: a fixed name collides between concurrent installs, and a sibling
	// keeps the final rename on the same filesystem.
	staging, err := os.MkdirTemp(filepath.Dir(destDir), filepath.Base(destDir)+".incoming-*")
	if err != nil {
		return "", fmt.Errorf("failed to create staging dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(staging) }()
	if err = unzip(tmpName, staging); err != nil {
		return "", err
	}

	// Record the checksum and version next to the install so it can be pinned/audited later.
	_ = os.WriteFile(filepath.Join(staging, zacChecksumFile), []byte(sum+"\n"), 0o644)
	_ = os.WriteFile(filepath.Join(staging, zacVersionFile), []byte(version+"\n"), 0o644)

	_ = os.RemoveAll(destDir)
	if err = os.Rename(staging, destDir); err != nil {
		return "", fmt.Errorf("failed to finalize install at '%s': %w", destDir, err)
	}
	return sum, nil
}

// installedVersion returns the version recorded by a prior downloadRelease into dir, or ""
// if the marker is absent (dir empty, missing, or not created by this command).
func installedVersion(dir string) string {
	b, err := os.ReadFile(filepath.Join(dir, zacVersionFile))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// validateAssetsDir returns nil if dir exists, is a directory, and holds the SPA index file.
func validateAssetsDir(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("console assets directory '%s' is not accessible: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("console assets path '%s' is not a directory", dir)
	}
	if _, err = os.Stat(filepath.Join(dir, zacIndexFile)); err != nil {
		return fmt.Errorf("console assets directory '%s' has no %s", dir, zacIndexFile)
	}
	return nil
}

// unzip expands src into dest, guarding against path traversal ("zip slip").
func unzip(src, dest string) error {
	reader, err := zip.OpenReader(src)
	if err != nil {
		return fmt.Errorf("failed to open archive: %w", err)
	}
	defer func() { _ = reader.Close() }()

	if err = os.MkdirAll(dest, 0o755); err != nil {
		return fmt.Errorf("failed to create '%s': %w", dest, err)
	}
	cleanDest := filepath.Clean(dest)

	for _, f := range reader.File {
		target := filepath.Join(cleanDest, f.Name)
		rel, relErr := filepath.Rel(cleanDest, target)
		if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("archive entry %q escapes destination", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err = os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err = os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err = writeZipEntry(f, target); err != nil {
			return err
		}
	}
	return nil
}

func writeZipEntry(f *zip.File, target string) error {
	if f.UncompressedSize64 > maxEntryBytes {
		return fmt.Errorf("archive entry %q is too large (%d bytes)", f.Name, f.UncompressedSize64)
	}

	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()

	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	// UncompressedSize64 is attacker-controlled metadata, so the LimitReader enforces the real cap.
	written, err := io.Copy(out, io.LimitReader(rc, maxEntryBytes+1))
	if err != nil {
		return fmt.Errorf("failed to extract %q: %w", f.Name, err)
	}
	if written > maxEntryBytes {
		return fmt.Errorf("archive entry %q exceeded size limit during extraction", f.Name)
	}
	return nil
}

type ghRelease struct {
	TagName    string `json:"tag_name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
}

// compareVersions compares dotted numeric versions, returning >0 if a is newer than b.
// Non-numeric segments sort as 0, as do missing segments.
func compareVersions(a, b string) int {
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	n := len(as)
	if len(bs) > n {
		n = len(bs)
	}
	for i := 0; i < n; i++ {
		av, bv := 0, 0
		if i < len(as) {
			av, _ = strconv.Atoi(as[i])
		}
		if i < len(bs) {
			bv, _ = strconv.Atoi(bs[i])
		}
		if av != bv {
			return av - bv
		}
	}
	return 0
}

func httpGet(url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ziti-cli")
	req.Header.Set("Accept", "application/octet-stream, application/json")
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request to %s failed: %w", url, err)
	}
	return resp, nil
}
