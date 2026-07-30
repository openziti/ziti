/*
	(c) Copyright NetFoundry Inc.

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
	"strconv"
	"strings"
	"time"

	"github.com/openziti/channel/v5"
	"github.com/openziti/fablab/kernel/lib/tui"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/v2/controller/rest_client/terminator"
	"github.com/openziti/ziti/v2/zitirest"
	"github.com/openziti/ziti/zititest/ziti-traffic-test/loop4"
	"github.com/openziti/ziti/zititest/zitilab"
	"github.com/openziti/ziti/zititest/zitilab/chaos"
	zitiLibOps "github.com/openziti/ziti/zititest/zitilab/runlevel/5_operation"
	"github.com/openziti/ziti/zititest/zitilab/validations"
)

// noopControllerCallback satisfies loop4.ControllerCallback without requesting any diagnostics.
type noopControllerCallback struct{}

func (noopControllerCallback) DiagnosticRequested(*channel.Message, channel.Channel) {
	tui.ValidationLogger().Debug("sim diagnostic requested; ignoring")
}

// expectedTerminator is a service and the number of terminators that must be present for it once the
// system is healthy.
type expectedTerminator struct {
	service string
	count   int64
}

// expectedTerminators is the full set of terminators the topology should have when healthy: one loop
// service per hosting flavor plus the two sim control-plane services. It is verified both on initial
// setup and after every disruptive step (each host must re-establish its terminator).
var expectedTerminators = []expectedTerminator{
	{service: "loop-sdk", count: 2},         // loop4-sdk-host-stable/-restart
	{service: "loop-ziti-tunnel", count: 2}, // ziti-tunnel-host-stable/-restart
	{service: "loop-zet", count: 2},         // ziti-edge-tunnel-host-stable/-restart
	{service: "loop-ert", count: 1},         // router-west edge-router tunneler
	{service: "sim-control", count: 1},      // sim harness
	{service: "metrics", count: 1},          // sim harness
}

// totalExpectedTerminators returns the sum of all expected terminator counts.
func totalExpectedTerminators() int64 {
	var total int64
	for _, e := range expectedTerminators {
		total += e.count
	}
	return total
}

// steadyStateGate validates that traffic is healthy by driving discrete loop4 scenario runs.
// A run is "clean" when every remote-controlled sim client reports success across every
// hosting flavor. The gate first waits out any churn (recovery), then requires sustained
// clean runs (stability) before it returns.
type steadyStateGate struct {
	sim *zitiLibOps.SimServices

	// requiredCleanRuns is how many consecutive clean runs declare the system recovered.
	requiredCleanRuns int
	// recoveryTimeout bounds how long we tolerate failures before giving up on recovery.
	recoveryTimeout time.Duration
	// stabilityWindow is how long runs must stay clean once recovered.
	stabilityWindow time.Duration

	// postDisruptionDelay is how long to wait after a disruptive step before checking terminators,
	// letting the initial churn begin to settle.
	postDisruptionDelay time.Duration
	// terminatorSettleTimeout bounds how long to wait for all expected terminators to (re-)establish.
	terminatorSettleTimeout time.Duration

	connectTimeout  time.Duration
	scenarioTimeout time.Duration
	interRunDelay   time.Duration
}

// newSteadyStateGate returns a gate with default tuning; tuning is refined from model
// variables under the steadyState.* namespace at validation time.
func newSteadyStateGate(sim *zitiLibOps.SimServices) *steadyStateGate {
	return &steadyStateGate{
		sim:                     sim,
		requiredCleanRuns:       3,
		recoveryTimeout:         time.Minute,
		stabilityWindow:         2 * time.Minute,
		postDisruptionDelay:     30 * time.Second,
		terminatorSettleTimeout: time.Minute,
		connectTimeout:          60 * time.Second,
		scenarioTimeout:         2 * time.Minute,
		interRunDelay:           time.Second,
	}
}

func (g *steadyStateGate) loadTuning(m *model.Model) {
	if v := m.GetStringVariableOr("steadyState.requiredCleanRuns", ""); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			g.requiredCleanRuns = n
		}
	}
	if v := m.GetStringVariableOr("steadyState.recoveryTimeout", ""); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			g.recoveryTimeout = d
		}
	}
	if v := m.GetStringVariableOr("steadyState.stabilityWindow", ""); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			g.stabilityWindow = d
		}
	}
	if v := m.GetStringVariableOr("steadyState.postDisruptionDelay", ""); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			g.postDisruptionDelay = d
		}
	}
	if v := m.GetStringVariableOr("steadyState.terminatorSettleTimeout", ""); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			g.terminatorSettleTimeout = d
		}
	}
}

func (g *steadyStateGate) validate(run model.Run) error {
	m := run.GetModel()
	g.loadTuning(m)

	// gate output (scenario runs, recovery/stability progress) routes to the TUI validation pane
	log := tui.ValidationLogger()

	// ensure the harness is hosting sim-control before we verify terminators; the first call binds it,
	// and its sim-control/metrics terminators are part of the expected set checked below. Post-disruption
	// this reuses the existing controller, so the terminator must re-establish on its own (SDK recovery).
	simControl, err := g.sim.GetSimController(run, "sim-control", noopControllerCallback{})
	if err != nil {
		return err
	}

	// every expected terminator must be present and valid before we drive traffic; on initial setup
	// this confirms the topology, after a disruption it confirms every host has re-established.
	if err = g.verifyTerminators(run); err != nil {
		return err
	}

	sims := m.FilterComponents(".loop-client", func(c *model.Component) bool {
		t, ok := c.Type.(*zitilab.Loop4SimType)
		return ok && t.Mode == zitilab.Loop4RemoteControlled
	})
	if len(sims) == 0 {
		return fmt.Errorf("no remote-controlled loop clients found")
	}

	log.Infof("steady-state gate: waiting up to %s for %d sim client(s) to connect", g.connectTimeout, len(sims))
	if err = simControl.WaitForAllConnected(g.connectTimeout, sims); err != nil {
		return err
	}

	// recovery: keep running until we string together enough clean runs, or time out
	deadline := time.Now().Add(g.recoveryTimeout)
	consecutive := 0
	for consecutive < g.requiredCleanRuns {
		if time.Now().After(deadline) {
			return fmt.Errorf("did not reach %d consecutive clean runs within %s", g.requiredCleanRuns, g.recoveryTimeout)
		}
		if err = g.runScenario(simControl, sims); err != nil {
			log.WithError(err).Warnf("scenario run failed during recovery; resetting clean-run count from %d", consecutive)
			consecutive = 0
			time.Sleep(g.interRunDelay)
			continue
		}
		consecutive++
		log.Infof("steady-state gate recovery: %d/%d consecutive clean runs", consecutive, g.requiredCleanRuns)
	}

	// stability: any failure in the window fails the gate
	log.Infof("steady-state gate: recovered; requiring clean runs for %s", g.stabilityWindow)
	stabilityEnd := time.Now().Add(g.stabilityWindow)
	for time.Now().Before(stabilityEnd) {
		if err = g.runScenario(simControl, sims); err != nil {
			return fmt.Errorf("stability window failed: %w", err)
		}
		time.Sleep(g.interRunDelay)
	}

	log.Info("steady-state gate: passed")
	return nil
}

// runScenario fires a single scenario across all sim clients and returns an error unless
// every client reports success. A disconnected client is treated as a failure.
func (g *steadyStateGate) runScenario(simControl *loop4.RemoteController, sims []*model.Component) error {
	if missing := simControl.MissingComponents(sims); len(missing) > 0 {
		return fmt.Errorf("sim clients not connected: %v", missing)
	}
	results, err := simControl.StartSimScenarios()
	if err != nil {
		return err
	}
	return results.GetResults(g.scenarioTimeout)
}

// validateAfterDisruption is the gate variant used after a disruptive step (controller/router
// restart). It waits for the churn to begin settling before running the normal gate, which then
// waits for every expected terminator to re-establish.
func (g *steadyStateGate) validateAfterDisruption(run model.Run) error {
	m := run.GetModel()
	g.loadTuning(m)
	log := tui.ValidationLogger()
	log.Infof("post-disruption settle: waiting %s before verifying terminators", g.postDisruptionDelay)
	time.Sleep(g.postDisruptionDelay)

	// Pre-2.0 routers have a bug where, when the controller upgrade purges their legacy sessions, they
	// drop the affected hosted terminators locally without telling the controller, leaving stale
	// terminators the router no longer hosts. Proactively reconcile them (delete the ones routers no
	// longer host) so the strict validation reflects reality. Gate on the routers' *current* version,
	// not fromVersion: once every router is upgraded to 2.0+ this must stop running so genuine 2.0+
	// terminator bugs are caught rather than masked.
	if anyPreV2Router(m) {
		if err := g.reconcileStaleTerminators(run); err != nil {
			return err
		}
	}

	return g.validate(run)
}

// reconcileStaleTerminators deletes edge terminators the routers no longer host (the pre-2.0 upgrade
// leaves these behind). It is scoped to edge terminators: an unscoped fix would inspect ERT terminators
// on the routers, which panics pre-2.0 routers that host none.
func (g *steadyStateGate) reconcileStaleTerminators(run model.Run) error {
	log := tui.ValidationLogger()
	ctrl := run.GetModel().MustSelectComponent("#ctrl1")
	clients, err := chaos.EnsureLoggedIntoCtrl(run, ctrl, time.Minute)
	if err != nil {
		return fmt.Errorf("unable to log into #ctrl1 to reconcile stale terminators: %w", err)
	}
	fixed, err := validations.FixInvalidTerminators(clients, `binding="edge" limit none`)
	if err != nil {
		return fmt.Errorf("failed to reconcile stale terminators: %w", err)
	}
	log.Infof("reconciled stale terminators (pre-2.0 upgrade workaround): %d fixed", fixed)
	return nil
}

// isPreV2 reports whether version is a pre-2.0 release (e.g. v1.x). Empty or unparseable versions are
// treated as not pre-2.0, so the stale-terminator workaround stays off unless we know it is needed.
func isPreV2(version string) bool {
	major, _, _ := strings.Cut(strings.TrimPrefix(strings.TrimSpace(version), "v"), ".")
	n, err := strconv.Atoi(major)
	if err != nil {
		return false
	}
	return n < 2
}

// anyPreV2Router reports whether any router component is still on a pre-2.0 version. The
// stale-terminator reconciliation only applies to pre-2.0 router behavior, so it must stop once
// every router has been upgraded, otherwise it masks genuine 2.0+ terminator bugs.
func anyPreV2Router(m *model.Model) bool {
	result := false
	_ = m.ForEachComponent("*", 1, func(c *model.Component) error {
		if rt, ok := c.Type.(*zitilab.RouterType); ok && isPreV2(rt.Version) {
			result = true
		}
		return nil
	})
	return result
}

// verifyTerminators waits until every expected terminator is present for its service, then confirms
// the router SDK and ERT terminators are valid all the way up the stack. It bounds the wait by
// terminatorSettleTimeout so a host that never re-establishes fails the gate.
func (g *steadyStateGate) verifyTerminators(run model.Run) error {
	log := tui.ValidationLogger()
	ctrl := run.GetModel().MustSelectComponent("#ctrl1")

	log.Infof("verifying %d expected terminators are present (timeout %s)", totalExpectedTerminators(), g.terminatorSettleTimeout)
	deadline := time.Now().Add(g.terminatorSettleTimeout)
	var clients *zitirest.Clients
	var lastLog time.Time
	for {
		if clients == nil {
			var err error
			if clients, err = chaos.EnsureLoggedIntoCtrl(run, ctrl, time.Minute); err != nil {
				if time.Now().After(deadline) {
					return fmt.Errorf("unable to log into #ctrl1 to verify terminators: %w", err)
				}
				time.Sleep(5 * time.Second)
				continue
			}
		}

		missing, err := g.missingTerminators(clients)
		if err != nil {
			clients = nil
			if time.Now().After(deadline) {
				return fmt.Errorf("unable to list terminators: %w", err)
			}
			time.Sleep(5 * time.Second)
			continue
		}
		if len(missing) == 0 {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("expected terminators not present within %s: %v", g.terminatorSettleTimeout, missing)
		}
		if time.Since(lastLog) > 15*time.Second {
			log.Infof("waiting for terminators: %v", missing)
			lastLog = time.Now()
		}
		time.Sleep(5 * time.Second)
	}

	// terminators are all present; confirm they're valid up the stack (router <-> controller agree).
	// ERT validation is scoped to edge-router tunnelers: inspecting the ERT registry on a non-hosting
	// router (e.g. an initiator) panics older (1.6.x) routers, and only ERT hosts have ERT terminators.
	log.Info("all expected terminators present; validating them up the stack")
	validationDeadline := time.Now().Add(g.terminatorSettleTimeout)
	return validations.ValidateTerminatorsForCtrlWithFilters(run, ctrl, validationDeadline,
		validations.MinCount(totalExpectedTerminators()),
		validations.ValidateSdkTerminators|validations.ValidateErtTerminators,
		"limit none", ertRouterFilter(run.GetModel()))
}

// ertRouterFilter builds a router filter selecting the edge-router tunnelers (the ".ert-host" routers),
// so ERT terminator validation only inspects routers that actually host ERT terminators. If none are
// found it returns a filter that matches no routers.
func ertRouterFilter(m *model.Model) string {
	routers := m.SelectComponents(".ert-host")
	if len(routers) == 0 {
		return `name = "" limit none`
	}
	names := make([]string, 0, len(routers))
	for _, r := range routers {
		names = append(names, fmt.Sprintf("%q", r.Id))
	}
	return fmt.Sprintf("name in [%s] limit none", strings.Join(names, ","))
}

// missingTerminators returns, for each expected service that is short, a human-readable "service:
// have/want" entry. An empty result means every expected terminator is present.
func (g *steadyStateGate) missingTerminators(clients *zitirest.Clients) ([]string, error) {
	var missing []string
	for _, e := range expectedTerminators {
		count, err := terminatorCountForService(clients, e.service)
		if err != nil {
			return nil, err
		}
		if count < e.count {
			missing = append(missing, fmt.Sprintf("%s: %d/%d", e.service, count, e.count))
		}
	}
	return missing, nil
}

// terminatorCountForService returns the number of terminators the controller has for the named service.
func terminatorCountForService(clients *zitirest.Clients, service string) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	filter := fmt.Sprintf(`service.name="%s" limit 1`, service)
	result, err := clients.Fabric.Terminator.ListTerminators(&terminator.ListTerminatorsParams{
		Filter:  &filter,
		Context: ctx,
	}, nil)
	if err != nil {
		return 0, err
	}
	return *result.Payload.Meta.Pagination.TotalCount, nil
}
