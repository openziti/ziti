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

package loop4

import (
	"errors"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/agent"
	"github.com/openziti/foundation/v2/util"
	"github.com/openziti/identity"
	"github.com/openziti/identity/dotziti"
	"github.com/openziti/metrics"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/transport/v2"
	"github.com/openziti/ziti/common/outputz"
	trafficMetrics "github.com/openziti/ziti/zititest/ziti-traffic-test/metrics"
	"github.com/openziti/ziti/zititest/ziti-traffic-test/subcmd"
	"github.com/spf13/cobra"
	"io"
	"net"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

func init() {
	subcmd.Root.AddCommand(loop4Cmd)
}

var loop4Cmd = &cobra.Command{
	Use:   "loop4",
	Short: "Loop testing tool, v4",
}

func NewSim() *Sim {
	return &Sim{
		sdkClients:  make(map[string]ziti.Context),
		dialers:     make(map[string]func(workload *Workload) (net.Conn, error)),
		listeners:   make(map[string]func(workload *Workload) (net.Listener, error)),
		closeNotify: make(chan struct{}),
	}
}

type Sim struct {
	sdkClients           map[string]ziti.Context
	dialers              map[string]func(workload *Workload) (net.Conn, error)
	listeners            map[string]func(workload *Workload) (net.Listener, error)
	sdkMetricsStartHooks []func(ctx ziti.Context) error
	closeNotify          chan struct{}
	scenario             *Scenario
	metrics              metrics.Registry
}

func (sim *Sim) InitScenario(path string) error {
	log := pfxlog.Logger()

	shutdownClean := false
	if err := agent.Listen(agent.Options{ShutdownCleanup: &shutdownClean}); err != nil {
		log.WithError(err).Error("unable to start CLI agent")
	}

	var err error

	sim.scenario, err = LoadScenario(path)
	if err != nil {
		return err
	}

	log.Debug(sim.scenario)

	if err = sim.InitConnectors(); err != nil {
		return err
	}

	if err = sim.StartMetrics(); err != nil {
		return err
	}

	return nil
}

func (sim *Sim) InitConnectors() error {
	log := pfxlog.Logger()
	log.Infof("loading %d connectors", len(sim.scenario.ConnectorConfigs))
	for name, connector := range sim.scenario.ConnectorConfigs {
		if connector.SdkOptions != nil {
			log.Infof("loading sdk connector '%s'", name)
			cfg, err := ziti.NewConfigFromFile(connector.SdkOptions.IdentityFile)
			if err != nil {
				return fmt.Errorf("for connector '%s', unable to load ziti identity config file '%s' (%w)",
					name, connector.SdkOptions.IdentityFile, err)
			}

			if !connector.SdkOptions.DisableMultiChannel {
				var cfgI any = cfg
				if i, ok := cfgI.(interface {
					SetMaxControlConnections(val uint32)
					SetMaxDefaultConnections(val uint32)
				}); ok {
					i.SetMaxControlConnections(1)
					i.SetMaxDefaultConnections(2)
				}
			}

			ctx, err := ziti.NewContext(cfg)
			if err != nil {
				return fmt.Errorf("for connector '%s', unable to create ziti sdk context for identity file '%s' (%w)",
					name, connector.SdkOptions.IdentityFile, err)
			}

			if connector.SdkOptions.TestService != "" {
				if conn, err := ctx.Dial(connector.SdkOptions.TestService); err != nil {
					return err
				} else {
					_ = conn.Close()
				}
			}

			sim.sdkClients[name] = ctx

			if connector.SdkOptions.ReportSdkMetrics {
				sim.sdkMetricsStartHooks = append(sim.sdkMetricsStartHooks, func(metricsCtx ziti.Context) error {
					return sim.StartSdkMetricsReporter(ctx.Metrics(), metricsCtx)
				})
			}

			sim.dialers[name] = func(workload *Workload) (net.Conn, error) {
				return sim.DialSdk(ctx, workload)
			}

			sim.listeners[name] = func(workload *Workload) (net.Listener, error) {
				return sim.ListenSdk(ctx, workload)
			}
		} else if connector.TransportOptions != nil {
			log.Infof("loading transport connector '%s'", name)
			addr, err := transport.ParseAddress(connector.TransportOptions.Address)
			if err != nil {
				return fmt.Errorf("for connector '%s', unable to parse transport address '%s' (%w)",
					name, connector.TransportOptions.Address, err)
			}

			id := &identity.TokenId{Token: "test"}
			if addr.Type() != "tcp" {
				if _, id, err = dotziti.LoadIdentity(connector.TransportOptions.IdentityFile); err != nil {
					return fmt.Errorf("for connector '%s', unable to load identity file '%s' (%w)",
						name, connector.TransportOptions.IdentityFile, err)
				}
			}

			sim.dialers[name] = func(workload *Workload) (net.Conn, error) {
				return sim.DialTransport(addr, id, workload)
			}

			sim.listeners[name] = func(workload *Workload) (net.Listener, error) {
				log.Infof("starting transport listener '%s' on '%s'", name, addr)
				return sim.ListenTransport(addr, id, workload)
			}
		} else {
			return fmt.Errorf("connector '%s' has no configuration for sdk or transport", name)
		}
	}
	return nil
}

func (sim *Sim) DialSdk(client ziti.Context, wf *Workload) (net.Conn, error) {
	dialOptions := &ziti.DialOptions{
		ConnectTimeout: wf.ConnectTimeout,
		SdkFlowControl: util.Ptr(true),
	}
	return client.DialWithOptions(wf.ServiceName, dialOptions)
}

func (sim *Sim) ListenSdk(client ziti.Context, wf *Workload) (net.Listener, error) {
	listenOptions := ziti.DefaultListenOptions()
	listenOptions.SdkFlowControl = util.Ptr(true)
	return client.ListenWithOptions(wf.ServiceName, listenOptions)
}

func (sim *Sim) DialTransport(addr transport.Address, id *identity.TokenId, wf *Workload) (net.Conn, error) {
	return addr.Dial("name", id, wf.ConnectTimeout, nil)
}

func (sim *Sim) ListenTransport(addr transport.Address, id *identity.TokenId, wf *Workload) (net.Listener, error) {
	adapter := &ListenerAdapter{
		addr:        addr,
		connC:       make(chan net.Conn, 1),
		closeNotify: make(chan struct{}),
	}

	closer, err := addr.Listen(wf.Name, id, adapter.connReceived, nil)
	if err != nil {
		return nil, err
	}
	adapter.closer = closer

	return adapter, nil
}

func (sim *Sim) StartMetrics() error {
	if sim.scenario.Metrics != nil {
		if sim.scenario.Metrics.Connector == "" {
			return errors.New("no sdk connector defined for metrics")
		}

		sim.metrics = metrics.NewRegistry(sim.scenario.Metrics.ClientId, nil)

		client, ok := sim.sdkClients[sim.scenario.Metrics.Connector]
		if !ok {
			return fmt.Errorf("sdk connector selected for metrics '%s' does not defined", sim.scenario.Metrics.Connector)
		}

		if err := sim.StartSdkMetricsReporter(sim.metrics, client); err != nil {
			return err
		}

		for _, initHook := range sim.sdkMetricsStartHooks {
			if err := initHook(client); err != nil {
				return err
			}
		}

	} else {
		sim.metrics = metrics.NewRegistry("no-op", nil)

		sim.StartStdoutMetricsReporter(sim.metrics)
		for _, ctx := range sim.sdkClients {
			sim.StartStdoutMetricsReporter(ctx.Metrics())
		}
	}

	return nil
}

func (sim *Sim) StartSdkMetricsReporter(registry metrics.Registry, ctx ziti.Context) error {
	if sim.scenario.Metrics.ReportInterval == 0 {
		return errors.New("metrics report interval must be greater than 0")
	}

	cfg := &trafficMetrics.ZitiReporterConfig{
		Registry:    registry,
		Client:      ctx,
		ClientId:    sim.scenario.Metrics.ClientId,
		CloseNotify: sim.closeNotify,
		ServiceName: sim.scenario.Metrics.Service,
	}

	reporter, err := trafficMetrics.NewZitiReporter(cfg)
	if err != nil {
		return err
	}

	go reporter.Run(sim.scenario.Metrics.ReportInterval)

	return nil
}

func (sim *Sim) StartStdoutMetricsReporter(registry metrics.Registry) {
	reporter := metrics.NewDelegatingReporter(registry, &metricsEmitter{}, sim.closeNotify)
	go reporter.Start(15 * time.Second)
}

type metricsEmitter struct {
	msgs []string
}

func (em *metricsEmitter) Filter(name string) bool {
	return strings.Contains(name, metrics.MetricNameCount) ||
		strings.Contains(name, metrics.MetricNameRateM1) ||
		strings.Contains(name, metrics.MetricNameMean) ||
		strings.Contains(name, metrics.MetricNamePercentile)
}

func (em *metricsEmitter) StartReport(metrics.Registry) {}

func (em *metricsEmitter) EndReport(metrics.Registry) {
	sort.Strings(em.msgs)
	for _, msg := range em.msgs {
		fmt.Println(msg)
	}
	em.msgs = nil
}

func (em *metricsEmitter) AcceptIntMetric(name string, value int64) {
	em.Printf("%s: %v", name, value)
}

func (em *metricsEmitter) AcceptFloatMetric(name string, value float64) {
	if strings.HasSuffix(name, metrics.MetricNameRateM1) && strings.Contains(name, "bytes") {
		em.Printf("%s: %v", name, outputz.FormatBytes(uint64(value)))
	}
	em.Printf("%s: %v", name, value)
}

func (em *metricsEmitter) AcceptPercentileMetric(name string, value metrics.PercentileSource) {
	em.Printf("%s.p95: %v", name, time.Duration(value.Percentile(0.95)))
}

func (em *metricsEmitter) Printf(msg string, args ...interface{}) {
	em.msgs = append(em.msgs, fmt.Sprintf(msg, args...))
}

type ListenerAdapter struct {
	addr        transport.Address
	connC       chan net.Conn
	closer      io.Closer
	closeNotify chan struct{}
	closed      atomic.Bool
}

func (self *ListenerAdapter) Network() string {
	return "tcp"
}

func (self *ListenerAdapter) String() string {
	return self.addr.String()
}

func (self *ListenerAdapter) connReceived(conn transport.Conn) {
	select {
	case self.connC <- conn:
	case <-self.closeNotify:
		return
	}
}

func (self *ListenerAdapter) Accept() (net.Conn, error) {
	select {
	case conn := <-self.connC:
		return conn, nil
	case <-self.closeNotify:
		return nil, io.EOF
	}
}

func (self *ListenerAdapter) Close() error {
	if self.closed.CompareAndSwap(false, true) {
		close(self.closeNotify)
		return self.closer.Close()
	}
	return nil
}

func (self *ListenerAdapter) Addr() net.Addr {
	return self
}
