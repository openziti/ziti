package router

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/ziti/v2/common/ctrlchan"
	"github.com/openziti/ziti/v2/router/forwarder"
	"github.com/openziti/ziti/v2/router/xlink"
	"github.com/sirupsen/logrus"
)

func newXlinkAccepter(f *forwarder.Forwarder) xlink.Acceptor {
	return &xlinkAccepter{
		forwarder: f,
	}
}

func (self *xlinkAccepter) Accept(xlink xlink.Xlink) error {
	if err := self.forwarder.RegisterLink(xlink); err != nil {
		return err
	}
	logrus.WithField("linkId", xlink.Id()).
		WithField("destId", xlink.DestinationId()).
		WithField("iteration", xlink.Iteration()).
		WithField("dialed", xlink.IsDialed()).
		Info("accepted new link")
	return nil
}

type xlinkAccepter struct {
	forwarder *forwarder.Forwarder
}

type ctrlChannelAcceptor struct {
	router  *Router
	options *channel.Options
}

func (self *ctrlChannelAcceptor) HandleGroupedUnderlay(underlay channel.Underlay, closeCallback func()) (channel.MultiChannel, error) {
	log := pfxlog.Logger().WithField("ctrlId", underlay.Id())
	log.Info("accepting inbound ctrl channel connection")

	listenerCtrlChan := ctrlchan.NewListenerCtrlChannel()
	address := underlay.GetRemoteAddr().String()

	bindHandler := channel.BindHandlers(
		channel.BindHandlerF(func(binding channel.Binding) error {
			binding.AddCloseHandler(channel.CloseHandlerF(func(ch channel.Channel) {
				closeCallback()
			}))
			return self.router.ctrls.AcceptCtrlChannel(address, listenerCtrlChan, binding, underlay)
		}),
		self.router.ctrlBindhandler,
	)

	multiConfig := &channel.MultiChannelConfig{
		LogicalName:     "ctrl/" + underlay.Id(),
		Options:         self.options,
		UnderlayHandler: listenerCtrlChan,
		BindHandler:     bindHandler,
		Underlay:        underlay,
	}

	mc, err := channel.NewMultiChannel(multiConfig)
	if err != nil {
		pfxlog.Logger().WithError(err).Errorf("failure accepting ctrl channel %v with multi-underlay", underlay.Label())
		return nil, err
	}

	self.router.NotifyOfReconnect(listenerCtrlChan)
	return mc, nil
}

func (self *ctrlChannelAcceptor) AcceptUnderlay(underlay channel.Underlay) error {
	_, err := self.HandleGroupedUnderlay(underlay, func() {})
	return err
}
