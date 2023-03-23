/*
	Copyright NetFoundry Inc.

	Licensed under the Apache License, Version 2.0 (the "License");
	you may not use this file except in compliance with the License.
	You may obtain a copy of the License at

	https://www.apache.org/licenses/LICENSE-2.0

	Unless required by applicable law or agreed to in writing, software
	distributed under the License is distributed on an "AS IS" BASIS,
	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
	See the License for the specific language governing permissions and
	limitations under the License.
*/

package handler_ctrl

import (
	"github.com/sirupsen/logrus"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2"
	"github.com/openziti/channel/v2/latency"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/controller/xctrl"
	metrics2 "github.com/openziti/fabric/router/metrics"
	"github.com/openziti/fabric/trace"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/metrics"
)

type bindHandler struct {
	heartbeatOptions *channel.HeartbeatOptions
	router           *network.Router
	network          *network.Network
	xctrls           []xctrl.Xctrl
}

func newBindHandler(heartbeatOptions *channel.HeartbeatOptions, router *network.Router, network *network.Network, xctrls []xctrl.Xctrl) channel.BindHandler {
	return &bindHandler{
		heartbeatOptions: heartbeatOptions,
		router:           router,
		network:          network,
		xctrls:           xctrls,
	}
}

func (self *bindHandler) BindChannel(binding channel.Binding) error {
	log := pfxlog.Logger().WithFields(map[string]interface{}{
		"routerId":      self.router.Id,
		"routerVersion": self.router.VersionInfo.Version,
	})

	traceDispatchWrapper := trace.NewDispatchWrapper(self.network.GetEventDispatcher().Dispatch)
	binding.AddTypedReceiveHandler(newCircuitRequestHandler(self.router, self.network))
	binding.AddTypedReceiveHandler(newRouteResultHandler(self.network, self.router))
	binding.AddTypedReceiveHandler(newCircuitConfirmationHandler(self.network, self.router))
	binding.AddTypedReceiveHandler(newCreateTerminatorHandler(self.network, self.router))
	binding.AddTypedReceiveHandler(newRemoveTerminatorHandler(self.network))
	binding.AddTypedReceiveHandler(newRemoveTerminatorsHandler(self.network))
	binding.AddTypedReceiveHandler(newUpdateTerminatorHandler(self.network))
	binding.AddTypedReceiveHandler(newLinkConnectedHandler(self.router, self.network))
	binding.AddTypedReceiveHandler(newRouterLinkHandler(self.router, self.network))
	binding.AddTypedReceiveHandler(newVerifyLinkHandler(self.router, self.network))
	binding.AddTypedReceiveHandler(newVerifyRouterHandler(self.router, self.network))
	binding.AddTypedReceiveHandler(newFaultHandler(self.router, self.network))
	binding.AddTypedReceiveHandler(newMetricsHandler(self.network))
	binding.AddTypedReceiveHandler(newTraceHandler(traceDispatchWrapper))
	binding.AddTypedReceiveHandler(newInspectHandler(self.network))
	binding.AddTypedReceiveHandler(newPingHandler())
	binding.AddPeekHandler(trace.NewChannelPeekHandler(self.network.GetAppId(), binding.GetChannel(), self.network.GetTraceController()))
	binding.AddPeekHandler(metrics2.NewCtrlChannelPeekHandler(self.router.Id, self.network.GetMetricsRegistry()))

	doHeartbeat, err := self.router.VersionInfo.HasMinimumVersion("0.25.5")
	if err != nil {
		doHeartbeat = false
		log.WithError(err).Error("version parsing error")
	}

	roundTripHistogram := self.network.GetMetricsRegistry().Histogram("ctrl.latency:" + self.router.Id)
	queueTimeHistogram := self.network.GetMetricsRegistry().Histogram("ctrl.queue_time:" + self.router.Id)
	binding.AddCloseHandler(channel.CloseHandlerF(func(ch channel.Channel) {
		roundTripHistogram.Dispose()
		queueTimeHistogram.Dispose()
	}))

	if doHeartbeat {
		log.Info("router supports heartbeats")
		cb := &heartbeatCallback{
			latencyMetric:            roundTripHistogram,
			queueTimeMetric:          queueTimeHistogram,
			ch:                       binding.GetChannel(),
			latencySemaphore:         concurrenz.NewSemaphore(2),
			closeUnresponsiveTimeout: self.heartbeatOptions.CloseUnresponsiveTimeout,
			lastResponse:             time.Now().Add(self.heartbeatOptions.CloseUnresponsiveTimeout * 2).UnixMilli(), // wait at least 2x timeout before closing
		}
		channel.ConfigureHeartbeat(binding, self.heartbeatOptions.SendInterval, self.heartbeatOptions.CheckInterval, cb)
	} else if supportLatency, err := self.router.VersionInfo.HasMinimumVersion("0.18.7"); supportLatency && err == nil {
		log.Info("router does not support heartbeats, using latency probe")
		latencyHandler := &ctrlChannelLatencyHandler{
			roundTripHistogram: roundTripHistogram,
			queueTimeHistogram: queueTimeHistogram,
		}
		latency.AddLatencyProbe(binding.GetChannel(), binding, self.network.GetOptions().CtrlChanLatencyInterval/time.Duration(10), 10, latencyHandler.HandleLatency)
	} else if err != nil {
		log.WithError(err).Error("version parsing error")
	}

	xctrlDone := make(chan struct{})
	for _, x := range self.xctrls {
		if err := binding.Bind(x); err != nil {
			return err
		}
		if err := x.Run(binding.GetChannel(), self.network.GetDb(), xctrlDone); err != nil {
			return err
		}
	}
	if len(self.xctrls) > 0 {
		binding.AddCloseHandler(newXctrlCloseHandler(xctrlDone))
	}

	binding.AddCloseHandler(newCloseHandler(self.router, self.network))
	return nil
}

