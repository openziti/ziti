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
	"strconv"

	"github.com/openziti/fablab/kernel/lib/tui"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/zititest/zitilab"
	"github.com/openziti/ziti/zititest/zitilab/stageziti"
)

// The model runs on more than one ziti release at a time, along two independent axes: what the routers run
// and what the controllers run. A soak picks a point on each.
//
// Set as model variables, so a run is configured from ~/.fablab/bindings.yml, from the environment
// (ROUTERVERSIONMIX and friends, per the default name mapper), or from the command line, in fablab's usual
// precedence.
const (
	// routerMixVar turns on a fleet split evenly across the three versions under test. Off means every
	// router runs the locally built one.
	routerMixVar = "routerVersionMix"

	// ctrlVersionVar selects what the controllers run: ctrlVersionLatest, ctrlVersionOld or
	// ctrlVersionMixed.
	ctrlVersionVar = "ctrlVersionMode"

	// Versions the run uses alongside the locally built one, so a soak can be pointed at whichever releases
	// matter.
	oldVersionVar    = "oldVersion"
	oldestVersionVar = "oldestVersion"
)

// Controller version modes.
const (
	// ctrlVersionLatest runs every controller on the build under test. The only mode where the cluster can
	// carry link gossip.
	ctrlVersionLatest = "latest"

	// ctrlVersionOld runs every controller on the previous release, so the fleet exercises the pre-gossip
	// path end to end while the routers span three versions.
	ctrlVersionOld = "old"

	// ctrlVersionMixed runs one controller on the build under test and the rest on the previous release.
	//
	// A cluster whose members disagree about their version is read-only: controller/raft refuses every
	// command with "different versions detected in cluster". So this mode cannot make model changes, which
	// is why entity churn is skipped while it is active. It is the upgrade window, not a state to sit in.
	ctrlVersionMixed = "mixed"
)

// currentVersion is the locally built binary, staged without a version suffix. Whatever the branch under
// test produces, which is the version these runs are really about.
const currentVersion = ""

// versionConfig is what this run was configured to use. Resolved once while the model is built, so the
// actions that run later read a settled answer rather than re-resolving as they go.
type versionConfig struct {
	routerMix bool
	ctrlMode  string
	old       string
	oldest    string
}

// activeVersions is seeded so that a caller reaching it before the model is built sees a single-version
// run rather than an empty one. The upgrade action moves ctrlMode as it goes.
var activeVersions = versionConfig{ctrlMode: ctrlVersionLatest}

// resolveVersionConfig reads the run's version configuration from the model.
func resolveVersionConfig(m *model.Model) {
	activeVersions = versionConfig{
		routerMix: boolVariable(m, routerMixVar),
		ctrlMode:  ctrlModeSetting(m),
		old:       m.MustStringVariable(oldVersionVar),
		oldest:    m.MustStringVariable(oldestVersionVar),
	}
}

// boolVariable reads a bool-valued model variable, false when it is unset.
func boolVariable(m *model.Model, name string) bool {
	v, _ := m.GetBoolVariable(name)
	return v
}

// intVariable reads an int-valued model variable, falling back to def where it is unset or unreadable.
// Bindings from the environment or the command line arrive as strings, so both shapes are accepted.
func intVariable(m *model.Model, name string, def int) int {
	v, found := m.GetVariable(name)
	if !found {
		return def
	}

	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(val)
	case string:
		if parsed, err := strconv.Atoi(val); err == nil {
			return parsed
		}
	}

	tui.ValidationLogger().Warnf("unusable %v %v, using %v", name, v, def)
	return def
}

func ctrlModeSetting(m *model.Model) string {
	switch mode := m.MustStringVariable(ctrlVersionVar); mode {
	case ctrlVersionOld, ctrlVersionMixed, ctrlVersionLatest:
		return mode
	default:
		tui.ValidationLogger().Warnf("unknown %v %q, running every controller on the build under test",
			ctrlVersionVar, mode)
		return ctrlVersionLatest
	}
}

func routerMixEnabled() bool {
	return activeVersions.routerMix
}

func ctrlVersionMode() string {
	return activeVersions.ctrlMode
}

func oldVersion() string {
	return activeVersions.old
}

func oldestVersion() string {
	return activeVersions.oldest
}

func versionLabel(v string) string {
	if v == currentVersion {
		return "current"
	}
	return v
}

// ctrlVersions returns what each controller should run, in the order the controllers are listed.
//
// Indexed per controller rather than given as a count, so which controller is old stays put instead of
// moving around as the mode changes.
func ctrlVersions(count int) []string {
	versions := make([]string, count)
	for i := range versions {
		switch ctrlVersionMode() {
		case ctrlVersionOld:
			versions[i] = oldVersion()
		case ctrlVersionMixed:
			// The first stays current; the rest are the previous release.
			if i > 0 {
				versions[i] = oldVersion()
			}
		default:
			versions[i] = currentVersion
		}
	}
	return versions
}

// clusterReadOnly reports whether the controllers disagree about their version, which puts raft into a
// state that refuses every model update.
//
// Derived from what the model configured rather than asked of a controller: it is the same answer, it
// cannot fail, and the upgrade action knows it before the cluster does.
func clusterReadOnly() bool {
	return ctrlVersionMode() == ctrlVersionMixed
}

