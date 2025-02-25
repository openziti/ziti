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
	"github.com/openziti/channel/v3"
	"github.com/openziti/channel/v3/protobufs"
	"github.com/openziti/ziti/common/pb/mgmt_pb"
	"github.com/openziti/ziti/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/common"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/proto"
	"os"
	"time"
)

type validateRouterErtTerminatorsAction struct {
	api.Options
	includeValidErtTerminators bool
	includeValidRouters        bool

	eventNotify chan *mgmt_pb.RouterErtTerminatorsDetails
}

func NewValidateRouterErtTerminatorsCmd(p common.OptionsProvider) *cobra.Command {
	action := validateRouterErtTerminatorsAction{
		Options: api.Options{
			CommonOptions: p(),
		},
	}

	validateErtTerminatorsCmd := &cobra.Command{
		Use:     "router-ert-terminators <router filter>",
		Short:   "Validate router tunneler terminators",
		Example: "ziti fabric validate router-ert-terminators --filter 'name=\"my-router\"' --include-valid",
		Args:    cobra.MaximumNArgs(1),
		RunE:    action.validateRouterErtTerminators,
	}

	action.AddCommonFlags(validateErtTerminatorsCmd)
	validateErtTerminatorsCmd.Flags().BoolVar(&action.includeValidErtTerminators, "include-valid-terminators", false, "Don't hide results for valid ert terminators")
	validateErtTerminatorsCmd.Flags().BoolVar(&action.includeValidRouters, "include-valid-routers", false, "Don't hide results for valid routers")
	return validateErtTerminatorsCmd
}

func (self *validateRouterErtTerminatorsAction) validateRouterErtTerminators(_ *cobra.Command, args []string) error {
	closeNotify := make(chan struct{})
	self.eventNotify = make(chan *mgmt_pb.RouterErtTerminatorsDetails, 1)

	bindHandler := func(binding channel.Binding) error {
		binding.AddReceiveHandler(int32(mgmt_pb.ContentType_ValidateRouterErtTerminatorsResultType), self)
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

	request := &mgmt_pb.ValidateRouterErtTerminatorsRequest{
		Filter: filter,
	}

	responseMsg, err := protobufs.MarshalTyped(request).WithTimeout(time.Duration(self.Timeout) * time.Second).SendForReply(ch)

	response := &mgmt_pb.ValidateRouterErtTerminatorsResponse{}
	if err = protobufs.TypedResponse(response).Unmarshall(responseMsg, err); err != nil {
		return err
	}

	if !response.Success {
		return fmt.Errorf("failed to start Ert terminator validation: %s", response.Message)
	}

	fmt.Printf("started validation of %v routers\n", response.RouterCount)

	expected := response.RouterCount

	errCount := 0
	for expected > 0 {
		select {
		case <-closeNotify:
			fmt.Printf("channel closed, exiting")
			return nil
		case routerDetail := <-self.eventNotify:
			result := "validation successful"
			if !routerDetail.ValidateSuccess {
				result = fmt.Sprintf("error: unable to validate (%s)", routerDetail.Message)
				errCount++
			}

			routerHeaderDone := false
			outputRouterHeader := func() {
				fmt.Printf("routerId: %s, routerName: %v, Ert-terminators: %v, %s\n",
					routerDetail.RouterId, routerDetail.RouterName, len(routerDetail.Details), result)
				routerHeaderDone = true
			}

			if self.includeValidRouters || routerDetail.Message != "" {
				outputRouterHeader()
			}

			for _, detail := range routerDetail.Details {
				if self.includeValidErtTerminators || !detail.IsValid {
					if !routerHeaderDone {
						outputRouterHeader()
					}
					fmt.Printf("\tid: %s, ctrlState: %v, routerState: %s\n",
						detail.TerminatorId, detail.CtrlState, detail.RouterState)
				}
				if !detail.IsValid {
					errCount++
				}
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

func (self *validateRouterErtTerminatorsAction) HandleReceive(msg *channel.Message, _ channel.Channel) {
	detail := &mgmt_pb.RouterErtTerminatorsDetails{}
	if err := proto.Unmarshal(msg.Body, detail); err != nil {
		pfxlog.Logger().WithError(err).Error("unable to unmarshal router Ert terminator details")
		return
	}

	self.eventNotify <- detail
}
