package handler_link

import (
	"github.com/netfoundry/ziti-fabric/router/forwarder"
	"github.com/netfoundry/ziti-fabric/router/xgress"
	"github.com/netfoundry/ziti-fabric/router/xlink"
	"github.com/netfoundry/ziti-fabric/router/xlink_transport"
	"github.com/netfoundry/ziti-fabric/trace"
	"github.com/netfoundry/ziti-foundation/channel2"
	"github.com/netfoundry/ziti-foundation/metrics"
)

func NewChannelAccepter(c xgress.CtrlChannel, f *forwarder.Forwarder, fo *forwarder.Options, mr metrics.Registry) xlink_transport.ChannelAccepter {
	return &channelAccepter{
		ctrl:             c,
		forwarder:        f,
		forwarderOptions: fo,
		metricsRegistry:  mr,
	}
}

func (self *channelAccepter) AcceptChannel(xlink xlink.Xlink, ch channel2.Channel) error {
	ch.SetLogicalName("l/" + xlink.Id().Token)
	ch.SetUserData(xlink.Id().Token)
	ch.AddCloseHandler(newCloseHandler(xlink, self.ctrl, self.forwarder))
	ch.AddErrorHandler(newErrorHandler(xlink, self.ctrl))
	ch.AddReceiveHandler(newPayloadHandler(xlink, self.ctrl, self.forwarder))
	ch.AddReceiveHandler(newAckHandler(xlink, self.ctrl, self.forwarder))
	ch.AddReceiveHandler(&channel2.LatencyHandler{})
	ch.AddPeekHandler(metrics.NewChannelPeekHandler(xlink.Id().Token, self.forwarder.MetricsRegistry()))
	ch.AddPeekHandler(trace.NewChannelPeekHandler(xlink.Id(), ch, self.forwarder.TraceController(), trace.NewChannelSink(self.ctrl.Channel())))

	go metrics.ProbeLatency(
		ch,
		self.metricsRegistry.Histogram("link."+xlink.Id().Token+".latency"),
		self.forwarderOptions.LatencyProbeInterval,
	)

	return nil
}

type channelAccepter struct {
	ctrl             xgress.CtrlChannel
	forwarder        *forwarder.Forwarder
	forwarderOptions *forwarder.Options
	metricsRegistry  metrics.Registry
}
