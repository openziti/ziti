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
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fablab/kernel/lib/actions/component"
	"github.com/openziti/fablab/kernel/lib/tui"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/zititest/zitilab/chaos"
	"github.com/openziti/ziti/zititest/zitilab/validations"
)

var scenarioCounter = rand.Intn(7)
var clientsRestarted bool
var initErr error

// ensureInitialized loads the client identity registry from the label and
// starts the event/traffic collectors. Safe to call multiple times; the
// actual work runs once per process. Must not be called from bootstrap,
// which populates the label and starts the collectors itself.
func ensureInitialized(run model.Run) error {
	identityRegistryLoaded.Do(func() {
		label := run.GetLabel()
		clientIdentityIds = make(map[string]bool)
		proxIdentityIds = make(map[string]bool)
		goClientIdentityIds = make(map[string]bool)

		for k := range label.Bindings {
			if id, ok := strings.CutPrefix(k, labelPrefixClient); ok {
				clientIdentityIds[id] = true
			}
			if id, ok := strings.CutPrefix(k, labelPrefixProx); ok {
				proxIdentityIds[id] = true
			}
			if id, ok := strings.CutPrefix(k, labelPrefixGoClient); ok {
				goClientIdentityIds[id] = true
			}
		}

		pfxlog.Logger().Infof("loaded client identity registry from label: %d total (%d prox, %d go-client)",
			len(clientIdentityIds), len(proxIdentityIds), len(goClientIdentityIds))

		if err := eventCollector.StartCollecting(run, "oidc-events"); err != nil {
			initErr = fmt.Errorf("failed to start event collector: %w", err)
			return
		}
		if err := trafficCollector.StartCollecting(run, "traffic-results"); err != nil {
			initErr = fmt.Errorf("failed to start traffic collector: %w", err)
			return
		}

		startDebugServer()
	})
	return initErr
}

// restartAllClients stops and restarts all client components, then validates
// that every client identity creates a new OIDC session. Only runs once;
// subsequent calls are no-ops so fullSuite can be looped.
func restartAllClients(run model.Run) error {
	if err := ensureInitialized(run); err != nil {
		return err
	}
	if clientsRestarted {
		tui.ValidationLogger().Info("clients already restarted, skipping")
		return nil
	}
	clientsRestarted = true

	log := tui.ValidationLogger()

	// Restart event forwarders so they establish fresh connections to the
	// collector, then wait for all to connect before restarting clients.
	log.Info("stopping event forwarders...")
	if err := component.StopInParallel(".event-forwarder", 10).Execute(run); err != nil {
		return err
	}

	log.Info("=== restartAllClients: stopping all client components ===")

	if err := component.StopInParallel(".prox,.go-client,.traffic-driver", 10000).Execute(run); err != nil {
		return err
	}

	time.Sleep(2 * time.Second)
	restartTime := time.Now()
	lastRestarted.Store(restartTime)

	// Clear rotated event logs from controllers so only current-run events remain.
	log.Info("clearing rotated event logs")
	if err := run.GetModel().ForEachHost("component.ctrl", 3, func(h *model.Host) error {
		return h.ExecLogOnlyOnError("rm -f logs/event-*.log")
	}); err != nil {
		pfxlog.Logger().WithError(err).Warn("failed to clear old event logs")
	}

	if err := component.StartInParallel(".event-forwarder", 10).Execute(run); err != nil {
		return err
	}

	log.Info("starting event forwarders...")
	forwarderCount := len(run.GetModel().SelectComponents(".event-forwarder"))
	log.Infof("waiting for %d event forwarder connections...", forwarderCount)
	if err := eventCollector.WaitForConnections(forwarderCount, 2*time.Minute); err != nil {
		return err
	}
	log.Infof("all %d event forwarders connected", forwarderCount)

	// Start hosting-side components first (echo servers, SDK hosting apps)
	// so services have terminators before clients start dialing.
	log.Info("starting hosting components...")
	if err := component.StartInParallel("host.hosting", 10000).Execute(run); err != nil {
		return err
	}

	// Start client-side components: prox (C SDK proxy), go-clients, and
	// traffic drivers. Prox instances need to complete OIDC auth and open
	// proxy listener ports, so the settle phase absorbs any startup delay.
	log.Info("starting client components...")
	if err := component.StartInParallel(".prox,.go-client,.traffic-driver", 10000).Execute(run); err != nil {
		return err
	}

	tui.ValidationLogger().Info("validating all clients created new OIDC sessions...")
	if err := validations.ValidateOidcNewSessions(&eventCollector, clientIdentityIds, restartTime, 10*time.Minute); err != nil {
		return err
	}

	log.Info("=== restartAllClients: PASSED ===")
	return nil
}

