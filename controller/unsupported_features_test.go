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

	"github.com/openziti/ziti/controller/webapis"
	"github.com/stretchr/testify/require"
)

func TestUnsupportedFeatureWarnings(t *testing.T) {
	legacyOnly := []string{webapis.ClientApiBinding, webapis.ManagementApiBinding}
	withOidc := append([]string{webapis.OidcApiBinding}, legacyOnly...)

	t.Run("standalone without edge-oidc has no warnings", func(t *testing.T) {
		require.Empty(t, unsupportedFeatureWarnings(legacyOnly, false))
	})

	t.Run("standalone with edge-oidc warns about OIDC", func(t *testing.T) {
		require.Equal(t, []string{oidcUnsupportedWarning}, unsupportedFeatureWarnings(withOidc, false))
	})

	t.Run("clustered warns only about HA", func(t *testing.T) {
		require.Equal(t, []string{haUnsupportedWarning}, unsupportedFeatureWarnings(withOidc, true))
		require.Equal(t, []string{haUnsupportedWarning}, unsupportedFeatureWarnings(legacyOnly, true))
	})
}
