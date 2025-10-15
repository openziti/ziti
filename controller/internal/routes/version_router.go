/*
	Copyright NetFoundry Inc.

	Licensed under the Apache License, Version 2.0 (the "License");
	you may not use this file except in compliance with the License.
	You may obtain a copy of the License at

	https://www.apache.org/licenses/LICENSE-2.0
	https://www.apache.org/licenses/LICENSE-2.0

	Unless required by applicable law or agreed to in writing, software
	distributed under the License is distributed on an "AS IS" BASIS,
	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
	See the License for the specific language governing permissions and
	limitations under the License.
*/

package routes

import (
	"runtime"
	"sync"

	"github.com/go-openapi/runtime/middleware"
	clientInformational "github.com/openziti/edge-api/rest_client_api_server/operations/informational"
	managementInformational "github.com/openziti/edge-api/rest_management_api_server/operations/informational"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/xweb/v2"
	"github.com/openziti/ziti/common/build"
	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/controller/internal/permissions"
	"github.com/openziti/ziti/controller/response"
	"github.com/openziti/ziti/controller/webapis"
)

func init() {
	r := NewVersionRouter()
	env.AddRouter(r)
}

type VersionRouter struct {
	BasePath           string
	cachedVersions     *rest_model.Version
	cachedVersionsOnce sync.Once
}

func NewVersionRouter() *VersionRouter {
	return &VersionRouter{
		BasePath: "/version",
	}
}

func (ir *VersionRouter) Register(ae *env.AppEnv) {
	ae.ClientApi.InformationalListVersionHandler = clientInformational.ListVersionHandlerFunc(func(params clientInformational.ListVersionParams) middleware.Responder {
		return ae.IsAllowed(ir.List, params.HTTPRequest, "", "", permissions.Always())
	})

	ae.ClientApi.InformationalListRootHandler = clientInformational.ListRootHandlerFunc(func(params clientInformational.ListRootParams) middleware.Responder {
		return ae.IsAllowed(ir.List, params.HTTPRequest, "", "", permissions.Always())
	})

	ae.ClientApi.InformationalListEnumeratedCapabilitiesHandler = clientInformational.ListEnumeratedCapabilitiesHandlerFunc(func(params clientInformational.ListEnumeratedCapabilitiesParams) middleware.Responder {
		return ae.IsAllowed(ir.ListCapabilities, params.HTTPRequest, "", "", permissions.Always())
	})

	ae.ManagementApi.InformationalListVersionHandler = managementInformational.ListVersionHandlerFunc(func(params managementInformational.ListVersionParams) middleware.Responder {
		return ae.IsAllowed(ir.List, params.HTTPRequest, "", "", permissions.Always())
	})

	ae.ManagementApi.InformationalListRootHandler = managementInformational.ListRootHandlerFunc(func(params managementInformational.ListRootParams) middleware.Responder {
		return ae.IsAllowed(ir.List, params.HTTPRequest, "", "", permissions.Always())
	})

	ae.ManagementApi.InformationalListEnumeratedCapabilitiesHandler = managementInformational.ListEnumeratedCapabilitiesHandlerFunc(func(params managementInformational.ListEnumeratedCapabilitiesParams) middleware.Responder {
		return ae.IsAllowed(ir.ListCapabilities, params.HTTPRequest, "", "", permissions.Always())
	})
}

