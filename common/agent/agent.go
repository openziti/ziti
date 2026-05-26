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

// Package agent provides hooks programs can register to retrieve
// diagnostics data by using the Ziti CLI.
package agent

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"io"
	"net"
	"os"
	"os/signal"
	"path"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"runtime/trace"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	mu       sync.Mutex
	tmpfile  string
	listener net.Listener

	units = []string{" bytes", "KB", "MB", "GB", "TB", "PB"}

	Magic      = []byte{0x1, 0xB, 0xA, 0xD, 0xE, 0xC, 0xA, 0xF, 0xE, 0xF, 0x0, 0x0, 0xD}
	SockPrefix = "gops-agent"
)

// Options allows configuring the started agent.
type Options struct {
	// Addr is the host:port the agent will be listening at.
	// Optional.
	Addr string

	// AppId is a way to identify the host application
	AppId string

	// Type is the host application type
	AppType string

	// AppAlias is an alternate way to identify the host application
	AppAlias string

	// AppVersion is the application version
	AppVersion string

	// ConfigDir is the directory to store the configuration file,
	// PID of the gops process, filename, port as well as content.
	// Optional.
	ConfigDir string

	// ShutdownCleanup automatically cleans up resources if the
	// running process receives an interrupt. Otherwise, users
	// can call Close before shutting down.
	// Optional.
	ShutdownCleanup *bool

	// Custom Operations
	CustomOps map[byte]func(conn net.Conn) error
}

// Listen starts the gops agent on a host process. Once agent started, users
// can use the advanced gops features. The agent will listen to Interrupt
// signals and exit the process, if you need to perform further work on the
// Interrupt signal use the options parameter to configure the agent
// accordingly.
//
// Note: The agent exposes an endpoint via a TCP connection that can be used by
// any program on the system. Review your security requirements before starting
// the agent.
func Listen(opts Options) error {
	mu.Lock()
	defer mu.Unlock()

	if listener != nil {
		return errors.Errorf("gops: agent already listening at: %v", listener.Addr())
	}

	if opts.ShutdownCleanup == nil || *opts.ShutdownCleanup {
		gracefulShutdown()
	}

	addr := opts.Addr

	network := "tcp"

	if addr == "" {
		network = "unix"
		tmpfile = path.Join(os.TempDir(), fmt.Sprintf("%v.%v.sock", SockPrefix, os.Getpid()))
		if err := os.Remove(tmpfile); err != nil && !os.IsNotExist(err) {
			return err
		}
		addr = tmpfile
	} else {
		parts := strings.SplitN(addr, ":", 2)

		if len(parts) == 2 {
			network = parts[0]
			addr = parts[1]
		}
		if network == "unix" {
			tmpfile = addr
		}
	}

	var err error
	if listener, err = net.Listen(network, addr); err != nil {
		return err
	}

	if network == "unix" {
		if err := os.Chmod(addr, 0700); err != nil {
			_ = listener.Close()
			return err
		}
	}

	if opts.ConfigDir != "" {
		if err := os.MkdirAll(opts.ConfigDir, os.ModePerm); err != nil {
			return err
		}

		if tcpAddr, ok := listener.Addr().(*net.TCPAddr); ok {
			port := tcpAddr.Port
			tmpfile = fmt.Sprintf("%s/%d", opts.ConfigDir, os.Getpid())
			if err = os.WriteFile(tmpfile, []byte(strconv.Itoa(port)), os.ModePerm); err != nil {
				return err
			}
		}
	}

	h := &handler{
		options: opts,
	}

	go h.listen()
	return nil
}

type handler struct {
	options Options
}

func (self *handler) listen() {
	logger := pfxlog.Logger()
	defer func() {
		if err := listener.Close(); err != nil {
			logger.WithError(err).Error("error closing gops listener")
		}
	}()
	for {
		if conn, err := listener.Accept(); err != nil {
			logger.WithError(err).Error("error accepting gops connection, closing gops listener")
			break
		} else {
			self.handleConnection(conn)
		}
	}
}

func (self *handler) handleConnection(conn net.Conn) {
	async := false
	defer func() {
		if !async {
			_ = conn.Close()
		}
	}()

	logger := pfxlog.Logger()

	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		logger.WithError(err).Error("unable to set read deadline on gops connection")
		return
	}

	buf := make([]byte, len(Magic)+1)

	if _, err := io.ReadFull(conn, buf); err != nil {
		logger.WithError(err).Error("unable to read gops request")
		return
	}

	if !bytes.Equal(Magic, buf[:len(Magic)]) {
		logger.Error("invalid magic number on gops connection", hex.Dump(buf[:len(Magic)]))
		return
	}

	var err error
	if async, err = self.handle(conn, buf[len(buf)-1]); err != nil {
		logger.WithError(err).Error("error while processing gops request")
		_, _ = conn.Write([]byte(err.Error()))
	}
}

func gracefulShutdown() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		// cleanup the socket on shutdown.
		<-c
		Close()
	}()
}

