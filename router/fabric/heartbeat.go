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

package fabric

import (
	"github.com/golang/protobuf/proto"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/pb/edge_ctrl_pb"
	"github.com/openziti/edge/runner"
	"time"
)

const maxTokensPerMessage = 10000

type heartbeatOperation struct {
	ctrl channel.Channel
	*runner.BaseOperation
	tokenProvider TokenProvider
}

type TokenProvider interface {
	ActiveApiSessionTokens() []string
	flushRecentlyRemoved()
}

func newHeartbeatOperation(ctrl channel.Channel, frequency time.Duration, tokenProvider TokenProvider) *heartbeatOperation {
	return &heartbeatOperation{
		ctrl:          ctrl,
		tokenProvider: tokenProvider,
		BaseOperation: runner.NewBaseOperation("Heartbeat Operation", frequency)}
}

func (operation *heartbeatOperation) Run() error {
	tokens := operation.tokenProvider.ActiveApiSessionTokens()

	operation.beat(tokens)
	operation.tokenProvider.flushRecentlyRemoved()

	return nil
}

func (operation *heartbeatOperation) beat(tokens []string) {
	var msgs []*channel.Message

	pfxlog.Logger().Tracef("heartbeat tokens: %d", len(tokens))

	for len(tokens) > 0 {

		if maxTokensPerMessage >= len(tokens) {
			msg := &edge_ctrl_pb.ApiSessionHeartbeat{
				Tokens: tokens,
			}
			bodyBytes, err := proto.Marshal(msg)

			if err != nil {
				pfxlog.Logger().Panic("could not marshal SessionHeartbeat type (1)")
			}

			msgs = append(msgs, channel.NewMessage(env.ApiSessionHeartbeatType, bodyBytes))

			tokens = nil
		} else {
			msg := &edge_ctrl_pb.ApiSessionHeartbeat{
				Tokens: tokens[:maxTokensPerMessage],
			}

			bodyBytes, err := proto.Marshal(msg)

			if err != nil {
				pfxlog.Logger().Panic("could not marshal SessionHeartbeat type (2)")
			}

			tokens = tokens[maxTokensPerMessage:]
			msgs = append(msgs, channel.NewMessage(env.ApiSessionHeartbeatType, bodyBytes))
		}
	}

	for _, msg := range msgs {
		if err := operation.ctrl.Send(msg); err != nil {
			pfxlog.Logger().WithError(err).Error("could not send heartbeats on control channel")
		}

	}

}