func (ir *VersionRouter) List(ae *env.AppEnv, rc *response.RequestContext) {
	ir.cachedVersionsOnce.Do(func() {

		buildInfo := build.GetBuildInfo()
		ir.cachedVersions = &rest_model.Version{
			BuildDate:      buildInfo.BuildDate(),
			Revision:       buildInfo.Revision(),
			RuntimeVersion: runtime.Version(),
			Version:        buildInfo.Version(),
			APIVersions: map[string]map[string]rest_model.APIVersion{
				webapis.ClientApiBinding:                {webapis.VersionV1: mapApiVersionToRestModel(webapis.ClientRestApiBaseUrlV1)},
				webapis.ManagementApiBinding:            {webapis.VersionV1: mapApiVersionToRestModel(webapis.ManagementRestApiBaseUrlV1)},
				webapis.OidcApiBinding:                  {webapis.VersionV1: mapApiVersionToRestModel(webapis.OidcRestApiBaseUrl)},
				webapis.ControllerHealthCheckApiBinding: {webapis.VersionV1: mapApiVersionToRestModel(webapis.ControllerHealthCheckApiBaseUrlV1)},
			},
			Capabilities: []string{},
		}

		for apiBinding, apiVersionToPathMap := range webapis.AllApiBindingVersions {
			ir.cachedVersions.APIVersions[apiBinding] = map[string]rest_model.APIVersion{}

			for apiVersion, apiPath := range apiVersionToPathMap {
				ir.cachedVersions.APIVersions[apiBinding][apiVersion] = mapApiVersionToRestModel(apiPath)
			}
		}

		xwebContext := xweb.ServerContextFromRequestContext(rc.Request.Context())

		apiToBaseUrls := map[string]map[string]struct{}{} //api -> webListener addresses + path

		activeBindings := map[string]struct{}{}
		for _, webListener := range xwebContext.Config.ServerConfigs {
			for _, api := range webListener.APIs {
				if _, ok := apiToBaseUrls[api.Binding()]; !ok {
					apiToBaseUrls[api.Binding()] = map[string]struct{}{}
					activeBindings[api.Binding()] = struct{}{}
				}

				for _, bindPoint := range webListener.BindPoints {
					apiBaseUrl := bindPoint.Address + apiBindingToPath(api.Binding())
					apiToBaseUrls[api.Binding()][apiBaseUrl] = struct{}{}
				}
			}
		}

		oidcEnabled := false

		for _, serverConfig := range ae.HostController.GetXWebInstance().GetConfig().ServerConfigs {
			for _, api := range serverConfig.APIs {
				if api.Binding() == webapis.OidcApiBinding {
					oidcEnabled = true
					break
				}
			}

			if oidcEnabled {
				break
			}
		}

		var apiToRemove []string
		for apiBinding, apiVersionMap := range ir.cachedVersions.APIVersions {
			if _, ok := activeBindings[apiBinding]; ok {
				for apiBaseUrl := range apiToBaseUrls[apiBinding] {
					apiVersion := apiVersionMap["v1"]
					apiVersion.APIBaseUrls = append(apiVersion.APIBaseUrls, "https://"+apiBaseUrl)
					apiVersionMap["v1"] = apiVersion
				}
			} else {
				apiToRemove = append(apiToRemove, apiBinding)
			}
		}

		for _, toRemove := range apiToRemove {
			delete(ir.cachedVersions.APIVersions, toRemove)
		}

		ir.cachedVersions.APIVersions[webapis.LegacyClientApiBinding] = ir.cachedVersions.APIVersions[webapis.ClientApiBinding]

		if oidcEnabled {
			ir.cachedVersions.Capabilities = append(ir.cachedVersions.Capabilities, string(rest_model.CapabilitiesOIDCAUTH))
		}

		if ae.HostController.IsRaftEnabled() {
			ir.cachedVersions.Capabilities = append(ir.cachedVersions.Capabilities, string(rest_model.CapabilitiesHACONTROLLER))
		}

	})

	rc.RespondWithOk(ir.cachedVersions, &rest_model.Meta{})
}

func (ir *VersionRouter) ListCapabilities(_ *env.AppEnv, rc *response.RequestContext) {
	capabilities := []rest_model.Capabilities{
		rest_model.CapabilitiesOIDCAUTH,
		rest_model.CapabilitiesHACONTROLLER,
	}

	rc.RespondWithOk(capabilities, &rest_model.Meta{})
}

func apiBindingToPath(binding string) string {
	switch binding {
	case "edge":
		return webapis.ClientRestApiBaseUrlV1
	case webapis.ClientApiBinding:
		return webapis.ClientRestApiBaseUrlV1
	case webapis.ManagementApiBinding:
		return webapis.ManagementRestApiBaseUrlV1
	case webapis.OidcApiBinding:
		return webapis.OidcRestApiBaseUrl
	case webapis.ControllerHealthCheckApiBinding:
		return webapis.ControllerHealthCheckApiBaseUrlV1
	}
	return ""
}

func mapApiVersionToRestModel(path string) rest_model.APIVersion {
	return rest_model.APIVersion{
		Path:        &path,
		APIBaseUrls: []string{},
	}
}
