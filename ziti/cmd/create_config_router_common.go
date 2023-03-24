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
	"log"
	"os"
)

func SetZitiRouterIdentity(r *RouterTemplateValues, routerName string) {
	SetZitiRouterIdentityCert(r, routerName)
	SetZitiRouterIdentityServerCert(r, routerName)
	SetZitiRouterIdentityKey(r, routerName)
	SetZitiRouterIdentityCA(r, routerName)

	// Edge router IP override
	edgeRouterIPOverride := os.Getenv(constants.ZitiEdgeRouterIPOverrideVarName)
	if edgeRouterIPOverride != "" {
		r.Edge.IPOverride = edgeRouterIPOverride
	}

	externalDNS := os.Getenv(constants.ExternalDNSVarName)
	edgeRouterRawName := os.Getenv(constants.ZitiEdgeRouterNameVarName)
	if externalDNS != "" {
		r.Edge.Hostname = externalDNS
		r.Edge.AdvertisedHost = r.Edge.Hostname //not redundant set AdvertisedHost
	} else if edgeRouterRawName != "" {
		r.Edge.Hostname = edgeRouterRawName
		r.Edge.AdvertisedHost = r.Edge.Hostname //not redundant set AdvertisedHost
	} else {
		r.Edge.Hostname, _ = os.Hostname()
		r.Edge.AdvertisedHost = r.Edge.Hostname //not redundant set AdvertisedHost
	}

	advertisedHost := os.Getenv(constants.ZitiEdgeRouterAdvertisedHostVarName)
	if advertisedHost != "" {
		if advertisedHost != edgeRouterIPOverride && advertisedHost != r.Edge.AdvertisedHost {
			log.Panicf("if %s[%s] is supplied, it *MUST* match the %s[%s] or resolved hostname[%s]", constants.ZitiEdgeRouterAdvertisedHostVarName, advertisedHost, constants.ZitiEdgeRouterIPOverrideVarName, edgeRouterIPOverride, r.Edge.Hostname)
		}
		r.Edge.AdvertisedHost = advertisedHost //finally override AdvertisedHost if provided
	} else {
		if externalDNS != "" || edgeRouterRawName != "" {
			r.Edge.AdvertisedHost = r.Edge.Hostname
		} else if edgeRouterIPOverride != "" {
			r.Edge.AdvertisedHost = edgeRouterIPOverride
		} else {
			r.Edge.AdvertisedHost = r.Edge.Hostname //not redundant set AdvertisedHost
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
