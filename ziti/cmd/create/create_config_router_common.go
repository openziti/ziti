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

package create

import (
	_ "embed"
	"os"

	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/constants"
)

func SetZitiRouterIdentity(r *RouterTemplateValues, routerName string) {
	SetZitiRouterIdentityCert(r, routerName)
	SetZitiRouterIdentityServerCert(r, routerName)
	SetZitiRouterIdentityKey(r, routerName)
	SetZitiRouterIdentityCA(r, routerName)
	SetRouterAltServerCerts(r)

	// Set the router name
	r.Name = routerName

	// Edge router IP override
	edgeRouterIPOverride := os.Getenv(constants.ZitiEdgeRouterIPOverrideVarName)
	if edgeRouterIPOverride != "" {
		r.Edge.IPOverride = edgeRouterIPOverride
	}

	// Set advertised host
	advertisedAddress := os.Getenv(constants.ZitiEdgeRouterAdvertisedAddressVarName)
	resolvedHostname, _ := os.Hostname()
	if advertisedAddress != "" {
		r.Edge.AdvertisedHost = advertisedAddress
	} else {
		// If advertised host is not provided, set to IP override, or default to resolved hostname
		if edgeRouterIPOverride != "" {
			r.Edge.AdvertisedHost = edgeRouterIPOverride
		} else {
			r.Edge.AdvertisedHost = resolvedHostname
		}
	}
}
func SetZitiRouterIdentityCert(r *RouterTemplateValues, routerName string) {
	val := os.Getenv(constants.ZitiRouterIdentityCertVarName)
	if val == "" {
		val = cmdhelper.GetZitiHome() + "/" + routerName + ".cert" // default
	}
	r.IdentityCert = cmdhelper.NormalizePath(val)
}
func SetZitiRouterIdentityServerCert(r *RouterTemplateValues, routerName string) {
	val := os.Getenv(constants.ZitiRouterIdentityServerCertVarName)
	if val == "" {
		val = cmdhelper.GetZitiHome() + "/" + routerName + ".server.chain.cert" // default
	}
	r.IdentityServerCert = cmdhelper.NormalizePath(val)
}
func SetZitiRouterIdentityKey(r *RouterTemplateValues, routerName string) {
	val := os.Getenv(constants.ZitiRouterIdentityKeyVarName)
	if val == "" {
		val = cmdhelper.GetZitiHome() + "/" + routerName + ".key" // default
	}
	r.IdentityKey = cmdhelper.NormalizePath(val)
}
func SetZitiRouterIdentityCA(r *RouterTemplateValues, routerName string) {
	val := os.Getenv(constants.ZitiRouterIdentityCAVarName)
	if val == "" {
		val = cmdhelper.GetZitiHome() + "/" + routerName + ".cas" // default
	}
	r.IdentityCA = cmdhelper.NormalizePath(val)
}

func validateRouterName(name string) string {
	// Currently, only worry about router name if it's blank
	if name == "" {
		hostname, _ := os.Hostname()
		return hostname
	}
	return name
}

func SetRouterAltServerCerts(c *RouterTemplateValues) {
	c.AltCertsEnabled = false
	altServerCert := os.Getenv(constants.PkiAltServerCertVarName)
	if altServerCert == "" {
		return //exit unless both vars are set
	}
	altServerKey := os.Getenv(constants.PkiAltServerKeyVarName)
	if altServerKey == "" {
		return //exit unless both vars are set
	}
	c.AltCertsEnabled = true
	c.AltServerCert = cmdhelper.NormalizePath(altServerCert)
	c.AltServerKey = cmdhelper.NormalizePath(altServerKey)
}
