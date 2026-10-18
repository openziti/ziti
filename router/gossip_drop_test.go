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
	"testing"

	"github.com/openziti/channel/v5"
	"google.golang.org/protobuf/proto"

	"github.com/openziti/ziti/v2/common/capabilities"
	"github.com/openziti/ziti/v2/common/pb/gossip_pb"
	"github.com/openziti/ziti/v2/controller/idgen"
	"github.com/openziti/ziti/v2/router/env"
	"github.com/stretchr/testify/require"
)

// stubCtrls embeds the interface so the rest is satisfied without implementing it; only the gossip answer
// is read here.
type stubCtrls struct {
	env.NetworkControllers
	usesGossip bool
	// perCapability overrides usesGossip when set, for the mixed case where the cluster carries one gossip
	// feature but not another.
	perCapability map[capabilities.ControllerCapability]bool
}

func (s *stubCtrls) UsesGossip(capability capabilities.ControllerCapability) bool {
	if s.perCapability != nil {
		return s.perCapability[capability]
	}
	return s.usesGossip
}

// deltaChannel records what is sent on it, so a test can assert nothing was.
type deltaChannel struct {
	channel.Channel
	sent        []*gossip_pb.GossipDigestResponse
	closeNotify chan struct{}
}

func newDeltaChannel() *deltaChannel {
	return &deltaChannel{closeNotify: make(chan struct{})}
}

func (c *deltaChannel) Send(s channel.Sendable) error {
	resp := &gossip_pb.GossipDigestResponse{}
	if err := proto.Unmarshal(s.Msg().Body, resp); err != nil {
		return err
	}
	c.sent = append(c.sent, resp)
	s.SendListener().NotifyAfterWrite()
	return nil
}

func (c *deltaChannel) IsClosed() bool               { return false }
func (c *deltaChannel) CloseNotify() <-chan struct{} { return c.closeNotify }

func (c *deltaChannel) entries() []*gossip_pb.GossipEntry {
	var all []*gossip_pb.GossipEntry
	for _, resp := range c.sent {
		all = append(all, resp.Entries...)
	}
	return all
}

// newDropTestClient returns a client holding one store with the given live entry keys advertised.
func newDropTestClient(ctrls env.NetworkControllers, keys ...string) (*gossipClient, *gossipStore) {
	store := newGossipStore(&stubGossipSource{name: linkGossipStoreType})
	g := &gossipClient{
		routerId: "r1",
		epoch:    idgen.NewEpochBytes(),
		ctrls:    ctrls,
		stores:   map[string]*gossipStore{linkGossipStoreType: store},
	}
	// Versioned from the client's own clock, as a real advertised entry is.
	for _, key := range keys {
		store.currentEntries.Set(key, &gossip_pb.GossipEntry{
			Key: key, Version: g.nextVersion(), Owner: "r1", Epoch: g.epoch,
		})
	}
	return g, store
}

// Test_dropAdvertisedEntries_KeepsThemWhileGossiping: the advertised set is exactly what should be
// advertised while this router is a gossip publisher.
func Test_dropAdvertisedEntries_KeepsThemWhileGossiping(t *testing.T) {
	req := require.New(t)

	g, store := newDropTestClient(&stubCtrls{usesGossip: true}, "link1:1", "link2:1")
	g.dropAdvertisedEntries()

	req.Equal(2, store.currentEntries.Count(), "a gossip publisher must keep advertising")
}

// Test_dropAdvertisedEntries_ForgetsThemOnceNotGossiping covers what a router must not carry across the
// switch to reporting links the pre-gossip way: entries describing state it no longer maintains, which a
// digest would otherwise carry back into a store that starts empty when the cluster can gossip again.
//
// Nothing is sent. Every controller that could hold an entry gave up its store on reaching the same
// conclusion about the cluster, so there is nobody left to tell.
func Test_dropAdvertisedEntries_ForgetsThemOnceNotGossiping(t *testing.T) {
	req := require.New(t)

	g, store := newDropTestClient(&stubCtrls{usesGossip: false}, "link1:1", "link2:1")
	g.dropAdvertisedEntries()

	req.Zero(store.currentEntries.Count(), "a router that is not a gossip publisher advertises nothing")
}

