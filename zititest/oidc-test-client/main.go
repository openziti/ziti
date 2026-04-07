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
	"runtime/debug"
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

	// destinationsFile, in proxy mode, is a JSON file listing the local proxy
	// instances this driver should rotate through. Each entry identifies the
	// target prox by ClientId and maps service names to local TCP ports.
	destinationsFile string

	// sdkDirectClientId, in sdk-direct mode, is the ClientId stamped on all
	// emitted events (since there's no rotation over destinations).
	sdkDirectClientId string

	// Results reporting via Ziti service.
	resultsIdentity string
	resultsService  string
}

// destination identifies a dial target. In proxy mode, ClientId names the
// target prox-c component and Ports maps service name to the local TCP port
// to dial. In sdk-direct mode there is a single destination whose ClientId is
// the driver's own component ID and whose Ports is nil (dial goes through the
// Ziti context rather than a local port).
type destination struct {
	ClientId string         `json:"client_id"`
	Ports    map[string]int `json:"ports,omitempty"`
}

type destinationsConfig struct {
	Destinations []destination `json:"destinations"`
}

type trafficEvent struct {
	Type      string `json:"type"`
	Service   string `json:"service"`
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
	LatencyMs int64  `json:"latency_ms,omitempty"`
	// ClientId is the component ID of the target being dialed (the prox in
	// proxy mode, or the sdk-direct client itself). The collector uses this to
	// track per-target coverage during convergence.
	ClientId string `json:"client_id"`
	// Port is the local TCP port dialed in proxy mode; 0 for sdk-direct.
	Port int `json:"port,omitempty"`
	// Timestamp marshals as RFC3339Nano JSON (Go's default for time.Time) so
	// the collector can compare events with sub-second precision.
	Timestamp time.Time `json:"ts"`
}

// reporter sends JSON events to a Ziti service connection.
type reporter struct {
	mu      sync.Mutex
	conn    net.Conn
	ctx     ziti.Context
	service string
}

func newReporter(identityFile, service string) (*reporter, error) {
	cfg, err := ziti.NewConfigFromFile(identityFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load results identity: %w", err)
	}
	ctx, err := ziti.NewContext(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create results context: %w", err)
	}

	r := &reporter{
		ctx:     ctx,
		service: service,
	}
	return r, nil
}

func (r *reporter) send(evt trafficEvent) {
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
	var destinations []destination
	var closer func()

	switch cfg.mode {
	case "sdk-direct":
		if cfg.sdkDirectClientId == "" {
			log.Fatal("sdk-direct mode requires --client-id")
		}
		d, c, err := newSdkDialer(cfg.identityFile)
		if err != nil {
			log.WithError(err).Fatal("failed to create SDK dialer")
		}
		dialer = d
		closer = c
		destinations = []destination{{ClientId: cfg.sdkDirectClientId}}
	case "proxy":
		if cfg.destinationsFile == "" {
			log.Fatal("proxy mode requires --destinations-file")
		}
		dests, err := loadDestinations(cfg.destinationsFile)
		if err != nil {
			log.WithError(err).Fatal("failed to load destinations")
		}
		if len(dests) == 0 {
			log.Fatal("destinations file contains no entries")
		}
		destinations = dests
		dialer = newProxyDialer()
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
		results, err = newReporter(resultsIdentity, cfg.resultsService)
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

	rotate := newDestinationRotator(destinations)

	// runLoop invokes fn inside a panic-recovering wrapper and, if fn panics,
	// logs the failure via pfxlog (which goes to --log-file, not stderr) and
	// re-runs it. Without this, a panic in any goroutine kills the process
	// silently because the launcher discards stderr.
	runLoop := func(name string, fn func()) {
		for {
			select {
			case <-ctx:
				return
			default:
			}
			func() {
				defer func() {
					if r := recover(); r != nil {
						pfxlog.Logger().Errorf("%s panicked: %v\n%s", name, r, debug.Stack())
					}
				}()
				fn()
			}()
			// fn returned or panicked; brief pause before restarting so we don't
			// tight-loop if the failure is deterministic.
			select {
			case <-ctx:
				return
			case <-time.After(time.Second):
			}
		}
	}

	var wg sync.WaitGroup

	if len(cfg.services) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runLoop("heartbeat", func() {
				runHeartbeat(ctx, dialer, rotate, cfg.services, cfg.heartbeatInterval, emit)
			})
		}()
	}

	if len(cfg.services) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runLoop("dials", func() {
				runDials(ctx, dialer, rotate, cfg.services, cfg.dialInterval, emit)
			})
		}()
	}

	wg.Wait()
	log.Info("oidc-test-client shutting down")
}

// dialFunc establishes one connection for service to the given destination.
// Returns the established conn, the local TCP port dialed (0 for sdk-direct),
// and any error.
type dialFunc func(service string, dest destination) (conn net.Conn, port int, err error)
type emitFunc func(trafficEvent)

