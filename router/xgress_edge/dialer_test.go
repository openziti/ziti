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

package xgress_edge

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/openziti/channel/v5"
	"github.com/openziti/identity"
	"github.com/openziti/sdk-golang/v2/xgress"
	sdkEdge "github.com/openziti/sdk-golang/v2/ziti/edge"
	"github.com/openziti/ziti/v2/common/logcontext"
	"github.com/openziti/ziti/v2/router/state"
	"github.com/openziti/ziti/v2/router/xgress_common"
	"github.com/openziti/ziti/v2/router/xgress_router"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/stretchr/testify/require"
)

// closeableCh answers only IsClosed. Every other method dispatches through the nil embedded
// interface and panics, so a dial that gets past the host checks fails loudly.
type closeableCh struct {
	channel.Channel
	closed bool
}

func (self *closeableCh) IsClosed() bool { return self.closed }

// stubDialParams answers what Dial reads before it reaches the host.
type stubDialParams struct {
	destination string
}

func (self *stubDialParams) GetCtrlId() string      { return "ctrl" }
func (self *stubDialParams) GetDestination() string { return self.destination }
func (self *stubDialParams) GetCircuitId() *identity.TokenId {
	return &identity.TokenId{Token: "circuit"}
}
func (self *stubDialParams) GetAddress() xgress.Address         { return "address" }
func (self *stubDialParams) GetBindHandler() xgress.BindHandler { return nil }
func (self *stubDialParams) GetLogContext() logcontext.Context  { return logcontext.NewContext() }
func (self *stubDialParams) GetDeadline() time.Time             { return time.Now().Add(time.Second) }
func (self *stubDialParams) GetCircuitTags() map[string]string  { return nil }

// newDialTestTerminator builds a hosted terminator whose SDK connection is open or closed. A closed
// connection has its channel marked closed and its mux closed, as the close handler leaves them.
func newDialTestTerminator(id string, termState xgress_common.TerminatorState, connClosed bool) *edgeTerminator {
	sdkCh := sdkEdge.NewBaseSdkChannel()
	sdkCh.InitChannel(&closeableCh{closed: connClosed})

	mux := sdkEdge.NewChannelConnMapMux[*state.ConnState](nil)
	if connClosed {
		mux.Close()
	}

	term := &edgeTerminator{
		terminatorId:        id,
		MsgChannel:          *sdkEdge.NewEdgeMsgChannel(sdkCh, 1),
		edgeClientConn:      &edgeClientConn{ch: sdkCh, msgMux: mux},
		serviceSessionToken: &state.ServiceSessionToken{JwtToken: &jwt.Token{Raw: "token"}},
		assignIds:           true,
	}
	term.state.Store(termState)
	return term
}

func newDialTestDialer(terminators ...*edgeTerminator) *dialer {
	registry := &hostedServiceRegistry{terminators: cmap.New[*edgeTerminator]()}
	for _, term := range terminators {
		registry.Put(term)
	}
	return &dialer{factory: &Factory{hostedServices: registry}, options: &Options{}}
}

func requireInvalidTerminator(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	require.True(t, errors.As(err, &xgress.InvalidTerminatorError{}), "expected InvalidTerminatorError, got %T: %v", err, err)
}

func requireUnusableTerminator(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	require.True(t, errors.As(err, &xgress_router.UnusableTerminatorError{}), "expected UnusableTerminatorError, got %T: %v", err, err)
	require.False(t, errors.As(err, &xgress.InvalidTerminatorError{}), "an unusable terminator must not be reported invalid: %v", err)
}

func Test_dialer_Dial_hostState(t *testing.T) {
	t.Run("unknown terminator is invalid", func(t *testing.T) {
		d := newDialTestDialer()
		_, err := d.Dial(&stubDialParams{destination: "hosted:t1"})
		requireInvalidTerminator(t, err)
	})

	t.Run("deleting terminator is unusable", func(t *testing.T) {
		term := newDialTestTerminator("t1", xgress_common.TerminatorStateDeleting, true)
		d := newDialTestDialer(term)

		_, err := d.Dial(&stubDialParams{destination: "hosted:t1"})
		requireUnusableTerminator(t, err)
		require.Zero(t, term.edgeClientConn.idSeq, "dial must be rejected before a conn id is allocated")
	})

	t.Run("terminator with a closed connection is unusable", func(t *testing.T) {
		term := newDialTestTerminator("t1", xgress_common.TerminatorStateEstablished, true)
		d := newDialTestDialer(term)

		_, err := d.Dial(&stubDialParams{destination: "hosted:t1"})
		requireUnusableTerminator(t, err)
		require.Zero(t, term.edgeClientConn.idSeq, "dial must be rejected before a conn id is allocated")
	})
}

func Test_dialer_lookupHost(t *testing.T) {
	t.Run("open, established host is returned", func(t *testing.T) {
		term := newDialTestTerminator("t1", xgress_common.TerminatorStateEstablished, false)
		d := newDialTestDialer(term)

		found, err := d.lookupHost("t1")
		require.NoError(t, err)
		require.Same(t, term, found)
	})

	t.Run("deleting host with an open connection is unusable", func(t *testing.T) {
		term := newDialTestTerminator("t1", xgress_common.TerminatorStateDeleting, false)
		d := newDialTestDialer(term)

		_, err := d.lookupHost("t1")
		requireUnusableTerminator(t, err)
	})
}
