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

package link

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/router/xlink"
)

// XlinkEnv is the subset of router env the auto-GC routine needs. Lets
// the routine be tested without spinning a full RouterEnv.
type XlinkEnv interface {
	GetXlinkRegistry() xlink.Registry
	GetXlinkListeners() []xlink.Listener
	GetXlinkDialers() []xlink.Dialer
}

// RunStaleLinkGc walks every established xlink and closes those that
// the configured mode considers stale relative to this router's
// current configuration. The router's CheckDialerSide /
// CheckListenerSide drive the verdict per link.
//
// One-sided action: the router acts on its own verdict rather than
// requiring the peer to agree. If the local side can no longer support
// the link, the peer was going to see it disconnect on its next
// re-establish attempt anyway. Operators who want a two-sided,
// controller-aggregated sweep use `ziti ops verify stale-links --gc`.
//
// Returns the link ids that were closed. Errors from individual
// link.Close calls are logged, not returned, so one bad link doesn't
// stop the sweep.
func RunStaleLinkGc(env XlinkEnv, mode GcMode) []string {
	if mode == GcModePreserve {
		return nil
	}
	registry := env.GetXlinkRegistry()
	if registry == nil {
		return nil
	}
	listeners := env.GetXlinkListeners()
	dialers := env.GetXlinkDialers()
	destListeners, ok := registry.GetDestinationListeners()
	if !ok {
		// An indeterminate snapshot (registry event loop busy/stuck) would
		// make every dialer-side link look orphaned. Skip this sweep rather
		// than close healthy links; GC re-triggers on the next config change.
		pfxlog.Logger().Warn("auto-GC: skipping, could not obtain destination listener snapshot")
		return nil
	}
	stalenessMode := StalenessModeFromGcMode(mode)

	var closed []string
	for xl := range registry.Iter() {
		if xl.IsClosed() {
			continue
		}
		stale, reason, side := evaluateStaleness(xl, dialers, listeners, destListeners, stalenessMode)
		if !stale {
			continue
		}
		log := pfxlog.Logger().
			WithField("linkId", xl.Id()).
			WithField("linkKey", xl.Key()).
			WithField("side", side).
			WithField("mode", mode.String()).
			WithField("reason", reason)
		if err := xl.Close(); err != nil {
			log.WithError(err).Warn("auto-GC: error closing stale link")
			continue
		}
		log.Info("auto-GC: closed stale link")
		closed = append(closed, xl.Id())
	}
	return closed
}

// evaluateStaleness dispatches to the right side-specific check and
// returns the verdict along with a human-readable side label for logs.
func evaluateStaleness(
	xl xlink.Xlink,
	dialers []xlink.Dialer,
	listeners []xlink.Listener,
	destListeners map[string][]*ctrl_pb.Listener,
	mode StalenessMode,
) (bool, string, string) {
	if xl.IsDialed() {
		stale, reason := CheckDialerSide(xl, dialers, destListeners, mode)
		return stale, reason, "dialer"
	}
	stale, reason := CheckListenerSide(xl, listeners, mode)
	return stale, reason, "listener"
}
