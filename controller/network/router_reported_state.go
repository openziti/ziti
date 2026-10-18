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
	"bytes"
	"maps"
	"math"
	"math/big"
	"slices"
	"sync"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/ziti/v2/common/capabilities"
	"github.com/openziti/ziti/v2/controller/change"
	"github.com/openziti/ziti/v2/controller/model"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
)

const (
	// reportedStateSyncInterval is how often the leader writes the routers marked since its last pass.
	reportedStateSyncInterval = time.Second

	// reportedStateFullPassInterval is how often the leader re-checks every router, marked or not. It catches a
	// write this controller did not make, such as one from an older controller's hello path.
	reportedStateFullPassInterval = 5 * time.Minute

	// reportedStateFailureLogInterval bounds how often failed writes are logged while they keep failing.
	reportedStateFailureLogInterval = time.Minute

	reportedStateSourceType = "router.reported-state"
)

// routerReportedStates keeps each edge router's persisted version and capabilities matched to what the router
// reports. Only a caught-up raft leader writes, one router at a time, so the stored values follow one
// controller's view of each router rather than whichever controller's write commits last.
//
// A router that takes part in link gossip is judged from its canary, which reaches every controller through
// gossip whether or not the router is connected to it, and is ordered by the router's own canary sequence. A
// router that does not, and every router while the cluster cannot carry canaries, is judged from this
// controller's own connection to it. Such routers connect to every controller, the leader included.
//
// A router is marked when what it reports may have changed, and each pass writes the marked ones. A write that
// fails leaves its router marked for the next pass.
type routerReportedStates struct {
	network  *Network
	isLeader func() bool

	lock    sync.Mutex
	pending map[string]struct{}

	// Used only by pass.
	wasLeader      bool
	lastFullPass   time.Time
	lastFailureLog time.Time
}

func newRouterReportedStates(network *Network) *routerReportedStates {
	return &routerReportedStates{
		network:  network,
		isLeader: network.Dispatcher.IsLeaderAndCaughtUp,
		pending:  map[string]struct{}{},
	}
}

func (self *routerReportedStates) mark(routerId string) {
	self.lock.Lock()
	defer self.lock.Unlock()
	self.pending[routerId] = struct{}{}
}

// markAll marks every router this controller holds a connection or a canary for.
func (self *routerReportedStates) markAll() {
	var routerIds []string
	for _, r := range self.network.AllConnectedRouters() {
		routerIds = append(routerIds, r.Id)
	}
	routerIds = append(routerIds, self.network.canaryListener.routerIds()...)

	self.lock.Lock()
	defer self.lock.Unlock()
	for _, routerId := range routerIds {
		self.pending[routerId] = struct{}{}
	}
}

func (self *routerReportedStates) forget(routerId string) {
	self.lock.Lock()
	defer self.lock.Unlock()
	delete(self.pending, routerId)
}

func (self *routerReportedStates) takePending() []string {
	self.lock.Lock()
	pending := self.pending
	self.pending = map[string]struct{}{}
	self.lock.Unlock()

	return slices.Collect(maps.Keys(pending))
}

// RouterConnected implements model.RouterPresenceHandler.
func (self *routerReportedStates) RouterConnected(r *model.Router) {
	self.mark(r.Id)
}

// RouterDisconnected implements model.RouterPresenceHandler. A router this controller loses a connection to may
// now be judged from its canary instead.
func (self *routerReportedStates) RouterDisconnected(r *model.Router) {
	self.mark(r.Id)
}

// canaryChanged marks the router when its canary reports a different version or capabilities than the one it
// replaced. prev is nil for a router with no previous canary. It runs on the gossip apply pool, so it only marks.
func (self *routerReportedStates) canaryChanged(routerId string, prev, value *CanaryValue) {
	if prev == nil || prev.Version != value.Version || !bytes.Equal(prev.Capabilities, value.Capabilities) {
		self.mark(routerId)
	}
}

