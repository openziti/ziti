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
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/openziti/fablab/kernel/lib/tui"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/foundation/v2/netz"
	"github.com/openziti/ziti/zititest/zitilab"
	"github.com/openziti/ziti/zititest/zitilab/actions/edge"
	"github.com/openziti/ziti/zititest/zitilab/chaos"
)

// cluster upgrade modes, selected by the clusterUpgradeMode variable.
const (
	// clusterUpgradeRolling restarts one node at a time, so the cluster runs mixed versions until
	// the last node is done.
	clusterUpgradeRolling = "rolling"
	// clusterUpgradeAllAtOnce restarts every node together, so no node sees a version mismatch but
	// every node pays its cold-start cost simultaneously.
	clusterUpgradeAllAtOnce = "all-at-once"
)

const (
	// clusterPortUpTimeout bounds how long a restarted node may take to listen again. A node that
	// keeps dying and restarting can flap the port open, so this is a floor on detection, not a
	// health check; clusterSettleTimeout is what actually decides health.
	clusterPortUpTimeout = 2 * time.Minute
	// clusterSettleTimeout bounds how long the cluster may take to report every peer connected with
	// a leader elected. A node stuck in a crash loop never gets there, which is how a node that
	// cannot stay up surfaces as a failure rather than a hang.
	clusterSettleTimeout = 3 * time.Minute
)

// ansiEscape matches the terminal color codes the inspect command emits, which are not stripped when
// its output is captured over ssh on some platforms and would otherwise be taken for JSON.
var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// ctrlPeer is the part of a controller's connected-peers inspection this model uses.
type ctrlPeer struct {
	Id          string `json:"id"`
	IsLeader    bool   `json:"isLeader"`
	IsConnected bool   `json:"isConnected"`
}

// inspectPeers returns c's own view of the cluster. The inspect request is made with c's own binary,
// so the agent client always matches the controller it is talking to even while a rolling upgrade has
// different nodes on different versions.
func inspectPeers(c *model.Component) ([]ctrlPeer, error) {
	ctrlType, ok := c.Type.(*zitilab.ControllerType)
	if !ok {
		return nil, fmt.Errorf("component %s is not a controller", c.Id)
	}
	cmd := fmt.Sprintf("%s agent inspect -a %s connected-peers", ctrlType.GetBinaryPath(c), c.Id)
	out, err := c.GetHost().ExecLogged(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect peers on %s: %w", c.Id, err)
	}
	out = ansiEscape.ReplaceAllString(out, "")
	start := strings.Index(out, "[")
	end := strings.LastIndex(out, "]")
	if start < 0 || end < start {
		return nil, fmt.Errorf("no peer list in the inspect output from %s: %s", c.Id, strings.TrimSpace(out))
	}
	var peers []ctrlPeer
	if err := json.Unmarshal([]byte(out[start:end+1]), &peers); err != nil {
		return nil, fmt.Errorf("unparsable peer list from %s: %w", c.Id, err)
	}
	return peers, nil
}

// clusterUpgradeOrder returns ctrls with the leader last, the ordering a rolling upgrade wants:
// restarting a follower costs nothing, while restarting the leader forces an election. When no node
// reports a leader, or reports one under an id that is not a component id, the bootstrap node stands
// in: it seeded the cluster and the others joined it, so it is the node most likely to be leading.
func clusterUpgradeOrder(ctrls []*model.Component) []*model.Component {
	leaderId := ""
	for _, c := range ctrls {
		peers, err := inspectPeers(c)
		if err != nil {
			continue
		}
		for _, p := range peers {
			if p.IsLeader {
				leaderId = p.Id
			}
		}
		if leaderId != "" {
			break
		}
	}

	var followers, leader []*model.Component
	for _, c := range ctrls {
		if c.Id == leaderId {
			leader = append(leader, c)
		} else {
			followers = append(followers, c)
		}
	}
	if len(leader) == 0 {
		for i, c := range followers {
			if c.HasTag("bootstrap-ctrl") {
				followers = append(followers[:i], followers[i+1:]...)
				leader = append(leader, c)
				break
			}
		}
	}
	return append(followers, leader...)
}

