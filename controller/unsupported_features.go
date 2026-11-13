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

package controller

import (
	"slices"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/ziti/controller/webapis"
)

const (
	oidcUnsupportedWarning = "OIDC is not supported on this release. Clients use OIDC sessions while the edge-oidc API is bound, " +
		"and routers on this release do not close existing connections when service policy, posture or revocations change " +
		"for those sessions. Remove the edge-oidc binding to keep clients on legacy API sessions, or upgrade to v2"

	haUnsupportedWarning = "HA is not supported on this release. Clustered controllers require OIDC sessions, which this " +
		"release also does not support. Upgrade to v2 to run a controller cluster"
)

// warnUnsupportedFeatures logs a warning for each configured feature this release does not support. Call it
// once the web configuration is loaded.
func (c *Controller) warnUnsupportedFeatures() {
	var bindings []string
	for _, serverConfig := range c.xweb.GetConfig().ServerConfigs {
		for _, api := range serverConfig.APIs {
			bindings = append(bindings, api.Binding())
		}
	}

	for _, warning := range unsupportedFeatureWarnings(bindings, c.IsRaftEnabled()) {
		pfxlog.Logger().Warn(warning)
	}
}

// unsupportedFeatureWarnings returns the warnings for a controller whose web listeners bind the given APIs.
// A clustered controller gets only the HA warning, since its clients need the edge-oidc binding.
func unsupportedFeatureWarnings(bindings []string, clustered bool) []string {
	if clustered {
		return []string{haUnsupportedWarning}
	}
	if slices.Contains(bindings, webapis.OidcApiBinding) {
		return []string{oidcUnsupportedWarning}
	}
	return nil
}
