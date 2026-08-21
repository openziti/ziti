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
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/sdk-golang/v2/ziti"
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
		connectTimeout:     60 * time.Second,
	}
}

type SimServices struct {
	listener           net.Listener
	model              *model.Model
	idToSelectorMapper func(string) string
	lock               sync.Mutex
	zitiContext        ziti.Context
	metricsStarted     atomic.Bool
	// connectTimeout bounds establishing the sim-controller ziti context, so an unreachable
	// controller fails fast instead of hanging on the SDK's unbounded version-info retry.
	connectTimeout time.Duration

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

		// Authenticate up front with a deadline so an unreachable controller fails fast. The SDK's
		// version-info caching retries forever with no timeout (openziti/sdk-golang#976), so the first
		// Listen/Dial would otherwise block indefinitely and hang the caller (e.g. the steady-state gate).
		if err = authenticateWithTimeout(context, self.connectTimeout); err != nil {
			context.Close()
			return nil, err
		}

		self.zitiContext = context
	}

	return self.zitiContext, nil
}

// authenticateWithTimeout runs context.Authenticate() with a deadline so an unreachable controller
// surfaces an error instead of blocking indefinitely on the SDK's unbounded version-info retry. If
// Authenticate is wedged in that retry, its goroutine can outlive the timeout; that is acceptable
// here since the run is failing anyway.
func authenticateWithTimeout(context ziti.Context, timeout time.Duration) error {
	done := make(chan error, 1)
	go func() { done <- context.Authenticate() }()
	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		return fmt.Errorf("timed out after %s establishing sim controller connection; controller unreachable?", timeout)
	}
}

func (self *SimServices) CollectSimMetrics(run model.Run, service string) error {
	if !self.metricsStarted.CompareAndSwap(false, true) {
		return nil
	}

	self.model = run.GetModel()

	context, err := self.GetZitiContext(run)
	if err != nil {
		return err
	}

	listener, err := context.Listen(service)
	if err != nil {
		return err
	}

	self.listener = listener

	go func() {
		pfxlog.Logger().Info("ziti client metrics listener started")
		for {
			conn, err := listener.Accept()
			if err != nil {
				pfxlog.Logger().WithError(err).Info("metrics listener closed, returning")
				return
			}
			go self.HandleMetricsConn(conn)
		}
	}()

	return nil
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

// Reset tears down the cached sim-controller context, remote controller, and metrics listener so a
// subsequent GetZitiContext/GetSimController/CollectSimMetrics rebuilds them. Use it after the
// controller has been re-bootstrapped (e.g. resetting a test between iterations), where the cached
// api session and sim-controller enrollment are stale and would otherwise be reused.
func (self *SimServices) Reset() {
	self.lock.Lock()
	defer self.lock.Unlock()

	if self.remoteController != nil {
		_ = self.remoteController.Close()
		self.remoteController = nil
	}
	if self.listener != nil {
		_ = self.listener.Close()
		self.listener = nil
	}
	if self.zitiContext != nil {
		self.zitiContext.Close()
		self.zitiContext = nil
	}
	self.metricsStarted.Store(false)
}
