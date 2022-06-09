/*
	Copyright NetFoundry, Inc.

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

package api_impl

const (
	ApiVersion = "1.0.0"

	VersionV1 = "v1"

	RestApiV1 = "/" + VersionV1

	FabricRestApiRootPath = "/fabric"

	FabricRestApiBaseUrlV1 = FabricRestApiRootPath + RestApiV1

	FabricRestApiBaseUrlLatest = FabricRestApiBaseUrlV1

	FabricRestApiSpecUrl = FabricRestApiBaseUrlLatest + "/swagger.json"

	FabricApiBinding = "fabric"

	MetricApiBinding = "metrics"
)

// AllApiBindingVersions is a map of: API Binding -> Api Version -> API Path
// Adding values here will add them to the /versions REST API endpoint
var AllApiBindingVersions = map[string]map[string]string{
	FabricApiBinding: {
		VersionV1: FabricRestApiBaseUrlV1,
	},
}
