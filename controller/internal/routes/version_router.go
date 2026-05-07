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
	"github.com/openziti/ziti/v2/common/bindpoints"
	"github.com/openziti/ziti/v2/common/build"
	"github.com/openziti/ziti/v2/controller/env"
	"github.com/openziti/ziti/v2/controller/permissions"
	"github.com/openziti/ziti/v2/controller/response"
	"github.com/openziti/ziti/v2/controller/webapis"
)

func init() {
	r := NewVersionRouter()
	env.AddRouter(r)
}

type VersionRouter struct {
	BasePath string

	//in deployed systems this will be a map of 1 but for API tests
	//they run in a single process space. This is used to avoid API
	//version test collision.
	versionCache sync.Map // AppEnv.InstanceId -> *rest_model.Version
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
	if v, ok := ir.versionCache.Load(ae.InstanceId); ok {
		rc.RespondWithOk(v.(*rest_model.Version), &rest_model.Meta{})
		return
	}
	v, _ := ir.versionCache.LoadOrStore(ae.InstanceId, ir.buildVersions(ae))
	rc.RespondWithOk(v.(*rest_model.Version), &rest_model.Meta{})
}

func (ir *VersionRouter) buildVersions(ae *env.AppEnv) *rest_model.Version {
	buildInfo := build.GetBuildInfo()
	v := &rest_model.Version{
		BuildDate:      buildInfo.BuildDate(),
		Revision:       buildInfo.Revision(),
		RuntimeVersion: runtime.Version(),
		Version:        buildInfo.Version(),
		APIVersions:    map[string]map[string]rest_model.APIVersion{},
		Capabilities:   []string{},
	}

	for apiBinding, apiVersionToPathMap := range webapis.AllApiBindingVersions {
		v.APIVersions[apiBinding] = map[string]rest_model.APIVersion{}
		for apiVersion, apiPath := range apiVersionToPathMap {
			v.APIVersions[apiBinding][apiVersion] = mapApiVersionToRestModel(apiPath)
		}
	}

	apiToBaseUrls := map[string]map[string]struct{}{} // api -> set of "host:port/path" strings
	activeBindings := map[string]struct{}{}
	oidcEnabled := false

	for _, serverConfig := range ae.HostController.GetXWebInstance().GetConfig().ServerConfigs {
		for _, api := range serverConfig.APIs {
			if _, ok := apiToBaseUrls[api.Binding()]; !ok {
				apiToBaseUrls[api.Binding()] = map[string]struct{}{}
				activeBindings[api.Binding()] = struct{}{}
			}
			for _, bindPoint := range serverConfig.BindPoints {
				if bindPoint.Type() != bindpoints.BindPointTypeUnderlay {
					continue
				}
				apiBaseUrl := bindPoint.ServerAddress() + apiBindingToPath(api.Binding())
				apiToBaseUrls[api.Binding()][apiBaseUrl] = struct{}{}
			}
			if api.Binding() == webapis.OidcApiBinding {
				oidcEnabled = true
			}
		}
	}

	var apiToRemove []string
	for apiBinding, apiVersionMap := range v.APIVersions {
		if _, ok := activeBindings[apiBinding]; ok {
			for apiBaseUrl := range apiToBaseUrls[apiBinding] {
				apiVersion := apiVersionMap[webapis.VersionV1]
				apiVersion.APIBaseUrls = append(apiVersion.APIBaseUrls, "https://"+apiBaseUrl)
				apiVersionMap[webapis.VersionV1] = apiVersion
			}
		} else {
			apiToRemove = append(apiToRemove, apiBinding)
		}
	}

	for _, toRemove := range apiToRemove {
		delete(v.APIVersions, toRemove)
	}

	v.APIVersions[webapis.LegacyClientApiBinding] = v.APIVersions[webapis.ClientApiBinding]

	if oidcEnabled {
		v.Capabilities = append(v.Capabilities, string(rest_model.CapabilitiesOIDCAUTH))
		v.Capabilities = append(v.Capabilities, string(rest_model.CapabilitiesOIDCAUTHWITHCSR))
	}

	if ae.HostController.IsRaftEnabled() {
		v.Capabilities = append(v.Capabilities, string(rest_model.CapabilitiesHACONTROLLER))
	}

	return v
}

func (ir *VersionRouter) Shutdown(ae *env.AppEnv) {
	ir.versionCache.Delete(ae.InstanceId)
}

func (ir *VersionRouter) ListCapabilities(_ *env.AppEnv, rc *response.RequestContext) {
	capabilities := []rest_model.Capabilities{
		rest_model.CapabilitiesOIDCAUTH,
		rest_model.CapabilitiesHACONTROLLER,
		rest_model.CapabilitiesOIDCAUTHWITHCSR,
	}

	rc.RespondWithOk(capabilities, &rest_model.Meta{})
}

func apiBindingToPath(binding string) string {
	switch binding {
	case webapis.LegacyClientApiBinding:
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
