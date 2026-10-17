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

package raft

import (
	"testing"
	"time"

	"github.com/hashicorp/raft"
	"github.com/openziti/ziti/v2/common/capabilities"
	"github.com/stretchr/testify/require"
)

func mask(caps ...capabilities.ControllerCapability) *capabilities.Mask[capabilities.ControllerCapability] {
	return capabilities.NewMask(caps...)
}

// maskOf asserts the cluster answer is known and returns it.
func maskOf(t *testing.T, tracker *clusterCapabilityTracker, members []raft.ServerID) *capabilities.Mask[capabilities.ControllerCapability] {
	t.Helper()
	m, known := tracker.clusterMask(members)
	require.True(t, known, "the answer should be known for %v", members)
	return m
}

// Test_clusterCapabilityTracker_RemembersAMemberThatIsNotConnected is the reason this is a tracker rather
// than a read of the mesh's peer map. That map holds only connected peers, so a controller partitioned from
// an older peer would read the cluster as uniform and turn a cluster-wide feature on. What a member last
// advertised holds until it reconnects, since it cannot change what it advertises without doing so.
func Test_clusterCapabilityTracker_RemembersAMemberThatIsNotConnected(t *testing.T) {
	req := require.New(t)

	tracker := newClusterCapabilityTracker("self")
	members := []raft.ServerID{"self", "old", "new"}

	tracker.observe("old", mask())
	tracker.observe("new", mask(capabilities.ControllerLinkGossip))
	req.False(maskOf(t, tracker, members).IsSet(capabilities.ControllerLinkGossip),
		"a member without the capability must hold it off")

	// "old" drops off the mesh; nothing re-observes it. Its last answer has to stand, and the answer stays
	// known: it was observed once, and a member cannot change what it advertises without reconnecting.
	req.False(maskOf(t, tracker, members).IsSet(capabilities.ControllerLinkGossip),
		"a member losing its connection must not make the cluster look more capable")

	// It comes back upgraded, which is the only way its answer can change.
	tracker.observe("old", mask(capabilities.ControllerLinkGossip))
	req.True(maskOf(t, tracker, members).IsSet(capabilities.ControllerLinkGossip))
}

// Test_clusterCapabilityTracker_ReportsEachCapabilitySeparately is what a store-wide answer could not do. A
// cluster part-way through an upgrade supports what the older release had while not yet supporting what the
// newer one adds, and collapsing that to one answer would switch a shipped feature off during the upgrade
// after the one that shipped it.
func Test_clusterCapabilityTracker_ReportsEachCapabilitySeparately(t *testing.T) {
	req := require.New(t)

	// ControllerGroupedCtrlChan stands in for a capability a later release adds: one member has it, one
	// does not, while both have link gossip.
	tracker := newClusterCapabilityTracker("self")
	tracker.observe("self", mask(capabilities.ControllerLinkGossip, capabilities.ControllerGroupedCtrlChan))
	tracker.observe("older", mask(capabilities.ControllerLinkGossip))

	clusterMask := maskOf(t, tracker, []raft.ServerID{"self", "older"})
	req.True(clusterMask.IsSet(capabilities.ControllerLinkGossip),
		"a capability every member has must survive the upgrade window")
	req.False(clusterMask.IsSet(capabilities.ControllerGroupedCtrlChan),
		"a capability only some members have must not be reported")
}

// Test_clusterCapabilityTracker_UnseenMemberLeavesTheAnswerUnknown is the difference that keeps a restart
// from becoming a fleet-wide flap. A controller that has just restarted has observed only itself, and
// reporting "supports nothing" there would tell every router connected to it that the cluster cannot carry
// a feature its peers are still carrying. Unknown means say nothing, so routers keep the last answer.
func Test_clusterCapabilityTracker_UnseenMemberLeavesTheAnswerUnknown(t *testing.T) {
	req := require.New(t)

	tracker := newClusterCapabilityTracker("self")
	req.True(maskOf(t, tracker, []raft.ServerID{"self"}).IsSet(capabilities.ControllerLinkGossip),
		"this controller's own capabilities are known by construction")

	_, known := tracker.clusterMask([]raft.ServerID{"self", "never-seen"})
	req.False(known, "a member never observed leaves the answer unknown, not unsupported")

	tracker.observe("never-seen", mask(capabilities.ControllerLinkGossip))
	req.True(maskOf(t, tracker, []raft.ServerID{"self", "never-seen"}).IsSet(capabilities.ControllerLinkGossip),
		"the first connection settles it")
}

// Test_clusterCapabilityTracker_AbsentCapabilitiesAreKnownAndUnsupported: a peer predating capability
// exchange between controllers connects and sends no mask. That is an answer, unlike never having been
// seen: the member is known not to support anything.
func Test_clusterCapabilityTracker_AbsentCapabilitiesAreKnownAndUnsupported(t *testing.T) {
	req := require.New(t)

	tracker := newClusterCapabilityTracker("self")
	tracker.observe("silent", nil)
	req.False(maskOf(t, tracker, []raft.ServerID{"self", "silent"}).IsSet(capabilities.ControllerLinkGossip))

	tracker.observe("empty", mask())
	req.False(maskOf(t, tracker, []raft.ServerID{"self", "empty"}).IsSet(capabilities.ControllerLinkGossip))
}

