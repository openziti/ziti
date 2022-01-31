package handler_link

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel"
	"github.com/openziti/channel/latency"
	"github.com/openziti/channel/protobufs"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/fabric/router/forwarder"
	metrics2 "github.com/openziti/fabric/router/metrics"
	"github.com/openziti/fabric/router/xgress"
	"github.com/openziti/fabric/router/xlink"
	"github.com/openziti/fabric/trace"
	"github.com/openziti/foundation/metrics"
	nfpem "github.com/openziti/foundation/util/pem"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"time"
)

func NewBindHandlerFactory(c xgress.CtrlChannel, f *forwarder.Forwarder, fo *forwarder.Options, mr metrics.Registry) *bindHandlerFactory {
	return &bindHandlerFactory{
		ctrl:             c,
		forwarder:        f,
		forwarderOptions: fo,
		metricsRegistry:  mr,
	}
}

type bindHandlerFactory struct {
	ctrl             xgress.CtrlChannel
	forwarder        *forwarder.Forwarder
	forwarderOptions *forwarder.Options
	metricsRegistry  metrics.Registry
}

func (self *bindHandlerFactory) NewBindHandler(link xlink.Xlink, latency bool, listenerSide bool) channel.BindHandler {
	return &bindHandler{
		bindHandlerFactory: self,
		xlink:              link,
		trackLatency:       latency,
		listenerSide:       listenerSide,
	}
}

type bindHandler struct {
	*bindHandlerFactory
	xlink        xlink.Xlink
	trackLatency bool
	listenerSide bool
}

func (self *bindHandler) BindChannel(binding channel.Binding) error {
	ch := binding.GetChannel()
	if self.listenerSide {
		if err := self.verifyLink(self.xlink, ch); err != nil {
			return err
		}
	}

	closeNotify := make(chan struct{})

	binding.GetChannel().SetLogicalName("l/" + self.xlink.Id().Token)
	binding.SetUserData(self.xlink.Id().Token)
	binding.AddCloseHandler(newCloseHandler(self.xlink, self.ctrl, self.forwarder, closeNotify))
	binding.AddErrorHandler(newErrorHandler(self.xlink, self.ctrl))
	binding.AddTypedReceiveHandler(newPayloadHandler(self.xlink, self.forwarder))
	binding.AddTypedReceiveHandler(newQueuingAckHandler(self.xlink, self.forwarder, closeNotify))
	binding.AddTypedReceiveHandler(&latency.LatencyHandler{})
	binding.AddTypedReceiveHandler(newControlHandler(self.xlink, self.forwarder, closeNotify))
	binding.AddPeekHandler(metrics2.NewChannelPeekHandler(self.xlink.Id().Token, self.forwarder.MetricsRegistry()))
	binding.AddPeekHandler(trace.NewChannelPeekHandler(self.xlink.Id().Token, ch, self.forwarder.TraceController(), trace.NewChannelSink(self.ctrl.Channel())))

	if self.trackLatency {
		latencyMetric := self.metricsRegistry.Histogram("link." + self.xlink.Id().Token + ".latency")
		latencyTimeout := self.forwarderOptions.LatencyProbeTimeout
		config := &latency.ProbeConfig{
			Channel:  ch,
			Interval: self.forwarderOptions.LatencyProbeInterval,
			Timeout:  latencyTimeout,
			ResultHandler: func(resultNanos int64) {
				latencyMetric.Update(resultNanos)
			},
			TimeoutHandler: func() {
				pfxlog.ContextLogger(ch.Label()).Errorf("latency timeout after [%s] on channel [%s]", latencyTimeout, ch.Label())
			},
			ExitHandler: func() {
				latencyMetric.Dispose()
			},
		}

		go latency.ProbeLatencyConfigurable(config)
	}

	return nil
}

func (self *bindHandler) verifyLink(l xlink.Xlink, ch channel.Channel) error {
	var fingerprints []string
	for _, cert := range ch.Certificates() {
		fingerprints = append(fingerprints, nfpem.FingerprintFromX509(cert))
	}

	verifyLink := &ctrl_pb.VerifyLink{
		LinkId:       l.Id().Token,
		Fingerprints: fingerprints,
	}

	reply, err := protobufs.MarshalTyped(verifyLink).WithTimeout(10 * time.Second).SendForReply(self.ctrl.Channel())
	if err != nil {
		return errors.Wrapf(err, "unable to verify link %v", l.Id().Token)
	}

	if reply.ContentType != channel.ContentTypeResultType {
		return errors.Errorf("unexpected response type to verify link: %v", reply.ContentType)
	}

	result := channel.UnmarshalResult(reply)
	if result.Success {
		logrus.WithField("linkId", l.Id().Token).Info("successfully verified link")
		return nil
	}

	return errors.Errorf("unable to verify link [%v]", result.Message)
}