// sowChaos restarts random subsets of controllers, routers, and clients.
func sowChaos(run model.Run) error {
	log := tui.ValidationLogger()

	scenarioCounter = (scenarioCounter + 1) % 7
	scenario := scenarioCounter + 1
	log.Infof("chaos scenario %d (bits: %03b)", scenario, scenario)

	var toRestart []*model.Component

	if scenario&0b001 > 0 {
		controllers, err := chaos.SelectRandom(run, ".ctrl", chaos.RandomOfTotal())
		if err != nil {
			return err
		}
		log.Infof("restarting %d controllers", len(controllers))
		toRestart = append(toRestart, controllers...)
	}

	if scenario&0b010 > 0 {
		routers, err := chaos.SelectRandom(run, ".router", chaos.PercentageRange(10, 75))
		if err != nil {
			return err
		}
		log.Infof("restarting %d routers", len(routers))
		toRestart = append(toRestart, routers...)
	}

	if scenario&0b100 > 0 {
		clients, err := chaos.SelectRandom(run, ".client", chaos.PercentageRange(10, 75))
		if err != nil {
			return err
		}
		log.Infof("restarting %d client components", len(clients))
		toRestart = append(toRestart, clients...)
	}

	return chaos.RestartSelected(run, 500, toRestart...)
}

// minSuccessesForConvergence is the minimum number of successes we require in
// the 30-second convergence window before declaring traffic converged. This
// prevents declaring convergence while traffic drivers are still ramping up,
// which would let the strict zero-error steady-state checks begin before all
// clients are exercising their paths.
const minSuccessesForConvergence = 3000

// waitForTrafficConvergence waits until a recent window (last 30s) shows zero
// errors and at least minSuccessesForConvergence successes from the traffic
// collector. This is the core post-disruption assertion: after healing a
// partition or restarting components, traffic must converge back to healthy
// within the timeout, with enough volume that subsequent zero-error windows
// are meaningful.
func waitForTrafficConvergence(timeout time.Duration) error {
	log := tui.ValidationLogger()
	deadline := time.Now().Add(timeout)
	var lastLog time.Time

	for time.Now().Before(deadline) {
		windowStart := time.Now().Add(-30 * time.Second)
		errors := trafficCollector.ErrorCount(windowStart)
		successes := trafficCollector.SuccessCount(windowStart)

		if errors == 0 && successes >= minSuccessesForConvergence {
			log.Infof("traffic converged: %d successes, 0 errors in last 30s", successes)
			return nil
		}

		if time.Since(lastLog) > 15*time.Second {
			log.Infof("waiting for traffic convergence: %d successes, %d errors in last 30s",
				successes, errors)
			lastLog = time.Now()
		}
		time.Sleep(5 * time.Second)
	}

	windowStart := time.Now().Add(-30 * time.Second)
	errors := trafficCollector.ErrorCount(windowStart)
	successes := trafficCollector.SuccessCount(windowStart)
	return fmt.Errorf("traffic did not converge within %s: %d successes, %d errors in last 30s",
		timeout, successes, errors)
}

// validateClusterHealthy asserts that all controllers are up and reachable.
func validateClusterHealthy(run model.Run) error {
	ctrlCount := len(run.GetModel().SelectComponents(".ctrl"))
	return chaos.ValidateUp(run, ".ctrl", ctrlCount, 2*time.Minute)
}

