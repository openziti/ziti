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
	"os"

	"github.com/michaelquigley/pfxlog"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func main() {
	pfxlog.GlobalInit(logrus.WarnLevel, pfxlog.DefaultOptions())

	rootCmd := &cobra.Command{
		Use:   "migration-test",
		Short: "Tool for validating service collapse migration",
	}

	queryCmd := &cobra.Command{
		Use:   "query <db-path> <output.json>",
		Short: "Query all service data and write deterministic JSON snapshot",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runQuery(args[0], args[1])
		},
	}

	verifyCmd := &cobra.Command{
		Use:   "verify <db-path>",
		Short: "Run structural migration checks (CheckIntegrity, edge index removed, idempotent) against a DB",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVerify(args[0])
		},
	}

	rootCmd.AddCommand(queryCmd, verifyCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
