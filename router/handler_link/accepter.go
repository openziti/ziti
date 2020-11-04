package handler_link

import (
	"github.com/openziti/fabric/router/forwarder"
	metrics2 "github.com/openziti/fabric/router/metrics"
	"github.com/openziti/fabric/router/xgress"
	"github.com/openziti/fabric/router/xlink"
	"github.com/openziti/fabric/router/xlink_transport"
	"github.com/openziti/fabric/trace"
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/foundation/metrics"
)

func NewChannelAccepter(c xgress.CtrlChannel, f *forwarder.Forwarder, fo *forwarder.Options, mr metrics.Registry) xlink_transport.ChannelAccepter {
	return &channelAccepter{
		ctrl:             c,
		forwarder:        f,
		forwarderOptions: fo,
		metricsRegistry:  mr,
	}
}

func (self *channelAccepter) AcceptChannel(xlink xlink.Xlink, ch channel2.Channel, trackLatency bool) error {
	ch.SetLogicalName("l/" + xlink.Id().Token)
	ch.SetUserData(xlink.Id().Token)
	ch.AddCloseHandler(newCloseHandler(xlink, self.ctrl, self.forwarder))
	ch.AddErrorHandler(newErrorHandler(xlink, self.ctrl))
	ch.AddReceiveHandler(newPayloadHandler(xlink, self.ctrl, self.forwarder))
	ch.AddReceiveHandler(newQueuingAckHandler(xlink, self.ctrl, self.forwarder))
	ch.AddReceiveHandler(&channel2.LatencyHandler{})
	ch.AddPeekHandler(metrics2.NewChannelPeekHandler(xlink.Id().Token, self.forwarder.MetricsRegistry()))
	ch.AddPeekHandler(trace.NewChannelPeekHandler(xlink.Id(), ch, self.forwarder.TraceController(), trace.NewChannelSink(self.ctrl.Channel())))

	if trackLatency {
		go metrics.ProbeLatency(
			ch,
			self.metricsRegistry.Histogram("link."+xlink.Id().Token+".latency"),
			self.forwarderOptions.LatencyProbeInterval,
		)
	}

	return nil
}

type channelAccepter struct {
	ctrl             xgress.CtrlChannel
	forwarder        *forwarder.Forwarder
	forwarderOptions *forwarder.Options
	metricsRegistry  metrics.Registry
}
