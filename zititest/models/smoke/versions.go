/*
	(c) Copyright NetFoundry Inc.

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

package smoke

import (
	"fmt"
	"strings"

	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/foundation/v2/versions"
	"github.com/openziti/ziti/v2/common/version"
	"github.com/openziti/ziti/zititest/zitilab"
)

// unstampedVersion is the version of a local build that ziti-ci didn't stamp.
const unstampedVersion = "v0.0.0"

// applyVersions sets each controller's and edge router's version from the variable
// <component id>_version, with dashes as underscores, such as router_east_1_version. A component
// without one runs the local build. It fails when a controller's version is on a different major.minor
// line than the local build.
func applyVersions(m *model.Model) error {
	return m.ForEachComponent("*", 1, func(c *model.Component) error {
		varName := strings.ReplaceAll(c.Id, "-", "_") + "_version"
		componentVersion, found := m.GetStringVariable(varName)
		if !found {
			return nil
		}

		switch componentType := c.Type.(type) {
		case *zitilab.ControllerType:
			if err := checkControllerLine(c.Id, componentVersion, version.GetVersion()); err != nil {
				return err
			}
			componentType.SetVersion(componentVersion)
		case *zitilab.RouterType:
			componentType.SetVersion(componentVersion)
		}
		return nil
	})
}

// checkControllerLine fails unless controller id on ctrlVersion is on buildVersion's major.minor line,
// the line the smoke tests and config templates target. An unstamped build matches every line.
func checkControllerLine(id, ctrlVersion, buildVersion string) error {
	if buildVersion == unstampedVersion {
		return nil
	}

	ctrlLine, err := releaseLine(ctrlVersion)
	if err != nil {
		return fmt.Errorf("%s version %q: %w", id, ctrlVersion, err)
	}
	buildLine, err := releaseLine(buildVersion)
	if err != nil {
		return fmt.Errorf("local build version %q: %w", buildVersion, err)
	}
	if ctrlLine != buildLine {
		return fmt.Errorf("%s on %s is not on this build's %s line (%s), whose tests and config target "+
			"%s controllers; run it from release-v%s.x", id, ctrlVersion, buildLine, buildVersion, buildLine, ctrlLine)
	}
	return nil
}

// releaseLine returns the major.minor of a semantic version, such as 2.0 for v2.0.8 or v2.0.9-pre1.
func releaseLine(semVer string) (string, error) {
	if _, err := versions.ParseSemVer(semVer); err != nil {
		return "", err
	}
	parts := strings.SplitN(strings.TrimPrefix(semVer, "v"), ".", 3)
	if len(parts) < 2 {
		return "", fmt.Errorf("%q has no minor version", semVer)
	}
	return parts[0] + "." + parts[1], nil
}
