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

package database

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/openziti/ziti-db-explorer/cmd/ziti-db-explorer/zdecli"
	"github.com/openziti/ziti/ziti/util"
	"github.com/spf13/cobra"
	"go.etcd.io/bbolt"
)

const (
	gzipMagic0 = 0x1f
	gzipMagic1 = 0x8b
)

func isGzipFile(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer func() { _ = f.Close() }()

	buf := make([]byte, 2)
	_, err = io.ReadFull(f, buf)
	if err != nil {
		return false, err
	}
	return buf[0] == gzipMagic0 && buf[1] == gzipMagic1, nil
}

func resolveEdgeDbPath(input string) (string, func(), error) {
	path := input
	cleanup := func() {}

	stat, err := os.Stat(input)
	if err != nil {
		return "", nil, err
	}

	if stat.IsDir() {
		// In HA/raft mode the controller state machine persists state to a BoltDB file in the raft data dir.
		candidate := filepath.Join(input, "ctrl-ha.db")
		if _, err := os.Stat(candidate); err != nil {
			return "", nil, fmt.Errorf("raft data dir '%s' does not contain ctrl-ha.db (%w)", input, err)
		}
		return candidate, cleanup, nil
	}

	// Support snapshot files created by `ziti edge db snapshot`. Those are often BoltDB snapshots,
	// but in clustered/raft mode the snapshot may be gzip-compressed.
	isGz, err := isGzipFile(input)
	if err != nil {
		return "", nil, err
	}
	if !isGz {
		return path, cleanup, nil
	}

	f, err := os.Open(input)
	if err != nil {
		return "", nil, err
	}
	defer func() { _ = f.Close() }()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return "", nil, fmt.Errorf("unable to open gzip snapshot '%s': %w", input, err)
	}
	defer func() { _ = gr.Close() }()

	data, err := io.ReadAll(gr)
	if err != nil {
		return "", nil, fmt.Errorf("unable to read gzip snapshot '%s': %w", input, err)
	}
	if len(data) == 0 {
		return "", nil, fmt.Errorf("gzip snapshot '%s' is empty", input)
	}

	tmp, err := os.CreateTemp("", "ziti-edge-db-snapshot-*.db")
	if err != nil {
		return "", nil, err
	}
	cleanup = func() { _ = os.Remove(tmp.Name()) }
	defer func() { _ = tmp.Close() }()

	if _, err := io.Copy(tmp, bytes.NewReader(data)); err != nil {
		cleanup()
		return "", nil, err
	}

	return tmp.Name(), cleanup, nil
}

func NewCmdDb(out io.Writer, errOut io.Writer) *cobra.Command {
	cmd := util.NewEmptyParentCmd("db", "Interact with Ziti database files")

	exploreCmd := &cobra.Command{
		Use:   "explore <ctrl.db>|help|version",
		Short: "Interactive CLI to explore Ziti database files",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if err := zdecli.Run("ziti db explore", args[0]); err != nil {
				_, _ = fmt.Fprintf(errOut, "Error: %s", err)
			}
		},
	}

	cmd.AddCommand(exploreCmd)
	cmd.AddCommand(NewDatastoreVersionCmd(out))
	cmd.AddCommand(NewCompactAction())
	cmd.AddCommand(NewDiskUsageAction())
	cmd.AddCommand(NewAddDebugAdminAction())
	cmd.AddCommand(NewAnonymizeAction())
	cmd.AddCommand(NewDeleteSessionsFromDbCmd())

	return cmd
}

func NewDatastoreVersionCmd(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "datastore-version <bolt-db-file|raft-data-dir|snapshot-file>",
		Short: "Print the current edge datastore version from a controller datastore (BoltDB file or raft data dir)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, cleanup, err := resolveEdgeDbPath(args[0])
			if err != nil {
				return err
			}
			defer cleanup()

			opts := *bbolt.DefaultOptions
			opts.ReadOnly = true

			db, err := bbolt.Open(path, 0400, &opts)
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()

			return db.View(func(tx *bbolt.Tx) error {
				root := tx.Bucket([]byte(RootBucketName))
				if root == nil {
					return fmt.Errorf("root '%s' bucket not found", RootBucketName)
				}
				versionBucket := root.Bucket([]byte("version"))
				if versionBucket == nil {
					return fmt.Errorf("'version' bucket not found")
				}
				val := versionBucket.Get([]byte("edge"))
				if val == nil {
					return fmt.Errorf("'edge' version not found")
				}
				// Historically stored as ascii digits
				if _, err := strconv.Atoi(string(val)); err != nil {
					return fmt.Errorf("invalid edge version value '%s': %w", string(val), err)
				}
				_, _ = fmt.Fprintln(out, string(val))
				return nil
			})
		},
	}
}
