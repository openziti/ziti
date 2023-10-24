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

package handler_ctrl

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2"
	"github.com/openziti/channel/v2/protobufs"
	"github.com/openziti/ziti/common/pb/ctrl_pb"
	"github.com/openziti/ziti/router/env"
	"github.com/openziti/ziti/router/xgress"
	"google.golang.org/protobuf/proto"
)

type validateTerminatorsV2Handler struct {
	env env.RouterEnv
}

func newValidateTerminatorsV2Handler(env env.RouterEnv) *validateTerminatorsV2Handler {
	return &validateTerminatorsV2Handler{
		env: env,
	}
}

func (handler *validateTerminatorsV2Handler) ContentType() int32 {
	return int32(ctrl_pb.ContentType_ValidateTerminatorsV2RequestType)
}

func (handler *validateTerminatorsV2Handler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	log := pfxlog.ContextLogger(ch.Label())

	req := &ctrl_pb.ValidateTerminatorsV2Request{}
	if err := proto.Unmarshal(msg.Body, req); err != nil {
		log.Errorf("error unmarshaling validate terminators V2 msg (%v)", err)
		return
	}

	// because this sends AND receives messages, must be done async, otherwise message responses will never
	// be received, as this handler will block processing
	go handler.validateTerminators(msg, ch, req)
}

func (handler *validateTerminatorsV2Handler) validateTerminators(msg *channel.Message, ch channel.Channel, req *ctrl_pb.ValidateTerminatorsV2Request) {
	log := pfxlog.ContextLogger(ch.Label())

	log.Debugf("validate terminators route request received: %v terminators", len(req.Terminators))
	dialers := map[string]xgress.Dialer{}

	response := &ctrl_pb.ValidateTerminatorsV2Response{
		States: map[string]*ctrl_pb.RouterTerminatorState{},
	}

	for _, terminator := range req.Terminators {
		binding := terminator.Binding
		dialer := dialers[binding]
		if dialer == nil {
			if factory, err := xgress.GlobalRegistry().Factory(binding); err == nil {
				if dialer, err = factory.CreateDialer(handler.env.GetDialerCfg()[binding]); err == nil {
					dialers[binding] = dialer
				}
			}
		}

		if dialer == nil {
			response.States[terminator.Id] = &ctrl_pb.RouterTerminatorState{
				Valid:  false,
				Reason: ctrl_pb.TerminatorInvalidReason_UnknownBinding,
			}
		} else if inspectable, ok := dialer.(xgress.InspectableDialer); ok {
			valid, state := inspectable.InspectTerminator(terminator.Id, terminator.Address, req.FixInvalid)
			response.States[terminator.Id] = &ctrl_pb.RouterTerminatorState{
				Valid:  valid,
				Detail: state,
				Reason: ctrl_pb.TerminatorInvalidReason_UnknownTerminator,
			}
		} else if !dialer.IsTerminatorValid(terminator.Id, terminator.Address) {
			response.States[terminator.Id] = &ctrl_pb.RouterTerminatorState{
				Valid:  false,
				Reason: ctrl_pb.TerminatorInvalidReason_UnknownTerminator,
			}
		} else {
			response.States[terminator.Id] = &ctrl_pb.RouterTerminatorState{
				Valid:  true,
				Detail: "valid",
			}
		}
	}

	err := protobufs.MarshalTyped(response).
		ReplyTo(msg).
		WithTimeout(handler.env.GetNetworkControllers().DefaultRequestTimeout()).
		SendAndWaitForWire(ch)

	if err != nil {
		log.WithError(err).Error("failed to send validate terminators v2 response")
	}
}
