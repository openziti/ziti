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
	"testing"

	"github.com/openziti/xweb/v3"
	"github.com/openziti/ziti/v2/controller/config"
	"github.com/openziti/ziti/v2/controller/webapis"
	"github.com/stretchr/testify/require"
)

func Test_OIDC_AutoBinding(t *testing.T) {
	tests := []struct {
		name     string
		enabled  bool
		disabled bool
		bindings [][]string
		want     []int
	}{
		{
			name:    "explicit OIDC on 8480 does not suppress management OIDC on 8481",
			enabled: true,
			bindings: [][]string{
				{webapis.ClientApiBinding, webapis.OidcApiBinding},
				{webapis.ManagementApiBinding, webapis.FabricApiBinding},
			},
			want: []int{1, 1},
		},
		{
			name:    "separate client and management listeners both receive OIDC",
			enabled: true,
			bindings: [][]string{
				{webapis.ClientApiBinding},
				{webapis.ManagementApiBinding},
			},
			want: []int{1, 1},
		},
		{
			name:     "fabric-only listener does not receive OIDC",
			enabled:  true,
			bindings: [][]string{{webapis.FabricApiBinding}},
			want:     []int{0},
		},
		{
			name:     "combined client and management listener receives OIDC once",
			enabled:  true,
			bindings: [][]string{{webapis.ClientApiBinding, webapis.ManagementApiBinding}},
			want:     []int{1},
		},
		{
			name:     "auto-binding disabled",
			enabled:  true,
			disabled: true,
			bindings: [][]string{{webapis.ClientApiBinding}, {webapis.ManagementApiBinding}},
			want:     []int{0, 0},
		},
		{
			name:     "edge disabled",
			bindings: [][]string{{webapis.ClientApiBinding}, {webapis.ManagementApiBinding}},
			want:     []int{0, 0},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			controller := &Controller{config: &config.Config{Edge: &config.EdgeConfig{
				Enabled: test.enabled,
				Api:     config.Api{DisableOidcAutoBinding: test.disabled},
			}}}
			instanceConfig := &xweb.InstanceConfig{}
			for _, bindings := range test.bindings {
				serverConfig := &xweb.ServerConfig{}
				for _, binding := range bindings {
					apiConfig := &xweb.ApiConfig{}
					require.NoError(t, apiConfig.Parse(map[interface{}]interface{}{"binding": binding}))
					serverConfig.APIs = append(serverConfig.APIs, apiConfig)
				}
				instanceConfig.ServerConfigs = append(instanceConfig.ServerConfigs, serverConfig)
			}

			// Repeated validation must not duplicate either explicit or auto-added OIDC bindings.
			for pass := 0; pass < 2; pass++ {
				require.NoError(t, controller.ensureOidcOnClientApiServer(instanceConfig))
				for i, serverConfig := range instanceConfig.ServerConfigs {
					count := 0
					for _, api := range serverConfig.APIs {
						if api.Binding() == webapis.OidcApiBinding {
							count++
						}
					}
					require.Equal(t, test.want[i], count, "listener %d on validation pass %d", i, pass)
				}
			}
		})
	}
}
