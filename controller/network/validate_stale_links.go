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
	"fmt"
	"sync"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v5/protobufs"
	"github.com/openziti/ziti/v2/common/capabilities"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/common/pb/mgmt_pb"
	"github.com/openziti/ziti/v2/controller/model"
)

// StaleLinkValidationCallback is invoked once per link in the controller's
// link map after aggregation completes.
type StaleLinkValidationCallback func(result *mgmt_pb.StaleLinkResult)

// ValidateStaleLinks fans out CheckStaleLinks to all routers matching
// filter, aggregates per-link verdicts from both endpoints, and emits a
// StaleLinkResult per link via cb.
//
// When gc is true, a link is removed only if all of the following hold, and
// each result reports whether removal actually happened via GcApplied, with a
// reason when it didn't:
//   - both endpoints reported stale; a partial response (one endpoint offline,
//     too old to answer, or timed out) never triggers GC
//   - both endpoints judged the same link iteration
//   - that iteration is still the current one when removal is attempted
//
// The iteration conditions matter because aggregation waits for every queried
// router, so a verdict can be up to the request timeout old by the time it is
// acted on, and a link id is reused across re-dials.
//
// Mirrors the (count, run, err) shape of ValidateLinks: caller invokes
// the returned func to actually fire off the async work.
func (n *Network) ValidateStaleLinks(filter string, mode mgmt_pb.StaleLinkMatchMode, gc bool, cb StaleLinkValidationCallback) (int64, func(), error) {
	// A protobuf enum field can carry a value this build doesn't know. Reject it
	// rather than falling back, since the fallback would be Changed — the
	// broadest removal criterion — and gc acts on it.
	if err := validateMatchMode(mode); err != nil {
		return 0, nil, err
	}

	routerResult, err := n.Router.BaseList(filter)
	if err != nil {
		return 0, nil, err
	}

	// Scope reported links to those involving a filtered router. The filter
	// selects which routers we query, so a link only belongs in the result if
	// at least one endpoint is in that set: links with neither endpoint
	// filtered would otherwise be reported with no verdicts, and a link with
	// only one filtered endpoint stays partial (so --gc, which needs both
	// sides to agree, leaves it alone).
	filteredRouterIds := make(map[string]struct{}, len(routerResult.Entities))
	for _, r := range routerResult.Entities {
		filteredRouterIds[r.Id] = struct{}{}
	}

	scopedLinks := scopeLinksToRouters(n.Link.GetLinkMap(), filteredRouterIds)
	expectedLinkCount := int64(len(scopedLinks))

	runF := func() {
		n.runStaleLinkValidation(routerResult.Entities, scopedLinks, mode, gc, cb)
	}

	return expectedLinkCount, runF, nil
}

// scopeLinksToRouters returns the subset of links that have at least one
// endpoint in routerIds. Used to limit stale-link validation and reporting to
// links that involve the filtered routers.
func scopeLinksToRouters(linkMap map[string]*model.Link, routerIds map[string]struct{}) map[string]*model.Link {
	scoped := make(map[string]*model.Link, len(linkMap))
	for id, link := range linkMap {
		_, srcFiltered := routerIds[link.Src.Id]
		_, dstFiltered := routerIds[link.DstId]
		if srcFiltered || dstFiltered {
			scoped[id] = link
		}
	}
	return scoped
}

// staleLinkQueryBlocker returns why a connected router can't answer a
// stale-link check, or "" when it can. A router without RouterStaleLinkCheck
// has no handler for the request and never replies, so asking it only burns the
// request timeout before the verdict lands as Unknown anyway.
func staleLinkQueryBlocker(router *model.Router) string {
	if router.HasCapability(capabilities.RouterStaleLinkCheck) {
		return ""
	}
	version := "unknown version"
	if router.VersionInfo != nil && router.VersionInfo.Version != "" {
		version = router.VersionInfo.Version
	}
	return fmt.Sprintf("router %s (%s) does not support stale-link checks", router.Id, version)
}

// appendUnqueryableReasons adds, for each endpoint of link that couldn't be
// asked, the reason why. Without this a link is reported partial with nothing
// distinguishing an offline peer from one too old to answer.
func appendUnqueryableReasons(reasons []string, unqueryable map[string]string, link *model.Link) []string {
	for _, routerId := range []string{link.Src.Id, link.DstId} {
		if reason, found := unqueryable[routerId]; found {
			reasons = append(reasons, reason)
		}
	}
	return reasons
}

