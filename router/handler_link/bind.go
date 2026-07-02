package handler_link

import (
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v5"
	"github.com/openziti/channel/v5/latency"
	"github.com/openziti/channel/v5/protobufs"
	"github.com/openziti/foundation/v2/concurrenz"
	nfpem "github.com/openziti/foundation/v2/pem"
	"github.com/openziti/metrics"
	"github.com/openziti/sdk-golang/v2/xgress"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/common/servermetrics"
	"github.com/openziti/ziti/v2/common/trace"
	"github.com/openziti/ziti/v2/router/env"
	"github.com/openziti/ziti/v2/router/forwarder"
	metrics2 "github.com/openziti/ziti/v2/router/metrics"
	"github.com/openziti/ziti/v2/router/xlink"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// LinkSettings supplies the per-link settings the bind handler reads, which can
// change at runtime via controller-managed config. It's an interface so
// handler_link doesn't depend on the link package; the link subsystem's
// FactoryRegistry satisfies it. Reads happen on the per-link heartbeat check,
// so the implementation must be safe for concurrent reads. New managed-config
// values (heartbeat intervals, queue sizes) get added here as they're wired up.
type LinkSettings interface {
	// CloseUnresponsiveTimeout is the current threshold after which an
	// unresponsive link is closed.
	CloseUnresponsiveTimeout() time.Duration
	// SendInterval is how often a heartbeat is sent on an otherwise idle link.
	SendInterval() time.Duration
	// CheckInterval is how often the heartbeat loop wakes to send/check.
	CheckInterval() time.Duration
}

// LinkChannelEnv bundles everything the link bind handler needs. Passing a
// single env keeps the constructor stable as more inputs are added. The router
// satisfies it directly; GetForwarderImpl returns the concrete
// *forwarder.Forwarder (distinct from the router's env.Forwarder-returning
// GetForwarder, which can't, to avoid an env->forwarder import cycle).
type LinkChannelEnv interface {
	GetNetworkControllers() env.NetworkControllers
	GetForwarderImpl() *forwarder.Forwarder
	GetMetricsRegistry() servermetrics.UsageRegistry
	GetXlinkRegistry() xlink.Registry
	GetLinkSettings() LinkSettings
}

func NewBindHandlerFactory(linkEnv LinkChannelEnv) *bindHandlerFactory {
	return &bindHandlerFactory{
		ctrl:            linkEnv.GetNetworkControllers(),
		forwarder:       linkEnv.GetForwarderImpl(),
		metricsRegistry: linkEnv.GetMetricsRegistry(),
		xlinkRegistry:   linkEnv.GetXlinkRegistry(),
		linkSettings:    linkEnv.GetLinkSettings(),
	}
}

type bindHandlerFactory struct {
	ctrl            env.NetworkControllers
	forwarder       *forwarder.Forwarder
	metricsRegistry metrics.Registry
	xlinkRegistry   xlink.Registry
	linkSettings    LinkSettings
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
		"iteration":     self.xlink.Iteration(),
		"dialed":        self.xlink.IsDialed(),
	})

	binding.GetChannel().SetLogicalName("l/" + self.xlink.Id())
	binding.SetUserData(self.xlink.Id())
	binding.AddCloseHandler(newCloseHandler(self.xlink, self.forwarder, self.xlinkRegistry))
	binding.AddErrorHandler(newErrorHandler(self.xlink, self.ctrl))
	channel.AddReceiveHandlers(binding, newPayloadHandler(self.xlink, self.forwarder))
	channel.AddReceiveHandlers(binding, newAckHandler(self.xlink, self.forwarder))
	binding.AddReceiveHandler(channel.ContentTypeLatencyType, &latency.LatencyHandler{})
	channel.AddReceiveHandlers(binding, newControlHandler(self.xlink, self.forwarder))
	binding.AddPeekHandler(metrics2.NewChannelPeekHandler(self.xlink.Id(), self.forwarder.MetricsRegistry()))
	binding.AddPeekHandler(trace.NewChannelPeekHandler(self.xlink.Id(), ch, self.forwarder.TraceController()))
	if self.xlink.LinkProtocol() == "dtls" {
		binding.AddTransformHandler(xgress.PayloadTransformer{})
	}
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
		linkId:           self.xlink.Id(),
		latencyMetric:    latencyMetric,
		queueTimeMetric:  queueTimeMetric,
		ch:               binding.GetChannel(),
		linkSettings:     self.linkSettings,
		latencySemaphore: concurrenz.NewSemaphore(2),
		lastResponse:     time.Now().Add(self.linkSettings.CloseUnresponsiveTimeout() * 2).UnixMilli(),
	}
	hc := channel.ConfigureHeartbeat(binding, self.linkSettings.SendInterval(), self.linkSettings.CheckInterval(), cb)
	self.xlink.SetHeartbeatControl(hc)

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

	ctrlCh := self.ctrl.AnyChannel()
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
	linkId           string
	latencyMetric    metrics.Histogram
	queueTimeMetric  metrics.Histogram
	lastResponse     int64
	linkSettings     LinkSettings
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
		self.latencyMetric.Update(8888888888888)

		if delta > self.linkSettings.CloseUnresponsiveTimeout().Milliseconds() {
			log.Error("heartbeat not received in time, closing router link connection")
			if err := self.ch.Close(); err != nil {
				log.WithError(err).Error("error while closing router link connection")
			}
		}
	}

	go self.checkQueueTime()
}

func (self *heartbeatCallback) checkQueueTime() {
	log := pfxlog.Logger().WithField("linkId", self.linkId)
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
