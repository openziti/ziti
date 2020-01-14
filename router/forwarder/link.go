/*
	Copyright 2019 NetFoundry, Inc.

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

package forwarder

import (
	"github.com/netfoundry/ziti-fabric/xgress"
	"github.com/netfoundry/ziti-foundation/channel2"
	"github.com/netfoundry/ziti-foundation/identity/identity"
)

type Link struct {
	Id      *identity.TokenId
	Channel channel2.Channel
}

func NewLink(id *identity.TokenId, channel channel2.Channel) *Link {
	link := &Link{
		Id:      id,
		Channel: channel,
	}
	return link
}

func (link *Link) SendPayload(payload *xgress.Payload) error {
	msg := payload.Marshall()
	return link.Channel.Send(msg)
}

func (link *Link) SendAcknowledgement(acknowledgement *xgress.Acknowledgement) error {
	msg := acknowledgement.Marshall()
	return link.Channel.SendWithPriority(msg, channel2.Highest)
}