// linkVerdicts holds the two sides' verdicts for one link.
type linkVerdicts struct {
	dialer   mgmt_pb.StaleVerdict
	listener mgmt_pb.StaleVerdict
	// dialerIteration and listenerIteration are the link incarnations each side
	// judged. GC needs them to agree with each other and with the controller's
	// current link before acting, since a link id outlives any one incarnation.
	dialerIteration   uint32
	listenerIteration uint32
	// reasons aggregates the human-readable explanations from whichever
	// side(s) reported stale, in the order the routers' responses arrived.
	reasons []string
}

// judgedIteration returns the incarnation both sides judged, and whether they
// agree. Disagreement means the link was re-dialed while the sweep was running,
// so neither verdict describes the link as it now stands.
func (v *linkVerdicts) judgedIteration() (uint32, bool) {
	if v.dialerIteration != v.listenerIteration {
		return 0, false
	}
	return v.dialerIteration, true
}

func (n *Network) runStaleLinkValidation(
	routers []*model.Router,
	linkMap map[string]*model.Link,
	mode mgmt_pb.StaleLinkMatchMode,
	gc bool,
	cb StaleLinkValidationCallback,
) {
	// Per-link verdicts keyed by linkId, populated as router responses
	// arrive.
	collector := newStaleLinkReportCollector(len(linkMap))

	// Fan out to routers in the filter set that can actually answer; collect
	// responses. Routers that can't are recorded with a reason instead, so the
	// partial verdicts they cause say what to fix.
	unqueryable := map[string]string{}
	var wg sync.WaitGroup
	for _, r := range routers {
		connected := n.GetConnectedRouter(r.Id)
		if connected == nil {
			unqueryable[r.Id] = fmt.Sprintf("router %s is not connected", r.Id)
			continue
		}
		if blocker := staleLinkQueryBlocker(connected); blocker != "" {
			unqueryable[r.Id] = blocker
			continue
		}
		wg.Add(1)
		go func(router *model.Router) {
			defer wg.Done()
			n.collectStaleReports(router, mode, collector.Record)
		}(connected)
	}
	wg.Wait()

	// Aggregate and emit one result per link. Emission order follows map
	// iteration, so it varies between runs; the client counts results rather
	// than relying on order.
	for linkId, link := range linkMap {
		v := collector.Get(linkId)
		stale, partial := aggregateVerdicts(v)
		result := &mgmt_pb.StaleLinkResult{
			LinkId:          linkId,
			SrcRouterId:     link.Src.Id,
			DstRouterId:     link.DstId,
			Stale:           stale,
			Partial:         partial,
			DialerVerdict:   v.dialer,
			ListenerVerdict: v.listener,
			Reasons:         appendUnqueryableReasons(v.reasons, unqueryable, link),
		}
		if gc && fullyConfirmedStale(v) {
			// Verdicts can be a minute old by now: aggregation waits for every
			// queried router. Remove only the incarnation that was actually
			// judged, so a link re-dialed in the meantime survives.
			if iteration, agreed := v.judgedIteration(); !agreed {
				result.Reasons = append(result.Reasons,
					"not removed: endpoints reported different link iterations, so the link changed mid-check")
			} else if !n.RemoveLinkAtIteration(linkId, iteration) {
				result.Reasons = append(result.Reasons,
					"not removed: link was re-dialed or removed after it was judged stale")
			} else {
				result.GcApplied = true
			}
		}
		cb(result)
	}
}

type staleLinkReportCollector struct {
	mu       sync.Mutex
	verdicts map[string]*linkVerdicts
}

func newStaleLinkReportCollector(capacity int) *staleLinkReportCollector {
	return &staleLinkReportCollector{
		verdicts: make(map[string]*linkVerdicts, capacity),
	}
}

func (c *staleLinkReportCollector) Record(report *ctrl_pb.LinkStaleReport) {
	c.mu.Lock()
	defer c.mu.Unlock()

	v, ok := c.verdicts[report.LinkId]
	if !ok {
		v = &linkVerdicts{}
		c.verdicts[report.LinkId] = v
	}
	applyStaleReport(v, report)
}

func (c *staleLinkReportCollector) Get(linkId string) *linkVerdicts {
	c.mu.Lock()
	defer c.mu.Unlock()

	v, ok := c.verdicts[linkId]
	if !ok {
		return &linkVerdicts{}
	}

	out := &linkVerdicts{
		dialer:            v.dialer,
		listener:          v.listener,
		dialerIteration:   v.dialerIteration,
		listenerIteration: v.listenerIteration,
	}
	if len(v.reasons) > 0 {
		out.reasons = append([]string(nil), v.reasons...)
	}
	return out
}

