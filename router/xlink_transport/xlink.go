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
	"github.com/netfoundry/ziti-foundation/channel2"
	"github.com/netfoundry/ziti-foundation/identity/identity"
	"github.com/sirupsen/logrus"
)

func (self *impl) Listen() error {
	self.listener = channel2.NewClassicListener(self.id, self.config.listener)
	if err := self.listener.Listen(); err != nil {
		return fmt.Errorf("error listening (%w)", err)
	}
	go self.runAccepter()
	return nil
}

func (_ *impl) Dial(address string) error {
	return nil
}

func (_ *impl) GetAdvertisement() string {
	return ""
}

func (self *impl) runAccepter() {
	defer logrus.Warn("exited")
	logrus.Info("started")

	for {
		ch, err := channel2.NewChannel("link", self.listener, self.options)
		if err == nil {
			if err := self.accepter.Accept(ch); err != nil {
				logrus.Errorf("error invoking accepter (%w)", err)
			}
		} else {
			logrus.Errorf("error accepting (%w)", err)
		}
	}
}

type impl struct {
	id       *identity.TokenId
	config   *config
	listener channel2.UnderlayListener
	accepter Accepter
	options  *channel2.Options
}
