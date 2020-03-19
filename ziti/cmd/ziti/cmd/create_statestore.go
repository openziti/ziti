/*
	Copyright NetFoundry, Inc.

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
	"os/user"

	cmdutil "github.com/netfoundry/ziti-cmd/ziti/cmd/ziti/cmd/factory"
	cmdhelper "github.com/netfoundry/ziti-cmd/ziti/cmd/ziti/cmd/helpers"
	"github.com/netfoundry/ziti-cmd/ziti/cmd/ziti/cmd/templates"
	"github.com/netfoundry/ziti-cmd/ziti/cmd/ziti/util"
)

// const (
// 	optionCtrlAddress  = "ctrlAddress"
// 	defaultCtrlAddress = "quic:0.0.0.0:6262"
// )

var (
	createStateStoreLong = templates.LongDesc(`
		Creates the ziti state-store in S3
`)

	createStateStoreExample = templates.Examples(`
		# Create the ziti state store in S3
		ziti create state-store

	`)
)

// CreateStateStoreOptions the options for the create spring command
type CreateStateStoreOptions struct {
	CreateOptions
}

// NewCmdCreateStateStore creates a command object for the "create" command
func NewCmdCreateStateStore(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	options := &CreateStateStoreOptions{
		CreateOptions: CreateOptions{
			CommonOptions: CommonOptions{
				Factory: f,
				Out:     out,
				Err:     errOut,
			},
		},
	}

	cmd := &cobra.Command{
		Use:     "state-store",
		Short:   "Create a ziti state-store",
		Aliases: []string{"ss"},
		Long:    createStateStoreLong,
		Example: createStateStoreExample,
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
func (o *CreateStateStoreOptions) Run() error {

	var defaultBucketName string
	defaultBucketName = "ziti-state-store"

	bucketName, err := util.PickValue("S3 Bucket Name:", defaultBucketName, true)
	if err != nil {
		return fmt.Errorf("%s", err)
	}

	if bucketName == defaultBucketName {
		usr, err := user.Current()

		if err != nil {
			return fmt.Errorf("%s", err)
		}

		bucketNamePrefix, err := util.PickValue("S3 Bucket Name Prefix (needed because you chose default bucket name):", usr.Username, true)
		if err != nil {
			return fmt.Errorf("%s", err)
		}

		bucketName = bucketNamePrefix + "-" + bucketName
	}

	err = util.RunAWSCommand("s3api", "create-bucket", "--bucket", bucketName, "--region", "us-east-1")
	if err != nil {
		return fmt.Errorf("Unable to create state-store bucket '%s'", bucketName)
	}

	err = util.RunAWSCommand("s3api", "put-bucket-versioning", "--bucket", bucketName, "--versioning-configuration", "Status=Enabled")
	if err != nil {
		return fmt.Errorf("Unable to activate versioning on state-store bucket '%s'", bucketName)
	}

	fmt.Println("\nSuccessfully created S3 bucket '" + bucketName + "'")

	fmt.Println("\nNow, you should create the following environment variable in your current shell and your bash profile:")
	fmt.Println("\n\tZITI_STATE_STORE=s3://" + bucketName)
	fmt.Println("")

	return nil
}
