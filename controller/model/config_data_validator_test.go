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

package model

import (
	"errors"
	"testing"

	"github.com/openziti/ziti/v2/common/config/routerlink"
	"github.com/stretchr/testify/require"
)

func TestConfigManager_DataValidatorRegistry(t *testing.T) {
	req := require.New(t)
	manager := &ConfigManager{dataValidators: map[string]ConfigDataValidator{}}

	// An unregistered type is not an error; most config types have no rules
	// beyond their schema.
	req.NoError(manager.validateData("some.other.type", map[string]interface{}{}))

	sentinel := errors.New("nope")
	manager.RegisterDataValidator("my.type.v1", func(map[string]interface{}) error {
		return sentinel
	})

	req.ErrorIs(manager.validateData("my.type.v1", nil), sentinel)
	req.NoError(manager.validateData("my.other.type.v1", nil), "validators are scoped to their type")

	// Re-registering replaces, so init order can't leave two validators fighting.
	manager.RegisterDataValidator("my.type.v1", func(map[string]interface{}) error { return nil })
	req.NoError(manager.validateData("my.type.v1", nil))
}

func TestValidateRouterLinkConfigData(t *testing.T) {
	tests := []struct {
		name      string
		data      map[string]interface{}
		expectErr string
	}{
		{
			name: "empty config is valid",
			data: map[string]interface{}{},
		},
		{
			name: "sane heartbeats",
			data: map[string]interface{}{
				"heartbeats": map[string]interface{}{
					"sendInterval":             "10s",
					"checkInterval":            "1s",
					"closeUnresponsiveTimeout": "60s",
				},
			},
		},
		{
			// The check interval outruns the timeout, so every pulse would send a
			// heartbeat and condemn the link before a response could arrive.
			name: "check interval past the timeout",
			data: map[string]interface{}{
				"heartbeats": map[string]interface{}{
					"checkInterval":            "2m",
					"closeUnresponsiveTimeout": "30s",
				},
			},
			expectErr: "closeUnresponsiveTimeout",
		},
		{
			name: "non-positive interval",
			data: map[string]interface{}{
				"heartbeats": map[string]interface{}{"checkInterval": "0s"},
			},
			expectErr: "must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := require.New(t)
			err := validateRouterLinkConfigData(tt.data)
			if tt.expectErr == "" {
				req.NoError(err)
				return
			}
			req.Error(err)
			req.Contains(err.Error(), tt.expectErr)
		})
	}
}

func TestNewConfigManager_RegistersRouterLinkValidator(t *testing.T) {
	// The built-in validator is wired at construction rather than by a separate
	// init step, so guard that the wiring exists and is keyed correctly.
	req := require.New(t)
	manager := &ConfigManager{dataValidators: map[string]ConfigDataValidator{}}
	manager.RegisterDataValidator(routerlink.ConfigTypeV1, validateRouterLinkConfigData)

	err := manager.validateData(routerlink.ConfigTypeV1, map[string]interface{}{
		"heartbeats": map[string]interface{}{
			"checkInterval":            "2m",
			"closeUnresponsiveTimeout": "30s",
		},
	})
	req.Error(err, "a router.link.v1 config with unsafe timing must be rejected")
}
