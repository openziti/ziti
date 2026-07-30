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
	"net"
	"path/filepath"
	"time"

	"github.com/openziti/fablab/kernel/lib/actions/component"
	"github.com/openziti/fablab/kernel/lib/tui"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/foundation/v2/netz"
	"github.com/openziti/ziti/zititest/zitilab"
	"github.com/openziti/ziti/zititest/zitilab/actions/edge"
)

// pushControllerConfig re-renders a controller's config and copies just that file to its host. It
// deliberately avoids the whole-kit rsync (RsyncStaged), which does a --delete sync and would wipe
// runtime state on the host, including the standalone bbolt db we migrate from and the raft data dir.
func pushControllerConfig(run model.Run, c *model.Component, ctrlType *zitilab.ControllerType) error {
	if err := ctrlType.StageFiles(run, c); err != nil {
		return err
	}
	configName := ctrlType.ConfigName
	if configName == "" {
		configName = c.Id + ".yml"
	}
	local := filepath.Join(run.GetConfigDir(), configName)
	remote := fmt.Sprintf("/home/%s/fablab/cfg/%s", c.GetHost().GetSshUser(), configName)
	return c.GetHost().SendFile(local, remote)
}

// waitForPortInactive blocks until a TCP dial to address fails (the port stops accepting) or the
// timeout elapses. It is the inverse of netz.WaitForPortActive, used to detect a controller exiting
// after a snapshot restore so we know when to restart it.
func waitForPortInactive(address string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		conn, err := net.DialTimeout("tcp", address, 2*time.Second)
		if err != nil {
			return nil
		}
		_ = conn.Close()
		if time.Now().After(deadline) {
			return fmt.Errorf("port %s still active after %s", address, timeout)
		}
		time.Sleep(time.Second)
	}
}

// upgradeControllerToHa converts the standalone ctrl1 into a single-node HA cluster in place: it flips
// ctrl1 to cluster mode, re-renders its config (with the cluster directive plus the migrateDb source
// so raft is seeded from the existing standalone bbolt), pushes it, then restarts. The controller
// snapshots its bbolt into raft, self-restarts to apply it, and comes up as a bootstrapped one-node
// cluster with the existing identities, services, and terminators intact.
func upgradeControllerToHa(run model.Run) error {
	m := run.GetModel()
	log := tui.ValidationLogger()

	c := m.MustSelectComponent("#ctrl1")
	ctrlType, ok := c.Type.(*zitilab.ControllerType)
	if !ok {
		return fmt.Errorf("component ctrl1 is not a controller")
	}

	log.Infof("converting ctrl1 to a single-node HA cluster")
	// cluster mode makes ctrl.yml.tmpl emit the cluster (raft) directive; migrateDb also emits the
	// source db line so the controller seeds raft from the existing standalone bbolt on first start.
	c.PutVariable("cluster", true)
	c.PutVariable("migrateDb", true)

	if err := pushControllerConfig(run, c, ctrlType); err != nil {
		return err
	}

	if err := component.Stop("#ctrl1").Execute(run); err != nil {
		return err
	}
	if err := component.Start("#ctrl1").Execute(run); err != nil {
		return err
	}
	// the migration snapshot triggers a self-restart (restartSelfOnSnapshot), so allow extra time for
	// the controller to come back up bootstrapped before checking availability and logging in.
	if err := edge.ControllerAvailable("#ctrl1", 120*time.Second).Execute(run); err != nil {
		return err
	}
	return edge.Login("#ctrl1").Execute(run)
}

// addClusterNodes brings ctrl2 and ctrl3 online and joins them to ctrl1's cluster. Each is staged on
// the upgraded (toVersion) binary in cluster mode and started, then the leader runs "agent cluster add"
// for each node's control address, growing the single-node cluster to three.
func addClusterNodes(run model.Run) error {
	m := run.GetModel()
	toVersion := m.MustStringVariable("toVersion")
	log := tui.ValidationLogger()

	nodes := m.SelectComponents(".cluster-node")
	if len(nodes) == 0 {
		return fmt.Errorf("no cluster-node controllers found to add")
	}

	// a joining node must run the same (upgraded) binary as the leader and be in cluster mode; it does
	// not migrate a db (no migrateDb), it replicates state from the leader after being added.
	for _, c := range nodes {
		ctrlType, ok := c.Type.(*zitilab.ControllerType)
		if !ok {
			return fmt.Errorf("component %s is not a controller", c.Id)
		}
		c.PutVariable("cluster", true)
		ctrlType.SetVersion(toVersion)
		if err := pushControllerConfig(run, c, ctrlType); err != nil {
			return err
		}
	}

	if err := component.StartInParallel(".cluster-node", 5).Execute(run); err != nil {
		return err
	}

	// add each new node from the leader (ctrl1); the controller advertises cluster membership on :6262
	leader := m.MustSelectComponent("#ctrl1")
	leaderType, ok := leader.Type.(*zitilab.ControllerType)
	if !ok {
		return fmt.Errorf("component ctrl1 is not a controller")
	}
	leaderBinary := leaderType.GetBinaryPath(leader)
	for _, c := range nodes {
		hostPort := c.GetHost().PublicIp + ":6262"

		// StartInParallel returns once the process is launched, not once it is listening, so wait for
		// the node's cluster port to be open before adding it rather than retrying a membership change.
		log.Infof("waiting for %s cluster port %s", c.Id, hostPort)
		if err := netz.WaitForPortActive(hostPort, 90*time.Second); err != nil {
			return fmt.Errorf("cluster port for %s (%s) never became available: %w", c.Id, hostPort, err)
		}

		log.Infof("adding %s to the cluster at tls:%s", c.Id, hostPort)
		cmd := fmt.Sprintf("%s agent cluster add tls:%s", leaderBinary, hostPort)
		if err := leader.GetHost().ExecLogOnlyOnError(cmd); err != nil {
			return fmt.Errorf("failed to add %s to the cluster at tls:%s: %w", c.Id, hostPort, err)
		}

		// The leader streams its db to the joining node, which restores the snapshot and then exits
		// (controller/network RestoreSnapshot exits the process expecting an external restart; it does
		// not self-restart). Wait for that exit, then restart the node so it comes back up with the
		// restored data and rejoins as a functional voting member.
		log.Infof("waiting for %s to restore the cluster snapshot and exit", c.Id)
		if err := waitForPortInactive(hostPort, 90*time.Second); err != nil {
			return fmt.Errorf("%s did not restart after joining (snapshot restore expected): %w", c.Id, err)
		}
		log.Infof("restarting %s to rejoin with the restored data", c.Id)
		if err := component.Start("#" + c.Id).Execute(run); err != nil {
			return err
		}
		if err := netz.WaitForPortActive(hostPort, 90*time.Second); err != nil {
			return fmt.Errorf("%s cluster port did not come back after restart: %w", c.Id, err)
		}
		// let the membership change commit and replicate before adding the next node
		time.Sleep(10 * time.Second)
	}

	// give raft time to settle the 3-node cluster before the stability gate runs
	log.Infof("waiting for the 3-node cluster to settle")
	time.Sleep(30 * time.Second)
	return nil
}
