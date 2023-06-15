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

package cmd

import (
	_ "embed"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/constants"
	"os"
)

func SetZitiRouterIdentity(r *RouterTemplateValues, routerName string) {
	SetZitiRouterIdentityCert(r, routerName)
	SetZitiRouterIdentityServerCert(r, routerName)
	SetZitiRouterIdentityKey(r, routerName)
	SetZitiRouterIdentityCA(r, routerName)

	// Set the router name
	r.Name = routerName

	// Edge router IP override
	edgeRouterIPOverride := os.Getenv(constants.ZitiEdgeRouterIPOverrideVarName)
	if edgeRouterIPOverride != "" {
		r.Edge.IPOverride = edgeRouterIPOverride
	}

	// Set advertised host
	advertisedHost := os.Getenv(constants.ZitiEdgeRouterAdvertisedHostVarName)
	resolvedHostname, _ := os.Hostname()
	if advertisedHost != "" {
		r.Edge.AdvertisedHost = advertisedHost
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
		val = workingDir + "/" + routerName + ".cert" // default
	}
	r.IdentityCert = cmdhelper.NormalizePath(val)
}
func SetZitiRouterIdentityServerCert(r *RouterTemplateValues, routerName string) {
	val := os.Getenv(constants.ZitiRouterIdentityServerCertVarName)
	if val == "" {
		val = workingDir + "/" + routerName + ".server.chain.cert" // default
	}
	r.IdentityServerCert = cmdhelper.NormalizePath(val)
}
func SetZitiRouterIdentityKey(r *RouterTemplateValues, routerName string) {
	val := os.Getenv(constants.ZitiRouterIdentityKeyVarName)
	if val == "" {
		val = workingDir + "/" + routerName + ".key" // default
	}
	r.IdentityKey = cmdhelper.NormalizePath(val)
}
func SetZitiRouterIdentityCA(r *RouterTemplateValues, routerName string) {
	val := os.Getenv(constants.ZitiRouterIdentityCAVarName)
	if val == "" {
		val = workingDir + "/" + routerName + ".cas" // default
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