// gossipExpected reports whether link gossip should be usable. Only a cluster where every controller runs
// the build under test can carry it.
func gossipExpected() bool {
	return ctrlVersionMode() == ctrlVersionLatest
}

// usesOldBinaries reports whether this run needs a release staged alongside the local build.
func usesOldBinaries() bool {
	return routerMixEnabled() || ctrlVersionMode() != ctrlVersionLatest
}

// assignRouterVersions spreads the routers evenly across the versions under test.
//
// Thirds rather than halves because links are pairs: with three versions in equal parts every combination
// of dialer and acceptor occurs, including the middle version against both of the others, which a two-way
// split would never produce.
func assignRouterVersions(m *model.Model) error {
	// Resolved here rather than at init, because model variables are not bound until the model is being
	// built. This is the first factory to run, so everything after it reads a settled answer.
	resolveVersionConfig(m)

	if !routerMixEnabled() {
		return nil
	}

	versions := []string{currentVersion, oldVersion(), oldestVersion()}
	counts := map[string]int{}

	err := m.ForEachComponent(".router", 1, func(c *model.Component) error {
		routerType, ok := c.Type.(*zitilab.RouterType)
		if !ok {
			return nil
		}
		// Cloned because the components share one type value; assigning to it directly would move every
		// router onto the last version assigned.
		clone := *routerType
		clone.Version = versions[int(c.ScaleIndex)%len(versions)]
		c.Type = &clone
		counts[versionLabel(clone.Version)]++
		return nil
	})
	if err != nil {
		return err
	}

	for version, count := range counts {
		tui.ValidationLogger().Infof("router versions: %v on %v", count, version)
	}
	return nil
}

// assignCtrlVersions puts the controllers on the versions their mode names, while the model is built, so
// the cluster is bootstrapped on them rather than arriving there by an upgrade.
func assignCtrlVersions(m *model.Model) error {
	changed, err := applyCtrlVersions(m)
	if err != nil {
		return err
	}

	for _, c := range changed {
		tui.ValidationLogger().Infof("controller %v on %v", c.Id, versionLabel(ctrlVersionOf(c)))
	}

	if clusterReadOnly() {
		tui.ValidationLogger().Info("controllers span versions: the cluster is read-only, so entity churn is skipped")
	}
	return nil
}

// setCtrlMode moves the run onto a controller version mode and puts each controller's component type on the
// version that mode names, returning the controllers whose version changed.
//
// Getting them there is the caller's business, since the two directions are not alike: an upgrade is a
// restart, a downgrade is a rebuild. Not safe to call while validations are running; the version state is
// read all over the model without synchronisation, and phases move only on the test goroutine.
func setCtrlMode(m *model.Model, mode string) ([]*model.Component, error) {
	activeVersions.ctrlMode = mode
	return applyCtrlVersions(m)
}

// applyCtrlVersions puts each controller's component type on the version the current mode names, and
// returns the controllers whose version actually changed.
func applyCtrlVersions(m *model.Model) ([]*model.Component, error) {
	ctrls := m.SelectComponents(".ctrl")
	versions := ctrlVersions(len(ctrls))

	var changed []*model.Component
	for i, c := range ctrls {
		ctrlType, ok := c.Type.(*zitilab.ControllerType)
		if !ok {
			return nil, fmt.Errorf("component %v is not a controller", c.Id)
		}
		if ctrlType.Version == versions[i] {
			continue
		}
		// Cloned rather than assigned to, since a component type value may be shared.
		clone := *ctrlType
		clone.Version = versions[i]
		c.Type = &clone
		changed = append(changed, c)
	}

	return changed, nil
}

// ctrlVersionOf returns what a controller component is set to run, empty for the build under test or for a
// component that is not a controller.
func ctrlVersionOf(c *model.Component) string {
	if ctrlType, ok := c.Type.(*zitilab.ControllerType); ok {
		return ctrlType.Version
	}
	return ""
}

// routerNarrowsMetrics reports whether a router sends its metrics message to only its subscription
// controller rather than to every controller. True where the cluster can carry link gossip and the router
// is the build under test; anything else fans out, which is the pre-gossip behaviour. Mirrors the gate in
// the router's controllersReporter.
func routerNarrowsMetrics(c *model.Component) bool {
	if !gossipExpected() {
		return false
	}
	routerType, ok := c.Type.(*zitilab.RouterType)
	return ok && routerType.Version == currentVersion
}

// stageMixedVersions downloads the releases this run needs, so starting a component on one is a matter of
// picking a binary that is already there.
func stageMixedVersions(run model.Run) error {
	if !usesOldBinaries() {
		return nil
	}

	c := run.GetModel().MustSelectComponent("#ctrl1")
	for _, version := range []string{oldVersion(), oldestVersion()} {
		tui.ValidationLogger().Infof("staging ziti %v", version)
		if err := stageziti.StageZitiOnce(run, c, version, ""); err != nil {
			return fmt.Errorf("staging ziti %v: %w", version, err)
		}
	}
	return nil
}