type ctrlChannelLatencyHandler struct {
	roundTripHistogram metrics.Histogram
	queueTimeHistogram metrics.Histogram
}

func (self *ctrlChannelLatencyHandler) HandleLatency(latencyType latency.Type, elapsed time.Duration) {
	if latencyType == latency.RoundTripType {
		self.roundTripHistogram.Update(elapsed.Nanoseconds())
	} else if latencyType == latency.BeforeSendType {
		self.queueTimeHistogram.Update(elapsed.Nanoseconds())
	}
}

type heartbeatCallback struct {
	latencyMetric            metrics.Histogram
	queueTimeMetric          metrics.Histogram
	lastResponse             int64
	ch                       channel.Channel
	latencySemaphore         concurrenz.Semaphore
	closeUnresponsiveTimeout time.Duration
}

func (self *heartbeatCallback) HeartbeatTx(int64) {}

func (self *heartbeatCallback) HeartbeatRx(int64) {}

func (self *heartbeatCallback) HeartbeatRespTx(int64) {}

func (self *heartbeatCallback) HeartbeatRespRx(ts int64) {
	now := time.Now()
	self.lastResponse = now.UnixMilli()
	self.latencyMetric.Update(now.UnixNano() - ts)
}

func (self *heartbeatCallback) timeSinceLastResponse(nowUnixMillis int64) time.Duration {
	return time.Duration(nowUnixMillis-self.lastResponse) * time.Millisecond
}

func (self *heartbeatCallback) CheckHeartBeat() {
	now := time.Now().UnixMilli()
	if self.timeSinceLastResponse(now) > self.closeUnresponsiveTimeout {
		log := self.logger()
		log.Error("heartbeat not received in time, closing link")
		if err := self.ch.Close(); err != nil {
			log.WithError(err).Error("error while closing link")
		}
	}
	go self.checkQueueTime()
}

func (self *heartbeatCallback) checkQueueTime() {
	if !self.latencySemaphore.TryAcquire() {
		self.logger().Warn("unable to check queue time, too many check already running")
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
		self.logger().WithError(err).Error("unable to send queue time tracer")
	}
}

func (self *heartbeatCallback) logger() *logrus.Entry {
	return pfxlog.Logger().WithField("channelType", "router").WithField("channelId", self.ch.Id())
}