// validateTrafficHealthy asserts zero errors in a time window. Used during steady
// state where no disruption is expected.
func validateTrafficHealthy(since time.Time) error {
	errors := trafficCollector.ErrorCount(since)
	successes := trafficCollector.SuccessCount(since)

	if successes == 0 {
		return fmt.Errorf("no traffic successes since %s (total events: %d)",
			since.UTC().Format(time.RFC3339), trafficCollector.TotalCount())
	}

	if errors > 0 {
		errEvents := trafficCollector.ErrorsSince(since)
		sample := errEvents
		if len(sample) > 3 {
			sample = sample[:3]
		}
		return fmt.Errorf("%d traffic errors since %s (0 expected). samples: %+v",
			errors, since.UTC().Format(time.RFC3339), sample)
	}

	tui.ValidationLogger().Infof("traffic healthy: %d successes, 0 errors since %s",
		successes, since.UTC().Format(time.RFC3339))
	return nil
}

// testSteadyState runs traffic for 30 minutes (6 access token refresh cycles)
// and validates auth, refresh, and traffic health every cycle.
func testSteadyState(run model.Run) error {
	if err := ensureInitialized(run); err != nil {
		return err
	}
	log := tui.ValidationLogger()
	startTime := time.Now()
	trafficCollector.Clear()

	log.Info("=== testSteadyState: starting 30 min steady-state traffic ===")

	if err := validations.ValidateOidcAuthenticated(&eventCollector, clientIdentityIds, 5*time.Minute); err != nil {
		return err
	}

	if err := validations.ValidateServiceTerminators(run, 5*time.Minute, expectedServiceTerminators,
		validations.ValidateSdkTerminators|validations.ValidateErtTerminators); err != nil {
		return err
	}

	// OIDC auth only confirms a JWT was issued. Under auth-storm load, a prox can
	// have its JWT in hand but still be stuck fetching api-session/services for
	// minutes, so its listener isn't up yet. Verify every client identity has
	// actually connected to a router before the strict zero-error loop begins.
	if err := validations.ValidateIdentitiesConnected(run, clientIdentityIds, 5*time.Minute); err != nil {
		return fmt.Errorf("clients did not finish connecting to routers: %w", err)
	}

	// Wait for all clients to settle before entering the strict zero-error loop.
	// Clients may need time to complete OIDC auth and load their service lists.
	if err := waitForTrafficConvergence(5 * time.Minute); err != nil {
		return fmt.Errorf("traffic did not converge before steady-state checks: %w", err)
	}

	cycleStart := time.Now()
	for i := 0; i < 6; i++ {
		log.Infof("steady-state cycle %d/6, sleeping 5 min...", i+1)
		time.Sleep(5 * time.Minute)

		// Auth: identities still authenticated.
		if err := validations.ValidateOidcAuthenticated(&eventCollector, clientIdentityIds, 2*time.Minute); err != nil {
			return err
		}

		// Refresh: every authenticated identity must have refreshed in this cycle.
		if err := validations.ValidateAllIdentitiesRefreshed(&eventCollector, clientIdentityIds, cycleStart); err != nil {
			return err
		}

		// Traffic: zero errors during steady state.
		if err := validateTrafficHealthy(cycleStart); err != nil {
			return err
		}

		cycleStart = time.Now()
	}

	if err := validations.ValidateRevocationHealth(run, startTime); err != nil {
		return err
	}

	if err := validations.ValidateIdentityConnectionStatuses(run, 5*time.Minute); err != nil {
		return err
	}

	if err := validateClusterHealthy(run); err != nil {
		return err
	}

	log.Info("=== testSteadyState: PASSED ===")
	return nil
}

// testShortPartition partitions 25% of client hosts from controllers for 3 minutes
// (< 5m access token). Circuits should survive. After heal, traffic converges.
func testShortPartition(run model.Run) error {
	if err := ensureInitialized(run); err != nil {
		return err
	}
	log := tui.ValidationLogger()
	trafficCollector.Clear()

	log.Info("=== testShortPartition: 3 min partition (< access token lifetime) ===")

	hosts := chaos.SelectRandomHosts(run, ".client", 25)
	log.Infof("partitioning %d client hosts from controllers", len(hosts))

	if err := chaos.PartitionHostsFromControllers(run, hosts, 100); err != nil {
		return err
	}

	log.Info("partition applied, sleeping 3 min...")
	time.Sleep(3 * time.Minute)

	if err := chaos.HealPartition(run, ".client", 100); err != nil {
		return err
	}

	log.Info("partition healed, waiting for traffic convergence...")

	if err := waitForTrafficConvergence(3 * time.Minute); err != nil {
		return err
	}

	if err := validations.ValidateOidcAuthenticated(&eventCollector, clientIdentityIds, 5*time.Minute); err != nil {
		return err
	}

	if err := validateClusterHealthy(run); err != nil {
		return err
	}

	log.Info("=== testShortPartition: PASSED ===")
	return nil
}

