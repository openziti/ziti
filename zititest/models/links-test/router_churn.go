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
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/openziti/fablab/kernel/lib/parallel"
	"github.com/openziti/fablab/kernel/lib/tui"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/zititest/zitilab"
	zitilib_actions "github.com/openziti/ziti/zititest/zitilab/actions"
	"github.com/openziti/ziti/zititest/zitilab/chaos"
	"github.com/openziti/ziti/zititest/zitilab/models"
	"github.com/openziti/ziti/zititest/zitirest"
)

// iterationCount drives how often entity churn is mixed into the chaos cycle. Not concurrent: test
// iterations run one after another.
var iterationCount int

// recreateRouters deletes a few routers' edge router entities and enrols them again under the same name.
//
// Every other form of chaos in this model is process-level: restarts, hard kills, freezes and netem all
// cycle a router's connection while the entity behind it stays put. Nothing has ever deleted an entity, so
// the delete path and the per-router state teardown hung off it have never run under load.
//
// A recreated router keeps its name and gets a new id, since edge router ids are assigned at create. That
// is the shape that matters here: over a long run the controllers see an ever-growing set of router ids,
// and per-router state that is not torn down on delete accumulates against ids that will never return.
// Churn alone proves nothing, because the leak is silent — validateGossipOwners is what turns it into a
// failure rather than something noticed in a heap profile months later.
//
// Half are deleted while still running. That is the harder case: the delete's cleanup races the router's
// own disconnect rather than following it.
func recreateRouters(run model.Run, clients *zitirest.Clients) error {
	selected := selectRoutersToRecreate(run)
	if len(selected) == 0 {
		return nil
	}

	type target struct {
		component  *model.Component
		routerType *zitilab.RouterType
		stopFirst  bool
	}

	var targets []target
	stoppedFirst := 0
	for _, c := range selected {
		routerType, ok := c.Type.(*zitilab.RouterType)
		if !ok {
			continue
		}
		stopFirst := rand.Intn(2) == 0
		if stopFirst {
			stoppedFirst++
		}
		targets = append(targets, target{component: c, routerType: routerType, stopFirst: stopFirst})
	}
	if len(targets) == 0 {
		return nil
	}

	tui.ValidationLogger().Infof("recreating %v routers (%v stopped before delete, %v deleted while running)",
		len(targets), stoppedFirst, len(targets)-stoppedFirst)

	for _, t := range targets {
		if t.stopFirst {
			if err := t.routerType.Stop(run, t.component); err != nil {
				return fmt.Errorf("stopping %v before delete: %w", t.component.Id, err)
			}
		}
	}

	for _, t := range targets {
		if err := zitilib_actions.EdgeExec(run.GetModel(), "delete", "edge-router", t.component.Id); err != nil {
			return fmt.Errorf("deleting edge router %v: %w", t.component.Id, err)
		}
	}

	// A router deleted while running loses its control channel when the controller closes it. Let that land
	// before stopping the process, so the recreate is not racing the previous incarnation's teardown.
	time.Sleep(2 * time.Second)
	for _, t := range targets {
		if !t.stopFirst {
			if err := t.routerType.Stop(run, t.component); err != nil {
				return fmt.Errorf("stopping %v after delete: %w", t.component.Id, err)
			}
		}
	}

	// Create and enrol run as one parallel set: the tasks for a router hand off to each other through
	// channels, so running them one at a time would wait on a step that has not been started.
	var tasks []parallel.LabeledTask
	for _, t := range targets {
		tasks = append(tasks, t.routerType.CreateAndEnrollTasks(run, t.component, clients)...)
	}
	if err := parallel.ExecuteLabeled(tasks, 20, models.RetryPolicy); err != nil {
		return fmt.Errorf("re-enrolling recreated routers: %w", err)
	}

	for _, t := range targets {
		if err := t.routerType.Start(run, t.component); err != nil {
			return fmt.Errorf("starting %v after recreate: %w", t.component.Id, err)
		}
	}

	return nil
}