// Test_clusterCapabilityTracker_UnknownMembershipIsUnknown: an empty membership means the raft
// configuration could not be read, not that the cluster is trivially uniform.
func Test_clusterCapabilityTracker_UnknownMembershipIsUnknown(t *testing.T) {
	req := require.New(t)

	tracker := newClusterCapabilityTracker("self")
	_, known := tracker.clusterMask(nil)
	req.False(known)
	_, known = tracker.clusterMask([]raft.ServerID{})
	req.False(known)
}

// Test_clusterCapabilityTracker_IgnoresNonMembers: a former member still in the tracker must not affect the
// answer, so that removing an old controller from the cluster enables a feature rather than blocking it
// forever.
func Test_clusterCapabilityTracker_IgnoresNonMembers(t *testing.T) {
	req := require.New(t)

	tracker := newClusterCapabilityTracker("self")
	tracker.observe("departed", mask())
	tracker.observe("peer", mask(capabilities.ControllerLinkGossip))

	req.True(maskOf(t, tracker, []raft.ServerID{"self", "peer"}).IsSet(capabilities.ControllerLinkGossip),
		"a controller no longer in the cluster must not hold a capability off")
}

// Test_reportClusterCapabilities_ReportsOnlyChanges: the check is requested on every peer connection, peer
// drop and membership change, so most evaluations find nothing new. A consumer gives up a gossip store when
// a capability stops holding, so repeating an answer would throw away a store that is still in use.
func Test_reportClusterCapabilities_ReportsOnlyChanges(t *testing.T) {
	req := require.New(t)

	c := &Controller{clusterCapabilityCheck: make(chan struct{}, 1)}

	var reported []bool
	c.RegisterClusterCapabilityHandler(func(m *capabilities.Mask[capabilities.ControllerCapability]) {
		reported = append(reported, m.IsSet(capabilities.ControllerLinkGossip))
	})
	req.Empty(reported, "a handler must not be told anything while the answer is unknown")

	c.reportClusterCapabilities(mask(capabilities.ControllerLinkGossip))
	c.reportClusterCapabilities(mask(capabilities.ControllerLinkGossip))
	req.Equal([]bool{true}, reported, "only a change may be reported")

	c.reportClusterCapabilities(mask())
	req.Equal([]bool{true, false}, reported)
}

// Test_RegisterClusterCapabilityHandler_TellsANewHandlerTheCurrentAnswer: a store that began by assuming
// its feature was usable would accept entries it is about to give up.
func Test_RegisterClusterCapabilityHandler_TellsANewHandlerTheCurrentAnswer(t *testing.T) {
	req := require.New(t)

	c := &Controller{clusterCapabilityCheck: make(chan struct{}, 1)}
	c.reportClusterCapabilities(mask(capabilities.ControllerLinkGossip))

	var reported []bool
	c.RegisterClusterCapabilityHandler(func(m *capabilities.Mask[capabilities.ControllerCapability]) {
		reported = append(reported, m.IsSet(capabilities.ControllerLinkGossip))
	})
	req.Equal([]bool{true}, reported)
}

// Test_evaluateClusterCapabilities_SaysNothingWhileUnknown is what keeps a restarted controller from
// telling its routers the cluster supports nothing. With no raft the answer cannot be derived at all, which
// stands in for a controller that has not yet observed its peers.
func Test_evaluateClusterCapabilities_SaysNothingWhileUnknown(t *testing.T) {
	req := require.New(t)

	c := &Controller{clusterCapabilityCheck: make(chan struct{}, 1)}
	c.reportClusterCapabilities(mask(capabilities.ControllerLinkGossip))

	var reported []bool
	c.RegisterClusterCapabilityHandler(func(m *capabilities.Mask[capabilities.ControllerCapability]) {
		reported = append(reported, m.IsSet(capabilities.ControllerLinkGossip))
	})
	req.Equal([]bool{true}, reported)

	c.evaluateClusterCapabilities()
	c.evaluateClusterCapabilities()

	req.Equal([]bool{true}, reported, "an unknown answer must leave the last one standing")
	req.True(c.clusterCapabilities.Load().IsSet(capabilities.ControllerLinkGossip),
		"and must not overwrite it")
}

// Test_requestClusterCapabilityCheck_NeverBlocks is what makes it safe to call from the mesh peer observer,
// which runs while the mesh holds its lock. A blocking send there would hold that lock for as long as the
// event loop took to drain.
func Test_requestClusterCapabilityCheck_NeverBlocks(t *testing.T) {
	req := require.New(t)

	c := &Controller{clusterCapabilityCheck: make(chan struct{}, 1)}

	done := make(chan struct{})
	go func() {
		defer close(done)
		// Far more than the channel can hold, so all but the first find a signal already pending.
		for i := 0; i < 1000; i++ {
			c.requestClusterCapabilityCheck()
		}
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		req.Fail("requesting a check must never block, however many are outstanding")
	}

	req.Len(c.clusterCapabilityCheck, 1, "signals must coalesce rather than queue")
}
