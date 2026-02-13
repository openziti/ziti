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

	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/v2/controller/db"
	"github.com/spf13/cobra"
)

func NewGetDbVersionAction() *cobra.Command {
	return &cobra.Command{
		Use:   "get-db-version <db-path>",
		Short: "Show the edge database version",
		Args:  cobra.ExactArgs(1),
		RunE:  runGetDbVersion,
	}
}

func runGetDbVersion(_ *cobra.Command, args []string) error {
	version, err := getEdgeDbVersion(args[0])
	if err != nil {
		return err
	}
	fmt.Printf("edge database version: %d\n", version)
	return nil
}

func getEdgeDbVersion(dbPath string) (int, error) {
	boltDb, err := db.Open(dbPath)
	if err != nil {
		return 0, fmt.Errorf("failed to open database %q: %w", dbPath, err)
	}
	defer func() { _ = boltDb.Close() }()

	mm := boltz.NewMigratorManager(boltDb)
	version, err := mm.GetComponentVersion("edge")
	if err != nil {
		return 0, fmt.Errorf("failed to read current version: %w", err)
	}
	return version, nil
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

	currentVersion, err := getEdgeDbVersion(dbPath)
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
