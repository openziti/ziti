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
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/openziti/ziti/v2/controller/db"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"github.com/spf13/cobra"
	"go.etcd.io/bbolt"
)

func NewGetDbVersionAction() *cobra.Command {
	return &cobra.Command{
		Use:   "get-db-version <db-path>",
		Short: "Show the edge database version and the migrations applied to it",
		Args:  cobra.ExactArgs(1),
		RunE:  runGetDbVersion,
	}
}

func runGetDbVersion(_ *cobra.Command, args []string) error {
	version, applied, err := readEdgeDbVersion(args[0])
	if err != nil {
		return err
	}
	fmt.Printf("edge database version: %d\n", version)
	if len(applied) == 0 {
		fmt.Println("no named migrations recorded")
		return nil
	}

	t := table.NewWriter()
	t.SetStyle(table.StyleRounded)
	t.AppendHeader(table.Row{"Migration", "Origin", "Applied At", "Applied By"})
	for _, migration := range applied {
		appliedAt := "unknown"
		if !migration.AppliedAt.IsZero() {
			appliedAt = migration.AppliedAt.Format(time.RFC3339)
		}
		t.AppendRow(table.Row{migration.Name, migration.Version, appliedAt, migration.AppliedBy})
	}
	fmt.Println(t.Render())
	return nil
}

// readEdgeDbVersion returns the edge version of the database at dbPath and the named migrations
// recorded as applied to it, in name order. It doesn't modify the database.
func readEdgeDbVersion(dbPath string) (int, []*db.AppliedMigration, error) {
	boltDb, err := db.Open(dbPath)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to open database %q: %w", dbPath, err)
	}
	defer func() { _ = boltDb.Close() }()

	var version int
	var applied []*db.AppliedMigration
	err = boltDb.View(func(tx *bbolt.Tx) error {
		version = db.ReadEdgeVersion(tx)
		var loadErr error
		applied, loadErr = db.LoadAppliedMigrations(tx)
		return loadErr
	})
	if err != nil {
		return 0, nil, fmt.Errorf("failed to read edge database version: %w", err)
	}
	return version, applied, nil
}

func NewSetDbVersionAction() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "set-db-version <db-path> <version>",
		Short:  "Set the edge database version",
		Args:   cobra.ExactArgs(2),
		Hidden: true,
		RunE:   runSetDbVersion,
	}
	return cmd
}

func runSetDbVersion(_ *cobra.Command, args []string) error {
	dbPath := args[0]
	targetVersion, err := strconv.Atoi(args[1])
	if err != nil {
		return fmt.Errorf("invalid version %q: %w", args[1], err)
	}

	fmt.Printf("" +
		"This command is meant for development and testing only.\n" +
		"If you use this against a production system without\n" +
		"knowing what you are doing, you may cause data\n" +
		"corruption in your database.\n" +
		"\nDo you wish to continue? [y/N] ")

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	response = strings.TrimSpace(strings.ToLower(response))
	if response != "y" && response != "yes" {
		fmt.Println("Aborted.")
		return nil
	}

	currentVersion, _, err := readEdgeDbVersion(dbPath)
	if err != nil {
		return err
	}

	boltDb, err := db.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database %q: %w", dbPath, err)
	}
	defer func() { _ = boltDb.Close() }()

	err = boltDb.Update(nil, func(ctx boltz.MutateContext) error {
		versionsBucket := boltz.GetOrCreatePath(ctx.Tx(), db.RootBucket, "versions")
		if versionsBucket.HasError() {
			return versionsBucket.GetError()
		}
		versionsBucket.SetInt64("edge", int64(targetVersion), nil)
		return versionsBucket.GetError()
	})
	if err != nil {
		return fmt.Errorf("failed to set version: %w", err)
	}

	fmt.Printf("database version updated from %d to %d\n", currentVersion, targetVersion)
	return nil
}