func (self *routerReportedStates) run() {
	ticker := time.NewTicker(reportedStateSyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			self.pass()
		case <-self.network.closeNotify:
			return
		}
	}
}

// pass writes the marked routers if this controller is a caught-up leader, first marking every router when it
// has just become one or a full-pass interval has gone by. Not safe to call concurrently.
func (self *routerReportedStates) pass() {
	if !self.isLeader() {
		self.wasLeader = false
		return
	}
	if !self.wasLeader || time.Since(self.lastFullPass) >= reportedStateFullPassInterval {
		self.wasLeader = true
		self.lastFullPass = time.Now()
		self.markAll()
	}

	pending := self.takePending()
	failed := 0
	var lastErr error
	for _, routerId := range pending {
		// Per write rather than per pass: a write sent after losing leadership is forwarded to a leader with its
		// own view, and one that becomes leader starts with a pass over every router anyway.
		if !self.isLeader() {
			self.wasLeader = false
			return
		}
		if err := self.write(routerId); err != nil {
			failed++
			lastErr = err
			self.mark(routerId)
		}
	}

	if failed > 0 && time.Since(self.lastFailureLog) >= reportedStateFailureLogInterval {
		self.lastFailureLog = time.Now()
		pfxlog.Logger().WithError(lastErr).WithField("failed", failed).WithField("attempted", len(pending)).
			Warn("failed to persist reported version and capabilities for some routers, will retry")
	}
}

// write persists what the router reports. A router this controller has no report for, or that is not an edge
// router, is left alone.
func (self *routerReportedStates) write(routerId string) error {
	caps, version, ok := self.reported(routerId)
	if !ok {
		return nil
	}

	ctx := change.New().
		SetSourceType(reportedStateSourceType).
		SetChangeAuthorType(change.AuthorTypeRouter).
		SetChangeAuthorId(routerId)

	err := self.network.EdgeRouter.UpdateRouterReportedState(routerId, persistedCapabilitiesMask(routerId, caps), version, ctx)
	if boltz.IsErrNotFoundErr(err) {
		return nil
	}
	return err
}

// reported returns the capabilities and version the router reports, chosen as routerReportedStates describes,
// and false when this controller has neither a connection to it nor a canary with a report. A gossiping router
// whose canary carries no report is judged from the connection, if there is one.
func (self *routerReportedStates) reported(routerId string) (*capabilities.RouterCapabilityMask, string, bool) {
	conn := self.network.Router.GetConnected(routerId)
	if conn != nil && (!conn.Capabilities.IsSet(capabilities.RouterLinkGossip) ||
		!self.network.GossipStore.IsAvailable(CanaryGossipStoreType)) {
		return connectionReport(conn)
	}

	if canary, found := self.network.GetCanaryValueForRouter(routerId); found && canary.Version != "" {
		return capabilities.MaskFromBytes[capabilities.RouterCapability](canary.Capabilities), canary.Version, true
	}

	if conn != nil {
		return connectionReport(conn)
	}
	return nil, "", false
}

func connectionReport(r *model.Router) (*capabilities.RouterCapabilityMask, string, bool) {
	version := ""
	if r.VersionInfo != nil {
		version = r.VersionInfo.Version
	}
	return r.Capabilities, version, true
}

// persistedCapabilitiesMask returns caps as the int64 the edge router entity stores. Only bit positions 0-62
// fit, so higher bits are dropped, with a warning, rather than stored as garbage. A nil mask is 0.
func persistedCapabilitiesMask(routerId string, caps *capabilities.RouterCapabilityMask) int64 {
	if caps == nil {
		return 0
	}
	bits := new(big.Int).SetBytes(caps.Bytes())
	if bits.BitLen() > 63 {
		pfxlog.Logger().WithField("routerId", routerId).
			Warnf("router capabilities mask uses %d bits; only bits 0-62 are persisted", bits.BitLen())
		bits = new(big.Int).And(bits, new(big.Int).SetInt64(math.MaxInt64))
	}
	return bits.Int64()
}
