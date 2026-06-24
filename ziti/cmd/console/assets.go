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
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// resolveAssets returns a local directory holding the ZAC build to serve. A --location
// directory is used verbatim, otherwise the requested --version is downloaded (and cached).
func (o *ConsoleOptions) resolveAssets() (string, error) {
	if o.Location != "" {
		if err := validateAssetsDir(o.Location); err != nil {
			return "", err
		}
		return o.Location, nil
	}
	if o.Version == "" {
		return "", fmt.Errorf("no console assets specified: pass --location <dir> or --version <x.y.z|latest>")
	}
	return o.downloadAssets()
}

func (o *ConsoleOptions) downloadAssets() (string, error) {
	version, err := resolveVersion(o.Version)
	if err != nil {
		return "", err
	}

	cacheRoot, err := o.cacheDir()
	if err != nil {
		return "", err
	}
	dest := filepath.Join(cacheRoot, version)

	if validateAssetsDir(dest) == nil {
		o.logger().Infof("using cached ZAC %s from %s", version, dest)
		return dest, nil
	}

	if !o.confirmDownload(version, downloadURL(version)) {
		return "", fmt.Errorf("download declined; re-run with --yes or supply --location")
	}

	if _, err = downloadRelease(version, dest); err != nil {
		return "", err
	}
	if err = validateAssetsDir(dest); err != nil {
		return "", fmt.Errorf("downloaded archive did not contain a usable console: %w", err)
	}
	o.logger().Infof("installed ZAC %s to %s", version, dest)
	return dest, nil
}

func (o *ConsoleOptions) confirmDownload(version, url string) bool {
	if o.Yes {
		return true
	}
	_, _ = fmt.Fprintf(o.Out, "Download ZAC %s from %s? [y/N]: ", version, url)
	reader := bufio.NewReader(o.In)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes"
}

func (o *ConsoleOptions) cacheDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("failed to determine cache directory: %w", err)
	}
	dir := filepath.Join(base, "ziti", "console")
	if err = os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create cache directory '%s': %w", dir, err)
	}
	return dir, nil
}
