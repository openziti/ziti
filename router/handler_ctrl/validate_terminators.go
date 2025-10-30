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
	"github.com/openziti/channel/v4"
	"github.com/openziti/ziti/common/pb/ctrl_pb"
	"github.com/openziti/ziti/router/env"
	"github.com/openziti/ziti/router/xgress_router"
	"google.golang.org/protobuf/proto"
)

const (
	TerminatorDeleteBatchSize = 50
)

type validateTerminatorsHandler struct {
	env env.RouterEnv
}

func newValidateTerminatorsHandler(env env.RouterEnv) *validateTerminatorsHandler {
	return &validateTerminatorsHandler{
		env: env,
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
	log := pfxlog.Logger()

	log.Debugf("validate terminators route request received: %v terminators", len(req.Terminators))
	dialers := map[string]xgress_router.Dialer{}

	var deleteList []string
	for _, terminator := range req.Terminators {
		binding := terminator.Binding
		dialer := dialers[binding]
		if dialer == nil {
			if factory, err := handler.env.GetXgressRegistry().Factory(binding); err == nil {
				if dialer, err = factory.CreateDialer(handler.env.GetDialerCfg()[binding]); err == nil {
					dialers[binding] = dialer
				}
			}
		}

		// TODO: We could distinguish between gone and invalid configuration and allow disabling terminators
		//       rather than deleting them
		if dialer == nil || !dialer.IsTerminatorValid(terminator.Id, terminator.Address) {
			log.Infof("removing invalid terminator %v with binding: %v. had dialer? %v", terminator.Id, terminator.Binding, dialer != nil)
			deleteList = append(deleteList, terminator.Id)
			if len(deleteList) >= TerminatorDeleteBatchSize {
				log.Infof("send batch of %v terminator deletes", len(deleteList))
				xgress_router.RemoveTerminators(handler.env.GetNetworkControllers(), deleteList)
				deleteList = nil
			}
		}
	}

	if len(deleteList) > 0 {
		log.Infof("send batch of %v terminator deletes", len(deleteList))
		xgress_router.RemoveTerminators(handler.env.GetNetworkControllers(), deleteList)
	}

}
