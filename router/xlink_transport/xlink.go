/*
	(c) Copyright NetFoundry, Inc.

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

package xlink_transport

import (
	"fmt"
	"github.com/netfoundry/ziti-fabric/router/forwarder"
	"github.com/netfoundry/ziti-fabric/router/handler_link"
	"github.com/netfoundry/ziti-fabric/router/xgress"
	"github.com/netfoundry/ziti-foundation/channel2"
	"github.com/netfoundry/ziti-foundation/identity/identity"
)

func (self *impl) Listen() error {
	self.listener = channel2.NewClassicListener(self.id, self.config.listener)
	if err := self.listener.Listen(); err != nil {
		return fmt.Errorf("error listening (%w)", err)
	}
	go handler_link.NewAccepter(self.id, self.ctrl, self.forwarder, self.listener, self.options, self.forwarderOptions)
	return nil
}

func (_ *impl) Dial(address string) error {
	return nil
}

func (_ *impl) GetAdvertisement() string {
	return ""
}

type impl struct {
	id               *identity.TokenId
	config           *config
	listener         channel2.UnderlayListener
	ctrl             xgress.CtrlChannel
	forwarder        *forwarder.Forwarder
	forwarderOptions *forwarder.Options
	options          *channel2.Options
}
