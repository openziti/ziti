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

package main

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/channel/v4/protobufs"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/common/pb/mgmt_pb"
	"github.com/openziti/ziti/controller/rest_client/link"
	"github.com/openziti/ziti/controller/rest_model"
	"github.com/openziti/ziti/zitirest"
	"github.com/openziti/ziti/zititest/zitilab/chaos"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

func sowChaos(run model.Run) error {
	controllers, err := chaos.SelectRandom(run, ".ctrl", chaos.RandomOfTotal())
	if err != nil {
		return err
	}
	time.Sleep(5 * time.Second)
	routers, err := chaos.SelectRandom(run, ".router", chaos.PercentageRange(10, 75))
	if err != nil {
		return err
	}
	toRestart := append(routers, controllers...)
	fmt.Printf("restarting %v controllers and %v routers\n", len(controllers), len(routers))
	return chaos.RestartSelected(run, 100, toRestart...)
}

func validateLinks(run model.Run) error {
	ctrls := run.GetModel().SelectComponents(".ctrl")
	errC := make(chan error, len(ctrls))
	deadline := time.Now().Add(15 * time.Minute)
	for _, ctrl := range ctrls {
		ctrlComponent := ctrl
		go validateLinksForCtrlWithChan(run, ctrlComponent, deadline, errC)
	}

	for i := 0; i < len(ctrls); i++ {
		err := <-errC
		if err != nil {
			return err
		}
	}

	return nil
}

func validateLinksForCtrlWithChan(run model.Run, c *model.Component, deadline time.Time, errC chan<- error) {
	errC <- validateLinksForCtrl(run, c, deadline)
}

// expectedLinkCount is the number of links in a fully-converged mesh: exactly one link per unordered router
// pair (links are single-dialer), C(400,2) = 400*399/2.
const expectedLinkCount = 79800

func validateLinksForCtrl(run model.Run, c *model.Component, deadline time.Time) error {
	clients, err := chaos.EnsureLoggedIntoCtrl(run, c, time.Minute)
	if err != nil {
		return err
	}

	allLinksPresent := false
	start := time.Now()

	logger := pfxlog.Logger().WithField("ctrl", c.Id)
	var lastLog time.Time
	for time.Now().Before(deadline) && !allLinksPresent {
		linkCount, err := getLinkCount(clients)
		if err != nil {
			// Keep retrying until the deadline, re-logging in best-effort: a controller that is restarting or
			// briefly unreachable is not converged, and treating the error as success skipped validation.
			logger.WithError(err).Warn("failed to get link count, retrying")
			time.Sleep(5 * time.Second)
			if newClients, loginErr := chaos.EnsureLoggedIntoCtrl(run, c, time.Minute); loginErr == nil {
				clients = newClients
			} else {
				logger.WithError(loginErr).Warn("failed to log in to controller, will retry")
			}
			continue
		}
		if linkCount == expectedLinkCount {
			allLinksPresent = true
		} else {
			time.Sleep(5 * time.Second)
		}
		if time.Since(lastLog) > time.Minute {
			logger.Infof("current link count: %v, elapsed time: %v", linkCount, time.Since(start))
			lastLog = time.Now()
		}
	}

	if allLinksPresent {
		logger.Infof("all links present, elapsed time: %v", time.Since(start))
	} else {
		linkCount, _ := getLinkCount(clients)
		logLinkDiagnostics(logger, clients, linkCount)
		return fmt.Errorf("fail to reach expected link count of %d on controller %v (got %v)", expectedLinkCount, c.Id, linkCount)
	}

	for {
		linkErrs, err := validateRouterLinks(c.Id, clients)
		if err == nil {
			return nil
		}

		if time.Now().After(deadline) {
			return err
		}

		// Log why the pass did not succeed, not just the counts: an unreachable router reports zero link
		// errors, so counts alone leave a retrying-until-deadline run with no visible cause.
		logger.Infof("validation not yet complete: %v (link errors: %v, elapsed time: %v)", err, linkErrs, time.Since(start))
		time.Sleep(15 * time.Second)
	}
}

