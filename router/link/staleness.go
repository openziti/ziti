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
	"fmt"

	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/router/xlink"
)

// StalenessMode mirrors the on-the-wire StaleLinkMatchMode enum in
// ctrl_pb. Kept as a small local type so the link package doesn't have
// to leak ctrl_pb into its public surface; conversion happens at the
// boundaries.
type StalenessMode int

const (
	// StalenessModeOrphaned: the link is stale only if its supporting
	// listener/dialer is entirely absent from current config.
	StalenessModeOrphaned StalenessMode = iota
	// StalenessModeChanged: the link is stale if any binding /
	// advertise / group detail no longer matches current config.
	StalenessModeChanged
)

// CheckDialerSide tests whether this router could still re-dial the
// link under its current configuration.
//
// Orphaned: re-dial is possible at all — there exists at least one
// (dialer, remote listener) pair where the dialer has the recorded
// binding, the listener has the recorded protocol, and their groups
// overlap. Catches binding-gone, protocol-gone, and group-divergence.
//
// Changed: orphaned's check, plus the compatible listener has the
// recorded ListenerBinding (so the re-dialed link's key would match
// the current one). Catches peer listener renames.
//
// destListeners is the registry-supplied snapshot of each destination's
// currently-advertised listener set.
func CheckDialerSide(
	link xlink.Xlink,
	dialers []xlink.Dialer,
	destListeners map[string][]*ctrl_pb.Listener,
	mode StalenessMode,
) (bool, string) {
	key := link.LinkKey()

	var matchingDialers []xlink.Dialer
	for _, d := range dialers {
		if d.GetBinding() == key.DialerBinding {
			matchingDialers = append(matchingDialers, d)
		}
	}
	if len(matchingDialers) == 0 {
		return true, fmt.Sprintf("no dialer with binding %q in current config", key.DialerBinding)
	}

	remoteListeners := destListeners[key.DestId]
	if len(remoteListeners) == 0 {
		return true, fmt.Sprintf("destination %q has no known listeners", key.DestId)
	}

	var hasCompatible, hasCompatibleWithBinding bool
	for _, rl := range remoteListeners {
		if rl.Protocol != key.Protocol {
			continue
		}
		groupsOverlap := false
		for _, d := range matchingDialers {
			if stringz.ContainsAny(rl.Groups, d.GetGroups()...) {
				groupsOverlap = true
				break
			}
		}
		if !groupsOverlap {
			continue
		}
		hasCompatible = true
		if rl.GetLocalBinding() == key.ListenerBinding {
			hasCompatibleWithBinding = true
		}
	}

	if !hasCompatible {
		return true, fmt.Sprintf(
			"no compatible (protocol %q + group-overlapping) listener on %q for dialer binding %q",
			key.Protocol, key.DestId, key.DialerBinding)
	}
	if mode == StalenessModeOrphaned {
		return false, ""
	}
	if !hasCompatibleWithBinding {
		return true, fmt.Sprintf(
			"no remote listener with binding %q on %q (a compatible listener exists with a different binding)",
			key.ListenerBinding, key.DestId)
	}
	return false, ""
}

// CheckListenerSide tests whether a listener that could have accepted
// this link still exists in this router's current listener set.
//
// Orphaned: any local listener of the same protocol still exists.
//
// Changed: a local listener with localBinding == LinkKey.ListenerBinding
// AND advertise == link.DialAddress() AND matching protocol. Group
// changes are caught on the dialer side via the snapshot lookup.
func CheckListenerSide(link xlink.Xlink, listeners []xlink.Listener, mode StalenessMode) (bool, string) {
	key := link.LinkKey()
	accepted := link.DialAddress() // On the accept side, DialAddress == this listener's advertise at accept time.

	if mode == StalenessModeOrphaned {
		for _, l := range listeners {
			if l.GetLinkProtocol() == key.Protocol {
				return false, ""
			}
		}
		return true, fmt.Sprintf("no listener with protocol %q in current config", key.Protocol)
	}

	for _, l := range listeners {
		if l.GetLinkProtocol() != key.Protocol {
			continue
		}
		if l.GetLocalBinding() != key.ListenerBinding {
			continue
		}
		if l.GetAdvertisement() == accepted {
			return false, ""
		}
	}
	return true, fmt.Sprintf(
		"no listener with binding %q advertising %q in current config",
		key.ListenerBinding, accepted)
}

// StalenessModeFromCtrl maps the wire enum to the link-package enum.
func StalenessModeFromCtrl(m ctrl_pb.StaleLinkMatchMode) StalenessMode {
	if m == ctrl_pb.StaleLinkMatchMode_StaleLinkMatchOrphaned {
		return StalenessModeOrphaned
	}
	return StalenessModeChanged
}

// StalenessModeFromGcMode maps the GcMode enum to the staleness mode.
// GcModePreserve has no staleness analog (no GC happens); callers
// should gate on preserve before invoking the staleness check.
func StalenessModeFromGcMode(m GcMode) StalenessMode {
	if m == GcModeOrphaned {
		return StalenessModeOrphaned
	}
	return StalenessModeChanged
}
