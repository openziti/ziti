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
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4/protobufs"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/common/pb/mgmt_pb"
	"github.com/openziti/ziti/v2/controller/model"
)

// StaleLinkValidationCallback is invoked once per link in the controller's
// link map after aggregation completes.
type StaleLinkValidationCallback func(result *mgmt_pb.StaleLinkResult)

// ValidateStaleLinks fans out CheckStaleLinks to all routers matching
// filter, aggregates per-link verdicts from both endpoints, and emits a
// StaleLinkResult per link via cb. When gc is true, the controller calls
// RemoveLink on each fully-confirmed-stale link (both endpoints reported
// stale). Partial responses (one endpoint offline / timed out) don't
// trigger GC even when gc=true — we want both sides to agree before
// breaking a link.
//
// Mirrors the (count, run, err) shape of ValidateLinks: caller invokes
// the returned func to actually fire off the async work.
func (n *Network) ValidateStaleLinks(filter string, mode mgmt_pb.StaleLinkMatchMode, gc bool, cb StaleLinkValidationCallback) (int64, func(), error) {
	routerResult, err := n.Router.BaseList(filter)
	if err != nil {
		return 0, nil, err
	}

	linkMap := n.Link.GetLinkMap()
	expectedLinkCount := int64(len(linkMap))

	runF := func() {
		n.runStaleLinkValidation(routerResult.Entities, linkMap, mode, gc, cb)
	}

	return expectedLinkCount, runF, nil
}

// linkVerdicts holds the two sides' verdicts for one link.
type linkVerdicts struct {
	dialer   mgmt_pb.StaleVerdict
	listener mgmt_pb.StaleVerdict
	// reasons aggregates the human-readable explanations from whichever
	// side(s) reported stale, in dialer-then-listener order.
	reasons []string
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

	// Fan out to connected routers in the filter set; collect responses.
	var wg sync.WaitGroup
	for _, r := range routers {
		connected := n.GetConnectedRouter(r.Id)
		if connected == nil {
			continue
		}
		wg.Add(1)
		go func(router *model.Router) {
			defer wg.Done()
			n.collectStaleReports(router, mode, collector.Record)
		}(connected)
	}
	wg.Wait()

	// Aggregate and emit per-link, in linkMap order for deterministic
	// output across runs.
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
			Reasons:         v.reasons,
		}
		if gc && fullyConfirmedStale(v) {
			n.RemoveLink(linkId)
			result.GcApplied = true
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
		dialer:   v.dialer,
		listener: v.listener,
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
	case ctrl_pb.StaleLinkSide_StaleLinkSideListener:
		v.listener = verdict
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

// ctrlMode bridges mgmt_pb's match-mode enum to ctrl_pb's. The mgmt
// enum is duplicated so the mgmt-plane proto doesn't need to import
// ctrl_pb; this is the only translation point.
func ctrlMode(m mgmt_pb.StaleLinkMatchMode) ctrl_pb.StaleLinkMatchMode {
	if m == mgmt_pb.StaleLinkMatchMode_StaleLinkMatchOrphaned {
		return ctrl_pb.StaleLinkMatchMode_StaleLinkMatchOrphaned
	}
	return ctrl_pb.StaleLinkMatchMode_StaleLinkMatchChanged
}
