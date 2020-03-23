package xlink_transport

import (
	"fmt"
	"github.com/netfoundry/ziti-fabric/router/xlink"
	"github.com/netfoundry/ziti-foundation/channel2"
	"github.com/netfoundry/ziti-foundation/identity/identity"
	"github.com/netfoundry/ziti-foundation/transport"
	"github.com/sirupsen/logrus"
)

func (self *dialerImpl) Dial(addressString string, linkId *identity.TokenId) error {
	address, err := transport.ParseAddress(addressString)
	if err == nil {
		name := "l/" + linkId.Token
		logrus.Infof("dialing link [%s]", name)

		dialer := channel2.NewClassicDialer(linkId, address, nil)
		ch, err := channel2.NewChannel(name, dialer, self.config.options)
		if err == nil {
			xlink := &impl{id: linkId, ch: ch}

			if self.chAccepter != nil {
				if err := self.chAccepter.AcceptChannel(xlink, ch); err != nil {
					logrus.Errorf("error accepting outgoing channel (%w)", err)
				}
			}

			if err := self.accepter.Accept(xlink); err != nil {
				return fmt.Errorf("error accepting outgoing Xlink (%w)", err)
			}

			return nil

		} else {
			return fmt.Errorf("error dialing link [%s] (%w)", name, err)
		}
	} else {
		return fmt.Errorf("error parsing link address [%s] (%w)", addressString, err)
	}
}

type dialerImpl struct {
	id         *identity.TokenId
	config     *dialerConfig
	accepter   xlink.Accepter
	chAccepter ChannelAccepter
}
