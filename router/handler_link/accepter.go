package handler_link

import (
	"crypto/sha1"
	"fmt"
	"github.com/openziti/channel"
	"github.com/openziti/channel/protobufs"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/fabric/router/forwarder"
	metrics2 "github.com/openziti/fabric/router/metrics"
	"github.com/openziti/fabric/router/xgress"
	"github.com/openziti/fabric/router/xlink"
	"github.com/openziti/fabric/router/xlink_transport"
	"github.com/openziti/fabric/trace"
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/foundation/metrics"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"time"
)

func NewChannelAccepter(c xgress.CtrlChannel, f *forwarder.Forwarder, fo *forwarder.Options, mr metrics.Registry) xlink_transport.ChannelAccepter {
	return &channelAccepter{
		ctrl:             c,
		forwarder:        f,
		forwarderOptions: fo,
		metricsRegistry:  mr,
	}
}

func (self *channelAccepter) AcceptChannel(xlink xlink.Xlink, ch channel2.Channel, trackLatency bool, listenerSide bool) error {
	if listenerSide {
		if err := self.verifyLink(xlink, ch); err != nil {
			return err
		}
	}

	closeNotify := make(chan struct{})

	ch.SetLogicalName("l/" + xlink.Id().Token)
	ch.SetUserData(xlink.Id().Token)
	ch.AddCloseHandler(newCloseHandler(xlink, self.ctrl, self.forwarder, closeNotify))
	ch.AddErrorHandler(newErrorHandler(xlink, self.ctrl))
	ch.AddReceiveHandler(newPayloadHandler(xlink, self.forwarder))
	ch.AddReceiveHandler(newQueuingAckHandler(xlink, self.forwarder, closeNotify))
	ch.AddReceiveHandler(&channel2.LatencyHandler{})
	ch.AddReceiveHandler(newControlHandler(xlink, ch, self.forwarder, closeNotify))
	ch.AddPeekHandler(metrics2.NewChannelPeekHandler(xlink.Id().Token, self.forwarder.MetricsRegistry()))
	ch.AddPeekHandler(trace.NewChannel2PeekHandler(xlink.Id().Token, ch, self.forwarder.TraceController(), trace.NewChannelSink(self.ctrl.Channel())))

	if trackLatency {
		go metrics.ProbeLatency(
			ch,
			self.metricsRegistry.Histogram("link."+xlink.Id().Token+".latency"),
			self.forwarderOptions.LatencyProbeInterval,
			self.forwarderOptions.LatencyProbeTimeout,
		)
	}

	return nil
}

func (self *channelAccepter) verifyLink(l xlink.Xlink, ch channel2.Channel) error {
	verifyLink := &ctrl_pb.VerifyLink{
		LinkId: l.Id().Token,
	}
	for _, cert := range ch.Certificates() {
		fingerprint := fmt.Sprintf("%x", sha1.Sum(cert.Raw))
		verifyLink.Fingerprints = append(verifyLink.Fingerprints, fingerprint)
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

type channelAccepter struct {
	ctrl             xgress.CtrlChannel
	forwarder        *forwarder.Forwarder
	forwarderOptions *forwarder.Options
	metricsRegistry  metrics.Registry
}
