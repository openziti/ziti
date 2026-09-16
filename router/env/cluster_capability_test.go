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

package env

import (
	"testing"

	"github.com/openziti/channel/v5"
	"github.com/openziti/ziti/v2/common/capabilities"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/stretchr/testify/require"
)

func clusterStatus(mask *capabilities.Mask[capabilities.ControllerCapability]) *channel.Message {
	m := channel.NewMessage(int32(ctrl_pb.ContentType_CanaryStatusType), nil)
	m.Headers[int32(ctrl_pb.ControlHeaders_ClusterCapabilitiesHeader)] = mask.Bytes()
	return m
}

// Test_ClusterSupportsCapability_DefaultsToSupported covers the standalone controller, which has no peers
// that could fail to take part and so never reports otherwise. Assuming nothing is supported until told
// would leave such a deployment unable to gossip at all.
func Test_ClusterSupportsCapability_DefaultsToSupported(t *testing.T) {
	req := require.New(t)

	ctrls := &networkControllers{}
	req.True(ctrls.ClusterSupportsCapability(capabilities.ControllerLinkGossip),
		"a router must not assume a capability is missing before anything says so")
}

// Test_ClusterSupportsCapability_TakesWhatTheControllerReports is how a router learns about a controller it
// cannot see. Its own connections can all support link gossip while a cluster member it never connects to
// does not, and only the controllers can tell it that.
func Test_ClusterSupportsCapability_TakesWhatTheControllerReports(t *testing.T) {
	req := require.New(t)

	ctrls := &networkControllers{}

	ctrls.handleCanaryStatusUpdate(clusterStatus(capabilities.NewMask[capabilities.ControllerCapability]()), nil)
	req.False(ctrls.ClusterSupportsCapability(capabilities.ControllerLinkGossip),
		"a cluster that cannot carry the feature must be believed")

	ctrls.handleCanaryStatusUpdate(clusterStatus(capabilities.NewMask(capabilities.ControllerLinkGossip)), nil)
	req.True(ctrls.ClusterSupportsCapability(capabilities.ControllerLinkGossip),
		"the answer must be a state, not a latch, or an upgrade never restores gossip")
}

// Test_ClusterSupportsCapability_ReportsEachCapabilitySeparately: gossip features arrive in different
// releases, so a router must be able to keep using one while the cluster cannot yet carry another.
func Test_ClusterSupportsCapability_ReportsEachCapabilitySeparately(t *testing.T) {
	req := require.New(t)

	ctrls := &networkControllers{}
	// ControllerGroupedCtrlChan stands in for a capability a later release adds.
	ctrls.handleCanaryStatusUpdate(clusterStatus(capabilities.NewMask(capabilities.ControllerLinkGossip)), nil)

	req.True(ctrls.ClusterSupportsCapability(capabilities.ControllerLinkGossip))
	req.False(ctrls.ClusterSupportsCapability(capabilities.ControllerGroupedCtrlChan))
}

// Test_ClusterSupportsCapability_IgnoresAStatusWithoutTheHeader: a controller predating the header sends
// canary status without it, and that must not read as a cluster supporting nothing.
func Test_ClusterSupportsCapability_IgnoresAStatusWithoutTheHeader(t *testing.T) {
	req := require.New(t)

	ctrls := &networkControllers{}
	ctrls.handleCanaryStatusUpdate(clusterStatus(capabilities.NewMask(capabilities.ControllerLinkGossip)), nil)

	ctrls.handleCanaryStatusUpdate(channel.NewMessage(int32(ctrl_pb.ContentType_CanaryStatusType), nil), nil)
	req.True(ctrls.ClusterSupportsCapability(capabilities.ControllerLinkGossip),
		"a status with no header must leave the last answer alone")
}

// Test_UsesGossip_NeedsBothAnswers is the whole point of there being two. They answer different questions,
// and each is the only party that can answer its own.
//
// The cluster answer says whether the feature can be carried at all: a member this router never connects to
// still has to take part, and only the controllers can see it. The router's own view says whether
// everything it is talking to can hear it, which only the router can see. Dropping either one publishes
// into somewhere that cannot receive it.
func Test_UsesGossip_NeedsBothAnswers(t *testing.T) {
	req := require.New(t)

	for _, tc := range []struct {
		name           string
		connectedAgree bool
		clusterAgrees  bool
		expected       bool
	}{
		{"both agree", true, true, true},
		{"a connected controller cannot gossip", false, true, false},
		{"a cluster member this router cannot see cannot gossip", true, false, false},
		{"neither", false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrls := &networkControllers{}
			if !tc.clusterAgrees {
				ctrls.handleCanaryStatusUpdate(
					clusterStatus(capabilities.NewMask[capabilities.ControllerCapability]()), nil)
			}

			// A connected controller that does not advertise the capability is what makes the router's own
			// fold disagree.
			if !tc.connectedAgree {
				ctrls.ctrls.Put("old", &networkCtrl{caps: capabilities.NewMask[capabilities.ControllerCapability]()})
			} else {
				ctrls.ctrls.Put("new", &networkCtrl{caps: capabilities.NewMask(capabilities.ControllerLinkGossip)})
			}

			req.Equal(tc.expected, ctrls.UsesGossip(capabilities.ControllerLinkGossip))
		})
	}
}
