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
	"fmt"
	"sort"
	"time"

	"github.com/openziti/fablab/kernel/lib/tui"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/v2/controller/rest_client/link"
	"github.com/openziti/ziti/v2/controller/rest_model"
	"github.com/openziti/ziti/zititest/zitilab/chaos"
	"github.com/openziti/ziti/zititest/zitilab/validations"
	"github.com/openziti/ziti/zititest/zitirest"
	"github.com/sirupsen/logrus"
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
	for _, ctrl := range ctrls {
		ctrlComponent := ctrl
		go validateLinksForCtrlWithChan(run, ctrlComponent, errC)
	}

	for i := 0; i < len(ctrls); i++ {
		err := <-errC
		if err != nil {
			return err
		}
	}

	return nil
}

func validateLinksForCtrlWithChan(run model.Run, c *model.Component, errC chan<- error) {
	errC <- validateLinksForCtrl(run, c)
}

// expectedLinkCount is the number of links in a fully-converged mesh: exactly
// one link per unordered router pair (links are single-dialer), C(400,2) =
// 400*399/2.
const expectedLinkCount = 79800

// validationTimeout is the per-controller budget to reach a fully-converged,
// validated state. maxHostUnreachableExtension is how much of that budget can be
// reclaimed when the controller's host is unreachable, so an infra/network blip
// (as opposed to slow convergence) does not fail the run.
const (
	validationTimeout           = 15 * time.Minute
	maxHostUnreachableExtension = 10 * time.Minute
)

func validateLinksForCtrl(run model.Run, c *model.Component) error {
	clients, err := chaos.EnsureLoggedIntoCtrl(run, c, time.Minute)
	if err != nil {
		return err
	}

	deadline := chaos.NewConvergenceDeadline(validationTimeout, maxHostUnreachableExtension)
	allLinksPresent := false
	start := time.Now()

	logger := tui.ValidationLogger().WithField("ctrl", c.Id)
	var lastLog time.Time
	for !deadline.Expired() && !allLinksPresent {
		linkCount, err := getLinkCount(clients)
		if err != nil {
			// A failure to reach the controller is only a convergence problem if
			// the host is up; if the host itself is unreachable it is infra, so
			// extend the deadline rather than charge it to the budget. Either way
			// keep retrying (re-login best-effort) until the deadline; a login
			// error no longer fails the run outright, since a transient blip
			// would otherwise look like non-convergence.
			if deadline.ExtendForUnreachableHost(run, c) {
				logger.Warn("controller host unreachable; extending deadline (infra, not convergence)")
			}
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
			// The poll reached the controller, so this is reachable, slow-converging
			// time that must be charged to the budget. Record it so the deadline's
			// unreachable-time accounting isn't credited for this interval.
			deadline.Observe(true)
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
		linkErrs, err := validations.ValidateRouterLinksOnce(c.Id, clients)
		if err == nil {
			logger.Infof("link validation success: elapsed time: %v", time.Since(start))
			return nil
		}

		// As above: don't let an unreachable-host (infra) outage count against the
		// deadline; a real validation error (host reachable, state wrong) still
		// fails once the deadline passes.
		deadline.ExtendForUnreachableHost(run, c)
		if deadline.Expired() {
			return err
		}

		// Log why the pass did not succeed, not just the counts: an unreachable router reports zero link
		// errors, so counts alone leave a retrying-until-deadline run with no visible cause.
		logger.Infof("validation not yet complete: %v (link errors: %v, elapsed time: %v)",
			err, linkErrs, time.Since(start))
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
	}, nil)

	if err != nil {
		return 0, err
	}
	linkCount := *result.Payload.Meta.Pagination.TotalCount
	return linkCount, nil
}

func logLinkDiagnostics(logger *logrus.Entry, clients *zitirest.Clients, linkCount int64) {
	logger.Infof("link count mismatch: expected %d, got %v, fetching diagnostics", expectedLinkCount, linkCount)

	ctx, cancelF := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelF()

	filter := "limit none"
	result, err := clients.Fabric.Link.ListLinks(&link.ListLinksParams{
		Filter:  &filter,
		Context: ctx,
	}, nil)
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
