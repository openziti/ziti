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

package install

import (
	"github.com/openziti/ziti/common/version"
	c "github.com/openziti/ziti/ziti/constants"
)

// UpgradeZitiTunnelOptions the options for the upgrade ziti-tunnel command
type UpgradeZitiTunnelOptions struct {
	InstallOptions

	Version string
}

// Run implements the command
func (o *UpgradeZitiTunnelOptions) Run() error {
	newVersion, err := o.getLatestZitiAppVersion(version.GetBranch(), c.ZITI_TUNNEL)
	if err != nil {
		return err
	}

	newVersionStr := newVersion.String()

	if o.Version != "" {
		newVersionStr = o.Version
	}

	o.deleteInstalledBinary(c.ZITI_TUNNEL)

	return o.installZitiApp(version.GetBranch(), c.ZITI_TUNNEL, true, newVersionStr)
}