func getLinkCount(clients *zitirest.Clients) (int64, error) {
	ctx, cancelF := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelF()

	filter := "limit 1"
	result, err := clients.Fabric.Link.ListLinks(&link.ListLinksParams{
		Filter:  &filter,
		Context: ctx,
	})

	if err != nil {
		return 0, err
	}
	linkCount := *result.Payload.Meta.Pagination.TotalCount
	return linkCount, nil
}

func validateRouterLinks(id string, clients *zitirest.Clients) (int, error) {
	logger := pfxlog.Logger().WithField("ctrl", id)

	closeNotify := make(chan struct{})
	eventNotify := make(chan *mgmt_pb.RouterLinkDetails, 1)

	handleLinkResults := func(msg *channel.Message, _ channel.Channel) {
		detail := &mgmt_pb.RouterLinkDetails{}
		if err := proto.Unmarshal(msg.Body, detail); err != nil {
			pfxlog.Logger().WithError(err).Error("unable to unmarshal router link details")
			return
		}
		eventNotify <- detail
	}

	bindHandler := func(binding channel.Binding) error {
		binding.AddReceiveHandlerF(int32(mgmt_pb.ContentType_ValidateRouterLinksResultType), handleLinkResults)
		binding.AddCloseHandler(channel.CloseHandlerF(func(ch channel.Channel) {
			close(closeNotify)
		}))
		return nil
	}

	ch, err := clients.NewWsMgmtChannel(channel.BindHandlerF(bindHandler))
	if err != nil {
		return 0, err
	}

	defer func() {
		_ = ch.Close()
	}()

	request := &mgmt_pb.ValidateRouterLinksRequest{
		Filter: "limit none",
	}
	responseMsg, err := protobufs.MarshalTyped(request).WithTimeout(10 * time.Second).SendForReply(ch)

	response := &mgmt_pb.ValidateRouterLinksResponse{}
	if err = protobufs.TypedResponse(response).Unmarshall(responseMsg, err); err != nil {
		return 0, err
	}

	if !response.Success {
		return 0, fmt.Errorf("failed to start link validation: %s", response.Message)
	}

	logger.Infof("started validation of %v routers", response.RouterCount)

	expected := response.RouterCount

	invalid := 0
	var unreachable []string
	for expected > 0 {
		select {
		case <-closeNotify:
			return 0, errors.New("unexpected close of mgmt channel")
		case routerDetail := <-eventNotify:
			expected--
			if !routerDetail.ValidateSuccess {
				// The controller could not reach this router, so its links are unknown rather than wrong.
				// Collect it and keep draining the pass: during chaos a router is routinely restarting or
				// mid-reconnect, and failing here would both abandon the remaining routers' results and
				// turn a momentary gap into a failed run. The caller retries until its deadline, so a
				// router that never becomes reachable still fails the run, named below.
				unreachable = append(unreachable,
					fmt.Sprintf("%s (%s): %s", routerDetail.RouterName, routerDetail.RouterId, routerDetail.Message))
				continue
			}
			for _, linkDetail := range routerDetail.LinkDetails {
				if !linkDetail.IsValid {
					invalid++
					logger.Infof("INVALID link %v on router %v (%v): ctrlState=%v routerState=%v destRouter=%v destConnected=%v dialed=%v messages=%v",
						linkDetail.LinkId, routerDetail.RouterId, routerDetail.RouterName,
						linkDetail.CtrlState, linkDetail.RouterState,
						linkDetail.DestRouterId, linkDetail.DestConnected,
						linkDetail.Dialed, linkDetail.Messages)
				}
			}
		}
	}

	// An inconsistent link is a real failure and is reported ahead of unreachability, which may just be
	// churn that has not settled yet.
	if invalid > 0 {
		return invalid, fmt.Errorf("invalid links found")
	}
	if len(unreachable) > 0 {
		return invalid, fmt.Errorf("%d of %d routers could not be validated on controller %s: %s",
			len(unreachable), response.RouterCount, id, describeUnreachable(unreachable))
	}
	logger.Infof("link validation of %v routers successful", response.RouterCount)
	return invalid, nil
}

