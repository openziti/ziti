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

// oidc-test-client is a traffic driver for the oidc-auth-test fablab model.
//
// Two connection modes:
//   - sdk-direct: Uses the Go SDK to authenticate via OIDC and dial services.
//   - proxy: Rotates through local ziti-prox-c proxy ports.
//
// Two concurrent traffic patterns:
//   - Long-lived heartbeat connection (tests circuit survival across token refresh)
//   - Periodic short-lived dials (tests new circuit establishment)
//
// Reports structured JSON events to a "traffic-results" Ziti service for
// collection by the test runner's TrafficResultsCollector.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/sirupsen/logrus"
)

type config struct {
	mode              string
	identityFile      string
	services          []string
	dialInterval      time.Duration
	heartbeatInterval time.Duration
	logFile           string

	// Proxy mode: port range for rotating through prox-c instances.
	proxyBasePort      int
	proxyInstanceCount int

	// Results reporting via Ziti service.
	resultsIdentity string
	resultsService  string
}

type trafficEvent struct {
	Type       string `json:"type"`
	Service    string `json:"service"`
	Status     string `json:"status"`
	Error      string `json:"error,omitempty"`
	LatencyMs  int64  `json:"latency_ms,omitempty"`
	ProxyPort  int    `json:"proxy_port,omitempty"`
	ProxyIndex int    `json:"proxy_index,omitempty"`
	ClientId   string `json:"client_id"`
	Timestamp  string `json:"ts"`
}

// reporter sends JSON events to a Ziti service connection.
type reporter struct {
	mu       sync.Mutex
	conn     net.Conn
	ctx      ziti.Context
	service  string
	clientId string
}

func newReporter(identityFile, service, clientId string) (*reporter, error) {
	cfg, err := ziti.NewConfigFromFile(identityFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load results identity: %w", err)
	}
	ctx, err := ziti.NewContext(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create results context: %w", err)
	}

	r := &reporter{
		ctx:      ctx,
		service:  service,
		clientId: clientId,
	}
	return r, nil
}

func (r *reporter) send(evt trafficEvent) {
	evt.ClientId = r.clientId
	data, _ := json.Marshal(evt)
	data = append(data, '\n')

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.conn == nil {
		conn, err := r.ctx.Dial(r.service)
		if err != nil {
			pfxlog.Logger().WithError(err).Debug("failed to dial results service")
			return
		}
		r.conn = conn
	}

	if _, err := r.conn.Write(data); err != nil {
		pfxlog.Logger().WithError(err).Debug("failed to write to results service, reconnecting")
		r.conn.Close()
		r.conn = nil
	}
}

func (r *reporter) close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.conn != nil {
		r.conn.Close()
	}
	r.ctx.Close()
}

func main() {
	cfg := parseFlags()
	setupLogging(cfg.logFile)

	log := pfxlog.Logger()
	log.Infof("starting oidc-test-client mode=%s services=%v", cfg.mode, cfg.services)

	var dialer dialFunc
	var closer func()

	switch cfg.mode {
	case "sdk-direct":
		d, c, err := newSdkDialer(cfg.identityFile)
		if err != nil {
			log.WithError(err).Fatal("failed to create SDK dialer")
		}
		dialer = d
		closer = c
	case "proxy":
		if cfg.proxyInstanceCount <= 0 || cfg.proxyBasePort <= 0 {
			log.Fatal("proxy mode requires --proxy-base-port and --proxy-instance-count")
		}
		dialer = newProxyRotatingDialer(cfg)
		closer = func() {}
	default:
		log.Fatalf("unknown mode: %s", cfg.mode)
	}

	// Set up results reporter. In sdk-direct mode, use the same identity for
	// reporting unless a separate results identity is provided.
	resultsIdentity := cfg.resultsIdentity
	if resultsIdentity == "" && cfg.mode == "sdk-direct" {
		resultsIdentity = cfg.identityFile
	}

	var results *reporter
	if resultsIdentity != "" && cfg.resultsService != "" {
		var err error
		results, err = newReporter(resultsIdentity, cfg.resultsService, cfg.identityFile)
		if err != nil {
			log.WithError(err).Warn("failed to create results reporter, events will only go to log")
		} else {
			defer results.close()
		}
	}

	ctx, cancel := signalContext()
	defer cancel()
	defer closer()

	emit := func(evt trafficEvent) {
		// Always log locally.
		data, _ := json.Marshal(evt)
		fmt.Println(string(data))
		// Also send to collector if available.
		if results != nil {
			results.send(evt)
		}
	}

	var wg sync.WaitGroup

	if len(cfg.services) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runHeartbeat(ctx, dialer, cfg.services, cfg.heartbeatInterval, emit)
		}()
	}

	if len(cfg.services) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runDials(ctx, dialer, cfg.services, cfg.dialInterval, emit)
		}()
	}

	wg.Wait()
	log.Info("oidc-test-client shutting down")
}

