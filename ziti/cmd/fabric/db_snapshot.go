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

package fabric

import (
	"context"
	"fmt"
	"github.com/openziti/ziti/controller/rest_client/database"
	"github.com/openziti/ziti/controller/rest_model"
	"github.com/openziti/ziti/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/util"
	"github.com/spf13/cobra"
)

type dbSnapshotOptions struct {
	api.Options
}

func newDbSnapshotCmd(p common.OptionsProvider) *cobra.Command {
	options := &dbSnapshotOptions{
		Options: api.Options{CommonOptions: p()},
	}

	cmd := &cobra.Command{
		Use:   "snapshot <snapshot file path>",
		Short: "Creates a database snapshot.",
		Long:  "Creates a database snapshot. The snapshot file destination may optionally be specified. The snapshot will be created on the controller's filesystem",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := runSnapshotDb(options)
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	options.AddCommonFlags(cmd)

	return cmd
}

func runSnapshotDb(o *dbSnapshotOptions) error {
	client, err := util.NewFabricManagementClient(o)
	if err != nil {
		return err
	}

	var path string
	if len(o.Args) > 0 {
		path = o.Args[0]
	}

	ok, err := client.Database.CreateDatabaseSnapshotWithPath(&database.CreateDatabaseSnapshotWithPathParams{
		Snapshot: &rest_model.DatabaseSnapshotCreate{
			Path: path,
		},
		Context: context.Background(),
	})

	if err != nil {
		return err
	}

	if !o.OutputJSONResponse {
		if ok != nil && ok.Payload != nil && ok.Payload.Data != nil && ok.Payload.Data.Path != nil {
			fmt.Println(*ok.Payload.Data.Path)
		} else {
			fmt.Printf("snapshot created successfully")
		}
	}
	return nil
}
