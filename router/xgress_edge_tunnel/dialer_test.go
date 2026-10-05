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

package xgress_edge_tunnel

import (
	"errors"
	"testing"

	"github.com/openziti/sdk-golang/v2/xgress"
	"github.com/openziti/ziti/v2/router/xgress_common"
	"github.com/openziti/ziti/v2/router/xgress_router"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/stretchr/testify/require"
)

func newLookupTestTunneler(terminators ...*tunnelTerminator) *tunneler {
	registry := &HostedServiceRegistry{terminators: cmap.New[*tunnelTerminator]()}
	for _, terminator := range terminators {
		registry.terminators.Set(terminator.id, terminator)
	}
	return &tunneler{hostedServices: registry}
}

func newLookupTestTerminator(id string, state xgress_common.TerminatorState) *tunnelTerminator {
	terminator := &tunnelTerminator{id: id}
	terminator.state.Store(state)
	return terminator
}

func requireUnusableTerminator(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	require.True(t, errors.As(err, &xgress_router.UnusableTerminatorError{}), "expected UnusableTerminatorError, got %T: %v", err, err)
	require.False(t, errors.As(err, &xgress.InvalidTerminatorError{}), "an unusable terminator must not be reported invalid: %v", err)
}

func Test_tunneler_lookupHost(t *testing.T) {
	t.Run("hosted terminator is returned", func(t *testing.T) {
		terminator := newLookupTestTerminator("t1", xgress_common.TerminatorStateEstablished)
		tun := newLookupTestTunneler(terminator)

		found, err := tun.lookupHost("t1")
		require.NoError(t, err)
		require.Same(t, terminator, found)
	})

	t.Run("unknown terminator is invalid", func(t *testing.T) {
		tun := newLookupTestTunneler()

		_, err := tun.lookupHost("t1")
		require.Error(t, err)
		require.True(t, errors.As(err, &xgress.InvalidTerminatorError{}), "expected InvalidTerminatorError, got %T: %v", err, err)
	})

	t.Run("deleting terminator is unusable", func(t *testing.T) {
		tun := newLookupTestTunneler(newLookupTestTerminator("t1", xgress_common.TerminatorStateDeleting))

		_, err := tun.lookupHost("t1")
		requireUnusableTerminator(t, err)
	})

	t.Run("closed terminator is unusable", func(t *testing.T) {
		terminator := newLookupTestTerminator("t1", xgress_common.TerminatorStateEstablished)
		terminator.closed.Store(true)
		tun := newLookupTestTunneler(terminator)

		_, err := tun.lookupHost("t1")
		requireUnusableTerminator(t, err)
	})
}
