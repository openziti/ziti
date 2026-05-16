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

// event-forwarder tails a controller's event.log and forwards matching JSON
// lines to a configurable destination (stdout or a Ziti service). Which event
// namespaces to forward is controlled by the config file.
package main

import (
	"flag"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

// Config controls what the event forwarder tails, which events it forwards,
// and where it sends them.
//
// Example config:
//
//	eventLog: /home/ubuntu/logs/event.log
//	identity: /home/ubuntu/fablab/cfg/event-fwd-1.json
//	destination: ziti:oidc-events
//	namespaces:
//	  - apiSession
type Config struct {
	// EventLog is the path to the controller's event log file.
	EventLog string `yaml:"eventLog"`

	// Identity is the path to the Ziti identity config file. Required when
	// destination is a Ziti service.
	Identity string `yaml:"identity"`

	// Destination is where to send matching events. Use "stdout" to echo to
	// stdout, or "ziti:<service>" to forward over a Ziti service.
	Destination string `yaml:"destination"`

	// Namespaces is the list of event namespaces to forward. An event matches
	// if its "namespace" JSON field equals any entry in this list. If empty,
	// all events are forwarded.
	Namespaces []string `yaml:"namespaces"`
}

// forwarder sends lines to a destination. Implementations handle connection
// lifecycle internally, so the caller just calls forward() for each line.
type forwarder interface {
	forward(line string) error
	close()
}

func main() {
	var configFile string

	flag.StringVar(&configFile, "config", "", "Path to YAML config file")
	flag.Parse()

	logrus.SetFormatter(&logrus.JSONFormatter{TimestampFormat: time.RFC3339})
	log := pfxlog.Logger()

	if configFile == "" {
		log.Fatal("--config is required")
	}

	data, err := os.ReadFile(configFile)
	if err != nil {
		log.WithError(err).Fatal("failed to read config file")
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		log.WithError(err).Fatal("failed to parse config file")
	}

	if cfg.EventLog == "" {
		log.Fatal("eventLog is required in config")
	}
	if cfg.Destination == "" {
		log.Fatal("destination is required in config")
	}

	// Build the namespace matcher.
	match := buildMatcher(cfg.Namespaces)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)

	tailer := newFileTailer(cfg.EventLog)
	if err := tailer.start(); err != nil {
		log.WithError(err).Fatal("failed to start tailing event log")
	}

	var fwd forwarder

	if cfg.Destination == "stdout" {
		fwd = &stdoutForwarder{}
	} else {
		serviceName, ok := strings.CutPrefix(cfg.Destination, "ziti:")
		if !ok || serviceName == "" {
			log.Fatalf("invalid destination %q: use \"stdout\" or \"ziti:<service>\"", cfg.Destination)
		}
		if cfg.Identity == "" {
			log.Fatal("identity is required in config when destination is a Ziti service")
		}

		zitiCfg, err := ziti.NewConfigFromFile(cfg.Identity)
		if err != nil {
			log.WithError(err).Fatal("failed to load identity config")
		}
		ctx, err := ziti.NewContext(zitiCfg)
		if err != nil {
			log.WithError(err).Fatal("failed to create ziti context")
		}
		defer ctx.Close()

		zf := &zitiForwarder{
			ctx:     ctx,
			service: serviceName,
			done:    make(chan struct{}),
		}
		// Connect eagerly so the collector sees us before any events arrive.
		zf.mu.Lock()
		if err := zf.ensureConnected(); err != nil {
			log.WithError(err).Fatal("failed to establish initial connection")
		}
		zf.mu.Unlock()
		fwd = zf
	}
	defer fwd.close()

	log.Infof("forwarding events to %s (namespaces: %v)", cfg.Destination, cfg.Namespaces)
	runForwardLoop(tailer, match, fwd, sigs)
}

