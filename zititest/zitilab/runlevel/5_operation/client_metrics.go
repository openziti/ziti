/*
	Copyright 2019 NetFoundry Inc.

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

package zitilib_runlevel_5_operation

import (
	"encoding/binary"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/ziti/v2/common/pb/mgmt_pb"
	"github.com/openziti/ziti/zititest/ziti-traffic-test/loop4"
	zitilibActions "github.com/openziti/ziti/zititest/zitilab/actions"
	"github.com/openziti/ziti/zititest/zitilab/cli"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

const (
	SimControllerName = "sim-controller"
)

func NewSimServices(hostSelectorF func(string) string) *SimServices {
	return &SimServices{
		idToSelectorMapper: hostSelectorF,
	}
}

type SimServices struct {
	listener           net.Listener
	model              *model.Model
	idToSelectorMapper func(string) string
	lock               sync.Mutex
	zitiContext        ziti.Context
	metricsStarted     atomic.Bool

	remoteController *loop4.RemoteController
}

func (self *SimServices) SetupSimControllerIdentity(run model.Run) error {
	if err := zitilibActions.EdgeExec(run.GetModel(), "delete", "identity", SimControllerName); err != nil {
		return err
	}

	jwtFilePath := run.GetLabel().GetFilePath("sim-controller.jwt")
	if err := zitilibActions.EdgeExec(run.GetModel(), "create", "identity", SimControllerName, "-a", "metrics-host,sim-services-host", "-o", jwtFilePath); err != nil {
		return err
	}

	identityConfigPath := run.GetLabel().GetFilePath("sim-controller.json")
	if _, err := cli.Exec(run.GetModel(), "edge", "enroll", jwtFilePath, "-o", identityConfigPath); err != nil {
		return err
	}

	return nil
}

func (self *SimServices) GetZitiContext(run model.Run) (ziti.Context, error) {
	self.lock.Lock()
	defer self.lock.Unlock()
	if self.zitiContext == nil {
		identityConfigPath := run.GetLabel().GetFilePath("sim-controller.json")
		pfxlog.Logger().Infof("loading ziti config from [%s]", identityConfigPath)
		cfg, err := ziti.NewConfigFromFile(identityConfigPath)
		if err != nil {
			return nil, err
		}
		pfxlog.Logger().Infof("loading ziti context from [%s]", identityConfigPath)
		context, err := ziti.NewContext(cfg)
		if err != nil {
			return nil, err
		}
		self.zitiContext = context
	}

	return self.zitiContext, nil
}

func (self *SimServices) CollectSimMetrics(run model.Run, service string) error {
	if !self.metricsStarted.CompareAndSwap(false, true) {
		return nil
	}

	self.model = run.GetModel()

	// Validate the context can listen at startup so callers see a synchronous
	// error if something is fundamentally wrong (bad identity, no service,
	// etc.). The supervisor goroutine handles transient mid-run failures.
	context, err := self.GetZitiContext(run)
	if err != nil {
		return err
	}
	listener, err := context.Listen(service)
	if err != nil {
		return err
	}
	self.listener = listener

	go self.superviseMetricsListener(run, service, listener)
	return nil
}

// superviseMetricsListener owns the listener Accept loop and re-establishes
// the listener if it dies (e.g. SDK channel closed by a transient network
// dropout). Without this, a single Accept error left the simController
// permanently unable to receive metric pushes for the rest of the test run.
//
// Recovery uses exponential backoff (capped) on consecutive listener-create
// failures, but resets to the base delay as soon as we have a working
// listener.
func (self *SimServices) superviseMetricsListener(run model.Run, service string, initialListener net.Listener) {
	log := pfxlog.Logger().WithField("service", service)
	log.Info("ziti client metrics listener started")

	listener := initialListener
	const baseBackoff = time.Second
	const maxBackoff = 30 * time.Second
	backoff := baseBackoff

	for {
		// Accept loop for the current listener.
		for {
			conn, err := listener.Accept()
			if err != nil {
				log.WithError(err).Warn("metrics listener accept failed; will re-listen")
				_ = listener.Close()
				break
			}
			go self.HandleMetricsConn(conn)
		}

		// Re-establish. Loop until we succeed or the run is shutting down.
		for {
			time.Sleep(backoff)
			if backoff < maxBackoff {
				backoff *= 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
			}
			ctx, err := self.GetZitiContext(run)
			if err != nil {
				log.WithError(err).Warn("metrics listener: failed to get ziti context, retrying")
				continue
			}
			newListener, err := ctx.Listen(service)
			if err != nil {
				log.WithError(err).Warn("metrics listener: failed to relisten, retrying")
				continue
			}
			listener = newListener
			self.listener = newListener
			backoff = baseBackoff
			log.Info("metrics listener re-established after failure")
			break
		}
	}
}

func (self *SimServices) CollectSimMetricStage(service string) model.Stage {
	return model.StageActionF(func(run model.Run) error {
		return self.CollectSimMetrics(run, service)
	})
}

func (self *SimServices) HandleMetricsConn(conn net.Conn) {
	defer func() { _ = conn.Close() }()

	log := pfxlog.Logger()
	log.Infof("new client metrics connection established from: %v", conn.RemoteAddr().String())
	lenBuf := make([]byte, 4)
	msgBuf := make([]byte, 4*1024)
	for {
		if _, err := io.ReadFull(conn, lenBuf); err != nil {
			log.WithError(err).Info("metrics conn closed, exiting read loop")
			return
		}
		msgLen := int(binary.LittleEndian.Uint32(lenBuf))
		if msgLen > 1024*128 {
			log.Errorf("got invalid metrics message len: %v, closing connection", msgLen)
			return
		}

		if msgLen > len(msgBuf) {
			msgBuf = make([]byte, msgLen)
		}

		if _, err := io.ReadFull(conn, msgBuf[:msgLen]); err != nil {
			pfxlog.Logger().WithError(err).Info("metrics conn closed, exiting read loop")
			return
		}

		event := &mgmt_pb.StreamMetricsEvent{}
		err := proto.Unmarshal(msgBuf[:msgLen], event)
		if err != nil {
			log.WithError(err).Error("error handling metrics receive, exiting")
			return
		}

		hostSelector := self.idToSelectorMapper(event.SourceId)
		host, err := self.model.SelectHost(hostSelector)
		if err == nil {
			modelEvent := self.toClientMetricsEvent(event)
			self.model.AcceptHostMetrics(host, modelEvent)
			log.Debugf("<$= [%s] - client metrics", event.SourceId)
		} else {
			log.WithError(err).Error("clientMetrics: unable to find host")
		}
	}
}

func (self *SimServices) CloseMetricsListenerOnNotify(closeNotify <-chan struct{}) error {
	logrus.Infof("starting")
	defer logrus.Infof("exiting")

	<-closeNotify
	return self.listener.Close()
}

func (self *SimServices) toClientMetricsEvent(fabricEvent *mgmt_pb.StreamMetricsEvent) *model.MetricsEvent {
	modelEvent := &model.MetricsEvent{
		Timestamp: time.Unix(fabricEvent.Timestamp.Seconds, int64(fabricEvent.Timestamp.Nanos)),
		Metrics:   model.MetricSet{},
	}

	for name, val := range fabricEvent.IntMetrics {
		group := fabricEvent.MetricGroup[name]
		if strings.Contains(name, "xgress") {
			modelEvent.Metrics.AddGroupedMetric(group, name, float64(val))
		} else {
			modelEvent.Metrics.AddGroupedMetric(group, name, val)
		}
	}

	for name, val := range fabricEvent.FloatMetrics {
		group := fabricEvent.MetricGroup[name]
		modelEvent.Metrics.AddGroupedMetric(group, name, val)
	}

	return modelEvent
}

func (self *SimServices) GetSimController(run model.Run, service string, callback loop4.ControllerCallback) (*loop4.RemoteController, error) {
	zitiContext, err := self.GetZitiContext(run)
	if err != nil {
		return nil, err
	}

	self.lock.Lock()
	defer self.lock.Unlock()

	if self.remoteController == nil {
		simControl := loop4.NewRemoteController(zitiContext, callback)
		if err = simControl.AcceptConnections(service); err != nil {
			return nil, err
		}
		self.remoteController = simControl
	}

	return self.remoteController, nil
}