// checkClusterConnected reports whether every controller sees the full peer set connected with a
// leader elected.
func checkClusterConnected(ctrls []*model.Component) error {
	for _, c := range ctrls {
		peers, err := inspectPeers(c)
		if err != nil {
			return err
		}
		if len(peers) != len(ctrls) {
			return fmt.Errorf("%s sees %d peers, expected %d", c.Id, len(peers), len(ctrls))
		}
		hasLeader := false
		for _, p := range peers {
			if !p.IsConnected {
				return fmt.Errorf("%s reports peer %s as not connected", c.Id, p.Id)
			}
			hasLeader = hasLeader || p.IsLeader
		}
		if !hasLeader {
			return fmt.Errorf("%s reports no leader", c.Id)
		}
	}
	return nil
}

// waitForClusterConnected blocks until the cluster reports itself healthy or timeout elapses.
func waitForClusterConnected(ctrls []*model.Component, timeout time.Duration) error {
	log := tui.ValidationLogger()
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		lastErr = checkClusterConnected(ctrls)
		if lastErr == nil {
			log.Infof("cluster reports %d peers connected with a leader", len(ctrls))
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("cluster did not settle within %s: %w", timeout, lastErr)
		}
		time.Sleep(5 * time.Second)
	}
}

// restartCtrlOnVersion swaps c's binary to version and restarts it, waiting for its cluster port to
// come back. The config is untouched: a cluster node's config does not depend on the ziti version.
func restartCtrlOnVersion(run model.Run, c *model.Component, version string) error {
	ctrlType, ok := c.Type.(*zitilab.ControllerType)
	if !ok {
		return fmt.Errorf("component %s is not a controller", c.Id)
	}
	ctrlType.SetVersion(version)

	// RestartSelected waits for the old process to exit before starting the new one; a plain
	// stop/start would race, since Start no-ops while the old process is still shutting down
	if err := chaos.RestartSelected(run, 1, c); err != nil {
		return err
	}
	hostPort := c.GetHost().PublicIp + ":6262"
	if err := netz.WaitForPortActive(hostPort, clusterPortUpTimeout); err != nil {
		return fmt.Errorf("%s cluster port did not come back after upgrading to %s: %w", c.Id, version, err)
	}
	return nil
}

// rollingClusterUpgrade upgrades one node at a time, waiting for the cluster to settle before moving
// on, so a node that cannot survive the upgrade fails the phase at the node that broke.
func rollingClusterUpgrade(run model.Run, ctrls []*model.Component, version string) error {
	log := tui.ValidationLogger()
	for _, c := range clusterUpgradeOrder(ctrls) {
		log.Infof("upgrading cluster node %s to %s", c.Id, version)
		if err := restartCtrlOnVersion(run, c, version); err != nil {
			return err
		}
		if err := waitForClusterConnected(ctrls, clusterSettleTimeout); err != nil {
			return fmt.Errorf("cluster unhealthy after upgrading %s to %s: %w", c.Id, version, err)
		}
	}
	return nil
}

// allAtOnceClusterUpgrade restarts every node on the new version together, so the cluster never runs
// mixed versions.
func allAtOnceClusterUpgrade(run model.Run, ctrls []*model.Component, version string) error {
	log := tui.ValidationLogger()
	for _, c := range ctrls {
		ctrlType, ok := c.Type.(*zitilab.ControllerType)
		if !ok {
			return fmt.Errorf("component %s is not a controller", c.Id)
		}
		ctrlType.SetVersion(version)
	}

	log.Infof("restarting all %d cluster nodes on %s at once", len(ctrls), version)
	if err := chaos.RestartSelected(run, len(ctrls), ctrls...); err != nil {
		return err
	}
	for _, c := range ctrls {
		hostPort := c.GetHost().PublicIp + ":6262"
		if err := netz.WaitForPortActive(hostPort, clusterPortUpTimeout); err != nil {
			return fmt.Errorf("%s cluster port did not come back after upgrading to %s: %w", c.Id, version, err)
		}
	}
	return waitForClusterConnected(ctrls, clusterSettleTimeout)
}

