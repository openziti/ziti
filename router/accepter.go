package router

import (
	"github.com/google/uuid"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v5"
	"github.com/openziti/ziti/v2/common/ctrlchan"
	"github.com/openziti/ziti/v2/router/env"
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

func (self *ctrlChannelAcceptor) HandleGroupedUnderlay(underlay channel.Underlay, closeCallback func()) (channel.Channel, error) {
	log := pfxlog.Logger().WithField("ctrlId", underlay.Id())
	log.Info("accepting inbound ctrl channel connection")

	// Ungrouped underlays (e.g. older/mixed-version peers) reach here via the ungrouped
	// fallback and carry no group secret, which channel.NewChannel requires. Generate one,
	// mirroring the controller-side accept path.
	if _, hasSecret := underlay.Headers()[channel.GroupSecretHeader]; !hasSecret {
		underlay.Headers()[channel.GroupSecretHeader] = []byte(uuid.NewString())
	}

	listenerCtrlChan := ctrlchan.NewListenerCtrlChannel()
	address := underlay.GetRemoteAddr().String()

	// WithSlowHandlerDiagnostic is temporary - see router/env/handler_diagnostic.go
	// for removal criteria.
	bindHandler := env.WithSlowHandlerDiagnostic(channel.BindHandlers(
		channel.BindHandlerF(func(binding channel.Binding) error {
			binding.AddCloseHandler(channel.CloseHandlerF(func(ch channel.Channel) {
				closeCallback()
			}))
			return self.router.ctrls.AcceptCtrlChannel(address, listenerCtrlChan, binding, underlay)
		}),
		self.router.ctrlBindhandler,
	))

	multiConfig := &channel.Config{
		LogicalName:            "ctrl/" + underlay.Id(),
		Options:                self.options,
		Underlay:               underlay,
		Binder:                 channel.MakeBinder(bindHandler),
		Senders:                listenerCtrlChan,
		MessageSourceProvider:  listenerCtrlChan,
		UnderlayEventListeners: []channel.UnderlayEventListener{listenerCtrlChan},
		// Multi-underlay-capable so the high/low-priority underlays are accepted;
		// MinTotalUnderlays closes the channel only when its last underlay is lost.
		Constraints:       listenerCtrlChan.GetConstraints(),
		MinTotalUnderlays: 1,
	}

	mc, err := channel.NewChannel(multiConfig)
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
