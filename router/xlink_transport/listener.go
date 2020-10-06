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
	"errors"
	"fmt"
	"github.com/openziti/fabric/router/xlink"
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/foundation/identity/identity"
	"github.com/openziti/foundation/transport"
	"github.com/sirupsen/logrus"
)

func (self *listener) Listen() error {
	listener := channel2.NewClassicListenerWithTransportConfiguration(self.id, self.config.bind, self.config.options.ConnectOptions, self.tcfg, nil)

	self.listener = listener
	if err := self.listener.Listen(); err != nil {
		return fmt.Errorf("error listening (%w)", err)
	}
	go self.acceptLoop()
	return nil
}

func (self *listener) GetAdvertisement() string {
	return self.config.advertise.String()
}

func (self *listener) Close() error {
	return self.listener.Close()
}

func (self *listener) acceptLoop() {
	for {
		ch, err := channel2.NewChannelWithTransportConfiguration("link", self.listener, self.config.options, self.tcfg)
		if err == nil {
			xlink := &impl{id: ch.Id(), ch: ch}
			logrus.Infof("accepted link id [l/%s]", xlink.Id().Token)

			if self.chAccepter != nil {
				if err := self.chAccepter.AcceptChannel(xlink, ch); err != nil {
					logrus.Errorf("error accepting incoming channel (%v)", err)
				}
			}

			if err := self.accepter.Accept(xlink); err != nil {
				logrus.Errorf("error accepting incoming Xlink (%v)", err)
			}

			logrus.Infof("accepted link [%s]", "l/"+ch.Id().Token)

		} else if errors.Is(err, channel2.ListenerClosedError) {
			logrus.Errorf("link underlay acceptor closed")
			return
		} else {
			logrus.Errorf("error creating link underlay (%v)", err)
		}
	}
}

type listener struct {
	id         *identity.TokenId
	config     *listenerConfig
	listener   channel2.UnderlayListener
	accepter   xlink.Accepter
	chAccepter ChannelAccepter
	tcfg       transport.Configuration
}