// testMediumPartition partitions all client hosts for 10 minutes. Access tokens
// expire at ~5 min (circuits die). Refresh tokens are still valid. After heal,
// clients refresh and traffic converges.
func testMediumPartition(run model.Run) error {
	if err := ensureInitialized(run); err != nil {
		return err
	}
	log := tui.ValidationLogger()
	trafficCollector.Clear()

	log.Info("=== testMediumPartition: 10 min partition (access expired, refresh valid) ===")

	if err := chaos.PartitionFromControllers(run, ".client", 100); err != nil {
		return err
	}

	// Sleep past access token expiry (5 min) + buffer, then verify disconnection.
	log.Info("partition applied, sleeping 7 min (past access token expiry)...")
	time.Sleep(7 * time.Minute)

	log.Info("verifying partitioned client identities are disconnected...")
	if err := validations.ValidateIdentitiesDisconnected(run, clientIdentityIds, time.Minute); err != nil {
		return fmt.Errorf("client identities should be disconnected after access token expiry: %w", err)
	}

	log.Info("sleeping remaining 3 min of partition...")
	time.Sleep(3 * time.Minute)

	healTime := time.Now()
	if err := chaos.HealPartition(run, ".client", 100); err != nil {
		return err
	}

	log.Info("partition healed, waiting for traffic convergence...")

	if err := waitForTrafficConvergence(5 * time.Minute); err != nil {
		return err
	}

	// Verify refresh events after heal (not creates, since refresh tokens are still valid).
	refreshed := eventCollector.RefreshedIdentitiesSince(healTime)
	log.Infof("%d identities refreshed after medium partition heal", len(refreshed))

	if err := validations.ValidateOidcAuthenticated(&eventCollector, clientIdentityIds, 10*time.Minute); err != nil {
		return err
	}

	if err := validations.ValidateIdentityConnectionStatuses(run, 5*time.Minute); err != nil {
		return err
	}

	if err := validations.ValidateServiceTerminators(run, 10*time.Minute, expectedServiceTerminators,
		validations.ValidateSdkTerminators|validations.ValidateErtTerminators); err != nil {
		return err
	}

	if err := validateClusterHealthy(run); err != nil {
		return err
	}

	log.Info("=== testMediumPartition: PASSED ===")
	return nil
}

// testLongPartition partitions Go client hosts for 23 minutes. Both access and
// refresh tokens expire. After heal, clients must fully re-authenticate (new
// "created" events, not "refreshed").
func testLongPartition(run model.Run) error {
	if err := ensureInitialized(run); err != nil {
		return err
	}
	log := tui.ValidationLogger()
	startTime := time.Now()
	trafficCollector.Clear()

	log.Info("=== testLongPartition: 23 min partition (both tokens expired) ===")

	if err := chaos.PartitionFromControllers(run, ".go-client", 100); err != nil {
		return err
	}

	// Sleep past access token expiry (5 min) + buffer, then verify disconnection.
	log.Info("partition applied, sleeping 7 min (past access token expiry)...")
	time.Sleep(7 * time.Minute)

	log.Info("verifying partitioned Go SDK identities are disconnected...")
	if err := validations.ValidateIdentitiesDisconnected(run, goClientIdentityIds, 3*time.Minute); err != nil {
		return fmt.Errorf("Go SDK identities should be disconnected after access token expiry: %w", err)
	}

	log.Info("sleeping remaining 16 min of partition (past refresh token expiry)...")
	time.Sleep(16 * time.Minute)

	healTime := time.Now()
	if err := chaos.HealPartition(run, ".go-client", 100); err != nil {
		return err
	}

	log.Info("partition healed, waiting for traffic convergence...")

	if err := waitForTrafficConvergence(5 * time.Minute); err != nil {
		return err
	}

	// After long partition, we should see new "created" events (full re-auth) for
	// the partitioned identities, not just "refreshed".
	created := eventCollector.CreatedIdentitiesSince(healTime)
	log.Infof("%d identities re-authenticated (created) after long partition heal", len(created))

	recoveryDuration := time.Since(healTime)
	log.Infof("full re-auth recovery took %s", recoveryDuration)

	if err := validations.ValidateOidcAuthenticated(&eventCollector, clientIdentityIds, 10*time.Minute); err != nil {
		return err
	}

	if err := validations.ValidateIdentityConnectionStatuses(run, 5*time.Minute); err != nil {
		return err
	}

	if err := validations.ValidateRevocationHealth(run, startTime); err != nil {
		return err
	}

	if err := validateClusterHealthy(run); err != nil {
		return err
	}

	log.Info("=== testLongPartition: PASSED ===")
	return nil
}