type dialFunc func(service string) (conn net.Conn, proxyPort int, proxyIndex int, err error)
type emitFunc func(trafficEvent)

func newSdkDialer(identityFile string) (dialFunc, func(), error) {
	zitiCfg, err := ziti.NewConfigFromFile(identityFile)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load identity config: %w", err)
	}

	zitiCtx, err := ziti.NewContext(zitiCfg)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create ziti context: %w", err)
	}

	dial := func(service string) (net.Conn, int, int, error) {
		conn, err := zitiCtx.Dial(service)
		return conn, 0, 0, err
	}

	return dial, func() { zitiCtx.Close() }, nil
}

func newProxyRotatingDialer(cfg config) dialFunc {
	var counter int
	var mu sync.Mutex
	numServices := len(cfg.services)

	svcIndex := map[string]int{}
	for i, svc := range cfg.services {
		svcIndex[svc] = i
	}

	return func(service string) (net.Conn, int, int, error) {
		sIdx, ok := svcIndex[service]
		if !ok {
			return nil, 0, 0, fmt.Errorf("unknown service %q for proxy dial", service)
		}

		mu.Lock()
		instance := counter % cfg.proxyInstanceCount
		counter++
		mu.Unlock()

		port := cfg.proxyBasePort + instance*numServices + sIdx
		addr := fmt.Sprintf("127.0.0.1:%d", port)

		conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
		return conn, port, instance, err
	}
}

func signalContext() (<-chan struct{}, func()) {
	done := make(chan struct{})
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)
	var once sync.Once
	go func() {
		<-sigs
		once.Do(func() { close(done) })
	}()
	return done, func() { once.Do(func() { close(done) }) }
}

func runHeartbeat(done <-chan struct{}, dial dialFunc, services []string, interval time.Duration, emit emitFunc) {
	log := pfxlog.Logger().WithField("pattern", "heartbeat")
	svc := services[rand.Intn(len(services))]
	payload := []byte("heartbeat\n")

	for {
		select {
		case <-done:
			return
		default:
		}

		conn, proxyPort, proxyIdx, err := dial(svc)
		if err != nil {
			emit(trafficEvent{Type: "heartbeat", Service: svc, Status: "error", Error: err.Error(),
				ProxyPort: proxyPort, ProxyIndex: proxyIdx, Timestamp: now()})
			log.WithError(err).Warn("heartbeat dial failed, retrying in 5s")
			sleepOrDone(done, 5*time.Second)
			continue
		}

		log.Infof("heartbeat connection established to %s (proxy_port=%d)", svc, proxyPort)

		for {
			select {
			case <-done:
				conn.Close()
				return
			default:
			}

			_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			_, err := conn.Write(payload)
			if err != nil {
				emit(trafficEvent{Type: "heartbeat", Service: svc, Status: "error", Error: err.Error(),
					ProxyPort: proxyPort, ProxyIndex: proxyIdx, Timestamp: now()})
				conn.Close()
				break
			}

			_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
			buf := make([]byte, 256)
			if _, err = conn.Read(buf); err != nil {
				emit(trafficEvent{Type: "heartbeat", Service: svc, Status: "error", Error: err.Error(),
					ProxyPort: proxyPort, ProxyIndex: proxyIdx, Timestamp: now()})
				conn.Close()
				break
			}

			emit(trafficEvent{Type: "heartbeat", Service: svc, Status: "ok",
				ProxyPort: proxyPort, ProxyIndex: proxyIdx, Timestamp: now()})
			sleepOrDone(done, interval)
		}

		sleepOrDone(done, 2*time.Second)
	}
}

