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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/router/xlink"
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/foundation/identity/identity"
	"github.com/openziti/foundation/transport"
	"github.com/sirupsen/logrus"
	"time"
)

func (self *listener) Listen() error {
	listener := channel2.NewClassicListenerWithTransportConfiguration(self.id, self.config.bind, self.config.options.ConnectOptions, self.tcfg, nil)

	self.listener = listener
	if err := self.listener.Listen(); err != nil {
		return fmt.Errorf("error listening (%w)", err)
	}
	go self.acceptLoop()
	go self.handleEvents()
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
		if err != nil && errors.Is(err, channel2.ListenerClosedError) {
			logrus.Errorf("link underlay acceptor closed")
			return
		} else if err != nil {
			logrus.Errorf("error creating link underlay (%v)", err)
			continue
		}

		headers := ch.Underlay().Headers()
		channelType := byte(0)
		if headers != nil {
			if v, ok := headers[LinkHeaderRouterId]; ok {
				logrus.Infof("accepting link from [r/%s]", string(v))
			}
			if val, ok := headers[LinkHeaderType]; ok {
				channelType = val[0]
			}
		}
		if channelType != 0 {
			id, ok := headers[LinkHeaderConnId]
			if !ok {
				logrus.Errorf("split conn %v received but missing connection id. closing", ch.Id())
				_ = ch.Close()
				continue
			}

			logrus.Infof("split conn of type %v for link [l/%s]", channelType, ch.Id().Token)

			event := &newChannelEvent{
				ch:          ch,
				channelType: channelType,
				id:          string(id),
				eventTime:   time.Now(),
			}
			self.eventC <- event
			continue
		}

		xlink := &impl{id: ch.Id(), ch: ch}
		logrus.Infof("accepting link id [l/%s]", xlink.Id().Token)

		if self.chAccepter != nil {
			if err := self.chAccepter.AcceptChannel(xlink, ch, true); err != nil {
				logrus.Errorf("error accepting incoming channel (%v)", err)
			}
		}

		if err := self.accepter.Accept(xlink); err != nil {
			logrus.Errorf("error accepting incoming Xlink (%v)", err)
		}

		logrus.Infof("accepted link [%s]", "l/"+ch.Id().Token)
	}
}

func (self *listener) handleEvents() {
	ticker := time.NewTicker(time.Minute)
	for {
		select {
		case event := <-self.eventC:
			event.handle(self)
		case <-ticker.C:
			now := time.Now()
			for k, v := range self.pendingChannels {
				if now.Sub(v.eventTime) > (30 * time.Second) {
					_ = v.ch.Close()
					delete(self.pendingChannels, k)
				}
			}
		}
	}
}

type listener struct {
	id              *identity.TokenId
	config          *listenerConfig
	listener        channel2.UnderlayListener
	accepter        xlink.Accepter
	chAccepter      ChannelAccepter
	tcfg            transport.Configuration
	eventC          chan linkEvent
	pendingChannels map[string]*newChannelEvent
}

type linkEvent interface {
	handle(l *listener)
}

type newChannelEvent struct {
	ch          channel2.Channel
	channelType byte
	id          string
	eventTime   time.Time
}

func (event *newChannelEvent) handle(l *listener) {
	partner, ok := l.pendingChannels[event.id]
	if !ok {
		l.pendingChannels[event.id] = event
		return
	}
	delete(l.pendingChannels, event.id)

	var payloadCh channel2.Channel

	if partner.channelType == PayloadChannel {
		payloadCh = partner.ch
	} else if event.channelType == PayloadChannel {
		payloadCh = event.ch
	}

	var ackCh channel2.Channel

	if partner.channelType == AckChannel {
		ackCh = partner.ch
	} else if event.channelType == AckChannel {
		ackCh = event.ch
	}

	if payloadCh == nil || ackCh == nil {
		pfxlog.Logger().Errorf("got two link channels, but types aren't correct. %v %v", partner.channelType, event.channelType)
		return
	}

	xlink := &splitImpl{
		id:        event.ch.Id(),
		payloadCh: payloadCh,
		ackCh:     ackCh,
	}

	logrus.Infof("accepting split link with id [l/%s]", xlink.Id().Token)

	if l.chAccepter != nil {
		if err := l.chAccepter.AcceptChannel(xlink, xlink.payloadCh, true); err != nil {
			logrus.Errorf("error accepting incoming channel (%v)", err)
			_ = xlink.Close()
		}

		if err := l.chAccepter.AcceptChannel(xlink, xlink.ackCh, true); err != nil {
			logrus.Errorf("error accepting incoming channel (%v)", err)
			_ = xlink.Close()
		}
	}

	if err := l.accepter.Accept(xlink); err != nil {
		logrus.Errorf("error accepting incoming Xlink (%v)", err)
	}

	logrus.Infof("accepted link [%s]", "l/"+xlink.Id().Token)

}
