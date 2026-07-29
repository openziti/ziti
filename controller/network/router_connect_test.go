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

package network

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/openziti/transport/v2"
	"github.com/openziti/transport/v2/tcp"
	"github.com/openziti/ziti/v2/common/ctrlchan"
	"github.com/openziti/ziti/v2/controller/model"
	"github.com/stretchr/testify/require"
)

// fakeCtrlChannel is a minimal ctrlchan.CtrlChannel double for connect/disconnect lifecycle tests.
// It embeds the interface (unimplemented methods panic if called), and implements only the close-related
// methods the reject/kick path exercises. onClose, if set, runs synchronously on the first Close to
// simulate the real channel close handler firing DisconnectRouter.
type fakeCtrlChannel struct {
	ctrlchan.CtrlChannel
	closed  atomic.Bool
	onClose func()
}

func (f *fakeCtrlChannel) Close() error {
	if f.closed.CompareAndSwap(false, true) && f.onClose != nil {
		f.onClose()
	}
	return nil
}

func (f *fakeCtrlChannel) IsClosed() bool { return f.closed.Load() }

func (f *fakeCtrlChannel) IsConnected() bool { return !f.closed.Load() }

func newConnectTestNetwork(t *testing.T) (*model.TestContext, *Network, transport.Address) {
	ctx := model.NewTestContext(t)
	t.Cleanup(ctx.Cleanup)

	config := newTestConfig(ctx)
	t.Cleanup(func() { close(config.closeNotify) })

	network, err := NewNetwork(config, ctx)
	require.NoError(t, err)

	addr, err := tcp.AddressParser{}.Parse("tcp:0.0.0.0:0")
	require.NoError(t, err)

	return ctx, network, addr
}

// TestConnectRouter_RejectsAndKicksWhenBusy: a connect into an occupied slot returns ErrConnectRejected,
// kicks the occupant (whose teardown clears the slot), and a subsequent redial then connects cleanly.
func TestConnectRouter_RejectsAndKicksWhenBusy(t *testing.T) {
	_, network, addr := newConnectTestNetwork(t)

	curCh := &fakeCtrlChannel{}
	cur := model.NewRouterForTest("r1", "", addr, curCh, 0, false)
	// Simulate the real close handler: closing the occupant runs its DisconnectRouter.
	curCh.onClose = func() { network.DisconnectRouter(cur) }

	require.NoError(t, network.ConnectRouter(cur))
	require.Equal(t, cur, network.Router.GetConnected("r1"))

	newCh := &fakeCtrlChannel{}
	rNew := model.NewRouterForTest("r1", "", addr, newCh, 0, false)
	err := network.ConnectRouter(rNew)
	require.ErrorIs(t, err, ErrConnectRejected)
	require.True(t, IsConnectRejected(err))
	require.True(t, curCh.IsClosed(), "occupant should have been kicked")
	require.Nil(t, network.Router.GetConnected("r1"), "kicked occupant's teardown should clear the slot")
	require.False(t, rNew.Connected.Load(), "rejected connect must not be registered")

	// Redial into the now-clear slot succeeds.
	rRedial := model.NewRouterForTest("r1", "", addr, &fakeCtrlChannel{}, 0, false)
	require.NoError(t, network.ConnectRouter(rRedial))
	require.Equal(t, rRedial, network.Router.GetConnected("r1"))
	require.True(t, rRedial.Connected.Load())
}

// TestConnectRouter_SetsUpWhenClear: a connect into an empty slot registers the router.
func TestConnectRouter_SetsUpWhenClear(t *testing.T) {
	_, network, addr := newConnectTestNetwork(t)

	r := model.NewRouterForTest("r1", "", addr, &fakeCtrlChannel{}, 0, false)
	require.NoError(t, network.ConnectRouter(r))
	require.Equal(t, r, network.Router.GetConnected("r1"))
	require.True(t, r.Connected.Load())
}

// TestDisconnectRouter_IgnoresStaleConnection: a disconnect for a superseded connection must not disturb
// the current one (the §9 race guard).
func TestDisconnectRouter_IgnoresStaleConnection(t *testing.T) {
	_, network, addr := newConnectTestNetwork(t)

	rNew := model.NewRouterForTest("r1", "", addr, &fakeCtrlChannel{}, 0, false)
	require.NoError(t, network.ConnectRouter(rNew))
	require.Equal(t, rNew, network.Router.GetConnected("r1"))

	// A stale disconnect for a different (old) instance of the same router id.
	rOld := model.NewRouterForTest("r1", "", addr, &fakeCtrlChannel{}, 0, false)
	network.DisconnectRouter(rOld)

	require.Equal(t, rNew, network.Router.GetConnected("r1"), "stale disconnect must not evict the current connection")
	require.True(t, rNew.Connected.Load())
}