func loadDestinations(path string) ([]destination, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading destinations file %s: %w", path, err)
	}
	var cfg destinationsConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing destinations file %s: %w", path, err)
	}
	return cfg.Destinations, nil
}

func newDestinationRotator(dests []destination) func() destination {
	var counter int
	var mu sync.Mutex
	return func() destination {
		mu.Lock()
		d := dests[counter%len(dests)]
		counter++
		mu.Unlock()
		return d
	}
}

func newSdkDialer(identityFile string) (dialFunc, func(), error) {
	zitiCfg, err := ziti.NewConfigFromFile(identityFile)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load identity config: %w", err)
	}

	zitiCtx, err := ziti.NewContext(zitiCfg)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create ziti context: %w", err)
	}

	dial := func(service string, _ destination) (net.Conn, int, error) {
		conn, err := zitiCtx.Dial(service)
		return conn, 0, err
	}

	return dial, func() { zitiCtx.Close() }, nil
}

func newProxyDialer() dialFunc {
	return func(service string, dest destination) (net.Conn, int, error) {
		port, ok := dest.Ports[service]
		if !ok {
			return nil, 0, fmt.Errorf("destination %s has no port for service %q", dest.ClientId, service)
		}
		addr := fmt.Sprintf("127.0.0.1:%d", port)
		conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
		return conn, port, err
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

func runHeartbeat(done <-chan struct{}, dial dialFunc, rotate func() destination, services []string, interval time.Duration, emit emitFunc) {
	log := pfxlog.Logger().WithField("pattern", "heartbeat")
	payload := []byte("heartbeat\n")

	for {
		select {
		case <-done:
			return
		default:
		}

		svc := services[rand.Intn(len(services))]
		dest := rotate()
		conn, port, err := dial(svc, dest)
		if err != nil {
			emit(trafficEvent{Type: "heartbeat", Service: svc, Status: "error", Error: err.Error(),
				ClientId: dest.ClientId, Port: port, Timestamp: now()})
			log.WithError(err).Warnf("heartbeat dial failed to %s, retrying in 5s", dest.ClientId)
			sleepOrDone(done, 5*time.Second)
			continue
		}

		log.Infof("heartbeat connection established to %s via %s (port=%d)", svc, dest.ClientId, port)

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
					ClientId: dest.ClientId, Port: port, Timestamp: now()})
				conn.Close()
				break
			}

			_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
			buf := make([]byte, 256)
			if _, err = conn.Read(buf); err != nil {
				emit(trafficEvent{Type: "heartbeat", Service: svc, Status: "error", Error: err.Error(),
					ClientId: dest.ClientId, Port: port, Timestamp: now()})
				conn.Close()
				break
			}

			emit(trafficEvent{Type: "heartbeat", Service: svc, Status: "ok",
				ClientId: dest.ClientId, Port: port, Timestamp: now()})
			sleepOrDone(done, interval)
		}

		sleepOrDone(done, 2*time.Second)
	}
}

func runDials(done <-chan struct{}, dial dialFunc, rotate func() destination, services []string, interval time.Duration, emit emitFunc) {
	log := pfxlog.Logger().WithField("pattern", "dial")
	for {
		select {
		case <-done:
			return
		default:
		}

		svc := services[rand.Intn(len(services))]
		dest := rotate()
		start := time.Now()

		conn, port, err := dial(svc, dest)
		if err != nil {
			log.WithError(err).Warnf("dial failed to %s (svc=%s)", dest.ClientId, svc)
			emit(trafficEvent{Type: "dial", Service: svc, Status: "error", Error: err.Error(),
				ClientId: dest.ClientId, Port: port, Timestamp: now()})
			sleepOrDone(done, interval)
			continue
		}

		payload := []byte("echo-test\n")
		_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		_, err = conn.Write(payload)
		if err != nil {
			log.WithError(err).Warnf("dial write failed to %s (svc=%s)", dest.ClientId, svc)
			emit(trafficEvent{Type: "dial", Service: svc, Status: "error", Error: err.Error(),
				ClientId: dest.ClientId, Port: port, Timestamp: now()})
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
			log.WithError(err).Warnf("dial read failed to %s (svc=%s, latency=%dms)", dest.ClientId, svc, latencyMs)
			emit(trafficEvent{Type: "dial", Service: svc, Status: "error", Error: err.Error(),
				LatencyMs: latencyMs, ClientId: dest.ClientId, Port: port, Timestamp: now()})
		} else {
			emit(trafficEvent{Type: "dial", Service: svc, Status: "ok",
				LatencyMs: latencyMs, ClientId: dest.ClientId, Port: port, Timestamp: now()})
		}

		sleepOrDone(done, interval)
	}
}

func now() time.Time {
	return time.Now().UTC()
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

	flag.StringVar(&cfg.destinationsFile, "destinations-file", "", "Path to JSON file listing proxy destinations (proxy mode)")
	flag.StringVar(&cfg.sdkDirectClientId, "client-id", "", "Driver identifier for events in sdk-direct mode")

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
