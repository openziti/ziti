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
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/openziti/fablab/kernel/lib/tui"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/foundation/v2/versions"
	"github.com/openziti/ziti/v2/ziti/util"
	"github.com/openziti/ziti/zititest/zitilab"
	"github.com/openziti/ziti/zititest/zitilab/actions/edge"
	"github.com/openziti/ziti/zititest/zitilab/chaos"
	"github.com/openziti/ziti/zititest/zitilab/models"
	"github.com/openziti/ziti/zititest/zitilab/stageziti"
)

// envOrModelVar returns the value of envVar when it is set in the environment, otherwise the model
// variable name. The environment wins so a single run can override a value the model hardcodes, and
// an env var set to the empty string is honored as such: that is the only way to switch off a
// non-empty model default without editing the model.
func envOrModelVar(m *model.Model, name, envVar string) string {
	if v, found := os.LookupEnv(envVar); found {
		return v
	}
	return m.GetStringVariableOr(name, "")
}

// upgradeTarget names the model variables and environment overrides that select the ziti binary for
// one of the model's upgrade targets.
type upgradeTarget struct {
	// version holds the version label the binary is staged under and, for ref and source builds, the
	// version stamped into it. An empty value means the target is not in play.
	version   string
	source    string
	ref       string
	sourceEnv string
	refEnv    string
}

var (
	// toTarget is the 1.6 -> 2.0 upgrade the model always runs.
	toTarget = upgradeTarget{
		version:   "toVersion",
		source:    "toVersionSource",
		ref:       "toVersionRef",
		sourceEnv: "ZITI_TOVERSION_PATH",
		refEnv:    "ZITI_TOVERSION_REF",
	}
	// nextTarget is the optional 2.0 -> 2.1 cluster upgrade that follows it.
	nextTarget = upgradeTarget{
		version:   "nextVersion",
		source:    "nextVersionSource",
		ref:       "nextVersionRef",
		sourceEnv: "ZITI_NEXTVERSION_PATH",
		refEnv:    "ZITI_NEXTVERSION_REF",
	}
)

// stage pre-stages the target's ziti binary into the kit so it is rsynced to every host alongside the
// fromVersion binary. In-place upgrades then only require a version swap and a restart, with no
// mid-run binary transfer. A target whose version is unset stages nothing.
//
// The binary is chosen in precedence order, with each environment variable overriding its model
// variable:
//   - the source variable: a locally-built ziti binary, copied under the target's version name. Use
//     this to run a patched controller/router from a binary you already built.
//   - the ref variable: a git ref (branch, tag, or SHA) on openziti/ziti, built from source via
//     acquire and staged under the target's version name. Use this to run a fix, or a release that
//     does not exist yet, without hand-building. The build targets the hosts' platform, so it works
//     from a mac or arm machine too, but it must be a git ref: a release version belongs in the
//     version variable.
//   - otherwise the released version is downloaded.
func (self upgradeTarget) stage(run model.Run) error {
	m := run.GetModel()
	version := m.GetStringVariableOr(self.version, "")
	if version == "" {
		return nil
	}
	log := tui.ValidationLogger()
	target := filepath.Join(run.GetBinDir(), "ziti-"+version)

	c := m.MustSelectComponent("#ctrl1")

	source := envOrModelVar(m, self.source, self.sourceEnv)
	if source != "" {
		log.Infof("staging local ziti [%s] as the %s binary [%s]", source, self.version, target)
		return util.CopyFile(source, target)
	}

	ref := envOrModelVar(m, self.ref, self.refEnv)
	if ref != "" {
		log.Infof("pre-staging %s ziti built from ref %s as [%s]", self.version, ref, target)
		return stageziti.StageZitiFromRefOnce(run, c, ref, version, "")
	}

	log.Infof("pre-staging %s ziti binary [%s]", self.version, version)
	return stageziti.StageZitiOnce(run, c, version, "")
}