// TestDisconnectRouter_CurrentTearsDown: a disconnect for the current connection clears it.
func TestDisconnectRouter_CurrentTearsDown(t *testing.T) {
	_, network, addr := newConnectTestNetwork(t)

	r := model.NewRouterForTest("r1", "", addr, &fakeCtrlChannel{}, 0, false)
	require.NoError(t, network.ConnectRouter(r))
	require.Equal(t, r, network.Router.GetConnected("r1"))

	network.DisconnectRouter(r)
	require.Nil(t, network.Router.GetConnected("r1"))
	require.False(t, r.Connected.Load())
}

// TestReject_DisplacesOccupantThatCannotTearItselfDown is the regression guard against a permanent
// reject loop. A connection whose channel closes without its close handler running can never remove
// itself from the connected map, so a reject that merely closed the channel would leave the slot occupied
// forever and every redial would be rejected against a slot nothing can free. The reject must displace
// the occupant itself, so the next redial finds the slot clear.
func TestReject_DisplacesOccupantThatCannotTearItselfDown(t *testing.T) {
	_, network, addr := newConnectTestNetwork(t)

	// No onClose: the occupant's channel goes closed without any teardown running, modeling both a
	// channel already closed before the reject and one whose close handler has already run.
	curCh := &fakeCtrlChannel{}
	cur := model.NewRouterForTest("r1", "", addr, curCh, 0, false)
	require.NoError(t, network.ConnectRouter(cur))
	require.Equal(t, cur, network.Router.GetConnected("r1"))
	curCh.closed.Store(true)

	rNew := model.NewRouterForTest("r1", "", addr, &fakeCtrlChannel{}, 0, false)
	require.ErrorIs(t, network.ConnectRouter(rNew), ErrConnectRejected)
	require.Nil(t, network.Router.GetConnected("r1"), "reject must displace an occupant that cannot tear itself down")
	require.False(t, cur.Connected.Load())
	require.False(t, rNew.Connected.Load(), "rejected connect must not be registered")

	// The redial therefore makes progress instead of bouncing off the slot forever.
	rRedial := model.NewRouterForTest("r1", "", addr, &fakeCtrlChannel{}, 0, false)
	require.NoError(t, network.ConnectRouter(rRedial))
	require.Equal(t, rRedial, network.Router.GetConnected("r1"))
	require.True(t, rRedial.Connected.Load())
}

// TestConnectRouter_RefusesAlreadyClosedChannel: a connect whose channel is already closed must be
// refused rather than registered. Registering it would put a connection in the connected map that no
// disconnect can ever remove, since its close handler has already run, wedging the router out of this
// controller permanently.
func TestConnectRouter_RefusesAlreadyClosedChannel(t *testing.T) {
	_, network, addr := newConnectTestNetwork(t)

	deadCh := &fakeCtrlChannel{}
	deadCh.closed.Store(true)
	r := model.NewRouterForTest("r1", "", addr, deadCh, 0, false)

	err := network.ConnectRouter(r)
	require.ErrorIs(t, err, ErrConnectChannelClosed)
	require.True(t, IsConnectRejected(err), "an already-closed channel is a refusal the router redials after")
	require.Nil(t, network.Router.GetConnected("r1"), "a closed connection must not occupy the slot")
	require.False(t, r.Connected.Load())

	// A subsequent healthy connect is unaffected.
	rOk := model.NewRouterForTest("r1", "", addr, &fakeCtrlChannel{}, 0, false)
	require.NoError(t, network.ConnectRouter(rOk))
	require.Equal(t, rOk, network.Router.GetConnected("r1"))
}

// TestConnectDisconnectRace exercises concurrent connect/disconnect for one router id under the race
// detector, asserting the map never ends holding a disconnected router.
func TestConnectDisconnectRace(t *testing.T) {
	_, network, addr := newConnectTestNetwork(t)

	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(connect bool) {
			defer wg.Done()
			if connect {
				r := model.NewRouterForTest("r1", "", addr, &fakeCtrlChannel{}, 0, false)
				_ = network.ConnectRouter(r)
			} else if cur := network.Router.GetConnected("r1"); cur != nil {
				network.DisconnectRouter(cur)
			}
		}(i%2 == 0)
	}
	wg.Wait()

	if cur := network.Router.GetConnected("r1"); cur != nil {
		require.True(t, cur.Connected.Load(), "map must not hold a disconnected router")
	}
}