// Test_dropUnchanged_KeepsAnEntryRepublishedBetweenReadAndDrop covers the collision between dropping and
// publishing, which run on different goroutines and each read their own answer. Removing by key alone
// forgot a link advertised in between, leaving the router holding one it no longer advertised, with the
// link registry already having marked it notified and nothing local left to resend it.
//
// The two halves are driven separately here because that gap is the whole window: there is no send between
// them to hang a hook on.
func Test_dropUnchanged_KeepsAnEntryRepublishedBetweenReadAndDrop(t *testing.T) {
	req := require.New(t)

	g, store := newDropTestClient(&stubCtrls{usesGossip: false}, "link1:1", "link2:1")

	advertised := store.collectAdvertised()
	req.Len(advertised, 2)

	// Publishing turns back on and re-advertises link1 at a fresh version, after the read.
	republished := &gossip_pb.GossipEntry{
		Key: "link1:1", Version: g.nextVersion(), Owner: "r1", Epoch: g.epoch,
	}
	store.currentEntries.Set("link1:1", republished)

	dropped, kept := store.dropUnchanged(advertised)
	req.Equal(1, dropped)
	req.Equal(1, kept)

	current, ok := store.currentEntries.Get("link1:1")
	req.True(ok, "a link republished after the read must stay advertised")
	req.Equal(republished.Version, current.Version)

	_, ok = store.currentEntries.Get("link2:1")
	req.False(ok, "an entry that did not change must be dropped")
}

// Test_collectAdvertised_SkipsTombstones: a tombstone is not something this router advertises, so dropping
// it would be a no-op that still counted.
func Test_collectAdvertised_SkipsTombstones(t *testing.T) {
	req := require.New(t)

	g, store := newDropTestClient(&stubCtrls{usesGossip: false}, "link1:1")
	store.currentEntries.Set("link2:1", &gossip_pb.GossipEntry{
		Key: "link2:1", Version: g.nextVersion(), Owner: "r1", Epoch: g.epoch, Tombstone: true,
	})

	advertised := store.collectAdvertised()
	req.Len(advertised, 1)
	req.Contains(advertised, "link1:1")
}

// Test_dropAdvertisedEntries_ForgetsOnlyTheStoreThatLostItsCapability is why the drop is per store rather
// than once for the client. Gossip features arrive in different releases, so a cluster part-way through an
// upgrade can carry one while not yet carrying the next, and giving up both would stop publishing something
// that still works.
func Test_dropAdvertisedEntries_ForgetsOnlyTheStoreThatLostItsCapability(t *testing.T) {
	req := require.New(t)

	// ControllerGroupedCtrlChan stands in for a capability a later release adds.
	ctrls := &stubCtrls{perCapability: map[capabilities.ControllerCapability]bool{
		capabilities.ControllerLinkGossip:      true,
		capabilities.ControllerGroupedCtrlChan: false,
	}}

	g, carried := newDropTestClient(ctrls, "link1:1")
	dropped := newGossipStore(&stubGossipSource{
		name:       "next",
		capability: capabilities.ControllerGroupedCtrlChan,
	})
	dropped.currentEntries.Set("next1:1", &gossip_pb.GossipEntry{
		Key: "next1:1", Version: g.nextVersion(), Owner: "r1", Epoch: g.epoch,
	})
	g.stores["next"] = dropped

	g.dropAdvertisedEntries()

	req.Equal(1, carried.currentEntries.Count(),
		"a feature the cluster still carries must keep being advertised")
	req.Zero(dropped.currentEntries.Count(),
		"only the store whose capability the cluster cannot carry may be forgotten")
}

// Test_handleDigest_IgnoredWhileNotPublishing: the exchange answers a digest by tombstoning whatever the
// controller holds that this router does not. A router that has just forgotten its advertised set holds
// nothing, so answering would delete the controller's entries rather than leave them to be given up on the
// controller's own reckoning.
func Test_handleDigest_IgnoredWhileNotPublishing(t *testing.T) {
	req := require.New(t)

	ctrls := &stubCtrls{usesGossip: false}
	g, store := newDropTestClient(ctrls, "link1:1")
	g.dropAdvertisedEntries()
	req.Zero(store.currentEntries.Count())

	ch := newDeltaChannel()
	g.handleDigest(&gossip_pb.GossipDigest{
		StoreType: linkGossipStoreType,
		Entries: []*gossip_pb.DigestEntry{
			{Key: "link1:1", Version: 5},
		},
	}, ch)

	req.Empty(ch.entries(), "a router not publishing for a capability must not retire entries for it")
}