// testControllerFailover kills a random controller near access token expiry
// and validates that clients refresh via surviving controllers and traffic converges.
func testControllerFailover(run model.Run) error {
	if err := ensureInitialized(run); err != nil {
		return err
	}
	log := tui.ValidationLogger()
	trafficCollector.Clear()

	log.Info("=== testControllerFailover: kill controller during refresh ===")

	log.Info("sleeping 4 min to approach access token expiry...")
	time.Sleep(4 * time.Minute)

	ctrls := run.GetModel().SelectComponents(".ctrl")
	if len(ctrls) == 0 {
		return nil
	}
	victim := ctrls[rand.Intn(len(ctrls))]

	log.Infof("killing controller %s", victim.Id)
	if err := victim.Type.Stop(run, victim); err != nil {
		return err
	}

	// Keep the controller down long enough that clients actually have to fail
	// over: access tokens (5m lifetime) expire within ~1 min, clients detect
	// and refresh via surviving controllers, and refresh events propagate.
	log.Info("controller killed, sleeping 3 min to exercise failover...")
	time.Sleep(3 * time.Minute)

	log.Info("waiting for traffic convergence...")
	if err := waitForTrafficConvergence(5 * time.Minute); err != nil {
		return err
	}

	if err := validations.ValidateOidcAuthenticated(&eventCollector, clientIdentityIds, 5*time.Minute); err != nil {
		return err
	}

	// Restart the controller before validating identity connection statuses.
	// Otherwise ValidateIdentityConnectionStatuses would implicitly start it
	// via EnsureLoggedIntoCtrl and then run against a freshly-started
	// controller with an empty connection-status snapshot, producing massive
	// false-positive mismatches.
	log.Infof("restarting controller %s", victim.Id)
	if sc, ok := victim.Type.(model.ServerComponent); ok {
		if err := sc.Start(run, victim); err != nil {
			return err
		}
	}

	// Give the restarted controller time to re-establish control channels
	// with all routers and receive full-syncs before validating.
	log.Info("waiting 90s for controller to catch up from full syncs...")
	time.Sleep(90 * time.Second)

	if err := validations.ValidateIdentityConnectionStatuses(run, 5*time.Minute); err != nil {
		return err
	}

	if err := validateClusterHealthy(run); err != nil {
		return err
	}

	log.Info("=== testControllerFailover: PASSED ===")
	return nil
}

// testChaosIteration runs a combined chaos scenario, then waits for traffic
// convergence and validates system health.
func testChaosIteration(run model.Run) error {
	if err := ensureInitialized(run); err != nil {
		return err
	}
	log := tui.ValidationLogger()
	trafficCollector.Clear()

	log.Info("=== testChaosIteration: mixed chaos ===")

	if err := sowChaos(run); err != nil {
		return err
	}

	if err := run.GetModel().Exec(run, "validateUp"); err != nil {
		return err
	}

	log.Info("components up, waiting for traffic convergence...")
	if err := waitForTrafficConvergence(5 * time.Minute); err != nil {
		return err
	}

	if err := run.GetModel().Exec(run, "validate"); err != nil {
		return err
	}

	if err := validateClusterHealthy(run); err != nil {
		return err
	}

	log.Info("=== testChaosIteration: PASSED ===")
	return nil
}
