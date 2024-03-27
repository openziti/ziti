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

package state

import (
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/common/runner"
	"github.com/openziti/ziti/controller/env"
	routerEnv "github.com/openziti/ziti/router/env"
	"google.golang.org/protobuf/proto"
)

const maxTokensPerMessage = 10000

type heartbeatOperation struct {
	env routerEnv.RouterEnv
	*runner.BaseOperation
	tokenProvider TokenProvider
}

type TokenProvider interface {
	ActiveApiSessionTokens() []*ApiSessionToken
	flushRecentlyRemoved()
}

func newHeartbeatOperation(env routerEnv.RouterEnv, frequency time.Duration, tokenProvider TokenProvider) *heartbeatOperation {
	return &heartbeatOperation{
		env:           env,
		tokenProvider: tokenProvider,
		BaseOperation: runner.NewBaseOperation("Heartbeat Operation", frequency)}
}

func (op *heartbeatOperation) Run() error {
	tokens := op.tokenProvider.ActiveApiSessionTokens()

	op.beat(tokens)
	op.tokenProvider.flushRecentlyRemoved()

	return nil
}

func (op *heartbeatOperation) beat(apiSessionTokens []*ApiSessionToken) {
	// Filter to only include legacy protobuf tokens that require heartbeating
	legacyTokens := op.filterLegacyProtobufTokens(apiSessionTokens)

	var messages []*channel.Message

	pfxlog.Logger().Tracef("heartbeat tokens: %d legacy of %d total", len(legacyTokens), len(apiSessionTokens))

	for len(legacyTokens) > 0 {
		var chunk []*ApiSessionToken

		if maxTokensPerMessage >= len(legacyTokens) {
			chunk = legacyTokens
			legacyTokens = nil
		} else {
			chunk = legacyTokens[:maxTokensPerMessage]
			legacyTokens = legacyTokens[maxTokensPerMessage:]
		}

		tokenChunk := make([]string, 0, len(chunk))
		for _, apiSessionToken := range chunk {
			tokenChunk = append(tokenChunk, apiSessionToken.GetToken())
		}

		msg := &edge_ctrl_pb.ApiSessionHeartbeat{
			Tokens: tokenChunk,
		}
		bodyBytes, err := proto.Marshal(msg)

		if err != nil {
			pfxlog.Logger().Error("could not marshal SessionHeartbeat type")
			return
		}

		messages = append(messages, channel.NewMessage(env.ApiSessionHeartbeatType, bodyBytes))
	}

	op.env.GetNetworkControllers().ForEach(func(ctrlId string, ch channel.Channel) {
		for _, msg := range messages {
			// can't send the same messages across channels, will run into problems with channel heartbeats and
			// concurrent writes
			msgCopy := channel.NewMessage(msg.ContentType, msg.Body)
			if err := ch.Send(msgCopy); err != nil {
				pfxlog.Logger().WithError(err).Error("could not send heartbeats on control channel")
			}

		}
	})

}

// filterLegacyProtobufTokens returns only tokens that require controller heartbeating.
// JWT tokens are self-contained and don't need heartbeat synchronization with controllers.
func (op *heartbeatOperation) filterLegacyProtobufTokens(tokens []*ApiSessionToken) []*ApiSessionToken {
	var legacyTokens []*ApiSessionToken

	for _, token := range tokens {
		if token.Type == ApiSessionTokenLegacyProtobuf {
			legacyTokens = append(legacyTokens, token)
		}
	}

	return legacyTokens
}
