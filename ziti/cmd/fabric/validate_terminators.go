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
	"time"
)

type validateTerminatorsAction struct {
	api.Options
	filter          string
	fixInvalid      bool
	showOnlyInvalid bool

	eventNotify chan *mgmt_pb.TerminatorDetail
}

func NewValidateTerminatorsCmd(p common.OptionsProvider) *cobra.Command {
	action := validateTerminatorsAction{
		Options: api.Options{
			CommonOptions: p(),
		},
	}

	validateTerminatorsCmd := &cobra.Command{
		Use:     "terminators",
		Short:   "Validate terminators",
		Example: "ziti fabric validate terminators --filter 'service.name=\"test\"' --show-only-invalid",
		Args:    cobra.ExactArgs(0),
		RunE:    action.validateTerminators,
	}

	action.AddCommonFlags(validateTerminatorsCmd)
	validateTerminatorsCmd.Flags().BoolVar(&action.fixInvalid, "fix-invalid", false, "Fix invalid terminators. Usually this means deleting them.")
	validateTerminatorsCmd.Flags().BoolVar(&action.showOnlyInvalid, "show-only-invalid", false, "Hide results for valid terminators")
	validateTerminatorsCmd.Flags().StringVar(&action.filter, "filter", "", "Specify which terminators to validate")
	return validateTerminatorsCmd
}

func (self *validateTerminatorsAction) validateTerminators(cmd *cobra.Command, _ []string) error {
	closeNotify := make(chan struct{})
	self.eventNotify = make(chan *mgmt_pb.TerminatorDetail, 1)

	bindHandler := func(binding channel.Binding) error {
		binding.AddReceiveHandler(int32(mgmt_pb.ContentType_ValidateTerminatorResultType), self)
		binding.AddCloseHandler(channel.CloseHandlerF(func(ch channel.Channel) {
			close(closeNotify)
		}))
		return nil
	}

	ch, err := api.NewWsMgmtChannel(channel.BindHandlerF(bindHandler))
	if err != nil {
		return err
	}

	request := &mgmt_pb.ValidateTerminatorsRequest{
		Filter:     self.filter,
		FixInvalid: self.fixInvalid,
	}

	responseMsg, err := protobufs.MarshalTyped(request).WithTimeout(time.Duration(self.Timeout) * time.Second).SendForReply(ch)

	response := &mgmt_pb.ValidateTerminatorsResponse{}
	if err = protobufs.TypedResponse(response).Unmarshall(responseMsg, err); err != nil {
		return err
	}

	if !response.Success {
		return fmt.Errorf("failed to start terminator validation: %s", response.Message)
	}

	fmt.Printf("started validation of %v terminators\n", response.TerminatorCount)

	expected := response.TerminatorCount

	for expected > 0 {
		select {
		case <-closeNotify:
			fmt.Printf("channel closed, exiting")
			return nil
		case detail := <-self.eventNotify:
			if !self.showOnlyInvalid || detail.State != mgmt_pb.TerminatorState_Valid {
				fmt.Printf("id: %s, binding: %s, hostId: %s, routerId: %s, state: %s, fixed: %v, detail: %s\n",
					detail.TerminatorId, detail.Binding, detail.HostId, detail.RouterId, detail.State.String(), detail.Fixed, detail.Detail)
			}
			expected--
		}
	}
	return nil
}

func (self *validateTerminatorsAction) HandleReceive(msg *channel.Message, _ channel.Channel) {
	detail := &mgmt_pb.TerminatorDetail{}
	if err := proto.Unmarshal(msg.Body, detail); err != nil {
		pfxlog.Logger().WithError(err).Error("unable to unmarshal terminator detail")
		return
	}

	self.eventNotify <- detail
}