// Close closes the agent, removing temporary files and closing the TCP listener.
// If no agent is listening, Close does nothing.
func Close() {
	mu.Lock()
	defer mu.Unlock()

	if listener != nil {
		_ = listener.Close()
	}

	if tmpfile != "" {
		_ = os.Remove(tmpfile)
	}
}

func formatBytes(val uint64) string {
	var i int
	var target uint64
	for i = range units {
		target = 1 << uint(10*(i+1))
		if val < target {
			break
		}
	}
	if i > 0 {
		return fmt.Sprintf("%0.2f%s (%d bytes)", float64(val)/(float64(target)/1024), units[i], val)
	}
	return fmt.Sprintf("%d bytes", val)
}

func (self *handler) handle(conn net.Conn, op byte) (bool, error) {
	switch op {
	case StackTrace:
		return false, pprof.Lookup("goroutine").WriteTo(conn, 2)
	case GC:
		runtime.GC()
		_, err := conn.Write([]byte("ok"))
		return false, err
	case MemStats:
		var s runtime.MemStats
		runtime.ReadMemStats(&s)
		_, _ = fmt.Fprintf(conn, "alloc: %v\n", formatBytes(s.Alloc))
		_, _ = fmt.Fprintf(conn, "total-alloc: %v\n", formatBytes(s.TotalAlloc))
		_, _ = fmt.Fprintf(conn, "sys: %v\n", formatBytes(s.Sys))
		_, _ = fmt.Fprintf(conn, "lookups: %v\n", s.Lookups)
		_, _ = fmt.Fprintf(conn, "mallocs: %v\n", s.Mallocs)
		_, _ = fmt.Fprintf(conn, "frees: %v\n", s.Frees)
		_, _ = fmt.Fprintf(conn, "heap-alloc: %v\n", formatBytes(s.HeapAlloc))
		_, _ = fmt.Fprintf(conn, "heap-sys: %v\n", formatBytes(s.HeapSys))
		_, _ = fmt.Fprintf(conn, "heap-idle: %v\n", formatBytes(s.HeapIdle))
		_, _ = fmt.Fprintf(conn, "heap-in-use: %v\n", formatBytes(s.HeapInuse))
		_, _ = fmt.Fprintf(conn, "heap-released: %v\n", formatBytes(s.HeapReleased))
		_, _ = fmt.Fprintf(conn, "heap-objects: %v\n", s.HeapObjects)
		_, _ = fmt.Fprintf(conn, "stack-in-use: %v\n", formatBytes(s.StackInuse))
		_, _ = fmt.Fprintf(conn, "stack-sys: %v\n", formatBytes(s.StackSys))
		_, _ = fmt.Fprintf(conn, "stack-mspan-inuse: %v\n", formatBytes(s.MSpanInuse))
		_, _ = fmt.Fprintf(conn, "stack-mspan-sys: %v\n", formatBytes(s.MSpanSys))
		_, _ = fmt.Fprintf(conn, "stack-mcache-inuse: %v\n", formatBytes(s.MCacheInuse))
		_, _ = fmt.Fprintf(conn, "stack-mcache-sys: %v\n", formatBytes(s.MCacheSys))
		_, _ = fmt.Fprintf(conn, "other-sys: %v\n", formatBytes(s.OtherSys))
		_, _ = fmt.Fprintf(conn, "gc-sys: %v\n", formatBytes(s.GCSys))
		_, _ = fmt.Fprintf(conn, "next-gc: when heap-alloc >= %v\n", formatBytes(s.NextGC))
		lastGC := "-"
		if s.LastGC != 0 {
			lastGC = fmt.Sprint(time.Unix(0, int64(s.LastGC)))
		}
		_, _ = fmt.Fprintf(conn, "last-gc: %v\n", lastGC)
		_, _ = fmt.Fprintf(conn, "gc-pause-total: %v\n", time.Duration(s.PauseTotalNs))
		_, _ = fmt.Fprintf(conn, "gc-pause: %v\n", s.PauseNs[(s.NumGC+255)%256])
		_, _ = fmt.Fprintf(conn, "num-gc: %v\n", s.NumGC)
		_, _ = fmt.Fprintf(conn, "enable-gc: %v\n", s.EnableGC)
		_, _ = fmt.Fprintf(conn, "debug-gc: %v\n", s.DebugGC)
	case Version:
		_, _ = fmt.Fprintf(conn, "%v\n", runtime.Version())
	case HeapProfile:
		_ = pprof.WriteHeapProfile(conn)
	case CPUProfile:
		if err := pprof.StartCPUProfile(conn); err != nil {
			return false, err
		}
		time.Sleep(30 * time.Second)
		pprof.StopCPUProfile()
	case Stats:
		_, _ = fmt.Fprintf(conn, "goroutines: %v\n", runtime.NumGoroutine())
		_, _ = fmt.Fprintf(conn, "OS threads: %v\n", pprof.Lookup("threadcreate").Count())
		_, _ = fmt.Fprintf(conn, "GOMAXPROCS: %v\n", runtime.GOMAXPROCS(0))
		_, _ = fmt.Fprintf(conn, "num CPU: %v\n", runtime.NumCPU())
	case BinaryDump:
		executable, err := os.Executable()
		if err != nil {
			return false, err
		}
		f, err := os.Open(executable)
		if err != nil {
			return false, err
		}
		defer func() { _ = f.Close() }()

		_, err = bufio.NewReader(f).WriteTo(conn)
		return false, err
	case Trace:
		if err := trace.Start(conn); err != nil {
			return false, err
		}
		time.Sleep(5 * time.Second)
		trace.Stop()
	case SetGCPercent:
		perc, err := binary.ReadVarint(bufio.NewReader(conn))
		if err != nil {
			return false, err
		}
		_, _ = fmt.Fprintf(conn, "New GC percent set to %v. Previous value was %v.\n", perc, debug.SetGCPercent(int(perc)))

	case AppInfo:
		result := map[string]string{}
		if self.options.AppType != "" {
			result["type"] = self.options.AppType
		}
		if self.options.AppId != "" {
			result["id"] = self.options.AppId
		}
		if self.options.AppAlias != "" {
			result["alias"] = self.options.AppAlias
		}
		if self.options.AppVersion != "" {
			result["version"] = self.options.AppVersion
		}
		marshalled, err := json.Marshal(result)
		if err != nil {
			return false, err
		}
		_, err = conn.Write(marshalled)
		return false, err

	case SetLogLevel:
		param, err := bufio.NewReader(conn).ReadByte()
		if err != nil {
			return false, err
		}
		if param < byte(logrus.PanicLevel) || param > byte(logrus.TraceLevel) {
			return false, errors.Errorf("invalid log level %v", param)
		}

		oldLevel := logrus.GetLevel()
		newLevel := logrus.Level(param)
		logrus.SetLevel(newLevel)

		pfxlog.Logger().Infof("log level set to from %v to %v", oldLevel, newLevel)
		_, _ = fmt.Fprintf(conn, "log level set from %v to %v\n", oldLevel, newLevel)

	case SetChannelLogLevel:
		reader := bufio.NewReader(conn)
		channel, err := self.readVarString(reader, 1024)
		if err != nil {
			return false, err
		}

		level, err := reader.ReadByte()
		if err != nil {
			return false, err
		}

		if level < byte(logrus.PanicLevel) || level > byte(logrus.TraceLevel) {
			return false, errors.Errorf("invalid log level %v", level)
		}

		var prevLevel string
		newLevel := logrus.Level(level)
		pfxlog.GlobalConfig(func(options *pfxlog.Options) *pfxlog.Options {
			if val, found := options.ChannelLogLevelOverrides[channel]; found {
				prevLevel = val.String()
			} else {
				prevLevel = "default"
			}
			options.SetChannelLogLevel(channel, newLevel)
			return options
		})

		pfxlog.Logger().Infof("log level for channel %v set from %v to %v", channel, prevLevel, newLevel)
		_, _ = fmt.Fprintf(conn, "log level for channel %v set from %v to %v\n", channel, prevLevel, newLevel)

	case ClearChannelLogLevel:
		reader := bufio.NewReader(conn)
		channel, err := self.readVarString(reader, 1024)
		if err != nil {
			return false, err
		}

		var prevLevel string
		pfxlog.GlobalConfig(func(options *pfxlog.Options) *pfxlog.Options {
			if val, found := options.ChannelLogLevelOverrides[channel]; found {
				prevLevel = val.String()
			} else {
				prevLevel = "default"
			}
			options.ClearChannelLogLevel(channel)
			return options
		})

		pfxlog.Logger().Infof("log level for channel %v cleared, was %v", channel, prevLevel)
		_, _ = fmt.Fprintf(conn, "log level for channel %v cleared, was %v\n", channel, prevLevel)

	case HeapDump:
		reader := bufio.NewReader(conn)
		outputFileName, err := self.readVarString(reader, 1024)
		if err != nil {
			return false, err
		}
		f, err := os.Create(outputFileName)
		if err != nil {
			return false, err
		}
		defer func() {
			if err = f.Close(); err != nil {
				pfxlog.Logger().WithError(err).Errorf("failed to close file %s", outputFileName)
			}
		}()

		debug.WriteHeapDump(f.Fd())
		pfxlog.Logger().Infof("heap dump save to %s", outputFileName)

	default:
		if self.options.CustomOps != nil {
			if customHandler, found := self.options.CustomOps[op]; found {
				if err := customHandler(conn); err != nil {
					return false, err
				}
				return op == CustomOpAsync, nil
			}
		}
	}
	return false, nil
}

func (self *handler) readVarString(reader *bufio.Reader, maxLen int64) (string, error) {
	strLen, err := binary.ReadVarint(reader)
	if err != nil {
		return "", err
	}

	if strLen > maxLen {
		return "", errors.Errorf("string exceeds max length of %v", maxLen)
	}

	buf := make([]byte, strLen)

	_, err = io.ReadFull(reader, buf)
	if err != nil {
		return "", err
	}

	return string(buf), nil
}
