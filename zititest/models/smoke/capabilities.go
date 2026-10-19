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
	"slices"
	"strings"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/foundation/v2/versions"
	"github.com/openziti/ziti/zititest/zitilab"
)

// Capability is a router feature the smoke model or its tests depend on, absent from routers older
// than MinVersion. Setting the variable capability_<Name> to "on" or "off" overrides the version
// check for every router.
type Capability struct {
	Name        string
	MinVersion  string
	Description string
}

// The router capabilities the smoke model and its tests check.
var (
	CapManagedLinkConfig = Capability{
		Name:        "managed_link_config",
		MinVersion:  "v2.1.0-pre2",
		Description: "applies controller-managed link config",
	}

	CapSdkHosting = Capability{
		Name:        "sdk_hosting",
		MinVersion:  "v1.3.0",
		Description: "binds SDK-hosted terminators with the JWT service sessions a 2.x controller issues",
	}

	// The floor is the oldest line the compat test has run with OIDC on, not a confirmed boundary.
	CapOidc = Capability{
		Name:        "oidc",
		MinVersion:  "v1.6.0",
		Description: "accepts OIDC api sessions from a 2.x controller",
	}
)

// Capabilities lists every capability the smoke model checks.
var Capabilities = []Capability{CapManagedLinkConfig, CapSdkHosting, CapOidc}

func (capability Capability) variable() string {
	return "capability_" + capability.Name
}

// Support is the outcome of a capability check. When Supported is false, Reason says why.
type Support struct {
	Supported bool
	Reason    string
}

// RouterSupports reports whether router c has capability. A set capability variable decides for every
// router; otherwise c's version does. A router on the local build (no version) supports everything, and
// one whose version doesn't parse supports nothing. A component that isn't a router supports
// everything.
func RouterSupports(m *model.Model, c *model.Component, capability Capability) Support {
	router, ok := c.Type.(*zitilab.RouterType)
	if !ok {
		return Support{Supported: true}
	}
	override, overridden := m.GetStringVariable(capability.variable())
	return capability.supportedBy(c.Id, router.Version, override, overridden)
}

// RoutersSupport reports whether every router selected by selector has every one of capabilities.
// When one doesn't, Reason lists each gap.
func RoutersSupport(m *model.Model, selector string, capabilities ...Capability) Support {
	var reasons []string
	for _, c := range m.SelectComponents(selector) {
		reasons = append(reasons, gaps(m, c, capabilities)...)
	}
	return combine(reasons)
}

// supportedBy reports whether router id on version has the capability. override is the capability
// variable's value when overridden is true.
func (capability Capability) supportedBy(id, version, override string, overridden bool) Support {
	if overridden {
		if override == "on" {
			return Support{Supported: true}
		}
		return Support{Reason: fmt.Sprintf("%s lacks %s (%s=%s)", id, capability.Name, capability.variable(), override)}
	}

	if version == "" {
		return Support{Supported: true}
	}

	missing := Support{Reason: fmt.Sprintf("%s on %s lacks %s, from %s: %s",
		id, version, capability.Name, capability.MinVersion, capability.Description)}
	v, err := versions.ParseSemVer(version)
	if err != nil {
		return missing
	}
	if v.CompareTo(versions.MustParseSemVer(capability.MinVersion)) < 0 {
		return missing
	}
	return Support{Supported: true}
}

// gaps returns why router c lacks each of capabilities it lacks.
func gaps(m *model.Model, c *model.Component, capabilities []Capability) []string {
	var reasons []string
	for _, capability := range capabilities {
		if support := RouterSupports(m, c, capability); !support.Supported {
			reasons = append(reasons, support.Reason)
		}
	}
	return reasons
}

func combine(reasons []string) Support {
	if len(reasons) > 0 {
		return Support{Reason: strings.Join(reasons, "; ")}
	}
	return Support{Supported: true}
}

// applyCapabilities fits the model's config to what its edge routers support: a router without
// managed link config keeps its link config local, and OIDC defaults off when any router lacks it,
// though an oidc label still decides. It runs after the routers' versions are set.
func applyCapabilities(m *model.Model) error {
	for _, capability := range Capabilities {
		if override, found := m.GetStringVariable(capability.variable()); found && override != "on" && override != "off" {
			return fmt.Errorf("%s must be on or off, not %q", capability.variable(), override)
		}
	}

	oidc := true
	err := m.ForEachComponent(".edge-router", 1, func(c *model.Component) error {
		if support := combine(gaps(m, c, Capabilities)); !support.Supported {
			pfxlog.Logger().Infof("router capabilities: %s", support.Reason)
		}

		if !RouterSupports(m, c, CapManagedLinkConfig).Supported {
			c.Scope.Tags = slices.DeleteFunc(c.Scope.Tags, func(tag string) bool {
				return tag == "ctrl-managed-link"
			})
		}
		if !RouterSupports(m, c, CapOidc).Supported {
			oidc = false
		}
		return nil
	})
	if err != nil {
		return err
	}

	if !oidc {
		m.Scope.Defaults["oidc"] = false
	}
	return nil
}
