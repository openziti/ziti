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
	"github.com/Jeffail/gabs"
	"github.com/openziti/ziti/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/spf13/cobra"
	"io"
)

type traceIdentityOptions struct {
	api.Options
	disable  bool
	duration string
	traceId  string
}

// newCreateIdentityCmd creates the 'edge controller create identity' command
func newTraceCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "trace",
		Short: "manages tracing by the Ziti Edge Controller",
	}

	cmd.AddCommand(newTraceIdentityCmd(out, errOut))

	return cmd
}

func newTraceIdentityCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &traceIdentityOptions{
		Options: api.Options{
			CommonOptions: common.CommonOptions{Out: out, Err: errOut},
		},
	}

	cmd := &cobra.Command{
		Use:   "identity",
		Short: "enables/disables tracing for sessions from an identity managed by the Ziti Edge Controller",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := runTraceIdentity(options)
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	cmd.Flags().BoolVar(&options.disable, "disable", false, "Disables tracing for the identity (default false)")
	cmd.Flags().StringVarP(&options.duration, "duration", "d", "10m", "how long to enable tracing for (default 10 minutes)")
	cmd.Flags().StringVar(&options.traceId, "trace-id", "", "Unique id to use when tracing")

	options.AddCommonFlags(cmd)

	return cmd
}

func runTraceIdentity(o *traceIdentityOptions) error {
	id, err := mapNameToID("identities", o.Args[0], o.Options)
	if err != nil {
		return err
	}

	entityData := gabs.New()
	api.SetJSONValue(entityData, !o.disable, "enabled")
	api.SetJSONValue(entityData, o.duration, "duration")

	if o.traceId != "" {
		api.SetJSONValue(entityData, o.traceId, "traceId")
	}

	if len(o.Args) > 1 {
		api.SetJSONValue(entityData, o.Args[1:], "channels")
	}

	result, err := putEntityOfType("identities/"+id+"/trace", entityData.String(), &o.Options)

	if err != nil {
		return err
	}

	traceId := result.S("data", "traceId").Data().(string)
	until := result.S("data", "until").Data().(string)
	enabled := result.S("data", "enabled").Data().(bool)

	if enabled {
		_, err = fmt.Fprintf(o.Out, "tracing enabled for identity %v until %v with id: %v\n", id, until, traceId)
		return err
	}

	_, err = fmt.Fprintf(o.Out, "tracing disabled for identity %v\n", id)
	return err
}
