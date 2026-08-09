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
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v5"
	"github.com/openziti/channel/v5/protobufs"
	"github.com/openziti/ziti/v2/common/pb/mgmt_pb"
	"github.com/openziti/ziti/v2/ziti/cmd/api"
	"github.com/openziti/ziti/v2/ziti/cmd/common"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/proto"
)

type validateGossipAction struct {
	api.Options
	includeValidLinks      bool
	includeValidComponents bool
	validateCtrl           bool

	eventNotify chan *mgmt_pb.GossipValidationDetails
}

func NewValidateGossipCmd(p common.OptionsProvider) *cobra.Command {
	action := validateGossipAction{
		Options: api.Options{
			CommonOptions: p(),
		},
	}

	cmd := &cobra.Command{
		Use:   "gossip <router filter>",
		Short: "Validate link gossip consistency (link registry vs router gossip store vs controller gossip store, and controller link manager vs gossip store)",
		Example: "ziti fabric validate gossip --validate-ctrl",
		Args:    cobra.MaximumNArgs(1),
		RunE:    action.validateGossip,
	}

	action.AddCommonFlags(cmd)
	cmd.Flags().BoolVar(&action.includeValidLinks, "include-valid-links", false, "Don't hide results for valid links")
	cmd.Flags().BoolVar(&action.includeValidComponents, "include-valid-components", false, "Don't hide results for valid components")
	cmd.Flags().BoolVar(&action.validateCtrl, "validate-ctrl", false, "Also validate this controller's link manager against its gossip store")
	return cmd
}

func (self *validateGossipAction) validateGossip(_ *cobra.Command, args []string) error {
	closeNotify := make(chan struct{})
	self.eventNotify = make(chan *mgmt_pb.GossipValidationDetails, 1)

	bindHandler := func(binding channel.Binding) error {
		binding.AddReceiveHandler(int32(mgmt_pb.ContentType_ValidateGossipResultType), self)
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

	request := &mgmt_pb.ValidateGossipRequest{
		Filter:       filter,
		ValidateCtrl: self.validateCtrl,
	}

	responseMsg, err := protobufs.MarshalTyped(request).WithTimeout(time.Duration(self.Timeout) * time.Second).SendForReply(ch)

	response := &mgmt_pb.ValidateGossipResponse{}
	if err = protobufs.TypedResponse(response).Unmarshall(responseMsg, err); err != nil {
		return err
	}

	if !response.Success {
		return fmt.Errorf("failed to start gossip validation: %s", response.Message)
	}

	fmt.Printf("started gossip validation of %v components\n", response.ComponentCount)

	expected := response.ComponentCount

	errCount := 0
	for expected > 0 {
		select {
		case <-closeNotify:
			fmt.Printf("channel closed, exiting")
			return nil
		case detail := <-self.eventNotify:
			result := "validation successful"
			if !detail.ValidateSuccess {
				if detail.Message != "" {
					result = fmt.Sprintf("error: unable to validate (%s)", detail.Message)
				} else {
					result = "errors found"
				}
			}

			headerDone := false
			outputHeader := func() {
				fmt.Printf("%s %s (%s), entries: %v, %s\n",
					detail.ComponentType, detail.ComponentId, detail.ComponentName, len(detail.LinkDetails), result)
				headerDone = true
			}

			if self.includeValidComponents || !detail.ValidateSuccess {
				outputHeader()
			}

			for _, linkDetail := range detail.LinkDetails {
				if self.includeValidLinks || !linkDetail.IsValid {
					if !headerDone {
						outputHeader()
					}
					fmt.Printf("\tlinkId: %s, iteration: %v, dest: %v, dialed: %v, inSource: %v, inLocalGossip: %v, inCtrlGossip: %v, gossipVersion: %v\n",
						linkDetail.LinkId, linkDetail.Iteration, linkDetail.DestRouterId, linkDetail.Dialed,
						linkDetail.InSource, linkDetail.InLocalGossip, linkDetail.InCtrlGossip, linkDetail.GossipVersion)
					for _, msg := range linkDetail.Messages {
						fmt.Printf("\t\t%s\n", msg)
					}
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
		return fmt.Errorf("%d error(s) occurred", errCount)
	}
	return nil
}

func (self *validateGossipAction) HandleReceive(msg *channel.Message, _ channel.Channel) {
	detail := &mgmt_pb.GossipValidationDetails{}
	if err := proto.Unmarshal(msg.Body, detail); err != nil {
		pfxlog.Logger().WithError(err).Error("unable to unmarshal gossip validation details")
		return
	}

	self.eventNotify <- detail
}
