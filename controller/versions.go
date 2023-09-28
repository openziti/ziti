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

const (
	VersionV1 = "v1"

	RestApiV1 = "/" + VersionV1

	RestApiRootPath       = "/edge"
	ClientRestApiBase     = "/edge/client"
	ManagementRestApiBase = "/edge/management"

	LegacyClientRestApiBaseUrlV1 = RestApiRootPath + RestApiV1
	ClientRestApiBaseUrlV1       = ClientRestApiBase + RestApiV1
	ManagementRestApiBaseUrlV1   = ManagementRestApiBase + RestApiV1

	ClientRestApiBaseUrlLatest     = ClientRestApiBaseUrlV1
	ManagementRestApiBaseUrlLatest = ManagementRestApiBaseUrlV1

	ClientRestApiSpecUrl     = ClientRestApiBaseUrlLatest + "/swagger.json"
	ManagementRestApiSpecUrl = ManagementRestApiBaseUrlLatest + "/swagger.json"

	LegacyClientApiBinding = "edge"
	ClientApiBinding       = "edge-client"
	ManagementApiBinding   = "edge-management"
	OidcApiBinding         = "edge-oidc"
)

// AllApiBindingVersions is a map of: API Binding -> Api Version -> API Path
// Adding values here will add them to the /versions REST API endpoint
var AllApiBindingVersions = map[string]map[string]string{
	ClientApiBinding: {
		VersionV1: ClientRestApiBaseUrlV1,
	},
	ManagementApiBinding: {
		VersionV1: ManagementRestApiBaseUrlV1,
	},
}
