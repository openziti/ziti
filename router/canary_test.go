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

package router

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/openziti/channel/v5"
	"github.com/openziti/ziti/v2/common/capabilities"
	"github.com/openziti/ziti/v2/common/pb/gossip_pb"
	"github.com/openziti/ziti/v2/router/env"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// canaryCtrls is a controller set whose subscription controller is sub.
type canaryCtrls struct {
	*env.MockNetworkControllers
	sub env.NetworkController
}

func (self *canaryCtrls) GetSubscriptionController() env.NetworkController {
	return self.sub
}

type canarySubscriptionCtrl struct {
	env.NetworkController
	ch        channel.Channel
	connected atomic.Bool
}

func (self *canarySubscriptionCtrl) IsConnected() bool        { return self.connected.Load() }
func (self *canarySubscriptionCtrl) Channel() channel.Channel { return self.ch }

// canaryCaptureChannel hands every message sent on it to sent.
type canaryCaptureChannel struct {
	channel.Channel
	sent chan *channel.Message
}

func (self *canaryCaptureChannel) Send(s channel.Sendable) error {
	self.sent <- s.Msg()
	return nil
}

func (self *canaryCaptureChannel) IsClosed() bool { return false }

func newTestCanaryEmitter(t *testing.T, caps *capabilities.RouterCapabilityMask) (*canaryEmitter, *canarySubscriptionCtrl, chan *channel.Message) {
	sent := make(chan *channel.Message, 16)
	sub := &canarySubscriptionCtrl{ch: &canaryCaptureChannel{sent: sent}}
	ctrls := &canaryCtrls{MockNetworkControllers: &env.MockNetworkControllers{}, sub: sub}
	emitter := newCanaryEmitter(ctrls, epochAt(1_000), "v2.1.0", caps, nil, nil, nil, t.Context().Done())
	return emitter, sub, sent
}

func Test_canaryEmitter_reportsVersionAndCapabilities(t *testing.T) {
	caps := capabilities.NewMask(capabilities.RouterLinkGossip, capabilities.RouterMultiChannel)
	emitter, sub, sent := newTestCanaryEmitter(t, caps)
	sub.connected.Store(true)

	require.True(t, emitter.send())

	payload := &gossip_pb.CanaryPayload{}
	require.NoError(t, proto.Unmarshal((<-sent).Body, payload))
	require.Equal(t, "v2.1.0", payload.Version)
	require.Equal(t, caps.Bytes(), payload.Capabilities)
}

// Test_canaryEmitter_retriesFirstCanaryUntilItGoesOut covers a router whose subscription controller is not
// connected yet when it starts. Until its first canary arrives, controllers hold the canary of its previous
// lifetime, so that one goes out as soon as it can and the router only then falls back to the full interval.
func Test_canaryEmitter_retriesFirstCanaryUntilItGoesOut(t *testing.T) {
	emitter, sub, sent := newTestCanaryEmitter(t, capabilities.NewMask[capabilities.RouterCapability]())
	emitter.firstSendRetryInterval = 10 * time.Millisecond
	emitter.interval = time.Hour

	go emitter.run()

	time.Sleep(50 * time.Millisecond)
	require.Empty(t, sent, "nothing can be sent before the subscription controller connects")
	sub.connected.Store(true)

	select {
	case <-sent:
	case <-time.After(5 * time.Second):
		require.Fail(t, "the first canary was not retried once the subscription controller connected")
	}

	select {
	case <-sent:
		require.Fail(t, "a canary was sent at the retry interval after the first had gone out")
	case <-time.After(100 * time.Millisecond):
	}
}
