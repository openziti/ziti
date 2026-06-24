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
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// NewConsoleOpsCmd returns the `ziti ops console` command group for managing ZAC assets.
func NewConsoleOpsCmd(out, errOut io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "console",
		Short: "Manage Ziti Admin Console (ZAC) assets",
	}
	cmd.AddCommand(newDownloadCmd(out, errOut))
	cmd.AddCommand(newConfigureCmd(out, errOut))
	return cmd
}

type DownloadOptions struct {
	Out      io.Writer
	Err      io.Writer
	Version  string
	Location string
}

func newDownloadCmd(out, errOut io.Writer) *cobra.Command {
	options := &DownloadOptions{
		Out:     out,
		Err:     errOut,
		Version: "latest",
	}

	cmd := &cobra.Command{
		Use:   "download [version]",
		Short: "Download the Ziti Admin Console and extract it to a directory",
		Long: `Downloads the Ziti Admin Console (ZAC) release archive and extracts it into a directory.
With no version, or "latest", the newest release is downloaded.

The target directory must be empty or not yet exist. This command never removes or overwrites
existing files: if the directory is not empty it fails and leaves it untouched.

Examples:
  # download the latest ZAC into ./console
  ziti ops console download --location ./console

  # download a specific version
  ziti ops console download 4.3.0 --location /opt/openziti/share/console`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				options.Version = args[0]
			}
			return options.Run()
		},
	}

	cmd.Flags().StringVarP(&options.Location, "location", "l", "", "Directory to extract the console into; must be empty or not yet exist (required)")

	return cmd
}

func (o *DownloadOptions) Run() error {
	if o.Location == "" {
		return fmt.Errorf("--location is required")
	}
	abs, err := filepath.Abs(o.Location)
	if err != nil {
		return fmt.Errorf("cannot resolve location '%s' to an absolute path: %w", o.Location, err)
	}
	o.Location = abs
	if err := o.requireEmptyLocation(); err != nil {
		return err
	}

	version, err := resolveVersion(o.Version)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(o.Out, "Downloading ZAC %s\n  source    %s\n  target    %s\n", version, downloadURL(version), o.Location)
	sum, err := downloadRelease(version, o.Location)
	if err != nil {
		return err
	}
	if err = validateAssetsDir(o.Location); err != nil {
		return fmt.Errorf("downloaded archive did not contain a usable console: %w", err)
	}

	_, _ = fmt.Fprintf(o.Out, "\nInstalled ZAC %s\n  location  %s\n  sha256    %s\n", version, o.Location, sum)
	o.printNextSteps()
	return nil
}

// printNextSteps shows how to make a controller serve the assets that were just installed,
// both via `ziti ops console configure` and as a manual config snippet.
func (o *DownloadOptions) printNextSteps() {
	_, _ = fmt.Fprintf(o.Out, `
Next steps
  Configure a controller quickly using this command:
    ziti ops console configure <controller-config.yml> --all --location %s

  Or add this to the listener's apis list in the controller config:
    - binding: spa
      options:
        path: zac
        location: %s
        indexFile: index.html
`, o.Location, o.Location)
}

// requireEmptyLocation fails unless the target directory is empty or does not yet exist, so
// the download never removes or overwrites files the user already placed there.
func (o *DownloadOptions) requireEmptyLocation() error {
	entries, err := os.ReadDir(o.Location)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("cannot inspect location '%s': %w", o.Location, err)
	}
	if len(entries) == 0 {
		return nil
	}
	if existing := installedVersion(o.Location); existing != "" {
		return fmt.Errorf("location '%s' already contains ZAC %s; remove it or choose an empty directory", o.Location, existing)
	}
	return fmt.Errorf("location '%s' is not empty; remove it or choose an empty directory", o.Location)
}
