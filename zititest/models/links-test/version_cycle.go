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
	"time"

	"github.com/openziti/fablab/kernel/lib/actions/component"
	"github.com/openziti/fablab/kernel/lib/tui"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/zititest/zitilab/actions/edge"
)

// The version cycle is the upgrade story this model exists to exercise: a cluster on the previous release,
// upgraded a controller at a time to the build under test, chaos run at every stop, and then put back where
// it started so the next pass begins from the same place.
//
// Only the controllers move. A mixed-version router fleet is settled once, while the model is built, and
// the question the cycle asks is what happens to those routers as capabilities the cluster either has or
// does not appear and disappear underneath them.
//
// Phases run one after another on the test goroutine, so the version state the rest of the model reads is
// never changed while a validation is in flight.

const (
	// ctrlAvailableTimeout bounds the wait for a restarted controller to listen again. Generous because the
	// binary changes: a controller coming up on the build under test migrates its datastore first.
	ctrlAvailableTimeout = 5 * time.Minute

	// iterationsPerPhaseVar is how many test iterations each phase runs before the cycle moves on. Zero is
	// a legitimate setting: enter the phase, check the cluster converged on it, move on.
	iterationsPerPhaseVar = "iterationsPerPhase"

	defaultIterationsPerPhase = 3
)

// versionCycle runs one pass of the upgrade story: the previous release, then a partial upgrade, then a
// complete one, iterating at each stop, and ending with the cluster back on the previous release.
//
// Meant for `fablab exec-loop forever versionCycle`, which is why it ends where it begins.
func versionCycle(run model.Run) error {
	for _, mode := range []string{ctrlVersionOld, ctrlVersionMixed, ctrlVersionLatest} {
		if err := runPhase(run, mode); err != nil {
			return err
		}
	}
	return nil
}

// runPhase puts the controllers on the versions mode names, checks the cluster converged there, and then
// runs the configured number of chaos iterations against it.
func runPhase(run model.Run, mode string) error {
	logger := tui.ValidationLogger()
	logger.Infof("version cycle: entering the %v controller phase", mode)

	if err := enterPhase(run, mode); err != nil {
		return fmt.Errorf("entering the %v controller phase: %w", mode, err)
	}

	// Checked before any chaos is added, so a failure here belongs to the version change rather than to
	// whatever the iteration did on top of it.
	logger.Infof("version cycle: validating the %v controller phase before adding chaos", mode)
	if err := run.GetModel().Exec(run, validationActions()...); err != nil {
		return fmt.Errorf("validating the %v controller phase: %w", mode, err)
	}

	iterations := iterationsPerPhase(run.GetModel())
	for i := 1; i <= iterations; i++ {
		logger.Infof("version cycle: %v controller phase, iteration %v of %v", mode, i, iterations)
		if err := run.GetModel().Exec(run, "testIteration"); err != nil {
			return fmt.Errorf("%v controller phase, iteration %v of %v: %w", mode, i, iterations, err)
		}
	}

	return nil
}

// enterPhase moves the cluster onto a controller version mode. Going forward is a restart; going back is
// not, which is why the two directions are different actions.
func enterPhase(run model.Run, mode string) error {
	if mode == ctrlVersionOld {
		return resetCtrlsToOld(run)
	}
	return upgradeCtrlsTo(run, mode)
}

// upgradeCtrlsTo moves the controllers onto the versions mode names, restarting the ones whose version
// actually changes.
//
// One at a time, each waited for, so the routers always have a controller to talk to. Restarting them
// together would test a full cluster outage instead, which the chaos iterations already do.
func upgradeCtrlsTo(run model.Run, mode string) error {
	changed, err := setCtrlMode(run.GetModel(), mode)
	if err != nil {
		return err
	}

	if len(changed) == 0 {
		tui.ValidationLogger().Infof("controllers are already on the %v layout, nothing to restart", mode)
		return nil
	}

	for _, c := range changed {
		spec := "#" + c.Id
		tui.ValidationLogger().Infof("restarting controller %v on %v", c.Id, versionLabel(ctrlVersionOf(c)))

		if err = component.Stop(spec).Execute(run); err != nil {
			return fmt.Errorf("stopping controller %v: %w", c.Id, err)
		}
		if err = component.Start(spec).Execute(run); err != nil {
			return fmt.Errorf("starting controller %v: %w", c.Id, err)
		}
		if err = edge.ControllerAvailable(spec, ctrlAvailableTimeout).Execute(run); err != nil {
			return fmt.Errorf("waiting for controller %v: %w", c.Id, err)
		}
	}

	return nil
}

// resetCtrlsToOld puts the whole cluster back on the previous release.
//
// A rebuild rather than a restart. A controller that has run the build under test has migrated its
// datastore past what the previous release will open, so going back means giving it a new one. bootstrap
// already does the whole job: it clears controller state, re-initialises raft and re-enrols every router.
//
// Skipped where the controllers are already on the previous release, which is the first pass of a soak that
// bootstrapped there. Nothing has migrated yet, so there is nothing to undo.
func resetCtrlsToOld(run model.Run) error {
	if ctrlVersionMode() == ctrlVersionOld {
		tui.ValidationLogger().Info("controllers are already on the previous release, skipping the reset")
		return nil
	}

	if _, err := setCtrlMode(run.GetModel(), ctrlVersionOld); err != nil {
		return err
	}

	tui.ValidationLogger().Infof("rebuilding the cluster on %v: a migrated datastore cannot be opened by it",
		oldVersion())
	return run.GetModel().Exec(run, "bootstrap")
}

// validationActions is the set of checks an iteration or a phase change ends with. What the cluster is
// running decides the last two.
//
// Where there is no gossip, the gossip consistency checks skip themselves, so something has to say what
// should be there instead: nothing. The metrics check needs every controller on the build under test, for
// the reason its own documentation gives, and is left out rather than run blind.
func validationActions() []string {
	actions := []string{"validateUp", "validateLinks", "validateGossipOwners"}
	if gossipExpected() {
		actions = append(actions, "validateMetrics")
	} else {
		actions = append(actions, "validateGossipEmpty")
	}
	return actions
}

func iterationsPerPhase(m *model.Model) int {
	return intVariable(m, iterationsPerPhaseVar, defaultIterationsPerPhase)
}
