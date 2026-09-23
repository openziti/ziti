package handler_link

import (
	"crypto/x509"
	"math"
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

// LinkSettings supplies the link settings the bind handler reads, which can
// change at runtime through controller-managed config. An interface so
// handler_link doesn't depend on the link package, whose Subsystem implements it.
// Implementations must be safe for concurrent reads.
type LinkSettings interface {
	// Heartbeats is the heartbeat timing currently in force, as one snapshot.
	// Taken whole so a link is never configured from a mix of generations.
	Heartbeats() xlink.HeartbeatSettings
}

// LinkChannelEnv bundles what the link bind handler needs. The router satisfies
// it directly; GetForwarderImpl returns the concrete *forwarder.Forwarder, which
// GetForwarder can't without an env->forwarder import cycle.
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

	// One sample, used for both the ticker and the seed, and handed to the link
	// so its callbacks enforce the timeout belonging to these intervals.
	settings := self.linkSettings.Heartbeats()

	cb := &heartbeatCallback{
		linkId:           self.xlink.Id(),
		latencyMetric:    latencyMetric,
		queueTimeMetric:  queueTimeMetric,
		ch:               binding.GetChannel(),
		link:             self.xlink,
		latencySemaphore: concurrenz.NewSemaphore(2),
		lastResponse:     time.Now().Add(settings.CloseUnresponsiveTimeout * 2).UnixMilli(),
		generation:       settings.Generation,
	}
	hc := channel.ConfigureHeartbeat(binding, settings.SendInterval, settings.CheckInterval, cb)
	self.xlink.SetHeartbeatControl(hc, settings)

	return nil
}

func (self *bindHandler) verifyRouter(l xlink.Xlink, ch channel.Channel) error {
	// Fingerprint only the leaf certificate whose key the TLS handshake proved possession of
	// (certs[0]). Fingerprinting the rest of the presented chain would let a peer present a victim
	// router's certificate as filler and be admitted under the victim's id, since the controller
	// accepts the link if any presented fingerprint matches the claimed router's enrolled one.
	fingerprint, err := leafFingerprint(ch.Certificates())
	if err != nil {
		return errors.Wrapf(err, "unable to verify router for link %v", l.Id())
	}

	verifyLink := &ctrl_pb.VerifyRouter{
		RouterId:     l.DestinationId(),
		Fingerprints: []string{fingerprint},
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

// leafFingerprint returns the fingerprint of the leaf certificate whose key the TLS handshake proved
// possession of (certs[0]). Only the leaf is fingerprinted: fingerprinting the rest of the presented
// chain would let a peer present a victim router's certificate as filler and be admitted under the
// victim's id.
func leafFingerprint(certs []*x509.Certificate) (string, error) {
	if len(certs) == 0 {
		return "", errors.New("no certificates presented")
	}
	return nfpem.FingerprintFromCertificate(certs[0]), nil
}

// minUnhealthyThreshold is the floor for reporting a silent link as unhealthy.
const minUnhealthyThreshold = 30 * time.Second

// unhealthyThresholdMs is how long a link can go without a heartbeat response
// before it is reported unhealthy, which poisons its latency metric and so its
// routing cost: twice the send + check gap a healthy link already shows, so a
// response has actually been missed, and never less than minUnhealthyThreshold.
func unhealthyThresholdMs(settings xlink.HeartbeatSettings) int64 {
	threshold := minUnhealthyThreshold
	if gap := settings.SendInterval + settings.CheckInterval; gap > threshold/2 {
		threshold = 2 * gap
		if threshold < gap { // validation bounds the gap, not its double
			threshold = math.MaxInt64
		}
	}
	return threshold.Milliseconds()
}

// heartbeatCallback enforces one channel's heartbeat liveness. Every method runs
// on that channel's heartbeat pulse goroutine, including the response callbacks,
// which the heartbeater queues rather than calling on the rxer, so lastResponse
// and generation need no synchronization.
type heartbeatCallback struct {
	linkId           string
	latencyMetric    metrics.Histogram
	queueTimeMetric  metrics.Histogram
	lastResponse     int64
	generation       uint64
	link             xlink.Xlink
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

	unhealthy, shouldClose := self.evaluate(time.Now().UnixMilli(), self.link.HeartbeatSettings())
	if unhealthy {
		log.Warn("heartbeat not received in time, link may be unhealthy")
		self.latencyMetric.Clear()
		self.latencyMetric.Update(8888888888888)
	}
	if shouldClose {
		log.Error("heartbeat not received in time, closing router link connection")
		if err := self.ch.Close(); err != nil {
			log.WithError(err).Error("error while closing router link connection")
		}
	}

	go self.checkQueueTime()
}

// evaluate advances the deadline state for a check run at now and reports what
// that check should do. Split from CheckHeartBeat so the timing rules can be
// exercised without a live channel.
//
// A change of generation re-arms the deadline, granting one fresh timeout window:
// under the previous intervals a healthy link's last response can be most of a
// send+check cycle old, and the channel only picks up a new check interval on its
// next pulse.
func (self *heartbeatCallback) evaluate(now int64, settings xlink.HeartbeatSettings) (unhealthy, shouldClose bool) {
	if settings.Generation != self.generation {
		self.generation = settings.Generation
		self.lastResponse = now
	}

	delta := now - self.lastResponse

	// The thresholds are independent. A zero timeout means no settings have been
	// published yet, so it closes nothing.
	if timeout := settings.CloseUnresponsiveTimeout.Milliseconds(); timeout > 0 && delta > timeout {
		return true, true
	}
	return delta > unhealthyThresholdMs(settings), false
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
