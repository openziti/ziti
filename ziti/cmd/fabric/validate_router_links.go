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
	"github.com/openziti/channel/v2"
	"github.com/openziti/channel/v2/protobufs"
	"github.com/openziti/ziti/common/pb/mgmt_pb"
	"github.com/openziti/ziti/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/common"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/proto"
	"os"
	"time"
)

type validateRouterLinksAction struct {
	api.Options
	includeValid bool

	eventNotify chan *mgmt_pb.RouterLinkDetails
}

func NewValidateRouterLinksCmd(p common.OptionsProvider) *cobra.Command {
	action := validateRouterLinksAction{
		Options: api.Options{
			CommonOptions: p(),
		},
	}

	validateLinksCmd := &cobra.Command{
		Use:     "router-links <router filter>",
		Short:   "Validate router links",
		Example: "ziti fabric validate router-links --filter 'name=\"my-router\"' --include-valid",
		Args:    cobra.MaximumNArgs(1),
		RunE:    action.validateRouterLinks,
	}

	action.AddCommonFlags(validateLinksCmd)
	validateLinksCmd.Flags().BoolVar(&action.includeValid, "include-valid", false, "Don't hide results for valid links")
	return validateLinksCmd
}

func (self *validateRouterLinksAction) validateRouterLinks(_ *cobra.Command, args []string) error {
	closeNotify := make(chan struct{})
	self.eventNotify = make(chan *mgmt_pb.RouterLinkDetails, 1)

	bindHandler := func(binding channel.Binding) error {
		binding.AddReceiveHandler(int32(mgmt_pb.ContentType_ValidateRouterLinksResultType), self)
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

	request := &mgmt_pb.ValidateRouterLinksRequest{
		Filter: filter,
	}

	responseMsg, err := protobufs.MarshalTyped(request).WithTimeout(time.Duration(self.Timeout) * time.Second).SendForReply(ch)

	response := &mgmt_pb.ValidateRouterLinksResponse{}
	if err = protobufs.TypedResponse(response).Unmarshall(responseMsg, err); err != nil {
		return err
	}

	if !response.Success {
		return fmt.Errorf("failed to start link validation: %s", response.Message)
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
				result = fmt.Sprintf("error: unable to validation (%s)", routerDetail.Message)
				errCount++
			}
			fmt.Printf("routerId: %s, routerName: %v, links: %v, %s\n",
				routerDetail.RouterId, routerDetail.RouterName, len(routerDetail.LinkDetails), result)

			for _, linkDetail := range routerDetail.LinkDetails {
				if self.includeValid || !linkDetail.IsValid {
					fmt.Printf("\tlinkId: %s, destConnected: %v, ctrlState: %v, routerState: %v, dest: %v, dialed: %v \n",
						linkDetail.LinkId, linkDetail.DestConnected, linkDetail.CtrlState, linkDetail.RouterState.String(),
						linkDetail.DestRouterId, linkDetail.Dialed)
				}
				if !linkDetail.IsValid {
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

func (self *validateRouterLinksAction) HandleReceive(msg *channel.Message, _ channel.Channel) {
	detail := &mgmt_pb.RouterLinkDetails{}
	if err := proto.Unmarshal(msg.Body, detail); err != nil {
		pfxlog.Logger().WithError(err).Error("unable to unmarshal router link details")
		return
	}

	self.eventNotify <- detail
}