// upgradeClusterControllers upgrades the whole controller cluster from toVersion to nextVersion,
// reporting each node's peak memory over the upgrade. clusterUpgradeMode selects between a rolling
// upgrade and an all-at-once one.
//
// No steady-state gate runs between nodes in rolling mode. Peers hold the cluster read-only while a
// version mismatch exists, so anything needing a write would fail for reasons this phase is not
// testing; the gate runs once the whole cluster is on nextVersion.
func upgradeClusterControllers(run model.Run) error {
	m := run.GetModel()
	log := tui.ValidationLogger()

	nextVersion := m.GetStringVariableOr("nextVersion", "")
	if nextVersion == "" {
		log.Infof("nextVersion is not set, skipping the cluster upgrade")
		return nil
	}

	ctrls := ctrlComponents(m)
	if len(ctrls) == 0 {
		return fmt.Errorf("no controllers found to upgrade")
	}

	mode := m.GetStringVariableOr("clusterUpgradeMode", clusterUpgradeRolling)
	// the steady-state gate just ran, so the window immediately behind us is settled traffic on
	// toVersion, which is the baseline each node's upgrade is measured against
	upgradeStart := time.Now()
	baselineStart := upgradeStart.Add(-ctrlMemBaselineWindow)

	var err error
	switch mode {
	case clusterUpgradeRolling:
		err = rollingClusterUpgrade(run, ctrls, nextVersion)
	case clusterUpgradeAllAtOnce:
		err = allAtOnceClusterUpgrade(run, ctrls, nextVersion)
	default:
		return fmt.Errorf("unknown clusterUpgradeMode [%s], expected %s or %s",
			mode, clusterUpgradeRolling, clusterUpgradeAllAtOnce)
	}

	// report either way: a node that died on the way up is the case the samples exist to explain
	if reportErr := reportCtrlMemoryUpgrade(run, baselineStart, upgradeStart, "cluster upgrade to "+nextVersion); reportErr != nil {
		if err == nil {
			return reportErr
		}
		log.WithError(reportErr).Warn("controller memory report after a failed cluster upgrade")
	}
	if err != nil {
		return err
	}
	return edge.Login("#ctrl1").Execute(run)
}

// upgradeClusterRouters rolls the routers onto nextVersion, completing the upgrade the controllers
// started.
func upgradeClusterRouters(run model.Run) error {
	nextVersion := run.GetModel().GetStringVariableOr("nextVersion", "")
	if nextVersion == "" {
		tui.ValidationLogger().Infof("nextVersion is not set, skipping the router upgrade")
		return nil
	}
	return upgradeRoutersTo(run, nextVersion)
}

// upgradeToNextVersion runs the optional cluster upgrade phase: the controllers, then the routers,
// with a steady-state gate after each. The whole phase is skipped when nextVersion is unset, so an
// iteration that ends at toVersion does not pay for two extra gates.
func upgradeToNextVersion(run model.Run) error {
	m := run.GetModel()
	if m.GetStringVariableOr("nextVersion", "") == "" {
		tui.ValidationLogger().Infof("nextVersion is not set, skipping the cluster upgrade phase")
		return nil
	}
	return m.Exec(run,
		"upgradeClusterControllers",          // 3-node cluster toVersion -> nextVersion
		"validateSteadyStateAfterDisruption", // cluster back, terminators + traffic recovered
		"upgradeClusterRouters",              // routers toVersion -> nextVersion, rolling one at a time
		"validateSteadyStateAfterDisruption", // whole system on nextVersion and clean
	)
}