func runDials(done <-chan struct{}, dial dialFunc, services []string, interval time.Duration, emit emitFunc) {
	for {
		select {
		case <-done:
			return
		default:
		}

		svc := services[rand.Intn(len(services))]
		start := time.Now()

		conn, proxyPort, proxyIdx, err := dial(svc)
		if err != nil {
			emit(trafficEvent{Type: "dial", Service: svc, Status: "error", Error: err.Error(),
				ProxyPort: proxyPort, ProxyIndex: proxyIdx, Timestamp: now()})
			sleepOrDone(done, interval)
			continue
		}

		payload := []byte("echo-test\n")
		_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		_, err = conn.Write(payload)
		if err != nil {
			emit(trafficEvent{Type: "dial", Service: svc, Status: "error", Error: err.Error(),
				ProxyPort: proxyPort, ProxyIndex: proxyIdx, Timestamp: now()})
			conn.Close()
			sleepOrDone(done, interval)
			continue
		}

		_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		buf := make([]byte, 256)
		_, err = io.ReadAtLeast(conn, buf, 1)
		conn.Close()

		latencyMs := time.Since(start).Milliseconds()
		if err != nil {
			emit(trafficEvent{Type: "dial", Service: svc, Status: "error", Error: err.Error(),
				LatencyMs: latencyMs, ProxyPort: proxyPort, ProxyIndex: proxyIdx, Timestamp: now()})
		} else {
			emit(trafficEvent{Type: "dial", Service: svc, Status: "ok",
				LatencyMs: latencyMs, ProxyPort: proxyPort, ProxyIndex: proxyIdx, Timestamp: now()})
		}

		sleepOrDone(done, interval)
	}
}

func now() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func sleepOrDone(done <-chan struct{}, d time.Duration) {
	select {
	case <-done:
	case <-time.After(d):
	}
}

func parseFlags() config {
	var cfg config
	var serviceList string

	flag.StringVar(&cfg.mode, "mode", "sdk-direct", "Connection mode: sdk-direct or proxy")
	flag.StringVar(&cfg.identityFile, "identity", "", "Path to Ziti identity config file (sdk-direct mode)")
	flag.StringVar(&serviceList, "services", "", "Comma-separated list of service names to test")
	flag.DurationVar(&cfg.dialInterval, "dial-interval", 30*time.Second, "Interval between short-lived dials")
	flag.DurationVar(&cfg.heartbeatInterval, "heartbeat-interval", 5*time.Second, "Interval between heartbeats")
	flag.StringVar(&cfg.logFile, "log-file", "", "Path to log file (stderr if empty)")

	flag.IntVar(&cfg.proxyBasePort, "proxy-base-port", 10000, "Base port for proxy instance rotation")
	flag.IntVar(&cfg.proxyInstanceCount, "proxy-instance-count", 0, "Number of proxy instances to rotate through")

	flag.StringVar(&cfg.resultsIdentity, "results-identity", "", "Ziti identity for reporting results (proxy mode)")
	flag.StringVar(&cfg.resultsService, "results-service", "traffic-results", "Ziti service name for results reporting")

	flag.Parse()

	if serviceList != "" {
		cfg.services = strings.Split(serviceList, ",")
	}

	return cfg
}

func setupLogging(logFile string) {
	logrus.SetFormatter(&logrus.JSONFormatter{TimestampFormat: time.RFC3339})

	if logFile != "" {
		f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			logrus.WithError(err).Fatal("failed to open log file")
		}
		logrus.SetOutput(f)
	}
}
