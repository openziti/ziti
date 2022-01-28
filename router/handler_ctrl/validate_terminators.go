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

package handler_ctrl

import (
	"github.com/golang/protobuf/proto"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/fabric/router/xgress"
)

type validateTerminatorsHandler struct {
	ctrl      xgress.CtrlChannel
	dialerCfg map[string]xgress.OptionsData
}

func newValidateTerminatorsHandler(ctrl xgress.CtrlChannel, dialerCfg map[string]xgress.OptionsData) *validateTerminatorsHandler {
	return &validateTerminatorsHandler{
		ctrl:      ctrl,
		dialerCfg: dialerCfg,
	}
}

func (handler *validateTerminatorsHandler) ContentType() int32 {
	return int32(ctrl_pb.ContentType_ValidateTerminatorsRequestType)
}

func (handler *validateTerminatorsHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	log := pfxlog.ContextLogger(ch.Label())

	req := &ctrl_pb.ValidateTerminatorsRequest{}
	if err := proto.Unmarshal(msg.Body, req); err != nil {
		log.Errorf("error unmarshaling validate terminators msg (%v)", err)
		return
	}

	// because this sends AND receives messages, must be done async, otherwise message responses will never
	// be received, as this handler will block processing
	go handler.validateTerminators(req)
}

func (handler *validateTerminatorsHandler) validateTerminators(req *ctrl_pb.ValidateTerminatorsRequest) {
	log := pfxlog.ContextLogger(handler.ctrl.Channel().Label())

	log.Debugf("validate terminators route request received: %v terminators", len(req.Terminators))
	dialers := map[string]xgress.Dialer{}
	for _, terminator := range req.Terminators {
		binding := terminator.Binding
		dialer := dialers[binding]
		if dialer == nil {
			if factory, err := xgress.GlobalRegistry().Factory(binding); err == nil {
				if dialer, err = factory.CreateDialer(handler.dialerCfg[binding]); err == nil {
					dialers[binding] = dialer
				}
			}
		}

		// TODO: We could distringuish between gone and invalid configuration and allow disabling terminators
		//       rather than deleting them
		if dialer == nil || !dialer.IsTerminatorValid(terminator.Id, terminator.Address) {
			log.Infof("removing invalid terminator %v with binding: %v. had dialer? %v", terminator.Id, terminator.Binding, dialer != nil)
			if err := xgress.RemoveTerminator(handler.ctrl, terminator.Id); err != nil {
				log.Errorf("failed to remove invalid terminator %v (%v)", terminator.Id, err)
			}
		}
	}
}
