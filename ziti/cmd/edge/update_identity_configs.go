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

package edge

import (
	"fmt"
	"github.com/openziti/ziti/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"io"

	"github.com/Jeffail/gabs"
	"github.com/spf13/cobra"
)

type updateIdentityConfigsOptions struct {
	api.Options
	remove bool
}

func newUpdateIdentityConfigsCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &updateIdentityConfigsOptions{
		Options: api.Options{
			CommonOptions: common.CommonOptions{Out: out, Err: errOut},
		},
	}

	cmd := &cobra.Command{
		Use:   "identity-configs <identity id or name> <service id or name> <config id or name>",
		Short: "for the specified identity, use the given config for the given service",
		Args:  cobra.ExactArgs(3),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := runupdateIdentityConfigs(options)
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	options.AddCommonFlags(cmd)

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	cmd.Flags().BoolVarP(&options.remove, "remove", "r", false, "Remove the sevice config override")

	return cmd
}

// runupdateIdentityConfigs update a new identity on the Ziti Edge Controller
func runupdateIdentityConfigs(o *updateIdentityConfigsOptions) error {
	id, err := mapNameToID("identities", o.Args[0], o.Options)
	if err != nil {
		return err
	}

	serviceId, err := mapNameToID("services", o.Args[1], o.Options)
	if err != nil {
		return err
	}

	configId, err := mapNameToID("configs", o.Args[2], o.Options)
	if err != nil {
		return err
	}

	entityData, _ := gabs.New().ArrayOfSize(1)
	serviceConfig := map[string]string{
		"serviceId": serviceId,
		"configId":  configId,
	}
	_, _ = entityData.SetIndex(serviceConfig, 0)

	path := fmt.Sprintf("identities/%v/service-configs", id)
	body := entityData.String()

	if o.remove {
		_, err = deleteEntityOfTypeWithBody(path, body, &o.Options)
	} else {
		_, err = postEntityOfType(path, body, &o.Options)
	}
	return err
}
