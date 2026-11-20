//go:build apitests

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

package tests

import (
	"testing"
	"time"

	"github.com/openziti/ziti/common/cert"
	"github.com/openziti/ziti/router/state"
)

func Test_RouterEnrollmentExtendTwiceOverOneCtrlChannel(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	ctx.createAndEnrollEdgeRouter(false)
	edgeRouter := ctx.startEdgeRouter(nil)

	routerId := ctx.edgeRouterEntity.id
	ctx.Req.Eventually(func() bool {
		return ctx.fabricController.GetNetwork().GetConnectedRouter(routerId) != nil
	}, 45*time.Second, 100*time.Millisecond, "router should connect to the controller")

	routerManager := ctx.EdgeController.AppEnv.Managers.EdgeRouter
	fingerprints := cert.NewFingerprintGenerator()
	ctrlCh := ctx.fabricController.GetNetwork().GetConnectedRouter(routerId).Control

	enrolled, err := routerManager.Read(routerId)
	ctx.Req.NoError(err)
	ctx.Req.NotNil(enrolled.Fingerprint, "an enrolled router should have a stored fingerprint")
	certsAFingerprint := *enrolled.Fingerprint

	certChecker := state.NewCertExpirationChecker(edgeRouter.Router, false)

	t.Run("first extension replaces the stored fingerprint", func(t *testing.T) {
		ctx.testContextChanged(t)

		err := certChecker.ExtendEnrollment()

		ctx.Req.NoError(err)
		ctx.Req.Eventually(func() bool {
			current, err := routerManager.Read(routerId)
			if err != nil || current.Fingerprint == nil || *current.Fingerprint == certsAFingerprint {
				return false
			}
			return fingerprints.FromCert(edgeRouter.Router.GetRouterId().Cert().Leaf) == *current.Fingerprint
		}, 15*time.Second, 100*time.Millisecond, "the controller should store a new fingerprint and the router should load the matching certificate")

		t.Run("second extension over the same control channel replaces it again", func(t *testing.T) {
			ctx.testContextChanged(t)

			extended, err := routerManager.Read(routerId)
			ctx.Req.NoError(err)
			certsBFingerprint := *extended.Fingerprint
			connected := ctx.fabricController.GetNetwork().GetConnectedRouter(routerId)
			ctx.Req.NotNil(connected, "the router should still be connected")
			ctx.Req.Same(ctrlCh, connected.Control, "the control channel should be the one opened before the first extension")
			certChecker.SetIsRequesting(false)

			err = certChecker.ExtendEnrollment()

			ctx.Req.NoError(err)
			ctx.Req.Eventually(func() bool {
				current, err := routerManager.Read(routerId)
				if err != nil || current.Fingerprint == nil || *current.Fingerprint == certsBFingerprint {
					return false
				}
				return fingerprints.FromCert(edgeRouter.Router.GetRouterId().Cert().Leaf) == *current.Fingerprint
			}, 15*time.Second, 100*time.Millisecond, "a second extension on a control channel opened with the first certificate should succeed")
		})
	})
}
