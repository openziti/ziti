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
	"github.com/openziti/foundation/v2/goroutines"
	"github.com/openziti/ziti/common/pb/ctrl_pb"
	"github.com/openziti/ziti/router/env"
	"github.com/openziti/ziti/router/xgress"
	"google.golang.org/protobuf/proto"
	"time"
)

type validateTerminatorsV2Handler struct {
	env  env.RouterEnv
	pool goroutines.Pool
}

func newValidateTerminatorsV2Handler(env env.RouterEnv, pool goroutines.Pool) *validateTerminatorsV2Handler {
	return &validateTerminatorsV2Handler{
		env:  env,
		pool: pool,
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

	expected := 0
	results := make(chan func(*ctrl_pb.ValidateTerminatorsV2Response), len(req.Terminators))

	for _, val := range req.Terminators {
		terminator := val
		binding := terminator.Binding
		dialer := dialers[binding]
		if dialer == nil {
			if factory, err := xgress.GlobalRegistry().Factory(binding); err == nil {
				if dialer, err = factory.CreateDialer(handler.env.GetDialerCfg()[binding]); err == nil {
					dialers[binding] = dialer
				}
			}
		}
		log.WithField("terminatorId", terminator.Id).Debug("beginning terminator validation")
		if dialer == nil {
			response.States[terminator.Id] = &ctrl_pb.RouterTerminatorState{
				Valid:  false,
				Reason: ctrl_pb.TerminatorInvalidReason_UnknownBinding,
				Marker: val.Marker,
			}
		} else {
			err := handler.pool.Queue(func() {
				log.WithField("terminatorId", terminator.Id).Info("validating terminator")
				result := handler.validateTerminator(dialer, terminator, req.FixInvalid)
				results <- func(response *ctrl_pb.ValidateTerminatorsV2Response) {
					response.States[terminator.Id] = result
				}
			})

			if err != nil {
				log.WithField("terminatorId", terminator.Id).WithError(err).Error("unable to queue inspect")
			} else {
				expected++
			}
		}
	}

	timeout := time.After(30 * time.Second)
	timedOut := false
	for i := 0; i < expected && !timedOut; i++ {
		select {
		case result := <-results:
			result(response)
		case <-timeout:
			timedOut = true
			log.Info("timed out waiting for terminator validations")
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

func (handler *validateTerminatorsV2Handler) validateTerminator(dialer xgress.Dialer, terminator *ctrl_pb.Terminator, fixInvalid bool) *ctrl_pb.RouterTerminatorState {
	if inspectable, ok := dialer.(xgress.InspectableDialer); ok {
		valid, state := inspectable.InspectTerminator(terminator.Id, terminator.Address, fixInvalid)
		if valid {
			return &ctrl_pb.RouterTerminatorState{
				Valid:  true,
				Detail: state,
				Marker: terminator.Marker,
			}
		}
		return &ctrl_pb.RouterTerminatorState{
			Valid:  false,
			Detail: state,
			Reason: ctrl_pb.TerminatorInvalidReason_UnknownTerminator,
			Marker: terminator.Marker,
		}
	}

	if !dialer.IsTerminatorValid(terminator.Id, terminator.Address) {
		return &ctrl_pb.RouterTerminatorState{
			Valid:  false,
			Reason: ctrl_pb.TerminatorInvalidReason_UnknownTerminator,
			Marker: terminator.Marker,
		}
	}

	return &ctrl_pb.RouterTerminatorState{
		Valid:  true,
		Detail: "valid",
		Marker: terminator.Marker,
	}
}