// collectStaleReports issues a single CheckStaleLinks to the router
// and routes each per-link report into the verdict collector.
// Router responses are independent, so this runs per-router.
func (n *Network) collectStaleReports(
	router *model.Router,
	mode mgmt_pb.StaleLinkMatchMode,
	record func(*ctrl_pb.LinkStaleReport),
) {
	log := pfxlog.Logger().WithField("routerId", router.Id)

	req := &ctrl_pb.CheckStaleLinksRequest{Mode: ctrlMode(mode)}
	resp := &ctrl_pb.CheckStaleLinksResponse{}
	respMsg, err := protobufs.MarshalTyped(req).
		WithTimeout(time.Minute).
		SendForReply(router.Control.GetDefaultSender())
	if err = protobufs.TypedResponse(resp).Unmarshall(respMsg, err); err != nil {
		log.WithError(err).Warn("CheckStaleLinks request failed; leaving this router's side as Unknown")
		return
	}
	if !resp.Success {
		log.WithField("message", resp.Message).Warn("router rejected CheckStaleLinks")
		return
	}

	for _, report := range resp.Reports {
		record(report)
	}
}

func applyStaleReport(v *linkVerdicts, report *ctrl_pb.LinkStaleReport) {
	var verdict mgmt_pb.StaleVerdict
	if report.Stale {
		verdict = mgmt_pb.StaleVerdict_StaleVerdictStale
	} else {
		verdict = mgmt_pb.StaleVerdict_StaleVerdictNotStale
	}
	switch report.Side {
	case ctrl_pb.StaleLinkSide_StaleLinkSideDialer:
		v.dialer = verdict
		v.dialerIteration = report.Iteration
	case ctrl_pb.StaleLinkSide_StaleLinkSideListener:
		v.listener = verdict
		v.listenerIteration = report.Iteration
	}
	if report.Stale && report.Reason != "" {
		v.reasons = append(v.reasons, report.Reason)
	}
}

// aggregateVerdicts decides if a link is stale overall and whether the
// decision is partial (only one endpoint reported).
//
// Stale rule: either endpoint reporting Stale flags the link.
// Partial rule: either endpoint with Unknown verdict.
func aggregateVerdicts(v *linkVerdicts) (stale bool, partial bool) {
	stale = v.dialer == mgmt_pb.StaleVerdict_StaleVerdictStale ||
		v.listener == mgmt_pb.StaleVerdict_StaleVerdictStale
	partial = v.dialer == mgmt_pb.StaleVerdict_StaleVerdictUnknown ||
		v.listener == mgmt_pb.StaleVerdict_StaleVerdictUnknown
	return
}

func fullyConfirmedStale(v *linkVerdicts) bool {
	return v.dialer == mgmt_pb.StaleVerdict_StaleVerdictStale &&
		v.listener == mgmt_pb.StaleVerdict_StaleVerdictStale
}

// validateMatchMode rejects a match mode this build doesn't declare. Callers
// run it before starting a sweep so an unrecognized value fails the request
// instead of resolving to a criterion the client didn't ask for.
func validateMatchMode(m mgmt_pb.StaleLinkMatchMode) error {
	switch m {
	case mgmt_pb.StaleLinkMatchMode_StaleLinkMatchChanged,
		mgmt_pb.StaleLinkMatchMode_StaleLinkMatchOrphaned:
		return nil
	default:
		return fmt.Errorf("unsupported stale link match mode %d", int32(m))
	}
}

// ctrlMode bridges mgmt_pb's match-mode enum to ctrl_pb's. The mgmt
// enum is duplicated so the mgmt-plane proto doesn't need to import
// ctrl_pb; this is the only translation point. Callers validate the mode
// first, so the default arm is only reached for an already-rejected value.
func ctrlMode(m mgmt_pb.StaleLinkMatchMode) ctrl_pb.StaleLinkMatchMode {
	if m == mgmt_pb.StaleLinkMatchMode_StaleLinkMatchOrphaned {
		return ctrl_pb.StaleLinkMatchMode_StaleLinkMatchOrphaned
	}
	return ctrl_pb.StaleLinkMatchMode_StaleLinkMatchChanged
}
