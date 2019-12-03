/*
	Copyright 2019 Netfoundry, Inc.

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

package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"io"
	"os"
	"strings"

	cmdutil "github.com/netfoundry/ziti-cmd/ziti/cmd/ziti/cmd/factory"
	cmdhelper "github.com/netfoundry/ziti-cmd/ziti/cmd/ziti/cmd/helpers"
	"github.com/netfoundry/ziti-cmd/ziti/cmd/ziti/cmd/templates"
	"github.com/netfoundry/ziti-cmd/ziti/cmd/ziti/util"
)

var (
	deleteStateStoreLong = templates.LongDesc(`
		Deletes the ziti state-store from S3
`)

	deleteStateStoreExample = templates.Examples(`
		# Delete the ziti state store from S3
		ziti delete state-store

	`)
)

// DeleteStateStoreOptions the options for the delete state-store command
type DeleteStateStoreOptions struct {
	DeleteOptions
}

// NewCmdDeleteStateStore creates a command object for the "delete" command
func NewCmdDeleteStateStore(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	options := &DeleteStateStoreOptions{
		DeleteOptions: DeleteOptions{
			CommonOptions: CommonOptions{
				Factory: f,
				Out:     out,
				Err:     errOut,
			},
		},
	}

	cmd := &cobra.Command{
		Use:     "state-store",
		Short:   "Delete a ziti state-store",
		Aliases: []string{"ss"},
		Long:    deleteStateStoreLong,
		Example: deleteStateStoreExample,
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			cmdhelper.CheckErr(err)
		},
	}

	options.addCommonFlags(cmd)

	return cmd
}

// Run implements the command
func (o *DeleteStateStoreOptions) Run() error {

	var bucketName string

	if bucketName = os.Getenv("ZITI_STATE_STORE"); bucketName == "" {
		return fmt.Errorf("ZITI_STATE_STORE environment variable is not set")
	}

	fmt.Println("\nZITI_STATE_STORE environment variable contains '" + bucketName + "'")

	bucketName = strings.Replace(bucketName, "s3://", "", 1)

	message := fmt.Sprintf("Are you sure you want to remove S3 bucket '%s' ?", bucketName)
	if util.Confirm(message, true, "Please indicate if you would like to remove this S3 bucket.") {

		err := util.RunAWSCommand("s3api", "delete-bucket", "--bucket", bucketName)
		if err != nil {
			return fmt.Errorf("Unable to delete state-store bucket '%s': '%s'", bucketName, err)
		}

		fmt.Println("\nSuccessfully removed S3 bucket '" + bucketName + "'")
	}

	return nil
}
