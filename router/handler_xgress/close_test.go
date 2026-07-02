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

package handler_xgress

import (
	"testing"

	"github.com/openziti/ziti/v2/router/env"
	"github.com/stretchr/testify/require"
)

func Test_HandleXgressClose(t *testing.T) {
	t.Run("a closing endpoint leaves the co-located endpoint of its circuit registered", func(t *testing.T) {
		req := require.New(t)

		// arrange: the xgress and the circuit's other endpoint are both registered on this router
		peer := &busyDestination{}
		x, fwd, _ := newRoutedXgress(t, peer, newRecordingFaultReceiver())
		fwd.RegisterDestination(testCircuitId, testSrcAddr, x)
		x.AddCloseHandler(NewCloseHandler(&env.MockNetworkControllers{}, fwd))

		// act
		x.Close()

		// assert
		req.False(fwd.HasDestination(testSrcAddr), "the closing endpoint must be removed")
		req.True(fwd.HasDestination(testDstAddr), "the co-located endpoint must stay registered")
	})
}
