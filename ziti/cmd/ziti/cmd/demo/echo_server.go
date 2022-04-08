/*
	Copyright NetFoundry, Inc.

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

package demo

import (
	"errors"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/sdk-golang/ziti/config"
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/common"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"io"
	"net"
)

type echoServer struct {
	port         uint16
	verbose      bool
	logFormatter string
	configFile   string
	service      string
}

func newEchoServerCmd(common.OptionsProvider) *cobra.Command {
	server := &echoServer{}

	cmd := &cobra.Command{
		Use:   "echo-server",
		Short: "Runs a simple network echo service",
		Args:  cobra.ExactArgs(0),
		Run:   server.run,
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	cmd.Flags().Uint16VarP(&server.port, "port", "p", 0, "Specify the port to listen on. If not specified the TCP listener won't be started")
	cmd.Flags().BoolVarP(&server.verbose, "verbose", "v", false, "Enable verbose logging")
	cmd.Flags().StringVar(&server.logFormatter, "log-formatter", "", "Specify log formatter [json|pfxlog|text]")
	cmd.Flags().StringVarP(&server.configFile, "identity", "i", "", "Specify the Ziti identity to use. If not specified the Ziti listener won't be started")
	cmd.Flags().StringVarP(&server.service, "service", "s", "echo", "Ziti service to bind. Defaults to 'echo'")
	cmd.Flags().SetInterspersed(true)

	return cmd
}

func (self *echoServer) initLogging() {
	logLevel := logrus.InfoLevel
	if self.verbose {
		logLevel = logrus.DebugLevel
	}

	options := pfxlog.DefaultOptions().SetTrimPrefix("github.com/openziti/").NoColor()
	pfxlog.GlobalInit(logLevel, options)

	switch self.logFormatter {
	case "pfxlog":
		pfxlog.SetFormatter(pfxlog.NewFormatter(pfxlog.DefaultOptions().SetTrimPrefix("github.com/openziti/").StartingToday()))
	case "json":
		pfxlog.SetFormatter(&logrus.JSONFormatter{TimestampFormat: "2006-01-02T15:04:05.000Z"})
	case "text":
		pfxlog.SetFormatter(&logrus.TextFormatter{})
	default:
		// let logrus do its own thing
	}
}

func (self *echoServer) run(*cobra.Command, []string) {
	self.initLogging()

	log := pfxlog.Logger()

	if self.port == 0 && self.configFile == "" {
		log.Fatalf("No port specified and no ziti identity specified. Not starting either listener, so nothing to do. exiting.")
	}

	if self.port > 0 {
		addr := fmt.Sprintf("0.0.0.0:%v", self.port)
		log.Infof("tcp: starting listener on address [%v]", addr)
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			log.WithError(err).Fatalf("unable to start TCP listener on addr [%v]", addr)
		}

		go self.accept("tcp", listener)
	}

	if self.configFile != "" {
		zitiConfig, err := config.NewFromFile(self.configFile)
		if err != nil {
			log.WithError(err).Fatalf("ziti: unable to load ziti identity from [%v]", self.configFile)
		}

		zitiContext := ziti.NewContextWithConfig(zitiConfig)

		log.Infof("ziti: starting listener for service (%v)", self.service)
		listener, err := zitiContext.Listen(self.service)
		if err != nil {
			log.WithError(err).Fatalf("ziti: failed to host service [%v]", self.service)
		}

		go self.accept("ziti", listener)
	}

	waitC := make(chan struct{})
	<-waitC
}

func (self *echoServer) accept(connType string, listener net.Listener) {
	log := pfxlog.Logger()

	for {
		conn, err := listener.Accept()
		if err != nil {
			panic(err)
		}
		log.Infof("%v: connection received: %v", connType, conn)
		go self.echo(connType, conn)
	}
}

func (self *echoServer) echo(connType string, conn io.ReadWriteCloser) {
	log := pfxlog.Logger()

	defer func() {
		if err := conn.Close(); err != nil {
			log.WithError(err).Errorf("%v: Error while closing connection", connType)
		}
		log.Infof("%v: connection closed", connType)
	}()

	for {
		buf := make([]byte, 1024)
		n, err := conn.Read(buf)
		if err != nil {
			if errors.Is(err, io.EOF) {
				log.Infof("%v: received eof, closing connection", connType)
			} else {
				log.WithError(err).Errorf("%v: error while reading", connType)
			}
			break
		}
		log.Infof("%v: read %v bytes", connType, n)
		writeBuf := buf[:n]
		if _, err := conn.Write(writeBuf); err != nil {
			log.WithError(err).Errorf("%v: error while writing", connType)
			break
		}
		log.Infof("%v: wrote %v bytes", connType, n)
	}
}
