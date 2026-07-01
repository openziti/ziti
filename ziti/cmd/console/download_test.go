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
	"io"
	"os"
	"path/filepath"
	"testing"
)

// installAssets writes a minimal valid ZAC install (index.html plus an optional version marker) into
// a fresh temp dir and returns its path.
func installAssets(t *testing.T, version string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, zacIndexFile), []byte("<html></html>"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	if version != "" {
		if err := os.WriteFile(filepath.Join(dir, zacVersionFile), []byte(version+"\n"), 0o644); err != nil {
			t.Fatalf("write version: %v", err)
		}
	}
	return dir
}

// TestEnsureAssetsReuse covers every branch that must NOT hit the network: reuse and keep-existing.
// The download/replace path is intentionally not exercised here (it requires GitHub).
func TestEnsureAssetsReuse(t *testing.T) {
	t.Run("matching explicit version reuses", func(t *testing.T) {
		dir := installAssets(t, "4.3.1")
		got, err := EnsureAssets(io.Discard, "4.3.1", dir, false)
		if err != nil || got != "4.3.1" {
			t.Fatalf("got (%q, %v), want (4.3.1, nil)", got, err)
		}
	})

	t.Run("latest reuses whatever is installed", func(t *testing.T) {
		dir := installAssets(t, "4.3.1")
		got, err := EnsureAssets(io.Discard, "latest", dir, false)
		if err != nil || got != "4.3.1" {
			t.Fatalf("got (%q, %v), want (4.3.1, nil)", got, err)
		}
	})

	t.Run("different explicit version without -y keeps existing", func(t *testing.T) {
		dir := installAssets(t, "4.3.1")
		got, err := EnsureAssets(io.Discard, "3.0.0", dir, false)
		if err != nil || got != "4.3.1" {
			t.Fatalf("got (%q, %v), want (4.3.1, nil)", got, err)
		}
		// The install must be untouched (no replace happened).
		if v := installedVersion(dir); v != "4.3.1" {
			t.Fatalf("installedVersion = %q, want 4.3.1 (must not be replaced without -y)", v)
		}
	})

	// Regression for the review finding: valid assets with NO version marker plus an explicit
	// version and no -y must be treated as "different" and kept (not silently reused), rather than
	// short-circuiting the explicit request. It must still not hit the network.
	t.Run("unversioned install with explicit version without -y keeps existing", func(t *testing.T) {
		dir := installAssets(t, "") // no .version marker
		got, err := EnsureAssets(io.Discard, "3.0.0", dir, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "" {
			t.Fatalf("got %q, want empty (unknown installed version)", got)
		}
		if _, statErr := os.Stat(filepath.Join(dir, zacVersionFile)); !os.IsNotExist(statErr) {
			t.Fatal("a version marker was written; no replace should have happened without -y")
		}
	})

	t.Run("non-empty non-install directory errors", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "some-user-file.txt"), []byte("x"), 0o644); err != nil {
			t.Fatalf("seed dir: %v", err)
		}
		if _, err := EnsureAssets(io.Discard, "latest", dir, false); err == nil {
			t.Fatal("expected an error for a non-empty directory that is not a console install")
		}
	})

	t.Run("empty location errors", func(t *testing.T) {
		if _, err := EnsureAssets(io.Discard, "latest", "", false); err == nil {
			t.Fatal("expected an error for an empty location")
		}
	})
}
