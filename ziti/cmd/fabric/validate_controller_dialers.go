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
	"github.com/openziti/channel/v4"
	"github.com/openziti/channel/v4/protobufs"
	"github.com/openziti/ziti/v2/common/pb/mgmt_pb"
	"github.com/openziti/ziti/v2/ziti/cmd/api"
	"github.com/openziti/ziti/v2/ziti/cmd/common"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/proto"
)

type validateControllerDialersAction struct {
	api.Options
	includeSuccesses bool

	eventNotify chan *mgmt_pb.ControllerDialerDetails
}

// NewValidateControllerDialersCmd creates the "validate controller-dialers" CLI command.
func NewValidateControllerDialersCmd(p common.OptionsProvider) *cobra.Command {
	action := validateControllerDialersAction{
		Options: api.Options{
			CommonOptions: p(),
		},
	}

	validateCmd := &cobra.Command{
		Use:   "controller-dialers",
		Short: "Validate controller dialer states",
		Args:  cobra.ExactArgs(0),
		RunE:  action.validateControllerDialers,
	}

	action.AddCommonFlags(validateCmd)
	validateCmd.Flags().BoolVar(&action.includeSuccesses, "include-successes", false, "Don't hide results for routers with no errors")
	return validateCmd
}

func (self *validateControllerDialersAction) validateControllerDialers(_ *cobra.Command, _ []string) error {
	closeNotify := make(chan struct{})
	self.eventNotify = make(chan *mgmt_pb.ControllerDialerDetails, 1)

	bindHandler := func(binding channel.Binding) error {
		binding.AddReceiveHandler(int32(mgmt_pb.ContentType_ValidateControllerDialersResultType), self)
		binding.AddCloseHandler(channel.CloseHandlerF(func(ch channel.Channel) {
			close(closeNotify)
		}))
		return nil
	}

	ch, err := api.NewWsMgmtChannel(channel.BindHandlerF(bindHandler))
	if err != nil {
		return err
	}

	request := &mgmt_pb.ValidateControllerDialersRequest{}

	responseMsg, err := protobufs.MarshalTyped(request).WithTimeout(time.Duration(self.Timeout) * time.Second).SendForReply(ch)

	response := &mgmt_pb.ValidateControllerDialersResponse{}
	if err = protobufs.TypedResponse(response).Unmarshall(responseMsg, err); err != nil {
		return err
	}

	if !response.Success {
		return fmt.Errorf("failed to start controller dialers validation: %s", response.Message)
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
					fmt.Printf("routerId: %s, routerName: %v, success: %v\n",
						detail.ComponentId, detail.ComponentName, detail.ValidateSuccess)
				}
				headerDone = true
			}

			if self.includeSuccesses {
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
		return fmt.Errorf("%d error(s) occurred", errCount)
	}
	return nil
}

func (self *validateControllerDialersAction) HandleReceive(msg *channel.Message, _ channel.Channel) {
	detail := &mgmt_pb.ControllerDialerDetails{}
	if err := proto.Unmarshal(msg.Body, detail); err != nil {
		pfxlog.Logger().WithError(err).Error("unable to unmarshal controller dialer details")
		return
	}

	self.eventNotify <- detail
}