// selectRoutersToRecreate picks one to three routers. Deliberately few: a recreate costs an enrolment and a
// restart, and what is wanted is a steady trickle of entity churn over a long run, not a mass replacement,
// which would look like a fleet rebuild rather than the ordinary comings and goings the delete path has to
// survive.
func selectRoutersToRecreate(run model.Run) []*model.Component {
	all := run.GetModel().SelectComponents(".router")
	if len(all) == 0 {
		return nil
	}

	count := 1 + rand.Intn(3)
	if count > len(all) {
		count = len(all)
	}

	rand.Shuffle(len(all), func(i, j int) { all[i], all[j] = all[j], all[i] })
	return all[:count]
}

// validateGossipOwners checks that per-router gossip state tracks the routers that exist, rather than every
// router that has ever existed.
//
// This is the assertion that gives recreateRouters a point. The leak it looks for is silent: a controller
// whose per-router state is never torn down keeps working, serves correct links, and only grows. Entry and
// tombstone counts do not show it either, since an owner that leaks after its last entry is gone
// contributes to neither — the owner count is the number that moves.
//
// The bound is deliberately loose. A recreated router's tombstones outlive it by design, and a router
// deleted moments ago may still be being torn down, so a small surplus is ordinary. What it refuses is a
// count that climbs with the number of recreates, which is what an absent teardown looks like.
func validateGossipOwners(run model.Run) error {
	ctrls, err := chaos.NewCtrlClients(run, ".ctrl")
	if err != nil {
		return err
	}

	liveRouters := len(run.GetModel().SelectComponents(".router"))
	if liveRouters == 0 {
		return nil
	}
	// Enough headroom for tombstones from recent recreates and for teardowns still in flight, while still
	// failing a count that grows with churn rather than with the fleet.
	limit := liveRouters*2 + 10

	logger := tui.ValidationLogger()
	var failures []string

	for _, c := range run.GetModel().SelectComponents(".ctrl") {
		stats, err := readGossipOwnerCounts(ctrls, c)
		if err != nil {
			return err
		}
		for storeType, owners := range stats {
			logger.Infof("%v: gossip store %v holds state for %v owners (%v routers live)",
				c.Id, storeType, owners, liveRouters)
			if owners > limit {
				failures = append(failures, fmt.Sprintf(
					"%v: gossip store %v holds state for %v owners, over the %v allowed for %v live routers",
					c.Id, storeType, owners, limit, liveRouters))
			}
		}
	}

	if len(failures) > 0 {
		return fmt.Errorf("per-router gossip state is not being cleaned up on delete:\n\t%s",
			strings.Join(failures, "\n\t"))
	}
	return nil
}

// readGossipOwnerCounts returns the owner count per gossip store type on one controller, from the
// "gossip-store" inspect value.
func readGossipOwnerCounts(ctrls *chaos.CtrlClients, c *model.Component) (map[string]int, error) {
	resp, err := ctrls.Inspect(c.Id, c.Id, "gossip-store")
	if err != nil {
		return nil, fmt.Errorf("failed to inspect gossip-store on %s: %w", c.Id, err)
	}
	if resp.Success != nil && !*resp.Success {
		return nil, fmt.Errorf("gossip-store inspection on %s failed: %v", c.Id, resp.Errors)
	}
	value, ok := ctrls.GetInspectValue(resp, c.Id, "gossip-store")
	if !ok {
		return nil, fmt.Errorf("no gossip-store inspect value from %s", c.Id)
	}

	var raw []byte
	if s, isStr := value.(string); isStr {
		raw = []byte(s)
	} else if raw, err = json.Marshal(value); err != nil {
		return nil, err
	}

	var parsed struct {
		TypeStats []struct {
			TypeName string `json:"typeName"`
			Owners   int    `json:"owners"`
		} `json:"typeStats"`
	}
	if err = json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse gossip-store from %s: %w", c.Id, err)
	}

	result := map[string]int{}
	for _, s := range parsed.TypeStats {
		result[s.TypeName] = s.Owners
	}
	return result, nil
}