// runForwardLoop reads matching lines from the tailer and sends them through
// the forwarder until a signal is received.
func runForwardLoop(tailer *fileTailer, match func(string) bool, fwd forwarder, stop <-chan os.Signal) {
	log := pfxlog.Logger()
	var forwarded int
	statsTicker := time.NewTicker(time.Minute)
	defer statsTicker.Stop()

	for {
		select {
		case <-stop:
			log.Info("shutting down")
			tailer.stop()
			return
		case <-statsTicker.C:
			if forwarded > 0 {
				log.Infof("forwarded %d events in the last minute", forwarded)
				forwarded = 0
			}
		case line, ok := <-tailer.Lines:
			if !ok {
				log.Warn("tailer channel closed")
				return
			}
			if !match(line) {
				continue
			}
			if err := fwd.forward(line); err != nil {
				log.WithError(err).Error("forwarding failed permanently")
				return
			}
			forwarded++
		}
	}
}

// stdoutForwarder prints lines to stdout.
type stdoutForwarder struct{}

func (f *stdoutForwarder) forward(line string) error {
	fmt.Println(line)
	return nil
}

func (f *stdoutForwarder) close() {}

// errShutdown is returned by forward when the forwarder is closed.
var errShutdown = errors.New("forwarder shut down")

// zitiForwarder sends lines over a Ziti service connection, managing the
// connection lifecycle internally. If a write fails, it reconnects and
// retries. A keepalive detects dead connections between events.
type zitiForwarder struct {
	ctx       ziti.Context
	service   string
	mu        sync.Mutex
	conn      net.Conn
	done      chan struct{}
	lastWrite concurrenz.AtomicValue[time.Time]
}

func (f *zitiForwarder) close() {
	close(f.done)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.conn != nil {
		f.conn.Close()
		f.conn = nil
	}
}

func (f *zitiForwarder) ensureConnected() error {
	// Caller must hold f.mu.
	if f.conn != nil {
		return nil
	}

	log := pfxlog.Logger()
	for {
		// Unlock while dialing so the keepalive goroutine from a previous
		// connection can finish and release resources.
		f.mu.Unlock()
		conn, err := f.ctx.Dial(f.service)
		f.mu.Lock()

		if err != nil {
			log.WithError(err).Warn("failed to dial service, retrying in 5s")
			f.mu.Unlock()
			select {
			case <-f.done:
				f.mu.Lock()
				return errShutdown
			case <-time.After(5 * time.Second):
			}
			f.mu.Lock()
			continue
		}
		f.conn = conn
		log.Infof("connected to %s", f.service)

		// Start keepalive in the background to detect dead connections.
		go f.runKeepalive(conn)
		return nil
	}
}

func (f *zitiForwarder) runKeepalive(conn net.Conn) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		if time.Since(f.lastWrite.Load()) < 5*time.Second {
			continue
		}

		f.mu.Lock()
		if f.conn != conn {
			f.mu.Unlock()
			return // connection was replaced
		}
		_, err := conn.Write([]byte("\n"))
		f.mu.Unlock()

		if err != nil {
			pfxlog.Logger().WithError(err).Warn("keepalive failed, closing connection")
			f.resetConn(conn)
			return
		}
	}
}

func (f *zitiForwarder) resetConn(conn net.Conn) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.conn == conn {
		f.conn = nil
		conn.Close()
	}
}

func (f *zitiForwarder) forward(line string) error {
	data := []byte(line + "\n")

	f.mu.Lock()
	defer f.mu.Unlock()

	for {
		if err := f.ensureConnected(); err != nil {
			return err // should not happen, ensureConnected retries forever
		}

		if _, err := f.conn.Write(data); err != nil {
			pfxlog.Logger().WithError(err).Warn("write failed, reconnecting")
			conn := f.conn
			f.conn = nil
			conn.Close()
			continue // retry on new connection
		}
		f.lastWrite.Store(time.Now())

		return nil
	}
}

// buildMatcher returns a function that reports whether a line matches any of
// the configured namespaces. If namespaces is empty, all lines match.
func buildMatcher(namespaces []string) func(string) bool {
	if len(namespaces) == 0 {
		return func(string) bool { return true }
	}

	// Pre-build the JSON snippets we're looking for, e.g. `"namespace":"apiSession"`
	patterns := make([]string, len(namespaces))
	for i, ns := range namespaces {
		patterns[i] = fmt.Sprintf(`"namespace":"%s"`, ns)
	}

	return func(line string) bool {
		for _, p := range patterns {
			if strings.Contains(line, p) {
				return true
			}
		}
		return false
	}
}
