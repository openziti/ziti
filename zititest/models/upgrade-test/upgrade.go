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

	"github.com/openziti/fablab/kernel/lib/actions/component"
	"github.com/openziti/fablab/kernel/lib/tui"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/foundation/v2/versions"
	"github.com/openziti/ziti/v2/ziti/util"
	"github.com/openziti/ziti/zititest/zitilab"
	"github.com/openziti/ziti/zititest/zitilab/actions/edge"
	"github.com/openziti/ziti/zititest/zitilab/models"
	"github.com/openziti/ziti/zititest/zitilab/stageziti"
)

// stageToVersionBinary pre-stages the toVersion ziti binary into the kit so it is rsynced to every
// host alongside the fromVersion binary. In-place upgrades then only require a version swap and a
// restart, with no mid-run binary transfer.
//
// The binary is chosen in precedence order:
//   - toVersionSource (or ZITI_TOVERSION_PATH): a locally-built ziti binary, copied under the toVersion
//     name. Use this to run a patched controller/router from a binary you already built.
//   - toVersionRef (or ZITI_TOVERSION_REF): a git ref (branch, tag, or SHA) on openziti/ziti, built
//     from source via acquire and staged under the toVersion name. Use this to run an unmerged fix
//     without hand-building.
//   - otherwise the released toVersion is downloaded.
func stageToVersionBinary(run model.Run) error {
	m := run.GetModel()
	toVersion := m.GetStringVariableOr("toVersion", "")
	if toVersion == "" {
		return nil
	}
	log := tui.ValidationLogger()
	target := filepath.Join(run.GetBinDir(), "ziti-"+toVersion)

	c := m.MustSelectComponent("#ctrl1")

	source := m.GetStringVariableOr("toVersionSource", "")
	if source == "" {
		source = os.Getenv("ZITI_TOVERSION_PATH")
	}
	if source != "" {
		log.Infof("staging local ziti [%s] as toVersion binary [%s]", source, target)
		return util.CopyFile(source, target)
	}

	ref := m.GetStringVariableOr("toVersionRef", "")
	if ref == "" {
		ref = os.Getenv("ZITI_TOVERSION_REF")
	}
	if ref != "" {
		log.Infof("pre-staging toVersion ziti built from ref %s as [%s]", ref, target)
		return stageziti.StageZitiFromRefOnce(run, c, ref, toVersion, "")
	}

	log.Infof("pre-staging toVersion ziti binary [%s]", toVersion)
	return stageziti.StageZitiOnce(run, c, toVersion, "")
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

	if err := component.Stop("#ctrl1").Execute(run); err != nil {
		return err
	}
	if err := component.Start("#ctrl1").Execute(run); err != nil {
		return err
	}
	if err := edge.ControllerAvailable("#ctrl1", 60*time.Second).Execute(run); err != nil {
		return err
	}
	return edge.Login("#ctrl1").Execute(run)
}

// upgradeRouters upgrades the edge routers from fromVersion to toVersion in place, one at a time so
// traffic keeps flowing through the others. Each router restart briefly disrupts circuits on that
// router, which is expected; the others carry traffic in the meantime.
func upgradeRouters(run model.Run) error {
	m := run.GetModel()
	toVersion := m.MustStringVariable("toVersion")
	log := tui.ValidationLogger()

	routers := m.SelectComponents(models.EdgeRouterTag)
	for _, c := range routers {
		rt, ok := c.Type.(*zitilab.RouterType)
		if !ok {
			continue
		}
		log.Infof("upgrading router %s to %s", c.Id, toVersion)
		rt.SetVersion(toVersion)
		if err := component.Stop("#" + c.Id).Execute(run); err != nil {
			return err
		}
		if err := component.Start("#" + c.Id).Execute(run); err != nil {
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
// restart workaround. An empty or unparseable version is treated as needing it, so an unknown build
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
	if err := component.StopInParallel(".zet", 10).Execute(run); err != nil {
		return err
	}
	return component.StartInParallel(".zet", 10).Execute(run)
}
