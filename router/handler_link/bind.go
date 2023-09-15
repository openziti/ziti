package handler_link

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2"
	"github.com/openziti/channel/v2/latency"
	"github.com/openziti/channel/v2/protobufs"
	"github.com/openziti/fabric/common/pb/ctrl_pb"
	"github.com/openziti/fabric/common/trace"
	"github.com/openziti/fabric/router/env"
	"github.com/openziti/fabric/router/forwarder"
	metrics2 "github.com/openziti/fabric/router/metrics"
	"github.com/openziti/fabric/router/xlink"
	"github.com/openziti/foundation/v2/concurrenz"
	nfpem "github.com/openziti/foundation/v2/pem"
	"github.com/openziti/metrics"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"time"
)

func NewBindHandlerFactory(c env.NetworkControllers, f *forwarder.Forwarder, hbo *channel.HeartbeatOptions, mr metrics.Registry, registry xlink.Registry) *bindHandlerFactory {
	return &bindHandlerFactory{
		ctrl:             c,
		forwarder:        f,
		metricsRegistry:  mr,
		xlinkRegistry:    registry,
		heartbeatOptions: hbo,
	}
}

type bindHandlerFactory struct {
	ctrl             env.NetworkControllers
	forwarder        *forwarder.Forwarder
	metricsRegistry  metrics.Registry
	xlinkRegistry    xlink.Registry
	heartbeatOptions *channel.HeartbeatOptions
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
		if err := self.verifyRouter(self.xlink, ch); err != nil {
			return err
		}
	}

	log := pfxlog.Logger().WithFields(map[string]interface{}{
		"linkId":        self.xlink.Id(),
		"routerId":      self.xlink.DestinationId(),
		"routerVersion": self.xlink.DestVersion(),
	})

	closeNotify := make(chan struct{})

	binding.GetChannel().SetLogicalName("l/" + self.xlink.Id())
	binding.SetUserData(self.xlink.Id())
	binding.AddCloseHandler(newCloseHandler(self.xlink, self.ctrl, self.forwarder, closeNotify, self.xlinkRegistry))
	binding.AddErrorHandler(newErrorHandler(self.xlink, self.ctrl))
	binding.AddTypedReceiveHandler(newPayloadHandler(self.xlink, self.forwarder))
	binding.AddTypedReceiveHandler(newAckHandler(self.xlink, self.forwarder, closeNotify))
	binding.AddTypedReceiveHandler(&latency.LatencyHandler{})
	binding.AddTypedReceiveHandler(newControlHandler(self.xlink, self.forwarder))
	binding.AddPeekHandler(metrics2.NewChannelPeekHandler(self.xlink.Id(), self.forwarder.MetricsRegistry()))
	binding.AddPeekHandler(trace.NewChannelPeekHandler(self.xlink.Id(), ch, self.forwarder.TraceController()))
	if err := self.xlink.Init(self.forwarder.MetricsRegistry()); err != nil {
		return err
	}

	latencyMetric := self.metricsRegistry.Histogram("link." + self.xlink.Id() + ".latency")
	queueTimeMetric := self.metricsRegistry.Histogram("link." + self.xlink.Id() + ".queue_time")
	binding.AddCloseHandler(channel.CloseHandlerF(func(ch channel.Channel) {
		latencyMetric.Dispose()
		queueTimeMetric.Dispose()
	}))

	log.Info("link destination support heartbeats")
	cb := &heartbeatCallback{
		latencyMetric:    latencyMetric,
		queueTimeMetric:  queueTimeMetric,
		ch:               binding.GetChannel(),
		heartbeatOptions: self.heartbeatOptions,
		latencySemaphore: concurrenz.NewSemaphore(2),
		lastResponse:     time.Now().Add(self.heartbeatOptions.CloseUnresponsiveTimeout * 2).UnixMilli(),
	}
	channel.ConfigureHeartbeat(binding, 10*time.Second, time.Second, cb)

	return nil
}

func (self *bindHandler) verifyRouter(l xlink.Xlink, ch channel.Channel) error {
	var fingerprints []string
	for _, cert := range ch.Certificates() {
		fingerprints = append(fingerprints, nfpem.FingerprintFromCertificate(cert))
	}

	verifyLink := &ctrl_pb.VerifyRouter{
		RouterId:     l.DestinationId(),
		Fingerprints: fingerprints,
	}

	ctrlCh := self.ctrl.AnyCtrlChannel()
	if ctrlCh == nil {
		return errors.Errorf("unable to verify link %v, no controller available", l.Id())
	}

	reply, err := protobufs.MarshalTyped(verifyLink).WithTimeout(10 * time.Second).SendForReply(ctrlCh)
	if err != nil {
		return errors.Wrapf(err, "unable to verify router %v for link %v", l.DestinationId(), l.Id())
	}

	if reply.ContentType != channel.ContentTypeResultType {
		return errors.Errorf("unexpected response type to verify link: %v", reply.ContentType)
	}

	result := channel.UnmarshalResult(reply)
	if result.Success {
		logrus.WithField("linkId", l.Id()).
			WithField("routerId", l.DestinationId()).
			Info("successfully verified router for link")
		return nil
	}

	return errors.Errorf("unable to verify link [%v]", result.Message)
}

type heartbeatCallback struct {
	latencyMetric    metrics.Histogram
	queueTimeMetric  metrics.Histogram
	lastResponse     int64
	heartbeatOptions *channel.HeartbeatOptions
	ch               channel.Channel
	latencySemaphore concurrenz.Semaphore
}

func (self *heartbeatCallback) HeartbeatTx(int64) {}

func (self *heartbeatCallback) HeartbeatRx(int64) {}

func (self *heartbeatCallback) HeartbeatRespTx(int64) {}

func (self *heartbeatCallback) HeartbeatRespRx(ts int64) {
	now := time.Now()
	self.lastResponse = now.UnixMilli()
	self.latencyMetric.Update(now.UnixNano() - ts)
}

func (self *heartbeatCallback) CheckHeartBeat() {
	log := pfxlog.Logger().WithField("channelId", self.ch.Label())
	now := time.Now().UnixMilli()
	if delta := now - self.lastResponse; delta > 30000 {
		log.Warn("heartbeat not received in time, link may be unhealthy")
		self.latencyMetric.Clear()
		self.latencyMetric.Update(88888888888)

		if delta > self.heartbeatOptions.CloseUnresponsiveTimeout.Milliseconds() {
			log.Error("heartbeat not received in time, closing router link connection")
			if err := self.ch.Close(); err != nil {
				log.WithError(err).Error("error while closing router link connection")
			}
		}
	}

	go self.checkQueueTime()
}

func (self *heartbeatCallback) checkQueueTime() {
	log := pfxlog.Logger().WithField("linkId", self.ch.Id())
	if !self.latencySemaphore.TryAcquire() {
		log.Warn("unable to check queue time, too many check already running")
		return
	}

	defer self.latencySemaphore.Release()

	sendTracker := &latency.SendTimeTracker{
		Handler: func(latencyType latency.Type, latency time.Duration) {
			self.queueTimeMetric.Update(latency.Nanoseconds())
		},
		StartTime: time.Now(),
	}
	if err := self.ch.Send(sendTracker); err != nil && !self.ch.IsClosed() {
		log.WithError(err).Error("unable to send queue time tracer")
	}
}
