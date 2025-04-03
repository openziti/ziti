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
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/channel/v4/protobufs"
	"github.com/openziti/ziti/common/pb/mgmt_pb"
	"github.com/openziti/ziti/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/common"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/proto"
	"os"
	"time"
)

type validateIdentityConnectionStatusesAction struct {
	api.Options
	includeValidRouters bool

	eventNotify chan *mgmt_pb.RouterIdentityConnectionStatusesDetails
}

func NewValidateIdentityConnectionStatusesCmd(p common.OptionsProvider) *cobra.Command {
	action := validateIdentityConnectionStatusesAction{
		Options: api.Options{
			CommonOptions: p(),
		},
	}

	validateLinksCmd := &cobra.Command{
		Use:     "identity-connection-statuses <router filter>",
		Short:   "Validate identity connection statuses",
		Example: "ziti fabric validate router-data-model --filter 'name=\"my-router\"' --include-valid-routers",
		Args:    cobra.MaximumNArgs(1),
		RunE:    action.validateRouterDataModel,
	}

	action.AddCommonFlags(validateLinksCmd)
	validateLinksCmd.Flags().BoolVar(&action.includeValidRouters, "include-successes", false, "Don't hide results for successes")
	return validateLinksCmd
}

func (self *validateIdentityConnectionStatusesAction) validateRouterDataModel(_ *cobra.Command, args []string) error {
	closeNotify := make(chan struct{})
	self.eventNotify = make(chan *mgmt_pb.RouterIdentityConnectionStatusesDetails, 1)

	bindHandler := func(binding channel.Binding) error {
		binding.AddReceiveHandler(int32(mgmt_pb.ContentType_ValidateIdentityConnectionStatusesResultType), self)
		binding.AddCloseHandler(channel.CloseHandlerF(func(ch channel.Channel) {
			close(closeNotify)
		}))
		return nil
	}

	ch, err := api.NewWsMgmtChannel(channel.BindHandlerF(bindHandler))
	if err != nil {
		return err
	}

	filter := ""
	if len(args) > 0 {
		filter = args[0]
	}

	request := &mgmt_pb.ValidateIdentityConnectionStatusesRequest{
		RouterFilter: filter,
	}

	responseMsg, err := protobufs.MarshalTyped(request).WithTimeout(time.Duration(self.Timeout) * time.Second).SendForReply(ch)

	response := &mgmt_pb.ValidateIdentityConnectionStatusesResponse{}
	if err = protobufs.TypedResponse(response).Unmarshall(responseMsg, err); err != nil {
		return err
	}

	if !response.Success {
		return fmt.Errorf("failed to start identity connection status validation: %s", response.Message)
	}

	fmt.Printf("started validation of %v components\n", response.ComponentCount)

	expected := response.ComponentCount

	errCount := 0
	for expected > 0 {
		select {
		case <-closeNotify:
			fmt.Printf("channel closed, exiting")
			return nil
		case detail := <-self.eventNotify:
			headerDone := false
			outputHeader := func() {
				if !headerDone {
					if detail.ComponentType == "controller" {
						fmt.Printf("controllerId: %s, success: %v\n", detail.ComponentId, detail.ValidateSuccess)
					} else if detail.ComponentType == "router" {
						fmt.Printf("routerId: %s, routerName: %v, success: %v\n",
							detail.ComponentId, detail.ComponentName, detail.ValidateSuccess)
					}
				}
				headerDone = true
			}

			if self.includeValidRouters {
				outputHeader()
			}

			for _, errDetail := range detail.Errors {
				outputHeader()
				fmt.Printf("\t%s\n", errDetail)
				errCount++
			}
			expected--
		}
	}
	fmt.Printf("%v errors found\n", errCount)
	if errCount > 0 {
		os.Exit(1)
	}
	return nil
}

func (self *validateIdentityConnectionStatusesAction) HandleReceive(msg *channel.Message, _ channel.Channel) {
	detail := &mgmt_pb.RouterIdentityConnectionStatusesDetails{}
	if err := proto.Unmarshal(msg.Body, detail); err != nil {
		pfxlog.Logger().WithError(err).Error("unable to unmarshal router data model details")
		return
	}

	self.eventNotify <- detail
}