// describeUnreachable renders the routers or components a controller could not reach, naming the first few
// so a failure says where to look, without embedding hundreds of entries in one error.
func describeUnreachable(entries []string) string {
	const maxNamed = 5
	if len(entries) <= maxNamed {
		return strings.Join(entries, "; ")
	}
	return fmt.Sprintf("%s; and %d more", strings.Join(entries[:maxNamed], "; "), len(entries)-maxNamed)
}

func logLinkDiagnostics(logger *logrus.Entry, clients *zitirest.Clients, linkCount int64) {
	logger.Infof("link count mismatch: expected %d, got %v, fetching diagnostics", expectedLinkCount, linkCount)

	ctx, cancelF := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelF()

	filter := "limit none"
	result, err := clients.Fabric.Link.ListLinks(&link.ListLinksParams{
		Filter:  &filter,
		Context: ctx,
	})
	if err != nil {
		logger.WithError(err).Error("failed to fetch links for diagnostics")
		return
	}

	// Links are single-dialer: each unordered router pair should have exactly one
	// link, in whichever direction the dialer chose. Normalize each link to an
	// unordered pair and look for pairs with the wrong number of links. A directed
	// view (expecting src->dst for every ordered pair) is wrong here — it flags
	// every normal one-directional link as "missing" and every router as off-count.
	type pairKey struct{ a, b string } // a <= b
	linksByPair := map[pairKey][]*rest_model.LinkDetail{}
	routerIds := map[string]struct{}{}

	for _, l := range result.Payload.Data {
		src := l.SourceRouter.ID
		dst := l.DestRouter.ID
		routerIds[src] = struct{}{}
		routerIds[dst] = struct{}{}
		k := pairKey{a: src, b: dst}
		if k.a > k.b {
			k.a, k.b = k.b, k.a
		}
		linksByPair[k] = append(linksByPair[k], l)
	}

	// Duplicate pairs: more than one link for the same unordered pair. This is the
	// real anomaly when the count is over expected (e.g., both directions present
	// because dedup didn't converge).
	dupPairs := 0
	for k, links := range linksByPair {
		if len(links) > 1 {
			dupPairs++
			logger.Infof("DUPLICATE pair %v <-> %v has %d links:", k.a, k.b, len(links))
			for _, l := range links {
				logger.Infof("  link %v: %v -> %v state=%v iteration=%v",
					*l.ID, l.SourceRouter.ID, l.DestRouter.ID, *l.State, *l.Iteration)
			}
		}
	}

	// Missing pairs: unordered router pairs with no link in either direction. This
	// is the real anomaly when the count is under expected.
	ids := make([]string, 0, len(routerIds))
	for id := range routerIds {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	missingPairs := 0
	const missingLogCap = 50
	for i := 0; i < len(ids); i++ {
		for j := i + 1; j < len(ids); j++ {
			k := pairKey{a: ids[i], b: ids[j]}
			if _, ok := linksByPair[k]; !ok {
				missingPairs++
				if missingPairs <= missingLogCap {
					logger.Infof("MISSING pair: %v <-> %v (no link in either direction)", ids[i], ids[j])
				}
			}
		}
	}
	if missingPairs > missingLogCap {
		logger.Infof("... and %d more missing pairs (log output capped at %d)", missingPairs-missingLogCap, missingLogCap)
	}

	logger.Infof("total links: %v, routers: %v, unique pairs: %v, duplicate pairs: %v, missing pairs: %v",
		len(result.Payload.Data), len(routerIds), len(linksByPair), dupPairs, missingPairs)
}