// upgradeController upgrades ctrl1 from fromVersion to toVersion in place: it swaps the controller
// binary and restarts. The controller config is unchanged, so the 2.0 binary auto-binds the edge-oidc
// API and OIDC becomes available with no config change.
func upgradeController(run model.Run) error {
	m := run.GetModel()
	toVersion := m.MustStringVariable("toVersion")
	log := tui.ValidationLogger()

	c := m.MustSelectComponent("#ctrl1")
	ctrlType, ok := c.Type.(*zitilab.ControllerType)
	if !ok {
		return fmt.Errorf("component ctrl1 is not a controller")
	}

	log.Infof("upgrading ctrl1 to %s", toVersion)
	ctrlType.SetVersion(toVersion)

	// RestartSelected waits for the old process to exit before starting the new one; a plain stop/start
	// would race, since Start no-ops while the old (pre-upgrade) process is still shutting down
	if err := chaos.RestartSelected(run, 1, c); err != nil {
		return err
	}
	if err := edge.ControllerAvailable("#ctrl1", 60*time.Second).Execute(run); err != nil {
		return err
	}
	return edge.Login("#ctrl1").Execute(run)
}

// upgradeRouters upgrades the edge routers from fromVersion to toVersion in place.
func upgradeRouters(run model.Run) error {
	return upgradeRoutersTo(run, run.GetModel().MustStringVariable("toVersion"))
}

// upgradeRoutersTo upgrades the edge routers to version in place, one at a time so traffic keeps
// flowing through the others. Each router restart briefly disrupts circuits on that router, which is
// expected; the others carry traffic in the meantime.
func upgradeRoutersTo(run model.Run, version string) error {
	m := run.GetModel()
	log := tui.ValidationLogger()

	routers := m.SelectComponents(models.EdgeRouterTag)
	for _, c := range routers {
		rt, ok := c.Type.(*zitilab.RouterType)
		if !ok {
			continue
		}
		log.Infof("upgrading router %s to %s", c.Id, version)
		rt.SetVersion(version)
		if err := chaos.RestartSelected(run, 1, c); err != nil {
			return err
		}
		// give the router time to reconnect and re-establish terminators before moving to the next
		time.Sleep(15 * time.Second)
	}
	return nil
}

// zetRestartWorkaroundMaxVersion is the newest ziti-edge-tunnel version known to need the
// post-controller-upgrade restart workaround. At or below this version, ziti-edge-tunnel does not
// rebuild its edge sessions after the 1.x->2.x controller upgrade invalidates them (the required JWT
// session migration): it keeps failing dials with "session closed" until its api-session refresh
// (~20 minutes), far outside the steady-state gate window. Restarting it forces a fresh authentication
// and valid sessions. Newer versions get no workaround, so if the gap is not actually fixed there the
// gate still catches it. Only raise this once a newer ziti-edge-tunnel is confirmed to self-recover.
const zetRestartWorkaroundMaxVersion = "v1.18.0"

// zetNeedsRestartWorkaround reports whether zetVersion is old enough to need the post-upgrade ZET
// restart workaround. An empty or unparsable version is treated as needing it, so an unknown build
// is not silently left in the broken state.
func zetNeedsRestartWorkaround(zetVersion string) bool {
	v, err := versions.ParseSemVer(zetVersion)
	if err != nil {
		return true
	}
	return v.CompareTo(versions.MustParseSemVer(zetRestartWorkaroundMaxVersion)) <= 0
}

// restartZetWorkaround restarts every ziti-edge-tunnel instance after the controller upgrade, but only
// when zetVersion is old enough to need it (see zetRestartWorkaroundMaxVersion). Those versions cannot
// recover their edge sessions on their own after the 1.x->2.x session migration, so the restart forces
// a re-auth and the ZET data path recovers within the gate window instead of after the api-session
// refresh. This deliberately breaks ZET traffic continuity across the controller upgrade, which is the
// known limitation being worked around.
func restartZetWorkaround(run model.Run) error {
	m := run.GetModel()
	zetVersion := m.GetStringVariableOr("zetVersion", "")
	log := tui.ValidationLogger()

	if !zetNeedsRestartWorkaround(zetVersion) {
		log.Infof("ziti-edge-tunnel %s is newer than %s; skipping post-upgrade ZET restart workaround",
			zetVersion, zetRestartWorkaroundMaxVersion)
		return nil
	}

	zets := m.SelectComponents(".zet")
	log.Infof("ziti-edge-tunnel %s needs the post-2.0-migration restart workaround; restarting %d ZET instance(s)",
		zetVersion, len(zets))
	return chaos.RestartSelected(run, 10, zets...)
}
