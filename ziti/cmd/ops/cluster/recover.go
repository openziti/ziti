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

package cluster

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb/v2"
	"github.com/openziti/ziti/v2/controller/command"
	"github.com/openziti/ziti/v2/controller/config"
	"github.com/openziti/ziti/v2/controller/event"
	zitiraft "github.com/openziti/ziti/v2/controller/raft"
	"github.com/spf13/cobra"
)

const recoverLong = `Force the local raft state at the controller's data directory to a single-node
configuration so the surviving controller can come back up after losing quorum.

Use this when a cluster has lost quorum (e.g. one of two nodes is permanently
gone) and 'ziti agent cluster add'/'cluster remove' fail with "no leader". After
recovery, restart the controller and add new peers with 'ziti agent cluster add'.

The controller process MUST be stopped before running this command. The
operation is destructive: the existing peer membership recorded in the raft log
is discarded.`

// NewCmdRecover builds the 'ziti ops cluster recover' command, which calls
// raft.RecoverCluster on a stopped controller's data directory to force the
// configuration to a single local node.
func NewCmdRecover(out io.Writer, errOut io.Writer) *cobra.Command {
	var skipConfirm bool

	cmd := &cobra.Command{
		Use:   "recover <controller-config>",
		Short: "Force the local raft state to a single-node configuration to recover from quorum loss",
		Long:  recoverLong,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRecover(out, os.Stdin, args[0], skipConfirm)
		},
	}

	cmd.Flags().BoolVar(&skipConfirm, "yes", false, "skip the confirmation prompt")

	return cmd
}

func runRecover(out io.Writer, in io.Reader, configPath string, skipConfirm bool) error {
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load controller config %q: %w", configPath, err)
	}
	if cfg.Raft == nil {
		return fmt.Errorf("controller config %q has no 'cluster' section; this controller is not configured for HA", configPath)
	}
	if cfg.Id == nil {
		return fmt.Errorf("controller config %q has no identity loaded; cannot determine local raft ID", configPath)
	}
	if cfg.Raft.AdvertiseAddress == nil {
		return fmt.Errorf("controller config %q has no cluster advertise address", configPath)
	}

	localID := raft.ServerID(cfg.Id.Token)
	localAddr := raft.ServerAddress(cfg.Raft.AdvertiseAddress.String())
	dataDir := cfg.Raft.DataDir

	_, _ = fmt.Fprintf(out, "About to force the raft configuration in %s to a single node:\n", dataDir)
	_, _ = fmt.Fprintf(out, "  ID:      %s\n", localID)
	_, _ = fmt.Fprintf(out, "  Address: %s\n", localAddr)
	_, _ = fmt.Fprintln(out, "Existing peer membership recorded in the raft log will be discarded.")
	_, _ = fmt.Fprintln(out, "The controller process must be stopped before continuing.")

	if !skipConfirm {
		_, _ = fmt.Fprint(out, "Continue? [y/N]: ")
		reader := bufio.NewReader(in)
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("failed to read confirmation: %w", err)
		}
		answer := strings.ToLower(strings.TrimSpace(line))
		if answer != "y" && answer != "yes" {
			return fmt.Errorf("recovery aborted by user")
		}
	}

	if err := recoverDataDir(dataDir, localID, localAddr); err != nil {
		return err
	}

	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, "Cluster configuration recovered.")
	_, _ = fmt.Fprintln(out, "Restart the controller; it will start as a single-node cluster.")
	_, _ = fmt.Fprintln(out, "Add new peers with 'ziti agent cluster add'.")
	return nil
}

// recoverDataDir opens the raft stores under dataDir and calls
// raft.RecoverCluster to force the configuration to a single server matching
// localID and localAddr. The data directory must belong to a stopped
// controller — BoltDB takes an exclusive file lock.
func recoverDataDir(dataDir string, localID raft.ServerID, localAddr raft.ServerAddress) error {
	boltPath := filepath.Join(dataDir, "raft.db")
	boltStore, err := raftboltdb.NewBoltStore(boltPath)
	if err != nil {
		return fmt.Errorf("failed to open raft bolt store %q: %w", boltPath, err)
	}
	defer func() { _ = boltStore.Close() }()

	logger := zitiraft.NewHcLogrusLogger()
	snapshotStore, err := raft.NewFileSnapshotStoreWithLogger(dataDir, 5, logger)
	if err != nil {
		return fmt.Errorf("failed to open snapshot store under %q: %w", dataDir, err)
	}

	fsm := zitiraft.NewFsm(dataDir, false, command.GetDefaultDecoders(), zitiraft.NewIndexTracker(), event.DispatcherMock{})
	if err = fsm.Init(); err != nil {
		return fmt.Errorf("failed to initialize fsm at %q: %w", dataDir, err)
	}
	defer func() { _ = fsm.Close() }()

	raftConf := raft.DefaultConfig()
	raftConf.LocalID = localID
	raftConf.Logger = logger
	raftConf.NoSnapshotRestoreOnStart = true

	_, transport := raft.NewInmemTransport(localAddr)

	configuration := raft.Configuration{
		Servers: []raft.Server{{ID: localID, Address: localAddr, Suffrage: raft.Voter}},
	}

	// Update ctrl-ha.db's servers bucket BEFORE RecoverCluster runs, so the
	// snapshot file fsm.Snapshot() writes inside RecoverCluster captures the
	// corrected configuration. Otherwise any node that later joins via
	// InstallSnapshot would see the stale member list until a subsequent
	// LogConfiguration entry overwrites it.
	if err = fsm.OverwriteServers(configuration.Servers); err != nil {
		return fmt.Errorf("failed to update FSM-tracked servers list in ctrl-ha.db: %w", err)
	}

	if err = raft.RecoverCluster(raftConf, fsm, boltStore, boltStore, snapshotStore, transport, configuration); err != nil {
		return fmt.Errorf("raft.RecoverCluster failed: %w", err)
	}
	return nil
}
