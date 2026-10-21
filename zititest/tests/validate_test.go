/*
	(c) Copyright NetFoundry Inc.

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

package tests

import (
	"testing"
	"time"

	"github.com/openziti/ziti/zititest/models/smoke"
	"github.com/openziti/ziti/zititest/zitilab/validations"
	"github.com/stretchr/testify/require"
)

const validationTimeout = 2 * time.Minute

// TestFabricValidation checks that each controller's view of circuits, terminators, links, the router
// data model and identity connections matches the routers' own, after the traffic tests have run. Go
// runs a package's tests in file order, and this file sorts after the traffic tests.
func TestFabricValidation(t *testing.T) {
	m := run.GetModel()

	t.Run("circuits", func(t *testing.T) {
		skipUnlessRoutersSupport(t, ".edge-router", smoke.CapValidateCircuits)
		require.NoError(t, validations.ValidateCircuits(run, validationTimeout, "limit none"))
	})

	t.Run("sdk-terminators", func(t *testing.T) {
		require.NoError(t, validations.ValidateTerminators(run, validationTimeout, validations.MinCount(1),
			validations.ValidateSdkTerminators))
	})

	// only routers hosting ERT terminators are inspected: asking another router for its ERT terminators
	// panics 1.6 routers
	t.Run("ert-terminators", func(t *testing.T) {
		skipUnlessRoutersSupport(t, ".ert-host", smoke.CapValidateErtTerminators)
		ertHosts := validations.RouterNameFilter(m.SelectComponents(".ert-host"))
		require.NoError(t, validations.ValidateTerminatorsWithFilters(run, validationTimeout, validations.MinCount(1),
			validations.ValidateErtTerminators, "limit none", ertHosts))
	})

	t.Run("router-links", func(t *testing.T) {
		require.NoError(t, validations.ValidateRouterLinks(run, validationTimeout))
	})

	t.Run("router-data-model", func(t *testing.T) {
		skipUnlessRoutersSupport(t, ".edge-router", smoke.CapValidateRouterDataModel)
		require.NoError(t, validations.ValidateRouterDataModel(run, validationTimeout))
	})

	t.Run("identity-connection-statuses", func(t *testing.T) {
		skipUnlessRoutersSupport(t, ".edge-router", smoke.CapValidateConnectionStatuses)
		require.NoError(t, validations.ValidateIdentityConnectionStatuses(run, validationTimeout))
	})
}
