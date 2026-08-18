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

package xgress_edge_tunnel_v2

import (
	"testing"
	"time"

	"github.com/openziti/identity"
	"github.com/openziti/sdk-golang/xgress"
	"github.com/openziti/ziti/common/inspect"
	"github.com/openziti/ziti/common/logcontext"
	"github.com/stretchr/testify/require"
)

// A router with no tunnel listener has no hosted service registry, so the dialer entry points must
// all work without one. Control channel requests for tunnel terminators reach every router, not just
// those hosting tunnel services.
func TestTunnelerWithoutHostedServices(t *testing.T) {
	newUnhostedTunneler := func() *tunneler {
		result := &tunneler{
			notifyReconnect: make(chan struct{}, 1),
			createTime:      time.Now(),
		}
		result.initialized.Store(true)
		return result
	}

	t.Run("inspect returns an empty result", func(t *testing.T) {
		req := require.New(t)

		result := newUnhostedTunneler().Inspect(inspect.ErtTerminatorsKey, time.Second)
		ertResult, ok := result.(*inspect.ErtTerminatorInspectResult)
		req.True(ok, "expected an ERT terminator inspect result, got %T", result)
		req.Empty(ertResult.Entries)
		req.Empty(ertResult.Errors)
	})

	t.Run("inspect of an unrelated key returns nil", func(t *testing.T) {
		req := require.New(t)
		req.Nil(newUnhostedTunneler().Inspect("router-data-model", time.Second))
	})

	t.Run("terminators are reported invalid", func(t *testing.T) {
		req := require.New(t)
		req.False(newUnhostedTunneler().IsTerminatorValid("terminator-id", "hosted:destination"))
	})

	t.Run("dial fails with an invalid terminator error", func(t *testing.T) {
		req := require.New(t)

		_, err := newUnhostedTunneler().Dial(&testDialParams{destination: "hosted:destination"})
		req.Error(err)
		req.IsType(xgress.InvalidTerminatorError{}, err)
	})
}

type testDialParams struct {
	destination string
}

func (self *testDialParams) GetCtrlId() string {
	return "test-ctrl"
}

func (self *testDialParams) GetDestination() string {
	return self.destination
}

func (self *testDialParams) GetCircuitId() *identity.TokenId {
	return &identity.TokenId{Token: "test-circuit"}
}

func (self *testDialParams) GetAddress() xgress.Address {
	return "test-address"
}

func (self *testDialParams) GetBindHandler() xgress.BindHandler {
	return nil
}

func (self *testDialParams) GetLogContext() logcontext.Context {
	return logcontext.NewContext()
}

func (self *testDialParams) GetDeadline() time.Time {
	return time.Now().Add(time.Minute)
}

func (self *testDialParams) GetCircuitTags() map[string]string {
	return nil
}
